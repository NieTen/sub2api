-- 精确链上金额独立保留为微 USDT，不改变原有两位金额账本。
CREATE TABLE IF NOT EXISTS payment_trc20_intents (
    order_id BIGINT PRIMARY KEY REFERENCES payment_orders(id),
    out_trade_no VARCHAR(128) NOT NULL UNIQUE,
    provider_instance_id VARCHAR(128) NOT NULL,
    wallet_address VARCHAR(34) NOT NULL,
    base_amount DECIMAL(20,2) NOT NULL CHECK (base_amount>0),
    amount_units BIGINT NOT NULL CHECK (amount_units>0),
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL CHECK (expires_at>created_at),
    transaction_hash VARCHAR(64) UNIQUE,
    transferred_at TIMESTAMPTZ,
    checked_at TIMESTAMPTZ,
    credited_at TIMESTAMPTZ,
    -- 永久不复用精确金额，避免旧订单超时转账误入新用户订单。
    UNIQUE(wallet_address,amount_units),
    CHECK ((transaction_hash IS NULL)=(transferred_at IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_payment_trc20_pending_check
    ON payment_trc20_intents(checked_at NULLS FIRST,order_id) WHERE credited_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_payment_trc20_expiry
    ON payment_trc20_intents(expires_at) WHERE credited_at IS NULL AND transaction_hash IS NULL;
