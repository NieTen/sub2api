package provider

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"go.uber.org/zap"
)

const okpayDebugMessageLimit = 512

var (
	okpayDebugLabelPattern         = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	okpayDebugCoinPattern          = regexp.MustCompile(`^[A-Z]{2,12}$`)
	okpayDebugDigitsPattern        = regexp.MustCompile(`^[0-9]+$`)
	okpayDebugHostPattern          = regexp.MustCompile(`^[A-Za-z0-9.:\[\]-]+$`)
	okpayDebugURLPattern           = regexp.MustCompile(`(?i)\b(?:https?|ftp)(?:://|%3a%2f%2f)[^\s<>"']+`)
	okpayDebugJWTPattern           = regexp.MustCompile(`\b[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)
	okpayDebugLongHexPattern       = regexp.MustCompile(`(?i)\b[0-9a-f]{24,}\b`)
	okpayDebugBearerPattern        = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/-]+=*`)
	okpayDebugPercentEscapePattern = regexp.MustCompile(`%[0-9A-Fa-f]{2}`)
)

// 请求原值只在本次调用内用于脱敏，任何日志都不得直接序列化此结构。
type okpayDebugRequest struct {
	started           time.Time
	operation         string
	algorithm         string
	transport         string
	payload           url.Values
	requestSent       bool
	httpStatus        int
	responseBytes     int
	response          okpayArray
	validationField   string
	validationReason  string
	validationMessage string
	urlSources        map[string]string
}

func (o *OKPay) debugLoggingEnabled() bool {
	return o != nil && strings.EqualFold(strings.TrimSpace(o.config["debugLogging"]), "true")
}

// 本地校验发生在签名和 HTTP 调用前，也必须在调试模式中留下可定位的记录。
func (o *OKPay) createPaymentValidationError(ctx context.Context, request payment.CreatePaymentRequest, field, reason string, err error) error {
	if !o.debugLoggingEnabled() {
		return err
	}
	returnURL, returnSource := okpayResolvePaymentURL(request.ReturnURL, o.config["returnUrl"])
	callbackURL, callbackSource := okpayResolvePaymentURL(request.NotifyURL, o.config["notifyUrl"])
	debug := &okpayDebugRequest{
		started: time.Now(), operation: "/payLink", algorithm: o.config["signatureAlgorithm"], transport: okpayTransportCurrent,
		payload: url.Values{
			"unique_id": {request.OrderID}, "name": {request.Subject}, "amount": {request.Amount},
			"coin": {okpayCurrency}, "return_url": {returnURL}, "callback_url": {callbackURL}, "status": {"0"},
		},
		validationField: field, validationReason: reason, validationMessage: err.Error(),
		urlSources: map[string]string{"return_url": returnSource, "callback_url": callbackSource},
	}
	o.logDebugRequest(ctx, debug, err)
	return err
}

func (o *OKPay) logDebugRequest(ctx context.Context, request *okpayDebugRequest, requestErr error) {
	if request == nil {
		return
	}
	sensitive := okpayDebugSensitiveValues(o.config, request.payload)
	stage := "upstream"
	result := "success"
	if requestErr != nil {
		result = okpayCheckRequestFailed
		var classified *okpayDiagnosticFailure
		if errors.As(requestErr, &classified) {
			switch classified.status {
			case okpayCheckRejected, okpayCheckInvalidResponse, okpayCheckRequestFailed:
				result = classified.status
			}
		}
	}
	if request.validationField != "" {
		stage, result = "validation", "validation_failed"
	}
	summary := okpayDebugRequestSummary(request.payload, sensitive)
	for _, key := range []string{"return_url", "callback_url"} {
		if source, exists := request.urlSources[key]; exists {
			if link, ok := summary[key].(map[string]any); ok {
				link["source"] = source
			}
		}
	}
	fields := []zap.Field{
		zap.String("component", "payment.okpay"),
		zap.String("provider", "okpay"),
		zap.String("instance_id", okpayDebugLabel(o.instanceID)),
		zap.String("operation", okpayDebugOperation(request.operation)),
		zap.String("signature_algorithm", okpayDebugLabel(request.algorithm)),
		zap.String("transport", okpayDebugLabel(request.transport)),
		zap.String("stage", stage),
		zap.Int("http_status", request.httpStatus),
		zap.Int64("duration_ms", time.Since(request.started).Milliseconds()),
		zap.String("result", result),
		zap.Bool("request_sent", request.requestSent),
		zap.Int("response_bytes", request.responseBytes),
		zap.Any("request", summary),
	}
	if request.validationField != "" {
		fields = append(fields,
			zap.String("validation_field", request.validationField),
			zap.String("validation_reason", request.validationReason),
			zap.String("validation_message", request.validationMessage),
		)
	}
	if status, ok := request.response.get("status"); ok {
		if text, ok := status.(string); ok {
			switch text {
			case "success", "warning", "error", "failed", "failure", "fail":
				fields = append(fields, zap.String("upstream_status", text))
			default:
				fields = append(fields, zap.String("upstream_status", "other"))
			}
		}
	}
	if code := okpaySafeBusinessCode(request.response, sensitive); code != "" {
		fields = append(fields, zap.String("business_code", code))
	}
	if requestErr != nil {
		messages := make(map[string]string)
		for _, key := range []string{"msg", "message"} {
			if value, exists := request.response.get(key); exists {
				if text, ok := value.(string); ok && text != "" {
					messages[key] = okpayDebugRedactMessage(text, sensitive)
				}
			}
		}
		if len(messages) > 0 {
			fields = append(fields, zap.Any("upstream_messages", messages))
		}
		logger.FromContext(ctx).Warn("OKPay debug", fields...)
		return
	}
	// 成功响应即使附带 msg/message 也不输出，更不记录 data、余额或支付链接。
	logger.FromContext(ctx).Info("OKPay debug", fields...)
}

func okpayDebugLabel(value string) string {
	if okpayDebugLabelPattern.MatchString(value) {
		return value
	}
	return "unknown"
}

func okpayDebugOperation(path string) string {
	switch path {
	case "/payLink":
		return "payLink"
	case "/balance":
		return "balance"
	case "/checkDeposit":
		return "checkDeposit"
	default:
		return "unknown"
	}
}

func okpayDebugRequestSummary(payload url.Values, sensitive []string) map[string]any {
	knownFields := []string{"amount", "callback_url", "coin", "id", "name", "nonce", "return_url", "sign", "status", "timestamp", "unique_id"}
	fieldNames := make([]string, 0, len(knownFields))
	for _, key := range knownFields {
		if _, exists := payload[key]; exists {
			fieldNames = append(fieldNames, key)
		}
	}
	summary := map[string]any{
		"field_count":     len(payload),
		"fields":          fieldNames,
		"name_bytes":      len(payload.Get("name")),
		"name_runes":      utf8.RuneCountInString(payload.Get("name")),
		"unique_id_bytes": len(payload.Get("unique_id")),
		"nonce_bytes":     len(payload.Get("nonce")),
		"sign_bytes":      len(payload.Get("sign")),
		"return_url":      okpayDebugURLSummary(payload.Get("return_url"), sensitive),
		"callback_url":    okpayDebugURLSummary(payload.Get("callback_url"), sensitive),
	}
	if amount := payload.Get("amount"); len(amount) <= 32 && okpayAmountPattern.MatchString(amount) {
		summary["amount"] = amount
	}
	if coin := payload.Get("coin"); okpayDebugCoinPattern.MatchString(coin) {
		summary["coin"] = coin
	}
	if status := payload.Get("status"); len(status) <= 3 && okpayDebugDigitsPattern.MatchString(status) {
		summary["status"] = status
	}
	if timestamp := payload.Get("timestamp"); len(timestamp) <= 16 && okpayDebugDigitsPattern.MatchString(timestamp) {
		summary["timestamp"] = timestamp
	}
	return summary
}

func okpayDebugURLSummary(raw string, sensitive []string) map[string]any {
	reason := ""
	if raw != "" {
		reason = okpayHTTPSURLFailure(raw)
	}
	summary := map[string]any{
		"present": raw != "", "bytes": len(raw), "https": false, "host": "", "path_bytes": 0, "query_bytes": 0, "query_params": 0,
		"valid": raw != "" && reason == "", "validation_reason": reason, "scheme": "relative",
		"has_userinfo": false, "has_control_chars": strings.ContainsAny(raw, "\r\n\x00"),
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		summary["scheme"] = "other"
		return summary
	}
	switch parsed.Scheme {
	case "http", "https":
		summary["scheme"] = parsed.Scheme
	case "":
	default:
		summary["scheme"] = "other"
	}
	summary["has_userinfo"] = parsed.User != nil
	summary["https"] = parsed.Scheme == "https"
	summary["path_bytes"] = len(parsed.EscapedPath())
	summary["query_bytes"] = len(parsed.RawQuery)
	count := 0
	for _, part := range strings.Split(parsed.RawQuery, "&") {
		if part != "" {
			count++
		}
	}
	summary["query_params"] = count
	if host := parsed.Hostname(); len(host) <= 253 && okpayDebugHostPattern.MatchString(host) {
		summary["host"] = okpayDebugReplaceKnown(host, sensitive)
	}
	return summary
}

func okpayDebugSensitiveValues(config map[string]string, payload url.Values) []string {
	values := []string{config["id"], config["token"]}
	for _, key := range []string{"id", "name", "unique_id", "nonce", "sign"} {
		values = append(values, payload[key]...)
	}
	urls := []string{config["apiBase"], config["returnUrl"], config["notifyUrl"]}
	urls = append(urls, payload["return_url"]...)
	urls = append(urls, payload["callback_url"]...)
	for _, raw := range urls {
		values = append(values, raw)
		// 原始查询值也加入集合，解析失败或上游反射编码文本时仍能脱敏。
		beforeFragment, fragment, _ := strings.Cut(raw, "#")
		values = append(values, fragment)
		if _, query, found := strings.Cut(beforeFragment, "?"); found {
			for _, pair := range strings.Split(query, "&") {
				if _, value, found := strings.Cut(pair, "="); found {
					values = append(values, value)
				}
			}
		}
		if parsed, err := url.Parse(raw); err == nil {
			if parsed.User != nil {
				values = append(values, parsed.User.Username())
				if password, found := parsed.User.Password(); found {
					values = append(values, password)
				}
			}
			values = append(values, parsed.Fragment, parsed.RawFragment)
			for _, queryValues := range parsed.Query() {
				values = append(values, queryValues...)
			}
		}
	}
	return values
}

func okpayDebugReplaceKnown(text string, sensitive []string) string {
	unique := make(map[string]struct{})
	for _, value := range sensitive {
		variants := []string{value, strings.TrimSpace(value)}
		if decoded, err := url.QueryUnescape(value); err == nil {
			variants = append(variants, decoded)
		}
		if decoded, err := url.PathUnescape(value); err == nil {
			variants = append(variants, decoded)
		}
		for _, variant := range variants {
			if variant == "" {
				continue
			}
			encoded, _ := json.Marshal(variant)
			for _, candidate := range []string{variant, url.QueryEscape(variant), url.PathEscape(variant), html.EscapeString(variant), string(encoded[1 : len(encoded)-1])} {
				unique[candidate] = struct{}{}
				// PHP 常见的斜杠转义及百分号编码大小写变化也不能留下反射凭据。
				unique[strings.ReplaceAll(candidate, "/", `\/`)] = struct{}{}
				unique[okpayDebugPercentEscapePattern.ReplaceAllStringFunc(candidate, strings.ToLower)] = struct{}{}
				unique[okpayDebugPercentEscapePattern.ReplaceAllStringFunc(candidate, strings.ToUpper)] = struct{}{}
			}
		}
	}
	if len(unique) == 0 {
		return text
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	// 长值优先且只替换一次，避免短 ID 提前切碎密钥、URL 或转义后的完整值。
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) == len(keys[j]) {
			return keys[i] < keys[j]
		}
		return len(keys[i]) > len(keys[j])
	})
	pairs := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		pairs = append(pairs, key, "***")
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

func okpayDebugRedactMessage(text string, sensitive []string) string {
	text = okpayDebugReplaceKnown(text, sensitive)
	text = logredact.RedactText(text, "id", "token", "merchant_id", "merchant_token", "sign", "signature", "nonce", "unique_id", "order_id", "name", "return_url", "callback_url", "resume_token", "authorization", "data", "balance", "usdt", "trx", "cny", "余额")
	text = okpayDebugURLPattern.ReplaceAllString(text, "***")
	text = okpayDebugJWTPattern.ReplaceAllString(text, "***")
	text = okpayDebugLongHexPattern.ReplaceAllString(text, "***")
	text = okpayDebugBearerPattern.ReplaceAllString(text, "***")
	text = okpayDebugReplaceKnown(text, sensitive)
	// 必须完整脱敏后再去掉控制字符并截断，不能留下被截断密钥的可见前缀。
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, text)
	runes := []rune(text)
	if len(runes) > okpayDebugMessageLimit {
		text = string(runes[:okpayDebugMessageLimit]) + "…"
	}
	return strings.TrimSpace(text)
}
