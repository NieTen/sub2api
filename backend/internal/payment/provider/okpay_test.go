package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

const okpayTestToken = "token+plus%2Band%zz"
const okpayTestDeposit = `{"id":"123","status":"success","code":10000,"data":{"order_id":"pay-900","unique_id":"site-100","pay_user_id":5639813059,"amount":"12.30","coin":"USDT","status":1,"type":"deposit"}}`

func newOKPayForTest(t *testing.T) *OKPay {
	t.Helper()
	provider, err := NewOKPay("1", map[string]string{"id": "123", "token": okpayTestToken, "signatureAlgorithm": OKPaySignatureLegacyMD5})
	require.NoError(t, err)
	return provider
}

func signedOKPayTestJSON(t *testing.T, raw string) string {
	t.Helper()
	fields, err := okpayDecodeJSON(raw)
	require.NoError(t, err)
	signature, err := okpaySign(fields, okpayTestToken)
	require.NoError(t, err)
	return strings.TrimSuffix(raw, "}") + `,"sign":"` + signature + `"}`
}

func TestOKPaySignatureMatchesIndependentPHPFixtures(t *testing.T) {
	// 以下结果由 PHP 7.4 原生 array_filter/ksort/http_build_query/urldecode/md5 生成，非 Go 实现自证。
	cases := []struct{ name, raw, expected string }{
		{"充值回调", okpayTestDeposit, "5C3DFCBF609D8BD0E66311EC73726275"},
		{"顶层真假值与嵌套索引", `{"id":"123","zero":"0","numberZero":0,"false":false,"nil":null,"empty":"","array":[],"decimalZero":"0.0","data":{"values":[0,false,null,"0","",1],"items":{"2":"first","0":"second","3":{"flag":false,"nil":null,"value":"0"}},"text":"中文 & + = %20","float":1.0e-7}}`, "71CAA751FE81154553839565FEDB5E8F"},
		{"PHP浮点文本转换", `{"id":"123","data":{"amount":12.30,"status":1,"float":123456789012345.0,"zero":-0.0}}`, "3BF932C3C4B85C3605D451B592A95D9E"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields, err := okpayDecodeJSON(tc.raw)
			require.NoError(t, err)
			signature, err := okpaySign(fields, okpayTestToken)
			require.NoError(t, err)
			require.Equal(t, tc.expected, signature)
		})
	}
}

func TestOKPayVerifiesJSONAndPHPNestedFormCallbacks(t *testing.T) {
	provider := newOKPayForTest(t)
	jsonBody := strings.TrimSuffix(okpayTestDeposit, "}") + `,"sign":"5C3DFCBF609D8BD0E66311EC73726275"}`
	// 按 PHP 数组插入顺序构造 data，顶层顺序不同不影响 ksort 后的签名。
	form := "id=123&status=success&code=10000&data[order_id]=pay-900&data[unique_id]=site-100&data[pay_user_id]=5639813059&data[amount]=12.30&data[coin]=USDT&data[status]=1&data[type]=deposit&sign=5C3DFCBF609D8BD0E66311EC73726275"
	for _, body := range []string{jsonBody, form} {
		notification, err := provider.VerifyNotification(context.Background(), body, nil)
		require.NoError(t, err)
		require.Equal(t, "pay-900", notification.TradeNo)
		require.Equal(t, "site-100", notification.OrderID)
		require.Equal(t, 12.3, notification.Amount)
		require.Equal(t, payment.NotificationStatusSuccess, notification.Status)
		require.Equal(t, "123", notification.Metadata["merchant_id"])
		require.Equal(t, "USDT", notification.Metadata["currency"])
	}
}

func TestOKPayRejectsForgedOrMismatchedNotifications(t *testing.T) {
	provider := newOKPayForTest(t)
	valid := signedOKPayTestJSON(t, okpayTestDeposit)
	for _, body := range []string{
		strings.Replace(valid, "12.30", "99.00", 1),
		strings.Replace(valid, "site-100", "site-200", 1),
		strings.Replace(valid, "5C3DFCBF", "5c3dfcbf", 1),
		`{"id":"123","id":"123","sign":"x"}`,
		"id=123&id=456&sign=x", "data[status]=1&data=bad&sign=x",
	} {
		_, err := provider.VerifyNotification(context.Background(), body, nil)
		require.Error(t, err)
	}
	for _, replacement := range []struct{ old, next string }{
		{`"id":"123"`, `"id":"456"`}, {`"coin":"USDT"`, `"coin":"TRX"`},
		{`"status":1`, `"status":2`}, {`"status":1`, `"status":true`},
		{`"type":"deposit"`, `"type":"unknown"`}, {`"amount":"12.30"`, `"amount":"0"`},
		{`"amount":"12.30"`, `"amount":"NaN"`}, {`"amount":"12.30"`, `"amount":"-12.30"`},
		{`"coin":"USDT",`, ""}, {`"status":1,`, ""}, {`"unique_id":"site-100",`, ""},
		{`"status":"success"`, `"status":"error"`}, {`"code":10000`, `"code":10001`},
	} {
		body := signedOKPayTestJSON(t, strings.Replace(okpayTestDeposit, replacement.old, replacement.next, 1))
		_, err := provider.VerifyNotification(context.Background(), body, nil)
		require.Error(t, err, replacement.next)
	}
}

func TestOKPayOnlyCreditsPaidDeposits(t *testing.T) {
	provider := newOKPayForTest(t)
	for _, raw := range []string{
		strings.Replace(okpayTestDeposit, `"status":1`, `"status":0`, 1),
		strings.Replace(okpayTestDeposit, `"type":"deposit"`, `"type":"withdraw"`, 1),
	} {
		notification, err := provider.VerifyNotification(context.Background(), signedOKPayTestJSON(t, raw), nil)
		require.NoError(t, err)
		require.Nil(t, notification)
	}
}

func TestOKPayFormPreservesNestedIndexesAndRejectsAmbiguity(t *testing.T) {
	fields, err := okpayDecodeForm("data%5B2%5D=two&data%5B0%5D=zero&data%5B%5D=three&id=123")
	require.NoError(t, err)
	value, _ := fields.get("data")
	data := value.(okpayArray)
	require.Equal(t, []string{"2", "0", "3"}, []string{data[0].key, data[1].key, data[2].key})
	for _, raw := range []string{"data[x]=1&data[x]=2", "data[x]tail=1", "data[x=1", "data=1&data[x]=2", "data[x][y]=1&data[x]=2"} {
		_, err := okpayDecodeForm(raw)
		require.Error(t, err)
	}
}

func TestOKPayCreatesSignedFormCheckoutAndQueriesByMerchantOrder(t *testing.T) {
	var paths []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		payload, err := url.ParseQuery(string(body))
		require.NoError(t, err)
		require.Equal(t, "123", payload.Get("id"))
		require.Equal(t, "site-100", payload.Get("unique_id"))
		require.NotContains(t, payload, "token")
		fields, err := okpayDecodeForm(string(body))
		require.NoError(t, err)
		signature, err := okpaySign(fields, okpayTestToken)
		require.NoError(t, err)
		require.Equal(t, signature, payload.Get("sign"))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/shop/payLink" {
			require.Equal(t, "USDT", payload.Get("coin"))
			require.Equal(t, "12.30", payload.Get("amount"))
			require.Equal(t, "账户充值 A+B & = %2B ?", payload.Get("name"))
			require.Equal(t, "https://site.example/api/v1/payment/webhook/okpay", payload.Get("callback_url"))
			require.Equal(t, "https://site.example/payment/result?name=A+B&encoded=%2B", payload.Get("return_url"))
			require.NotContains(t, payload, "status")
			fmt.Fprint(w, `{"status":"success","code":10000,"data":{"order_id":"pay-900","pay_url":"https://t.me/OkayPayBot?start=pay-900"}}`)
		} else {
			require.Equal(t, "/shop/checkDeposit", r.URL.Path)
			fmt.Fprint(w, `{"status":"success","code":10000,"data":{"order_id":"pay-900","unique_id":"site-100","amount":"12.30","status":1}}`)
		}
	}))
	defer server.Close()
	provider, err := NewOKPay("1", map[string]string{"id": "123", "token": okpayTestToken, "apiBase": server.URL, "paymentMode": "redirect", "signatureAlgorithm": OKPaySignatureLegacyMD5})
	require.NoError(t, err)
	provider.httpClient.Transport = server.Client().Transport
	created, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "site-100", Amount: "12.3", Subject: "账户充值 A+B & = %2B ?", NotifyURL: "https://site.example/api/v1/payment/webhook/okpay", ReturnURL: "https://site.example/payment/result?name=A+B&encoded=%2B"})
	require.NoError(t, err)
	require.Equal(t, "pay-900", created.TradeNo)
	require.Equal(t, "USDT", created.Currency)
	require.Contains(t, created.PayURL, "https://t.me/")
	queried, err := provider.QueryOrderByMerchantOrderID(context.Background(), "site-100")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, queried.Status)
	require.Equal(t, 12.3, queried.Amount)
	require.Equal(t, "pay-900", queried.TradeNo)
	require.Equal(t, "USDT", queried.Metadata["currency"])
	_, err = provider.QueryOrder(context.Background(), "pay-900")
	require.ErrorContains(t, err, "商户订单号")
	_, err = provider.Refund(context.Background(), payment.RefundRequest{TradeNo: "pay-900", Amount: "12.30"})
	require.ErrorContains(t, err, "不支持自动退款")
	require.Equal(t, []string{"/shop/payLink", "/shop/checkDeposit"}, paths)
}

func TestOKPayRejectsUnsafeConfigurationAndUpstreamResponses(t *testing.T) {
	for _, apiBase := range []string{"http://api.okaypay.me/shop", "https://user:secret@api.okaypay.me/shop", "https://api.okaypay.me/shop?token=x", "https://api.okaypay.me/shop#x"} {
		_, err := NewOKPay("1", map[string]string{"id": "123", "token": "secret", "apiBase": apiBase})
		require.Error(t, err)
	}
	_, err := NewOKPay("1", map[string]string{"id": "123", "token": "secret", "paymentMode": "qrcode"})
	require.Error(t, err)
	_, err = NewOKPay("1", map[string]string{"id": "123"})
	require.Error(t, err)
	for _, response := range []string{
		`{"data":{"order_id":"pay-900","unique_id":"other","amount":"12.30","status":1}}`,
		`{"data":{"order_id":"pay-900","unique_id":"site-100","amount":"12.30","status":1,"coin":"TRX"}}`,
		`{"data":{"order_id":"pay-900","unique_id":"site-100","amount":"12.30","status":1,"type":"withdraw"}}`,
		`{"data":{"order_id":"pay-900","unique_id":"site-100","amount":"12.30","status":9}}`,
		`{"status":"error","code":10000,"data":{"status":1}}`,
		`{"data":{}}`, strings.Repeat("x", okpayMaxResponseSize+1),
	} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, response) }))
		provider := newOKPayForTest(t)
		provider.config["apiBase"] = server.URL
		provider.httpClient.Transport = server.Client().Transport
		_, err := provider.QueryOrderByMerchantOrderID(context.Background(), "site-100")
		require.Error(t, err)
		server.Close()
	}
}

func TestOKPayDoesNotFollowRedirectsOrLeakToken(t *testing.T) {
	var destinationCalls int
	destination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationCalls++ }))
	defer destination.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	provider := newOKPayForTest(t)
	provider.config["apiBase"] = server.URL
	provider.httpClient.Transport = server.Client().Transport
	_, err := provider.QueryOrderByMerchantOrderID(context.Background(), "site-100")
	require.ErrorContains(t, err, "307")
	require.NotContains(t, err.Error(), okpayTestToken)
	require.Zero(t, destinationCalls)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = provider.QueryOrderByMerchantOrderID(ctx, "site-100")
	require.Error(t, err)
}

func TestOKPayRejectsInvalidAmountAndCheckoutURL(t *testing.T) {
	provider := newOKPayForTest(t)
	for _, amount := range []string{"0", "-1", "NaN", "1e3", "1.001", " 1"} {
		_, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "site-100", Amount: amount})
		require.Error(t, err)
	}
	for _, link := range []string{"javascript:alert(1)", "http://site.example/pay", "https://user:pass@site.example/pay"} {
		require.False(t, okpayHTTPSURL(link))
	}
	// URL 查询值保留真实加号，签名不能多做一次 urldecode。
	fields, err := okpayDecodeForm("id=123&data[name]=" + url.QueryEscape("A+B %2B"))
	require.NoError(t, err)
	var pairs []string
	data, _ := fields.get("data")
	require.NoError(t, okpayPHPQuery(&pairs, "data", data))
	require.Equal(t, []string{"data[name]=A+B %2B"}, pairs)
}
