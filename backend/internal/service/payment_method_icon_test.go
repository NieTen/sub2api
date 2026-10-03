//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestPaymentMethodIconURLValidation(t *testing.T) {
	for _, raw := range []string{"https://images.example/okpay.png", "/images/okpay.svg", " https://images.example/icon.png?v=2 "} {
		require.Equal(t, strings.TrimSpace(raw), normalizePaymentMethodIconURL(raw))
	}
	for _, raw := range []string{"", "http://images.example/icon.png", "//images.example/icon.png", "javascript:alert(1)", "data:image/svg+xml,test", "file:///icon.png", "https://user:pass@images.example/icon.png", "https://@images.example/icon.png", "/\\images.example/icon.png", "/images/%ZZ.png", "https://images.example/icon.png#x", "https://images.example/\nicon.png", strings.Repeat("x", 2049)} {
		require.Empty(t, normalizePaymentMethodIconURL(raw), raw)
	}
}

func TestPaymentMethodIconSelectionIsStableAndIgnoresInvalidInstances(t *testing.T) {
	svc := &PaymentConfigService{}
	instances := []*dbent.PaymentProviderInstance{
		{ID: 8, SortOrder: 5, Enabled: true, ProviderKey: payment.TypeOKPay, Config: `{"iconUrl":"/later.png"}`},
		{ID: 3, SortOrder: 1, Enabled: true, ProviderKey: payment.TypeOKPay, Config: `{"iconUrl":"/chosen.png"}`},
		{ID: 4, SortOrder: 1, Enabled: true, ProviderKey: payment.TypeOKPay, Config: `{"iconUrl":"/same-sort-later.png"}`},
		{ID: 1, SortOrder: 0, Enabled: false, ProviderKey: payment.TypeOKPay, Config: `{"iconUrl":"/disabled.png"}`},
		{ID: 2, SortOrder: 0, Enabled: true, ProviderKey: payment.TypeEasyPay, Config: `{"iconUrl":"/other.png"}`},
		{ID: 5, SortOrder: 0, Enabled: true, ProviderKey: payment.TypeOKPay, Config: `{"iconUrl":"javascript:alert(1)"}`},
		{ID: 6, SortOrder: 0, Enabled: true, ProviderKey: payment.TypeOKPay, Config: `{"iconUrl":""}`},
	}
	require.Equal(t, "/chosen.png", svc.pcAggregateMethodIconURL(payment.TypeOKPay, instances))
	for left, right := 0, len(instances)-1; left < right; left, right = left+1, right-1 {
		instances[left], instances[right] = instances[right], instances[left]
	}
	require.Equal(t, "/chosen.png", svc.pcAggregateMethodIconURL(payment.TypeOKPay, instances))
	require.Empty(t, svc.pcAggregateMethodIconURL(payment.TypeAlipay, instances))
}

func TestPaymentMethodIconPublicResponseOnlyExposesIconURL(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().SetProviderKey(payment.TypeOKPay).SetName("图标测试").
		SetSupportedTypes(payment.TypeOKPay).SetEnabled(true).
		SetConfig(`{"id":"private-merchant","token":"private-token","apiBase":"https://private-gateway.example/shop","iconUrl":"https://images.example/okpay.png"}`).Save(ctx)
	require.NoError(t, err)
	svc := &PaymentConfigService{entClient: client}
	response, err := svc.GetAvailableMethodLimits(ctx)
	require.NoError(t, err)
	require.Equal(t, "https://images.example/okpay.png", response.Methods[payment.TypeOKPay].IconURL)
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	for _, secret := range []string{"private-merchant", "private-token", "private-gateway", `"config"`} {
		require.NotContains(t, string(encoded), secret)
	}
	limits, err := svc.GetMethodLimits(ctx, []string{payment.TypeOKPay})
	require.NoError(t, err)
	require.Len(t, limits, 1)
	require.Equal(t, response.Methods[payment.TypeOKPay].IconURL, limits[0].IconURL)
}

func TestPaymentMethodIconCanBeSavedClearedAndValidated(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	instance, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey: payment.TypeOKPay, Name: "图标设置", SupportedTypes: []string{payment.TypeOKPay},
		Config: map[string]string{"id": "merchant", "token": "saved-secret", "iconUrl": "https://images.example/okpay.png"},
	})
	require.NoError(t, err)
	_, err = svc.UpdateProviderInstance(ctx, instance.ID, UpdateProviderInstanceRequest{Config: map[string]string{"iconUrl": "/images/new.png", "token": ""}})
	require.NoError(t, err)
	saved, err := client.PaymentProviderInstance.Get(ctx, instance.ID)
	require.NoError(t, err)
	config, err := svc.decryptConfig(saved.Config)
	require.NoError(t, err)
	require.Equal(t, "/images/new.png", config["iconUrl"])
	require.Equal(t, "saved-secret", config["token"])
	_, err = svc.UpdateProviderInstance(ctx, instance.ID, UpdateProviderInstanceRequest{Config: map[string]string{"iconUrl": ""}})
	require.NoError(t, err)
	saved, err = client.PaymentProviderInstance.Get(ctx, instance.ID)
	require.NoError(t, err)
	config, err = svc.decryptConfig(saved.Config)
	require.NoError(t, err)
	require.Empty(t, config["iconUrl"])
	require.Equal(t, "saved-secret", config["token"])
	_, err = svc.UpdateProviderInstance(ctx, instance.ID, UpdateProviderInstanceRequest{Config: map[string]string{"iconUrl": "javascript:alert(1)"}})
	require.Error(t, err)
	_, err = svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{ProviderKey: payment.TypeOKPay, Name: "无效图标", Config: map[string]string{"iconUrl": "//images.example/icon.png"}})
	require.Error(t, err)
}
