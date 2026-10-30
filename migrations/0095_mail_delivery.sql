-- Actual Postfix recipient outcomes; mail_send_log remains SMTP policy accounting.
CREATE TABLE IF NOT EXISTS mail_delivery_sources (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  `cursor` TEXT NULL,
  last_event_at DATETIME(6) NULL,
  collected_at DATETIME(6) NULL,
  state VARCHAR(16) NOT NULL DEFAULT 'starting',
  warning VARCHAR(64) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS mail_delivery_messages (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  source_id VARCHAR(64) NOT NULL,
  queue_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  sender VARCHAR(320) NOT NULL DEFAULT '',
  message_id VARCHAR(998) NOT NULL DEFAULT '',
  authenticated_sender VARCHAR(320) NOT NULL DEFAULT '',
  pickup_uid BIGINT UNSIGNED NULL,
  outbound_domain_id BIGINT UNSIGNED NULL,
  first_seen_at DATETIME(6) NOT NULL,
  last_seen_at DATETIME(6) NOT NULL,
  closed_at DATETIME(6) NULL,
  KEY ix_delivery_queue (source_id, queue_id, closed_at),
  KEY ix_delivery_message_seen (last_seen_at),
  CONSTRAINT fk_delivery_message_source FOREIGN KEY (source_id) REFERENCES mail_delivery_sources(id),
  CONSTRAINT fk_delivery_outbound_domain FOREIGN KEY (outbound_domain_id) REFERENCES domains(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS mail_deliveries (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  message_fk BIGINT UNSIGNED NOT NULL,
  recipient VARCHAR(320) NOT NULL,
  original_recipient VARCHAR(320) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL,
  first_seen_at DATETIME(6) NOT NULL,
  last_seen_at DATETIME(6) NOT NULL,
  relay VARCHAR(512) NOT NULL DEFAULT '',
  dsn VARCHAR(32) NOT NULL DEFAULT '',
  response TEXT NOT NULL,
  UNIQUE KEY uq_delivery_recipient (message_fk, recipient),
  KEY ix_delivery_seen (last_seen_at, id),
  KEY ix_delivery_status_seen (status, last_seen_at),
  CONSTRAINT fk_delivery_message FOREIGN KEY (message_fk) REFERENCES mail_delivery_messages(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS mail_delivery_events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  message_fk BIGINT UNSIGNED NOT NULL,
  delivery_id BIGINT UNSIGNED NULL,
  event_key BINARY(32) NOT NULL,
  at DATETIME(6) NOT NULL,
  status VARCHAR(16) NOT NULL,
  relay VARCHAR(512) NOT NULL DEFAULT '',
  dsn VARCHAR(32) NOT NULL DEFAULT '',
  response TEXT NOT NULL,
  delay_seconds DOUBLE NULL,
  UNIQUE KEY uq_delivery_event_key (event_key),
  KEY ix_delivery_event_timeline (delivery_id, at, id),
  KEY ix_delivery_message_timeline (message_fk, at, id),
  CONSTRAINT fk_delivery_event_message FOREIGN KEY (message_fk) REFERENCES mail_delivery_messages(id) ON DELETE CASCADE,
  CONSTRAINT fk_delivery_event_recipient FOREIGN KEY (delivery_id) REFERENCES mail_deliveries(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS mail_delivery_visibility (
  delivery_id BIGINT UNSIGNED NOT NULL,
  domain_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (delivery_id, domain_id),
  KEY ix_delivery_domain (domain_id, delivery_id),
  CONSTRAINT fk_delivery_visibility_recipient FOREIGN KEY (delivery_id) REFERENCES mail_deliveries(id) ON DELETE CASCADE,
  CONSTRAINT fk_delivery_visibility_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
