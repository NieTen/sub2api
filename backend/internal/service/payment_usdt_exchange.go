package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

// PaymentExchangeRateReader 让下单只读取已采集的报价，不等待外部行情接口。
type PaymentExchangeRateReader interface {
	GetQuote(context.Context) (*PaymentExchangeRateQuote, error)
	History(context.Context) ([]PaymentExchangeRatePoint, error)
}

// PaymentOrderExchangeDetails 保留订单使用的费率与人民币基数，后续行情变化不修改该快照。
type PaymentOrderExchangeDetails struct {
	Rate                     float64    `json:"rate"`
	Source                   string     `json:"source"`
	FetchedAt                *time.Time `json:"fetched_at,omitempty"`
	ObservedAt               time.Time  `json:"observed_at"`
	FallbackReason           string     `json:"fallback_reason,omitempty"`
	CNYBaseAmount            float64    `json:"cny_base_amount"`
	CNYPayAmount             float64    `json:"cny_pay_amount"`
	USDTPayAmount            float64    `json:"usdt_pay_amount"`
	PricingMode              string     `json:"pricing_mode"`
	PlanPrice                float64    `json:"plan_price,omitempty"`
	SubscriptionUSDToCNYRate float64    `json:"subscription_usd_to_cny_rate,omitempty"`
}

type PaymentUSDTRatesResponse struct {
	Current *PaymentExchangeRateQuote  `json:"current"`
	History []PaymentExchangeRatePoint `json:"history"`
	Error   string                     `json:"error,omitempty"`
}

func (s *PaymentService) SetExchangeRateService(reader PaymentExchangeRateReader) {
	s.exchangeRateSvc = reader
}

func (s *PaymentConfigService) SetExchangeRateService(reader PaymentExchangeRateReader) {
	s.exchangeRateSvc = reader
}

func normalizeUSDTCNYFallbackRate(rate float64) float64 {
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 1 || rate > 100 {
		return 0
	}
	return rate
}

func (s *PaymentConfigService) GetUSDTCNYFallbackRate(ctx context.Context) (float64, error) {
	values, err := s.settingRepo.GetMultiple(ctx, []string{SettingUSDTCNYFallbackRate})
	if err != nil {
		return 0, err
	}
	return normalizeUSDTCNYFallbackRate(pcParseFloat(values[SettingUSDTCNYFallbackRate], 0)), nil
}

func usdtQuoteUnavailable() *PaymentExchangeRateQuote {
	return &PaymentExchangeRateQuote{Source: "unavailable", ObservedAt: time.Now(), FallbackReason: "汇率服务暂不可用，请联系管理员配置兜底费率"}
}

func (s *PaymentConfigService) methodUSDTQuote(ctx context.Context) *PaymentExchangeRateQuote {
	if s.exchangeRateSvc != nil {
		quote, err := s.exchangeRateSvc.GetQuote(ctx)
		if quote != nil && (err == nil || quote.Source == "unavailable") {
			return quote
		}
	}
	return usdtQuoteUnavailable()
}

func (s *PaymentService) GetUSDTRates(ctx context.Context) (*PaymentUSDTRatesResponse, error) {
	result := &PaymentUSDTRatesResponse{Current: usdtQuoteUnavailable(), History: []PaymentExchangeRatePoint{}}
	if s.exchangeRateSvc == nil {
		return result, nil
	}
	quote, err := s.exchangeRateSvc.GetQuote(ctx)
	if quote != nil {
		result.Current = quote
	}
	if err != nil {
		result.Error = "当前汇率读取失败，请检查行情采集与兜底设置"
	}
	history, err := s.exchangeRateSvc.History(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询 USDT 费率历史失败: %w", err)
	}
	if history != nil {
		result.History = history
	}
	return result, nil
}

func calculateUSDTExchangePayment(amount, fee float64, orderType string, subscriptionRate float64, quote *PaymentExchangeRateQuote) (*PaymentOrderExchangeDetails, error) {
	if quote == nil || normalizeUSDTCNYFallbackRate(quote.Rate) == 0 || (quote.Source != "okx" && quote.Source != "fallback") {
		return nil, infraerrors.ServiceUnavailable("USDT_EXCHANGE_RATE_UNAVAILABLE", "暂无可用的 USDT 费率，请联系管理员设置兜底费率")
	}
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 || math.IsNaN(fee) || math.IsInf(fee, 0) || fee < 0 || fee > 100 {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "充值金额或手续费无效")
	}
	detail := &PaymentOrderExchangeDetails{
		Rate: quote.Rate, Source: quote.Source, ObservedAt: quote.ObservedAt,
		FallbackReason: quote.FallbackReason, CNYBaseAmount: amount, PricingMode: "balance_cny",
	}
	if quote.FetchedAt != nil {
		fetchedAt := *quote.FetchedAt
		detail.FetchedAt = &fetchedAt
	}
	if orderType == payment.OrderTypeSubscription {
		detail.PlanPrice = amount
		detail.SubscriptionUSDToCNYRate = normalizeSubscriptionUSDToCNYRate(subscriptionRate)
		detail.CNYBaseAmount = calculateSubscriptionGatewayBaseAmount(amount, subscriptionRate, "CNY")
		detail.PricingMode = "subscription_legacy_cny"
		if detail.SubscriptionUSDToCNYRate > 0 {
			detail.PricingMode = "subscription_cny"
		}
	}
	// 先沿用人民币渠道手续费算法，再将含手续费金额向上换算至 USDT 分。
	cnyPay, err := decimal.NewFromString(payment.CalculatePayAmountForCurrency(detail.CNYBaseAmount, fee, "CNY"))
	if err != nil || !cnyPay.IsPositive() {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "人民币应付金额必须大于零")
	}
	detail.CNYPayAmount = cnyPay.InexactFloat64()
	detail.USDTPayAmount = cnyPay.Div(decimal.NewFromFloat(quote.Rate)).RoundUp(2).InexactFloat64()
	return detail, nil
}

// PaymentOrderUSDTExchange 只返回订单原始快照；旧订单不使用当前行情补造历史。
func PaymentOrderUSDTExchange(order *dbent.PaymentOrder) *PaymentOrderExchangeDetails {
	if order == nil || order.ProviderSnapshot["usdt_exchange"] == nil {
		return nil
	}
	raw, err := json.Marshal(order.ProviderSnapshot["usdt_exchange"])
	if err != nil {
		return nil
	}
	var detail PaymentOrderExchangeDetails
	if json.Unmarshal(raw, &detail) != nil || normalizeUSDTCNYFallbackRate(detail.Rate) == 0 {
		return nil
	}
	return &detail
}

func paymentOrderDailyLimitAmount(order *dbent.PaymentOrder) float64 {
	if order.OrderType == payment.OrderTypeBalance {
		if detail := PaymentOrderUSDTExchange(order); detail != nil && detail.CNYPayAmount > 0 {
			return detail.CNYPayAmount
		}
		return order.PayAmount
	}
	return order.Amount
}
