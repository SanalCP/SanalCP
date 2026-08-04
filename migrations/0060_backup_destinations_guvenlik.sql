-- 0060 — backup_destinations güvenlik sıkılaştırması:
--   * parola artık AES-256-GCM şifreli (v1: önekli) saklanıyor, ciphertext
--     düz metinden uzun olabildiği için sütun genişletildi.
--   * host_key: SFTP hedefleri için TOFU (trust-on-first-use) ile pinlenen
--     SSH host key (ssh-keyscan çıktısı, known_hosts satırları).
ALTER TABLE backup_destinations MODIFY COLUMN parola varchar(512) NOT NULL;
ALTER TABLE backup_destinations ADD COLUMN IF NOT EXISTS host_key varchar(2048) NOT NULL DEFAULT '';
