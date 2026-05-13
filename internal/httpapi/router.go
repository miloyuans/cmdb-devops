package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cmdb-devops/internal/auth"
	"cmdb-devops/internal/config"
	"cmdb-devops/internal/jobs"
	"cmdb-devops/internal/model"
	"cmdb-devops/internal/security"
	"cmdb-devops/internal/service"
	"cmdb-devops/internal/store"
	"cmdb-devops/internal/telegram"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type Server struct {
	Cfg      config.Config
	Store    *store.Store
	Jobs     *jobs.Manager
	Query    *service.QueryService
	Telegram *telegram.Service
}

func (s *Server) Router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(cors.New(cors.Config{AllowOrigins: []string{"*"}, AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}, AllowHeaders: []string{"Authorization", "Content-Type"}}))
	r.Static("/static", "./web/static")
	r.GET("/", func(c *gin.Context) { c.File("./web/static/index.html") })
	r.POST("/api/login", s.login)
	r.POST("/api/telegram/webhook", s.telegramWebhook)

	api := r.Group("/api", s.authRequired())
	api.GET("/me", s.me)
	api.PUT("/me/profile", s.updateMyProfile)
	api.GET("/cloud/platforms", s.platforms)

	api.GET("/accounts", s.listAccounts)
	api.GET("/accounts/:id", s.getAccount)
	api.POST("/accounts", s.adminOnly(), s.upsertAccount)
	api.PUT("/accounts/:id", s.adminOnly(), s.upsertAccount)
	api.POST("/accounts/:id/jobs/:type", s.adminOnly(), s.triggerAccountJob)

	api.GET("/jobs", s.listJobs)
	api.POST("/query/ip", s.queryIP)
	api.POST("/query/connectivity", s.connectivity)
	api.GET("/identity/users", s.listIdentityUsers)
	api.GET("/identity/access-keys", s.listAccessKeys)
	api.POST("/identity/access-keys/lookup", s.lookupAK)

	api.GET("/telegram/bots", s.adminOnly(), s.listTelegramBots)
	api.POST("/telegram/bots", s.adminOnly(), s.upsertTelegramBot)
	api.PUT("/telegram/bots/:id", s.adminOnly(), s.upsertTelegramBot)
	api.GET("/telegram/chats", s.adminOnly(), s.listTelegramChats)
	api.POST("/telegram/chats", s.adminOnly(), s.upsertTelegramChat)
	api.PUT("/telegram/chats/:id", s.adminOnly(), s.upsertTelegramChat)
	api.GET("/telegram/users", s.adminOnly(), s.listTelegramUsers)
	api.POST("/telegram/users", s.adminOnly(), s.upsertTelegramUser)
	api.PUT("/telegram/users/:id", s.adminOnly(), s.upsertTelegramUser)
	// Backward-compatible config endpoint; maps to default bot.
	api.GET("/telegram/config", s.adminOnly(), s.getTelegramConfig)
	api.PUT("/telegram/config", s.adminOnly(), s.putTelegramConfig)

	api.GET("/users", s.adminOnly(), s.listUsers)
	api.POST("/users", s.adminOnly(), s.upsertUser)
	api.PUT("/users/:id", s.adminOnly(), s.upsertUser)
	api.DELETE("/users/:id", s.adminOnly(), s.deleteUser)

	api.GET("/settings", s.adminOnly(), s.getSettings)
	api.PUT("/settings", s.adminOnly(), s.putSettings)
	api.GET("/audit", s.adminOnly(), s.listAudit)
	return r
}

func (s *Server) authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		claims, err := auth.ParseToken(s.Cfg.JWTSecret, strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		if user, err := s.Store.GetUserByID(c.Request.Context(), claims.UserID); err != nil || user == nil || !user.Enabled {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user disabled or not found"})
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}

func (s *Server) adminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := c.MustGet("claims").(*auth.Claims)
		if claims.Role != model.RoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin required"})
			return
		}
		c.Next()
	}
}

func (s *Server) current(c *gin.Context) *auth.Claims { return c.MustGet("claims").(*auth.Claims) }

func (s *Server) audit(ctx context.Context, actor, action, target string, meta map[string]any) {
	_ = s.Store.InsertAudit(ctx, model.AuditLog{ID: fmt.Sprintf("audit_%d", time.Now().UnixNano()), Actor: actor, Action: action, Target: target, Meta: meta, CreatedAt: time.Now().UTC()})
}

func (s *Server) login(c *gin.Context) {
	var req struct{ Username, Password string }
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	u, err := s.Store.FindUserByUsername(c.Request.Context(), req.Username)
	if err != nil || u == nil || !u.Enabled || !auth.CheckPassword(u.PasswordHash, req.Password) {
		c.JSON(401, gin.H{"error": "bad credentials"})
		return
	}
	tok, err := auth.CreateToken(s.Cfg.JWTSecret, *u)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	s.audit(c.Request.Context(), u.Username, "login", u.ID, nil)
	c.JSON(200, gin.H{"token": tok, "user": u})
}

func (s *Server) me(c *gin.Context) {
	claims := s.current(c)
	u, err := s.Store.GetUserByID(c.Request.Context(), claims.UserID)
	respond(c, u, err)
}

func (s *Server) updateMyProfile(c *gin.Context) {
	claims := s.current(c)
	u, err := s.Store.GetUserByID(c.Request.Context(), claims.UserID)
	if err != nil || u == nil {
		respond(c, nil, fmt.Errorf("user not found"))
		return
	}
	var req struct {
		Password   string `json:"password"`
		TelegramID int64  `json:"telegram_user_id"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Password != "" {
		h, err := auth.HashPassword(req.Password)
		if err != nil {
			respond(c, nil, err)
			return
		}
		u.PasswordHash = h
	}
	if req.TelegramID != 0 {
		u.TelegramID = req.TelegramID
	}
	u.UpdatedAt = time.Now().UTC()
	err = s.Store.UpsertUser(c.Request.Context(), *u)
	s.audit(c.Request.Context(), claims.Username, "profile.update", u.ID, nil)
	respond(c, u, err)
}

func (s *Server) platforms(c *gin.Context) {
	c.JSON(200, []gin.H{{"id": "aws", "name": "AWS"}, {"id": "aliyun", "name": "阿里云"}})
}

func (s *Server) listUsers(c *gin.Context) {
	users, err := s.Store.ListUsers(c.Request.Context())
	respond(c, users, err)
}

func (s *Server) upsertUser(c *gin.Context) {
	var req struct {
		ID, Username, Password string
		Role                   model.Role
		Enabled                bool
		TelegramID             int64
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Username == "" {
		c.JSON(400, gin.H{"error": "username required"})
		return
	}
	if req.ID == "" {
		req.ID = c.Param("id")
	}
	if req.ID == "" {
		req.ID = "user_" + strings.ReplaceAll(req.Username, " ", "_")
	}
	if req.Role == "" {
		req.Role = model.RoleViewer
	}
	if req.Role != model.RoleViewer && req.Role != model.RoleAdmin {
		c.JSON(400, gin.H{"error": "role must be viewer or admin"})
		return
	}
	var old *model.User
	if req.ID != "" {
		old, _ = s.Store.GetUserByID(c.Request.Context(), req.ID)
	}
	if old == nil {
		old, _ = s.Store.FindUserByUsername(c.Request.Context(), req.Username)
	}
	now := time.Now().UTC()
	u := model.User{ID: req.ID, Username: req.Username, Role: req.Role, Enabled: req.Enabled, TelegramID: req.TelegramID, CreatedAt: now, UpdatedAt: now}
	if req.Password != "" {
		h, err := auth.HashPassword(req.Password)
		if err != nil {
			respond(c, nil, err)
			return
		}
		u.PasswordHash = h
	} else if old != nil {
		u.PasswordHash = old.PasswordHash
		u.CreatedAt = old.CreatedAt
	} else {
		h, _ := auth.HashPassword("ChangeMe123!")
		u.PasswordHash = h
	}
	err := s.Store.UpsertUser(c.Request.Context(), u)
	s.audit(c.Request.Context(), s.current(c).Username, "user.upsert", u.ID, map[string]any{"role": u.Role, "enabled": u.Enabled})
	respond(c, u, err)
}

func (s *Server) deleteUser(c *gin.Context) {
	id := c.Param("id")
	if id == s.current(c).UserID {
		c.JSON(400, gin.H{"error": "cannot delete current user"})
		return
	}
	err := s.Store.DeleteUser(c.Request.Context(), id)
	s.audit(c.Request.Context(), s.current(c).Username, "user.delete", id, nil)
	respond(c, gin.H{"deleted": id}, err)
}

func (s *Server) listAccounts(c *gin.Context) {
	accs, err := s.Store.ListAccounts(c.Request.Context(), false)
	respond(c, accs, err)
}

func (s *Server) getAccount(c *gin.Context) {
	acc, err := s.Store.GetAccount(c.Request.Context(), c.Param("id"))
	respond(c, acc, err)
}

func (s *Server) upsertAccount(c *gin.Context) {
	var req struct {
		ID              string   `json:"id"`
		Provider        string   `json:"provider"`
		Alias           string   `json:"alias"`
		AccountID       string   `json:"account_id"`
		AccessKeyID     string   `json:"access_key_id"`
		AccessKeySecret string   `json:"access_key_secret"`
		Enabled         bool     `json:"enabled"`
		RegionMode      string   `json:"region_mode"`
		SelectedRegions []string `json:"selected_regions"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	if req.Provider != "aws" && req.Provider != "aliyun" {
		c.JSON(400, gin.H{"error": "provider must be aws or aliyun"})
		return
	}
	if strings.TrimSpace(req.Alias) == "" {
		c.JSON(400, gin.H{"error": "alias required"})
		return
	}
	if req.ID == "" {
		req.ID = c.Param("id")
	}
	if req.ID == "" {
		req.ID = "acc_" + req.Provider + "_" + strings.ReplaceAll(req.Alias, " ", "_")
	}
	if req.RegionMode == "" {
		req.RegionMode = "auto"
	}
	now := time.Now().UTC()
	old, _ := s.Store.GetAccount(c.Request.Context(), req.ID)
	secretEnc := ""
	accessKeyID := strings.TrimSpace(req.AccessKeyID)
	if old != nil {
		secretEnc = old.Credential.AccessKeySecretEnc
		if accessKeyID == "" {
			accessKeyID = old.Credential.AccessKeyID
		}
	}
	if req.AccessKeySecret != "" {
		enc, err := security.Encrypt(req.AccessKeySecret, s.Cfg.EncryptionKey)
		if err != nil {
			respond(c, nil, err)
			return
		}
		secretEnc = enc
	}
	acc := model.CloudAccount{ID: req.ID, Provider: req.Provider, Alias: req.Alias, AccountID: req.AccountID, Credential: model.Credential{AccessKeyID: accessKeyID, AccessKeySecretEnc: secretEnc, SecretProvided: secretEnc != ""}, Enabled: req.Enabled, RegionMode: req.RegionMode, SelectedRegions: req.SelectedRegions, CreatedAt: now, UpdatedAt: now}
	if old != nil {
		acc.CreatedAt = old.CreatedAt
		acc.DetectedRegions = old.DetectedRegions
		acc.EffectiveRegions = old.EffectiveRegions
		acc.LastSyncAt = old.LastSyncAt
		acc.LastSyncStatus = old.LastSyncStatus
	}
	if acc.RegionMode == "manual" {
		acc.EffectiveRegions = acc.SelectedRegions
	}
	if err := s.Store.UpsertAccount(c.Request.Context(), acc); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	s.audit(c.Request.Context(), s.current(c).Username, "cloud_account.upsert", acc.ID, map[string]any{"provider": acc.Provider, "alias": acc.Alias})
	c.JSON(200, acc)
}

func (s *Server) triggerAccountJob(c *gin.Context) {
	jobType := c.Param("type")
	id := c.Param("id")
	claims := s.current(c)
	ok, jobID, err := s.Jobs.TriggerAccountJob(c.Request.Context(), jobType, id, claims.Username)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(409, gin.H{"error": "job already running"})
		return
	}
	s.audit(c.Request.Context(), claims.Username, "job.trigger", id, map[string]any{"job_type": jobType, "job_id": jobID})
	c.JSON(202, gin.H{"job_id": jobID, "status": "running"})
}

func (s *Server) listJobs(c *gin.Context) {
	jobs, err := s.Store.ListJobs(c.Request.Context(), 100)
	respond(c, jobs, err)
}

func (s *Server) queryIP(c *gin.Context) {
	var req struct {
		Query string `json:"query"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	res, err := s.Query.SearchIP(c.Request.Context(), req.Query)
	if err == nil && res != nil && !res.CacheHit {
		s.Jobs.RequestMissRefresh(context.Background(), "inventory_sync", "all")
	}
	respond(c, res, err)
}

func (s *Server) connectivity(c *gin.Context) {
	var req service.ConnectivityRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	res, err := s.Query.AnalyzeConnectivity(c.Request.Context(), req)
	respond(c, res, err)
}

func (s *Server) listIdentityUsers(c *gin.Context) {
	users, err := s.Store.ListIAMUsers(c.Request.Context(), c.Query("provider"), c.Query("account_alias"))
	respond(c, users, err)
}

func (s *Server) listAccessKeys(c *gin.Context) {
	keys, err := s.Store.ListAccessKeys(c.Request.Context(), c.Query("provider"), c.Query("account_alias"), c.Query("status"), c.Query("enabled"))
	respond(c, keys, err)
}

func (s *Server) lookupAK(c *gin.Context) {
	var req struct {
		AccessKeyID string `json:"access_key_id"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	idx, err := s.Store.FindAccessKeyGlobal(c.Request.Context(), security.HashAccessKeyID(req.AccessKeyID))
	if err != nil {
		respond(c, nil, err)
		return
	}
	if idx == nil {
		s.Jobs.RequestMissRefresh(context.Background(), "identity_sync", "all")
		c.JSON(200, gin.H{"found": false, "message": "AK not found in current cache. identity_sync miss refresh has been requested with debounce."})
		return
	}
	user, userErr := s.Store.FindIAMUserByNameInDB(c.Request.Context(), idx.AccountDB, idx.OwnerUserName)
	if userErr != nil {
		respond(c, nil, userErr)
		return
	}
	c.JSON(200, gin.H{"found": true, "access_key": idx, "owner_user": user})
}

func (s *Server) listTelegramBots(c *gin.Context) {
	bots, err := s.Store.ListTelegramBots(c.Request.Context())
	respond(c, bots, err)
}

func (s *Server) upsertTelegramBot(c *gin.Context) {
	var req struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Enabled         bool   `json:"enabled"`
		Mode            string `json:"mode"`
		BotToken        string `json:"bot_token"`
		TokenEnv        string `json:"token_env"`
		WebhookURL      string `json:"webhook_url"`
		ParseMode       string `json:"parse_mode"`
		RateLimitPerSec int    `json:"rate_limit_per_second"`
		MaxWorkers      int    `json:"max_workers"`
		IsDefault       bool   `json:"is_default"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" {
		req.Name = "CMDB DevOps Bot"
	}
	if req.ID == "" {
		req.ID = c.Param("id")
	}
	if req.ID == "" {
		req.ID = "bot_" + strings.ReplaceAll(strings.ToLower(req.Name), " ", "_")
	}
	old, _ := s.Store.GetTelegramBot(c.Request.Context(), req.ID)
	now := time.Now().UTC()
	bot := model.TelegramBot{ID: req.ID, Name: req.Name, Enabled: req.Enabled, Mode: req.Mode, TokenEnv: req.TokenEnv, WebhookURL: req.WebhookURL, ParseMode: req.ParseMode, RateLimitPerSec: req.RateLimitPerSec, MaxWorkers: req.MaxWorkers, IsDefault: req.IsDefault, UpdatedBy: s.current(c).Username, CreatedAt: now, UpdatedAt: now}
	if bot.Mode == "" {
		bot.Mode = "webhook"
	}
	if bot.ParseMode == "" {
		bot.ParseMode = "PlainText"
	}
	if bot.RateLimitPerSec == 0 {
		bot.RateLimitPerSec = 20
	}
	if bot.MaxWorkers == 0 {
		bot.MaxWorkers = 4
	}
	if old != nil {
		bot.TokenEnc = old.TokenEnc
		bot.CreatedAt = old.CreatedAt
	}
	if req.BotToken != "" {
		enc, err := security.Encrypt(req.BotToken, s.Cfg.EncryptionKey)
		if err != nil {
			respond(c, nil, err)
			return
		}
		bot.TokenEnc = enc
	}
	err := s.Store.UpsertTelegramBot(c.Request.Context(), bot)
	s.audit(c.Request.Context(), s.current(c).Username, "telegram_bot.upsert", bot.ID, map[string]any{"enabled": bot.Enabled, "default": bot.IsDefault})
	respond(c, bot, err)
}

func (s *Server) getTelegramConfig(c *gin.Context) {
	bot, err := s.Store.GetDefaultTelegramBot(c.Request.Context())
	respond(c, bot, err)
}

func (s *Server) putTelegramConfig(c *gin.Context) { s.upsertTelegramBot(c) }

func (s *Server) listTelegramChats(c *gin.Context) {
	chats, err := s.Store.ListTelegramChats(c.Request.Context())
	respond(c, chats, err)
}

func (s *Server) upsertTelegramChat(c *gin.Context) {
	var chat model.TelegramChat
	if err := c.BindJSON(&chat); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if chat.ID == "" {
		chat.ID = c.Param("id")
	}
	if chat.ID == "" {
		chat.ID = "chat_" + strings.TrimPrefix(strings.ReplaceAll(strconv.FormatInt(chat.ChatID, 10), "-", "n"), "+")
	}
	now := time.Now().UTC()
	if chat.CreatedAt.IsZero() {
		chat.CreatedAt = now
	}
	chat.UpdatedAt = now
	err := s.Store.UpsertTelegramChat(c.Request.Context(), chat)
	s.audit(c.Request.Context(), s.current(c).Username, "telegram_chat.upsert", chat.ID, map[string]any{"chat_id": chat.ChatID, "enabled": chat.Enabled})
	respond(c, chat, err)
}

func (s *Server) listTelegramUsers(c *gin.Context) {
	users, err := s.Store.ListTelegramUsers(c.Request.Context())
	respond(c, users, err)
}

func (s *Server) upsertTelegramUser(c *gin.Context) {
	var user model.TelegramAllowedUser
	if err := c.BindJSON(&user); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if user.ID == "" {
		user.ID = c.Param("id")
	}
	if user.ID == "" {
		user.ID = fmt.Sprintf("tguser_%d", user.TelegramUserID)
	}
	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	user.UpdatedAt = now
	err := s.Store.UpsertTelegramUser(c.Request.Context(), user)
	s.audit(c.Request.Context(), s.current(c).Username, "telegram_user.upsert", user.ID, map[string]any{"telegram_user_id": user.TelegramUserID, "enabled": user.Enabled})
	respond(c, user, err)
}

func (s *Server) getSettings(c *gin.Context) {
	cfg, err := s.Store.GetSettings(c.Request.Context())
	respond(c, cfg, err)
}

func (s *Server) putSettings(c *gin.Context) {
	var cfg model.SystemSettings
	if err := c.BindJSON(&cfg); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if cfg.ID == "" {
		cfg.ID = "default"
	}
	if cfg.SiteName == "" {
		cfg.SiteName = "CMDB DevOps"
	}
	cfg.UpdatedBy = s.current(c).Username
	cfg.UpdatedAt = time.Now().UTC()
	err := s.Store.UpsertSettings(c.Request.Context(), cfg)
	s.audit(c.Request.Context(), s.current(c).Username, "settings.update", cfg.ID, nil)
	respond(c, cfg, err)
}

func (s *Server) listAudit(c *gin.Context) {
	limit := int64(200)
	if v, err := strconv.ParseInt(c.Query("limit"), 10, 64); err == nil && v > 0 {
		limit = v
	}
	logs, err := s.Store.ListAudit(c.Request.Context(), limit)
	respond(c, logs, err)
}

func (s *Server) telegramWebhook(c *gin.Context) {
	var upd telegram.Update
	if err := c.BindJSON(&upd); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := s.Telegram.HandleUpdate(c.Request.Context(), upd); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func respond(c *gin.Context, payload any, err error) {
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, payload)
}
