//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func startBulkEmailDeliveryTestBatch(t *testing.T, count int) (service.BulkEmailRepository, int64) {
	t.Helper()
	ctx := context.Background()
	repo := NewBulkEmailRepository(integrationDB)
	ids := make([]int64, count)
	for i := range ids {
		email := fmt.Sprintf("bulk-%d-%d@example.com", time.Now().UnixNano(), i)
		err := integrationDB.QueryRowContext(ctx, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, email).Scan(&ids[i])
		require.NoError(t, err)
		id := ids[i]
		t.Cleanup(func() {
			_, err := integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, id)
			require.NoError(t, err)
		})
	}
	batch := &service.BulkEmailBatch{CreatedBy: ids[0], Subject: "投递回归", Body: "测试正文"}
	require.NoError(t, repo.CreateDraft(ctx, batch, ids, false))
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, `DELETE FROM bulk_email_batches WHERE id=$1`, batch.ID)
		require.NoError(t, err)
	})
	require.NoError(t, repo.StartBatch(ctx, batch.ID))
	return repo, batch.ID
}

func TestBulkEmailDeliveryConcurrentClaimAndComplete(t *testing.T) {
	ctx := context.Background()
	repo, batchID := startBulkEmailDeliveryTestBatch(t, 1)
	// 两个工作进程同时领取同一个收件人，仅持有租约的一方可以发送。
	items := make(chan []service.BulkEmailRecipient, 2)
	errs := make(chan error, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			claimed, err := repo.Claim(ctx, 1, time.Minute)
			items <- claimed
			errs <- err
		}()
	}
	close(start)
	claimed := append(<-items, (<-items)...)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	require.Len(t, claimed, 1)
	require.Equal(t, batchID, claimed[0].BatchID)
	require.Equal(t, 1, claimed[0].Attempts)
	require.NoError(t, repo.Complete(ctx, claimed[0], service.BulkEmailDeliverySent, time.Now()))
	batch, err := repo.Get(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, "completed", batch.Status)
	require.EqualValues(t, 1, batch.SentCount)
	list, _, err := repo.Recipients(ctx, batchID, 1, 10)
	require.NoError(t, err)
	require.NotNil(t, list[0].SentAt)
	again, err := repo.Claim(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Empty(t, again)
}

func TestBulkEmailDeliveryExpiredAndUnconfirmedNeverRetried(t *testing.T) {
	ctx := context.Background()
	repo, batchID := startBulkEmailDeliveryTestBatch(t, 3)
	list, _, err := repo.Recipients(ctx, batchID, 1, 10)
	require.NoError(t, err)
	require.Len(t, list, 3)
	// 模拟旧版本反复领取且无法写回结果的行，以及即将用完重试次数的明确失败行。
	_, err = integrationDB.ExecContext(ctx, `UPDATE bulk_email_recipients SET status='sending',attempts=258,lease_token='旧租约',lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, list[0].ID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE bulk_email_recipients SET attempts=4 WHERE id=$1`, list[1].ID)
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		claimed, err := repo.Claim(ctx, 1, time.Minute)
		require.NoError(t, err)
		require.Len(t, claimed, 1)
		item := claimed[0]
		switch item.ID {
		case list[1].ID:
			require.Equal(t, 5, item.Attempts)
			require.NoError(t, repo.Complete(ctx, item, service.BulkEmailDeliveryFailed, time.Now().Add(time.Hour)))
		case list[2].ID:
			require.NoError(t, repo.Complete(ctx, item, service.BulkEmailDeliveryUnconfirmed, time.Now()))
		default:
			t.Fatalf("错误地重新领取了投递结果未知的收件人 %d", item.ID)
		}
	}
	batch, err := repo.Get(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, "needs_review", batch.Status)
	require.EqualValues(t, 1, batch.FailedCount)
	require.EqualValues(t, 2, batch.UnconfirmedCount)
	require.NoError(t, repo.Retry(ctx, batchID))
	claimed, err := repo.Claim(ctx, 3, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, list[1].ID, claimed[0].ID)
	require.NoError(t, repo.Complete(ctx, claimed[0], service.BulkEmailDeliverySent, time.Now()))
	require.ErrorIs(t, repo.Retry(ctx, batchID), service.ErrBulkEmailState)
	again, err := repo.Claim(ctx, 3, time.Minute)
	require.NoError(t, err)
	require.Empty(t, again)
	list, _, err = repo.Recipients(ctx, batchID, 1, 10)
	require.NoError(t, err)
	require.Equal(t, "unconfirmed", list[0].Status)
	require.Equal(t, 258, list[0].Attempts)
	require.Equal(t, "sent", list[1].Status)
	require.Equal(t, "unconfirmed", list[2].Status)
	require.Equal(t, 1, list[2].Attempts)
}

func TestBulkEmailDeliveryLostReceiptClosesBatchWithoutResending(t *testing.T) {
	ctx := context.Background()
	repo, batchID := startBulkEmailDeliveryTestBatch(t, 1)
	claimed, err := repo.Claim(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	item := claimed[0]
	// 模拟 SMTP 已接收但进程未写回结果。租约到期前后都不应再次领取该地址。
	again, err := repo.Claim(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Empty(t, again)
	_, err = integrationDB.ExecContext(ctx, `UPDATE bulk_email_recipients SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, item.ID)
	require.NoError(t, err)
	again, err = repo.Claim(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Empty(t, again)
	require.ErrorIs(t, repo.Complete(ctx, item, service.BulkEmailDeliverySent, time.Now()), service.ErrBulkEmailState)
	batch, err := repo.Get(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, "needs_review", batch.Status)
	require.EqualValues(t, 1, batch.UnconfirmedCount)
	require.Zero(t, batch.FailedCount)
	require.Zero(t, batch.SentCount)
	require.ErrorIs(t, repo.Retry(ctx, batchID), service.ErrBulkEmailState)
}

func TestBulkEmailDeliveryDefiniteFailureHonorsBackoff(t *testing.T) {
	ctx := context.Background()
	repo, batchID := startBulkEmailDeliveryTestBatch(t, 1)
	claimed, err := repo.Claim(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	item := claimed[0]
	require.NoError(t, repo.Complete(ctx, item, service.BulkEmailDeliveryFailed, time.Now().Add(time.Hour)))
	again, err := repo.Claim(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Empty(t, again)
	_, err = integrationDB.ExecContext(ctx, `UPDATE bulk_email_recipients SET next_attempt_at=NOW() WHERE id=$1`, item.ID)
	require.NoError(t, err)
	again, err = repo.Claim(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, again, 1)
	require.Equal(t, item.ID, again[0].ID)
	require.Equal(t, 2, again[0].Attempts)
	require.NoError(t, repo.Complete(ctx, again[0], service.BulkEmailDeliverySent, time.Now()))
	batch, err := repo.Get(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, "completed", batch.Status)
}
