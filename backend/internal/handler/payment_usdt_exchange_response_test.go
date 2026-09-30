package handler

import (
	"encoding/json"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestUSDTQuoteUserAndResumeDTOExposeOnlyLockedExchangeDetails(t *testing.T) {
	order := &dbent.PaymentOrder{
		ID: 31, UserID: 12, OutTradeNo: "usdt-response-31", PaymentType: payment.TypeOKPay,
		OrderType: payment.OrderTypeBalance, Amount: 1.4, PayAmount: 1.43,
		ProviderSnapshot: map[string]any{
			"currency": "USDT", "provider_key": payment.TypeOKPay,
			"token": "商户密钥不得外泄", "apiKey": "接口密钥不得外泄",
			"usdt_exchange": map[string]any{
				"rate": 7.2, "source": "fallback", "fallback_reason": "OKX 抓取失败",
				"fetched_at": "2026-09-30T01:00:00Z", "observed_at": "2026-09-30T01:30:00Z",
				"cny_base_amount": 10, "cny_pay_amount": 10.25, "usdt_pay_amount": 1.43,
				"pricing_mode": "balance_cny", "token": "快照嵌套密钥不得外泄",
			},
		},
	}
	for _, projection := range []struct {
		name  string
		value any
	}{
		{"用户订单详情", sanitizePaymentOrderForResponse(order)},
		{"用户订单列表", sanitizePaymentOrdersForResponse([]*dbent.PaymentOrder{order})[0]},
		{"签名链接恢复订单", buildPublicOrderResult(order)},
	} {
		t.Run(projection.name, func(t *testing.T) {
			raw, err := json.Marshal(projection.value)
			require.NoError(t, err)
			var result map[string]any
			require.NoError(t, json.Unmarshal(raw, &result))
			require.Equal(t, "USDT", result["currency"])
			detail, ok := result["usdt_exchange"].(map[string]any)
			require.True(t, ok, "前端必须收到本单锁定报价")
			require.Equal(t, 7.2, detail["rate"])
			require.Equal(t, "fallback", detail["source"])
			require.Equal(t, "OKX 抓取失败", detail["fallback_reason"])
			require.Equal(t, "2026-09-30T01:00:00Z", detail["fetched_at"])
			require.Equal(t, "2026-09-30T01:30:00Z", detail["observed_at"])
			require.Equal(t, 10.0, detail["cny_base_amount"])
			require.Equal(t, 10.25, detail["cny_pay_amount"])
			require.Equal(t, 1.43, detail["usdt_pay_amount"])
			for _, forbidden := range []string{"provider_snapshot", "token", "apiKey", "商户密钥不得外泄", "接口密钥不得外泄", "快照嵌套密钥不得外泄"} {
				require.NotContains(t, string(raw), forbidden)
			}
		})
	}
	// 无签名的历史查单接口仍只公开最少状态，不扩展为完整收银台恢复接口。
	public, err := json.Marshal(buildPublicOrderVerifyResult(order))
	require.NoError(t, err)
	require.NotContains(t, string(public), "usdt_exchange")
}

func TestUSDTQuoteUserAndResumeDTOOmitQuoteForLegacyOrders(t *testing.T) {
	for _, snapshot := range []map[string]any{nil, {"currency": "USDT", "provider_key": payment.TypeOKPay}} {
		order := &dbent.PaymentOrder{ID: 31, PaymentType: payment.TypeOKPay, ProviderSnapshot: snapshot}
		for _, value := range []any{sanitizePaymentOrderForResponse(order), sanitizePaymentOrdersForResponse([]*dbent.PaymentOrder{order})[0], buildPublicOrderResult(order)} {
			raw, err := json.Marshal(value)
			require.NoError(t, err)
			var result map[string]any
			require.NoError(t, json.Unmarshal(raw, &result))
			require.NotContains(t, result, "usdt_exchange", "没有历史快照的订单不能套用当前报价")
			require.NotContains(t, result, "provider_snapshot")
		}
	}
}
