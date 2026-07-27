-- 0052 — otomatik yanıtlayıcı ve Sieve posta filtreleri.
CREATE TABLE IF NOT EXISTS mail_autoresponders (
  mailbox_id    BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  domain_id     BIGINT UNSIGNED NOT NULL,
  enabled       TINYINT(1) NOT NULL DEFAULT 1,
  subject_text  VARCHAR(255) NOT NULL,
  body_text     TEXT NOT NULL,
  interval_days INT NOT NULL DEFAULT 7,
  updated_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY ix_autoresponder_domain (domain_id),
  CONSTRAINT fk_autoresponder_mailbox FOREIGN KEY (mailbox_id) REFERENCES mailboxes(id) ON DELETE CASCADE,
  CONSTRAINT fk_autoresponder_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS mail_filters (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  mailbox_id   BIGINT UNSIGNED NOT NULL,
  domain_id    BIGINT UNSIGNED NOT NULL,
  name         VARCHAR(128) NOT NULL,
  match_field  ENUM('from','to','subject') NOT NULL,
  match_value  VARCHAR(255) NOT NULL,
  action_type  ENUM('move','redirect','discard') NOT NULL,
  action_value VARCHAR(320) NOT NULL DEFAULT '',
  priority_n   INT NOT NULL DEFAULT 100,
  enabled      TINYINT(1) NOT NULL DEFAULT 1,
  created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  KEY ix_mail_filter_mailbox (mailbox_id, priority_n),
  KEY ix_mail_filter_domain (domain_id),
  CONSTRAINT fk_mail_filter_mailbox FOREIGN KEY (mailbox_id) REFERENCES mailboxes(id) ON DELETE CASCADE,
  CONSTRAINT fk_mail_filter_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
