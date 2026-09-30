package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

const trc20PostgresWallet = "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7"

// 仅使用显式测试连接与随机 schema，不访问业务表，也不输出连接凭据。
func trc20Postgres(t *testing.T) (*paymentTRC20Repository, context.Context) {
	t.Helper()
	var dsn string
	for _, name := range []string{"PAYMENT_TRC20_TEST_DSN", "COMMUNITY_TEST_DSN", "MODEL_DETECTION_TEST_DSN"} {
		if dsn = os.Getenv(name); dsn != "" {
			break
		}
	}
	if dsn == "" {
		t.Skip("未设置显式测试数据库连接，跳过 TRC20 PostgreSQL 金额分配回归")
	}
	u, err := url.Parse(dsn)
	require.True(t, err == nil, "测试数据库连接格式无效")
	require.Equal(t, "postgres", u.Scheme)
	require.NotEmpty(t, u.Host)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	schema := "trc20_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		_, err := admin.ExecContext(cleanup, "DROP SCHEMA "+pq.QuoteIdentifier(schema)+" CASCADE")
		require.NoError(t, err)
	})
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(16)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var currentSchema string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&currentSchema))
	require.Equal(t, schema, currentSchema)
	_, err = db.ExecContext(ctx, `CREATE TABLE payment_orders (
		id BIGINT PRIMARY KEY, out_trade_no VARCHAR(128) NOT NULL,
		provider_key VARCHAR(32) NOT NULL, pay_amount NUMERIC(20,2) NOT NULL,
		provider_instance_id VARCHAR(128) NOT NULL,
		created_at TIMESTAMPTZ NOT NULL, expires_at TIMESTAMPTZ NOT NULL
	)`)
	require.NoError(t, err)
	ddl, err := migrations.FS.ReadFile("245_payment_trc20_intents.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(ddl))
	require.NoError(t, err, "直接使用已发布迁移，验证两位新单与六位旧单共享现有表结构")
	return &paymentTRC20Repository{db: db}, ctx
}

func trc20PostgresOrder(t *testing.T, ctx context.Context, repo *paymentTRC20Repository, id int64, instanceID, wallet string, baseUnits int64) service.PaymentTRC20CreateInput {
	t.Helper()
	start := time.Now().Add(-time.Minute).Truncate(time.Microsecond)
	input := service.PaymentTRC20CreateInput{
		OrderID: id, OutTradeNo: fmt.Sprintf("trc20-%d", id), ProviderInstanceID: instanceID,
		WalletAddress: wallet, BaseAmount: float64(baseUnits) / float64(payment.TRC20USDTUnitsPerToken),
		CreatedAt: start, ExpiresAt: start.Add(time.Hour),
	}
	_, err := repo.db.ExecContext(ctx, `INSERT INTO payment_orders(id,out_trade_no,provider_key,pay_amount,provider_instance_id,created_at,expires_at) VALUES($1,$2,'usdt_trc20',$3,$4,$5,$6)`, input.OrderID, input.OutTradeNo, input.BaseAmount, input.ProviderInstanceID, input.CreatedAt, input.ExpiresAt)
	require.NoError(t, err)
	return input
}

func TestTRC20PostgresAllocatesSmallestCentAcrossProvidersAndNearbyBaseAmounts(t *testing.T) {
	repo, ctx := trc20Postgres(t)
	for index, tc := range []struct {
		instanceID string
		wallet     string
		baseUnits  int64
		wantUnits  int64
	}{
		{"1", trc20PostgresWallet, 10000000, 10000000},
		{"2", trc20PostgresWallet, 10000000, 10010000},
		{"1", trc20PostgresWallet, 10010000, 10020000},
		{"3", payment.TRC20USDTContract, 10000000, 10000000},
	} {
		input := trc20PostgresOrder(t, ctx, repo, int64(index+1), tc.instanceID, tc.wallet, tc.baseUnits)
		intent, err := repo.Allocate(ctx, input, tc.baseUnits)
		require.NoError(t, err)
		require.Equal(t, tc.wantUnits, intent.AmountUnits)
		require.Zero(t, intent.AmountUnits%payment.TRC20AmountStepUnits)
		again, err := repo.Allocate(ctx, input, tc.baseUnits)
		require.NoError(t, err)
		require.Equal(t, intent.AmountUnits, again.AmountUnits, "重复请求必须返回原金额，不额外占用下一分")
	}
}

type trc20PostgresAllocationResult struct {
	intent *service.PaymentTRC20Intent
	err    error
}

func TestTRC20PostgresConcurrentInstancesNeverShareAmount(t *testing.T) {
	repo, ctx := trc20Postgres(t)
	const count = 20
	inputs := make([]service.PaymentTRC20CreateInput, count)
	for index := range inputs {
		inputs[index] = trc20PostgresOrder(t, ctx, repo, int64(index+1), fmt.Sprint(index%2+1), trc20PostgresWallet, 10000000)
	}
	start := make(chan struct{})
	results := make(chan trc20PostgresAllocationResult, count)
	for _, input := range inputs {
		go func(input service.PaymentTRC20CreateInput) {
			<-start
			// 独立仓储对象及数据库连接依靠事务锁协调，不能依赖进程内互斥。
			instance := &paymentTRC20Repository{db: repo.db}
			intent, err := instance.Allocate(ctx, input, 10000000)
			results <- trc20PostgresAllocationResult{intent: intent, err: err}
		}(input)
	}
	close(start)
	amounts := make([]int64, 0, count)
	for range count {
		result := <-results
		require.NoError(t, result.err)
		amounts = append(amounts, result.intent.AmountUnits)
	}
	sort.Slice(amounts, func(i, j int) bool { return amounts[i] < amounts[j] })
	for index, units := range amounts {
		require.Equal(t, int64(10000000)+int64(index)*payment.TRC20AmountStepUnits, units)
	}
}

func TestTRC20PostgresExhaustionNeverReusesCreditedOrExpiredAmounts(t *testing.T) {
	repo, ctx := trc20Postgres(t)
	for index := int64(0); index <= payment.TRC20AmountExtraSteps; index++ {
		input := trc20PostgresOrder(t, ctx, repo, index+1, "1", trc20PostgresWallet, 10000000)
		intent, err := repo.Allocate(ctx, input, 10000000)
		require.NoError(t, err)
		require.Equal(t, int64(10000000)+index*payment.TRC20AmountStepUnits, intent.AmountUnits)
		if index == 0 {
			_, err = repo.ClaimTransfer(ctx, input.OrderID, payment.TRC20Transfer{TransactionHash: strings.Repeat("a", 64), WalletAddress: input.WalletAddress, AmountUnits: intent.AmountUnits, BlockTimestamp: input.CreatedAt.Add(time.Second)})
			require.NoError(t, err)
			require.NoError(t, repo.MarkCredited(ctx, input.OrderID))
		}
	}
	// 模拟剩余旧账单已过期，永久约束仍不得向新订单回收这些金额。
	_, err := repo.db.ExecContext(ctx, `UPDATE payment_orders SET expires_at=created_at+INTERVAL '2 seconds' WHERE id<=100`)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, `UPDATE payment_trc20_intents SET expires_at=created_at+INTERVAL '2 seconds' WHERE order_id<=100`)
	require.NoError(t, err)
	input := trc20PostgresOrder(t, ctx, repo, 101, "2", trc20PostgresWallet, 10000000)
	_, err = repo.Allocate(ctx, input, 10000000)
	require.ErrorIs(t, err, service.ErrTRC20AmountExhausted)
	_, err = repo.Get(ctx, input.OrderID)
	require.ErrorIs(t, err, service.ErrTRC20IntentNotFound, "候选用尽不能创建越界或重复金额")
}

func TestTRC20PostgresLegacyMicroAmountRemainsIdempotentAndExact(t *testing.T) {
	repo, ctx := trc20Postgres(t)
	input := trc20PostgresOrder(t, ctx, repo, 1, "1", trc20PostgresWallet, 10000000)
	_, err := repo.db.ExecContext(ctx, `INSERT INTO payment_trc20_intents(order_id,out_trade_no,provider_instance_id,wallet_address,base_amount,amount_units,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, input.OrderID, input.OutTradeNo, input.ProviderInstanceID, input.WalletAddress, input.BaseAmount, int64(10000042), input.CreatedAt, input.ExpiresAt)
	require.NoError(t, err)
	for range 2 {
		intent, err := service.NewPaymentTRC20Service(repo).Allocate(ctx, input)
		require.NoError(t, err)
		require.Equal(t, int64(10000042), intent.AmountUnits)
		require.Equal(t, "10.000042", intent.ExactAmount)
	}
	transfer := payment.TRC20Transfer{TransactionHash: strings.Repeat("b", 64), WalletAddress: input.WalletAddress, AmountUnits: 10000000, BlockTimestamp: input.CreatedAt.Add(time.Second)}
	_, err = repo.ClaimTransfer(ctx, input.OrderID, transfer)
	require.Error(t, err, "旧订单不能按显示两位后的舍入金额认领")
	transfer.AmountUnits = 10000042
	intent, err := repo.ClaimTransfer(ctx, input.OrderID, transfer)
	require.NoError(t, err)
	require.Equal(t, int64(10000042), intent.AmountUnits)
	newInput := trc20PostgresOrder(t, ctx, repo, 2, "2", trc20PostgresWallet, 10000000)
	newIntent, err := repo.Allocate(ctx, newInput, 10000000)
	require.NoError(t, err)
	require.Equal(t, int64(10000000), newIntent.AmountUnits, "历史微尾数与新整分金额必须按完整链上整数区分")
}
