//go:build unit

package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type okpayDiagnosticTransport struct {
	handle func(*http.Request) (*http.Response, error)
}

func (transport okpayDiagnosticTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport.handle(request)
}

func TestOKPayDiagnosticUsesOneSavedConfigurationWithoutCreatingOrders(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	configService := &PaymentConfigService{entClient: client}
	instance, err := configService.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		Name: "认证对照", ProviderKey: payment.TypeOKPay, SupportedTypes: []string{payment.TypeOKPay},
		Config: map[string]string{"id": "merchant-private", "token": "token-private", "apiBase": "https://diagnostic.example/shop"},
	})
	require.NoError(t, err)
	previousTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	var signatures []string
	modes := []string{"current", "php_reference", "hmac_sha256"}
	http.DefaultTransport = okpayDiagnosticTransport{handle: func(request *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, "diagnostic.example", request.URL.Host)
		require.Equal(t, "/shop/balance", request.URL.Path, "诊断只能查询，不能调用下单或转账接口")
		require.NoError(t, request.ParseForm())
		require.Equal(t, "merchant-private", request.PostForm.Get("id"))
		require.NotContains(t, request.PostForm, "token")
		require.Less(t, len(signatures), len(modes), "每个诊断模式只能请求一次")
		mode := modes[len(signatures)]
		signatures = append(signatures, request.PostForm.Get("sign"))
		if mode == "php_reference" {
			require.Len(t, request.PostForm, 2)
			require.Len(t, signatures[len(signatures)-1], 32)
			sum := md5.Sum([]byte("id=merchant-private&token=token-private"))
			require.Equal(t, strings.ToUpper(hex.EncodeToString(sum[:])), request.PostForm.Get("sign"))
		} else {
			require.Len(t, request.PostForm, 4)
			require.Len(t, signatures[len(signatures)-1], 64)
			require.Regexp(t, `^[0-9a-f]{32}$`, request.PostForm.Get("nonce"))
			timestamp, parseErr := strconv.ParseInt(request.PostForm.Get("timestamp"), 10, 64)
			require.NoError(t, parseErr)
			require.WithinDuration(t, time.Now(), time.Unix(timestamp, 0), 5*time.Second)
			fields := make(map[string]string)
			for key := range request.PostForm {
				fields[key] = request.PostForm.Get(key)
			}
			require.Equal(t, okpayIntegrationHMACSignature(fields, "token-private"), request.PostForm.Get("sign"),
				"后续保存的新密钥不得改变本轮任意诊断模式的签名")
		}
		if len(signatures) == 1 {
			// 并发保存不应让第二、三次诊断改用另一组凭据，制造错误对照结果。
			_, updateErr := client.PaymentProviderInstance.UpdateOneID(instance.ID).
				SetConfig(`{"id":"changed-merchant","token":"changed-token","apiBase":"https://diagnostic.example/shop"}`).Save(ctx)
			require.NoError(t, updateErr)
		}
		body := `{"status":"success","code":200,"data":{"usdt":"987654.32","trx":"0.00","cny":"0.00"}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	}}
	result, err := configService.DiagnoseOKPayProvider(ctx, instance.ID)
	require.NoError(t, err)
	require.Equal(t, instance.ID, result.ProviderInstanceID)
	require.Equal(t, instance.Name, result.ProviderName)
	require.Equal(t, "both_authenticated", result.Conclusion)
	require.Len(t, result.Checks, 3)
	require.Len(t, signatures, 3)
	for index, check := range result.Checks {
		require.Equal(t, modes[index], check.Mode)
		require.Equal(t, "authenticated", check.Status)
	}
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	for _, privateValue := range append([]string{"merchant-private", "token-private", "changed-token", "987654.32"}, signatures...) {
		require.NotContains(t, string(raw), privateValue)
	}
	orders, err := client.PaymentOrder.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, orders)
}

func TestOKPayDiagnosticRejectsMissingWrongOrIncompleteProvider(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	configService := &PaymentConfigService{entClient: client}
	_, err := configService.DiagnoseOKPayProvider(ctx, 0)
	require.Equal(t, "INVALID_PROVIDER_ID", infraerrors.Reason(err))
	_, err = configService.DiagnoseOKPayProvider(ctx, 999)
	require.Equal(t, "PROVIDER_NOT_FOUND", infraerrors.Reason(err))
	for _, key := range []string{payment.TypeUSDTTRC20, payment.TypeOKPay} {
		instance, createErr := configService.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
			Name: key, ProviderKey: key, SupportedTypes: []string{key}, Config: map[string]string{},
		})
		require.NoError(t, createErr)
		_, err = configService.DiagnoseOKPayProvider(ctx, instance.ID)
		expected := "INVALID_PROVIDER_TYPE"
		if key == payment.TypeOKPay {
			expected = "INVALID_PROVIDER_CONFIG"
		}
		require.Equal(t, expected, infraerrors.Reason(err))
	}
}
