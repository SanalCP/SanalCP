-- 0063 — Ortak (paylaşımlı) nameserver modeli.
--
-- ÖNCEKİ DURUM (yanlış model): DNS şablonu her müşteri domaini için
--
--   ns1  IN A  {IP}
--   @    IN NS ns1.{DOMAIN}.
--   @    IN NS ns2.{DOMAIN}.
--
-- üretiyordu. Bu "vanity nameserver" modelidir ve çalışması için HER domainde
-- kayıt şirketinde ayrı glue record (child host) tanımlanması gerekir —
-- paylaşımlı hostingde uygulanabilir değil. Sonuç: müşteriye nameserver
-- verilemiyor, herkes elle A kaydı giriyordu.
--
-- YENİ MODEL: tüm domainler panelin ORTAK nameserver'larını gösterir
-- (ns1.saglayici.com / ns2.saglayici.com). Glue yalnız bir kez, sağlayıcının
-- kendi alan adı için gerekir.
--
-- Bayiler (white-label) kendi nameserver adlarını tanımlayabilir; o bayinin
-- müşterilerinin domainleri bayinin NS'lerini kullanır.

-- Panel geneli varsayılan nameserver çifti.
ALTER TABLE panel_ayarlari
  ADD COLUMN IF NOT EXISTS ns1_hostname VARCHAR(255) NULL,
  ADD COLUMN IF NOT EXISTS ns2_hostname VARCHAR(255) NULL;

-- Bayi başına nameserver (white-label). Kayıt yoksa panel geneli kullanılır.
CREATE TABLE IF NOT EXISTS bayi_nameserver (
  user_id     BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  ns1         VARCHAR(255) NOT NULL,
  ns2         VARCHAR(255) NOT NULL,
  guncellenme TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_bayi_ns_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Şablonu yeni modele taşı.
--
-- WHERE koşulları BİLEREK dar: yalnız dokunulmamış YERLEŞİK varsayılan satırlar
-- güncellenir. Admin şablonu özelleştirdiyse (farklı değer yazdıysa) satırı
-- olduğu gibi kalır — kimsenin elle kurduğu düzen bozulmaz.
UPDATE dns_template SET deger = '{NS1}' WHERE tip = 'NS' AND deger = 'ns1.{DOMAIN}';
UPDATE dns_template SET deger = '{NS2}' WHERE tip = 'NS' AND deger = 'ns2.{DOMAIN}';

-- Müşteri domaininin altındaki ns1/ns2 A kayıtları yeni modelde anlamsız:
-- nameserver artık sağlayıcının/bayinin alan adında yaşıyor.
DELETE FROM dns_template WHERE tip = 'A' AND ad IN ('ns1', 'ns2') AND deger = '{IP}';
