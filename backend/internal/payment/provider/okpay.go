package provider

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

const (
	okpayDefaultAPIBase  = "https://api.okaypay.me/shop"
	okpayMaxResponseSize = 1 << 20
	okpayCurrency        = "USDT"
)

var okpayAmountPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// OKPay 使用用户提供的商户 API 收银台协议，不直接托管钱包或扫描链上交易。
type OKPay struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

var _ payment.MerchantOrderQueryProvider = (*OKPay)(nil)
var _ payment.MerchantIdentityProvider = (*OKPay)(nil)

func NewOKPay(instanceID string, config map[string]string) (*OKPay, error) {
	for _, key := range []string{"id", "token"} {
		if strings.TrimSpace(config[key]) == "" {
			return nil, fmt.Errorf("OKPay 配置缺少 %s", key)
		}
	}
	cfg := cloneStringMap(config)
	cfg["id"] = strings.TrimSpace(cfg["id"])
	if len(cfg["id"]) > 128 || strings.ContainsAny(cfg["id"], "\r\n\x00") {
		return nil, fmt.Errorf("OKPay 商户 ID 无效")
	}
	apiBase := strings.TrimSpace(cfg["apiBase"])
	if apiBase == "" {
		apiBase = okpayDefaultAPIBase
	}
	parsed, err := url.Parse(apiBase)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("OKPay apiBase 必须是无凭据、查询参数及片段的 HTTPS 地址")
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/shop"
	}
	cfg["apiBase"] = strings.TrimRight(parsed.String(), "/")
	if mode := strings.TrimSpace(cfg["paymentMode"]); mode != "" && mode != "redirect" {
		return nil, fmt.Errorf("OKPay 仅支持 redirect 收银台模式")
	}
	return &OKPay{instanceID: instanceID, config: cfg, httpClient: &http.Client{
		Timeout: 10 * time.Second,
		// 禁止重定向携带商户身份和签名，TLS 使用系统证书校验。
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (o *OKPay) Name() string        { return "OKPay" }
func (o *OKPay) ProviderKey() string { return payment.TypeOKPay }
func (o *OKPay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeOKPay}
}
func (o *OKPay) MerchantIdentityMetadata() map[string]string {
	return map[string]string{"merchant_id": o.config["id"], "currency": okpayCurrency}
}

func (o *OKPay) CreatePayment(ctx context.Context, request payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if strings.TrimSpace(request.OrderID) == "" {
		return nil, fmt.Errorf("OKPay 缺少商户订单号")
	}
	amount, err := okpayPositiveAmount(request.Amount)
	if err != nil {
		return nil, err
	}
	if !amount.Equal(amount.Truncate(2)) {
		return nil, fmt.Errorf("OKPay 下单金额最多两位小数")
	}
	returnURL := strings.TrimSpace(request.ReturnURL)
	if returnURL == "" {
		returnURL = strings.TrimSpace(o.config["returnUrl"])
	}
	callbackURL := strings.TrimSpace(request.NotifyURL)
	if callbackURL == "" {
		callbackURL = strings.TrimSpace(o.config["notifyUrl"])
	}
	for _, link := range []string{returnURL, callbackURL} {
		if link != "" && !okpayHTTPSURL(link) {
			return nil, fmt.Errorf("OKPay 返回或回调地址须为 HTTPS 地址")
		}
	}
	fields := okpayArray{
		{key: "unique_id", value: request.OrderID}, {key: "name", value: request.Subject},
		{key: "amount", value: amount.StringFixed(2)}, {key: "coin", value: okpayCurrency},
		{key: "return_url", value: returnURL}, {key: "callback_url", value: callbackURL},
		// PHP array_filter 会移除状态 0，服务端默认值仍为 0。
		{key: "status", value: "0"},
	}
	response, err := o.post(ctx, "/payLink", fields)
	if err != nil {
		return nil, err
	}
	data, err := okpayResponseData(response)
	if err != nil {
		return nil, err
	}
	tradeNo, err := okpayRequiredScalar(data, "order_id")
	if err != nil {
		return nil, err
	}
	payURL, err := okpayRequiredScalar(data, "pay_url")
	if err != nil || !okpayHTTPSURL(payURL) {
		return nil, fmt.Errorf("OKPay 返回的支付链接无效")
	}
	if err = o.validateIdentityAndCurrency(response, data, false); err != nil {
		return nil, err
	}
	if unique, found := data.get("unique_id"); found {
		text, err := okpayPHPScalar(unique)
		if err != nil || text != request.OrderID {
			return nil, fmt.Errorf("OKPay 下单响应的商户订单号不匹配")
		}
	}
	return &payment.CreatePaymentResponse{TradeNo: tradeNo, PayURL: payURL, Currency: okpayCurrency, ResultType: payment.CreatePaymentResultOrderCreated}, nil
}

func (o *OKPay) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return nil, fmt.Errorf("OKPay 查单需要商户订单号，请使用 QueryOrderByMerchantOrderID")
}

func (o *OKPay) QueryOrderByMerchantOrderID(ctx context.Context, merchantOrderID string) (*payment.QueryOrderResponse, error) {
	if strings.TrimSpace(merchantOrderID) == "" {
		return nil, fmt.Errorf("OKPay 查单缺少商户订单号")
	}
	response, err := o.post(ctx, "/checkDeposit", okpayArray{{key: "unique_id", value: merchantOrderID}})
	if err != nil {
		return nil, err
	}
	data, err := okpayResponseData(response)
	if err != nil {
		return nil, err
	}
	uniqueID, err := okpayRequiredScalar(data, "unique_id")
	if err != nil || uniqueID != merchantOrderID {
		return nil, fmt.Errorf("OKPay 查单响应的商户订单号不匹配")
	}
	if err = o.validateIdentityAndCurrency(response, data, false); err != nil {
		return nil, err
	}
	if kind, found := data.get("type"); found {
		text, err := okpayPHPScalar(kind)
		if err != nil || text != "deposit" {
			return nil, fmt.Errorf("OKPay 查单返回非充值交易")
		}
	}
	status, err := okpayRequiredScalar(data, "status")
	if err != nil || (status != "0" && status != "1") {
		return nil, fmt.Errorf("OKPay 查单状态无效")
	}
	tradeNo, err := okpayRequiredScalar(data, "order_id")
	if err != nil {
		return nil, err
	}
	amount, err := okpayDataAmount(data)
	if err != nil {
		return nil, err
	}
	resultStatus := payment.ProviderStatusPending
	if status == "1" {
		resultStatus = payment.ProviderStatusPaid
	}
	metadata := o.MerchantIdentityMetadata()
	metadata["unique_id"], metadata["type"], metadata["status"] = uniqueID, "deposit", status
	return &payment.QueryOrderResponse{TradeNo: tradeNo, Status: resultStatus, Amount: amount, Metadata: metadata}, nil
}

func (o *OKPay) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if len(rawBody) > okpayMaxResponseSize {
		return nil, fmt.Errorf("OKPay 回调内容过大")
	}
	var fields okpayArray
	var err error
	if strings.HasPrefix(strings.TrimSpace(rawBody), "{") || strings.Contains(strings.ToLower(headers["content-type"]), "application/json") {
		fields, err = okpayDecodeJSON(rawBody)
	} else {
		fields, err = okpayDecodeForm(rawBody)
	}
	if err != nil {
		return nil, fmt.Errorf("OKPay 回调格式无效: %w", err)
	}
	provided, err := okpayRequiredScalar(fields, "sign")
	if err != nil || len(provided) != 32 {
		return nil, fmt.Errorf("OKPay 回调缺少有效签名")
	}
	expected, err := okpaySign(fields, o.config["token"])
	if err != nil || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		return nil, fmt.Errorf("OKPay 回调签名无效")
	}
	data, err := okpayResponseData(fields)
	if err != nil {
		return nil, err
	}
	if err = o.validateIdentityAndCurrency(fields, data, true); err != nil {
		return nil, err
	}
	kind, err := okpayRequiredScalar(data, "type")
	if err != nil {
		return nil, err
	}
	if kind == "withdraw" {
		return nil, nil
	}
	if kind != "deposit" {
		return nil, fmt.Errorf("OKPay 回调交易类型无效")
	}
	status, err := okpayRequiredScalar(data, "status")
	if err != nil || (status != "0" && status != "1") {
		return nil, fmt.Errorf("OKPay 充值回调状态无效")
	}
	if status == "0" {
		return nil, nil
	}
	orderID, err := okpayRequiredScalar(data, "unique_id")
	if err != nil {
		return nil, err
	}
	tradeNo, err := okpayRequiredScalar(data, "order_id")
	if err != nil {
		return nil, err
	}
	amount, err := okpayDataAmount(data)
	if err != nil {
		return nil, err
	}
	metadata := o.MerchantIdentityMetadata()
	metadata["unique_id"], metadata["type"], metadata["status"] = orderID, kind, status
	return &payment.PaymentNotification{TradeNo: tradeNo, OrderID: orderID, Amount: amount, Status: payment.NotificationStatusSuccess, RawData: rawBody, Metadata: metadata}, nil
}

func (o *OKPay) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	// 文档的 transfer 是向 Telegram 用户转账，不能替代商户原路退款。
	return nil, fmt.Errorf("OKPay 商户文档未提供原路退款接口，不支持自动退款")
}

func (o *OKPay) validateIdentityAndCurrency(fields, data okpayArray, required bool) error {
	_, found := fields.get("id")
	if found {
		text, err := okpayRequiredScalar(fields, "id")
		if err != nil || text != o.config["id"] {
			return fmt.Errorf("OKPay 商户 ID 不匹配")
		}
	} else if required {
		return fmt.Errorf("OKPay 回调缺少商户 ID")
	}
	coin, found := data.get("coin")
	if found {
		text, err := okpayPHPScalar(coin)
		if err != nil || text != okpayCurrency {
			return fmt.Errorf("OKPay 交易币种不匹配")
		}
	} else if required {
		return fmt.Errorf("OKPay 回调缺少交易币种")
	}
	return nil
}

func okpayResponseData(fields okpayArray) (okpayArray, error) {
	if value, found := fields.get("status"); found {
		status, err := okpayPHPScalar(value)
		if err != nil || status != "success" {
			return nil, fmt.Errorf("OKPay 返回业务失败状态")
		}
	}
	if value, found := fields.get("code"); found {
		code, err := okpayPHPScalar(value)
		if err != nil || code != "10000" {
			return nil, fmt.Errorf("OKPay 返回业务失败代码")
		}
	}
	value, found := fields.get("data")
	data, ok := value.(okpayArray)
	if !found || !ok || len(data) == 0 {
		return nil, fmt.Errorf("OKPay 响应缺少有效 data")
	}
	return data, nil
}

func okpayRequiredScalar(fields okpayArray, key string) (string, error) {
	value, found := fields.get(key)
	if !found || value == nil {
		return "", fmt.Errorf("OKPay 缺少 %s", key)
	}
	// 状态、金额及身份不能通过布尔值的 PHP 强制转换蒙混过关。
	if _, invalid := value.(bool); invalid {
		return "", fmt.Errorf("OKPay %s 类型无效", key)
	}
	text, err := okpayPHPScalar(value)
	if err != nil || strings.TrimSpace(text) == "" || strings.ContainsAny(text, "\r\n\x00") {
		return "", fmt.Errorf("OKPay %s 无效", key)
	}
	return text, nil
}

func okpayPositiveAmount(raw string) (decimal.Decimal, error) {
	if len(raw) > 32 || !okpayAmountPattern.MatchString(raw) {
		return decimal.Zero, fmt.Errorf("OKPay 金额格式无效")
	}
	amount, err := decimal.NewFromString(raw)
	if err != nil || !amount.IsPositive() {
		return decimal.Zero, fmt.Errorf("OKPay 金额必须大于零")
	}
	return amount, nil
}

func okpayDataAmount(data okpayArray) (float64, error) {
	raw, err := okpayRequiredScalar(data, "amount")
	if err != nil {
		return 0, err
	}
	amount, err := okpayPositiveAmount(raw)
	if err != nil {
		return 0, err
	}
	value, _ := amount.Float64()
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("OKPay 金额超出范围")
	}
	return value, nil
}

func okpayHTTPSURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && !strings.ContainsAny(raw, "\r\n\x00")
}

func (o *OKPay) post(ctx context.Context, path string, fields okpayArray) (okpayArray, error) {
	fields = append(fields, okpayField{key: "id", value: o.config["id"]})
	signature, err := okpaySign(fields, o.config["token"])
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"sign": signature}
	for _, field := range fields {
		if okpayPHPTruthy(field.value) {
			payload[field.key] = field.value
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, o.config["apiBase"]+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := o.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("OKPay 请求失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("OKPay HTTP 状态异常: %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, okpayMaxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("OKPay 读取响应失败")
	}
	if len(body) > okpayMaxResponseSize {
		return nil, fmt.Errorf("OKPay 响应内容过大")
	}
	result, err := okpayDecodeJSON(string(body))
	if err != nil {
		return nil, fmt.Errorf("OKPay 响应 JSON 无效")
	}
	return result, nil
}
