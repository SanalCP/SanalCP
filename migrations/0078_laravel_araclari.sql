CREATE TABLE IF NOT EXISTS laravel_deploy_jobs (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  dizin VARCHAR(255) NOT NULL DEFAULT 'public_html',
  tur ENUM('deploy','composer','artisan','queue','scheduler') NOT NULL,
  komut VARCHAR(100) NOT NULL,
  status ENUM('queued','running','success','failed','rolled_back') NOT NULL DEFAULT 'queued',
  progress TINYINT UNSIGNED NOT NULL DEFAULT 0,
  message TEXT NULL,
  output MEDIUMTEXT NULL,
  recovery_file VARCHAR(255) NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  started_at TIMESTAMP NULL,
  finished_at TIMESTAMP NULL,
  KEY ix_laravel_jobs_domain (domain_id,id),
  CONSTRAINT fk_laravel_jobs_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

