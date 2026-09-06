-- 登录用户先领取个人邀请，首次申请加入时再原子登记 Telegram 身份。
ALTER TABLE community_invites
    ALTER COLUMN telegram_user_id DROP NOT NULL;

-- 未领取过邀请的用户没有会员记录，因此将生成链接的租约独立保存。
CREATE TABLE IF NOT EXISTS community_invite_leases (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    group_chat_id BIGINT NOT NULL,
    lease_token VARCHAR(64) NOT NULL,
    lease_until TIMESTAMPTZ NOT NULL
);

-- 兼容升级时尚未结束的旧租约，不更改已有 Telegram 归属和会员记录。
INSERT INTO community_invite_leases(user_id,group_chat_id,lease_token,lease_until)
SELECT user_id,group_chat_id,invite_lease_token,invite_lease_until
FROM community_memberships
WHERE invite_lease_token IS NOT NULL AND invite_lease_until>NOW()
ON CONFLICT(user_id) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_community_membership_group_status_user
    ON community_memberships(group_chat_id,status,user_id);
