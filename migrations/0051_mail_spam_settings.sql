-- 0051 — domain bazlı Rspamd spam politikaları.
CREATE TABLE IF NOT EXISTS mail_spam_settings (
  domain_id        BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  enabled          TINYINT(1) NOT NULL DEFAULT 1,
  greylist_score   DECIMAL(5,2) NOT NULL DEFAULT 4.00,
  add_header_score DECIMAL(5,2) NOT NULL DEFAULT 6.00,
  reject_score     DECIMAL(5,2) NOT NULL DEFAULT 15.00,
  updated_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_mail_spam_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
