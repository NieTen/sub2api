package service

import (
	"net/url"
	"strings"
	"unicode"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 图标仅由浏览器加载；公开响应只返回经过校验的地址，不返回服务商配置。
func normalizePaymentMethodIconURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 || strings.Contains(raw, "\\") || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Fragment != "" {
		return ""
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") && parsed.Host == "" && parsed.Scheme == "" {
		return raw
	}
	if parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.Opaque == "" {
		return raw
	}
	return ""
}

func validatePaymentMethodIcon(providerKey string, config map[string]string) error {
	if providerKey != payment.TypeOKPay || strings.TrimSpace(config["iconUrl"]) == "" {
		return nil
	}
	if normalizePaymentMethodIconURL(config["iconUrl"]) == "" {
		return infraerrors.BadRequest("VALIDATION_ERROR", "OKPay 图标必须是 HTTPS 图片地址或以 / 开头的站内路径，不能包含账号密码或片段")
	}
	return nil
}

func (s *PaymentConfigService) pcAggregateMethodIconURL(method string, instances []*dbent.PaymentProviderInstance) string {
	if method != payment.TypeOKPay || s == nil {
		return ""
	}
	var selected *dbent.PaymentProviderInstance
	iconURL := ""
	for _, inst := range instances {
		if inst == nil || !inst.Enabled || inst.ProviderKey != payment.TypeOKPay {
			continue
		}
		config, err := s.decryptConfig(inst.Config)
		if err != nil {
			continue
		}
		candidate := normalizePaymentMethodIconURL(config["iconUrl"])
		if candidate == "" {
			continue
		}
		// 同一方式聚合多个实例时，按后台排序及编号稳定选择首个已配置图标。
		if selected == nil || inst.SortOrder < selected.SortOrder || (inst.SortOrder == selected.SortOrder && inst.ID < selected.ID) {
			selected, iconURL = inst, candidate
		}
	}
	return iconURL
}
