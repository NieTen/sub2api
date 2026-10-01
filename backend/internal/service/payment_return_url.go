package service

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 在签发恢复令牌前确定最终返回地址，避免浏览器的 HTTP/IP 地址覆盖管理员配置。
func resolveCreateOrderReturnURL(req CreateOrderRequest, sel *payment.InstanceSelection) (string, error) {
	if sel == nil || sel.ProviderKey != payment.TypeOKPay || strings.TrimSpace(sel.Config["returnUrl"]) == "" {
		return CanonicalizeReturnURL(req.ReturnURL, req.SrcHost, req.SrcURL)
	}

	// 配置属于管理员信任目标，可以与当前浏览器域名不同，但必须先通过 HTTPS 校验。
	raw := sel.Config["returnUrl"]
	invalid := func(message string) error {
		return infraerrors.ServiceUnavailable("PAYMENT_PROVIDER_MISCONFIGURED", "OKPay 实例配置的返回地址（returnUrl）无效："+message).
			WithMetadata(map[string]string{"provider": payment.TypeOKPay, "instance_id": sel.InstanceID, "field": "return_url", "source": "provider_config"})
	}
	if strings.IndexFunc(raw, unicode.IsControl) >= 0 || strings.Contains(raw, "\\") {
		return "", invalid("不能包含控制字符或反斜杠，请编辑同步跳转地址")
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return "", invalid("必须填写完整有效的 HTTPS 地址，请编辑同步跳转地址")
	}
	if parsed.Scheme != "https" {
		return "", invalid("必须使用 https:// 开头的完整地址，请编辑同步跳转地址")
	}
	if parsed.User != nil {
		return "", invalid("不能包含账号密码，请编辑同步跳转地址")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", invalid("查询参数格式不正确，请编辑同步跳转地址")
	}
	// 保留反代子路径和普通参数，订单参数统一由当前订单生成，不能沿用旧恢复令牌。
	for _, key := range []string{"order_id", "out_trade_no", "resume_token", "status"} {
		query.Del(key)
	}
	parsed.RawQuery = query.Encode()
	parsed.Fragment, parsed.RawFragment = "", ""
	return parsed.String(), nil
}
