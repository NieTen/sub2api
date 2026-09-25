package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
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
	TelegramWebhookURL              string   `json:"telegram_webhook_url,omitempty"`
	TelegramWebhookRegistered       bool     `json:"telegram_webhook_registered"`
}

type SupportDeliveryService struct {
	settings SettingRepository
	email    *EmailService
	// notificationEmails 独立持有模板服务，避免启动过程中 EmailService 的回调指针发生竞态。
	notificationEmails *NotificationEmailService
	repo               SupportTicketRepository
	tickets            *SupportTicketService
	client             *http.Client
	configMu           sync.Mutex
	mu                 sync.Mutex
	wg                 sync.WaitGroup
	cancel             context.CancelFunc
}

type supportEmailRecipient struct {
	address string
	user    bool
}

func NewSupportDeliveryService(settings SettingRepository, email *EmailService, repo SupportTicketRepository, tickets *SupportTicketService) *SupportDeliveryService {
	return &SupportDeliveryService{
		settings:           settings,
		email:              email,
		notificationEmails: &NotificationEmailService{settingRepo: settings, emailService: email},
		repo:               repo,
		tickets:            tickets,
		client:             &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
	}
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

// 保留原有错误码，按字段说明失败原因；错误中不回显令牌或验证密钥。
func supportDeliveryFieldError(field, message string) error {
	err := ErrSupportDeliveryInvalid.WithMetadata(map[string]string{"field": field})
	err.Message = message
	return err
}

func (s *SupportDeliveryService) UpdateSettings(ctx context.Context, c SupportDeliverySettings) (*SupportDeliverySettings, error) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	old, err := s.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	c.TelegramBotToken = strings.TrimSpace(c.TelegramBotToken)
	c.TelegramWebhookSecret = strings.TrimSpace(c.TelegramWebhookSecret)
	c.TelegramChatID = strings.TrimSpace(c.TelegramChatID)
	c.TelegramWebhookURL = strings.TrimSpace(c.TelegramWebhookURL)
	if c.TelegramWebhookURL == "" {
		c.TelegramWebhookURL = old.TelegramWebhookURL
	}
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
	if len(c.AdminEmails) > 20 {
		return nil, supportDeliveryFieldError("admin_emails", "管理员通知邮箱最多可设置 20 个")
	}
	if len(c.TelegramAllowedUserIDs) > 100 {
		return nil, supportDeliveryFieldError("telegram_allowed_user_ids", "允许回复的 Telegram 用户 ID 最多可设置 100 个")
	}
	emails := []string{}
	seen := map[string]bool{}
	for _, email := range c.AdminEmails {
		address, err := parseSMTPAddress(email, "管理员")
		if err != nil {
			return nil, supportDeliveryFieldError("admin_emails", "管理员通知邮箱格式无效，请填写完整的邮箱地址")
		}
		email = strings.ToLower(address.Address)
		if !hasBindableEmailIdentitySubject(email) {
			return nil, supportDeliveryFieldError("admin_emails", "管理员通知邮箱必须为可接收邮件的真实邮箱地址")
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
			return nil, supportDeliveryFieldError("telegram_allowed_user_ids", "允许回复的 Telegram 用户 ID 必须为正整数；负数群组 ID 请填写在会话 ID 中")
		}
		if !seenIDs[id] {
			ids = append(ids, id)
			seenIDs[id] = true
		}
	}
	c.TelegramAllowedUserIDs = ids
	if c.TelegramBotToken != "" && !supportTelegramTokenPattern.MatchString(c.TelegramBotToken) {
		return nil, supportDeliveryFieldError("telegram_bot_token", "Telegram 机器人令牌格式无效，请填写 BotFather 提供的完整令牌（数字编号:令牌）")
	}
	if c.TelegramWebhookSecret != "" && !supportTelegramSecretPattern.MatchString(c.TelegramWebhookSecret) {
		return nil, supportDeliveryFieldError("telegram_webhook_secret", "Webhook 验证密钥必须为 16～256 位，且只能包含英文字母、数字、下划线（_）和短横线（-）")
	}
	if c.TelegramChatID != "" {
		id, err := strconv.ParseInt(c.TelegramChatID, 10, 64)
		if err != nil || id == 0 {
			return nil, supportDeliveryFieldError("telegram_chat_id", "Telegram 会话 ID 必须为非零整数，支持负数群组 ID，请保留群组 ID 前的负号")
		}
	}
	if c.Enabled && len(c.AdminEmails) == 0 && c.TelegramBotToken == "" {
		return nil, supportDeliveryFieldError("enabled", "启用工单通知时，请至少配置管理员通知邮箱或 Telegram 机器人令牌")
	}
	if c.Enabled && c.TelegramBotToken != "" {
		if c.TelegramChatID == "" {
			return nil, supportDeliveryFieldError("telegram_chat_id", "启用 Telegram 工单通知时，请填写 Telegram 会话 ID（群组 ID 可为负数）")
		}
		if len(c.TelegramAllowedUserIDs) == 0 {
			return nil, supportDeliveryFieldError("telegram_allowed_user_ids", "启用 Telegram 工单通知时，请至少填写一位允许回复的 Telegram 用户 ID")
		}
		if c.TelegramWebhookSecret == "" {
			return nil, supportDeliveryFieldError("telegram_webhook_secret", "启用 Telegram 工单通知时，请设置 Webhook 验证密钥（16～256 位英文字母、数字、下划线或短横线）")
		}
	}
	if c.TelegramWebhookURL != "" {
		if err = validateSupportTelegramWebhookURL(c.TelegramWebhookURL); err != nil {
			return nil, err
		}
	}
	// 只有 Telegram 确认注册成功才记录状态，不信任客户端传入的注册标志。
	c.TelegramWebhookRegistered = false
	if c.TelegramWebhookURL != "" && c.TelegramBotToken != "" && c.TelegramWebhookSecret != "" {
		if err = s.registerTelegramWebhook(ctx, &c); err != nil {
			return nil, err
		}
		c.TelegramWebhookRegistered = true
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
		if c.TelegramWebhookRegistered {
			return nil, infraerrors.New(http.StatusInternalServerError, "SUPPORT_TELEGRAM_WEBHOOK_SAVE_FAILED", "Telegram 回调已注册，但本地配置保存失败，请重新保存机器人设置")
		}
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

// loadTicketReplyEmailEnabled 读取管理员回复工单的用户邮件开关。历史安装没有该配置时默认开启，
// 升级后无需额外开启机器人通知即可接收工单回复邮件。
func (s *SupportDeliveryService) loadTicketReplyEmailEnabled(ctx context.Context) (bool, error) {
	if s.settings == nil {
		return true, nil
	}
	value, err := s.settings.GetValue(ctx, SettingKeySupportTicketReplyEmailEnabled)
	if errors.Is(err, ErrSettingNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return !strings.EqualFold(strings.TrimSpace(value), "false"), nil
}

func (s *SupportDeliveryService) process(ctx context.Context) {
	c, err := s.loadSettings(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("工单通知配置读取失败", "error", err)
		}
		return
	}
	replyEmailEnabled, err := s.loadTicketReplyEmailEnabled(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("工单回复邮件开关读取失败", "error", err)
		}
		return
	}
	// 机器人通知关闭时，管理员回复邮件仍应继续发送；两类通知都关闭时不领取队列，
	// 避免无意义地读取附件或连接 SMTP。
	if !c.Enabled && !replyEmailEnabled {
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
				err = s.sendTicketEmail(deliveryCtx, item.ID, c, replyEmailEnabled, ticket, message)
			case "telegram":
				if c.Enabled {
					err = s.sendTicketTelegram(deliveryCtx, item.ID, c, ticket, message)
				}
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

func (s *SupportDeliveryService) sendTicketEmail(ctx context.Context, notificationID int64, c *SupportDeliverySettings, replyEmailEnabled bool, ticket *SupportTicket, message *SupportTicketMessage) error {
	// 用户收件人排在管理员之前，管理员地址异常时也不会阻止用户收到回复提醒。
	recipients := make([]supportEmailRecipient, 0, len(c.AdminEmails)+1)
	if message.SenderRole == "admin" && replyEmailEnabled && hasBindableEmailIdentitySubject(ticket.UserEmail) {
		recipients = append(recipients, supportEmailRecipient{address: ticket.UserEmail, user: true})
	}
	if c.Enabled {
		for _, address := range c.AdminEmails {
			recipients = append(recipients, supportEmailRecipient{address: address})
		}
	}
	if len(recipients) == 0 {
		return nil
	}
	if s.email == nil {
		return errors.New("email service is not configured")
	}
	images, err := s.messageImages(ctx, message)
	if err != nil {
		return err
	}
	legacyBody := supportEmailHTML("工单 #"+strconv.FormatInt(ticket.ID, 10)+"："+ticket.Subject+"\n\n"+message.Content, images)
	legacySubject := "[工单 #" + strconv.FormatInt(ticket.ID, 10) + "] " + ticket.Subject
	ticketURL := ""
	ticketURLLoaded := false
	seen := map[string]bool{}
	var firstErr error
	for _, item := range recipients {
		key := strings.ToLower(strings.TrimSpace(item.address))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		receiptKey := fmt.Sprintf("email:%x", sha256.Sum256([]byte(key)))
		done, receiptErr := s.repo.HasNotificationReceipt(ctx, notificationID, receiptKey)
		if receiptErr != nil {
			if firstErr == nil {
				firstErr = receiptErr
			}
			continue
		}
		if done {
			continue
		}

		if item.user {
			if !ticketURLLoaded {
				ticketURL, err = s.supportTicketURL(ctx, ticket.ID)
				ticketURLLoaded = true
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
			}
			receiptErr = s.sendTicketReplyEmail(ctx, item.address, ticket, message, ticketURL, images)
		} else {
			receiptErr = s.email.SendEmailWithImages(ctx, item.address, legacySubject, legacyBody, images)
		}
		if receiptErr != nil {
			// 每个收件人独立发送，记录错误后继续处理其他地址，尤其不能阻止用户地址。
			if firstErr == nil {
				firstErr = receiptErr
			}
			continue
		}
		if receiptErr = s.repo.SaveNotificationReceipt(ctx, notificationID, receiptKey); receiptErr != nil && firstErr == nil {
			firstErr = receiptErr
		}
	}
	return firstErr
}

// sendTicketReplyEmail 使用系统设置中的工单回复模板，并把图片作为安全的 CID 附件发送。
func (s *SupportDeliveryService) sendTicketReplyEmail(ctx context.Context, recipient string, ticket *SupportTicket, message *SupportTicketMessage, ticketURL string, images []EmailInlineImage) error {
	notificationEmails := s.notificationEmails
	if notificationEmails == nil {
		// 兼容测试或旧调用方直接构造 SupportDeliveryService 的情况。
		notificationEmails = &NotificationEmailService{settingRepo: s.settings, emailService: s.email}
	}
	name := strings.TrimSpace(ticket.Username)
	if name == "" {
		name = emailRecipientName(recipient)
	}
	sourceID := strconv.FormatInt(message.ID, 10)
	if message.ID <= 0 {
		sourceID = strconv.FormatInt(ticket.ID, 10) + ":" + message.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	return notificationEmails.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventSupportTicketReply,
		RecipientEmail: recipient,
		RecipientName:  name,
		UserID:         ticket.UserID,
		SourceType:     "support_ticket_reply",
		SourceID:       sourceID,
		Variables: map[string]string{
			"ticket_id":      strconv.FormatInt(ticket.ID, 10),
			"ticket_subject": ticket.Subject,
			"reply_content":  message.Content,
			"reply_time":     message.CreatedAt.UTC().Format(time.RFC3339),
			"ticket_url":     ticketURL,
		},
		Images: images,
	})
}

// supportTicketURL 只读取管理员已保存的前端地址，不使用请求中的 Host，避免伪造邮件链接。
func (s *SupportDeliveryService) supportTicketURL(ctx context.Context, ticketID int64) (string, error) {
	path := "/tickets/" + strconv.FormatInt(ticketID, 10)
	if s.settings == nil {
		return "", nil
	}
	base, err := s.settings.GetValue(ctx, SettingKeyFrontendURL)
	if errors.Is(err, ErrSettingNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return "", nil
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return "", nil
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	parsed.RawPath = ""
	return parsed.String(), nil
}
