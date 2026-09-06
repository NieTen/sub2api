package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBulkEmailSnapshotAppliesBalanceAndPaidOrderConditions(t *testing.T) {
	threshold := "999999999999.99999999"
	paidPredicate := " AND EXISTS (SELECT 1 FROM payment_orders po WHERE po.user_id = candidate.id AND po.order_type = 'balance' AND po.paid_at IS NOT NULL AND po.pay_amount > 0)"
	cases := []struct {
		name      string
		filter    service.BulkEmailRecipientFilter
		predicate string
		allActive bool
	}{
		{"有余额", service.BulkEmailRecipientFilter{BalanceCondition: "positive"}, " AND candidate.balance > 0", true},
		{"无余额含负数", service.BulkEmailRecipientFilter{BalanceCondition: "non_positive"}, " AND candidate.balance <= 0", true},
		{"严格大于高精度阈值", service.BulkEmailRecipientFilter{BalanceCondition: "greater_than", BalanceThreshold: &threshold}, " AND candidate.balance > $5::numeric", false},
		{"曾支付余额充值", service.BulkEmailRecipientFilter{RechargeCondition: "recharged"}, paidPredicate, true},
		{"组合且限定指定用户", service.BulkEmailRecipientFilter{BalanceCondition: "greater_than", BalanceThreshold: &threshold, RechargeCondition: "recharged"}, " AND candidate.balance > $5::numeric" + paidPredicate, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectBegin()
			mock.ExpectQuery(`INSERT INTO bulk_email_batches.*recipient_filter`).WithArgs(int64(7), "标题", "正文", "[]", 0, sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(1), time.Now(), time.Now()))
			args := []driver.Value{int64(1), tc.allActive, "{2}", sqlmock.AnyArg()}
			if tc.filter.BalanceThreshold != nil {
				args = append(args, threshold)
			}
			pattern := `(?s)INSERT INTO bulk_email_recipients.*FROM users AS candidate WHERE deleted_at IS NULL AND status='active'.*` + regexp.QuoteMeta("AND ($2 OR id=ANY($3::bigint[])) AND NOT (LOWER(BTRIM(email)) LIKE ANY($4::text[]))"+tc.predicate+" ORDER BY")
			mock.ExpectExec(pattern).WithArgs(args...).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			batch := &service.BulkEmailBatch{CreatedBy: 7, Subject: "标题", Body: "正文", RecipientFilter: tc.filter}
			require.NoError(t, NewBulkEmailRepository(db).CreateDraft(context.Background(), batch, []int64{2}, tc.allActive))
			require.EqualValues(t, 1, batch.TotalCount)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestBulkEmailInvalidFilterNeverCreatesTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	err = NewBulkEmailRepository(db).CreateDraft(context.Background(), &service.BulkEmailBatch{RecipientFilter: service.BulkEmailRecipientFilter{RechargeCondition: "gifted"}}, nil, true)
	require.ErrorIs(t, err, service.ErrBulkEmailRecipientFilter)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBulkEmailStoredFiltersAndLegacyBatchesRemainReadable(t *testing.T) {
	for _, raw := range []string{`{}`, `{"balance_condition":"greater_than","balance_threshold":"0.00000001","recharge_condition":"recharged"}`} {
		t.Run(raw, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			columns := []string{"id", "created_by", "subject", "body", "status", "created_at", "updated_at", "image_count", "total", "sent", "failed", "recipient_filter", "images"}
			mock.ExpectQuery(`(?s)SELECT .*b.recipient_filter,b.images FROM bulk_email_batches b WHERE b.id=\$1`).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows(columns).AddRow(1, 7, "标题", "正文", "draft", time.Now(), time.Now(), 0, 1, 0, 0, raw, `[]`))
			batch, err := NewBulkEmailRepository(db).Get(context.Background(), 1)
			require.NoError(t, err)
			if raw == `{}` {
				require.Equal(t, "all", batch.RecipientFilter.BalanceCondition)
				require.Equal(t, "all", batch.RecipientFilter.RechargeCondition)
			} else {
				require.Equal(t, "0.00000001", *batch.RecipientFilter.BalanceThreshold)
				require.Equal(t, "recharged", batch.RecipientFilter.RechargeCondition)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestBulkEmailFilterSnapshotIsStoredWithDraft(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	filter := service.BulkEmailRecipientFilter{BalanceCondition: "non_positive", RechargeCondition: "recharged"}
	encoded, err := json.Marshal(filter)
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO bulk_email_batches.*recipient_filter`).WithArgs(int64(7), "标题", "正文", "[]", 0, string(encoded)).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(1), time.Now(), time.Now()))
	mock.ExpectExec(`(?s)INSERT INTO bulk_email_recipients.*candidate.balance <= 0.*po.paid_at IS NOT NULL`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, NewBulkEmailRepository(db).CreateDraft(context.Background(), &service.BulkEmailBatch{CreatedBy: 7, Subject: "标题", Body: "正文", RecipientFilter: filter}, nil, true))
	require.NoError(t, mock.ExpectationsWereMet())
}
