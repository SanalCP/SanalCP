-- 0048 — çok kullanıcılı panel (WHM/cPanel modeli) için sahiplik ve bayi kotaları.
--
-- Bu migration YALNIZCA şemayı hazırlar; yetkilendirme mantığı henüz bu
-- kolonları okumaz (bkz. Faz 5D). Var olan tek kullanıcılı davranış
-- değişmez: kolonlar NULL varsayılanlıdır ve NULL = "sahipsiz / doğrudan
-- admin'e ait" anlamına gelir.

-- Sahiplik zinciri: domain -> customer -> (bayi kullanıcısı | müşteri hesabı)
--   owner_user_id : bu müşteriyi hangi BAYİ yönetiyor (NULL = doğrudan admin)
--   user_id       : bu müşterinin PANEL GİRİŞ hesabı (NULL = giriş hesabı yok;
--                   müşteri hâlâ eski FTP kimlikli /cp yolunu kullanıyor)
ALTER TABLE customers ADD COLUMN IF NOT EXISTS owner_user_id BIGINT UNSIGNED NULL;
ALTER TABLE customers ADD COLUMN IF NOT EXISTS user_id       BIGINT UNSIGNED NULL;

-- Kapsam sorguları bu iki kolon üzerinden döner; indekssiz her istekte tam
-- tarama olurdu.
ALTER TABLE customers ADD INDEX IF NOT EXISTS ix_customer_owner (owner_user_id);
ALTER TABLE customers ADD INDEX IF NOT EXISTS ix_customer_user  (user_id);

-- Bayi limitleri (WHM'deki "reseller limits" karşılığı). 0 = sınırsız —
-- service_plans'taki mevcut kota deseniyle aynı sözleşme.
CREATE TABLE IF NOT EXISTS reseller_limits (
  user_id        BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  max_customer   INT    NOT NULL DEFAULT 0,
  max_domain     INT    NOT NULL DEFAULT 0,
  disk_kota_mb   BIGINT NOT NULL DEFAULT 0,
  trafik_kota_mb BIGINT NOT NULL DEFAULT 0,
  -- Bayinin müşterisine atayabileceği plan kimlikleri (JSON dizi).
  -- NULL = kısıt yok (tüm planları atayabilir).
  izinli_planlar JSON NULL,
  created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT fk_reseller_limits_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB;
