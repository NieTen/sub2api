package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestOKPayBusinessErrorsPreserveSafeDetailsWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statusCode int
		body       string
		expected   string
	}{
		{
			name: "公开接口实际认证失败响应", statusCode: http.StatusOK,
			body:     `{"status":"warning","msg":"身份认证失败"}`,
			expected: "OKPay 返回业务失败状态（身份认证失败，请核对签名协议、商户 ID 和 Token）",
		},
		{
			name: "状态失败保留代码", statusCode: http.StatusOK,
			body:     `{"status":"error","code":20001,"msg":"签名错误"}`,
			expected: "OKPay 返回业务失败状态（code=20001；签名校验失败）",
		},
		{
			name: "代码失败不能因状态成功放行", statusCode: http.StatusOK,
			body:     `{"status":"success","code":"20002","message":"商户不存在"}`,
			expected: "OKPay 返回业务失败代码（code=20002；商户不存在或无效）",
		},
		{
			name: "成功代码不能放宽未知状态", statusCode: http.StatusOK,
			body:     `{"status":"ok","code":10000,"data":{"order_id":"pay-900","pay_url":"https://cashier.example/pay"}}`,
			expected: "OKPay 返回业务失败状态（code=10000）",
		},
		{
			name: "非成功HTTP状态保留业务原因", statusCode: http.StatusForbidden,
			body:     `{"status":"warning","code":20003,"msg":"身份认证失败"}`,
			expected: "OKPay HTTP 状态异常: 403（code=20003；身份认证失败，请核对签名协议、商户 ID 和 Token）",
		},
		{
			name: "HTTP失败不能被成功业务内容覆盖", statusCode: http.StatusServiceUnavailable,
			body:     `{"status":"success","code":10000,"data":{"order_id":"pay-900","pay_url":"https://cashier.example/pay"}}`,
			expected: "OKPay HTTP 状态异常: 503（code=10000）",
		},
		{
			name: "未知业务文案只保留有效代码", statusCode: http.StatusOK,
			body:     `{"status":"error","code":20004,"msg":"未知错误：用户 5639813059，订单 private-order"}`,
			expected: "OKPay 返回业务失败状态（code=20004）",
		},
		{
			name: "非JSON错误页面不泄漏正文", statusCode: http.StatusBadGateway,
			body:     `<html>private-token https://upstream.example/?token=private-token</html>`,
			expected: "OKPay HTTP 状态异常: 502",
		},
		{
			name: "拒绝异常代码类型及超长代码", statusCode: http.StatusOK,
			body:     `{"status":"error","code":"5639813059","msg":{"token":"private-token"}}`,
			expected: "OKPay 返回业务失败状态",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(tc.statusCode)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			provider := newOKPayForTest(t)
			provider.config["apiBase"] = server.URL
			provider.httpClient.Transport = server.Client().Transport
			response, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "site-100", Amount: "12.3"})
			require.Nil(t, response)
			require.EqualError(t, err, tc.expected)
			require.Equal(t, 1, requests, "创建支付失败后不能自动重试下单")
		})
	}
}

func TestOKPayErrorDiagnosticsNeverReflectCredentialsOrRequestData(t *testing.T) {
	for _, tc := range []struct {
		name, token string
		code        any
		message     string
	}{
		{name: "商户ID伪装业务代码", token: okpayTestToken, code: "123", message: "身份认证失败 token={token} sign={sign} id=123"},
		{name: "密钥伪装业务代码", token: "20001", code: "20001", message: "签名错误 {token}"},
		{name: "白名单文案也不能反射实际密钥", token: "身份认证失败", code: nil, message: "{token}"},
		{name: "请求签名伪装错误文案", token: okpayTestToken, code: true, message: "{sign}"},
		{name: "URL查询参数不能输出", token: okpayTestToken, code: "https://gateway.example/?token={token}", message: "身份认证失败 https://gateway.example/?sign={sign}&user=5639813059"},
		{name: "控制字符不能注入日志", token: okpayTestToken, code: "20001\n", message: "身份认证失败\nInjected: {token}"},
		{name: "超长上游消息不能输出", token: okpayTestToken, code: nil, message: string(make([]byte, 300))},
		{name: "未知非标准字段不能输出", token: okpayTestToken, code: map[string]string{"token": "private-token"}, message: "未知错误 {token}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var signature string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, r.ParseForm())
				signature = r.PostForm.Get("sign")
				replace := func(text string) string {
					return strings.NewReplacer("{token}", tc.token, "{sign}", signature).Replace(text)
				}
				code := tc.code
				if text, ok := code.(string); ok {
					code = replace(text)
				}
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"status": "error", "code": code, "msg": replace(tc.message),
					"debug": map[string]string{"token": tc.token, "sign": signature, "id": "123"},
				}))
			}))
			defer server.Close()
			provider := newOKPayForTest(t)
			provider.config["apiBase"], provider.config["token"] = server.URL, tc.token
			provider.httpClient.Transport = server.Client().Transport
			_, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "private-order", Amount: "12.3"})
			require.EqualError(t, err, "OKPay 返回业务失败状态")
			require.NotEmpty(t, signature)
			require.NotContains(t, err.Error(), tc.token)
			require.NotContains(t, err.Error(), signature)
			require.NotContains(t, err.Error(), "123")
		})
	}
}

func TestOKPayHTTPRequiresExplicitSuccessMarker(t *testing.T) {
	for _, operation := range []string{"创建支付", "查询已付款订单"} {
		for _, tc := range []struct {
			name     string
			envelope string
			wantErr  string
		}{
			{name: "缺少全部成功标记", wantErr: "缺少明确成功状态或代码"},
			{name: "仅成功状态", envelope: `"status":"success",`},
			{name: "仅成功代码", envelope: `"code":10000,`},
			{name: "成功状态不能覆盖失败代码", envelope: `"status":"success","code":20001,`, wantErr: "业务失败代码"},
			{name: "成功代码不能覆盖失败状态", envelope: `"status":"warning","code":10000,`, wantErr: "业务失败状态"},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// 完整订单数据不能代替外层成功标记，尤其不能让查单误认已付款。
					fmt.Fprintf(w, `{%s"data":{"order_id":"pay-900","unique_id":"site-100","pay_url":"https://cashier.example/pay","amount":"12.30","status":1}}`, tc.envelope)
				}))
				defer server.Close()
				provider := newOKPayForTest(t)
				provider.config["apiBase"] = server.URL
				provider.httpClient.Transport = server.Client().Transport
				if operation == "创建支付" {
					response, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "site-100", Amount: "12.3"})
					if tc.wantErr != "" {
						require.ErrorContains(t, err, tc.wantErr)
						require.Nil(t, response)
						return
					}
					require.NoError(t, err)
					require.Equal(t, "pay-900", response.TradeNo)
					return
				}
				response, err := provider.QueryOrderByMerchantOrderID(context.Background(), "site-100")
				if tc.wantErr != "" {
					require.ErrorContains(t, err, tc.wantErr)
					require.Nil(t, response)
					return
				}
				require.NoError(t, err)
				require.Equal(t, payment.ProviderStatusPaid, response.Status)
			})
		}
	}
}

func TestOKPaySignedCallbackKeepsOptionalEnvelopeFields(t *testing.T) {
	provider := newOKPayForTest(t)
	raw := strings.Replace(okpayTestDeposit, `"status":"success","code":10000,`, "", 1)
	notification, err := provider.VerifyNotification(context.Background(), signedOKPayTestJSON(t, raw), nil)
	require.NoError(t, err)
	require.Equal(t, payment.NotificationStatusSuccess, notification.Status)
	require.Equal(t, "site-100", notification.OrderID)
}
