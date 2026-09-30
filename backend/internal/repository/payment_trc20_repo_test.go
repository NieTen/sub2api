package repository

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func trc20RepoTestRows(start time.Time, hash string) *sqlmock.Rows {
	return trc20RepoTestAmountRows(start, hash, 10000001)
}

func trc20RepoTestAmountRows(start time.Time, hash string, amountUnits int64) *sqlmock.Rows {
	var transferred any
	if hash != "" {
		transferred = start.Add(time.Minute)
	}
	return sqlmock.NewRows([]string{"order_id", "out_trade_no", "provider_instance_id", "wallet_address", "base_amount", "amount_units", "created_at", "expires_at", "transaction_hash", "transferred_at", "credited"}).AddRow(1, "order-1", "2", "wallet", 10, amountUnits, start, start.Add(time.Hour), hash, transferred, false)
}

func TestTRC20AllocationSerializesWalletAndPermanentlyExcludesUsedAmounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := NewPaymentTRC20Repository(db)
	start := time.Now().Truncate(time.Millisecond)
	input := service.PaymentTRC20CreateInput{OrderID: 1, OutTradeNo: "order-1", ProviderInstanceID: "2", WalletAddress: "wallet", BaseAmount: 10, CreatedAt: start, ExpiresAt: start.Add(time.Hour)}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM payment_orders .*provider_key='usdt_trc20'.*FOR UPDATE`).WithArgs(input.OrderID, input.OutTradeNo, input.BaseAmount, input.ProviderInstanceID, input.CreatedAt, input.ExpiresAt).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery(`SELECT .* FROM payment_trc20_intents WHERE order_id=\$1`).WithArgs(int64(1)).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1,20260930))`)).WithArgs("wallet").WillReturnResult(sqlmock.NewResult(0, 1))
	// 不按过期状态回收尾数，否则旧订单迟到的转账会落入另一用户新订单。
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT $1::bigint+n*$3::bigint FROM generate_series(0,$4::bigint) AS n WHERE NOT EXISTS (SELECT 1 FROM payment_trc20_intents i WHERE i.wallet_address=$2 AND i.amount_units=$1::bigint+n*$3::bigint) ORDER BY n LIMIT 1`)).WithArgs(int64(10000000), "wallet", int64(10000), int64(99)).WillReturnRows(sqlmock.NewRows([]string{"amount_units"}).AddRow(10000000))
	mock.ExpectQuery(`INSERT INTO payment_trc20_intents.*RETURNING`).WithArgs(input.OrderID, input.OutTradeNo, input.ProviderInstanceID, input.WalletAddress, input.BaseAmount, int64(10000000), input.CreatedAt, input.ExpiresAt).WillReturnRows(trc20RepoTestAmountRows(start, "", 10000000))
	mock.ExpectCommit()
	intent, err := r.Allocate(context.Background(), input, 10000000)
	require.NoError(t, err)
	require.Equal(t, int64(10000000), intent.AmountUnits)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTRC20AllocationReturnsLegacyIntentWithoutReallocating(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	start := time.Now().Truncate(time.Millisecond)
	input := service.PaymentTRC20CreateInput{OrderID: 1, OutTradeNo: "order-1", ProviderInstanceID: "2", WalletAddress: "wallet", BaseAmount: 10, CreatedAt: start, ExpiresAt: start.Add(time.Hour)}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM payment_orders .*FOR UPDATE`).WithArgs(input.OrderID, input.OutTradeNo, input.BaseAmount, input.ProviderInstanceID, input.CreatedAt, input.ExpiresAt).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery(`SELECT .* FROM payment_trc20_intents WHERE order_id=\$1`).WithArgs(int64(1)).WillReturnRows(trc20RepoTestRows(start, ""))
	mock.ExpectCommit()
	intent, err := NewPaymentTRC20Repository(db).Allocate(context.Background(), input, 10000000)
	require.NoError(t, err)
	require.Equal(t, int64(10000001), intent.AmountUnits)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTRC20AllocationExhaustionRollsBackWithoutIncreasingLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	start := time.Now().Truncate(time.Millisecond)
	input := service.PaymentTRC20CreateInput{OrderID: 1, OutTradeNo: "order-1", ProviderInstanceID: "2", WalletAddress: "wallet", BaseAmount: 10, CreatedAt: start, ExpiresAt: start.Add(time.Hour)}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM payment_orders .*FOR UPDATE`).WithArgs(input.OrderID, input.OutTradeNo, input.BaseAmount, input.ProviderInstanceID, input.CreatedAt, input.ExpiresAt).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery(`SELECT .* FROM payment_trc20_intents WHERE order_id=\$1`).WithArgs(int64(1)).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1,20260930))`)).WithArgs("wallet").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM generate_series\(0,\$4::bigint\) AS n WHERE NOT EXISTS`).WithArgs(int64(10000000), "wallet", int64(10000), int64(99)).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	_, err = NewPaymentTRC20Repository(db).Allocate(context.Background(), input, 10000000)
	require.ErrorIs(t, err, service.ErrTRC20AmountExhausted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTRC20ClaimValidatesExactAmountAddressAndWindowBeforeWriting(t *testing.T) {
	start := time.Now().Truncate(time.Millisecond)
	for _, change := range []func(*payment.TRC20Transfer){
		func(transfer *payment.TRC20Transfer) { transfer.AmountUnits++ },
		func(transfer *payment.TRC20Transfer) { transfer.WalletAddress = "other" },
		func(transfer *payment.TRC20Transfer) { transfer.BlockTimestamp = start.Add(-time.Second) },
		func(transfer *payment.TRC20Transfer) { transfer.BlockTimestamp = start.Add(2 * time.Hour) },
	} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		transfer := payment.TRC20Transfer{TransactionHash: strings.Repeat("a", 64), WalletAddress: "wallet", AmountUnits: 10000001, BlockTimestamp: start.Add(time.Minute)}
		change(&transfer)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT .* FROM payment_trc20_intents WHERE order_id=\$1 FOR UPDATE`).WithArgs(int64(1)).WillReturnRows(trc20RepoTestRows(start, ""))
		mock.ExpectRollback()
		_, err = NewPaymentTRC20Repository(db).ClaimTransfer(context.Background(), 1, transfer)
		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
		db.Close()
	}
}

func TestTRC20DuplicateTransactionCannotBeCreditedToAnotherOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	start := time.Now().Truncate(time.Millisecond)
	transfer := payment.TRC20Transfer{TransactionHash: strings.Repeat("a", 64), WalletAddress: "wallet", AmountUnits: 10000001, BlockTimestamp: start.Add(time.Minute)}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM payment_trc20_intents WHERE order_id=\$1 FOR UPDATE`).WithArgs(int64(1)).WillReturnRows(trc20RepoTestRows(start, ""))
	mock.ExpectExec(`UPDATE payment_trc20_intents SET transaction_hash=\$2,transferred_at=\$3 WHERE order_id=\$1 AND transaction_hash IS NULL`).WithArgs(int64(1), transfer.TransactionHash, transfer.BlockTimestamp).WillReturnError(&pq.Error{Code: "23505"})
	mock.ExpectRollback()
	_, err = NewPaymentTRC20Repository(db).ClaimTransfer(context.Background(), 1, transfer)
	require.ErrorIs(t, err, service.ErrTRC20TransferClaimed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTRC20RepeatedClaimForSameOrderIsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	start := time.Now().Truncate(time.Millisecond)
	transfer := payment.TRC20Transfer{TransactionHash: strings.Repeat("a", 64), WalletAddress: "wallet", AmountUnits: 10000001, BlockTimestamp: start.Add(time.Minute)}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM payment_trc20_intents WHERE order_id=\$1 FOR UPDATE`).WithArgs(int64(1)).WillReturnRows(trc20RepoTestRows(start, transfer.TransactionHash))
	mock.ExpectCommit()
	intent, err := NewPaymentTRC20Repository(db).ClaimTransfer(context.Background(), 1, transfer)
	require.NoError(t, err)
	require.Equal(t, transfer.TransactionHash, intent.TransactionHash)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTRC20PollIncludesDelayedConfirmationAndUnfulfilledClaims(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`SELECT .* FROM payment_trc20_intents WHERE credited_at IS NULL AND \(transaction_hash IS NOT NULL OR expires_at>NOW\(\)-INTERVAL '30 minutes'\) ORDER BY checked_at ASC NULLS FIRST,order_id LIMIT \$1`).WithArgs(20).WillReturnRows(trc20RepoTestRows(time.Now(), ""))
	intents, err := NewPaymentTRC20Repository(db).ListForPoll(context.Background(), 20)
	require.NoError(t, err)
	require.Len(t, intents, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}
