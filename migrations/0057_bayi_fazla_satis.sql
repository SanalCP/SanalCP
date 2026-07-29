-- 0057 — bayi "fazla satış" (overselling) ilkesi.
--
-- WHM'in "Overselling Allowed" karşılığı: varsayılan olarak AÇIK (1) — mevcut
-- davranış hiç değişmez, bayinin müşterilerine atadığı hizmet planı kotalarının
-- TOPLAMI (taahhüt) hiç kontrol edilmez, yalnız gerçek kullanım (bkz.
-- internal/kota CheckBayiDiskKotasi/TrafikKotasi) sınırlanır. Admin bir bayi
-- için bunu KAPATIRSA (0), o bayi artık kendi disk_kota_mb/trafik_kota_mb
-- limitinden FAZLA taahhüt veremez — yeni domain'e plan atarken taahhüt toplamı
-- da ayrıca kontrol edilir (bkz. internal/kota CheckBayiTaahhutKotasi).
ALTER TABLE reseller_plans  ADD COLUMN IF NOT EXISTS fazla_satis TINYINT(1) NOT NULL DEFAULT 1;
ALTER TABLE reseller_limits ADD COLUMN IF NOT EXISTS fazla_satis TINYINT(1) NOT NULL DEFAULT 1;
