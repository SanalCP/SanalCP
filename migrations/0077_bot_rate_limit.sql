CREATE TABLE IF NOT EXISTS domain_rate_limits (
  domain_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  profil ENUM('kapali','dengeli','siki','ozel') NOT NULL DEFAULT 'kapali',
  istek_dakika SMALLINT UNSIGNED NOT NULL DEFAULT 120,
  burst SMALLINT UNSIGNED NOT NULL DEFAULT 30,
  bot_engelle TINYINT(1) NOT NULL DEFAULT 0,
  ip_istisnalari TEXT NULL,
  yol_istisnalari TEXT NULL,
  guncellenme TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_domain_rate_limits_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE panel_ayarlari
  ADD COLUMN IF NOT EXISTS hiz_profili ENUM('kapali','dengeli','siki','ozel') NOT NULL DEFAULT 'dengeli',
  ADD COLUMN IF NOT EXISTS hiz_istek_dakika SMALLINT UNSIGNED NOT NULL DEFAULT 600,
  ADD COLUMN IF NOT EXISTS hiz_burst SMALLINT UNSIGNED NOT NULL DEFAULT 100,
  ADD COLUMN IF NOT EXISTS hiz_ip_istisnalari TEXT NULL;

CREATE TABLE IF NOT EXISTS panel_hiz_olaylari (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  ip VARCHAR(45) NOT NULL,
  yol VARCHAR(255) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY ix_panel_hiz_olay_zaman (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS remote_transfer_jobs (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  provider ENUM('cpanel','plesk','directadmin') NOT NULL,
  source_host VARCHAR(253) NOT NULL,
  source_port SMALLINT UNSIGNED NOT NULL DEFAULT 22,
  source_account VARCHAR(64) NOT NULL,
  source_domain VARCHAR(253) NOT NULL,
  customer_id BIGINT UNSIGNED NOT NULL,
  plan_id BIGINT UNSIGNED NULL,
  php_version VARCHAR(8) NOT NULL DEFAULT '8.3',
  status ENUM('queued','packaging','downloading','importing','success','failed') NOT NULL DEFAULT 'queued',
  progress TINYINT UNSIGNED NOT NULL DEFAULT 0,
  message TEXT NULL,
  target_domain_id BIGINT UNSIGNED NULL,
  source_http_status SMALLINT UNSIGNED NULL,
  target_http_status SMALLINT UNSIGNED NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  started_at TIMESTAMP NULL,
  finished_at TIMESTAMP NULL,
  KEY ix_remote_transfer_status (status,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
