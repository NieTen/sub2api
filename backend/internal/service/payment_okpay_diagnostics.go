package service

import (
	"context"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// PaymentOKPayDiagnostic 仅返回认证结果，不包含商户凭据、签名和余额。
type PaymentOKPayDiagnostic struct {
	ProviderInstanceID int64  `json:"provider_instance_id"`
	ProviderName       string `json:"provider_name"`
	*provider.OKPayAuthenticationDiagnostic
}

// DiagnoseOKPayProvider 使用同一份已保存配置完成两种传输方式的只读认证对照。
func (s *PaymentConfigService) DiagnoseOKPayProvider(ctx context.Context, id int64) (*PaymentOKPayDiagnostic, error) {
	if id <= 0 {
		return nil, infraerrors.BadRequest("INVALID_PROVIDER_ID", "服务商实例编号无效")
	}
	instance, err := s.entClient.PaymentProviderInstance.Get(ctx, id)
	if dbent.IsNotFound(err) {
		return nil, infraerrors.NotFound("PROVIDER_NOT_FOUND", "支付服务商不存在")
	}
	if err != nil {
		return nil, err
	}
	if instance.ProviderKey != payment.TypeOKPay {
		return nil, infraerrors.BadRequest("INVALID_PROVIDER_TYPE", "此诊断仅支持 OKPay 服务商")
	}
	config, err := s.decryptConfig(instance.Config)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_PROVIDER_CONFIG", "无法读取已保存的 OKPay 配置，请重新保存后诊断")
	}
	client, err := provider.NewOKPay(strconv.FormatInt(instance.ID, 10), config)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_PROVIDER_CONFIG", "已保存的 OKPay 配置不完整或格式无效，请检查后重新保存")
	}
	// 诊断固定查询余额接口，仅使用成功状态，既不创建订单也不返回余额。
	probeContext, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	return &PaymentOKPayDiagnostic{
		ProviderInstanceID:            instance.ID,
		ProviderName:                  instance.Name,
		OKPayAuthenticationDiagnostic: client.DiagnoseAuthentication(probeContext),
	}, nil
}
