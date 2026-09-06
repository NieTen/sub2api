-- 批量邮件先创建草稿并冻结收件人，管理员确认后才开始发送。
CREATE TABLE IF NOT EXISTS bulk_email_batches (
    id BIGSERIAL PRIMARY KEY,
    created_by BIGINT NOT NULL REFERENCES users(id),
    subject VARCHAR(800) NOT NULL,
    body TEXT NOT NULL,
    images JSONB NOT NULL DEFAULT '[]'::jsonb,
    image_count INTEGER NOT NULL DEFAULT 0 CHECK (image_count BETWEEN 0 AND 4),
    status VARCHAR(32) NOT NULL DEFAULT 'draft',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT bulk_email_batches_status_check CHECK (status IN ('draft','queued','sending','completed','partial_failed','failed'))
);

CREATE TABLE IF NOT EXISTS bulk_email_recipients (
    id BIGSERIAL PRIMARY KEY,
    batch_id BIGINT NOT NULL REFERENCES bulk_email_batches(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    email VARCHAR(320) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    lease_token VARCHAR(64) NOT NULL DEFAULT '',
    sent_at TIMESTAMPTZ,
    CONSTRAINT bulk_email_recipients_status_check CHECK (status IN ('pending','sending','sent','failed')),
    CONSTRAINT bulk_email_recipients_unique_email UNIQUE (batch_id,email)
);

CREATE INDEX IF NOT EXISTS idx_bulk_email_batches_created ON bulk_email_batches(created_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS idx_bulk_email_recipients_pending ON bulk_email_recipients(next_attempt_at,id) WHERE status IN ('pending','sending');
CREATE INDEX IF NOT EXISTS idx_bulk_email_recipients_batch_status ON bulk_email_recipients(batch_id,status,id);
