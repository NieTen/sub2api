package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func communitySettingsResponseForTest(t *testing.T, result any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"ok": true, "result": result})
	require.NoError(t, err)
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}
}

func requireCommunitySettingsError(t *testing.T, s *CommunityService, input CommunitySettings, field string, status int, parts ...string) {
	t.Helper()
	ctx := context.Background()
	previous, err := s.settings.GetValue(ctx, SettingKeyCommunity)
	require.NoError(t, err)
	shared, err := s.delivery.loadSettings(ctx)
	require.NoError(t, err)
	saved, err := s.UpdateSettings(ctx, input)
	require.Nil(t, saved)
	require.Error(t, err)
	appError := infraerrors.FromError(err)
	require.EqualValues(t, status, appError.Code)
	if status == http.StatusBadRequest {
		require.ErrorIs(t, err, ErrCommunityInvalid)
		require.Equal(t, "COMMUNITY_INVALID", appError.Reason)
	} else {
		require.Equal(t, "COMMUNITY_TELEGRAM_CHECK_FAILED", appError.Reason)
	}
	require.Equal(t, field, appError.Metadata["field"])
	require.Regexp(t, `\p{Han}`, appError.Message)
	require.NotEqual(t, ErrCommunityInvalid.Message, appError.Message)
	for _, part := range parts {
		require.Contains(t, appError.Message, part)
	}
	response, marshalErr := json.Marshal(appError.Status)
	require.NoError(t, marshalErr)
	for _, credential := range []string{shared.TelegramBotToken, shared.TelegramWebhookSecret, "PRIVATE_TELEGRAM_BODY"} {
		if credential != "" {
			require.NotContains(t, string(response), credential)
			require.NotContains(t, err.Error(), credential)
		}
	}
	current, readErr := s.settings.GetValue(ctx, SettingKeyCommunity)
	require.NoError(t, readErr)
	require.Equal(t, previous, current, "校验失败不得覆盖已保存的配置")
}

func TestCommunitySettingsAcceptNegativeGroupIDs(t *testing.T) {
	for _, group := range []struct {
		id, kind string
	}{{"-5391524769", "group"}, {"-1001888000999", "supergroup"}} {
		t.Run(group.kind, func(t *testing.T) {
			s, _, telegram := communityTestService(t)
			id, err := strconv.ParseInt(group.id, 10, 64)
			require.NoError(t, err)
			s.delivery.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if path.Base(request.URL.Path) == "getChat" {
					var input communityChatRequest
					require.NoError(t, json.NewDecoder(request.Body).Decode(&input))
					require.Equal(t, group.id, input.ChatID)
					return communitySettingsResponseForTest(t, communityTelegramChat{ID: id, Type: group.kind, Title: "私密群"}), nil
				}
				return telegram.RoundTrip(request)
			})
			config := CommunitySettings{Enabled: true, ContactURL: " https://zzzai.pro/kf ", GroupName: " 众智AI ", GroupChatID: " " + group.id + " ", BotUsername: " @zzzaiprobot ", BotID: 999}
			saved, err := s.UpdateSettings(context.Background(), config)
			require.NoError(t, err)
			require.Equal(t, group.id, saved.GroupChatID)
			require.Equal(t, "https://zzzai.pro/kf", saved.ContactURL)
			require.Equal(t, "众智AI", saved.GroupName)
			require.EqualValues(t, 77, saved.BotID)
			require.Equal(t, "site_test_bot", saved.BotUsername)
			stored, err := s.GetSettings(context.Background())
			require.NoError(t, err)
			require.Equal(t, saved, stored)
		})
	}
}

func TestCommunitySettingsRejectInvalidFieldsBeforeTelegramRequests(t *testing.T) {
	cases := []struct {
		name, field, part string
		change            func(*CommunitySettings)
	}{
		{"客服地址过长", "contact_url", "2048", func(c *CommunitySettings) { c.ContactURL = "https://example.com/" + strings.Repeat("a", 2048) }},
		{"客服地址协议无效", "contact_url", "HTTP", func(c *CommunitySettings) { c.ContactURL = "file:///contact" }},
		{"客服地址含认证信息", "contact_url", "地址", func(c *CommunitySettings) { c.ContactURL = "https://user:password@example.com" }},
		{"群组名称过长", "group_name", "100", func(c *CommunitySettings) { c.GroupName = strings.Repeat("群", 101) }},
		{"群组名称含换行", "group_name", "名称", func(c *CommunitySettings) { c.GroupName = "群\n组" }},
		{"机器人用户名无效", "bot_username", "用户名", func(c *CommunitySettings) { c.BotUsername = "中文机器人" }},
		{"群组ID为正数", "group_chat_id", "负", func(c *CommunitySettings) { c.GroupChatID = "5939067819" }},
		{"群组ID为零", "group_chat_id", "负", func(c *CommunitySettings) { c.GroupChatID = "0" }},
		{"群组ID不是数字", "group_chat_id", "ID", func(c *CommunitySettings) { c.GroupChatID = "@group_name" }},
		{"群组ID溢出", "group_chat_id", "ID", func(c *CommunitySettings) { c.GroupChatID = "-9223372036854775809" }},
		{"启用社群未填群组ID", "group_chat_id", "ID", func(c *CommunitySettings) { c.GroupChatID = "" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			s, _, telegram := communityTestService(t)
			config, err := s.GetSettings(context.Background())
			require.NoError(t, err)
			test.change(config)
			requireCommunitySettingsError(t, s, *config, test.field, http.StatusBadRequest, test.part)
			require.Empty(t, telegram.methods)
		})
	}
}

func TestCommunitySettingsValidateSharedCredentialsWithoutLeakingValues(t *testing.T) {
	cases := []struct {
		name, field, part string
		change            func(*SupportDeliverySettings)
	}{
		{"缺少机器人令牌", "telegram_bot_token", "令牌", func(c *SupportDeliverySettings) { c.TelegramBotToken = "" }},
		{"机器人令牌格式无效", "telegram_bot_token", "令牌", func(c *SupportDeliverySettings) { c.TelegramBotToken = "invalid:private_bot_token" }},
		{"缺少Webhook密钥", "telegram_webhook_secret", "Webhook", func(c *SupportDeliverySettings) { c.TelegramWebhookSecret = "" }},
		{"Webhook密钥过短", "telegram_webhook_secret", "16", func(c *SupportDeliverySettings) { c.TelegramWebhookSecret = "short_secret" }},
		{"Webhook密钥字符无效", "telegram_webhook_secret", "Webhook", func(c *SupportDeliverySettings) { c.TelegramWebhookSecret = "private*invalid_secret" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			s, _, telegram := communityTestService(t)
			ctx := context.Background()
			shared, err := s.delivery.loadSettings(ctx)
			require.NoError(t, err)
			test.change(shared)
			raw, err := json.Marshal(shared)
			require.NoError(t, err)
			require.NoError(t, s.settings.Set(ctx, SettingKeySupportDelivery, string(raw)))
			config, err := s.GetSettings(ctx)
			require.NoError(t, err)
			requireCommunitySettingsError(t, s, *config, test.field, http.StatusBadRequest, test.part)
			require.Empty(t, telegram.methods)
		})
	}
}

func TestCommunitySettingsIdentifyGroupAndBotPermissionFailures(t *testing.T) {
	cases := []struct {
		name, method, field, part string
		result                    any
	}{
		{"机器人身份无效", "getMe", "telegram_bot_token", "机器人", communityTelegramUser{ID: 77, Username: "site_test_bot"}},
		{"群组实际为频道", "getChat", "group_chat_id", "群", communityTelegramChat{ID: -100, Type: "channel"}},
		{"群组有公开用户名", "getChat", "group_chat_id", "私密", communityTelegramChat{ID: -100, Type: "supergroup", Username: "public_group"}},
		{"群组有生效的公开用户名", "getChat", "group_chat_id", "私密", communityTelegramChat{ID: -100, Type: "supergroup", ActiveUsernames: []string{"public_group"}}},
		{"群组ID不一致", "getChat", "group_chat_id", "ID", communityTelegramChat{ID: -200, Type: "supergroup"}},
		{"机器人未设为管理员", "getChatMember", "bot_permissions", "管理员", communityTelegramMember{User: communityTelegramUser{ID: 77}, Status: "member"}},
		{"机器人缺邀请权限", "getChatMember", "bot_permissions", "邀请", communityTelegramMember{User: communityTelegramUser{ID: 77}, Status: "administrator", CanRestrictMembers: true}},
		{"机器人缺封禁权限", "getChatMember", "bot_permissions", "封禁", communityTelegramMember{User: communityTelegramUser{ID: 77}, Status: "administrator", CanInviteUsers: true}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			s, _, telegram := communityTestService(t)
			s.delivery.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if path.Base(request.URL.Path) == test.method {
					return communitySettingsResponseForTest(t, test.result), nil
				}
				return telegram.RoundTrip(request)
			})
			config, err := s.GetSettings(context.Background())
			require.NoError(t, err)
			parts := []string{test.part}
			if test.field == "bot_permissions" {
				parts = append(parts, "@site_test_bot")
			}
			requireCommunitySettingsError(t, s, *config, test.field, http.StatusBadRequest, parts...)
		})
	}
	// 字段错误必须创建独立实例，不能污染邀请流程共用的错误常量。
	require.Empty(t, ErrCommunityInvalid.Metadata)
	require.Equal(t, "社群配置或邀请信息无效", ErrCommunityInvalid.Message)
}

func TestCommunitySettingsReportTelegramFailuresSafely(t *testing.T) {
	for method, field := range map[string]string{"getMe": "telegram_bot_token", "getChat": "group_chat_id", "getChatMember": "bot_permissions"} {
		for _, status := range []int{400, 401, 403, 404, 429, 500, 502, 0} {
			t.Run(method+"/"+strconv.Itoa(status), func(t *testing.T) {
				s, _, telegram := communityTestService(t)
				s.delivery.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
					if path.Base(request.URL.Path) == method {
						if status == 0 {
							return nil, errors.New("PRIVATE_TELEGRAM_BODY " + request.URL.String())
						}
						return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("PRIVATE_TELEGRAM_BODY " + request.URL.String()))}, nil
					}
					return telegram.RoundTrip(request)
				})
				config, err := s.GetSettings(context.Background())
				require.NoError(t, err)
				wantStatus := http.StatusBadRequest
				if status == 0 || status == 429 || status >= 500 {
					wantStatus = http.StatusBadGateway
				}
				wantField := field
				parts := []string{"Telegram"}
				if status == 401 || status == 404 {
					wantField = "telegram_bot_token"
				} else if field == "bot_permissions" && status != 429 {
					parts = append(parts, "@site_test_bot")
				}
				requireCommunitySettingsError(t, s, *config, wantField, wantStatus, parts...)
			})
		}
	}
}
