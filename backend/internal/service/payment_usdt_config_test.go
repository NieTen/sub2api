//go:build unit

package service

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestUSDTProviderConfiguration(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client, encryptionKey: []byte("0123456789abcdef0123456789abcdef")}
	for _, key := range []string{payment.TypeOKPay, payment.TypeUSDTTRC20} {
		t.Run(key, func(t *testing.T) {
			secretKey := "token"
			if key == payment.TypeUSDTTRC20 {
				secretKey = "apiKey"
			}
			inst, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
				Name: key, ProviderKey: key, SupportedTypes: []string{key},
				Config:        map[string]string{secretKey: "secret-value", "id": "123"},
				RefundEnabled: true, AllowUserRefund: true,
			})
			require.NoError(t, err)
			require.False(t, inst.RefundEnabled)
			require.False(t, inst.AllowUserRefund)
			masked, err := svc.decryptAndMaskConfig(key, inst.Config)
			require.NoError(t, err)
			require.NotEqual(t, "secret-value", masked[secretKey])
			merged, err := svc.mergeConfig(ctx, inst.ID, map[string]string{secretKey: ""})
			require.NoError(t, err)
			require.Equal(t, "secret-value", merged[secretKey])
			enabled := true
			updated, err := svc.UpdateProviderInstance(ctx, inst.ID, UpdateProviderInstanceRequest{RefundEnabled: &enabled, AllowUserRefund: &enabled})
			require.NoError(t, err)
			require.False(t, updated.RefundEnabled)
			require.False(t, updated.AllowUserRefund)
			require.Error(t, validateDedicatedUSDTTypes(key, "alipay"))
			require.NoError(t, validateDedicatedUSDTTypes(key, key))
		})
	}
}

func TestUSDTCurrencyAndBalanceUseConfiguredMultiplier(t *testing.T) {
	for _, key := range []string{payment.TypeOKPay, payment.TypeUSDTTRC20} {
		currency := paymentProviderConfigCurrency(key, map[string]string{"currency": "CNY"})
		require.Equal(t, "USDT", currency)
		require.Equal(t, 1.4, calculateCreditedBalance(10, balanceRechargeMultiplierForCurrency(currency, 0.14)))
		_, amount, err := calculateCreateOrderPayAmountForOrderType(10, 0, currency, payment.OrderTypeSubscription, 7)
		require.NoError(t, err)
		require.Equal(t, 10.0, amount)
	}
	require.Equal(t, 1.4, calculateCreditedBalance(10, balanceRechargeMultiplierForCurrency("CNY", 0.14)))
	require.True(t, hasPendingOrderProtectedConfigChange(payment.TypeOKPay, map[string]string{"id": "a"}, map[string]string{"id": "b"}))
	require.True(t, hasPendingOrderProtectedConfigChange(payment.TypeUSDTTRC20, map[string]string{"walletAddress": "a"}, map[string]string{"walletAddress": "b"}))
}

func TestUSDTRefundCapabilityPreservesEasyPayCustomMethod(t *testing.T) {
	for _, key := range []string{payment.TypeOKPay, payment.TypeUSDTTRC20} {
		order := &dbent.PaymentOrder{PaymentType: key, ProviderSnapshot: map[string]any{"provider_key": key}}
		require.NotNil(t, PaymentOrderRefundSupported(order))
		require.False(t, *PaymentOrderRefundSupported(order))
	}
	require.Nil(t, PaymentOrderRefundSupported(&dbent.PaymentOrder{
		PaymentType: payment.TypeUSDTTRC20, ProviderSnapshot: map[string]any{"provider_key": payment.TypeEasyPay},
	}))
}
