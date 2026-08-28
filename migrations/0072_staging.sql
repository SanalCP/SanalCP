-- 0072 - Gercek staging ortamlari
CREATE TABLE IF NOT EXISTS staging_environments (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  source_domain_id BIGINT UNSIGNED NOT NULL,
  staging_domain_id BIGINT UNSIGNED NOT NULL UNIQUE,
  durum ENUM('hazir','isleniyor','hata') NOT NULL DEFAULT 'hazir',
  son_push_at TIMESTAMP NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_staging_source (source_domain_id),
  CONSTRAINT fk_staging_source FOREIGN KEY (source_domain_id) REFERENCES domains(id) ON DELETE CASCADE,
  CONSTRAINT fk_staging_target FOREIGN KEY (staging_domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB;
