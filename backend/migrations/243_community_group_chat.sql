-- 群消息只保存在管理员接口；编辑事件独立存档，避免覆盖原消息。
CREATE TABLE IF NOT EXISTS community_chat_messages (
    id BIGSERIAL PRIMARY KEY,
    bot_id BIGINT NOT NULL,
    group_chat_id BIGINT NOT NULL,
    update_id BIGINT,
    telegram_message_id BIGINT NOT NULL,
    telegram_user_id BIGINT NOT NULL DEFAULT 0,
    telegram_username VARCHAR(256) NOT NULL DEFAULT '',
    telegram_name VARCHAR(512) NOT NULL DEFAULT '',
    sender_kind VARCHAR(16) NOT NULL DEFAULT 'user',
    sender_chat_id BIGINT NOT NULL DEFAULT 0,
    is_bot BOOLEAN NOT NULL DEFAULT FALSE,
    message_type VARCHAR(32) NOT NULL,
    text TEXT NOT NULL DEFAULT '',
    file_id TEXT NOT NULL DEFAULT '',
    file_name TEXT NOT NULL DEFAULT '',
    mime_type VARCHAR(256) NOT NULL DEFAULT '',
    file_size BIGINT NOT NULL DEFAULT 0,
    reply_to_message_id BIGINT NOT NULL DEFAULT 0,
    outgoing BOOLEAN NOT NULL DEFAULT FALSE,
    admin_user_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL,
    edited_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(bot_id, update_id)
);
CREATE INDEX IF NOT EXISTS idx_community_chat_group_page ON community_chat_messages(group_chat_id,id DESC);
CREATE INDEX IF NOT EXISTS idx_community_chat_sender ON community_chat_messages(group_chat_id,telegram_user_id,id DESC);

-- 发消息先记录幂等键；网络结果未知时不自动重发，避免重复向群内广播。
CREATE TABLE IF NOT EXISTS community_chat_sends (
    id BIGSERIAL PRIMARY KEY,
    group_chat_id BIGINT NOT NULL,
    bot_id BIGINT NOT NULL,
    admin_user_id BIGINT NOT NULL REFERENCES users(id),
    client_request_id VARCHAR(64) NOT NULL,
    content_hash VARCHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'sending' CHECK(status IN ('sending','sent','failed','uncertain')),
    message_id BIGINT REFERENCES community_chat_messages(id),
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(group_chat_id,admin_user_id,client_request_id)
);
CREATE INDEX IF NOT EXISTS idx_community_chat_send_rate ON community_chat_sends(group_chat_id,created_at DESC);

-- 仅缓存小尺寸头像，不向浏览器暴露机器人令牌或 Telegram 文件路径。
CREATE TABLE IF NOT EXISTS community_chat_avatars (
    bot_id BIGINT NOT NULL,
    telegram_user_id BIGINT NOT NULL,
    mime_type VARCHAR(64) NOT NULL,
    data BYTEA NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY(bot_id,telegram_user_id)
);
