-- 工单、图片附件及可靠通知队列；图片存储于数据库以兼容多实例部署。
CREATE TABLE IF NOT EXISTS support_tickets (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subject VARCHAR(200) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_message_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_support_tickets_user_activity ON support_tickets(user_id, last_message_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_support_tickets_status_activity ON support_tickets(status, last_message_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_support_tickets_activity ON support_tickets(last_message_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS support_ticket_messages (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    sender_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    sender_role VARCHAR(16) NOT NULL CHECK (sender_role IN ('user', 'admin')),
    source VARCHAR(16) NOT NULL CHECK (source IN ('web', 'telegram')),
    content TEXT NOT NULL DEFAULT '' CHECK (char_length(content) <= 20000),
    external_id VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_support_ticket_messages_ticket ON support_ticket_messages(ticket_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_support_ticket_messages_external ON support_ticket_messages(external_id) WHERE external_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS support_ticket_attachments (
    id BIGSERIAL PRIMARY KEY,
    owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id BIGINT REFERENCES support_ticket_messages(id) ON DELETE CASCADE,
    file_name VARCHAR(200) NOT NULL,
    mime_type VARCHAR(32) NOT NULL CHECK (mime_type IN ('image/png', 'image/jpeg', 'image/gif', 'image/webp')),
    size INTEGER NOT NULL CHECK (size > 0 AND size <= 5242880),
    data BYTEA NOT NULL CHECK (octet_length(data) = size),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_support_ticket_attachments_message ON support_ticket_attachments(message_id) WHERE message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_support_ticket_attachments_drafts ON support_ticket_attachments(owner_id, created_at) WHERE message_id IS NULL;

CREATE TABLE IF NOT EXISTS support_ticket_notifications (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    message_id BIGINT NOT NULL REFERENCES support_ticket_messages(id) ON DELETE CASCADE,
    channel VARCHAR(16) NOT NULL CHECK (channel IN ('email', 'telegram')),
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    lease_token VARCHAR(36),
    delivered_at TIMESTAMPTZ,
    last_error VARCHAR(1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (message_id, channel)
);
CREATE INDEX IF NOT EXISTS idx_support_ticket_notifications_pending ON support_ticket_notifications(available_at, id) WHERE delivered_at IS NULL;

-- 分段发送的成功回执，失败重试时仅补发尚未完成的收件人或图片。
CREATE TABLE IF NOT EXISTS support_ticket_notification_receipts (
    notification_id BIGINT NOT NULL REFERENCES support_ticket_notifications(id) ON DELETE CASCADE,
    delivery_key VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (notification_id, delivery_key)
);

-- 只能回复机器人已发送并记录的消息，不能通过猜测工单编号建立映射。
CREATE TABLE IF NOT EXISTS support_ticket_telegram_messages (
    chat_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    ticket_id BIGINT NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, message_id)
);
CREATE INDEX IF NOT EXISTS idx_support_ticket_telegram_ticket ON support_ticket_telegram_messages(ticket_id);
