package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

var (
	ErrTRC20IntentNotFound  = errors.New("TRC20 收款订单不存在")
	ErrTRC20AmountExhausted = infraerrors.Conflict("TRC20_AMOUNT_EXHAUSTED", "此收款地址从基础金额到增加 0.99 USDT 的两位小数金额均已使用，请更换订单金额或收款地址")
	ErrTRC20TransferClaimed = errors.New("该 TRC20 交易已经归属于其他订单")
)

type PaymentTRC20CreateInput struct {
	OrderID            int64
	OutTradeNo         string
	ProviderInstanceID string
	WalletAddress      string
	BaseAmount         float64
	CreatedAt          time.Time
	ExpiresAt          time.Time
}

type PaymentTRC20Intent struct {
	OrderID            int64
	OutTradeNo         string
	ProviderInstanceID string
	WalletAddress      string
	BaseAmount         float64
	AmountUnits        int64
	ExactAmount        string
	Network            string
	CreatedAt          time.Time
	ExpiresAt          time.Time
	TransactionHash    string
	TransferredAt      *time.Time
	Credited           bool
}

type PaymentTRC20Repository interface {
	Allocate(context.Context, PaymentTRC20CreateInput, int64) (*PaymentTRC20Intent, error)
	Get(context.Context, int64) (*PaymentTRC20Intent, error)
	ClaimTransfer(context.Context, int64, payment.TRC20Transfer) (*PaymentTRC20Intent, error)
	ListForPoll(context.Context, int) ([]PaymentTRC20Intent, error)
	MarkChecked(context.Context, int64) error
	MarkCredited(context.Context, int64) error
}

type PaymentTRC20Service struct{ repo PaymentTRC20Repository }

func NewPaymentTRC20Service(repo PaymentTRC20Repository) *PaymentTRC20Service {
	return &PaymentTRC20Service{repo: repo}
}

func (s *PaymentService) SetTRC20Service(trc20 *PaymentTRC20Service) { s.trc20Svc = trc20 }

func (s *PaymentTRC20Service) Allocate(ctx context.Context, input PaymentTRC20CreateInput) (*PaymentTRC20Intent, error) {
	if input.OrderID <= 0 || input.OutTradeNo == "" || input.ProviderInstanceID == "" || input.CreatedAt.IsZero() || !input.ExpiresAt.After(input.CreatedAt) || input.BaseAmount <= 0 || math.IsNaN(input.BaseAmount) || math.IsInf(input.BaseAmount, 0) {
		return nil, errors.New("TRC20 收款订单参数无效")
	}
	if err := provider.ValidateTRC20WalletAddress(input.WalletAddress); err != nil {
		return nil, err
	}
	amount := decimal.NewFromFloat(input.BaseAmount)
	if !amount.Equal(amount.Truncate(2)) {
		return nil, errors.New("TRC20 账单金额必须保持两位精度")
	}
	units := amount.Shift(6)
	if units.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return nil, errors.New("TRC20 金额超出支持范围")
	}
	// 由仓储选择最小未用的整分金额，事务与永久唯一索引保证跨实例不冲突。
	intent, err := s.repo.Allocate(ctx, input, units.IntPart())
	if err == nil {
		completeTRC20Intent(intent)
	}
	return intent, err
}

func completeTRC20Intent(intent *PaymentTRC20Intent) {
	if intent == nil {
		return
	}
	intent.Network = "TRC20"
	whole := strconv.FormatInt(intent.AmountUnits/payment.TRC20USDTUnitsPerToken, 10)
	fraction := intent.AmountUnits % payment.TRC20USDTUnitsPerToken
	if intent.AmountUnits%payment.TRC20AmountStepUnits == 0 {
		intent.ExactAmount = whole + "." + fmt.Sprintf("%02d", fraction/payment.TRC20AmountStepUnits)
		return
	}
	// 历史订单保留完整六位尾数，禁止把旧订单四舍五入成新的收款金额。
	intent.ExactAmount = whole + "." + fmt.Sprintf("%06d", fraction)
}

func (s *PaymentTRC20Service) Get(ctx context.Context, orderID int64) (*PaymentTRC20Intent, error) {
	intent, err := s.repo.Get(ctx, orderID)
	if err == nil {
		completeTRC20Intent(intent)
	}
	return intent, err
}

func trc20Notification(intent *PaymentTRC20Intent) *payment.PaymentNotification {
	if intent == nil || intent.TransactionHash == "" || intent.TransferredAt == nil {
		return nil
	}
	completeTRC20Intent(intent)
	return &payment.PaymentNotification{TradeNo: intent.TransactionHash, OrderID: intent.OutTradeNo, Amount: intent.BaseAmount, Status: payment.NotificationStatusSuccess, Metadata: map[string]string{"merchant_id": intent.WalletAddress, "currency": "USDT", "unique_id": intent.OutTradeNo, "network": "TRC20", "amount_exact": intent.ExactAmount}}
}

func (s *PaymentTRC20Service) Check(ctx context.Context, orderID int64, client *provider.TRC20) (*payment.PaymentNotification, error) {
	intent, err := s.Get(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if notification := trc20Notification(intent); notification != nil {
		return notification, nil
	}
	if client == nil {
		return nil, errors.New("TRC20 查询客户端未配置")
	}
	transfer, err := client.FindConfirmedTransfer(ctx, intent.WalletAddress, intent.AmountUnits, intent.CreatedAt, intent.ExpiresAt)
	if err != nil || transfer == nil {
		return nil, err
	}
	intent, err = s.repo.ClaimTransfer(ctx, orderID, *transfer)
	if err != nil {
		return nil, err
	}
	return trc20Notification(intent), nil
}

// Poll 基于持久化意图轮询，过期后继续等待 30 分钟的链上确认；已认领未履约订单持续重试。
func (s *PaymentTRC20Service) Poll(ctx context.Context, resolve func(context.Context, string) (*provider.TRC20, error), fulfill func(context.Context, *payment.PaymentNotification) error) (int, error) {
	if resolve == nil || fulfill == nil {
		return 0, errors.New("TRC20 轮询缺少服务商或入账处理器")
	}
	intents, err := s.repo.ListForPoll(ctx, 20)
	if err != nil {
		return 0, err
	}
	completed := 0
	var firstErr error
	for _, intent := range intents {
		err = nil
		if err = ctx.Err(); err != nil {
			return completed, err
		}
		// 每轮先更新检查时间，失败订单不会永久抢占前面的轮询名额。
		if err = s.repo.MarkChecked(ctx, intent.OrderID); err != nil {
			return completed, err
		}
		var notification *payment.PaymentNotification
		if intent.TransactionHash != "" {
			notification = trc20Notification(&intent)
		} else {
			var client *provider.TRC20
			client, err = resolve(ctx, intent.ProviderInstanceID)
			if err == nil {
				notification, err = s.Check(ctx, intent.OrderID, client)
			}
		}
		if err == nil && notification != nil {
			err = fulfill(ctx, notification)
			if err == nil {
				err = s.repo.MarkCredited(ctx, intent.OrderID)
			}
			if err == nil {
				completed++
			}
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return completed, firstErr
}
