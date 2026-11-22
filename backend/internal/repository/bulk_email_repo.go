package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type bulkEmailRepository struct{ db *sql.DB }

func NewBulkEmailRepository(db *sql.DB) service.BulkEmailRepository {
	return &bulkEmailRepository{db: db}
}

func (r *bulkEmailRepository) CreateDraft(ctx context.Context, b *service.BulkEmailBatch, ids []int64, allActive bool) error {
	filter, err := b.RecipientFilter.Normalize()
	if err != nil {
		return err
	}
	b.RecipientFilter = filter
	filterJSON, err := json.Marshal(filter)
	if err != nil {
		return err
	}
	filterSQL, filterArgs := bulkEmailRecipientConditions(filter)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// 锁住将要消费的草稿，避免并发工单绑定与批量邮件创建同时使用同一张图片。
	if len(b.DraftAttachmentIDs) > 0 {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM support_ticket_attachments WHERE id=ANY($1::bigint[]) AND owner_id=$2 AND message_id IS NULL AND created_at>=NOW()-INTERVAL '24 hours' FOR UPDATE`, pq.Array(b.DraftAttachmentIDs), b.CreatedBy)
		if err != nil {
			return err
		}
		count := 0
		for rows.Next() {
			var id int64
			if err = rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			count++
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		if count != len(b.DraftAttachmentIDs) {
			return service.ErrSupportAttachmentInvalid
		}
	}
	if b.Images == nil {
		b.Images = []service.EmailInlineImage{}
	}
	b.ImageCount = len(b.Images)
	images, err := json.Marshal(b.Images)
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO bulk_email_batches(created_by,subject,body,images,image_count,recipient_filter) VALUES($1,$2,$3,$4::jsonb,$5,$6::jsonb) RETURNING id,created_at,updated_at`, b.CreatedBy, b.Subject, b.Body, string(images), b.ImageCount, string(filterJSON)).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return err
	}
	// 收件人快照和批次使用同一事务，确认页面看到的人数与实际发送一致。
	syntheticDomains := []string{"%" + service.LinuxDoConnectSyntheticEmailDomain, "%" + service.OIDCConnectSyntheticEmailDomain, "%" + service.WeChatConnectSyntheticEmailDomain, "%" + service.DingTalkConnectSyntheticEmailDomain}
	args := []any{b.ID, allActive, pq.Array(ids), pq.Array(syntheticDomains)}
	args = append(args, filterArgs...)
	result, err := tx.ExecContext(ctx, `INSERT INTO bulk_email_recipients(batch_id,user_id,email)
		SELECT $1,u.id,u.email FROM (SELECT DISTINCT ON (LOWER(BTRIM(email))) id,LOWER(BTRIM(email)) AS email
		FROM users AS candidate WHERE deleted_at IS NULL AND status='active' AND BTRIM(email)<>'' AND ($2 OR id=ANY($3::bigint[])) AND NOT (LOWER(BTRIM(email)) LIKE ANY($4::text[]))`+filterSQL+` ORDER BY LOWER(BTRIM(email)),id) u`, args...)
	if err != nil {
		return err
	}
	b.TotalCount, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if b.TotalCount == 0 {
		return service.ErrBulkEmailNoRecipients
	}
	// 批次已保存二进制快照，可以释放当前管理员已消费的上传额度。
	if len(b.DraftAttachmentIDs) > 0 {
		if _, err = tx.ExecContext(ctx, `DELETE FROM support_ticket_attachments WHERE id=ANY($1::bigint[]) AND owner_id=$2 AND message_id IS NULL`, pq.Array(b.DraftAttachmentIDs), b.CreatedBy); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const bulkBatchProjection = `b.id,b.created_by,b.subject,b.body,b.status,b.created_at,b.updated_at,b.image_count,
	(SELECT COUNT(r.id) FROM bulk_email_recipients r WHERE r.batch_id=b.id),
	(SELECT COUNT(r.id) FROM bulk_email_recipients r WHERE r.batch_id=b.id AND r.status='sent'),
	(SELECT COUNT(r.id) FROM bulk_email_recipients r WHERE r.batch_id=b.id AND r.status='failed'),
	(SELECT COUNT(r.id) FROM bulk_email_recipients r WHERE r.batch_id=b.id AND r.status='unconfirmed'),b.recipient_filter`

func (r *bulkEmailRepository) List(ctx context.Context, page, pageSize int) ([]service.BulkEmailBatch, int64, error) {
	page, pageSize = bulkEmailPage(page, pageSize)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(id) FROM bulk_email_batches`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+bulkBatchProjection+` FROM bulk_email_batches b ORDER BY b.id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := []service.BulkEmailBatch{}
	for rows.Next() {
		var b service.BulkEmailBatch
		var filterJSON []byte
		if err = rows.Scan(&b.ID, &b.CreatedBy, &b.Subject, &b.Body, &b.Status, &b.CreatedAt, &b.UpdatedAt, &b.ImageCount, &b.TotalCount, &b.SentCount, &b.FailedCount, &b.UnconfirmedCount, &filterJSON); err != nil {
			return nil, 0, err
		}
		if err = decodeBulkEmailFilter(filterJSON, &b); err != nil {
			return nil, 0, err
		}
		items = append(items, b)
	}
	return items, total, rows.Err()
}

func (r *bulkEmailRepository) Get(ctx context.Context, id int64) (*service.BulkEmailBatch, error) {
	var b service.BulkEmailBatch
	var images []byte
	var filterJSON []byte
	err := r.db.QueryRowContext(ctx, `SELECT `+bulkBatchProjection+`,b.images FROM bulk_email_batches b WHERE b.id=$1`, id).Scan(&b.ID, &b.CreatedBy, &b.Subject, &b.Body, &b.Status, &b.CreatedAt, &b.UpdatedAt, &b.ImageCount, &b.TotalCount, &b.SentCount, &b.FailedCount, &b.UnconfirmedCount, &filterJSON, &images)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrBulkEmailNotFound
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(images, &b.Images); err != nil {
		return nil, err
	}
	if err = decodeBulkEmailFilter(filterJSON, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// 条件只从已校验的枚举生成，金额独立绑定为 numeric，筛选先于邮箱去重和快照保存。
func bulkEmailRecipientConditions(filter service.BulkEmailRecipientFilter) (string, []any) {
	condition := ""
	var args []any
	switch filter.BalanceCondition {
	case service.BulkEmailBalancePositive:
		condition += " AND candidate.balance > 0"
	case service.BulkEmailBalanceNonPositive:
		condition += " AND candidate.balance <= 0"
	case service.BulkEmailBalanceGreaterThan:
		condition += " AND candidate.balance > $5::numeric"
		args = append(args, *filter.BalanceThreshold)
	}
	if filter.RechargeCondition == service.BulkEmailRechargePaid {
		// paid_at 是支付验签、金额核对成功后写入的历史标记，后续退款不会抹除曾支付的事实。
		condition += " AND EXISTS (SELECT 1 FROM payment_orders po WHERE po.user_id = candidate.id AND po.order_type = 'balance' AND po.paid_at IS NOT NULL AND po.pay_amount > 0)"
	}
	return condition, args
}

func decodeBulkEmailFilter(raw []byte, batch *service.BulkEmailBatch) error {
	if err := json.Unmarshal(raw, &batch.RecipientFilter); err != nil {
		return err
	}
	filter, err := batch.RecipientFilter.Normalize()
	if err != nil {
		return err
	}
	batch.RecipientFilter = filter
	return nil
}

func bulkEmailPage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

func (r *bulkEmailRepository) Recipients(ctx context.Context, id int64, page, pageSize int) ([]service.BulkEmailRecipient, int64, error) {
	page, pageSize = bulkEmailPage(page, pageSize)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(id) FROM bulk_email_recipients WHERE batch_id=$1`, id).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,batch_id,user_id,email,status,attempts,last_error,sent_at FROM bulk_email_recipients WHERE batch_id=$1 ORDER BY id LIMIT $2 OFFSET $3`, id, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := []service.BulkEmailRecipient{}
	for rows.Next() {
		var v service.BulkEmailRecipient
		if err = rows.Scan(&v.ID, &v.BatchID, &v.UserID, &v.Email, &v.Status, &v.Attempts, &v.LastError, &v.SentAt); err != nil {
			return nil, 0, err
		}
		items = append(items, v)
	}
	return items, total, rows.Err()
}

func (r *bulkEmailRepository) StartBatch(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `UPDATE bulk_email_batches SET status='queued',updated_at=NOW() WHERE id=$1 AND status='draft'`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrBulkEmailState
	}
	return nil
}

func (r *bulkEmailRepository) Retry(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM bulk_email_batches WHERE id=$1 FOR UPDATE`, id).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return service.ErrBulkEmailNotFound
	} else if err != nil {
		return err
	}
	if status != "failed" && status != "partial_failed" && status != "needs_review" {
		return service.ErrBulkEmailState
	}
	result, err := tx.ExecContext(ctx, `UPDATE bulk_email_recipients SET status='pending',attempts=0,last_error='',next_attempt_at=NOW(),lease_until=NULL,lease_token='' WHERE batch_id=$1 AND status='failed'`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return service.ErrBulkEmailState
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_email_batches SET status='queued',updated_at=NOW() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

const (
	bulkEmailMaxAttempts        = 5
	bulkEmailUnconfirmedMessage = "投递结果待核对，请查阅 SMTP 服务商的发送记录。"
)

func (r *bulkEmailRepository) Claim(ctx context.Context, limit int, lease time.Duration) ([]service.BulkEmailRecipient, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// 与完成和手动重试保持相同锁序，事务提交后才允许调用 SMTP。
	var batchID int64
	err = tx.QueryRowContext(ctx, `SELECT b.id FROM bulk_email_recipients r JOIN bulk_email_batches b ON b.id=r.batch_id
		WHERE b.status IN ('queued','sending') AND (
			(r.status='pending' AND r.next_attempt_at<=NOW()) OR
			(r.status='sending' AND (r.lease_until<=NOW() OR r.lease_until IS NULL)))
		ORDER BY r.next_attempt_at,r.id LIMIT 1 FOR UPDATE OF b SKIP LOCKED`).Scan(&batchID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// 旧版本可能在 SMTP 已接收后一直未能写回。保留尝试次数，供管理员核对。
	_, err = tx.ExecContext(ctx, `UPDATE bulk_email_recipients SET status='unconfirmed',last_error=$2,lease_until=NULL,lease_token=''
		WHERE batch_id=$1 AND ((status='sending' AND (lease_until<=NOW() OR lease_until IS NULL))
			OR (status='pending' AND attempts>=$3))`, batchID, bulkEmailUnconfirmedMessage, bulkEmailMaxAttempts)
	if err != nil {
		return nil, err
	}
	items, err := claimPendingBulkEmails(ctx, tx, batchID, limit, lease)
	if err != nil {
		return nil, err
	}
	if err = refreshBulkEmailBatchStatus(ctx, tx, batchID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func claimPendingBulkEmails(ctx context.Context, tx *sql.Tx, batchID int64, limit int, lease time.Duration) ([]service.BulkEmailRecipient, error) {
	var key [16]byte
	if _, err := rand.Read(key[:]); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(key[:])
	rows, err := tx.QueryContext(ctx, `WITH picked AS (
		SELECT id FROM bulk_email_recipients WHERE batch_id=$1 AND status='pending' AND next_attempt_at<=NOW() AND attempts<$2
		ORDER BY next_attempt_at,id FOR UPDATE SKIP LOCKED LIMIT $3)
		UPDATE bulk_email_recipients r SET status='sending',attempts=r.attempts+1,lease_until=NOW()+($4*INTERVAL '1 second'),lease_token=$5
		FROM picked p WHERE r.id=p.id RETURNING r.id,r.batch_id,r.user_id,r.email,r.status,r.attempts,r.last_error,r.sent_at,r.lease_token`, batchID, bulkEmailMaxAttempts, limit, lease.Seconds(), token)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []service.BulkEmailRecipient{}
	for rows.Next() {
		var v service.BulkEmailRecipient
		if err = rows.Scan(&v.ID, &v.BatchID, &v.UserID, &v.Email, &v.Status, &v.Attempts, &v.LastError, &v.SentAt, &v.LeaseToken); err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, rows.Err()
}

func (r *bulkEmailRepository) Complete(ctx context.Context, item service.BulkEmailRecipient, delivery service.BulkEmailDeliveryResult, retryAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// 同批次完成步骤串行化，避免两个工作进程同时读到尚未提交的统计。
	var id int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM bulk_email_batches WHERE id=$1 FOR UPDATE`, item.BatchID).Scan(&id); err != nil {
		return err
	}
	status := string(delivery)
	lastError := ""
	switch delivery {
	case service.BulkEmailDeliverySent:
	case service.BulkEmailDeliveryFailed:
		lastError = "邮件发送失败，请检查 SMTP 配置或收件地址"
		if item.Attempts < bulkEmailMaxAttempts {
			status = "pending"
		}
	case service.BulkEmailDeliveryUnconfirmed:
		lastError = bulkEmailUnconfirmedMessage
	default:
		return service.ErrBulkEmailState
	}
	// 同一参数用于写入 varchar 列和比较时需指定类型，否则 PostgreSQL 会拒绝整条更新。
	result, err := tx.ExecContext(ctx, `UPDATE bulk_email_recipients SET status=$1::varchar,last_error=$2,next_attempt_at=$3,lease_until=NULL,lease_token='',
		sent_at=CASE WHEN $1::varchar='sent' THEN NOW() ELSE NULL END WHERE id=$4 AND lease_token=$5 AND status='sending' AND batch_id=$6`,
		status, lastError, retryAt, item.ID, item.LeaseToken, item.BatchID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return service.ErrBulkEmailState
	}
	if err = refreshBulkEmailBatchStatus(ctx, tx, item.BatchID); err != nil {
		return err
	}
	return tx.Commit()
}

func refreshBulkEmailBatchStatus(ctx context.Context, tx *sql.Tx, batchID int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE bulk_email_batches SET status=CASE
		WHEN EXISTS(SELECT 1 FROM bulk_email_recipients WHERE batch_id=$1 AND status IN ('pending','sending')) THEN 'sending'
		WHEN EXISTS(SELECT 1 FROM bulk_email_recipients WHERE batch_id=$1 AND status='unconfirmed') THEN 'needs_review'
		WHEN NOT EXISTS(SELECT 1 FROM bulk_email_recipients WHERE batch_id=$1 AND status='failed') THEN 'completed'
		WHEN EXISTS(SELECT 1 FROM bulk_email_recipients WHERE batch_id=$1 AND status='sent') THEN 'partial_failed' ELSE 'failed' END,
		updated_at=NOW() WHERE id=$1`, batchID)
	return err
}
