-- 管理员模型检测独立保存计划、执行快照与证据，不改变既有连通性测试。
CREATE TABLE IF NOT EXISTS model_detection_plans (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    model_id VARCHAR(200) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    schedule_type VARCHAR(16) NOT NULL DEFAULT 'interval' CHECK (schedule_type IN ('interval','daily')),
    interval_minutes INTEGER NOT NULL DEFAULT 1440 CHECK (interval_minutes BETWEEN 1 AND 10080),
    daily_time VARCHAR(5) NOT NULL DEFAULT '00:00',
    timezone VARCHAR(100) NOT NULL DEFAULT 'Asia/Shanghai',
    reference_model VARCHAR(200) NOT NULL DEFAULT '',
    drop_threshold DOUBLE PRECISION NOT NULL DEFAULT 20 CHECK (drop_threshold > 0 AND drop_threshold <= 100),
    max_results INTEGER NOT NULL DEFAULT 100 CHECK (max_results BETWEEN 100 AND 1000),
    baseline_score DOUBLE PRECISION,
    baseline_version VARCHAR(100) NOT NULL DEFAULT '',
    baseline_run_id BIGINT,
    baseline_generation BIGINT NOT NULL DEFAULT 1,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_model_detection_plans_account ON model_detection_plans(account_id,id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_model_detection_plans_account_model ON model_detection_plans(account_id,model_id);
CREATE INDEX IF NOT EXISTS idx_model_detection_plans_due ON model_detection_plans(next_run_at,id) WHERE enabled=TRUE;

CREATE TABLE IF NOT EXISTS model_detection_runs (
    id BIGSERIAL PRIMARY KEY,
    plan_id BIGINT NOT NULL REFERENCES model_detection_plans(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    account_name VARCHAR(255) NOT NULL,
    model_id VARCHAR(200) NOT NULL,
    status VARCHAR(16) NOT NULL CHECK (status IN ('queued','running','completed','error','inconclusive')),
    verdict VARCHAR(32) NOT NULL DEFAULT '',
    score DOUBLE PRECISION,
    baseline_score DOUBLE PRECISION,
    drop_points DOUBLE PRECISION,
    fingerprint JSONB NOT NULL DEFAULT '{}',
    details JSONB NOT NULL DEFAULT '[]',
    error_message TEXT NOT NULL DEFAULT '',
    suite_version VARCHAR(100) NOT NULL,
    progress INTEGER NOT NULL DEFAULT 0,
    requests_total INTEGER NOT NULL DEFAULT 4,
    trigger VARCHAR(16) NOT NULL,
    plan_snapshot JSONB NOT NULL,
    lease_token VARCHAR(64),
    lease_until TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- 一个计划同时最多有一个等待或运行任务，手动点击和定时扫描使用同一约束。
CREATE UNIQUE INDEX IF NOT EXISTS idx_model_detection_runs_active_plan ON model_detection_runs(plan_id) WHERE status IN ('queued','running');
CREATE INDEX IF NOT EXISTS idx_model_detection_runs_queue ON model_detection_runs(id) WHERE status='queued';
CREATE INDEX IF NOT EXISTS idx_model_detection_runs_lease ON model_detection_runs(lease_until,id) WHERE status='running';
CREATE INDEX IF NOT EXISTS idx_model_detection_runs_account ON model_detection_runs(account_id,id DESC);
CREATE INDEX IF NOT EXISTS idx_model_detection_runs_plan_history ON model_detection_runs(plan_id,id DESC);
CREATE INDEX IF NOT EXISTS idx_model_detection_runs_finished ON model_detection_runs(finished_at DESC,id DESC) WHERE finished_at IS NOT NULL;
