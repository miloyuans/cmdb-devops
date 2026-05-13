package httpapi

import (
	"context"
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
	r.Use(cors.New(cors.Config{AllowOrigins: []string{"*"}, AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}, AllowHeaders: []string{"Authorization", "Content-Type"}}))
	r.Static("/static", "./web/static")
	r.GET("/", func(c *gin.Context) { c.File("./web/static/index.html") })
	r.POST("/api/login", s.login)
	r.POST("/api/telegram/webhook", s.telegramWebhook)
	api := r.Group("/api", s.authRequired())
	api.GET("/me", s.me)
	api.GET("/accounts", s.listAccounts)
	api.POST("/accounts", s.adminOnly(), s.upsertAccount)
	api.PUT("/accounts/:id", s.adminOnly(), s.upsertAccount)
	api.POST("/accounts/:id/jobs/:type", s.adminOnly(), s.triggerAccountJob)
	api.GET("/jobs", s.listJobs)
	api.POST("/query/ip", s.queryIP)
	api.POST("/query/connectivity", s.connectivity)
	api.GET("/identity/access-keys", s.listAccessKeys)
	api.POST("/identity/access-keys/lookup", s.lookupAK)
	api.GET("/telegram/config", s.adminOnly(), s.getTelegramConfig)
	api.PUT("/telegram/config", s.adminOnly(), s.putTelegramConfig)
	api.GET("/telegram/chats", s.adminOnly(), s.listTelegramChats)
	api.POST("/telegram/chats", s.adminOnly(), s.putTelegramChat)
	api.GET("/users", s.adminOnly(), s.listUsers)
	api.POST("/users", s.adminOnly(), s.upsertUser)
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
	c.JSON(200, gin.H{"token": tok, "user": u})
}

func (s *Server) me(c *gin.Context) { c.JSON(200, c.MustGet("claims")) }

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
	if req.ID == "" {
		req.ID = "user_" + req.Username
	}
	if req.Role == "" {
		req.Role = model.RoleViewer
	}
	now := time.Now().UTC()
	u := model.User{ID: req.ID, Username: req.Username, Role: req.Role, Enabled: req.Enabled, TelegramID: req.TelegramID, CreatedAt: now, UpdatedAt: now}
	if req.Password != "" {
		h, _ := auth.HashPassword(req.Password)
		u.PasswordHash = h
	} else if old, _ := s.Store.FindUserByUsername(c.Request.Context(), req.Username); old != nil {
		u.PasswordHash = old.PasswordHash
		u.CreatedAt = old.CreatedAt
	}
	respond(c, u, s.Store.UpsertUser(c.Request.Context(), u))
}

func (s *Server) listAccounts(c *gin.Context) {
	accs, err := s.Store.ListAccounts(c.Request.Context(), false)
	respond(c, accs, err)
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
	if req.ID == "" {
		req.ID = "acc_" + req.Provider + "_" + req.Alias
	}
	if req.RegionMode == "" {
		req.RegionMode = "auto"
	}
	now := time.Now().UTC()
	old, _ := s.Store.GetAccount(c.Request.Context(), req.ID)
	secretEnc := ""
	accessKeyID := req.AccessKeyID
	if old != nil {
		secretEnc = old.Credential.AccessKeySecretEnc
		if accessKeyID == "" {
			accessKeyID = old.Credential.AccessKeyID
		}
	}
	if req.AccessKeySecret != "" {
		secretEnc, _ = security.Encrypt(req.AccessKeySecret, s.Cfg.EncryptionKey)
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
	c.JSON(200, acc)
}

func (s *Server) triggerAccountJob(c *gin.Context) {
	jobType := c.Param("type")
	id := c.Param("id")
	claims := c.MustGet("claims").(*auth.Claims)
	ok, jobID, err := s.Jobs.TriggerAccountJob(c.Request.Context(), jobType, id, claims.Username)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(409, gin.H{"error": "job already running"})
		return
	}
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

func (s *Server) listAccessKeys(c *gin.Context) {
	keys, err := s.Store.ListAccessKeys(c.Request.Context(), c.Query("account_alias"), c.Query("status"))
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
	if err == nil && idx == nil {
		s.Jobs.RequestMissRefresh(context.Background(), "identity_sync", "all")
	}
	respond(c, idx, err)
}

func (s *Server) getTelegramConfig(c *gin.Context) {
	cfg, err := s.Store.GetTelegramConfig(c.Request.Context())
	respond(c, cfg, err)
}

func (s *Server) putTelegramConfig(c *gin.Context) {
	var req struct {
		Enabled         bool   `json:"enabled"`
		Mode            string `json:"mode"`
		BotName         string `json:"bot_name"`
		BotToken        string `json:"bot_token"`
		BotTokenEnv     string `json:"bot_token_env"`
		WebhookURL      string `json:"webhook_url"`
		ParseMode       string `json:"parse_mode"`
		RateLimitPerSec int    `json:"rate_limit_per_second"`
		MaxWorkers      int    `json:"max_workers"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	old, _ := s.Store.GetTelegramConfig(c.Request.Context())
	enc := ""
	version := int64(1)
	if old != nil {
		enc = old.BotTokenEnc
		version = old.Version + 1
	}
	if req.BotToken != "" {
		enc, _ = security.Encrypt(req.BotToken, s.Cfg.EncryptionKey)
	}
	claims := c.MustGet("claims").(*auth.Claims)
	cfg := model.TelegramConfig{ID: "default", Enabled: req.Enabled, Mode: req.Mode, BotName: req.BotName, BotTokenEnc: enc, BotTokenEnv: req.BotTokenEnv, WebhookURL: req.WebhookURL, ParseMode: req.ParseMode, RateLimitPerSec: req.RateLimitPerSec, MaxWorkers: req.MaxWorkers, Version: version, UpdatedBy: claims.Username, UpdatedAt: time.Now().UTC()}
	respond(c, cfg, s.Store.UpsertTelegramConfig(c.Request.Context(), cfg))
}

func (s *Server) listTelegramChats(c *gin.Context) {
	chats, err := s.Store.ListTelegramChats(c.Request.Context())
	respond(c, chats, err)
}

func (s *Server) putTelegramChat(c *gin.Context) {
	var chat model.TelegramChat
	if err := c.BindJSON(&chat); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if chat.ID == "" {
		chat.ID = "chat_" + strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(stringInt(chat.ChatID)), "-", "n"), "+")
	}
	now := time.Now().UTC()
	if chat.CreatedAt.IsZero() {
		chat.CreatedAt = now
	}
	chat.UpdatedAt = now
	respond(c, chat, s.Store.UpsertTelegramChat(c.Request.Context(), chat))
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

func stringInt(v int64) string { return strconv.FormatInt(v, 10) }
