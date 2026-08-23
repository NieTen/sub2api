-- Codex 额度透支分组接力配置。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS codex_overdraft_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS codex_overdraft_next_group_id BIGINT NULL;

COMMENT ON COLUMN groups.codex_overdraft_enabled IS '是否允许该 OpenAI 分组参与 Codex 额度透支调度';
COMMENT ON COLUMN groups.codex_overdraft_next_group_id IS '当前分组无可透支账号时接力查询的 OpenAI 分组 ID';

CREATE INDEX IF NOT EXISTS idx_groups_codex_overdraft_enabled_active
    ON groups (codex_overdraft_enabled)
    WHERE codex_overdraft_enabled = TRUE AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_groups_codex_overdraft_next_group_id_active
    ON groups (codex_overdraft_next_group_id)
    WHERE codex_overdraft_next_group_id IS NOT NULL AND deleted_at IS NULL;
