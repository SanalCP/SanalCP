-- 0079 — Faz 9: açıklanabilir, puanlı PHP zararlı yazılım bulguları.
ALTER TABLE av_bulgular
  ADD COLUMN IF NOT EXISTS puan TINYINT UNSIGNED NOT NULL DEFAULT 0 AFTER karantina,
  ADD COLUMN IF NOT EXISTS risk VARCHAR(16) NOT NULL DEFAULT '' AFTER puan,
  ADD COLUMN IF NOT EXISTS gerekceler JSON NULL AFTER risk,
  ADD COLUMN IF NOT EXISTS sha256 CHAR(64) NOT NULL DEFAULT '' AFTER gerekceler,
  ADD COLUMN IF NOT EXISTS istisna TINYINT(1) NOT NULL DEFAULT 0 AFTER sha256,
  ADD COLUMN IF NOT EXISTS karantina_yolu VARCHAR(512) NOT NULL DEFAULT '' AFTER istisna,
  ADD COLUMN IF NOT EXISTS karantina_zamani TIMESTAMP NULL AFTER karantina_yolu,
  ADD KEY IF NOT EXISTS ix_avb_risk (domain_id, risk, puan);

CREATE TABLE IF NOT EXISTS av_istisnalar (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  domain_id INT NOT NULL,
  dosya VARCHAR(512) NOT NULL,
  imza VARCHAR(255) NOT NULL,
  sha256 CHAR(64) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_av_istisna (domain_id, dosya(255), imza, sha256),
  KEY ix_av_istisna_domain (domain_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
