package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"cmdb-devops/internal/config"
	"cmdb-devops/internal/model"
	"cmdb-devops/internal/security"
	"cmdb-devops/internal/service"
	"cmdb-devops/internal/store"
)

type Service struct {
	Store  *store.Store
	Query  *service.QueryService
	Config config.Config
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from,omitempty"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text,omitempty"`
}

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}
type Chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

func (s *Service) HandleUpdate(ctx context.Context, upd Update) error {
	if upd.Message == nil || strings.TrimSpace(upd.Message.Text) == "" {
		return nil
	}
	msg := upd.Message
	if !s.Store.TelegramChatAllowed(ctx, msg.Chat.ID) {
		return nil
	}
	if msg.From != nil && !s.Store.TelegramUserAllowed(ctx, msg.From.ID) {
		return s.SendMessage(ctx, msg.Chat.ID, "当前 Telegram 用户未被允许使用 CMDB DevOps Bot。")
	}
	text := strings.TrimSpace(msg.Text)
	if strings.HasPrefix(text, "/list") {
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/list"))
		if arg == "" {
			return s.startSession(ctx, msg, "/list", "waiting_ip_input", "请输入 IP 地址或地址段，例如：\n10.0.1.12\n8.8.8.8\n2001:db8::1\n10.0.0.0/24")
		}
		return s.replyIP(ctx, msg.Chat.ID, arg)
	}
	if strings.HasPrefix(text, "/ak") {
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/ak"))
		if arg == "" {
			return s.startSession(ctx, msg, "/ak", "waiting_ak_input", "请输入 AccessKeyId，例如：\nAKIA...\nLTAI...")
		}
		return s.replyAK(ctx, msg.Chat.ID, arg)
	}
	if strings.HasPrefix(text, "/help") {
		return s.SendMessage(ctx, msg.Chat.ID, "CMDB DevOps 指令：\n/list 查询 IP 或 CIDR\n/ak 反查 AccessKeyId\n/help 查看帮助")
	}
	ses, _ := s.Store.FindTelegramSession(ctx, msg.Chat.ID, msg.FromID())
	if ses == nil {
		return nil
	}
	if ses.State == "waiting_ip_input" {
		_ = s.completeSession(ctx, ses)
		return s.replyIP(ctx, msg.Chat.ID, text)
	}
	if ses.State == "waiting_ak_input" {
		_ = s.completeSession(ctx, ses)
		return s.replyAK(ctx, msg.Chat.ID, text)
	}
	return nil
}

func (m Message) FromID() int64 {
	if m.From == nil {
		return 0
	}
	return m.From.ID
}

func (s *Service) startSession(ctx context.Context, msg *Message, command, state, prompt string) error {
	id := fmt.Sprintf("tg_%d_%d_%s", msg.Chat.ID, msg.FromID(), command)
	ses := model.TelegramSession{ID: id, ChatID: msg.Chat.ID, TelegramUserID: msg.FromID(), Command: command, State: state, CreatedAt: time.Now().UTC(), ExpireAt: time.Now().UTC().Add(2 * time.Minute)}
	_ = s.Store.UpsertTelegramSession(ctx, ses)
	return s.SendMessage(ctx, msg.Chat.ID, prompt)
}

func (s *Service) completeSession(ctx context.Context, ses *model.TelegramSession) error {
	ses.State = "completed"
	return s.Store.UpsertTelegramSession(ctx, *ses)
}

func (s *Service) replyIP(ctx context.Context, chatID int64, input string) error {
	res, err := s.Query.SearchIP(ctx, input)
	if err != nil {
		return s.SendMessage(ctx, chatID, "查询参数错误："+err.Error())
	}
	if len(res.Matches) == 0 {
		return s.SendMessage(ctx, chatID, "当前缓存未命中："+res.Query.Raw+"\n已记录 miss，可在 Web 触发同步或等待后台同步。")
	}
	var b strings.Builder
	b.WriteString("CMDB DevOps 查询结果\n")
	b.WriteString("查询：" + res.Query.Raw + "\n")
	b.WriteString("类型：IPv")
	b.WriteString(fmt.Sprintf("%d / %s\n", res.Query.Version, res.Query.IPType))
	max := 5
	if len(res.Matches) < max {
		max = len(res.Matches)
	}
	b.WriteString(fmt.Sprintf("命中：%d 个，展示前 %d 个\n\n", len(res.Matches), max))
	for i := 0; i < max; i++ {
		m := res.Matches[i]
		b.WriteString(fmt.Sprintf("[%d] %s / %s / %s\n资源：%s %s\n名称：%s\n状态：%s\nVPC：%s\n子网：%s\n安全组：%s\nLB：%s\nNAT：%s\n\n", i+1, strings.ToUpper(m.Provider), m.AccountAlias, m.Region, m.ResourceType, m.ResourceID, m.ResourceName, m.State, m.VpcID, m.SubnetID, strings.Join(m.SecurityGroupIDs, ","), strings.Join(m.LoadBalancerIDs, ","), strings.Join(m.NatGatewayIDs, ",")))
	}
	b.WriteString("详情：" + s.Config.PublicBaseURL + "/#/ip?q=" + res.Query.Raw)
	return s.SendMessage(ctx, chatID, b.String())
}

func (s *Service) replyAK(ctx context.Context, chatID int64, input string) error {
	hash := security.HashAccessKeyID(input)
	idx, err := s.Store.FindAccessKeyGlobal(ctx, hash)
	if err != nil {
		return s.SendMessage(ctx, chatID, "查询失败："+err.Error())
	}
	if idx == nil {
		return s.SendMessage(ctx, chatID, "没有在当前已采集账户中找到该 AK。可能原因：不属于已配置账户、身份同步未执行、AK 已删除或权限不足。")
	}
	last := "从未使用/未获取到"
	if idx.LastUsedDate != nil {
		last = idx.LastUsedDate.Format("2006-01-02 15:04:05")
	}
	body := fmt.Sprintf("AK 反查结果\nAK：%s\n云平台：%s\n账户：%s / %s\n归属用户：%s\n状态：%s\n创建时间：%s\n最近使用：%s\n最近服务：%s\n最近区域：%s\n详情：%s/#/ak?q=%s",
		idx.AccessKeyIDMasked, strings.ToUpper(idx.Provider), idx.AccountAlias, idx.AccountID, idx.OwnerUserName, idx.Status, idx.CreateDate.Format("2006-01-02 15:04:05"), last, idx.LastUsedService, idx.LastUsedRegion, s.Config.PublicBaseURL, idx.AccessKeyIDHash)
	return s.SendMessage(ctx, chatID, body)
}

func (s *Service) SendMessage(ctx context.Context, chatID int64, text string) error {
	bot, err := s.Store.GetDefaultTelegramBot(ctx)
	if err != nil {
		return err
	}
	// Compatibility: deployments upgraded from earlier versions may still have telegram_config only.
	var parseMode string
	token := ""
	if bot != nil && bot.Enabled {
		parseMode = bot.ParseMode
		if bot.TokenEnv != "" {
			token = os.Getenv(bot.TokenEnv)
		}
		if token == "" && bot.TokenEnc != "" {
			token, _ = security.Decrypt(bot.TokenEnc, s.Config.EncryptionKey)
		}
	} else {
		cfg, cfgErr := s.Store.GetTelegramConfig(ctx)
		if cfgErr != nil || cfg == nil || !cfg.Enabled {
			return cfgErr
		}
		parseMode = cfg.ParseMode
		if cfg.BotTokenEnv != "" {
			token = os.Getenv(cfg.BotTokenEnv)
		}
		if token == "" && cfg.BotTokenEnc != "" {
			token, _ = security.Decrypt(cfg.BotTokenEnc, s.Config.EncryptionKey)
		}
	}
	if token == "" {
		return nil
	}
	payload := map[string]any{"chat_id": chatID, "text": text}
	if parseMode != "" && parseMode != "PlainText" {
		payload["parse_mode"] = parseMode
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram sendMessage status %s", resp.Status)
	}
	return nil
}
