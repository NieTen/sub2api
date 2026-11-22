-- 保留投递结果不明的收件人，供管理员核对 SMTP 记录。
ALTER TABLE bulk_email_recipients
    DROP CONSTRAINT bulk_email_recipients_status_check,
    ADD CONSTRAINT bulk_email_recipients_status_check
        CHECK (status IN ('pending','sending','sent','failed','unconfirmed'));

ALTER TABLE bulk_email_batches
    DROP CONSTRAINT bulk_email_batches_status_check,
    ADD CONSTRAINT bulk_email_batches_status_check
        CHECK (status IN ('draft','queued','sending','completed','partial_failed','failed','needs_review'));
