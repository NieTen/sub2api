package admin

import (
	"encoding/json"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestUSDTQuoteAdminDTOExposesLockedExchangeWithoutMerchantCredentials(t *testing.T) {
	order := &dbent.PaymentOrder{
		ID: 31, UserID: 12, OutTradeNo: "usdt-response-31", PaymentType: payment.TypeUSDTTRC20,
		OrderType: payment.OrderTypeBalance, Amount: 1.68, PayAmount: 1.43, BonusAmount: 0.28,
		ProviderSnapshot: map[string]any{
			"currency": "USDT", "provider_key": payment.TypeUSDTTRC20,
			"token": "商户密钥不得外泄", "apiKey": "接口密钥不得外泄",
			"usdt_exchange": map[string]any{
				"rate": 7.2, "source": "okx", "fetched_at": "2026-09-30T01:00:00Z", "observed_at": "2026-09-30T01:00:00Z",
				"cny_base_amount": 10, "cny_pay_amount": 10.25, "usdt_pay_amount": 1.43, "pricing_mode": "balance_cny",
				"token": "快照嵌套密钥不得外泄",
			},
		},
	}
	for _, value := range []any{sanitizeAdminPaymentOrderForResponse(order), sanitizeAdminPaymentOrdersForResponse([]*dbent.PaymentOrder{order})[0]} {
		raw, err := json.Marshal(value)
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.Unmarshal(raw, &result))
		require.Equal(t, 1.68, result["amount"])
		require.Equal(t, 0.28, result["bonus_amount"])
		require.Equal(t, false, result["refund_supported"])
		detail, ok := result["usdt_exchange"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, 7.2, detail["rate"])
		require.Equal(t, "okx", detail["source"])
		require.Equal(t, "2026-09-30T01:00:00Z", detail["fetched_at"])
		require.Equal(t, 10.25, detail["cny_pay_amount"])
		require.Equal(t, 1.43, detail["usdt_pay_amount"])
		for _, forbidden := range []string{"provider_snapshot", "token", "apiKey", "商户密钥不得外泄", "接口密钥不得外泄", "快照嵌套密钥不得外泄"} {
			require.NotContains(t, string(raw), forbidden)
		}
	}
}

func TestUSDTQuoteAdminDTOOmitsQuoteForLegacyOrders(t *testing.T) {
	for _, snapshot := range []map[string]any{nil, {"currency": "USDT", "provider_key": payment.TypeUSDTTRC20}} {
		order := &dbent.PaymentOrder{ID: 31, PaymentType: payment.TypeUSDTTRC20, ProviderSnapshot: snapshot}
		for _, value := range []any{sanitizeAdminPaymentOrderForResponse(order), sanitizeAdminPaymentOrdersForResponse([]*dbent.PaymentOrder{order})[0]} {
			raw, err := json.Marshal(value)
			require.NoError(t, err)
			var result map[string]any
			require.NoError(t, json.Unmarshal(raw, &result))
			require.NotContains(t, result, "usdt_exchange", "管理员页面同样不能补造旧订单报价")
			require.NotContains(t, result, "provider_snapshot")
		}
	}
}
