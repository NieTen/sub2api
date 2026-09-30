package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const okpayDiagnosticBalanceResponse = `{"status":"success","code":10000,"data":{"usdt":"9845631.73","trx":0,"cny":"0.00"}}`
const okpayDiagnosticRejectedResponse = `{"status":"warning","msg":"身份认证失败"}`

type okpayDiagnosticTestTransport struct {
	calls int
}

func (r *okpayDiagnosticTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	r.calls++
	return nil, fmt.Errorf("网络错误包含私密内容 token=%s sign=private-sign balance=9845631.73", okpayTestToken)
}

func TestOKPayAuthenticationDiagnosticOnlyUsesReadOnlyBalanceRequests(t *testing.T) {
	var modes, signatures, amounts []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/shop/balance", request.URL.Path)
		require.Equal(t, http.MethodPost, request.Method)
		require.NoError(t, request.ParseForm())
		if request.PostForm.Has("timestamp") {
			require.Len(t, request.PostForm, 4)
		} else {
			require.Len(t, request.PostForm, 2)
		}
		require.Equal(t, "123", request.PostForm.Get("id"))
		require.NotContains(t, request.PostForm, "token")
		if request.UserAgent() == "HTTP CLIENT" {
			modes = append(modes, okpayTransportPHPReference)
			require.Equal(t, "*/*", request.Header.Get("Accept"))
			require.Empty(t, request.Header.Get("Accept-Encoding"))
		} else {
			mode := okpayTransportCurrent
			if request.PostForm.Has("timestamp") {
				mode = okpayTransportHMAC
			}
			modes = append(modes, mode)
			require.Equal(t, "application/json", request.Header.Get("Accept"))
			require.Equal(t, "gzip", request.Header.Get("Accept-Encoding"))
		}
		signatures = append(signatures, request.PostForm.Get("sign"))
		amounts = append(amounts, request.PostForm.Get("amount"))
		fmt.Fprint(w, okpayDiagnosticBalanceResponse)
	}))
	defer server.Close()
	provider := newOKPayForTest(t)
	provider.config["apiBase"] = server.URL + "/shop"
	provider.httpClient.Transport = server.Client().Transport
	result := provider.DiagnoseAuthentication(context.Background())
	require.Equal(t, []string{okpayTransportCurrent, okpayTransportPHPReference, okpayTransportHMAC}, modes)
	require.Equal(t, []string{"", "", ""}, amounts)
	require.Equal(t, signatures[0], signatures[1])
	require.NotEmpty(t, signatures[0])
	require.Equal(t, "both_authenticated", result.Conclusion)
	require.Len(t, result.Checks, 3)
	for _, check := range result.Checks {
		require.Equal(t, okpayCheckAuthenticated, check.Status)
	}
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	for _, secret := range []string{okpayTestToken, signatures[0], "9845631.73", "123", "usdt", "trx", "cny"} {
		require.NotContains(t, string(raw), secret)
	}
}

func TestOKPayAuthenticationDiagnosticClassifiesWithoutGuessingCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, current, reference, currentStatus, referenceStatus, conclusion string
		currentHTTP                                                          int
	}{
		{name: "两个请求都通过", current: okpayDiagnosticBalanceResponse, reference: okpayDiagnosticBalanceResponse, currentStatus: okpayCheckAuthenticated, referenceStatus: okpayCheckAuthenticated, conclusion: "both_authenticated"},
		{name: "两个请求都拒绝", current: okpayDiagnosticRejectedResponse, reference: okpayDiagnosticRejectedResponse, currentStatus: okpayCheckRejected, referenceStatus: okpayCheckRejected, conclusion: "both_rejected"},
		{name: "仅PHP传输通过", current: okpayDiagnosticRejectedResponse, reference: okpayDiagnosticBalanceResponse, currentStatus: okpayCheckRejected, referenceStatus: okpayCheckAuthenticated, conclusion: "inconclusive"},
		{name: "仅当前传输通过", current: okpayDiagnosticBalanceResponse, reference: okpayDiagnosticRejectedResponse, currentStatus: okpayCheckAuthenticated, referenceStatus: okpayCheckRejected, conclusion: "inconclusive"},
		{name: "HTTP异常不能推断凭据", currentHTTP: http.StatusServiceUnavailable, current: okpayDiagnosticBalanceResponse, reference: okpayDiagnosticBalanceResponse, currentStatus: okpayCheckRequestFailed, referenceStatus: okpayCheckAuthenticated, conclusion: "inconclusive"},
		{name: "缺少成功标记", current: `{"data":{"usdt":"1"}}`, reference: okpayDiagnosticBalanceResponse, currentStatus: okpayCheckInvalidResponse, referenceStatus: okpayCheckAuthenticated, conclusion: "inconclusive"},
		{name: "缺少余额数据", current: `{"status":"success","data":{}}`, reference: okpayDiagnosticBalanceResponse, currentStatus: okpayCheckInvalidResponse, referenceStatus: okpayCheckAuthenticated, conclusion: "inconclusive"},
		{name: "错误形状不能当余额", current: `{"status":"success","data":{"error":"private-token"}}`, reference: okpayDiagnosticBalanceResponse, currentStatus: okpayCheckInvalidResponse, referenceStatus: okpayCheckAuthenticated, conclusion: "inconclusive"},
		{name: "无效JSON", current: `{"status":`, reference: okpayDiagnosticRejectedResponse, currentStatus: okpayCheckInvalidResponse, referenceStatus: okpayCheckRejected, conclusion: "inconclusive"},
		{name: "余额类型无效", current: `{"status":"success","data":{"usdt":true}}`, reference: okpayDiagnosticBalanceResponse, currentStatus: okpayCheckInvalidResponse, referenceStatus: okpayCheckAuthenticated, conclusion: "inconclusive"},
		{name: "布尔状态不能视为认证拒绝", current: `{"status":true,"code":10000,"data":{"usdt":"1"}}`, reference: okpayDiagnosticBalanceResponse, currentStatus: okpayCheckInvalidResponse, referenceStatus: okpayCheckAuthenticated, conclusion: "inconclusive"},
		{name: "对象代码不能视为认证拒绝", current: `{"status":"success","code":{},"data":{"usdt":"1"}}`, reference: okpayDiagnosticBalanceResponse, currentStatus: okpayCheckInvalidResponse, referenceStatus: okpayCheckAuthenticated, conclusion: "inconclusive"},
		{name: "空状态不能视为认证拒绝", current: `{"status":"","code":10000,"data":{"usdt":"1"}}`, reference: okpayDiagnosticBalanceResponse, currentStatus: okpayCheckInvalidResponse, referenceStatus: okpayCheckAuthenticated, conclusion: "inconclusive"},
		{name: "空代码不能视为认证拒绝", current: `{"status":"success","code":null,"data":{"usdt":"1"}}`, reference: okpayDiagnosticBalanceResponse, currentStatus: okpayCheckInvalidResponse, referenceStatus: okpayCheckAuthenticated, conclusion: "inconclusive"},
		{name: "成功代码不能覆盖失败状态", current: `{"status":"warning","code":10000,"data":{"usdt":"1"}}`, reference: okpayDiagnosticBalanceResponse, currentStatus: okpayCheckRejected, referenceStatus: okpayCheckAuthenticated, conclusion: "inconclusive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls++
				if request.UserAgent() == "HTTP CLIENT" {
					fmt.Fprint(w, tc.reference)
					return
				}
				if tc.currentHTTP != 0 {
					w.WriteHeader(tc.currentHTTP)
				}
				fmt.Fprint(w, tc.current)
			}))
			defer server.Close()
			provider := newOKPayForTest(t)
			provider.config["apiBase"] = server.URL
			provider.httpClient.Transport = server.Client().Transport
			result := provider.DiagnoseAuthentication(context.Background())
			require.Equal(t, tc.conclusion, result.Conclusion)
			require.Equal(t, tc.currentStatus, result.Checks[0].Status)
			require.Equal(t, tc.referenceStatus, result.Checks[1].Status)
			require.Equal(t, 3, calls, "每种协议只能查询一次，不能自动重试")
		})
	}
}

func TestOKPayAuthenticationDiagnosticDoesNotExposeNetworkErrorsOrFollowRedirects(t *testing.T) {
	provider := newOKPayForTest(t)
	transport := &okpayDiagnosticTestTransport{}
	provider.httpClient.Transport = transport
	result := provider.DiagnoseAuthentication(context.Background())
	require.Equal(t, "inconclusive", result.Conclusion)
	require.Equal(t, 3, transport.calls)
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	for _, secret := range []string{okpayTestToken, "private-sign", "9845631.73", "网络错误包含私密内容"} {
		require.NotContains(t, string(raw), secret)
	}
	var destinationCalls, sourceCalls int
	destination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) { destinationCalls++ }))
	defer destination.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		sourceCalls++
		http.Redirect(w, request, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	provider.config["apiBase"] = server.URL
	provider.httpClient.Transport = server.Client().Transport
	result = provider.DiagnoseAuthentication(context.Background())
	require.Equal(t, "inconclusive", result.Conclusion)
	require.Equal(t, 3, sourceCalls)
	require.Zero(t, destinationCalls)
	for _, check := range result.Checks {
		require.Equal(t, okpayCheckRequestFailed, check.Status)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = provider.DiagnoseAuthentication(ctx)
	require.Equal(t, "inconclusive", result.Conclusion)
	require.Equal(t, 3, sourceCalls)
	for _, check := range result.Checks {
		require.Equal(t, okpayCheckRequestFailed, check.Status)
		require.False(t, strings.Contains(check.Message, "Token错误"))
	}
}
