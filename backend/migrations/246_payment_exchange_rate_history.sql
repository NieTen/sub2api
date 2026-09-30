-- 每次采集均保留当时的报价或兜底结果，历史失败不能伪装成成功价格。
CREATE TABLE IF NOT EXISTS payment_exchange_rate_history (
    id BIGSERIAL PRIMARY KEY,
    rate NUMERIC(18,8) NOT NULL,
    source VARCHAR(16) NOT NULL CHECK (source IN ('okx','fallback','unavailable')),
    fetched_at TIMESTAMPTZ NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    fallback_reason TEXT NOT NULL DEFAULT '',
    sample_prices JSONB NOT NULL DEFAULT '[]'::jsonb,
    sample_count INTEGER NOT NULL CHECK (sample_count BETWEEN 0 AND 10),
    aggregation VARCHAR(48) NOT NULL,
    CHECK ((source='unavailable' AND rate=0) OR (source IN ('okx','fallback') AND rate BETWEEN 1 AND 100)),
    CHECK (jsonb_typeof(sample_prices)='array' AND jsonb_array_length(sample_prices)=sample_count),
    CHECK (source<>'okx' OR (sample_count>0 AND aggregation='median_first_10_sell'))
);
-- 同时支持查最新状态与最近 72 小时的时间范围查询。
CREATE INDEX IF NOT EXISTS idx_payment_exchange_rate_history_time
    ON payment_exchange_rate_history(fetched_at DESC,id DESC);
