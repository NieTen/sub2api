//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBulkEmailPauseIsolatesBatchesAndPreservesBackoff(t *testing.T) {
	ctx := context.Background()
	repo, batchID := startBulkEmailDeliveryTestBatch(t, 3)
	claimed, err := repo.Claim(ctx, 2, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 2)
	retryAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	require.NoError(t, repo.Complete(ctx, claimed[0], service.BulkEmailDeliveryFailed, retryAt))
	require.NoError(t, repo.Pause(ctx, batchID))
	require.NoError(t, repo.Pause(ctx, batchID))
	require.NoError(t, repo.Complete(ctx, claimed[1], service.BulkEmailDeliverySent, time.Now()))
	batch, err := repo.Get(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, "paused", batch.Status)
	require.EqualValues(t, 1, batch.SentCount)

	_, otherID := startBulkEmailDeliveryTestBatch(t, 1)
	other, err := repo.Claim(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, other, 1)
	require.Equal(t, otherID, other[0].BatchID)
	require.NoError(t, repo.Complete(ctx, other[0], service.BulkEmailDeliverySent, time.Now()))
	require.ErrorIs(t, repo.Pause(ctx, otherID), service.ErrBulkEmailState)
	require.ErrorIs(t, repo.Resume(ctx, otherID), service.ErrBulkEmailState)
	none, err := repo.Claim(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Empty(t, none)

	require.NoError(t, repo.Resume(ctx, batchID))
	require.NoError(t, repo.Resume(ctx, batchID))
	ready, err := repo.Claim(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, ready, 1)
	require.NotEqual(t, claimed[0].ID, ready[0].ID)
	require.NotEqual(t, claimed[1].ID, ready[0].ID)
	require.Equal(t, 1, ready[0].Attempts)
	var actualRetryAt time.Time
	var attempts int
	err = integrationDB.QueryRowContext(ctx, `SELECT next_attempt_at,attempts FROM bulk_email_recipients WHERE id=$1`, claimed[0].ID).Scan(&actualRetryAt, &attempts)
	require.NoError(t, err)
	require.True(t, retryAt.Equal(actualRetryAt))
	require.Equal(t, 1, attempts)
}

func TestBulkEmailResumeRacingReleaseUnsentReturnsOneAttempt(t *testing.T) {
	ctx := context.Background()
	repo, batchID := startBulkEmailDeliveryTestBatch(t, 1)
	_, err := integrationDB.ExecContext(ctx, `UPDATE bulk_email_recipients SET attempts=4,last_error='前一次明确失败' WHERE batch_id=$1`, batchID)
	require.NoError(t, err)
	claimed, err := repo.Claim(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, 5, claimed[0].Attempts)
	require.NoError(t, repo.Pause(ctx, batchID))

	// 与归还并发的继续操作不会多扣次数，也不会使已经归还的租约重新生效。
	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() { <-start; errs <- repo.Resume(ctx, batchID) }()
	go func() { <-start; errs <- repo.ReleaseUnsent(ctx, claimed[0]) }()
	close(start)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	require.ErrorIs(t, repo.ReleaseUnsent(ctx, claimed[0]), service.ErrBulkEmailState)
	list, _, err := repo.Recipients(ctx, batchID, 1, 10)
	require.NoError(t, err)
	require.Equal(t, "pending", list[0].Status)
	require.Equal(t, "前一次明确失败", list[0].LastError)
	require.Equal(t, 4, list[0].Attempts)
	again, err := repo.Claim(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, again, 1)
	require.Equal(t, claimed[0].ID, again[0].ID)
	require.Equal(t, 5, again[0].Attempts)
	require.NotEqual(t, claimed[0].LeaseToken, again[0].LeaseToken)
	require.ErrorIs(t, repo.ReleaseUnsent(ctx, claimed[0]), service.ErrBulkEmailState)
	require.NoError(t, repo.Pause(ctx, batchID))
	require.NoError(t, repo.Complete(ctx, again[0], service.BulkEmailDeliverySent, time.Now()))
	batch, err := repo.Get(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, "completed", batch.Status)
}

func TestBulkEmailPausedExpiredLeaseIsOnlyQuarantined(t *testing.T) {
	for _, recovery := range []string{"归还时过期", "后台发现过期"} {
		t.Run(recovery, func(t *testing.T) {
			ctx := context.Background()
			repo, batchID := startBulkEmailDeliveryTestBatch(t, 1)
			claimed, err := repo.Claim(ctx, 1, time.Minute)
			require.NoError(t, err)
			require.Len(t, claimed, 1)
			require.NoError(t, repo.Pause(ctx, batchID))
			_, err = integrationDB.ExecContext(ctx, `UPDATE bulk_email_recipients SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, claimed[0].ID)
			require.NoError(t, err)
			if recovery == "归还时过期" {
				require.NoError(t, repo.ReleaseUnsent(ctx, claimed[0]))
			}
			again, err := repo.Claim(ctx, 1, time.Minute)
			require.NoError(t, err)
			require.Empty(t, again)
			batch, err := repo.Get(ctx, batchID)
			require.NoError(t, err)
			require.Equal(t, "needs_review", batch.Status)
			require.EqualValues(t, 1, batch.UnconfirmedCount)
			require.ErrorIs(t, repo.Resume(ctx, batchID), service.ErrBulkEmailState)
			list, _, err := repo.Recipients(ctx, batchID, 1, 10)
			require.NoError(t, err)
			require.Equal(t, 1, list[0].Attempts)
		})
	}
}

func TestBulkEmailPauseRacingClaimKeepsRemainingWorkPaused(t *testing.T) {
	ctx := context.Background()
	repo, batchID := startBulkEmailDeliveryTestBatch(t, 2)
	start := make(chan struct{})
	paused := make(chan error, 1)
	go func() { <-start; paused <- repo.Pause(ctx, batchID) }()
	close(start)
	claimed, err := repo.Claim(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.NoError(t, <-paused)
	// 两种锁顺序均允许；先领取的一方最多持有一份在途租约，归还后剩余任务保持暂停。
	if len(claimed) > 0 {
		require.Len(t, claimed, 1)
		require.NoError(t, repo.ReleaseUnsent(ctx, claimed[0]))
	}
	none, err := repo.Claim(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Empty(t, none)
	list, _, err := repo.Recipients(ctx, batchID, 1, 10)
	require.NoError(t, err)
	require.Len(t, list, 2)
	for _, item := range list {
		require.Equal(t, "pending", item.Status)
		require.Zero(t, item.Attempts)
	}
	batch, err := repo.Get(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, "paused", batch.Status)
}

func TestBulkEmailPauseRejectsUnconfirmedDraftAndMissingBatch(t *testing.T) {
	ctx := context.Background()
	repo, batchID := startBulkEmailDeliveryTestBatch(t, 1)
	_, err := integrationDB.ExecContext(ctx, `UPDATE bulk_email_batches SET status='draft' WHERE id=$1`, batchID)
	require.NoError(t, err)
	require.ErrorIs(t, repo.Pause(ctx, batchID), service.ErrBulkEmailState)
	require.ErrorIs(t, repo.Resume(ctx, batchID), service.ErrBulkEmailState)
	require.ErrorIs(t, repo.Pause(ctx, -1), service.ErrBulkEmailNotFound)
	require.ErrorIs(t, repo.Resume(ctx, -1), service.ErrBulkEmailNotFound)
}
