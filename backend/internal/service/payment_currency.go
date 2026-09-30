package service

import (
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func paymentProviderConfigCurrency(providerKey string, cfg map[string]string) string {
	switch strings.TrimSpace(providerKey) {
	case payment.TypeOKPay, "usdt_trc20":
		return "USDT"
	case payment.TypeStripe, payment.TypeAirwallex:
		currency, err := payment.NormalizePaymentCurrency(cfg["currency"])
		if err == nil {
			return currency
		}
	}
	return payment.DefaultPaymentCurrency
}

func PaymentOrderCurrency(order *dbent.PaymentOrder) string {
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil {
		if currency, err := payment.NormalizePaymentCurrency(snapshot.Currency); err == nil {
			return currency
		}
	}
	return payment.DefaultPaymentCurrency
}

// PaymentOrderRefundSupported 仅声明明确不支持原路退款的渠道，兼容旧订单及易支付自定义方式。
func PaymentOrderRefundSupported(order *dbent.PaymentOrder) *bool {
	if order == nil {
		return nil
	}
	key := psStringValue(order.ProviderKey)
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil && snapshot.ProviderKey != "" {
		key = snapshot.ProviderKey
	}
	if providerDisablesRefund(key) {
		supported := false
		return &supported
	}
	return nil
}
