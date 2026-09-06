package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBulkEmailDraftRollsBackWhenAudienceIsEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO bulk_email_batches`).WithArgs(int64(7), "标题", "正文", "[]", 0, sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(1), time.Now(), time.Now()))
	mock.ExpectExec(`(?s)INSERT INTO bulk_email_recipients.*DISTINCT ON.*deleted_at IS NULL AND status='active'.*id=ANY.*LIKE ANY`).WithArgs(int64(1), false, "{2}", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	err = NewBulkEmailRepository(db).CreateDraft(context.Background(), &service.BulkEmailBatch{CreatedBy: 7, Subject: "标题", Body: "正文", Images: []service.EmailInlineImage{}}, []int64{2}, false)
	require.ErrorIs(t, err, service.ErrBulkEmailNoRecipients)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBulkEmailDraftConsumesOnlyLockedOwnedAttachments(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT id FROM support_ticket_attachments.*owner_id=\$2 AND message_id IS NULL.*FOR UPDATE`).WithArgs("{11}", int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)))
	mock.ExpectQuery(`INSERT INTO bulk_email_batches`).WithArgs(int64(7), "标题", "正文", sqlmock.AnyArg(), 1, sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(1), time.Now(), time.Now()))
	mock.ExpectExec(`(?s)INSERT INTO bulk_email_recipients.*DISTINCT ON.*LIKE ANY`).WithArgs(int64(1), true, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 20))
	mock.ExpectExec(`DELETE FROM support_ticket_attachments WHERE id=ANY\(\$1::bigint\[\]\) AND owner_id=\$2 AND message_id IS NULL`).WithArgs("{11}", int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	batch := &service.BulkEmailBatch{CreatedBy: 7, Subject: "标题", Body: "正文", DraftAttachmentIDs: []int64{11}, Images: []service.EmailInlineImage{{MimeType: "image/png", Data: []byte{1}}}}
	require.NoError(t, NewBulkEmailRepository(db).CreateDraft(context.Background(), batch, nil, true))
	require.EqualValues(t, 20, batch.TotalCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBulkEmailDraftRejectsAlreadyConsumedAttachment(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM support_ticket_attachments`).WithArgs("{11}", int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	err = NewBulkEmailRepository(db).CreateDraft(context.Background(), &service.BulkEmailBatch{CreatedBy: 7, DraftAttachmentIDs: []int64{11}}, []int64{2}, false)
	require.ErrorIs(t, err, service.ErrSupportAttachmentInvalid)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBulkEmailCompleteRejectsExpiredLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	retryAt := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM bulk_email_batches WHERE id=\$1 FOR UPDATE`).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectExec(`(?s)UPDATE bulk_email_recipients.*WHERE id=\$4 AND lease_token=\$5 AND status='sending'`).WithArgs("sent", "", retryAt, int64(3), "old-lease").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	err = NewBulkEmailRepository(db).Complete(context.Background(), service.BulkEmailRecipient{ID: 3, BatchID: 1, LeaseToken: "old-lease"}, "", retryAt)
	require.ErrorIs(t, err, service.ErrBulkEmailState)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBulkEmailClaimOnlyConfirmedBatchesAndUsesLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`(?s)WITH picked AS.*b.status IN \('queued','sending'\).*r.lease_until<NOW\(\).*FOR UPDATE OF r SKIP LOCKED.*lease_token=\$3`).WithArgs(1, float64(180), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "batch_id", "user_id", "email", "status", "attempts", "last_error", "sent_at", "lease_token"}).AddRow(int64(1), int64(2), int64(3), "to@example.com", "sending", 1, "", nil, "lease"))
	items, err := NewBulkEmailRepository(db).Claim(context.Background(), 1, 3*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "lease", items[0].LeaseToken)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBulkEmailRetryDoesNotResendSuccessfulRecipients(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status FROM bulk_email_batches WHERE id=\$1 FOR UPDATE`).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("partial_failed"))
	mock.ExpectExec(`(?s)UPDATE bulk_email_recipients.*WHERE batch_id=\$1 AND status='failed'`).WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`UPDATE bulk_email_batches SET status='queued',updated_at=NOW\(\) WHERE id=\$1`).WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, NewBulkEmailRepository(db).Retry(context.Background(), 1))
	require.NoError(t, mock.ExpectationsWereMet())
}
