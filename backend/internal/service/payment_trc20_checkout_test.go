//go:build unit

package service

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func newTRC20CheckoutFixture(t *testing.T, status string) (okpayIntegrationFixture, *paymentTRC20TestRepository) {
	t.Helper()
	ctx := context.Background()
	fixture := newOKPayIntegrationFixture(t)
	instanceID, err := strconv.ParseInt(psStringValue(fixture.order.ProviderInstanceID), 10, 64)
	require.NoError(t, err)
	const wallet = "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7"
	_, err = fixture.service.entClient.PaymentProviderInstance.UpdateOneID(instanceID).
		SetProviderKey(payment.TypeUSDTTRC20).
		SetName("TRC20 主网收款").
		SetConfig(`{"walletAddress":"` + wallet + `","apiKey":"test-api-key","currency":"USDT"}`).
		SetSupportedTypes(payment.TypeUSDTTRC20).
		SetPaymentMode("qrcode").Save(ctx)
	require.NoError(t, err)
	snapshot := buildPaymentOrderProviderSnapshot(&payment.InstanceSelection{ProviderKey: payment.TypeUSDTTRC20, InstanceID: psStringValue(fixture.order.ProviderInstanceID), PaymentMode: "qrcode", Config: map[string]string{"walletAddress": wallet, "apiKey": "test-api-key", "currency": "USDT"}}, CreateOrderRequest{})
	require.Equal(t, wallet, snapshot["merchant_id"])
	require.Equal(t, "USDT", snapshot["currency"])
	require.NotContains(t, snapshot, "apiKey")
	createdAt := time.Now().Add(-2 * time.Hour).Truncate(time.Millisecond)
	expiresAt := createdAt.Add(time.Hour)
	paidAt := createdAt.Add(10 * time.Minute)
	fixture.order, err = fixture.service.entClient.PaymentOrder.UpdateOneID(fixture.order.ID).
		SetPaymentType(payment.TypeUSDTTRC20).
		SetProviderKey(payment.TypeUSDTTRC20).
		SetPaymentTradeNo("").
		SetProviderSnapshot(snapshot).
		SetStatus(status).
		SetExpiresAt(expiresAt).
		SetUpdatedAt(time.Now().Add(-20 * time.Minute)).Save(ctx)
	require.NoError(t, err)
	// 创建时间在 Ent 中不可变，测试数据库单独回拨以模拟真实历史订单。
	_, err = fixture.service.entClient.ExecContext(ctx, `UPDATE payment_orders SET created_at=? WHERE id=?`, createdAt, fixture.order.ID)
	require.NoError(t, err)
	fixture.order, err = fixture.service.entClient.PaymentOrder.Get(ctx, fixture.order.ID)
	require.NoError(t, err)
	repo := &paymentTRC20TestRepository{intents: []PaymentTRC20Intent{{
		OrderID: fixture.order.ID, OutTradeNo: fixture.order.OutTradeNo, ProviderInstanceID: psStringValue(fixture.order.ProviderInstanceID),
		WalletAddress: wallet, BaseAmount: fixture.order.PayAmount, AmountUnits: 12300042,
		CreatedAt: createdAt, ExpiresAt: expiresAt, TransactionHash: strings.Repeat("a", 64), TransferredAt: &paidAt,
	}}}
	fixture.service.SetTRC20Service(NewPaymentTRC20Service(repo))
	return fixture, repo
}

func TestTRC20CheckoutConfirmedPaymentRecoversTerminalOrdersAndCreditsOnce(t *testing.T) {
	for _, status := range []string{OrderStatusExpired, OrderStatusFailed, OrderStatusCancelled} {
		t.Run(status, func(t *testing.T) {
			fixture, repo := newTRC20CheckoutFixture(t, status)
			ctx := context.Background()
			require.True(t, fixture.order.UpdatedAt.Before(time.Now().Add(-paymentGraceMinutes*time.Minute)))
			notification := trc20Notification(&repo.intents[0])
			require.NotNil(t, notification)
			require.Equal(t, "12.300042", notification.Metadata["amount_exact"])
			// 第一笔恢复已终止账单；后续重复处理同一链上交易不能重复增加余额。
			require.NoError(t, fixture.service.HandlePaymentNotification(ctx, notification, payment.TypeUSDTTRC20))
			require.NoError(t, fixture.service.HandlePaymentNotification(ctx, notification, payment.TypeUSDTTRC20))
			reloaded, err := fixture.service.entClient.PaymentOrder.Get(ctx, fixture.order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusCompleted, reloaded.Status)
			require.Equal(t, repo.intents[0].TransactionHash, reloaded.PaymentTradeNo)
			require.NotNil(t, reloaded.PaidAt)
			require.Equal(t, 12.3, reloaded.PayAmount, "链上微小尾数不能改变两位账本金额")
			require.Equal(t, 12.3, fixture.userRepo.getByIDUser.Balance)
			require.Len(t, fixture.redeemRepo.useCalls, 1)
		})
	}
}

func TestTRC20CheckoutPollWaitsForFulfillmentLeaseThenRetries(t *testing.T) {
	fixture, repo := newTRC20CheckoutFixture(t, OrderStatusRecharging)
	ctx := context.Background()
	_, err := fixture.service.entClient.PaymentOrder.UpdateOneID(fixture.order.ID).
		SetPaymentTradeNo(repo.intents[0].TransactionHash).
		SetPaidAt(*repo.intents[0].TransferredAt).
		SetUpdatedAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	completed, err := fixture.service.ReconcileTRC20Orders(ctx)
	require.Error(t, err)
	require.Zero(t, completed)
	require.Empty(t, repo.credited, "另一个进程正在履约时，不能提前停止链上订单重试")
	require.Zero(t, fixture.userRepo.getByIDUser.Balance)
	require.Empty(t, fixture.redeemRepo.useCalls)
	reloaded, err := fixture.service.entClient.PaymentOrder.Get(ctx, fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRecharging, reloaded.Status)
	_, err = fixture.service.entClient.PaymentOrder.UpdateOneID(fixture.order.ID).
		SetUpdatedAt(time.Now().Add(-paymentFulfillmentLeaseDuration - time.Minute)).Save(ctx)
	require.NoError(t, err)
	completed, err = fixture.service.ReconcileTRC20Orders(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, completed)
	require.Equal(t, []int64{fixture.order.ID}, repo.credited)
	require.Equal(t, 12.3, fixture.userRepo.getByIDUser.Balance)
	require.Len(t, fixture.redeemRepo.useCalls, 1)
	reloaded, err = fixture.service.entClient.PaymentOrder.Get(ctx, fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
}

func TestTRC20CheckoutRejectsMissingMismatchedAndOutOfWindowClaims(t *testing.T) {
	for _, scenario := range []string{"missing_claim", "wrong_hash", "missing_transferred_at", "before_creation", "after_expiry"} {
		t.Run(scenario, func(t *testing.T) {
			fixture, repo := newTRC20CheckoutFixture(t, OrderStatusExpired)
			ctx := context.Background()
			notification := trc20Notification(&repo.intents[0])
			switch scenario {
			case "missing_claim":
				repo.intents = nil
			case "wrong_hash":
				notification.TradeNo = strings.Repeat("b", 64)
			case "missing_transferred_at":
				repo.intents[0].TransferredAt = nil
			case "before_creation":
				outside := repo.intents[0].CreatedAt.Add(-time.Millisecond)
				repo.intents[0].TransferredAt = &outside
			case "after_expiry":
				outside := repo.intents[0].ExpiresAt.Add(time.Millisecond)
				repo.intents[0].TransferredAt = &outside
			}
			require.Error(t, fixture.service.HandlePaymentNotification(ctx, notification, payment.TypeUSDTTRC20))
			reloaded, err := fixture.service.entClient.PaymentOrder.Get(ctx, fixture.order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusExpired, reloaded.Status)
			require.Empty(t, reloaded.PaymentTradeNo)
			require.Nil(t, reloaded.PaidAt)
			require.Zero(t, fixture.userRepo.getByIDUser.Balance)
			require.Empty(t, fixture.redeemRepo.useCalls)
		})
	}
}
