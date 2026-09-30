package provider

import (
	"context"
	"errors"
)

const (
	okpayTransportCurrent      = "current"
	okpayTransportPHPReference = "php_reference"
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
	Mode    string `json:"mode"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// okpayDiagnosticFailure 保留原有错误链，诊断接口只使用分类，绝不输出底层错误文本。
type okpayDiagnosticFailure struct {
	status string
	cause  error
}

func (e *okpayDiagnosticFailure) Error() string { return e.cause.Error() }
func (e *okpayDiagnosticFailure) Unwrap() error { return e.cause }

func okpayClassifiedFailure(status string, cause error) error {
	return &okpayDiagnosticFailure{status: status, cause: cause}
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

// DiagnoseAuthentication 对同一份已保存配置执行两次只读余额请求，不创建订单或重试。
func (o *OKPay) DiagnoseAuthentication(ctx context.Context) *OKPayAuthenticationDiagnostic {
	diagnostic := &OKPayAuthenticationDiagnostic{Checks: make([]OKPayAuthenticationCheck, 0, 2)}
	for _, mode := range []string{okpayTransportCurrent, okpayTransportPHPReference} {
		fields, err := o.postWithTransport(ctx, "/balance", nil, mode)
		status := okpayCheckAuthenticated
		if err != nil {
			status = okpayCheckRequestFailed
			var failure *okpayDiagnosticFailure
			if errors.As(err, &failure) {
				status = failure.status
			}
		} else if !okpayHasValidBalanceData(fields) {
			status = okpayCheckInvalidResponse
		}
		diagnostic.Checks = append(diagnostic.Checks, OKPayAuthenticationCheck{Mode: mode, Status: status, Message: okpayDiagnosticMessage(status)})
	}
	current, reference := diagnostic.Checks[0].Status, diagnostic.Checks[1].Status
	switch {
	case current == okpayCheckAuthenticated && reference == okpayCheckAuthenticated:
		diagnostic.Conclusion = "both_authenticated"
	case current == okpayCheckRejected && reference == okpayCheckAuthenticated:
		diagnostic.Conclusion = "php_only_authenticated"
	case current == okpayCheckAuthenticated && reference == okpayCheckRejected:
		diagnostic.Conclusion = "current_only_authenticated"
	case current == okpayCheckRejected && reference == okpayCheckRejected:
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
		return "上游未接受本次认证，请检查商户配置或上游访问限制"
	case okpayCheckInvalidResponse:
		return "上游响应缺少可确认认证成功的有效数据"
	default:
		return "只读请求失败，请检查网络或上游服务状态"
	}
}
