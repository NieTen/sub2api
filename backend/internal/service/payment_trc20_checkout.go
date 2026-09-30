package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
)

// PaymentTransferDetails 保存用户实际转账需要的信息，金额使用字符串避免丢失尾数。
type PaymentTransferDetails struct {
	PaymentNetwork     string `json:"payment_network,omitempty"`
	PaymentAddress     string `json:"payment_address,omitempty"`
	PaymentAmountExact string `json:"payment_amount_exact,omitempty"`
}

func PaymentOrderTransferDetails(order *dbent.PaymentOrder) PaymentTransferDetails {
	if order == nil || psSnapshotStringValue(order.ProviderSnapshot["provider_key"]) != payment.TypeUSDTTRC20 {
		return PaymentTransferDetails{}
	}
	return PaymentTransferDetails{
		PaymentNetwork:     psSnapshotStringValue(order.ProviderSnapshot["payment_network"]),
		PaymentAddress:     psSnapshotStringValue(order.ProviderSnapshot["payment_address"]),
		PaymentAmountExact: psSnapshotStringValue(order.ProviderSnapshot["payment_amount_exact"]),
	}
}

func (s *PaymentService) createTRC20Checkout(ctx context.Context, order *dbent.PaymentOrder, sel *payment.InstanceSelection) (*payment.CreatePaymentResponse, error) {
	if s.trc20Svc == nil {
		return nil, fmt.Errorf("TRC20 收款服务未初始化")
	}
	intent, err := s.trc20Svc.Allocate(ctx, PaymentTRC20CreateInput{
		OrderID: order.ID, OutTradeNo: order.OutTradeNo, ProviderInstanceID: sel.InstanceID,
		WalletAddress: strings.TrimSpace(sel.Config["walletAddress"]), BaseAmount: order.PayAmount,
		CreatedAt: order.CreatedAt, ExpiresAt: order.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}
	if order.ProviderSnapshot == nil {
		order.ProviderSnapshot = make(map[string]any)
	}
	order.ProviderSnapshot["payment_network"] = "TRC20"
	order.ProviderSnapshot["payment_address"] = intent.WalletAddress
	order.ProviderSnapshot["payment_amount_exact"] = intent.ExactAmount
	return &payment.CreatePaymentResponse{QRCode: intent.WalletAddress, Currency: "USDT"}, nil
}

func (s *PaymentService) checkTRC20Paid(ctx context.Context, order *dbent.PaymentOrder, client *provider.TRC20) string {
	if s.trc20Svc == nil {
		return ""
	}
	notification, err := s.trc20Svc.Check(ctx, order.ID, client)
	if err != nil || notification == nil {
		return ""
	}
	if err := s.HandlePaymentNotification(ctx, notification, payment.TypeUSDTTRC20); err != nil {
		return ""
	}
	return checkPaidResultAlreadyPaid
}

// ReconcileTRC20Orders 复用后台领导者锁轮询，用户离开收银台后仍可自动入账。
func (s *PaymentService) ReconcileTRC20Orders(ctx context.Context) (int, error) {
	if s.trc20Svc == nil {
		return 0, nil
	}
	return s.trc20Svc.Poll(ctx, func(ctx context.Context, instanceID string) (*provider.TRC20, error) {
		id, err := strconv.ParseInt(instanceID, 10, 64)
		if err != nil {
			return nil, err
		}
		inst, err := s.entClient.PaymentProviderInstance.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		prov, err := s.createProviderFromInstance(ctx, inst)
		if err != nil {
			return nil, err
		}
		client, ok := prov.(*provider.TRC20)
		if !ok {
			return nil, fmt.Errorf("TRC20 订单服务商类型不匹配")
		}
		return client, nil
	}, func(ctx context.Context, notification *payment.PaymentNotification) error {
		if err := s.HandlePaymentNotification(ctx, notification, payment.TypeUSDTTRC20); err != nil {
			return err
		}
		// 另一个进程持有履约租约时，通用处理可能暂时返回成功；仅完成订单才停止轮询。
		order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNo(notification.OrderID)).Only(ctx)
		if err != nil {
			return err
		}
		if order.Status != OrderStatusCompleted {
			return fmt.Errorf("TRC20 付款已确认，等待订单履约完成")
		}
		return nil
	})
}

func (s *PaymentService) validateTRC20PaymentClaim(ctx context.Context, order *dbent.PaymentOrder, tradeNo string) error {
	if s.trc20Svc == nil {
		return fmt.Errorf("TRC20 收款服务未初始化")
	}
	intent, err := s.trc20Svc.Get(ctx, order.ID)
	if err != nil {
		return err
	}
	if intent == nil || tradeNo == "" || intent.TransactionHash != tradeNo || intent.OutTradeNo != order.OutTradeNo ||
		intent.TransferredAt == nil || intent.TransferredAt.Before(intent.CreatedAt) || intent.TransferredAt.After(intent.ExpiresAt) {
		return fmt.Errorf("TRC20 交易缺少已确认的链上收款记录")
	}
	return nil
}
