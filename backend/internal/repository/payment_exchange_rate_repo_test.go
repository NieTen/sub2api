package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func paymentExchangeRateTestRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"rate", "source", "fetched_at", "observed_at", "fallback_reason", "sample_prices", "sample_count", "aggregation"})
}

func TestPaymentExchangeRateRepositoryLatestIncludesFailedAttempt(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT ` + paymentExchangeRateColumns + ` FROM payment_exchange_rate_history ORDER BY id DESC LIMIT 1`)).WillReturnRows(paymentExchangeRateTestRows().AddRow(7.1, "fallback", now, now, "网络失败", `[]`, 0, "configured_fallback"))
	point, err := NewPaymentExchangeRateRepository(db).Latest(context.Background())
	require.NoError(t, err)
	require.Equal(t, "fallback", point.Source)
	require.Equal(t, 7.1, point.Rate)
	require.Equal(t, now, *point.FetchedAt)
	require.Empty(t, point.SamplePrices)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPaymentExchangeRateRepositoryNoHistoryAndUnavailableEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewPaymentExchangeRateRepository(db)
	mock.ExpectQuery(`SELECT .* FROM payment_exchange_rate_history ORDER BY id DESC LIMIT 1`).WillReturnError(sql.ErrNoRows)
	point, err := repo.Latest(context.Background())
	require.NoError(t, err)
	require.Nil(t, point)
	now := time.Now().UTC()
	event := service.PaymentExchangeRatePoint{Source: "unavailable", FetchedAt: &now, ObservedAt: now, FallbackReason: "无有效卖单", Aggregation: "configured_fallback"}
	mock.ExpectExec(`INSERT INTO payment_exchange_rate_history.*VALUES\(\$1,\$2,\$3,\$4,\$5,\$6::jsonb,\$7,\$8\)`).WithArgs(float64(0), "unavailable", now, now, "无有效卖单", "[]", 0, "configured_fallback").WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, repo.Insert(context.Background(), event))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPaymentExchangeRateRepositoryHistoryKeepsFailedPointsInTimeOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now().UTC()
	since := now.Add(-72 * time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT `+paymentExchangeRateColumns+` FROM payment_exchange_rate_history WHERE fetched_at >= $1 AND fetched_at <= $2 ORDER BY fetched_at ASC,id ASC`)).WithArgs(since, now).WillReturnRows(paymentExchangeRateTestRows().AddRow(6.8, "okx", since, since, "", `[6.7,6.9]`, 2, "median_first_10_sell").AddRow(0, "unavailable", now, now, "网络失败", `[]`, 0, "configured_fallback"))
	points, err := NewPaymentExchangeRateRepository(db).History(context.Background(), since, now)
	require.NoError(t, err)
	require.Len(t, points, 2)
	require.Equal(t, []float64{6.7, 6.9}, points[0].SamplePrices)
	require.Equal(t, "unavailable", points[1].Source)
	require.Zero(t, points[1].Rate)
	require.NoError(t, mock.ExpectationsWereMet())
}
