-- 0050 — her yedeğin uzak kopya durumunu izler.
ALTER TABLE backups ADD COLUMN IF NOT EXISTS uzak_durum  varchar(32)  NOT NULL DEFAULT '';
ALTER TABLE backups ADD COLUMN IF NOT EXISTS uzak_anahtar varchar(512) NOT NULL DEFAULT '';
ALTER TABLE backups ADD COLUMN IF NOT EXISTS uzak_hata   varchar(512) NOT NULL DEFAULT '';
