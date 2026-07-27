-- 0053 — SMTP AUTH hesap bazlı gönderim limitleri ve otomatik askıya alma.
ALTER TABLE mailboxes ADD COLUMN IF NOT EXISTS send_limit_hour INT NOT NULL DEFAULT 100;
ALTER TABLE mailboxes ADD COLUMN IF NOT EXISTS send_limit_day  INT NOT NULL DEFAULT 500;
ALTER TABLE mailboxes ADD COLUMN IF NOT EXISTS spam_suspended_at TIMESTAMP NULL DEFAULT NULL;
ALTER TABLE mail_send_log ADD COLUMN IF NOT EXISTS recipient_count INT NOT NULL DEFAULT 1;
ALTER TABLE mail_send_log ADD INDEX IF NOT EXISTS ix_sendlog_domain_ts (domain_id, ts);
