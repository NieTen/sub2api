//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type okpayReturnURLIntegrationTransport struct {
	delegate *usdtQuoteIntegrationTransport
	requests []url.Values
	calls    int
}

func (r *okpayReturnURLIntegrationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	r.calls++
	if err := request.ParseForm(); err != nil {
		return nil, err
	}
	values := make(url.Values, len(request.PostForm))
	for key, items := range request.PostForm {
		values[key] = append([]string(nil), items...)
	}
	r.requests = append(r.requests, values)
	// 既有模拟器仅接受 okpay.test，并直接构造响应，绝不访问真实网关。
	return r.delegate.RoundTrip(request)
}

func newOKPayReturnURLIntegrationFixture(t *testing.T, configuredURL string, signingKey []byte) (*usdtQuoteIntegrationFixture, *okpayReturnURLIntegrationTransport) {
	t.Helper()
	fixture := newUSDTQuoteIntegrationFixture(t, payment.TypeOKPay, payment.TypeOKPay)
	fixture.balancer.selection.Config["returnUrl"] = configuredURL
	rawConfig, err := json.Marshal(fixture.balancer.selection.Config)
	require.NoError(t, err)
	instanceID, err := strconv.ParseInt(fixture.balancer.selection.InstanceID, 10, 64)
	require.NoError(t, err)
	_, err = fixture.service.entClient.PaymentProviderInstance.UpdateOneID(instanceID).
		SetConfig(string(rawConfig)).Save(context.Background())
	require.NoError(t, err)
	fixture.service.resumeService = NewPaymentResumeService(signingKey)
	transport := &okpayReturnURLIntegrationTransport{delegate: fixture.transport}
	// 原夹具已注册恢复 DefaultTransport 的清理，所有调用都交给本地模拟器。
	http.DefaultTransport = transport
	return fixture, transport
}

func TestOKPayReturnURLIntegrationSavedHTTPSOverridesHTTPOrIPClient(t *testing.T) {
	for _, tc := range []struct {
		name          string
		clientHost    string
		clientURL     string
		configuredURL string
		canonicalURL  string
	}{
		{
			name: "HTTP客户端由已保存HTTPS覆盖", clientHost: "client.example",
			clientURL:     "http://client.example/payment/result?resume_token=old-client-token#client-fragment",
			configuredURL: "https://zzzai.pro/payment/result", canonicalURL: "https://zzzai.pro/payment/result",
		},
		{
			name: "IP客户端保留配置子路径和普通参数", clientHost: "192.0.2.10:8080",
			clientURL:     "http://192.0.2.10:8080/payment/result?resume_token=old-client-token&status=cancelled",
			configuredURL: "https://zzzai.pro/sub/app/payment/result?lang=zh-CN&campaign=a%2Bb&order_id=old-id&out_trade_no=old-trade&resume_token=old-config-token&status=cancelled#saved-fragment",
			canonicalURL:  "https://zzzai.pro/sub/app/payment/result?campaign=a%2Bb&lang=zh-CN",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, transport := newOKPayReturnURLIntegrationFixture(t, tc.configuredURL, []byte("test-payment-resume-signing-key"))
			response, err := fixture.service.CreateOrder(context.Background(), CreateOrderRequest{
				UserID: fixture.userID, Amount: 10, PaymentType: payment.TypeOKPay, OrderType: payment.OrderTypeBalance,
				SrcHost: tc.clientHost, SrcURL: tc.clientURL, ReturnURL: tc.clientURL, PaymentSource: PaymentSourceHostedRedirect,
			})
			require.NoError(t, err, "OKPay 应使用后台已保存的 HTTPS 返回地址，不受客户端 HTTP 或 IP 访问影响")
			require.NotNil(t, response)
			require.Equal(t, 1, transport.calls)
			require.Len(t, transport.requests, 1)
			order, err := fixture.service.entClient.PaymentOrder.Get(context.Background(), response.OrderID)
			require.NoError(t, err)
			actual, err := url.Parse(transport.requests[0].Get("return_url"))
			require.NoError(t, err)
			expected, err := url.Parse(tc.canonicalURL)
			require.NoError(t, err)
			require.Equal(t, "https", actual.Scheme)
			require.Equal(t, expected.Host, actual.Host)
			require.Equal(t, expected.Path, actual.Path)
			require.Empty(t, actual.Fragment)
			query := actual.Query()
			require.Equal(t, []string{strconv.FormatInt(order.ID, 10)}, query["order_id"])
			require.Equal(t, []string{order.OutTradeNo}, query["out_trade_no"])
			require.Equal(t, []string{"success"}, query["status"])
			require.Len(t, response.ResumeToken, 102)
			require.Equal(t, []string{response.ResumeToken}, query["resume_token"])
			for key, values := range expected.Query() {
				require.Equal(t, values, query[key], "配置中的普通查询参数必须完整保留")
			}
			for _, stale := range []string{"old-client-token", "old-config-token", "old-id", "old-trade", "saved-fragment", "client-fragment"} {
				require.NotContains(t, actual.String(), stale)
			}
			claims, err := fixture.service.resumeService.ParseToken(response.ResumeToken)
			require.NoError(t, err)
			require.Empty(t, claims.CanonicalReturnURL, "短令牌不再重复嵌入可信返回地址")
			require.Equal(t, order.ID, claims.OrderID)
			require.Equal(t, fixture.userID, claims.UserID)
			require.Equal(t, fixture.balancer.selection.InstanceID, claims.ProviderInstanceID)
			require.Equal(t, payment.TypeOKPay, claims.ProviderKey)
			require.Equal(t, payment.TypeOKPay, claims.PaymentType)
			legacyClaims := *claims
			legacyClaims.CanonicalReturnURL = tc.canonicalURL
			legacyToken, err := fixture.service.resumeService.CreateToken(legacyClaims)
			require.NoError(t, err)
			legacyURL, err := buildPaymentReturnURL(tc.canonicalURL, order.ID, order.OutTradeNo, legacyToken)
			require.NoError(t, err)
			require.Less(t, len(actual.String()), len(legacyURL)-100, "压缩主体应显著缩短最终链接，同时保留全部恢复查询参数")
			if tc.canonicalURL == "https://zzzai.pro/payment/result" {
				// 这里只约束本项目典型链接长度，不代表 OKPay 公开过此长度上限。
				require.LessOrEqual(t, len(actual.String()), 220)
			}
			for _, key := range []string{"order_id", "out_trade_no", "resume_token", "status"} {
				query.Del(key)
			}
			actual.RawQuery = query.Encode()
			require.Equal(t, tc.canonicalURL, actual.String(), "缩短令牌不能改变已经验证的 HTTPS 目标、子路径和普通参数")
		})
	}
}

func TestOKPayReturnURLIntegrationCompactTokenResolvesOnlyMatchingOrder(t *testing.T) {
	ctx := context.Background()
	fixture, transport := newOKPayReturnURLIntegrationFixture(t, "https://zzzai.pro/payment/result", []byte("test-payment-resume-signing-key"))
	response, err := fixture.service.CreateOrder(ctx, CreateOrderRequest{
		UserID: fixture.userID, Amount: 10, PaymentType: payment.TypeOKPay, OrderType: payment.OrderTypeBalance,
		SrcHost: "192.0.2.20:8080", ReturnURL: "http://192.0.2.20:8080/payment/result",
	})
	require.NoError(t, err)
	require.Len(t, response.ResumeToken, 102)
	// 仅修改夹具订单状态，避免查询触发主动对账；本测试不执行付款或余额变更。
	_, err = fixture.service.entClient.PaymentOrder.UpdateOneID(response.OrderID).SetStatus(OrderStatusCompleted).Save(ctx)
	require.NoError(t, err)
	resolved, err := fixture.service.GetPublicOrderByResumeToken(ctx, response.ResumeToken)
	require.NoError(t, err, "跨域结果页不依赖登录态或浏览器本地缓存即可恢复订单")
	require.Equal(t, response.OrderID, resolved.ID)
	require.Equal(t, fixture.userID, resolved.UserID)
	claims, err := fixture.service.resumeService.ParseToken(response.ResumeToken)
	require.NoError(t, err)
	for _, mismatch := range []string{"用户", "实例"} {
		t.Run(mismatch, func(t *testing.T) {
			changed := *claims
			if mismatch == "用户" {
				changed.UserID++
			} else {
				instanceID, err := strconv.ParseInt(changed.ProviderInstanceID, 10, 64)
				require.NoError(t, err)
				changed.ProviderInstanceID = strconv.FormatInt(instanceID+1, 10)
			}
			token, err := fixture.service.resumeService.CreateOKPayToken(changed)
			require.NoError(t, err)
			got, err := fixture.service.GetPublicOrderByResumeToken(ctx, token)
			require.Error(t, err)
			require.Nil(t, got)
			require.Equal(t, "INVALID_RESUME_TOKEN", infraerrors.Reason(err))
		})
	}
	require.Equal(t, 1, transport.calls, "恢复及不匹配校验不能额外创建支付链接")
}

func TestOKPayReturnURLIntegrationWithoutSigningKeyRemovesStaleToken(t *testing.T) {
	fixture, transport := newOKPayReturnURLIntegrationFixture(t,
		"https://zzzai.pro/sub/payment/result?lang=zh&resume_token=old-config-token&resume_token=another-old-token&order_id=old-id&out_trade_no=old-trade&status=failed#saved-fragment", nil)
	response, err := fixture.service.CreateOrder(context.Background(), CreateOrderRequest{
		UserID: fixture.userID, Amount: 10, PaymentType: payment.TypeOKPay, OrderType: payment.OrderTypeBalance,
		SrcHost: "192.0.2.20:8080", ReturnURL: "http://192.0.2.20:8080/payment/result?resume_token=old-client-token",
	})
	require.NoError(t, err)
	require.Equal(t, 1, transport.calls)
	require.Len(t, transport.requests, 1)
	require.Empty(t, response.ResumeToken)
	actual, err := url.Parse(transport.requests[0].Get("return_url"))
	require.NoError(t, err)
	require.Equal(t, "https", actual.Scheme)
	require.Equal(t, "zzzai.pro", actual.Host)
	require.Equal(t, "/sub/payment/result", actual.Path)
	require.Empty(t, actual.Fragment)
	query := actual.Query()
	require.NotContains(t, query, "resume_token", "没有签名密钥时不能继承配置或客户端中的旧恢复令牌")
	require.Equal(t, "zh", query.Get("lang"))
	require.Equal(t, []string{strconv.FormatInt(response.OrderID, 10)}, query["order_id"])
	require.Equal(t, []string{response.OutTradeNo}, query["out_trade_no"])
	require.Equal(t, []string{"success"}, query["status"])
	for _, stale := range []string{"old-config-token", "another-old-token", "old-client-token", "old-id", "old-trade", "saved-fragment"} {
		require.NotContains(t, actual.String(), stale)
	}
}

func TestOKPayReturnURLIntegrationInvalidSavedConfigNeverCallsGateway(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
	}{
		{"HTTP配置", "http://saved.example/payment/result?resume_token=saved-secret"},
		{"相对地址", "/payment/result?resume_token=saved-secret"},
		{"缺少主机", "https:///payment/result?resume_token=saved-secret"},
		{"用户名密码", "https://saved-user:saved-password@saved.example/payment/result?resume_token=saved-secret"},
		{"控制字符", "https://saved.example/payment/\nresult?resume_token=saved-secret"},
		{"反斜杠", "https://saved.example/payment\\result?resume_token=saved-secret"},
		{"转义损坏", "https://saved.example/payment/%ZZ?resume_token=saved-secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, transport := newOKPayReturnURLIntegrationFixture(t, tc.url, []byte("test-payment-resume-signing-key"))
			response, err := fixture.service.CreateOrder(context.Background(), CreateOrderRequest{
				UserID: fixture.userID, Amount: 10, PaymentType: payment.TypeOKPay, OrderType: payment.OrderTypeBalance,
				SrcHost: "client.example", ReturnURL: "https://client.example/payment/result",
			})
			require.Error(t, err, "后台返回地址无效时不能悄悄切回客户端地址")
			require.Nil(t, response)
			require.Zero(t, transport.calls)
			require.Empty(t, transport.requests)
			require.Empty(t, fixture.transport.amounts)
			for _, secret := range []string{tc.url, "saved-user", "saved-password", "saved-secret"} {
				require.NotContains(t, err.Error(), secret)
			}
		})
	}
}

func TestResolveCreateOrderReturnURLPreservesFallbackAndOtherProviders(t *testing.T) {
	for _, tc := range []struct {
		name       string
		selection  *payment.InstanceSelection
		request    CreateOrderRequest
		expected   string
		expectFail bool
	}{
		{
			name: "OKPay空配置保持同源HTTP规则", selection: &payment.InstanceSelection{ProviderKey: payment.TypeOKPay, Config: map[string]string{}},
			request:  CreateOrderRequest{SrcHost: "client.example", ReturnURL: "http://client.example/payment/result?lang=zh#fragment"},
			expected: "http://client.example/payment/result?lang=zh",
		},
		{
			name: "空白配置保持浏览器来源规则", selection: &payment.InstanceSelection{ProviderKey: payment.TypeOKPay, Config: map[string]string{"returnUrl": " \t "}},
			request:  CreateOrderRequest{SrcHost: "api.example", SrcURL: "https://frontend.example/purchase", ReturnURL: "https://frontend.example/payment/result"},
			expected: "https://frontend.example/payment/result",
		},
		{
			name: "OKPay空配置拒绝外域", selection: &payment.InstanceSelection{ProviderKey: payment.TypeOKPay, Config: map[string]string{}},
			request: CreateOrderRequest{SrcHost: "client.example", ReturnURL: "https://external.example/payment/result"}, expectFail: true,
		},
		{
			name: "其他服务商不采用配置覆盖", selection: &payment.InstanceSelection{ProviderKey: payment.TypeEasyPay, Config: map[string]string{"returnUrl": "https://configured.example/another/path"}},
			request:  CreateOrderRequest{PaymentType: payment.TypeOKPay, SrcHost: "client.example", ReturnURL: "https://client.example/payment/result?lang=zh"},
			expected: "https://client.example/payment/result?lang=zh",
		},
		{
			name: "其他服务商仍拒绝外域客户端地址", selection: &payment.InstanceSelection{ProviderKey: payment.TypeEasyPay, Config: map[string]string{"returnUrl": "https://external.example/payment/result"}},
			request: CreateOrderRequest{SrcHost: "client.example", ReturnURL: "https://external.example/payment/result"}, expectFail: true,
		},
		{
			name: "其他服务商仍限制支付结果路径", selection: &payment.InstanceSelection{ProviderKey: payment.TypeStripe},
			request: CreateOrderRequest{SrcHost: "client.example", ReturnURL: "https://client.example/sub/payment/result"}, expectFail: true,
		},
		{
			name: "无实例沿用原规范化", request: CreateOrderRequest{SrcHost: "client.example", ReturnURL: "https://client.example/payment/result#fragment"},
			expected: "https://client.example/payment/result",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := resolveCreateOrderReturnURL(tc.request, tc.selection)
			if tc.expectFail {
				require.Error(t, err)
				require.Empty(t, actual)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
			require.False(t, strings.Contains(actual, "#fragment"))
		})
	}
}
