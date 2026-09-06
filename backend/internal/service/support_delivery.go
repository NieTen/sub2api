package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const SettingKeySupportDelivery = "support_delivery_config"
const SupportTelegramWebhookPath = "/api/v1/support/telegram/webhook"

var ErrSupportDeliveryInvalid = infraerrors.BadRequest("SUPPORT_DELIVERY_INVALID", "工单通知配置无效，请检查邮箱、机器人令牌及授权用户")
var ErrSupportTelegramUnauthorized = infraerrors.Forbidden("SUPPORT_TELEGRAM_UNAUTHORIZED", "Telegram 回调验证失败")

type SupportDeliverySettings struct {
	Enabled                         bool     `json:"enabled"`
	AdminEmails                     []string `json:"admin_emails"`
	TelegramBotToken                string   `json:"telegram_bot_token"`
	TelegramBotTokenConfigured      bool     `json:"telegram_bot_token_configured"`
	TelegramChatID                  string   `json:"telegram_chat_id"`
	TelegramAllowedUserIDs          []int64  `json:"telegram_allowed_user_ids"`
	TelegramWebhookSecret           string   `json:"telegram_webhook_secret"`
	TelegramWebhookSecretConfigured bool     `json:"telegram_webhook_secret_configured"`
	ClearTelegramBotToken           bool     `json:"clear_telegram_bot_token,omitempty"`
	ClearTelegramWebhookSecret      bool     `json:"clear_telegram_webhook_secret,omitempty"`
	TelegramWebhookPath             string   `json:"telegram_webhook_path,omitempty"`
}

type SupportDeliveryService struct {
	settings SettingRepository
	email    *EmailService
	repo     SupportTicketRepository
	tickets  *SupportTicketService
	client   *http.Client
	mu       sync.Mutex
	wg       sync.WaitGroup
	cancel   context.CancelFunc
}

func NewSupportDeliveryService(settings SettingRepository, email *EmailService, repo SupportTicketRepository, tickets *SupportTicketService) *SupportDeliveryService {
	return &SupportDeliveryService{settings: settings, email: email, repo: repo, tickets: tickets, client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (s *SupportDeliveryService) loadSettings(ctx context.Context) (*SupportDeliverySettings, error) {
	value, err := s.settings.GetValue(ctx, SettingKeySupportDelivery)
	if errors.Is(err, ErrSettingNotFound) {
		return &SupportDeliverySettings{AdminEmails: []string{}, TelegramAllowedUserIDs: []int64{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var c SupportDeliverySettings
	if err = json.Unmarshal([]byte(value), &c); err != nil {
		return nil, err
	}
	if c.AdminEmails == nil {
		c.AdminEmails = []string{}
	}
	if c.TelegramAllowedUserIDs == nil {
		c.TelegramAllowedUserIDs = []int64{}
	}
	return &c, nil
}

func supportDeliveryPublic(c *SupportDeliverySettings) *SupportDeliverySettings {
	v := *c
	v.TelegramBotTokenConfigured = c.TelegramBotToken != ""
	v.TelegramWebhookSecretConfigured = c.TelegramWebhookSecret != ""
	v.TelegramBotToken = ""
	v.TelegramWebhookSecret = ""
	v.TelegramWebhookPath = SupportTelegramWebhookPath
	return &v
}
func (s *SupportDeliveryService) GetSettings(ctx context.Context) (*SupportDeliverySettings, error) {
	c, err := s.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	return supportDeliveryPublic(c), nil
}

var supportTelegramTokenPattern = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]{20,200}$`)
var supportTelegramSecretPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,256}$`)

func (s *SupportDeliveryService) UpdateSettings(ctx context.Context, c SupportDeliverySettings) (*SupportDeliverySettings, error) {
	old, err := s.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	c.TelegramBotToken = strings.TrimSpace(c.TelegramBotToken)
	c.TelegramWebhookSecret = strings.TrimSpace(c.TelegramWebhookSecret)
	c.TelegramChatID = strings.TrimSpace(c.TelegramChatID)
	if c.TelegramBotToken == "" {
		c.TelegramBotToken = old.TelegramBotToken
	}
	if c.TelegramWebhookSecret == "" {
		c.TelegramWebhookSecret = old.TelegramWebhookSecret
	}
	if c.ClearTelegramBotToken {
		c.TelegramBotToken = ""
	}
	if c.ClearTelegramWebhookSecret {
		c.TelegramWebhookSecret = ""
	}
	if len(c.AdminEmails) > 20 || len(c.TelegramAllowedUserIDs) > 100 {
		return nil, ErrSupportDeliveryInvalid
	}
	emails := []string{}
	seen := map[string]bool{}
	for _, email := range c.AdminEmails {
		address, err := parseSMTPAddress(email, "管理员")
		if err != nil {
			return nil, ErrSupportDeliveryInvalid
		}
		email = strings.ToLower(address.Address)
		if !hasBindableEmailIdentitySubject(email) {
			return nil, ErrSupportDeliveryInvalid
		}
		if !seen[email] {
			emails = append(emails, email)
			seen[email] = true
		}
	}
	c.AdminEmails = emails
	ids := []int64{}
	seenIDs := map[int64]bool{}
	for _, id := range c.TelegramAllowedUserIDs {
		if id <= 0 {
			return nil, ErrSupportDeliveryInvalid
		}
		if !seenIDs[id] {
			ids = append(ids, id)
			seenIDs[id] = true
		}
	}
	c.TelegramAllowedUserIDs = ids
	if c.TelegramBotToken != "" && !supportTelegramTokenPattern.MatchString(c.TelegramBotToken) {
		return nil, ErrSupportDeliveryInvalid
	}
	if c.TelegramWebhookSecret != "" && !supportTelegramSecretPattern.MatchString(c.TelegramWebhookSecret) {
		return nil, ErrSupportDeliveryInvalid
	}
	if c.TelegramChatID != "" {
		id, err := strconv.ParseInt(c.TelegramChatID, 10, 64)
		if err != nil || id == 0 {
			return nil, ErrSupportDeliveryInvalid
		}
	}
	if c.Enabled && len(c.AdminEmails) == 0 && c.TelegramBotToken == "" {
		return nil, ErrSupportDeliveryInvalid
	}
	if c.Enabled && c.TelegramBotToken != "" && (c.TelegramChatID == "" || len(c.TelegramAllowedUserIDs) == 0 || c.TelegramWebhookSecret == "") {
		return nil, ErrSupportDeliveryInvalid
	}
	c.ClearTelegramBotToken = false
	c.ClearTelegramWebhookSecret = false
	c.TelegramBotTokenConfigured = false
	c.TelegramWebhookSecretConfigured = false
	c.TelegramWebhookPath = ""
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	if err = s.settings.Set(ctx, SettingKeySupportDelivery, string(raw)); err != nil {
		return nil, err
	}
	return supportDeliveryPublic(&c), nil
}

func (s *SupportDeliveryService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.process(ctx)
			}
		}
	}()
}
func (s *SupportDeliveryService) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *SupportDeliveryService) process(ctx context.Context) {
	c, err := s.loadSettings(ctx)
	if err != nil || !c.Enabled {
		return
	}
	claimCtx, claimCancel := context.WithTimeout(ctx, 10*time.Second)
	items, err := s.repo.ClaimNotifications(claimCtx, 1, 5*time.Minute)
	claimCancel()
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("工单通知领取失败", "error", err)
		}
		return
	}
	for _, item := range items {
		deliveryCtx, deliveryCancel := context.WithTimeout(ctx, 2*time.Minute)
		ticket, message, err := s.repo.GetNotificationMessage(deliveryCtx, item.MessageID)
		if err == nil {
			switch item.Channel {
			case "email":
				err = s.sendTicketEmail(deliveryCtx, item.ID, c, ticket, message)
			case "telegram":
				err = s.sendTicketTelegram(deliveryCtx, item.ID, c, ticket, message)
			}
		}
		deliveryCancel()
		completeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		if err == nil {
			err = s.repo.CompleteNotification(completeCtx, item.ID, item.LeaseToken)
		} else {
			err = s.repo.FailNotification(completeCtx, item.ID, item.LeaseToken, "通知发送失败，请检查邮件或 Telegram 配置", time.Now().Add(time.Duration(1<<min(item.Attempts, 6))*time.Minute))
		}
		if err != nil {
			slog.Warn("工单通知状态保存失败", "error", err)
		}
		cancel()
	}
}

func (s *SupportDeliveryService) messageImages(ctx context.Context, message *SupportTicketMessage) ([]EmailInlineImage, error) {
	images := []EmailInlineImage{}
	for _, ref := range message.Attachments {
		a, err := s.repo.GetAttachment(ctx, ref.ID)
		if err != nil {
			return nil, err
		}
		images = append(images, EmailInlineImage{FileName: a.FileName, MimeType: a.MimeType, Data: a.Data})
	}
	return images, nil
}

func (s *SupportDeliveryService) sendTicketEmail(ctx context.Context, notificationID int64, c *SupportDeliverySettings, ticket *SupportTicket, message *SupportTicketMessage) error {
	recipients := append([]string{}, c.AdminEmails...)
	if message.SenderRole == "admin" && hasBindableEmailIdentitySubject(ticket.UserEmail) {
		recipients = append(recipients, ticket.UserEmail)
	}
	if len(recipients) == 0 {
		return nil
	}
	images, err := s.messageImages(ctx, message)
	if err != nil {
		return err
	}
	body := supportEmailHTML("工单 #"+strconv.FormatInt(ticket.ID, 10)+"："+ticket.Subject+"\n\n"+message.Content, images)
	seen := map[string]bool{}
	for _, to := range recipients {
		key := strings.ToLower(strings.TrimSpace(to))
		if seen[key] {
			continue
		}
		seen[key] = true
		receiptKey := fmt.Sprintf("email:%x", sha256.Sum256([]byte(key)))
		done, err := s.repo.HasNotificationReceipt(ctx, notificationID, receiptKey)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		if err = s.email.SendEmailWithImages(ctx, to, "[工单 #"+strconv.FormatInt(ticket.ID, 10)+"] "+ticket.Subject, body, images); err != nil {
			return err
		}
		if err = s.repo.SaveNotificationReceipt(ctx, notificationID, receiptKey); err != nil {
			return err
		}
	}
	return nil
}
