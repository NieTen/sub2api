package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestSupportTicketCreateCommitsMessageWithOutbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO support_tickets`).WithArgs(int64(2), "工单").WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "last_message_at"}).AddRow(5, now, now, now))
	mock.ExpectQuery(`INSERT INTO support_ticket_messages`).WithArgs(int64(5), int64(2), "user", "web", "消息", "").WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(7, now))
	mock.ExpectExec(`UPDATE support_ticket_attachments SET message_id = \$1 WHERE id = ANY\(\$2\) AND owner_id = \$3 AND message_id IS NULL`).WithArgs(int64(7), pq.Array([]int64{8}), int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE support_tickets SET updated_at`).WithArgs(int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO support_ticket_notifications`).WithArgs(int64(5), int64(7), "web").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	ticket := &service.SupportTicket{UserID: 2, Subject: "工单"}
	message := &service.SupportTicketMessage{SenderID: 2, SenderRole: "user", Source: "web", Content: "消息"}
	require.NoError(t, NewSupportTicketRepository(db).CreateTicket(context.Background(), ticket, message, []int64{8}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSupportTicketWrongOwnerAttachmentRollsBackEntireMessage(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status, user_id FROM support_tickets WHERE id = \$1 FOR UPDATE`).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"status", "user_id"}).AddRow("open", 2))
	mock.ExpectQuery(`INSERT INTO support_ticket_messages`).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(7, time.Now()))
	mock.ExpectExec(`UPDATE support_ticket_attachments SET message_id = \$1 WHERE id = ANY\(\$2\) AND owner_id = \$3 AND message_id IS NULL`).WithArgs(int64(7), pq.Array([]int64{8}), int64(2)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	message := &service.SupportTicketMessage{TicketID: 5, SenderID: 2, SenderRole: "user", Source: "web", Content: "消息"}
	require.ErrorIs(t, NewSupportTicketRepository(db).AddMessage(context.Background(), message, []int64{8}), service.ErrSupportAttachmentInvalid)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSupportTicketDuplicateTelegramRollsBackWithoutOutbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status, user_id`).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"status", "user_id"}).AddRow("open", 2))
	mock.ExpectQuery(`INSERT INTO support_ticket_messages`).WillReturnError(&pq.Error{Code: "23505", Constraint: "idx_support_ticket_messages_external"})
	mock.ExpectRollback()
	message := &service.SupportTicketMessage{TicketID: 5, SenderRole: "admin", Source: "telegram", Content: "消息", ExternalID: "telegram:101"}
	require.ErrorIs(t, NewSupportTicketRepository(db).AddMessage(context.Background(), message, nil), service.ErrSupportDuplicateMessage)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSupportTicketNotificationRequiresCurrentLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectExec(`UPDATE support_ticket_notifications SET delivered_at = NOW\(\).*WHERE id = \$1 AND lease_token = \$2 AND delivered_at IS NULL`).WithArgs(int64(7), "旧租约").WillReturnResult(sqlmock.NewResult(0, 0))
	err = NewSupportTicketRepository(db).CompleteNotification(context.Background(), 7, "旧租约")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSupportTicketNotificationClaimUsesSkipLocked(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`WITH pending AS .*FOR UPDATE SKIP LOCKED.*attempts = n.attempts \+ 1`).WithArgs(10, sqlmock.AnyArg(), int64(60)).WillReturnRows(sqlmock.NewRows([]string{"id", "ticket_id", "message_id", "channel", "attempts", "lease_token"}).AddRow(1, 5, 7, "email", 1, "租约"))
	items, err := NewSupportTicketRepository(db).ClaimNotifications(context.Background(), 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "租约", items[0].LeaseToken)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSupportTicketDeleteDraftRequiresOwnerAndUnboundMessage(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectExec(`DELETE FROM support_ticket_attachments WHERE id = \$1 AND owner_id = \$2 AND message_id IS NULL`).WithArgs(int64(8), int64(2)).WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, NewSupportTicketRepository(db).DeleteDraftAttachment(context.Background(), 2, 8), service.ErrSupportAttachmentNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
