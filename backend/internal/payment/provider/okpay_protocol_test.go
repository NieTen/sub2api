package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestOKPayNewProtocolCreatesQueriesAndDiagnosesWithoutDowngrade(t *testing.T) {
	var paths, nonces []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		require.Equal(t, "123", r.PostForm.Get("id"))
		require.NotContains(t, r.PostForm, "token")
		if len(r.PostForm.Get("sign")) == 32 {
			fmt.Fprint(w, `{"status":"warning","msg":"身份认证失败"}`)
			return
		}
		nonce := r.PostForm.Get("nonce")
		require.Len(t, nonce, 32)
		_, err := hex.DecodeString(nonce)
		require.NoError(t, err)
		require.NotContains(t, nonces, nonce)
		nonces = append(nonces, nonce)
		timestamp, err := strconv.ParseInt(r.PostForm.Get("timestamp"), 10, 64)
		require.NoError(t, err)
		require.WithinDuration(t, time.Now(), time.Unix(timestamp, 0), 5*time.Second)
		// 独立按接收到的字段验签，确保签名原文和实际发送值相同。
		var pairs []string
		for key, values := range r.PostForm {
			require.Len(t, values, 1)
			if key != "sign" {
				pairs = append(pairs, key+"="+values[0])
			}
		}
		sort.Strings(pairs)
		mac := hmac.New(sha256.New, []byte(okpayTestToken))
		_, _ = mac.Write([]byte(strings.Join(pairs, "&")))
		require.Equal(t, strings.ToUpper(hex.EncodeToString(mac.Sum(nil))), r.PostForm.Get("sign"))
		switch r.URL.Path {
		case "/shop/payLink":
			require.Equal(t, "0", r.PostForm.Get("status"))
			require.Equal(t, "12.30", r.PostForm.Get("amount"))
			require.Equal(t, "充值 A+B & %2B", r.PostForm.Get("name"))
			fmt.Fprint(w, `{"status":"success","code":200,"data":{"order_id":"pay-900","pay_url":"https://cashier.example/pay"}}`)
		case "/shop/checkDeposit":
			require.Equal(t, "site-100", r.PostForm.Get("unique_id"))
			fmt.Fprint(w, `{"status":"success","code":200,"data":{"order_id":"pay-900","unique_id":"site-100","amount":"12.30","coin":"USDT","status":1}}`)
		case "/shop/balance":
			fmt.Fprint(w, `{"status":"success","code":200,"data":{"usdt":"4.20","trx":0}}`)
		default:
			t.Errorf("不应调用其他接口: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := NewOKPay("1", map[string]string{"id": "123", "token": okpayTestToken, "apiBase": server.URL + "/shop"})
	require.NoError(t, err)
	require.Equal(t, OKPaySignatureHMACSHA256, client.config["signatureAlgorithm"])
	client.httpClient.Transport = server.Client().Transport
	created, err := client.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "site-100", Amount: "12.30", Subject: "充值 A+B & %2B"})
	require.NoError(t, err)
	require.Equal(t, "pay-900", created.TradeNo)
	queried, err := client.QueryOrderByMerchantOrderID(context.Background(), "site-100")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, queried.Status)
	diagnostic := client.DiagnoseAuthentication(context.Background())
	require.Equal(t, "hmac_only_authenticated", diagnostic.Conclusion)
	require.Equal(t, "auth_failed", diagnostic.Checks[1].Reason)
	require.Equal(t, http.StatusOK, diagnostic.Checks[1].HTTPStatus)
	require.Equal(t, []string{"/shop/payLink", "/shop/checkDeposit", "/shop/balance", "/shop/balance", "/shop/balance"}, paths)
}

func TestOKPayNewProtocolVerifiesPublishedCallbackAndRejectsLegacyOrTampering(t *testing.T) {
	client, err := NewOKPay("1", map[string]string{"id": "10001", "token": "TESTtoken123456789abcdefghijABCD"})
	require.NoError(t, err)
	raw := `{"status":"success","code":200,"data":{"order_id":"abc123def456","unique_id":"ORDER-20260628-001","pay_user_id":123456789,"amount":"100.5","coin":"USDT","status":1,"type":"deposit"},"id":10001,"sign":"64B09C8847849FA6921D8FFBDF8E406D4A8EA623E53970712350F61783403F7D"}`
	notification, err := client.VerifyNotification(context.Background(), raw, nil)
	require.NoError(t, err)
	require.Equal(t, 100.5, notification.Amount)
	require.Equal(t, "ORDER-20260628-001", notification.OrderID)
	require.Equal(t, payment.NotificationStatusSuccess, notification.Status)
	_, err = client.VerifyNotification(context.Background(), strings.Replace(raw, "64B09C8847849FA6921D8FFBDF8E406D4A8EA623E53970712350F61783403F7D", "64b09c8847849fa6921d8ffbdf8e406d4a8ea623e53970712350f61783403f7d", 1), nil)
	require.NoError(t, err)
	for _, body := range []string{
		strings.Replace(raw, `"100.5"`, `"999.5"`, 1),
		strings.Replace(raw, `"USDT"`, `"TRX"`, 1),
		strings.Replace(raw, `"id":10001`, `"id":10002`, 1),
		signedOKPayTestJSON(t, okpayTestDeposit),
		"id=10001&sign=" + strings.Repeat("A", 64),
	} {
		_, err := client.VerifyNotification(context.Background(), body, nil)
		require.Error(t, err)
	}
	_, err = NewOKPay("1", map[string]string{"id": "123", "token": "fake", "signatureAlgorithm": "auto"})
	require.Error(t, err)
}

func TestOKPayDiagnosticBusinessReasonIsSafeAndSpecific(t *testing.T) {
	for message, expected := range map[string]string{
		"身份认证失败": "auth_failed", "签名错误": "signature_failed", "参数错误": "invalid_parameters",
		"请求过于频繁": "rate_limited", "商户不存在": "merchant_invalid", "私密信息和余额": "unknown_business_error",
	} {
		require.Equal(t, expected, okpayBusinessReason(okpayArray{{key: "msg", value: message}}, nil))
	}
	require.Equal(t, "unknown_business_error", okpayBusinessReason(okpayArray{{key: "msg", value: "身份认证失败"}}, []string{"身份认证失败"}))
	require.Empty(t, okpaySafeBusinessCode(okpayArray{{key: "code", value: "123"}}, []string{"fake123token"}))
}

func TestOKPayDiagnosticConflictingSameProtocolCannotIdentifyAlgorithm(t *testing.T) {
	for _, algorithm := range []string{OKPaySignatureHMACSHA256, OKPaySignatureLegacyMD5} {
		t.Run(algorithm, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				// 当前请求通过，但使用相同算法的独立探测被拒绝，不能作出协议归因。
				rejected := (algorithm == OKPaySignatureHMACSHA256 && calls == 3) || (algorithm == OKPaySignatureLegacyMD5 && calls == 2)
				if rejected {
					fmt.Fprint(w, okpayDiagnosticRejectedResponse)
				} else {
					fmt.Fprint(w, okpayDiagnosticBalanceResponse)
				}
			}))
			defer server.Close()
			client, err := NewOKPay("1", map[string]string{"id": "123", "token": "fake-token", "apiBase": server.URL, "signatureAlgorithm": algorithm})
			require.NoError(t, err)
			client.httpClient.Transport = server.Client().Transport
			require.Equal(t, "inconclusive", client.DiagnoseAuthentication(context.Background()).Conclusion)
			require.Equal(t, 3, calls)
		})
	}
}
