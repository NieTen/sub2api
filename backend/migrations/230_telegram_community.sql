-- 社群入群必须先在机器人声明身份，再由登录用户在网页确认。
CREATE TABLE IF NOT EXISTS community_challenges (
    id VARCHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bot_id BIGINT NOT NULL,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    telegram_user_id BIGINT,
    telegram_username VARCHAR(256) NOT NULL DEFAULT '',
    telegram_name VARCHAR(512) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting','claimed','confirmed','superseded')),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_community_challenge_current ON community_challenges(user_id) WHERE status IN ('waiting','claimed');
CREATE INDEX IF NOT EXISTS idx_community_challenge_user ON community_challenges(user_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_community_challenge_cleanup ON community_challenges(expires_at,id);

-- 撤销任务独立于会员状态保留，入群成功后撤链失败仍能继续重试。
CREATE TABLE IF NOT EXISTS community_invites (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bot_id BIGINT NOT NULL,
    telegram_user_id BIGINT NOT NULL,
    group_chat_id BIGINT NOT NULL,
    url TEXT NOT NULL,
    url_hash VARCHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoke_pending','revoked')),
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_token VARCHAR(64),
    lease_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_community_invite_user ON community_invites(user_id,id DESC);
CREATE INDEX IF NOT EXISTS idx_community_invite_revoke ON community_invites(available_at,id) WHERE status='revoke_pending';
CREATE INDEX IF NOT EXISTS idx_community_invite_expiry ON community_invites(expires_at,id) WHERE status='active';
CREATE INDEX IF NOT EXISTS idx_community_invite_cleanup ON community_invites(revoked_at,id) WHERE status='revoked';

-- pending 是加入前的唯一身份预留；离群后仍保留归属，禁止另一网站账号抢绑。
CREATE TABLE IF NOT EXISTS community_memberships (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    telegram_user_id BIGINT NOT NULL UNIQUE,
    telegram_username VARCHAR(256) NOT NULL DEFAULT '',
    telegram_name VARCHAR(512) NOT NULL DEFAULT '',
    group_chat_id BIGINT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','joined','left')),
    joined_at TIMESTAMPTZ,
    authorized_invite_id BIGINT REFERENCES community_invites(id),
    last_event_date BIGINT NOT NULL DEFAULT 0,
    last_update_id BIGINT NOT NULL DEFAULT -1,
    invite_lease_token VARCHAR(64),
    invite_lease_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_community_membership_authorized_invite ON community_memberships(authorized_invite_id) WHERE authorized_invite_id IS NOT NULL;

-- 回调验签后先落盘；按机器人隔离 update_id，外部接口失败可在多实例间恢复。
CREATE TABLE IF NOT EXISTS community_webhook_events (
    id BIGSERIAL PRIMARY KEY,
    bot_id BIGINT NOT NULL,
    update_id BIGINT NOT NULL,
    payload JSONB NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_token VARCHAR(64),
    lease_until TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(bot_id,update_id)
);
CREATE INDEX IF NOT EXISTS idx_community_webhook_pending ON community_webhook_events(available_at,id) WHERE completed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_community_webhook_cleanup ON community_webhook_events(completed_at,id) WHERE completed_at IS NOT NULL;
