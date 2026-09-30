package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	"github.com/stretchr/testify/require"
)

type paymentTRC20TestRepository struct {
	PaymentTRC20Repository
	intents     []PaymentTRC20Intent
	input       PaymentTRC20CreateInput
	baseUnits   int64
	checked     []int64
	credited    []int64
	allocated   *PaymentTRC20Intent
	extraUnits  int64
	allocateErr error
}

func (r *paymentTRC20TestRepository) Allocate(_ context.Context, input PaymentTRC20CreateInput, units int64) (*PaymentTRC20Intent, error) {
	r.input, r.baseUnits = input, units
	if r.allocateErr != nil {
		return nil, r.allocateErr
	}
	if r.allocated != nil {
		return r.allocated, nil
	}
	return &PaymentTRC20Intent{OrderID: input.OrderID, BaseAmount: input.BaseAmount, AmountUnits: units + r.extraUnits}, nil
}
func (r *paymentTRC20TestRepository) Get(_ context.Context, orderID int64) (*PaymentTRC20Intent, error) {
	for _, intent := range r.intents {
		if intent.OrderID == orderID {
			copy := intent
			return &copy, nil
		}
	}
	return nil, ErrTRC20IntentNotFound
}
func (r *paymentTRC20TestRepository) ListForPoll(context.Context, int) ([]PaymentTRC20Intent, error) {
	return r.intents, nil
}
func (r *paymentTRC20TestRepository) MarkChecked(_ context.Context, id int64) error {
	r.checked = append(r.checked, id)
	return nil
}
func (r *paymentTRC20TestRepository) MarkCredited(_ context.Context, id int64) error {
	r.credited = append(r.credited, id)
	return nil
}

func TestTRC20AllocateUsesTwoDecimalTransferAndKeepsLedgerBaseAmount(t *testing.T) {
	repo := &paymentTRC20TestRepository{}
	s := NewPaymentTRC20Service(repo)
	input := PaymentTRC20CreateInput{OrderID: 1, OutTradeNo: "order", ProviderInstanceID: "1", WalletAddress: "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7", BaseAmount: 12.34, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	intent, err := s.Allocate(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, int64(12340000), repo.baseUnits)
	require.Equal(t, "12.34", intent.ExactAmount)
	require.Equal(t, 12.34, intent.BaseAmount)
	require.Equal(t, "TRC20", intent.Network)
	repo.extraUnits = payment.TRC20AmountStepUnits
	intent, err = s.Allocate(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "12.35", intent.ExactAmount)
	require.Equal(t, 12.34, intent.BaseAmount, "识别金额差额不能变更充值余额或汇率快照中的基础应付")
	for _, amount := range []float64{0, -1, 12.345, math.NaN(), math.Inf(1), 1e30} {
		input.BaseAmount = amount
		_, err = s.Allocate(context.Background(), input)
		require.Error(t, err)
	}
}

func TestTRC20AllocationDoesNotRoundExistingMicroAmount(t *testing.T) {
	start := time.Now().Truncate(time.Millisecond)
	input := PaymentTRC20CreateInput{OrderID: 7, OutTradeNo: "legacy", ProviderInstanceID: "1", WalletAddress: "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7", BaseAmount: 12.34, CreatedAt: start, ExpiresAt: start.Add(time.Hour)}
	repo := &paymentTRC20TestRepository{allocated: &PaymentTRC20Intent{OrderID: 7, BaseAmount: 12.34, AmountUnits: 12340042}}
	intent, err := NewPaymentTRC20Service(repo).Allocate(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, int64(12340042), intent.AmountUnits)
	require.Equal(t, "12.340042", intent.ExactAmount)
	repo.allocateErr = ErrTRC20AmountExhausted
	_, err = NewPaymentTRC20Service(repo).Allocate(context.Background(), input)
	require.ErrorIs(t, err, ErrTRC20AmountExhausted)
	require.ErrorContains(t, err, "0.99 USDT")
}

func TestTRC20SavedAmountFormattingPreservesNewAndLegacyPrecision(t *testing.T) {
	for _, tc := range []struct {
		units int64
		exact string
	}{
		{10000, "0.01"}, {10000000, "10.00"}, {10010000, "10.01"},
		{10990000, "10.99"}, {11000000, "11.00"},
		{10000001, "10.000001"}, {10009999, "10.009999"}, {10010001, "10.010001"},
	} {
		repo := &paymentTRC20TestRepository{intents: []PaymentTRC20Intent{{OrderID: 1, AmountUnits: tc.units}}}
		intent, err := NewPaymentTRC20Service(repo).Get(context.Background(), 1)
		require.NoError(t, err)
		require.Equal(t, tc.exact, intent.ExactAmount)
		require.Equal(t, tc.units, intent.AmountUnits)
	}
}

func TestTRC20ClaimSurvivesRestartAndReturnsOnlyLedgerBaseAmount(t *testing.T) {
	paidAt := time.Now().Add(-time.Minute)
	intent := PaymentTRC20Intent{OrderID: 1, OutTradeNo: "order-1", ProviderInstanceID: "1", WalletAddress: "wallet", BaseAmount: 10, AmountUnits: 10009999, TransactionHash: strings.Repeat("a", 64), TransferredAt: &paidAt}
	s := NewPaymentTRC20Service(&paymentTRC20TestRepository{intents: []PaymentTRC20Intent{intent}})
	notification, err := s.Check(context.Background(), 1, nil)
	require.NoError(t, err)
	require.Equal(t, 10.0, notification.Amount)
	require.Equal(t, "10.009999", notification.Metadata["amount_exact"])
	require.Equal(t, "wallet", notification.Metadata["merchant_id"])
	require.Equal(t, "USDT", notification.Metadata["currency"])
	require.Equal(t, "order-1", notification.Metadata["unique_id"])
	require.Equal(t, intent.TransactionHash, notification.TradeNo)
}

func TestTRC20PollRetainsClaimsOnFulfillmentFailureAndRetriesAfterRestart(t *testing.T) {
	paidAt := time.Now().Add(-time.Hour)
	repo := &paymentTRC20TestRepository{intents: []PaymentTRC20Intent{{OrderID: 1, OutTradeNo: "order-1", BaseAmount: 10, AmountUnits: 10000001, TransactionHash: strings.Repeat("a", 64), TransferredAt: &paidAt}}}
	resolve := func(context.Context, string) (*provider.TRC20, error) {
		t.Fatal("已认领交易的重试不应再访问链上")
		return nil, nil
	}
	failure := errors.New("余额服务暂时不可用")
	count, err := NewPaymentTRC20Service(repo).Poll(context.Background(), resolve, func(context.Context, *payment.PaymentNotification) error { return failure })
	require.ErrorIs(t, err, failure)
	require.Zero(t, count)
	require.Empty(t, repo.credited)
	count, err = NewPaymentTRC20Service(repo).Poll(context.Background(), resolve, func(_ context.Context, n *payment.PaymentNotification) error {
		require.Equal(t, 10.0, n.Amount)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, []int64{1}, repo.credited)
}

func TestTRC20PollDoesNotStarveClaimedOrderAfterAnotherProviderFailure(t *testing.T) {
	paidAt := time.Now()
	repo := &paymentTRC20TestRepository{intents: []PaymentTRC20Intent{{OrderID: 1, ProviderInstanceID: "bad"}, {OrderID: 2, OutTradeNo: "order-2", BaseAmount: 5, AmountUnits: 5000001, TransactionHash: strings.Repeat("b", 64), TransferredAt: &paidAt}}}
	count, err := NewPaymentTRC20Service(repo).Poll(context.Background(), func(context.Context, string) (*provider.TRC20, error) { return nil, errors.New("实例已删除") }, func(context.Context, *payment.PaymentNotification) error { return nil })
	require.Error(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, []int64{1, 2}, repo.checked)
	require.Equal(t, []int64{2}, repo.credited)
}
