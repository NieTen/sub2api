package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestFinalizeCodexQuotaOverdraftProbeFailedIsAtomic(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	until := now.Add(5 * time.Hour)
	state := &service.CodexQuotaOverdraftProbeState{
		Status:      "failed",
		CycleKey:    "5h:quota-cycle",
		QuotaWindow: "5h",
		Attempts:    1,
		Limit:       1,
		StartedAt:   now,
		RecoverAt:   &until,
	}
	reason := service.BuildTempUnschedReasonPayload("codex_quota_overdraft", "quota exhausted")

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts.*temp_unschedulable_until.*codex_quota_overdraft_probe`).
		WithArgs(service.CodexQuotaOverdraftProbeExtraKey, sqlmock.AnyArg(), until, reason, int64(77), state.CycleKey, false).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload, dedup_key)")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(77), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	finalized, err := repo.FinalizeCodexQuotaOverdraftProbeFailed(context.Background(), 77, state, until, reason)

	require.NoError(t, err)
	require.True(t, finalized)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFinalizeCodexQuotaOverdraftProbeFailedRollsBackWhenOutboxFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	until := now.Add(5 * time.Hour)
	state := &service.CodexQuotaOverdraftProbeState{
		Status:      "failed",
		CycleKey:    "5h:quota-cycle",
		QuotaWindow: "5h",
		Attempts:    1,
		Limit:       1,
		StartedAt:   now,
		RecoverAt:   &until,
	}

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts.*temp_unschedulable_until.*codex_quota_overdraft_probe`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload, dedup_key)")).
		WillReturnError(errors.New("outbox unavailable"))
	mock.ExpectRollback()

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	finalized, err := repo.FinalizeCodexQuotaOverdraftProbeFailed(context.Background(), 77, state, until, "quota exhausted")

	require.ErrorContains(t, err, "outbox unavailable")
	require.False(t, finalized)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPersistCodexQuotaOverdraftProbeUnlessFailedPreservesTerminalFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	state := &service.CodexQuotaOverdraftProbeState{
		Status:      "passed",
		CycleKey:    "5h:quota-cycle",
		QuotaWindow: "5h",
		Limit:       1,
		StartedAt:   now,
	}
	mock.ExpectExec(`(?s)UPDATE accounts.*status}', ''\) <> 'failed'`).
		WithArgs(service.CodexQuotaOverdraftProbeExtraKey, sqlmock.AnyArg(), int64(77), state.CycleKey).
		WillReturnResult(sqlmock.NewResult(0, 0))

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	persisted, err := repo.PersistCodexQuotaOverdraftProbeUnlessFailed(context.Background(), 77, state)

	require.NoError(t, err)
	require.False(t, persisted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClearCodexQuotaOverdraftPauseIfStateIsAtomic(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	reason := service.BuildTempUnschedReasonPayload("codex_quota_overdraft", "quota exhausted")
	observedReset := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)WITH target AS.*rate_limit_reset_at = \$5::timestamptz.*codex_quota_overdraft_probe,cycle_key.*FOR UPDATE.*RETURNING target.clear_rate_limit, target.clear_temp`).
		WithArgs(int64(77), "5h:quota-cycle", "passed", reason, observedReset).
		WillReturnRows(sqlmock.NewRows([]string{"clear_rate_limit", "clear_temp"}).AddRow(true, true))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload, dedup_key)")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(77), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	clearedRate, clearedTemp, err := repo.ClearCodexQuotaOverdraftPauseIfState(
		context.Background(),
		77,
		"5h:quota-cycle",
		"passed",
		reason,
		&observedReset,
	)

	require.NoError(t, err)
	require.True(t, clearedRate)
	require.True(t, clearedTemp)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClearCodexQuotaOverdraftPauseIfStateKeepsRearmedRateLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	observedReset := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)WITH target AS.*rate_limit_reset_at = \$5::timestamptz.*codex_quota_overdraft_probe,cycle_key.*FOR UPDATE`).
		WithArgs(int64(77), "5h:quota-cycle", "passed", "", observedReset).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	clearedRate, clearedTemp, err := repo.ClearCodexQuotaOverdraftPauseIfState(
		context.Background(),
		77,
		"5h:quota-cycle",
		"passed",
		"",
		&observedReset,
	)

	require.NoError(t, err)
	require.False(t, clearedRate)
	require.False(t, clearedTemp)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFinalizeCodexQuotaOverdraftBusinessFailureCanReplacePassedState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	until := now.Add(5 * time.Hour)
	state := &service.CodexQuotaOverdraftProbeState{
		Status:      "failed",
		CycleKey:    "5h:quota-cycle",
		QuotaWindow: "5h",
		Limit:       1,
		ReasonCode:  "business_quota_limited",
		StartedAt:   now,
		RecoverAt:   &until,
	}
	reason := service.BuildTempUnschedReasonPayload("codex_quota_overdraft", "quota exhausted")

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts.*'passed', 'inconclusive', 'recovered'`).
		WithArgs(service.CodexQuotaOverdraftProbeExtraKey, sqlmock.AnyArg(), until, reason, int64(77), state.CycleKey, true).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload, dedup_key)")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(77), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	finalized, err := repo.FinalizeCodexQuotaOverdraftProbeFailed(context.Background(), 77, state, until, reason)

	require.NoError(t, err)
	require.True(t, finalized)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountListStatusFiltersCodexOverdraftingSQL(t *testing.T) {
	tests := []struct {
		name               string
		status             string
		wantContains       []string
		wantNotContains    []string
		wantOverdraftCheck bool
	}{
		{
			name:   "active_excludes_overdrafting",
			status: service.StatusActive,
			wantContains: []string{
				"\"accounts\".\"status\" = $1",
				"\"accounts\".\"schedulable\"",
				"rate_limit_reset_at",
			},
			wantOverdraftCheck: true,
		},
		{
			name:   "overdrafting_matches_passed_probe_before_recover_at",
			status: service.StatusFilterOverdrafting,
			wantContains: []string{
				"\"accounts\".\"status\" = $1",
				"#>> '{codex_quota_overdraft_probe,status}' = 'passed'",
				"NULLIF(",
				"timestamptz > NOW()",
			},
			wantNotContains: []string{
				`schedulable =`,
			},
			wantOverdraftCheck: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedSQL string
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureEntQueryMatcher{actual: &capturedSQL}))
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })

			driver := entsql.OpenDB(dialect.Postgres, db)
			client := dbent.NewClient(dbent.Driver(driver))
			t.Cleanup(func() { _ = client.Close() })

			mock.ExpectQuery("account list").WillReturnRows(sqlmock.NewRows([]string{"id"}))
			repo := newAccountRepositoryWithSQL(client, db, nil)

			accounts, err := repo.ListAllWithFilters(context.Background(), "", "", tt.status, "", 0, "")

			require.NoError(t, err)
			require.Empty(t, accounts)
			require.NoError(t, mock.ExpectationsWereMet())

			normalized := normalizeSQLWhitespace(capturedSQL)
			for _, want := range tt.wantContains {
				require.Contains(t, normalized, want)
			}
			for _, unwanted := range tt.wantNotContains {
				require.NotContains(t, normalized, unwanted)
			}
			if tt.wantOverdraftCheck {
				require.Contains(t, normalized, "codex_quota_overdraft_probe")
			}
		})
	}
}
