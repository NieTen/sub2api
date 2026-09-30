package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type paymentExchangeRateRepository struct{ db *sql.DB }
type paymentExchangeRateScanner interface{ Scan(...any) error }

func NewPaymentExchangeRateRepository(db *sql.DB) service.PaymentExchangeRateRepository {
	return &paymentExchangeRateRepository{db: db}
}

const paymentExchangeRateColumns = `rate,source,fetched_at,observed_at,fallback_reason,sample_prices,sample_count,aggregation`

func scanPaymentExchangeRate(row paymentExchangeRateScanner) (*service.PaymentExchangeRatePoint, error) {
	point := &service.PaymentExchangeRatePoint{}
	var prices []byte
	if err := row.Scan(&point.Rate, &point.Source, &point.FetchedAt, &point.ObservedAt, &point.FallbackReason, &prices, &point.SampleCount, &point.Aggregation); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(prices, &point.SamplePrices); err != nil {
		return nil, err
	}
	if point.SamplePrices == nil {
		point.SamplePrices = []float64{}
	}
	return point, nil
}

func (r *paymentExchangeRateRepository) Latest(ctx context.Context) (*service.PaymentExchangeRatePoint, error) {
	// 必须读取最新一次尝试，不能只查询成功记录，否则失败后会继续沿用旧实时价。
	// 自增主键表示持久化顺序，避免主机校时后未来时间的旧记录压住新报价。
	return scanPaymentExchangeRate(r.db.QueryRowContext(ctx, `SELECT `+paymentExchangeRateColumns+` FROM payment_exchange_rate_history ORDER BY id DESC LIMIT 1`))
}

func (r *paymentExchangeRateRepository) Insert(ctx context.Context, point service.PaymentExchangeRatePoint) error {
	prices := point.SamplePrices
	if prices == nil {
		prices = []float64{}
	}
	encoded, err := json.Marshal(prices)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO payment_exchange_rate_history(rate,source,fetched_at,observed_at,fallback_reason,sample_prices,sample_count,aggregation) VALUES($1,$2,$3,$4,$5,$6::jsonb,$7,$8)`, point.Rate, point.Source, point.FetchedAt, point.ObservedAt, point.FallbackReason, string(encoded), point.SampleCount, point.Aggregation)
	return err
}

func (r *paymentExchangeRateRepository) History(ctx context.Context, since, until time.Time) ([]service.PaymentExchangeRatePoint, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+paymentExchangeRateColumns+` FROM payment_exchange_rate_history WHERE fetched_at >= $1 AND fetched_at <= $2 ORDER BY fetched_at ASC,id ASC`, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := []service.PaymentExchangeRatePoint{}
	for rows.Next() {
		point, err := scanPaymentExchangeRate(rows)
		if err != nil {
			return nil, err
		}
		points = append(points, *point)
	}
	return points, rows.Err()
}
