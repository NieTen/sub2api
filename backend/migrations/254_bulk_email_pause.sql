-- 暂停状态保存在批次行，领取和回写通过同一批次锁串行化。
ALTER TABLE bulk_email_batches
    DROP CONSTRAINT bulk_email_batches_status_check,
    ADD CONSTRAINT bulk_email_batches_status_check
        CHECK (status IN ('draft','queued','sending','paused','completed','partial_failed','failed','needs_review'));
