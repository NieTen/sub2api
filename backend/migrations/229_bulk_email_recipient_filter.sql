-- 保存创建草稿时的筛选条件；旧批次保留原有收件人快照，空对象表示不限。
ALTER TABLE bulk_email_batches
    ADD COLUMN IF NOT EXISTS recipient_filter JSONB NOT NULL DEFAULT '{}'::jsonb;

-- 当前余额在计费时频繁更新，群发筛选仅在生成草稿时执行一次。
-- 复用 users 的状态/删除标记索引，避免新增余额索引增加日常计费写入开销。
-- 充值条件通过 payment_orders 的现有 user_id 索引定位用户订单。
