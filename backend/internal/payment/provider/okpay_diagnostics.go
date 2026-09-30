package provider

import (
	"context"
	"errors"
	"strings"
)

const (
	okpayTransportCurrent      = "current"
	okpayTransportPHPReference = "php_reference"
	okpayTransportHMAC         = "hmac_sha256"
	okpayCheckAuthenticated    = "authenticated"
	okpayCheckRejected         = "rejected"
	okpayCheckRequestFailed    = "request_failed"
	okpayCheckInvalidResponse  = "invalid_response"
)

// OKPayAuthenticationDiagnostic 只包含认证分类，不携带凭据、余额或上游正文。
type OKPayAuthenticationDiagnostic struct {
	Checks     []OKPayAuthenticationCheck `json:"checks"`
	Conclusion string                     `json:"conclusion"`
}

type OKPayAuthenticationCheck struct {
	Mode         string `json:"mode"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	Reason       string `json:"reason"`
	HTTPStatus   int    `json:"http_status,omitempty"`
	BusinessCode string `json:"business_code,omitempty"`
}

// okpayDiagnosticFailure 保留原有错误链，诊断接口只使用分类，绝不输出底层错误文本。
type okpayDiagnosticFailure struct {
	status       string
	cause        error
	reason       string
	httpStatus   int
	businessCode string
}

func (e *okpayDiagnosticFailure) Error() string { return e.cause.Error() }
func (e *okpayDiagnosticFailure) Unwrap() error { return e.cause }

func okpayClassifiedFailure(status string, cause error) error {
	return &okpayDiagnosticFailure{status: status, cause: cause}
}

func okpayUpstreamFailure(status string, httpStatus int, fields okpayArray, cause error, sensitive ...string) error {
	return &okpayDiagnosticFailure{status: status, cause: cause, httpStatus: httpStatus,
		reason: okpayBusinessReason(fields, sensitive), businessCode: okpaySafeBusinessCode(fields, sensitive)}
}

func okpaySafeBusinessCode(fields okpayArray, sensitive []string) string {
	code := okpaySafeDiagnosticScalar(fields, "code", sensitive)
	if !okpayBusinessCodePattern.MatchString(code) {
		return ""
	}
	for _, secret := range sensitive {
		if secret != "" && strings.Contains(secret, code) {
			return ""
		}
	}
	return code
}

// 只将精确命中的上游消息归类，未知文本可能包含商户凭据，不能返回给前端。
func okpayBusinessReason(fields okpayArray, sensitive []string) string {
	for _, key := range []string{"msg", "message"} {
		switch strings.ToLower(okpaySafeDiagnosticScalar(fields, key, sensitive)) {
		case "身份认证失败", "认证失败", "authentication failed", "unauthorized":
			return "auth_failed"
		case "签名错误", "签名验证失败", "签名校验失败", "invalid signature", "signature error":
			return "signature_failed"
		case "商户不存在", "无效商户", "merchant not found", "invalid merchant":
			return "merchant_invalid"
		case "参数错误", "参数不完整", "缺少参数", "参数缺失", "invalid parameters", "missing parameters":
			return "invalid_parameters"
		case "请求过于频繁", "请求频繁", "too many requests", "rate limit exceeded":
			return "rate_limited"
		}
	}
	return "unknown_business_error"
}

func okpayBusinessFailureStatus(fields okpayArray) string {
	for _, key := range []string{"status", "code"} {
		if _, found := fields.get(key); !found {
			continue
		}
		// 畸形或空状态不代表明确的认证拒绝，不能据此归因于请求传输方式。
		if _, err := okpayRequiredScalar(fields, key); err != nil {
			return okpayCheckInvalidResponse
		}
	}
	return okpayCheckRejected
}

// DiagnoseAuthentication 对同一份已保存配置比较当前、新旧签名协议，只查询余额，不下单或重试。
func (o *OKPay) DiagnoseAuthentication(ctx context.Context) *OKPayAuthenticationDiagnostic {
	diagnostic := &OKPayAuthenticationDiagnostic{Checks: make([]OKPayAuthenticationCheck, 0, 3)}
	for _, mode := range []string{okpayTransportCurrent, okpayTransportPHPReference, okpayTransportHMAC} {
		fields, err := o.postWithTransport(ctx, "/balance", nil, mode)
		check := OKPayAuthenticationCheck{Mode: mode, Status: okpayCheckAuthenticated, Reason: "success"}
		if err != nil {
			check.Status, check.Reason = okpayCheckRequestFailed, "network_error"
			var failure *okpayDiagnosticFailure
			if errors.As(err, &failure) {
				check.Status, check.HTTPStatus, check.BusinessCode = failure.status, failure.httpStatus, failure.businessCode
				if failure.reason != "" && (check.Status == okpayCheckRejected || failure.reason != "unknown_business_error") {
					check.Reason = failure.reason
				}
			}
		} else if !okpayHasValidBalanceData(fields) {
			check.Status = okpayCheckInvalidResponse
		}
		if check.Status == okpayCheckInvalidResponse {
			check.Reason = "invalid_response"
		}
		if err == nil {
			check.BusinessCode = okpaySafeBusinessCode(fields, []string{o.config["id"], o.config["token"]})
		}
		check.Message = okpayDiagnosticMessage(check.Status)
		diagnostic.Checks = append(diagnostic.Checks, check)
	}
	for _, check := range diagnostic.Checks {
		if check.Status != okpayCheckAuthenticated && check.Status != okpayCheckRejected {
			diagnostic.Conclusion = "inconclusive"
			return diagnostic
		}
	}
	current, reference, hmac := diagnostic.Checks[0].Status, diagnostic.Checks[1].Status, diagnostic.Checks[2].Status
	// 同协议的两次结果不一致时，仍可能存在传输或短暂上游变化，不能归因于算法。
	if (o.config["signatureAlgorithm"] == OKPaySignatureHMACSHA256 && current != hmac) ||
		(o.config["signatureAlgorithm"] == OKPaySignatureLegacyMD5 && current != reference) {
		diagnostic.Conclusion = "inconclusive"
		return diagnostic
	}
	switch {
	case current == okpayCheckAuthenticated && reference == okpayCheckAuthenticated && hmac == okpayCheckAuthenticated:
		diagnostic.Conclusion = "both_authenticated"
	case hmac == okpayCheckAuthenticated && reference == okpayCheckRejected:
		diagnostic.Conclusion = "hmac_only_authenticated"
	case reference == okpayCheckAuthenticated && hmac == okpayCheckRejected:
		diagnostic.Conclusion = "legacy_only_authenticated"
	case current == okpayCheckRejected && reference == okpayCheckRejected && hmac == okpayCheckRejected:
		diagnostic.Conclusion = "both_rejected"
	default:
		// 网络或格式异常不能用来推断传输方式或商户凭据是否正确。
		diagnostic.Conclusion = "inconclusive"
	}
	return diagnostic
}

func okpayHasValidBalanceData(fields okpayArray) bool {
	value, found := fields.get("data")
	data, ok := value.(okpayArray)
	if !found || !ok || len(data) == 0 {
		return false
	}
	foundBalance := false
	for _, currency := range []string{"usdt", "trx", "cny"} {
		if _, found := data.get(currency); !found {
			continue
		}
		amount, err := okpayRequiredScalar(data, currency)
		if err != nil || len(amount) > 32 || !okpayAmountPattern.MatchString(amount) {
			return false
		}
		foundBalance = true
	}
	return foundBalance
}

func okpayDiagnosticMessage(status string) string {
	switch status {
	case okpayCheckAuthenticated:
		return "只读商户认证通过"
	case okpayCheckRejected:
		return "上游返回业务拒绝，请结合具体分类检查签名协议、商户配置或上游限制"
	case okpayCheckInvalidResponse:
		return "上游响应缺少可确认认证成功的有效数据"
	default:
		return "只读请求失败，请检查网络或上游服务状态"
	}
}
