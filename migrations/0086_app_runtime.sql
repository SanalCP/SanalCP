-- Reverse proxy siteleri için panel tarafından yönetilen tek uygulama süreci.
CREATE TABLE IF NOT EXISTS app_runtimes (
  domain_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  runtime ENUM('node','python') NOT NULL,
  entrypoint VARCHAR(255) NOT NULL,
  port INT UNSIGNED NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uq_app_runtimes_port (port),
  CONSTRAINT fk_app_runtime_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
