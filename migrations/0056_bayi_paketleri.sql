-- 0056 — bayi (reseller) paket kataloğu.
--
-- Admin isimli/limitli hazır bayi paketleri (Başlangıç/Profesyonel gibi) tanımlar;
-- bayi oluştururken/limitlerini düzenlerken bu katalogdan seçim yapılabilir ve
-- seçilen paketin limitleri reseller_limits'e ANLIK GÖRÜNTÜ olarak kopyalanır —
-- paket sonradan değişse mevcut bayinin limitleri kendiliğinden değişmez.
-- Paket seçimi ZORUNLU DEĞİLDİR: mevcut "elle limit gir / hiç limit tanımlama
-- (0 = sınırsız)" akışı olduğu gibi korunur, paket yalnız hızlı doldurma aracıdır.
CREATE TABLE IF NOT EXISTS reseller_plans (
  id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  ad             VARCHAR(100) NOT NULL,
  aciklama       VARCHAR(255) NOT NULL DEFAULT '',
  max_customer   INT    NOT NULL DEFAULT 0,
  max_domain     INT    NOT NULL DEFAULT 0,
  disk_kota_mb   BIGINT NOT NULL DEFAULT 0,
  trafik_kota_mb BIGINT NOT NULL DEFAULT 0,
  -- Yalnız bilgi/referans amaçlı (kuruş cinsinden); panelin kendi bir fatura/ödeme
  -- akışı yok, admin kendi iş süreci için fiyatı burada tutabilir.
  fiyat_kurus    BIGINT NOT NULL DEFAULT 0,
  varsayilan     TINYINT(1) NOT NULL DEFAULT 0,
  created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY ux_reseller_plans_ad (ad)
) ENGINE=InnoDB;

-- Bayinin hangi pakete bağlı olduğu (yalnız köken/gösterim bilgisi — limitler
-- yukarıdaki gibi anlık görüntüdür, bu kolon canlı bir join için kullanılmaz).
-- Paket silinirse NULL'a düşer, bayi ETKİLENMEZ (mevcut limitleri elinde kalır).
ALTER TABLE reseller_limits ADD COLUMN IF NOT EXISTS reseller_plan_id BIGINT UNSIGNED NULL;
ALTER TABLE reseller_limits
  ADD CONSTRAINT fk_reseller_limits_plan FOREIGN KEY (reseller_plan_id)
    REFERENCES reseller_plans(id) ON DELETE SET NULL;
