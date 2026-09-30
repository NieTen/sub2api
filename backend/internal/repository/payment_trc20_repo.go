package repository

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type paymentTRC20Repository struct{ db *sql.DB }
type trc20IntentScanner interface{ Scan(...any) error }

func NewPaymentTRC20Repository(db *sql.DB) service.PaymentTRC20Repository {
	return &paymentTRC20Repository{db: db}
}

const trc20IntentColumns = `order_id,out_trade_no,provider_instance_id,wallet_address,base_amount,amount_units,created_at,expires_at,COALESCE(transaction_hash,''),transferred_at,(credited_at IS NOT NULL)`

func scanTRC20Intent(row trc20IntentScanner) (*service.PaymentTRC20Intent, error) {
	intent := &service.PaymentTRC20Intent{}
	err := row.Scan(&intent.OrderID, &intent.OutTradeNo, &intent.ProviderInstanceID, &intent.WalletAddress, &intent.BaseAmount, &intent.AmountUnits, &intent.CreatedAt, &intent.ExpiresAt, &intent.TransactionHash, &intent.TransferredAt, &intent.Credited)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrTRC20IntentNotFound
	}
	return intent, err
}

func (r *paymentTRC20Repository) Allocate(ctx context.Context, input service.PaymentTRC20CreateInput, baseUnits int64) (*service.PaymentTRC20Intent, error) {
	if baseUnits <= 0 || baseUnits%payment.TRC20AmountStepUnits != 0 {
		return nil, errors.New("TRC20 基础金额必须为有效的两位小数金额")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var orderID int64
	// 绑定已存在账单及实例，前端或其他调用方不能为不同金额的订单分配链上意图。
	err = tx.QueryRowContext(ctx, `SELECT id FROM payment_orders WHERE id=$1 AND out_trade_no=$2 AND provider_key='usdt_trc20' AND pay_amount=$3 AND provider_instance_id=$4 AND created_at=$5 AND expires_at=$6 FOR UPDATE`, input.OrderID, input.OutTradeNo, input.BaseAmount, input.ProviderInstanceID, input.CreatedAt, input.ExpiresAt).Scan(&orderID)
	if err != nil {
		return nil, fmt.Errorf("核对 TRC20 账单失败: %w", err)
	}
	intent, err := scanTRC20Intent(tx.QueryRowContext(ctx, `SELECT `+trc20IntentColumns+` FROM payment_trc20_intents WHERE order_id=$1`, input.OrderID))
	if err == nil {
		if intent.WalletAddress != input.WalletAddress || intent.OutTradeNo != input.OutTradeNo || intent.ProviderInstanceID != input.ProviderInstanceID || intent.BaseAmount != input.BaseAmount {
			return nil, errors.New("TRC20 收款意图已存在且不能修改")
		}
		return intent, tx.Commit()
	}
	if !errors.Is(err, service.ErrTRC20IntentNotFound) {
		return nil, err
	}
	// 只约束新分配的候选范围；历史意图即使接近整数上限，也必须原样幂等返回。
	if baseUnits > math.MaxInt64-payment.TRC20AmountStepUnits*payment.TRC20AmountExtraSteps {
		return nil, errors.New("TRC20 金额超出支持范围")
	}
	// 按钱包串行分配，两个实例并发下单也不能获得相同金额；唯一索引作为最终约束。
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,20260930))`, input.WalletAddress); err != nil {
		return nil, err
	}
	var units int64
	// 从基础金额开始按一分递增，最多增加 0.99 USDT；历史订单永不释放金额。
	err = tx.QueryRowContext(ctx, `SELECT $1::bigint+n*$3::bigint FROM generate_series(0,$4::bigint) AS n WHERE NOT EXISTS (SELECT 1 FROM payment_trc20_intents i WHERE i.wallet_address=$2 AND i.amount_units=$1::bigint+n*$3::bigint) ORDER BY n LIMIT 1`, baseUnits, input.WalletAddress, payment.TRC20AmountStepUnits, payment.TRC20AmountExtraSteps).Scan(&units)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrTRC20AmountExhausted
	}
	if err != nil {
		return nil, err
	}
	intent, err = scanTRC20Intent(tx.QueryRowContext(ctx, `INSERT INTO payment_trc20_intents(order_id,out_trade_no,provider_instance_id,wallet_address,base_amount,amount_units,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+trc20IntentColumns, input.OrderID, input.OutTradeNo, input.ProviderInstanceID, input.WalletAddress, input.BaseAmount, units, input.CreatedAt, input.ExpiresAt))
	if err != nil {
		return nil, err
	}
	return intent, tx.Commit()
}

func (r *paymentTRC20Repository) Get(ctx context.Context, orderID int64) (*service.PaymentTRC20Intent, error) {
	return scanTRC20Intent(r.db.QueryRowContext(ctx, `SELECT `+trc20IntentColumns+` FROM payment_trc20_intents WHERE order_id=$1`, orderID))
}

func (r *paymentTRC20Repository) ClaimTransfer(ctx context.Context, orderID int64, transfer payment.TRC20Transfer) (*service.PaymentTRC20Intent, error) {
	if len(transfer.TransactionHash) != 64 {
		return nil, errors.New("TRC20 交易哈希无效")
	}
	if _, err := hex.DecodeString(transfer.TransactionHash); err != nil {
		return nil, errors.New("TRC20 交易哈希无效")
	}
	transfer.TransactionHash = strings.ToLower(transfer.TransactionHash)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	intent, err := scanTRC20Intent(tx.QueryRowContext(ctx, `SELECT `+trc20IntentColumns+` FROM payment_trc20_intents WHERE order_id=$1 FOR UPDATE`, orderID))
	if err != nil {
		return nil, err
	}
	if intent.WalletAddress != transfer.WalletAddress || intent.AmountUnits != transfer.AmountUnits || transfer.BlockTimestamp.Before(intent.CreatedAt) || transfer.BlockTimestamp.After(intent.ExpiresAt) {
		return nil, errors.New("TRC20 交易与账单的地址、精确金额或付款时间不匹配")
	}
	if intent.TransactionHash != "" {
		if intent.TransactionHash != transfer.TransactionHash {
			return nil, service.ErrTRC20TransferClaimed
		}
		return intent, tx.Commit()
	}
	// 唯一索引在数据库中原子认领哈希，任何其他订单均不能重复使用同一笔链上交易。
	_, err = tx.ExecContext(ctx, `UPDATE payment_trc20_intents SET transaction_hash=$2,transferred_at=$3 WHERE order_id=$1 AND transaction_hash IS NULL`, orderID, transfer.TransactionHash, transfer.BlockTimestamp)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, service.ErrTRC20TransferClaimed
		}
		return nil, err
	}
	intent.TransactionHash = transfer.TransactionHash
	intent.TransferredAt = &transfer.BlockTimestamp
	return intent, tx.Commit()
}

func (r *paymentTRC20Repository) ListForPoll(ctx context.Context, limit int) ([]service.PaymentTRC20Intent, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+trc20IntentColumns+` FROM payment_trc20_intents WHERE credited_at IS NULL AND (transaction_hash IS NOT NULL OR expires_at>NOW()-INTERVAL '30 minutes') ORDER BY checked_at ASC NULLS FIRST,order_id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	intents := []service.PaymentTRC20Intent{}
	for rows.Next() {
		intent, scanErr := scanTRC20Intent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		intents = append(intents, *intent)
	}
	return intents, rows.Err()
}

func (r *paymentTRC20Repository) MarkChecked(ctx context.Context, orderID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE payment_trc20_intents SET checked_at=NOW() WHERE order_id=$1 AND credited_at IS NULL`, orderID)
	return err
}
func (r *paymentTRC20Repository) MarkCredited(ctx context.Context, orderID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE payment_trc20_intents SET credited_at=NOW() WHERE order_id=$1 AND transaction_hash IS NOT NULL AND credited_at IS NULL`, orderID)
	return err
}
