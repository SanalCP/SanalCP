-- Uyarı bildirimleri: kanal ayarları (panel geneli) + kontrol başına son durum.
-- Bildirim yalnız durum değişince gider; son durum bu yüzden kalıcı tutulur.
ALTER TABLE panel_ayarlari
  ADD COLUMN IF NOT EXISTS bildirim_acik TINYINT(1) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS telegram_token_enc TEXT NULL,
  ADD COLUMN IF NOT EXISTS telegram_chat_id VARCHAR(64) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS bildirim_eposta VARCHAR(320) NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS uyari_durumlari (
  domain_id BIGINT UNSIGNED NOT NULL,
  kontrol VARCHAR(16) NOT NULL,
  durum VARCHAR(8) NOT NULL,
  ardisik_hata INT NOT NULL DEFAULT 0,
  degisti_at DATETIME NOT NULL,
  PRIMARY KEY (domain_id, kontrol),
  CONSTRAINT fk_uyari_durum_domain FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
