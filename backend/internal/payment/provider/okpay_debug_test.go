package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func okpayDebugTestContext() (context.Context, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.InfoLevel)
	return logger.IntoContext(context.Background(), zap.New(core).With(zap.String("request_id", "req-okpay"))), logs
}

func okpayDebugTestJSON(t *testing.T, entry observer.LoggedEntry) string {
	t.Helper()
	data, err := json.Marshal(entry.ContextMap())
	require.NoError(t, err)
	return string(data)
}

func TestOKPayDebugLoggingRequiresExplicitInstanceOptIn(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"status":"warning","code":400,"msg":"amount 必须大于 1 USDT"}`)
	}))
	defer server.Close()
	for _, enabled := range []string{"", "false", "invalid", "true"} {
		t.Run(enabled, func(t *testing.T) {
			ctx, logs := okpayDebugTestContext()
			provider, err := NewOKPay("4", map[string]string{"id": "shop-123", "token": okpayTestToken, "debugLogging": enabled})
			require.NoError(t, err)
			provider.config["apiBase"] = server.URL
			provider.httpClient.Transport = server.Client().Transport
			_, err = provider.CreatePayment(ctx, payment.CreatePaymentRequest{OrderID: "site-100", Amount: "0.99"})
			require.ErrorContains(t, err, "code=400")
			require.NotContains(t, err.Error(), "必须大于", "详细上游消息只能进入管理员日志")
			entries := logs.FilterMessage("OKPay debug").All()
			if enabled != "true" {
				require.Empty(t, entries)
				return
			}
			require.Len(t, entries, 1)
			require.Equal(t, zapcore.WarnLevel, entries[0].Level)
			fields := entries[0].ContextMap()
			require.Equal(t, "req-okpay", fields["request_id"])
			require.Equal(t, "4", fields["instance_id"])
			require.Equal(t, "payLink", fields["operation"])
			require.Equal(t, OKPaySignatureHMACSHA256, fields["signature_algorithm"])
			require.EqualValues(t, 200, fields["http_status"])
			require.Equal(t, "400", fields["business_code"])
			require.Equal(t, "rejected", fields["result"])
			require.Equal(t, true, fields["request_sent"])
			require.Contains(t, okpayDebugTestJSON(t, entries[0]), "amount 必须大于 1 USDT")
		})
	}
	require.Equal(t, 4, requests, "开启调试不能追加下单或重试")
}

func TestOKPayDebugRedactsReflectedCredentialsAndResumeURL(t *testing.T) {
	const merchant = "merchant-private-579"
	const token = "private Token+&%中"
	const order = "site-private-order"
	const subject = "用户私密标题"
	const resume = "private-resume+value"
	const opaqueJWT = "eyJhbGciOiJIUzI1NiJ9.eyJ1c2VyIjoiMzEyMzQ1Njc4In0.c2lnbmF0dXJl"
	returnURL := "https://site.example/payment/result?resume_token=" + url.QueryEscape(resume) + "&order_id=" + order
	var signature, nonce string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		signature, nonce = r.PostForm.Get("sign"), r.PostForm.Get("nonce")
		message := strings.Join([]string{
			"return_url 参数长度超限", token, url.QueryEscape(token), merchant, signature, strings.ToLower(signature),
			nonce, order, subject, returnURL, resume, opaqueJWT, "\r\n\x00\u202e",
		}, " | ")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"status": "warning", "code": 400, "msg": message,
			"message": "callback_url 仅支持 HTTPS",
			"data":    map[string]string{"balance": "9845631.73", "token": token, "pay_url": "https://cashier.example/secret-pay-url"},
		}))
	}))
	defer server.Close()
	provider, err := NewOKPay("4", map[string]string{"id": merchant, "token": token, "debugLogging": "true"})
	require.NoError(t, err)
	provider.config["apiBase"] = server.URL
	provider.httpClient.Transport = server.Client().Transport
	ctx, logs := okpayDebugTestContext()
	_, err = provider.CreatePayment(ctx, payment.CreatePaymentRequest{OrderID: order, Amount: "1.50", Subject: subject, ReturnURL: returnURL, NotifyURL: "https://site.example/api/v1/payment/webhook/okpay"})
	require.Error(t, err)
	entries := logs.FilterMessage("OKPay debug").All()
	require.Len(t, entries, 1)
	encoded := okpayDebugTestJSON(t, entries[0])
	for _, secret := range []string{token, url.QueryEscape(token), merchant, signature, strings.ToLower(signature), nonce, order, subject, returnURL, resume, opaqueJWT, "9845631.73", "secret-pay-url", `\r`, `\n`, `\u0000`, "\u202e"} {
		require.NotEmpty(t, secret)
		require.NotContains(t, encoded, secret)
	}
	require.Contains(t, encoded, "return_url 参数长度超限")
	require.Contains(t, encoded, "callback_url 仅支持 HTTPS")
	fields := entries[0].ContextMap()
	request := fields["request"].(map[string]any)
	require.Equal(t, "1.50", request["amount"])
	require.Equal(t, "USDT", request["coin"])
	require.Equal(t, "0", request["status"])
	require.EqualValues(t, 64, request["sign_bytes"])
	require.EqualValues(t, 32, request["nonce_bytes"])
	link := request["return_url"].(map[string]any)
	require.Equal(t, "site.example", link["host"])
	require.EqualValues(t, len(returnURL), link["bytes"])
	require.EqualValues(t, 2, link["query_params"])
	require.NotContains(t, link, "path")
}

func TestOKPayDebugHTTPAndInvalidResponseAreDistinguishable(t *testing.T) {
	for _, tc := range []struct {
		name       string
		httpStatus int
		body       string
	}{
		{"非成功HTTP的JSON参数错误", 400, `{"code":400,"message":"callback_url 不允许此域名"}`},
		{"非JSON网关页面", 502, `<html>private-error-body token=secret</html>`},
		{"HTTP成功但JSON损坏", 200, `{"msg":"private-error-body",`},
		{"响应超限", 200, strings.Repeat("private-error-body", okpayMaxResponseSize/10)},
		{"缺少成功状态", 200, `{"data":{"secret":"private-error-body"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(tc.httpStatus)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			ctx, logs := okpayDebugTestContext()
			provider := newOKPayForTest(t)
			provider.config["apiBase"], provider.config["debugLogging"] = server.URL, "true"
			provider.httpClient.Transport = server.Client().Transport
			_, err := provider.CreatePayment(ctx, payment.CreatePaymentRequest{OrderID: "site-100", Amount: "1.50"})
			require.Error(t, err)
			require.Equal(t, 1, requests)
			entries := logs.FilterMessage("OKPay debug").All()
			require.Len(t, entries, 1)
			require.EqualValues(t, tc.httpStatus, entries[0].ContextMap()["http_status"])
			require.NotEqual(t, "success", entries[0].ContextMap()["result"])
			encoded := okpayDebugTestJSON(t, entries[0])
			require.NotContains(t, encoded, "private-error-body")
			if strings.Contains(tc.body, "callback_url") {
				require.Contains(t, encoded, "callback_url 不允许此域名")
			}
		})
	}
}

func TestOKPayDebugNetworkFailureDoesNotLogUnderlyingError(t *testing.T) {
	provider := newOKPayForTest(t)
	provider.config["debugLogging"] = "true"
	transport := &okpayDiagnosticTestTransport{}
	provider.httpClient.Transport = transport
	ctx, logs := okpayDebugTestContext()
	_, err := provider.CreatePayment(ctx, payment.CreatePaymentRequest{OrderID: "site-100", Amount: "1.50"})
	require.Error(t, err)
	require.Equal(t, 1, transport.calls)
	entries := logs.FilterMessage("OKPay debug").All()
	require.Len(t, entries, 1)
	require.EqualValues(t, 0, entries[0].ContextMap()["http_status"])
	require.Equal(t, "request_failed", entries[0].ContextMap()["result"])
	encoded := okpayDebugTestJSON(t, entries[0])
	for _, private := range []string{okpayTestToken, "9845631.73", "private-sign", "网络错误包含私密内容"} {
		require.NotContains(t, encoded, private)
	}
}

func TestOKPayDebugSuccessOmitsBalanceAndCashierData(t *testing.T) {
	const returnURL = "https://zzzai.pro/payment/result?order_id=42&resume_token=private-test-resume"
	const callbackURL = "https://zzzai.pro/api/v1/payment/webhook/okpay"
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/balance" {
			fmt.Fprint(w, `{"status":"success","code":200,"msg":"私密成功文案","data":{"usdt":"9845631.73"}}`)
			return
		}
		require.NoError(t, r.ParseForm())
		require.Equal(t, returnURL, r.PostForm.Get("return_url"))
		require.Equal(t, callbackURL, r.PostForm.Get("callback_url"))
		fmt.Fprint(w, `{"status":"success","code":200,"data":{"order_id":"pay-private-900","pay_url":"https://cashier.example/private-pay-url"}}`)
	}))
	defer server.Close()
	provider := newOKPayForTest(t)
	provider.config["apiBase"], provider.config["debugLogging"] = server.URL, "true"
	provider.httpClient.Transport = server.Client().Transport
	ctx, logs := okpayDebugTestContext()
	response, err := provider.CreatePayment(ctx, payment.CreatePaymentRequest{OrderID: "site-100", Amount: "1.50", ReturnURL: returnURL, NotifyURL: callbackURL})
	require.NoError(t, err)
	require.NotEmpty(t, response.PayURL)
	payEntries := logs.FilterMessage("OKPay debug").All()
	require.Len(t, payEntries, 1, "正常 HTTPS 地址拼接不得重复记录本地校验日志")
	payFields := payEntries[0].ContextMap()
	require.Equal(t, "upstream", payFields["stage"])
	require.NotContains(t, payFields, "validation_field")
	for _, field := range []string{"return_url", "callback_url"} {
		link := payFields["request"].(map[string]any)[field].(map[string]any)
		require.Equal(t, true, link["valid"])
		require.Equal(t, "https", link["scheme"])
		require.Equal(t, "zzzai.pro", link["host"])
	}
	diagnostic := provider.DiagnoseAuthentication(ctx)
	require.Equal(t, "both_authenticated", diagnostic.Conclusion)
	entries := logs.FilterMessage("OKPay debug").All()
	require.Len(t, entries, 4)
	require.Equal(t, 4, requests)
	for _, entry := range entries {
		require.Equal(t, zapcore.InfoLevel, entry.Level)
		require.Equal(t, "success", entry.ContextMap()["result"])
		encoded := okpayDebugTestJSON(t, entry)
		for _, private := range []string{"9845631.73", "私密成功文案", "private-pay-url", "pay-private-900", "upstream_messages", returnURL, callbackURL, "private-test-resume"} {
			require.NotContains(t, encoded, private)
		}
	}
}

func TestOKPayDebugRedactsBeforeTruncatingLongMessages(t *testing.T) {
	secret := "Token-At-Truncation-Boundary-Private"
	text := strings.Repeat("中", okpayDebugMessageLimit-5) + secret + " 后续原因"
	redacted := okpayDebugRedactMessage(text, []string{secret})
	require.NotContains(t, redacted, "Token")
	require.LessOrEqual(t, len([]rune(redacted)), okpayDebugMessageLimit+1)
	require.Contains(t, redacted, "***")
}

func TestOKPayDebugRedactsEncodedCredentialVariants(t *testing.T) {
	const secret = "Private+/Token中"
	encoded := url.QueryEscape(secret)
	lowerEscapes := strings.NewReplacer("%2B", "%2b", "%2F", "%2f", "%E4", "%e4", "%B8", "%b8", "%AD", "%ad").Replace(encoded)
	for _, reflected := range []string{secret, encoded, lowerEscapes, strings.ReplaceAll(secret, "/", `\/`)} {
		redacted := okpayDebugRedactMessage("参数被拒绝："+reflected, []string{secret})
		require.Equal(t, "参数被拒绝：***", redacted)
	}
}

func okpayDebugValidationTestProvider(t *testing.T, enabled string) (*OKPay, *okpayDiagnosticTestTransport) {
	t.Helper()
	provider, err := NewOKPay("4", map[string]string{
		"id": "private-validation-merchant", "token": "private-validation-token", "debugLogging": enabled,
	})
	require.NoError(t, err)
	transport := &okpayDiagnosticTestTransport{}
	provider.httpClient.Transport = transport
	return provider, transport
}

func okpayRequireLocalValidationLog(t *testing.T, logs *observer.ObservedLogs, transport *okpayDiagnosticTestTransport, field, reason string) map[string]any {
	t.Helper()
	require.Zero(t, transport.calls, "本地校验失败不得访问上游或重试下单")
	entries := logs.FilterMessage("OKPay debug").All()
	require.Len(t, entries, 1, "一次本地校验失败只能有一条诊断日志")
	require.Equal(t, zapcore.WarnLevel, entries[0].Level)
	fields := entries[0].ContextMap()
	require.Equal(t, "req-okpay", fields["request_id"])
	require.Equal(t, "payLink", fields["operation"])
	require.Equal(t, "validation", fields["stage"])
	require.Equal(t, "validation_failed", fields["result"])
	require.Equal(t, false, fields["request_sent"])
	require.EqualValues(t, 0, fields["http_status"])
	require.Equal(t, field, fields["validation_field"])
	require.Equal(t, reason, fields["validation_reason"])
	require.NotEmpty(t, fields["validation_message"])
	require.NotContains(t, fields, "business_code")
	require.NotContains(t, fields, "upstream_messages")
	encoded := okpayDebugTestJSON(t, entries[0])
	for _, secret := range []string{
		"private-validation-merchant", "private-validation-token", "private-validation-order", "私密校验标题",
		"private-user", "private-password", "private-resume", "private-fragment", "private-path",
	} {
		require.NotContains(t, encoded, secret)
	}
	return fields
}

func TestOKPayDebugLocalURLValidationExplainsRejectedFieldWithoutNetwork(t *testing.T) {
	for _, tc := range []struct {
		name        string
		field       string
		value       string
		reason      string
		scheme      string
		hasUserinfo bool
		hasControls bool
	}{
		{name: "返回地址使用HTTP", field: "return_url", value: "http://site.example/private-path?resume_token=private-resume#private-fragment", reason: "scheme_not_https", scheme: "http"},
		{name: "回调地址使用HTTP", field: "callback_url", value: "http://site.example/private-path?secret=private-resume#private-fragment", reason: "scheme_not_https", scheme: "http"},
		{name: "相对返回地址", field: "return_url", value: "/private-path?resume_token=private-resume", reason: "scheme_not_https", scheme: "relative"},
		{name: "非HTTP协议", field: "callback_url", value: "ftp://site.example/private-path", reason: "scheme_not_https", scheme: "other"},
		{name: "HTTPS缺少主机", field: "callback_url", value: "https:///private-path?resume_token=private-resume", reason: "missing_host", scheme: "https"},
		{name: "返回地址含凭据", field: "return_url", value: "https://private-user:private-password@site.example/private-path?resume_token=private-resume#private-fragment", reason: "userinfo_not_allowed", scheme: "https", hasUserinfo: true},
		{name: "回调地址含凭据", field: "callback_url", value: "https://private-user:private-password@site.example/private-path", reason: "userinfo_not_allowed", scheme: "https", hasUserinfo: true},
		{name: "地址中间含控制字符", field: "return_url", value: "https://site.example/private-path\nmore?resume_token=private-resume", reason: "control_characters", hasControls: true},
		{name: "地址百分号转义损坏", field: "return_url", value: "https://site.example/private-path%ZZ?resume_token=private-resume#private-fragment", reason: "parse_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider, transport := okpayDebugValidationTestProvider(t, "true")
			request := payment.CreatePaymentRequest{
				OrderID: "private-validation-order", Amount: "1.50", Subject: "私密校验标题",
				ReturnURL: "https://site.example/return", NotifyURL: "https://site.example/callback",
			}
			if tc.field == "return_url" {
				request.ReturnURL = tc.value
			} else {
				request.NotifyURL = tc.value
			}
			ctx, logs := okpayDebugTestContext()
			response, err := provider.CreatePayment(ctx, request)
			require.Nil(t, response)
			require.ErrorContains(t, err, tc.field, "公开错误应准确指出返回地址或回调地址")
			fields := okpayRequireLocalValidationLog(t, logs, transport, tc.field, tc.reason)
			message, ok := fields["validation_message"].(string)
			require.True(t, ok)
			require.Contains(t, err.Error(), message)
			for _, secret := range []string{tc.value, "private-user", "private-password", "private-resume", "private-fragment", "private-path"} {
				require.NotContains(t, err.Error(), secret)
			}
			requestSummary := fields["request"].(map[string]any)
			require.EqualValues(t, 0, requestSummary["sign_bytes"])
			require.EqualValues(t, 0, requestSummary["nonce_bytes"])
			link := requestSummary[tc.field].(map[string]any)
			require.Equal(t, false, link["valid"])
			require.Equal(t, tc.reason, link["validation_reason"])
			require.Equal(t, "request", link["source"])
			require.Equal(t, true, link["present"])
			require.EqualValues(t, len(tc.value), link["bytes"])
			require.Equal(t, tc.hasUserinfo, link["has_userinfo"])
			require.Equal(t, tc.hasControls, link["has_control_chars"])
			if tc.scheme != "" {
				require.Equal(t, tc.scheme, link["scheme"])
			}
			require.NotContains(t, link, "path")
			require.NotContains(t, link, "query")
			require.NotContains(t, link, "fragment")
			require.NotContains(t, link, "userinfo")
		})
	}
}

func TestOKPayDebugLocalURLValidationReportsBothAddressesAndFirstFailure(t *testing.T) {
	provider, transport := okpayDebugValidationTestProvider(t, "true")
	ctx, logs := okpayDebugTestContext()
	_, err := provider.CreatePayment(ctx, payment.CreatePaymentRequest{
		OrderID: "private-validation-order", Amount: "1.50", Subject: "私密校验标题",
		ReturnURL: "http://site.example/private-path", NotifyURL: "https://private-user:private-password@site.example/private-path",
	})
	require.ErrorContains(t, err, "return_url")
	fields := okpayRequireLocalValidationLog(t, logs, transport, "return_url", "scheme_not_https")
	summary := fields["request"].(map[string]any)
	for key, reason := range map[string]string{"return_url": "scheme_not_https", "callback_url": "userinfo_not_allowed"} {
		link := summary[key].(map[string]any)
		require.Equal(t, false, link["valid"])
		require.Equal(t, reason, link["validation_reason"])
		require.Equal(t, "request", link["source"])
	}
}

func TestOKPayDebugLocalValidationUsesConfigurationFallback(t *testing.T) {
	for _, field := range []string{"return_url", "callback_url"} {
		t.Run(field, func(t *testing.T) {
			provider, transport := okpayDebugValidationTestProvider(t, "true")
			configKey := "returnUrl"
			if field == "callback_url" {
				configKey = "notifyUrl"
			}
			provider.config[configKey] = "  http://config.example/private-path?resume_token=private-resume  "
			ctx, logs := okpayDebugTestContext()
			_, err := provider.CreatePayment(ctx, payment.CreatePaymentRequest{
				OrderID: "private-validation-order", Amount: "1.50", ReturnURL: " \t ", NotifyURL: " \t ",
			})
			require.ErrorContains(t, err, field)
			fields := okpayRequireLocalValidationLog(t, logs, transport, field, "scheme_not_https")
			link := fields["request"].(map[string]any)[field].(map[string]any)
			require.Equal(t, "provider_config", link["source"])
			require.Equal(t, "config.example", link["host"])
			require.EqualValues(t, len(strings.TrimSpace(provider.config[configKey])), link["bytes"])
		})
	}
}

func TestOKPayDebugRequestAddressesOverrideConfigurationAndEmptyURLsRemainAllowed(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprintf("地址为空=%t", empty), func(t *testing.T) {
			provider, transport := okpayDebugValidationTestProvider(t, "true")
			request := payment.CreatePaymentRequest{OrderID: "private-validation-order", Amount: "1.50"}
			if !empty {
				provider.config["returnUrl"] = "http://config.example/private-path?resume_token=private-resume"
				provider.config["notifyUrl"] = "http://config.example/private-path#private-fragment"
				request.ReturnURL = "  https://request.example/return  "
				request.NotifyURL = "  https://request.example/callback  "
			}
			ctx, logs := okpayDebugTestContext()
			_, err := provider.CreatePayment(ctx, request)
			require.Error(t, err, "模拟传输固定报错，但本地参数校验必须已经通过")
			require.Equal(t, 1, transport.calls, "合法请求只发起一次上游调用，不额外记校验失败或重试")
			entries := logs.FilterMessage("OKPay debug").All()
			require.Len(t, entries, 1)
			fields := entries[0].ContextMap()
			require.Equal(t, "upstream", fields["stage"])
			require.Equal(t, true, fields["request_sent"])
			require.NotContains(t, fields, "validation_field")
			for _, field := range []string{"return_url", "callback_url"} {
				link := fields["request"].(map[string]any)[field].(map[string]any)
				require.Equal(t, "", link["validation_reason"])
				if empty {
					require.Equal(t, false, link["present"])
					require.Equal(t, false, link["valid"])
				} else {
					require.Equal(t, "request.example", link["host"])
					require.Equal(t, true, link["valid"])
				}
			}
		})
	}
}

func TestOKPayDebugLocalOrderAndAmountValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		order  string
		amount string
		field  string
		reason string
	}{
		{name: "订单号为空", order: " \t ", amount: "1.50", field: "unique_id", reason: "missing_order_id"},
		{name: "金额格式错误", order: "private-validation-order", amount: "private-invalid-amount", field: "amount", reason: "invalid_amount"},
		{name: "金额非正数", order: "private-validation-order", amount: "0", field: "amount", reason: "invalid_amount"},
		{name: "金额超过两位小数", order: "private-validation-order", amount: "1.501", field: "amount", reason: "amount_precision"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider, transport := okpayDebugValidationTestProvider(t, "true")
			ctx, logs := okpayDebugTestContext()
			_, err := provider.CreatePayment(ctx, payment.CreatePaymentRequest{OrderID: tc.order, Amount: tc.amount})
			require.Error(t, err)
			fields := okpayRequireLocalValidationLog(t, logs, transport, tc.field, tc.reason)
			for _, key := range []string{"return_url", "callback_url"} {
				link := fields["request"].(map[string]any)[key].(map[string]any)
				require.Equal(t, "empty", link["source"])
				require.Equal(t, false, link["present"])
				require.Equal(t, false, link["valid"])
				require.Equal(t, "", link["validation_reason"])
			}
			require.NotContains(t, err.Error(), "private-invalid-amount")
			require.NotContains(t, okpayDebugTestJSON(t, logs.FilterMessage("OKPay debug").All()[0]), "private-invalid-amount")
		})
	}
}

func TestOKPayDebugLocalValidationKeepsLoggingOptIn(t *testing.T) {
	for _, enabled := range []string{"", "false", "invalid"} {
		t.Run(enabled, func(t *testing.T) {
			provider, transport := okpayDebugValidationTestProvider(t, enabled)
			ctx, logs := okpayDebugTestContext()
			_, err := provider.CreatePayment(ctx, payment.CreatePaymentRequest{
				OrderID: "private-validation-order", Amount: "1.50", ReturnURL: "http://site.example/private-path?resume_token=private-resume",
			})
			require.ErrorContains(t, err, "return_url")
			require.NotContains(t, err.Error(), "private-path")
			require.NotContains(t, err.Error(), "private-resume")
			require.Zero(t, transport.calls)
			require.Empty(t, logs.FilterMessage("OKPay debug").All())
		})
	}
}
