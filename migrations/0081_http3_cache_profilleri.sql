ALTER TABLE nginx_settings
  ADD COLUMN IF NOT EXISTS http3 TINYINT(1) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS cache_profili ENUM('kapali','genel','wordpress','prestashop','ozel') NOT NULL DEFAULT 'kapali';

CREATE TABLE IF NOT EXISTS performans_olcumleri (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  cache_profili VARCHAR(16) NOT NULL,
  ilk_ttfb_ms INT UNSIGNED NOT NULL,
  ikinci_ttfb_ms INT UNSIGNED NOT NULL,
  ilk_cache VARCHAR(16) NOT NULL DEFAULT '',
  ikinci_cache VARCHAR(16) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY ix_perf_olcum_domain (domain_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
