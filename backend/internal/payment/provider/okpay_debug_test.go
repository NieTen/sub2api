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
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/balance" {
			fmt.Fprint(w, `{"status":"success","code":200,"msg":"私密成功文案","data":{"usdt":"9845631.73"}}`)
			return
		}
		fmt.Fprint(w, `{"status":"success","code":200,"data":{"order_id":"pay-private-900","pay_url":"https://cashier.example/private-pay-url"}}`)
	}))
	defer server.Close()
	provider := newOKPayForTest(t)
	provider.config["apiBase"], provider.config["debugLogging"] = server.URL, "true"
	provider.httpClient.Transport = server.Client().Transport
	ctx, logs := okpayDebugTestContext()
	response, err := provider.CreatePayment(ctx, payment.CreatePaymentRequest{OrderID: "site-100", Amount: "1.50"})
	require.NoError(t, err)
	require.NotEmpty(t, response.PayURL)
	diagnostic := provider.DiagnoseAuthentication(ctx)
	require.Equal(t, "both_authenticated", diagnostic.Conclusion)
	entries := logs.FilterMessage("OKPay debug").All()
	require.Len(t, entries, 4)
	require.Equal(t, 4, requests)
	for _, entry := range entries {
		require.Equal(t, zapcore.InfoLevel, entry.Level)
		require.Equal(t, "success", entry.ContextMap()["result"])
		encoded := okpayDebugTestJSON(t, entry)
		for _, private := range []string{"9845631.73", "私密成功文案", "private-pay-url", "pay-private-900", "upstream_messages"} {
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
