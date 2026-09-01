-- Native SanalCP -> SanalCP uzaktan aktarım sağlayıcısı.
ALTER TABLE remote_transfer_jobs
  MODIFY COLUMN provider ENUM('sanalcp','cpanel','plesk','directadmin') NOT NULL;
