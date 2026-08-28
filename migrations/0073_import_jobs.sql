-- 0073 - Web sitesi ice aktarim isleri ve ilerleme gecmisi
CREATE TABLE IF NOT EXISTS import_jobs (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  tur ENUM('files','database') NOT NULL,
  durum ENUM('queued','running','success','failed','rolled_back') NOT NULL DEFAULT 'queued',
  ilerleme TINYINT UNSIGNED NOT NULL DEFAULT 0,
  adim VARCHAR(128) NOT NULL DEFAULT '',
  hedef VARCHAR(255) NOT NULL DEFAULT '',
  mesaj TEXT NULL,
  recovery_file VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  started_at TIMESTAMP NULL,
  finished_at TIMESTAMP NULL,
  KEY ix_import_jobs_domain (domain_id, id),
  CONSTRAINT fk_import_jobs_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB;
