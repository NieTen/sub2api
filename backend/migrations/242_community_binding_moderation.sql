-- 已确认身份与首次真实入群都形成持久绑定；临时入群预留仍可以安全回收。
ALTER TABLE community_memberships ADD COLUMN IF NOT EXISTS bound_at TIMESTAMPTZ;
UPDATE community_memberships m
SET bound_at = COALESCE(m.joined_at, (
    SELECT MIN(c.updated_at) FROM community_challenges c
    WHERE c.user_id=m.user_id AND c.telegram_user_id=m.telegram_user_id AND c.status='confirmed'
))
WHERE m.bound_at IS NULL AND (m.joined_at IS NOT NULL OR EXISTS (
    SELECT 1 FROM community_challenges c
    WHERE c.user_id=m.user_id AND c.telegram_user_id=m.telegram_user_id AND c.status='confirmed'
));
ALTER TABLE community_memberships DROP CONSTRAINT IF EXISTS community_memberships_status_check;
ALTER TABLE community_memberships ADD CONSTRAINT community_memberships_status_check
    CHECK (status IN ('pending','joined','left','banned'));

-- 禁入独立于绑定保存，管理员解绑不会清除网站账号或 Telegram 身份的禁入记录。
CREATE TABLE IF NOT EXISTS community_bans (
    group_chat_id BIGINT NOT NULL,
    telegram_user_id BIGINT NOT NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    actor_id BIGINT NOT NULL,
    last_event_date BIGINT NOT NULL,
    last_update_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_chat_id,telegram_user_id)
);
CREATE INDEX IF NOT EXISTS idx_community_ban_user_group ON community_bans(user_id,group_chat_id) WHERE user_id IS NOT NULL;

-- 工单只能消费一次，审计记录保留原始编号，不因账号或工单删除而丢失依据。
CREATE TABLE IF NOT EXISTS community_unbind_audits (
    id BIGSERIAL PRIMARY KEY,
    admin_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    telegram_user_id BIGINT NOT NULL,
    group_chat_id BIGINT NOT NULL,
    ticket_id BIGINT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_community_unbind_user_created ON community_unbind_audits(user_id,created_at DESC);
