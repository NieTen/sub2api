package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func validSupportDeliverySettingsForTest() SupportDeliverySettings {
	return SupportDeliverySettings{
		Enabled:                true,
		AdminEmails:            []string{"admin@example.com"},
		TelegramBotToken:       "123456:abcdefghijklmnopqrstuvwxyz123456",
		TelegramChatID:         "-10012345",
		TelegramAllowedUserIDs: []int64{5939067819},
		TelegramWebhookSecret:  "secret_12345678901234567890",
	}
}

func TestSupportDeliverySettingsAcceptLargeUserAndNegativeChatIDs(t *testing.T) {
	ctx := context.Background()
	service := NewSupportDeliveryService(newNotificationEmailMemorySettingRepo(), nil, nil, nil)
	config := validSupportDeliverySettingsForTest()
	config.TelegramChatID = "-1001888000999"
	saved, err := service.UpdateSettings(ctx, config)
	require.NoError(t, err)
	require.Equal(t, []int64{5939067819}, saved.TelegramAllowedUserIDs)
	require.Equal(t, "-1001888000999", saved.TelegramChatID)
	require.Empty(t, saved.TelegramBotToken)
	require.Empty(t, saved.TelegramWebhookSecret)

	stored, err := service.loadSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, config.TelegramAllowedUserIDs, stored.TelegramAllowedUserIDs)
	require.Equal(t, config.TelegramChatID, stored.TelegramChatID)
}

func TestSupportDeliverySettingsPreserveConfiguredSecretsWhenInputIsBlank(t *testing.T) {
	for _, value := range []string{"", " \t\r\n "} {
		t.Run("空白输入="+strings.ReplaceAll(value, "\n", "\\n"), func(t *testing.T) {
			ctx := context.Background()
			service := NewSupportDeliveryService(supportDeliveryTestSettings(t), nil, nil, nil)
			view, err := service.GetSettings(ctx)
			require.NoError(t, err)
			view.TelegramBotToken = value
			view.TelegramWebhookSecret = value
			saved, err := service.UpdateSettings(ctx, *view)
			require.NoError(t, err)
			require.True(t, saved.TelegramBotTokenConfigured)
			require.True(t, saved.TelegramWebhookSecretConfigured)
			require.Empty(t, saved.TelegramBotToken)
			require.Empty(t, saved.TelegramWebhookSecret)
			stored, err := service.loadSettings(ctx)
			require.NoError(t, err)
			require.Equal(t, "123456:abcdefghijklmnopqrstuvwxyz123456", stored.TelegramBotToken)
			require.Equal(t, "secret_12345678901234567890", stored.TelegramWebhookSecret)
		})
	}
}

func TestSupportDeliverySettingsErrorsIdentifyFieldsWithoutLeakingSecrets(t *testing.T) {
	cases := []struct {
		name   string
		change func(*SupportDeliverySettings)
		field  string
		parts  []string
	}{
		{
			name: "管理员邮箱格式无效", field: "admin_emails", parts: []string{"邮箱"},
			change: func(c *SupportDeliverySettings) { c.AdminEmails = []string{"not-an-email"} },
		},
		{
			name: "管理员邮箱数量超限", field: "admin_emails", parts: []string{"20"},
			change: func(c *SupportDeliverySettings) {
				for len(c.AdminEmails) <= 20 {
					c.AdminEmails = append(c.AdminEmails, "admin@example.com")
				}
			},
		},
		{
			name: "授权用户不能填写群组负数ID", field: "telegram_allowed_user_ids", parts: []string{"用户"},
			change: func(c *SupportDeliverySettings) { c.TelegramAllowedUserIDs = []int64{-10012345} },
		},
		{
			name: "授权用户不能为零", field: "telegram_allowed_user_ids", parts: []string{"用户"},
			change: func(c *SupportDeliverySettings) { c.TelegramAllowedUserIDs = []int64{0} },
		},
		{
			name: "授权用户数量超限", field: "telegram_allowed_user_ids", parts: []string{"100"},
			change: func(c *SupportDeliverySettings) {
				c.TelegramAllowedUserIDs = make([]int64, 101)
				for i := range c.TelegramAllowedUserIDs {
					c.TelegramAllowedUserIDs[i] = int64(i + 1)
				}
			},
		},
		{
			name: "机器人令牌格式无效", field: "telegram_bot_token", parts: []string{"令牌"},
			change: func(c *SupportDeliverySettings) { c.TelegramBotToken = "invalid:private-bot-token" },
		},
		{
			name: "Webhook密钥不足16位", field: "telegram_webhook_secret", parts: []string{"Webhook", "16"},
			change: func(c *SupportDeliverySettings) { c.TelegramWebhookSecret = "short-secret" },
		},
		{
			name: "Webhook密钥包含非法字符", field: "telegram_webhook_secret", parts: []string{"Webhook", "16", "英文字母"},
			change: func(c *SupportDeliverySettings) { c.TelegramWebhookSecret = "private*invalid-secret" },
		},
		{
			name: "群组ID不是数字", field: "telegram_chat_id", parts: []string{"ID"},
			change: func(c *SupportDeliverySettings) { c.TelegramChatID = "@ticket-team" },
		},
		{
			name: "群组ID不能为零", field: "telegram_chat_id", parts: []string{"ID"},
			change: func(c *SupportDeliverySettings) { c.TelegramChatID = "0" },
		},
		{
			name: "启用通知但没有通知渠道", field: "enabled", parts: []string{"启用"},
			change: func(c *SupportDeliverySettings) { c.AdminEmails = nil; c.TelegramBotToken = "" },
		},
		{
			name: "启用机器人但没有群组ID", field: "telegram_chat_id", parts: []string{"ID"},
			change: func(c *SupportDeliverySettings) { c.TelegramChatID = "" },
		},
		{
			name: "启用机器人但没有授权用户", field: "telegram_allowed_user_ids", parts: []string{"用户"},
			change: func(c *SupportDeliverySettings) { c.TelegramAllowedUserIDs = nil },
		},
		{
			name: "启用机器人但没有Webhook密钥", field: "telegram_webhook_secret", parts: []string{"Webhook"},
			change: func(c *SupportDeliverySettings) { c.TelegramWebhookSecret = "" },
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repository := newNotificationEmailMemorySettingRepo()
			service := NewSupportDeliveryService(repository, nil, nil, nil)
			config := validSupportDeliverySettingsForTest()
			test.change(&config)
			saved, err := service.UpdateSettings(ctx, config)
			require.Nil(t, saved)
			require.ErrorIs(t, err, ErrSupportDeliveryInvalid)
			appError := infraerrors.FromError(err)
			require.EqualValues(t, http.StatusBadRequest, appError.Code)
			require.Equal(t, "SUPPORT_DELIVERY_INVALID", appError.Reason)
			require.Equal(t, test.field, appError.Metadata["field"])
			require.Regexp(t, `\p{Han}`, appError.Message)
			for _, part := range test.parts {
				require.Contains(t, appError.Message, part)
			}

			response, marshalErr := json.Marshal(appError.Status)
			require.NoError(t, marshalErr)
			for _, credential := range []string{config.TelegramBotToken, config.TelegramWebhookSecret} {
				if credential != "" {
					require.NotContains(t, string(response), credential)
					require.NotContains(t, err.Error(), credential)
				}
			}
			_, storedErr := repository.GetValue(ctx, SettingKeySupportDelivery)
			require.ErrorIs(t, storedErr, ErrSettingNotFound)
		})
	}
	require.Empty(t, ErrSupportDeliveryInvalid.Metadata)
}
