-- Domain yedeklerinin bütünlük ve geri açılabilirlik durumu.
ALTER TABLE backups ADD COLUMN IF NOT EXISTS dogrulama_durum varchar(16) NOT NULL DEFAULT 'bekliyor';
ALTER TABLE backups ADD COLUMN IF NOT EXISTS dogrulama_hata varchar(1000) NOT NULL DEFAULT '';
ALTER TABLE backups ADD COLUMN IF NOT EXISTS dogrulama_sha256 char(64) NOT NULL DEFAULT '';
ALTER TABLE backups ADD COLUMN IF NOT EXISTS dogrulama_zamani timestamp NULL DEFAULT NULL;
ALTER TABLE backups ADD KEY IF NOT EXISTS ix_backup_dogrulama (dogrulama_durum, dogrulama_zamani);
