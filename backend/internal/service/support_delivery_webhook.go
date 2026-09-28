package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const supportTelegramWebhookRevision = 2

// 升级后自动补订阅编辑消息；配置锁防止后台同步覆盖管理员刚保存的令牌或地址。
func (s *SupportDeliveryService) syncTelegramWebhook(ctx context.Context) {
	s.mu.Lock()
	if time.Since(s.webhookSyncAttempt) < time.Minute {
		s.mu.Unlock()
		return
	}
	s.webhookSyncAttempt = time.Now()
	s.mu.Unlock()
	s.configMu.Lock()
	defer s.configMu.Unlock()
	c, err := s.loadSettings(ctx)
	if err != nil || c.TelegramWebhookRevision == supportTelegramWebhookRevision || c.TelegramBotToken == "" || !supportTelegramSecretPattern.MatchString(c.TelegramWebhookSecret) || validateSupportTelegramWebhookURL(c.TelegramWebhookURL) != nil {
		return
	}
	work, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	if err = s.registerTelegramWebhook(work, c); err != nil {
		slog.Warn("Telegram 群消息回调订阅更新失败，将稍后重试")
		return
	}
	c.TelegramWebhookRevision = supportTelegramWebhookRevision
	c.TelegramWebhookRegistered = true
	raw, err := json.Marshal(c)
	if err == nil {
		err = s.settings.Set(work, SettingKeySupportDelivery, string(raw))
	}
	if err != nil {
		slog.Warn("Telegram 群消息回调订阅状态保存失败，将稍后重试")
	}
}

type supportTelegramWebhookRequest struct {
	URL            string   `json:"url"`
	SecretToken    string   `json:"secret_token"`
	AllowedUpdates []string `json:"allowed_updates"`
}

// 回调由 Telegram 访问，只允许本站固定接收路径及 Telegram 支持的公开 HTTPS 地址。
func validateSupportTelegramWebhookURL(value string) error {
	invalid := func() error {
		return supportDeliveryFieldError("telegram_webhook_url", "Telegram 回调地址必须为公网 HTTPS 地址，使用本站 /api/v1/support/telegram/webhook 路径，且不能包含账号密码、查询参数或片段")
	}
	u, err := url.Parse(value)
	if err != nil || len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n\t ") || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.EscapedPath() != SupportTelegramWebhookPath {
		return invalid()
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return invalid()
	}
	if address, parseErr := netip.ParseAddr(host); parseErr == nil {
		address = address.Unmap()
		if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
			return invalid()
		}
	} else if !strings.Contains(host, ".") {
		return invalid()
	}
	switch u.Port() {
	case "", "443", "80", "88", "8443":
		return nil
	default:
		return supportDeliveryFieldError("telegram_webhook_url", "Telegram 回调地址端口只支持 443、80、88 或 8443")
	}
}

func (s *SupportDeliveryService) registerTelegramWebhook(ctx context.Context, c *SupportDeliverySettings) error {
	var registered bool
	err := s.telegramJSON(ctx, c.TelegramBotToken, "setWebhook", supportTelegramWebhookRequest{
		URL: c.TelegramWebhookURL, SecretToken: c.TelegramWebhookSecret,
		// 必须显式订阅成员变化；Telegram 默认订阅不包含 chat_member。
		AllowedUpdates: []string{"message", "edited_message", "chat_join_request", "chat_member"},
	}, &registered)
	if err == nil && registered {
		return nil
	}
	var api *SupportTelegramAPIError
	if errors.As(err, &api) {
		switch api.StatusCode {
		case http.StatusUnauthorized, http.StatusNotFound:
			return supportDeliveryFieldError("telegram_bot_token", "Telegram 回调注册失败，请检查机器人令牌是否有效")
		case http.StatusBadRequest:
			return supportDeliveryFieldError("telegram_webhook_url", "Telegram 拒绝注册回调，请确认回调地址可从公网通过 HTTPS 访问，域名解析和证书有效")
		case http.StatusForbidden:
			return supportDeliveryFieldError("telegram_bot_token", "Telegram 禁止此机器人注册回调，请检查机器人状态及令牌")
		case http.StatusTooManyRequests:
			return infraerrors.New(http.StatusBadGateway, "SUPPORT_TELEGRAM_WEBHOOK_FAILED", "Telegram 回调注册请求过于频繁，请稍后重新保存")
		default:
			return infraerrors.New(http.StatusBadGateway, "SUPPORT_TELEGRAM_WEBHOOK_FAILED", fmt.Sprintf("Telegram 回调注册失败（状态 %d），请稍后重新保存", api.StatusCode))
		}
	}
	// 不附加原始错误，避免请求地址中的令牌或 Telegram 响应内容被返回给客户端。
	return infraerrors.New(http.StatusBadGateway, "SUPPORT_TELEGRAM_WEBHOOK_FAILED", "Telegram 未确认回调注册成功，请检查服务器与 Telegram 的连接后重新保存")
}
