-- Panel API erişimini IPv4/IPv6 adresleri ve CIDR ağlarıyla sınırlandırır.
-- Boş değer = kısıtlama kapalı; mevcut kurulumların davranışı değişmez.
ALTER TABLE panel_ayarlari
  ADD COLUMN IF NOT EXISTS erisim_cidrleri TEXT NULL,
  ADD COLUMN IF NOT EXISTS gecici_erisim_cidr VARCHAR(64) NULL,
  ADD COLUMN IF NOT EXISTS gecici_erisim_bitis DATETIME NULL;
