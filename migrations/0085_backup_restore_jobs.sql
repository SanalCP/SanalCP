-- Uzun yedek geri yüklemelerini HTTP isteğinden bağımsız, izlenebilir işler yapar.
CREATE TABLE IF NOT EXISTS backup_restore_jobs (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id BIGINT UNSIGNED NOT NULL,
  backup_id BIGINT UNSIGNED NOT NULL,
  scope ENUM('full','files','file','database','email') NOT NULL,
  target_path VARCHAR(1024) NOT NULL DEFAULT '',
  database_name VARCHAR(64) NOT NULL DEFAULT '',
  status ENUM('queued','running','success','failed','cancelled','rolled_back') NOT NULL DEFAULT 'queued',
  progress TINYINT UNSIGNED NOT NULL DEFAULT 0,
  message VARCHAR(2000) NOT NULL DEFAULT '',
  result VARCHAR(2000) NOT NULL DEFAULT '',
  recovery_file VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  started_at TIMESTAMP NULL DEFAULT NULL,
  finished_at TIMESTAMP NULL DEFAULT NULL,
  KEY ix_backup_restore_domain (domain_id, id),
  KEY ix_backup_restore_active (status, created_at),
  CONSTRAINT fk_backup_restore_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE,
  CONSTRAINT fk_backup_restore_backup FOREIGN KEY (backup_id) REFERENCES backups(id) ON DELETE CASCADE
) ENGINE=InnoDB;
