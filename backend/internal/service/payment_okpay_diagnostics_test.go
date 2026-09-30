//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

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
	http.DefaultTransport = okpayDiagnosticTransport{handle: func(request *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, "diagnostic.example", request.URL.Host)
		require.Equal(t, "/shop/balance", request.URL.Path, "诊断只能查询，不能调用下单或转账接口")
		require.NoError(t, request.ParseForm())
		require.Len(t, request.PostForm, 2)
		require.Equal(t, "merchant-private", request.PostForm.Get("id"))
		signatures = append(signatures, request.PostForm.Get("sign"))
		require.Len(t, signatures[len(signatures)-1], 32)
		if len(signatures) == 1 {
			// 并发保存不应让第二次诊断改用另一组凭据，制造错误对照结果。
			_, updateErr := client.PaymentProviderInstance.UpdateOneID(instance.ID).
				SetConfig(`{"id":"changed-merchant","token":"changed-token","apiBase":"https://diagnostic.example/shop"}`).Save(ctx)
			require.NoError(t, updateErr)
		}
		body := `{"status":"success","code":10000,"data":{"usdt":"987654.32","trx":"0.00","cny":"0.00"}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	}}
	result, err := configService.DiagnoseOKPayProvider(ctx, instance.ID)
	require.NoError(t, err)
	require.Equal(t, instance.ID, result.ProviderInstanceID)
	require.Equal(t, instance.Name, result.ProviderName)
	require.Equal(t, "both_authenticated", result.Conclusion)
	require.Len(t, result.Checks, 2)
	require.Len(t, signatures, 2)
	require.Equal(t, signatures[0], signatures[1], "两次只读认证必须使用同一份凭据")
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	for _, privateValue := range []string{"merchant-private", "token-private", "changed-token", "987654.32", signatures[0]} {
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
