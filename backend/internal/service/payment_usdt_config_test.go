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
			if key == payment.TypeUSDTTRC20 {
				require.Empty(t, merged[secretKey])
			} else {
				require.Equal(t, "secret-value", merged[secretKey])
			}
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

func TestTRC20PublicConfigurationAndExplicitKeyClearing(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	wallet := "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7"
	instance, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		Name: "公共 TRC20", ProviderKey: payment.TypeUSDTTRC20,
		Config:         map[string]string{"walletAddress": wallet},
		SupportedTypes: []string{payment.TypeUSDTTRC20}, Enabled: true,
	})
	require.NoError(t, err, "只填写钱包地址应能创建已启用的公共查询实例")

	_, err = svc.UpdateProviderInstance(ctx, instance.ID, UpdateProviderInstanceRequest{Config: map[string]string{"apiKey": "optional-key"}})
	require.NoError(t, err)
	updated, err := svc.UpdateProviderInstance(ctx, instance.ID, UpdateProviderInstanceRequest{Config: map[string]string{"walletAddress": wallet}})
	require.NoError(t, err)
	stored, err := svc.decryptConfig(updated.Config)
	require.NoError(t, err)
	require.Equal(t, "optional-key", stored["apiKey"], "编辑时省略密钥字段必须保留已有凭据")
	loadBalancer := payment.NewDefaultLoadBalancer(client, nil)
	queryConfig, err := loadBalancer.GetInstanceConfig(ctx, instance.ID)
	require.NoError(t, err)
	require.Equal(t, "optional-key", queryConfig["apiKey"])

	// 查询凭据变更不改变收款身份，已有待支付订单仍可继续确认。
	createPendingProviderConfigOrder(t, ctx, client, instance)
	updated, err = svc.UpdateProviderInstance(ctx, instance.ID, UpdateProviderInstanceRequest{Config: map[string]string{"apiKey": ""}})
	require.NoError(t, err)
	require.True(t, updated.Enabled)
	stored, err = svc.decryptConfig(updated.Config)
	require.NoError(t, err)
	require.NotContains(t, stored, "apiKey")
	require.Equal(t, wallet, stored["walletAddress"])
	queryConfig, err = loadBalancer.GetInstanceConfig(ctx, instance.ID)
	require.NoError(t, err)
	require.NotContains(t, queryConfig, "apiKey", "已有查账服务必须重新读取已清除密钥的当前配置")
	order, err := client.PaymentOrder.Query().Only(ctx)
	require.NoError(t, err)
	order, err = order.Update().SetPaymentType(payment.TypeUSDTTRC20).Save(ctx)
	require.NoError(t, err)
	paymentSvc := &PaymentService{entClient: client, loadBalancer: loadBalancer}
	orderProvider, err := paymentSvc.getOrderProvider(ctx, order)
	require.NoError(t, err, "已有订单必须能够在清除密钥后重新创建公共查询服务商")
	require.Equal(t, payment.TypeUSDTTRC20, orderProvider.ProviderKey())

	responses, err := svc.ListProviderInstancesWithConfig(ctx)
	require.NoError(t, err)
	require.Len(t, responses, 1)
	require.NotContains(t, responses[0].Config, "apiKey")
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
