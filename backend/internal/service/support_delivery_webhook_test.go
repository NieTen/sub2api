package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func supportWebhookResponseForTest(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

type supportWebhookFailingSettingRepo struct {
	*notificationEmailMemorySettingRepo
	setErr error
}

func (r *supportWebhookFailingSettingRepo) Set(context.Context, string, string) error {
	return r.setErr
}

func TestSupportDeliveryWebhookRegistersAllRequiredUpdates(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "启用工单通知"
		if !enabled {
			name = "仅使用社群机器人"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s := NewSupportDeliveryService(newNotificationEmailMemorySettingRepo(), nil, nil, nil)
			config := validSupportDeliverySettingsForTest()
			config.Enabled = enabled
			config.TelegramWebhookURL = " https://portal.example.com" + SupportTelegramWebhookPath + " "
			config.TelegramWebhookPath = "/client-controlled-path"
			if !enabled {
				config.AdminEmails = nil
				config.TelegramChatID = ""
				config.TelegramAllowedUserIDs = nil
			}
			calls := 0
			s.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, http.MethodPost, request.Method)
				require.Equal(t, "https", request.URL.Scheme)
				require.Equal(t, "api.telegram.org", request.URL.Host)
				require.Equal(t, "/bot"+config.TelegramBotToken+"/setWebhook", request.URL.Path)
				require.Equal(t, "application/json", request.Header.Get("Content-Type"))
				var payload map[string]any
				require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
				require.Equal(t, "https://portal.example.com"+SupportTelegramWebhookPath, payload["url"])
				require.Equal(t, config.TelegramWebhookSecret, payload["secret_token"])
				require.ElementsMatch(t, []any{"message", "chat_join_request", "chat_member"}, payload["allowed_updates"])
				if drop, exists := payload["drop_pending_updates"]; exists {
					require.Equal(t, false, drop, "注册不能丢弃等待中的入群或消息回调")
				}
				return supportWebhookResponseForTest(http.StatusOK, `{"ok":true,"result":true}`), nil
			})
			saved, err := s.UpdateSettings(ctx, config)
			require.NoError(t, err)
			require.Equal(t, 1, calls)
			require.True(t, saved.TelegramWebhookRegistered)
			require.Equal(t, "https://portal.example.com"+SupportTelegramWebhookPath, saved.TelegramWebhookURL)
			require.Equal(t, SupportTelegramWebhookPath, saved.TelegramWebhookPath)
			require.Empty(t, saved.TelegramBotToken)
			require.Empty(t, saved.TelegramWebhookSecret)
			stored, err := s.loadSettings(ctx)
			require.NoError(t, err)
			require.True(t, stored.TelegramWebhookRegistered)
			require.Equal(t, saved.TelegramWebhookURL, stored.TelegramWebhookURL)
			require.Empty(t, stored.TelegramWebhookPath)
			view, err := s.GetSettings(ctx)
			require.NoError(t, err)
			require.Equal(t, saved, view)
		})
	}
}

func TestSupportDeliveryWebhookPreservesSecretsAndRetriesSavedURL(t *testing.T) {
	ctx := context.Background()
	r := supportDeliveryTestSettings(t)
	s := NewSupportDeliveryService(r, nil, nil, nil)
	stored, err := s.loadSettings(ctx)
	require.NoError(t, err)
	view, err := s.GetSettings(ctx)
	require.NoError(t, err)
	view.TelegramWebhookURL = "https://portal.example.com" + SupportTelegramWebhookPath
	view.TelegramBotToken = " \t "
	view.TelegramWebhookSecret = " \t "
	calls := 0
	s.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "/bot"+stored.TelegramBotToken+"/setWebhook", request.URL.Path)
		var payload map[string]any
		require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
		require.Equal(t, view.TelegramWebhookURL, payload["url"])
		require.Equal(t, stored.TelegramWebhookSecret, payload["secret_token"])
		return supportWebhookResponseForTest(http.StatusOK, `{"ok":true,"result":true}`), nil
	})
	saved, err := s.UpdateSettings(ctx, *view)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	_, err = s.UpdateSettings(ctx, *saved)
	require.NoError(t, err)
	require.Equal(t, 2, calls, "再次保存同一地址也要重新注册，以修复被其他服务覆盖的回调")
	saved.TelegramWebhookURL = ""
	resaved, err := s.UpdateSettings(ctx, *saved)
	require.NoError(t, err)
	require.Equal(t, 3, calls)
	require.Equal(t, view.TelegramWebhookURL, resaved.TelegramWebhookURL, "旧客户端未提交地址时保留已保存的回调")
	current, err := s.loadSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, stored.TelegramBotToken, current.TelegramBotToken)
	require.Equal(t, stored.TelegramWebhookSecret, current.TelegramWebhookSecret)
}

func TestSupportDeliveryWebhookDoesNotRegisterWithoutCallbackURL(t *testing.T) {
	for _, mode := range []string{"旧客户端", "仅邮箱"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s := NewSupportDeliveryService(newNotificationEmailMemorySettingRepo(), nil, nil, nil)
			config := validSupportDeliverySettingsForTest()
			if mode == "仅邮箱" {
				config.TelegramBotToken = ""
				config.TelegramWebhookSecret = ""
				config.TelegramChatID = ""
				config.TelegramAllowedUserIDs = nil
			}
			config.TelegramWebhookRegistered = true
			s.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("未配置回调地址不得发起 Telegram 网络请求")
				return nil, errors.New("意外请求")
			})
			saved, err := s.UpdateSettings(ctx, config)
			require.NoError(t, err)
			require.False(t, saved.TelegramWebhookRegistered, "不能信任客户端传入的注册状态")
			stored, err := s.loadSettings(ctx)
			require.NoError(t, err)
			require.False(t, stored.TelegramWebhookRegistered)
		})
	}
}

func TestSupportDeliveryWebhookDoesNotRegisterWithoutCredentials(t *testing.T) {
	for _, missing := range []string{"令牌", "密钥", "全部凭据"} {
		t.Run(missing, func(t *testing.T) {
			ctx := context.Background()
			s := NewSupportDeliveryService(supportDeliveryTestSettings(t), nil, nil, nil)
			config := validSupportDeliverySettingsForTest()
			config.Enabled = false
			config.TelegramWebhookURL = "https://portal.example.com" + SupportTelegramWebhookPath
			config.TelegramWebhookRegistered = true
			config.ClearTelegramBotToken = missing != "密钥"
			config.ClearTelegramWebhookSecret = missing != "令牌"
			s.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("缺少凭据时不得发起 Telegram 网络请求")
				return nil, errors.New("意外请求")
			})
			saved, err := s.UpdateSettings(ctx, config)
			require.NoError(t, err)
			require.False(t, saved.TelegramWebhookRegistered)
			require.Equal(t, config.TelegramWebhookURL, saved.TelegramWebhookURL)
			stored, err := s.loadSettings(ctx)
			require.NoError(t, err)
			require.False(t, stored.TelegramWebhookRegistered)
			require.Equal(t, config.ClearTelegramBotToken, stored.TelegramBotToken == "")
			require.Equal(t, config.ClearTelegramWebhookSecret, stored.TelegramWebhookSecret == "")
		})
	}
}

func TestSupportDeliveryWebhookRejectsInvalidCallbackURLsBeforeNetwork(t *testing.T) {
	for name, callbackURL := range map[string]string{
		"非HTTPS":        "http://portal.example.com" + SupportTelegramWebhookPath,
		"无协议":           "portal.example.com" + SupportTelegramWebhookPath,
		"缺少固定路径":        "https://portal.example.com",
		"自定义路径":         "https://portal.example.com/other/webhook",
		"路径后缀":          "https://portal.example.com" + SupportTelegramWebhookPath + "/extra",
		"地址含认证信息":       "https://user:password@portal.example.com" + SupportTelegramWebhookPath,
		"地址含查询参数":       "https://portal.example.com" + SupportTelegramWebhookPath + "?token=private",
		"地址含片段":         "https://portal.example.com" + SupportTelegramWebhookPath + "#callback",
		"本机域名":          "https://localhost" + SupportTelegramWebhookPath,
		"本机域名大写":        "https://LOCALHOST" + SupportTelegramWebhookPath,
		"本机域名结尾点":       "https://localhost." + SupportTelegramWebhookPath,
		"本机子域名":         "https://portal.localhost" + SupportTelegramWebhookPath,
		"IPv4环回地址":      "https://127.0.0.1" + SupportTelegramWebhookPath,
		"IPv4私网10":      "https://10.20.30.40" + SupportTelegramWebhookPath,
		"IPv4私网172":     "https://172.16.1.2" + SupportTelegramWebhookPath,
		"IPv4私网192":     "https://192.168.1.2" + SupportTelegramWebhookPath,
		"IPv4链路本地":      "https://169.254.1.2" + SupportTelegramWebhookPath,
		"IPv4未指定地址":     "https://0.0.0.0" + SupportTelegramWebhookPath,
		"IPv6环回地址":      "https://[::1]" + SupportTelegramWebhookPath,
		"IPv6私网地址":      "https://[fd00::1]" + SupportTelegramWebhookPath,
		"IPv6链路本地":      "https://[fe80::1]" + SupportTelegramWebhookPath,
		"IPv6映射私网地址":    "https://[::ffff:192.168.1.2]" + SupportTelegramWebhookPath,
		"Telegram不支持端口": "https://portal.example.com:9443" + SupportTelegramWebhookPath,
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			r := supportDeliveryTestSettings(t)
			previous, err := r.GetValue(ctx, SettingKeySupportDelivery)
			require.NoError(t, err)
			s := NewSupportDeliveryService(r, nil, nil, nil)
			s.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("无效回调地址必须在网络请求前被拒绝")
				return nil, errors.New("意外请求")
			})
			config := validSupportDeliverySettingsForTest()
			config.TelegramWebhookURL = callbackURL
			saved, err := s.UpdateSettings(ctx, config)
			require.Nil(t, saved)
			require.ErrorIs(t, err, ErrSupportDeliveryInvalid)
			appError := infraerrors.FromError(err)
			require.EqualValues(t, http.StatusBadRequest, appError.Code)
			require.Equal(t, "telegram_webhook_url", appError.Metadata["field"])
			require.Regexp(t, `\p{Han}`, appError.Message)
			current, readErr := r.GetValue(ctx, SettingKeySupportDelivery)
			require.NoError(t, readErr)
			require.Equal(t, previous, current)
		})
	}
}

func TestSupportDeliveryWebhookAcceptsTelegramSupportedPorts(t *testing.T) {
	for _, host := range []string{"portal.example.com", "portal.example.com:443", "portal.example.com:80", "portal.example.com:88", "portal.example.com:8443"} {
		t.Run(host, func(t *testing.T) {
			s := NewSupportDeliveryService(newNotificationEmailMemorySettingRepo(), nil, nil, nil)
			s.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return supportWebhookResponseForTest(http.StatusOK, `{"ok":true,"result":true}`), nil
			})
			config := validSupportDeliverySettingsForTest()
			config.TelegramWebhookURL = "https://" + host + SupportTelegramWebhookPath
			saved, err := s.UpdateSettings(context.Background(), config)
			require.NoError(t, err)
			require.True(t, saved.TelegramWebhookRegistered)
		})
	}
}

func TestSupportDeliveryWebhookRegistrationFailuresPreserveSettingsAndRedactErrors(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		networkErr bool
		field      string
	}{
		{name: "Telegram拒绝回调地址", status: http.StatusBadRequest, body: `{"ok":false,"error_code":400,"description":"PRIVATE_TELEGRAM_BODY"}`, field: "telegram_webhook_url"},
		{name: "Telegram拒绝机器人令牌", status: http.StatusUnauthorized, body: `{"ok":false,"error_code":401,"description":"PRIVATE_TELEGRAM_BODY"}`, field: "telegram_bot_token"},
		{name: "Telegram禁止机器人操作", status: http.StatusForbidden, body: `{"ok":false,"error_code":403,"description":"PRIVATE_TELEGRAM_BODY"}`, field: "telegram_bot_token"},
		{name: "Telegram找不到机器人", status: http.StatusNotFound, body: `{"ok":false,"error_code":404,"description":"PRIVATE_TELEGRAM_BODY"}`, field: "telegram_bot_token"},
		{name: "Telegram请求过于频繁", status: http.StatusTooManyRequests, body: `{"ok":false,"error_code":429,"description":"PRIVATE_TELEGRAM_BODY"}`},
		{name: "Telegram服务不可用", status: http.StatusBadGateway, body: "PRIVATE_TELEGRAM_BODY"},
		{name: "成功HTTP状态但接口失败", status: http.StatusOK, body: `{"ok":false,"error_code":400,"description":"PRIVATE_TELEGRAM_BODY"}`, field: "telegram_webhook_url"},
		{name: "接口返回false", status: http.StatusOK, body: `{"ok":true,"result":false}`},
		{name: "接口结果缺失", status: http.StatusOK, body: `{"ok":true}`},
		{name: "接口结果为空", status: http.StatusOK, body: `{"ok":true,"result":null}`},
		{name: "接口结果类型无效", status: http.StatusOK, body: `{"ok":true,"result":{"unexpected":"PRIVATE_TELEGRAM_BODY"}}`},
		{name: "非法接口响应", status: http.StatusOK, body: "PRIVATE_TELEGRAM_BODY"},
		{name: "网络失败", networkErr: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			r := supportDeliveryTestSettings(t)
			s := NewSupportDeliveryService(r, nil, nil, nil)
			previous, err := r.GetValue(ctx, SettingKeySupportDelivery)
			require.NoError(t, err)
			config := validSupportDeliverySettingsForTest()
			config.AdminEmails = []string{"changed@example.com"}
			config.TelegramWebhookURL = "https://portal.example.com" + SupportTelegramWebhookPath
			config.TelegramWebhookRegistered = true
			calls := 0
			s.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if test.networkErr {
					return nil, errors.New("PRIVATE_TELEGRAM_BODY " + config.TelegramBotToken + " " + config.TelegramWebhookSecret)
				}
				return supportWebhookResponseForTest(test.status, test.body), nil
			})
			saved, err := s.UpdateSettings(ctx, config)
			require.Nil(t, saved)
			require.Error(t, err)
			require.Equal(t, 1, calls)
			appError := infraerrors.FromError(err)
			if test.field != "" {
				require.EqualValues(t, http.StatusBadRequest, appError.Code)
				require.Equal(t, "SUPPORT_DELIVERY_INVALID", appError.Reason)
				require.Equal(t, test.field, appError.Metadata["field"])
			} else {
				require.EqualValues(t, http.StatusBadGateway, appError.Code)
				require.Equal(t, "SUPPORT_TELEGRAM_WEBHOOK_FAILED", appError.Reason)
			}
			require.Regexp(t, `\p{Han}`, appError.Message)
			require.Contains(t, appError.Message, "回调")
			response, marshalErr := json.Marshal(appError.Status)
			require.NoError(t, marshalErr)
			for _, credential := range []string{config.TelegramBotToken, config.TelegramWebhookSecret, "PRIVATE_TELEGRAM_BODY"} {
				require.NotContains(t, string(response), credential)
				require.NotContains(t, err.Error(), credential)
			}
			current, readErr := r.GetValue(ctx, SettingKeySupportDelivery)
			require.NoError(t, readErr)
			require.Equal(t, previous, current, "注册失败不能覆盖之前的通知配置")
		})
	}
}

func TestSupportDeliveryWebhookReportsLocalSaveFailureAfterRegistration(t *testing.T) {
	ctx := context.Background()
	r := &supportWebhookFailingSettingRepo{
		notificationEmailMemorySettingRepo: supportDeliveryTestSettings(t),
		setErr:                             errors.New("PRIVATE_DATABASE_ERROR"),
	}
	previous, err := r.GetValue(ctx, SettingKeySupportDelivery)
	require.NoError(t, err)
	s := NewSupportDeliveryService(r, nil, nil, nil)
	calls := 0
	s.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return supportWebhookResponseForTest(http.StatusOK, `{"ok":true,"result":true}`), nil
	})
	config := validSupportDeliverySettingsForTest()
	config.TelegramWebhookURL = "https://portal.example.com" + SupportTelegramWebhookPath
	saved, err := s.UpdateSettings(ctx, config)
	require.Nil(t, saved)
	require.Error(t, err)
	require.Equal(t, 1, calls)
	appError := infraerrors.FromError(err)
	require.EqualValues(t, http.StatusInternalServerError, appError.Code)
	require.Equal(t, "SUPPORT_TELEGRAM_WEBHOOK_SAVE_FAILED", appError.Reason)
	require.Contains(t, appError.Message, "已注册")
	require.Contains(t, appError.Message, "本地")
	require.Contains(t, appError.Message, "重新保存")
	require.NotContains(t, err.Error(), "PRIVATE_DATABASE_ERROR")
	require.NotContains(t, err.Error(), config.TelegramBotToken)
	require.NotContains(t, err.Error(), config.TelegramWebhookSecret)
	current, readErr := r.GetValue(ctx, SettingKeySupportDelivery)
	require.NoError(t, readErr)
	require.Equal(t, previous, current)
}
