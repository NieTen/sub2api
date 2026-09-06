package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type supportTicketRepository struct{ db *sql.DB }

func NewSupportTicketRepository(db *sql.DB) service.SupportTicketRepository {
	return &supportTicketRepository{db: db}
}

type supportTicketScanner interface{ Scan(...any) error }

const supportTicketColumns = `t.id, t.user_id, t.subject, t.status, u.email, COALESCE(u.username, ''), t.created_at, t.updated_at, t.last_message_at`
const supportMessageColumns = `id, ticket_id, COALESCE(sender_id, 0), sender_role, source, content, COALESCE(external_id, ''), created_at`

func scanSupportTicket(row supportTicketScanner) (*service.SupportTicket, error) {
	t := &service.SupportTicket{}
	err := row.Scan(&t.ID, &t.UserID, &t.Subject, &t.Status, &t.UserEmail, &t.Username, &t.CreatedAt, &t.UpdatedAt, &t.LastMessageAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSupportTicketNotFound
	}
	return t, err
}

func scanSupportMessage(row supportTicketScanner) (*service.SupportTicketMessage, error) {
	m := &service.SupportTicketMessage{Attachments: []service.SupportTicketAttachment{}}
	err := row.Scan(&m.ID, &m.TicketID, &m.SenderID, &m.SenderRole, &m.Source, &m.Content, &m.ExternalID, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSupportTicketNotFound
	}
	return m, err
}

func (r *supportTicketRepository) GetTicket(ctx context.Context, id int64) (*service.SupportTicket, error) {
	return scanSupportTicket(r.db.QueryRowContext(ctx, `SELECT `+supportTicketColumns+` FROM support_tickets t JOIN users u ON u.id = t.user_id WHERE t.id = $1`, id))
}

func (r *supportTicketRepository) ListTickets(ctx context.Context, ownerID int64, status string, page, pageSize int) (*service.SupportTicketPage, error) {
	result := &service.SupportTicketPage{Items: []service.SupportTicket{}, Page: page, PageSize: pageSize}
	where := ` WHERE ($1::bigint = 0 OR t.user_id = $1) AND ($2::text = '' OR t.status = $2)`
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(t.id) FROM support_tickets t`+where, ownerID, status).Scan(&result.Total); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+supportTicketColumns+` FROM support_tickets t JOIN users u ON u.id = t.user_id`+where+` ORDER BY t.last_message_at DESC, t.id DESC LIMIT $3 OFFSET $4`, ownerID, status, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanSupportTicket(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *t)
	}
	return result, rows.Err()
}

func (r *supportTicketRepository) ListMessages(ctx context.Context, ticketID, afterID int64, limit int) ([]service.SupportTicketMessage, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+supportMessageColumns+` FROM support_ticket_messages WHERE ticket_id = $1 AND id > $2 ORDER BY id ASC LIMIT $3`, ticketID, afterID, limit)
	if err != nil {
		return nil, err
	}
	messages := []service.SupportTicketMessage{}
	for rows.Next() {
		m, err := scanSupportMessage(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		messages = append(messages, *m)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if err := r.loadAttachments(ctx, messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (r *supportTicketRepository) loadAttachments(ctx context.Context, messages []service.SupportTicketMessage) error {
	if len(messages) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(messages))
	indexes := make(map[int64]int, len(messages))
	for i := range messages {
		ids = append(ids, messages[i].ID)
		indexes[messages[i].ID] = i
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, owner_id, message_id, file_name, mime_type, size, created_at FROM support_ticket_attachments WHERE message_id = ANY($1) ORDER BY id`, pq.Array(ids))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a service.SupportTicketAttachment
		if err := rows.Scan(&a.ID, &a.OwnerID, &a.MessageID, &a.FileName, &a.MimeType, &a.Size, &a.CreatedAt); err != nil {
			return err
		}
		i := indexes[a.MessageID]
		a.TicketID = messages[i].TicketID
		messages[i].Attachments = append(messages[i].Attachments, a)
	}
	return rows.Err()
}

func (r *supportTicketRepository) CreateTicket(ctx context.Context, ticket *service.SupportTicket, message *service.SupportTicketMessage, attachmentIDs []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `INSERT INTO support_tickets (user_id, subject, status) VALUES ($1, $2, 'open') RETURNING id, created_at, updated_at, last_message_at`, ticket.UserID, ticket.Subject).Scan(&ticket.ID, &ticket.CreatedAt, &ticket.UpdatedAt, &ticket.LastMessageAt)
	if err != nil {
		return err
	}
	message.TicketID = ticket.ID
	if err := insertSupportMessage(ctx, tx, message, attachmentIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *supportTicketRepository) AddMessage(ctx context.Context, message *service.SupportTicketMessage, attachmentIDs []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	var userID int64
	err = tx.QueryRowContext(ctx, `SELECT status, user_id FROM support_tickets WHERE id = $1 FOR UPDATE`, message.TicketID).Scan(&status, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrSupportTicketNotFound
	}
	if err != nil {
		return err
	}
	if status == service.SupportTicketStatusClosed {
		return service.ErrSupportTicketClosed
	}
	if message.SenderRole == "user" && message.SenderID != userID {
		return service.ErrSupportTicketNotFound
	}
	if err := insertSupportMessage(ctx, tx, message, attachmentIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func insertSupportMessage(ctx context.Context, tx *sql.Tx, message *service.SupportTicketMessage, attachmentIDs []int64) error {
	if len(attachmentIDs)+len(message.Attachments) > service.SupportTicketMaxAttachments {
		return service.ErrSupportAttachmentInvalid
	}
	err := tx.QueryRowContext(ctx, `INSERT INTO support_ticket_messages (ticket_id, sender_id, sender_role, source, content, external_id) VALUES ($1, NULLIF($2::bigint, 0), $3, $4, $5, NULLIF($6, '')) RETURNING id, created_at`, message.TicketID, message.SenderID, message.SenderRole, message.Source, message.Content, message.ExternalID).Scan(&message.ID, &message.CreatedAt)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.Constraint == "idx_support_ticket_messages_external" {
			return service.ErrSupportDuplicateMessage
		}
		return err
	}
	if len(attachmentIDs) > 0 {
		// 仅绑定当前发送者尚未使用的附件；绑定数量不一致时整条消息和通知一并回滚。
		result, err := tx.ExecContext(ctx, `UPDATE support_ticket_attachments SET message_id = $1 WHERE id = ANY($2) AND owner_id = $3 AND message_id IS NULL AND created_at > NOW() - INTERVAL '24 hours'`, message.ID, pq.Array(attachmentIDs), message.SenderID)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != int64(len(attachmentIDs)) {
			return service.ErrSupportAttachmentInvalid
		}
	}
	for i := range message.Attachments {
		a := &message.Attachments[i]
		if message.Source != "telegram" {
			return service.ErrSupportAttachmentInvalid
		}
		err := tx.QueryRowContext(ctx, `INSERT INTO support_ticket_attachments (owner_id, message_id, file_name, mime_type, size, data) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`, a.OwnerID, message.ID, a.FileName, a.MimeType, a.Size, a.Data).Scan(&a.ID, &a.CreatedAt)
		if err != nil {
			return err
		}
		a.MessageID = message.ID
		a.TicketID = message.TicketID
	}
	if _, err := tx.ExecContext(ctx, `UPDATE support_tickets SET updated_at = NOW(), last_message_at = NOW() WHERE id = $1`, message.TicketID); err != nil {
		return err
	}
	// Telegram 来源不再发回 Telegram，避免通知循环；邮件始终经过可靠队列。
	_, err = tx.ExecContext(ctx, `INSERT INTO support_ticket_notifications (ticket_id, message_id, channel) SELECT $1, $2, channel FROM (VALUES ('email'), ('telegram')) AS channels(channel) WHERE channel = 'email' OR $3 <> 'telegram'`, message.TicketID, message.ID, message.Source)
	return err
}

func (r *supportTicketRepository) SetStatus(ctx context.Context, ticketID int64, status string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE support_tickets SET status = $2, updated_at = NOW() WHERE id = $1`, ticketID, status)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return service.ErrSupportTicketNotFound
	}
	return nil
}

func (r *supportTicketRepository) CreateAttachment(ctx context.Context, a *service.SupportTicketAttachment) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var ownerID int64
	// 用户行锁使草稿数量限制在并发上传时仍然有效。
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id = $1 AND deleted_at IS NULL FOR NO KEY UPDATE`, a.OwnerID).Scan(&ownerID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM support_ticket_attachments WHERE owner_id = $1 AND message_id IS NULL AND created_at <= NOW() - INTERVAL '24 hours'`, a.OwnerID); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(id) FROM support_ticket_attachments WHERE owner_id = $1 AND message_id IS NULL`, a.OwnerID).Scan(&count); err != nil {
		return err
	}
	if count >= 20 {
		return service.ErrSupportAttachmentInvalid
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO support_ticket_attachments (owner_id, file_name, mime_type, size, data) VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`, a.OwnerID, a.FileName, a.MimeType, a.Size, a.Data).Scan(&a.ID, &a.CreatedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *supportTicketRepository) GetAttachment(ctx context.Context, id int64) (*service.SupportTicketAttachment, error) {
	a := &service.SupportTicketAttachment{}
	err := r.db.QueryRowContext(ctx, `SELECT a.id, a.owner_id, COALESCE(a.message_id, 0), COALESCE(m.ticket_id, 0), a.file_name, a.mime_type, a.size, a.data, a.created_at FROM support_ticket_attachments a LEFT JOIN support_ticket_messages m ON m.id = a.message_id WHERE a.id = $1`, id).Scan(&a.ID, &a.OwnerID, &a.MessageID, &a.TicketID, &a.FileName, &a.MimeType, &a.Size, &a.Data, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSupportAttachmentNotFound
	}
	return a, err
}

func (r *supportTicketRepository) DeleteDraftAttachment(ctx context.Context, ownerID, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM support_ticket_attachments WHERE id = $1 AND owner_id = $2 AND message_id IS NULL`, id, ownerID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return service.ErrSupportAttachmentNotFound
	}
	return nil
}

func (r *supportTicketRepository) GetNotificationMessage(ctx context.Context, messageID int64) (*service.SupportTicket, *service.SupportTicketMessage, error) {
	m, err := scanSupportMessage(r.db.QueryRowContext(ctx, `SELECT `+supportMessageColumns+` FROM support_ticket_messages WHERE id = $1`, messageID))
	if err != nil {
		return nil, nil, err
	}
	messages := []service.SupportTicketMessage{*m}
	if err := r.loadAttachments(ctx, messages); err != nil {
		return nil, nil, err
	}
	t, err := r.GetTicket(ctx, m.TicketID)
	return t, &messages[0], err
}

func (r *supportTicketRepository) GetMessageByExternalID(ctx context.Context, externalID string) (*service.SupportTicketMessage, error) {
	m, err := scanSupportMessage(r.db.QueryRowContext(ctx, `SELECT `+supportMessageColumns+` FROM support_ticket_messages WHERE external_id = $1`, externalID))
	if err != nil {
		return nil, err
	}
	messages := []service.SupportTicketMessage{*m}
	if err := r.loadAttachments(ctx, messages); err != nil {
		return nil, err
	}
	return &messages[0], nil
}

func (r *supportTicketRepository) ClaimNotifications(ctx context.Context, limit int, lease time.Duration) ([]service.SupportTicketNotification, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	seconds := int64(lease / time.Second)
	if seconds < 1 {
		seconds = 60
	}
	token := uuid.NewString()
	rows, err := r.db.QueryContext(ctx, `WITH pending AS (SELECT id FROM support_ticket_notifications WHERE delivered_at IS NULL AND available_at <= NOW() AND (lease_until IS NULL OR lease_until < NOW()) ORDER BY available_at, id LIMIT $1 FOR UPDATE SKIP LOCKED) UPDATE support_ticket_notifications n SET lease_token = $2, lease_until = NOW() + ($3 * INTERVAL '1 second'), attempts = n.attempts + 1 FROM pending p WHERE n.id = p.id RETURNING n.id, n.ticket_id, n.message_id, n.channel, n.attempts, n.lease_token`, limit, token, seconds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []service.SupportTicketNotification{}
	for rows.Next() {
		var n service.SupportTicketNotification
		if err := rows.Scan(&n.ID, &n.TicketID, &n.MessageID, &n.Channel, &n.Attempts, &n.LeaseToken); err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	return items, rows.Err()
}

func (r *supportTicketRepository) CompleteNotification(ctx context.Context, id int64, leaseToken string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE support_ticket_notifications SET delivered_at = NOW(), lease_until = NULL, lease_token = NULL, last_error = NULL WHERE id = $1 AND lease_token = $2 AND delivered_at IS NULL`, id, leaseToken)
	return supportLeaseResult(result, err)
}

func (r *supportTicketRepository) FailNotification(ctx context.Context, id int64, leaseToken, lastError string, retryAt time.Time) error {
	runes := []rune(lastError)
	if len(runes) > 1000 {
		lastError = string(runes[:1000])
	}
	result, err := r.db.ExecContext(ctx, `UPDATE support_ticket_notifications SET available_at = $3, last_error = $4, lease_until = NULL, lease_token = NULL WHERE id = $1 AND lease_token = $2 AND delivered_at IS NULL`, id, leaseToken, retryAt, lastError)
	return supportLeaseResult(result, err)
}

func supportLeaseResult(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("工单通知租约已失效")
	}
	return nil
}

func (r *supportTicketRepository) HasNotificationReceipt(ctx context.Context, notificationID int64, key string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM support_ticket_notification_receipts WHERE notification_id = $1 AND delivery_key = $2)`, notificationID, key).Scan(&exists)
	return exists, err
}

func (r *supportTicketRepository) SaveNotificationReceipt(ctx context.Context, notificationID int64, key string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO support_ticket_notification_receipts (notification_id, delivery_key) VALUES ($1, $2) ON CONFLICT (notification_id, delivery_key) DO NOTHING`, notificationID, key)
	return err
}

func (r *supportTicketRepository) SaveTelegramMapping(ctx context.Context, chatID, messageID, ticketID int64) error {
	if chatID == 0 || messageID <= 0 || ticketID <= 0 {
		return service.ErrSupportTelegramUnmapped
	}
	result, err := r.db.ExecContext(ctx, `INSERT INTO support_ticket_telegram_messages (chat_id, message_id, ticket_id) VALUES ($1, $2, $3) ON CONFLICT (chat_id, message_id) DO UPDATE SET ticket_id = EXCLUDED.ticket_id WHERE support_ticket_telegram_messages.ticket_id = EXCLUDED.ticket_id`, chatID, messageID, ticketID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return service.ErrSupportTelegramUnmapped
	}
	return nil
}

func (r *supportTicketRepository) FindTelegramTicket(ctx context.Context, chatID, messageID int64) (int64, error) {
	var ticketID int64
	err := r.db.QueryRowContext(ctx, `SELECT ticket_id FROM support_ticket_telegram_messages WHERE chat_id = $1 AND message_id = $2`, chatID, messageID).Scan(&ticketID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, service.ErrSupportTelegramUnmapped
	}
	return ticketID, err
}
