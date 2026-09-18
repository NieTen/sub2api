package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type supportReplyProcessRepository struct {
	SupportTicketRepository
	ticket        *SupportTicket
	message       *SupportTicketMessage
	notifications []SupportTicketNotification
	claims        int
	completed     []int64
	failed        []int64
	receipts      map[string]bool
}

func (r *supportReplyProcessRepository) ClaimNotifications(context.Context, int, time.Duration) ([]SupportTicketNotification, error) {
	r.claims++
	return r.notifications, nil
}

func (r *supportReplyProcessRepository) GetNotificationMessage(context.Context, int64) (*SupportTicket, *SupportTicketMessage, error) {
	return r.ticket, r.message, nil
}

func (r *supportReplyProcessRepository) CompleteNotification(_ context.Context, id int64, _ string) error {
	r.completed = append(r.completed, id)
	return nil
}

func (r *supportReplyProcessRepository) FailNotification(_ context.Context, id int64, _, _ string, _ time.Time) error {
	r.failed = append(r.failed, id)
	return nil
}

func (r *supportReplyProcessRepository) HasNotificationReceipt(_ context.Context, _ int64, key string) (bool, error) {
	return r.receipts[key], nil
}

func (r *supportReplyProcessRepository) SaveNotificationReceipt(_ context.Context, _ int64, key string) error {
	r.receipts[key] = true
	return nil
}

func newSupportReplyProcessRepository(role, channel string) *supportReplyProcessRepository {
	return &supportReplyProcessRepository{
		ticket: &SupportTicket{ID: 17, UserID: 9, UserEmail: "user@example.com", Subject: "连接问题"},
		message: &SupportTicketMessage{
			ID: 51, TicketID: 17, SenderRole: role, Source: "web", Content: "问题已经处理",
			CreatedAt: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
		},
		notifications: []SupportTicketNotification{{ID: 25, MessageID: 51, Channel: channel, LeaseToken: "test-lease"}},
		receipts:      map[string]bool{},
	}
}

func TestSupportReplyEmailWorkerSendsWithBotDisabled(t *testing.T) {
	ctx := context.Background()
	settings := newNotificationEmailMemorySettingRepo()
	smtpServer := startNotificationEmailTestSMTPServer(t)
	require.NoError(t, settings.SetMultiple(ctx, smtpServer.settings()))
	require.NoError(t, settings.Set(ctx, SettingKeyFrontendURL, "https://site.example.com"))
	repo := newSupportReplyProcessRepository("admin", "email")
	worker := NewSupportDeliveryService(settings, NewEmailService(settings, nil), repo, nil)

	// 没有机器人配置、没有保存新开关的旧安装也必须发送管理员回复提醒。
	worker.process(ctx)

	require.Equal(t, 1, repo.claims)
	require.Equal(t, []int64{25}, repo.completed)
	require.Empty(t, repo.failed)
	require.Equal(t, int64(1), smtpServer.messageCount())
	require.Contains(t, smtpServer.lastMessage(), "user@example.com")
	require.Contains(t, smtpServer.lastMessageBody(t), "https://site.example.com/tickets/17")
	require.Len(t, repo.receipts, 1)
}

func TestSupportReplyEmailWorkerSkipsUserReplyAndDisabledTelegram(t *testing.T) {
	for _, pair := range []string{"user/email", "admin/telegram"} {
		t.Run(pair, func(t *testing.T) {
			role, channel, _ := strings.Cut(pair, "/")
			repo := newSupportReplyProcessRepository(role, channel)
			settings := newNotificationEmailMemorySettingRepo()
			require.NoError(t, settings.Set(context.Background(), SettingKeySupportDelivery,
				`{"enabled":false,"telegram_bot_token":"123456:test-token","telegram_chat_id":"-10012345"}`))
			worker := NewSupportDeliveryService(settings, nil, repo, nil)
			telegramCalled := false
			worker.client = &http.Client{Transport: supportDeliveryTestTransport{respond: func(*http.Request) (*http.Response, error) {
				telegramCalled = true
				return nil, errors.New("测试中禁止访问 Telegram")
			}}}

			// 即使保留了机器人凭据，停用的通知也不能尝试发送。
			worker.process(context.Background())

			require.Equal(t, []int64{25}, repo.completed)
			require.Empty(t, repo.failed)
			require.Empty(t, repo.receipts)
			require.False(t, telegramCalled)
		})
	}
}

func TestSupportReplyEmailWorkerDoesNotClaimWhenAllDisabled(t *testing.T) {
	ctx := context.Background()
	settings := newNotificationEmailMemorySettingRepo()
	require.NoError(t, settings.Set(ctx, SettingKeySupportTicketReplyEmailEnabled, "false"))
	repo := newSupportReplyProcessRepository("admin", "email")
	worker := NewSupportDeliveryService(settings, nil, repo, nil)

	worker.process(ctx)

	require.Zero(t, repo.claims)
	require.Empty(t, repo.completed)
	require.Empty(t, repo.failed)
}

func TestSupportReplyEmailWorkerRetriesSMTPFailure(t *testing.T) {
	repo := newSupportReplyProcessRepository("admin", "email")
	settings := newNotificationEmailMemorySettingRepo()
	worker := NewSupportDeliveryService(settings, NewEmailService(settings, nil), repo, nil)

	// 未配置 SMTP 时，队列应进入失败重试而非标记投递成功。
	worker.process(context.Background())

	require.Equal(t, []int64{25}, repo.failed)
	require.Empty(t, repo.completed)
	require.Empty(t, repo.receipts)
}

func TestSupportReplyEmailWorkerDisabledUserReminderKeepsAdminMail(t *testing.T) {
	ctx := context.Background()
	settings := newNotificationEmailMemorySettingRepo()
	smtpServer := startNotificationEmailTestSMTPServer(t)
	require.NoError(t, settings.SetMultiple(ctx, smtpServer.settings()))
	require.NoError(t, settings.Set(ctx, SettingKeySupportTicketReplyEmailEnabled, "false"))
	require.NoError(t, settings.Set(ctx, SettingKeySupportDelivery,
		`{"enabled":true,"admin_emails":["admin@example.com"]}`))
	repo := newSupportReplyProcessRepository("admin", "email")
	worker := NewSupportDeliveryService(settings, NewEmailService(settings, nil), repo, nil)

	// 用户提醒关闭时，已开启的管理员邮件通知仍需独立投递。
	worker.process(ctx)

	require.Equal(t, []int64{25}, repo.completed)
	require.Empty(t, repo.failed)
	require.Equal(t, int64(1), smtpServer.messageCount())
	require.Contains(t, smtpServer.lastMessage(), "To: <admin@example.com>")
	require.NotContains(t, smtpServer.lastMessage(), "To: <user@example.com>")
}
