-- 0061 — github_connections.pat artık AES-256-GCM ile şifreli saklanıyor
-- (internal/secretcrypt; db_accounts.db_pass_plain ve backup_destinations.parola
-- ile aynı desen). Ciphertext düz metinden uzun olduğu için sütun genişletildi.
-- Mevcut düz-metin satırlar panel açılışında idempotent olarak göçürülür
-- (github.HealLegacyPlaintextPATs).
ALTER TABLE github_connections MODIFY COLUMN pat varchar(512) NOT NULL;
