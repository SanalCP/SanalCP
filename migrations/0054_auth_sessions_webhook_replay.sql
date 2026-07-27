-- Oturum iptali: parola/rol/durum/2FA değişince auth_version artırılır;
-- eski JWT içindeki sürüm DB ile eşleşmez ve anında reddedilir.
-- Varsayılan 1 özellikle seçildi: migration öncesi üretilmiş JWT'lerde `ver`
-- claim'i yoktur ve Go bunu 0 okur. Mevcut hesapları 1 ile başlatmak, upgrade
-- anında bütün eski tokenları güvenli biçimde geçersiz kılar.
ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_version BIGINT UNSIGNED NOT NULL DEFAULT 1;

-- GitHub webhook replay koruması. delivery_id GitHub'ın
-- X-GitHub-Delivery UUID değeridir ve global olarak benzersizdir.
CREATE TABLE IF NOT EXISTS git_webhook_deliveries (
  delivery_id VARCHAR(128) NOT NULL PRIMARY KEY,
  git_repo_id BIGINT UNSIGNED NOT NULL,
  event       VARCHAR(64) NOT NULL DEFAULT '',
  received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY ix_git_webhook_received (received_at),
  KEY ix_git_webhook_repo (git_repo_id),
  CONSTRAINT fk_git_webhook_repo
    FOREIGN KEY (git_repo_id) REFERENCES git_repos(id) ON DELETE CASCADE
) ENGINE=InnoDB;
