-- 0067 — Otomatik saldırı engelleme (fail2ban muadili).
--
-- Panel, sistem servislerinin (sshd / dovecot / postfix / pure-ftpd) kimlik
-- doğrulama hatalarını journald üzerinden izler; bir IP belirlenen pencere
-- içinde eşiği aşarsa nftables tarafında SÜRELİ olarak engellenir.
--
-- otoban_aktif = 0 (VARSAYILAN): kapalı. Mevcut kurulumlarda davranış birebir
-- korunur; admin bilinçli olarak açar (Araçlar ve Ayarlar > Güvenlik Duvarı).
--
-- Otomatik banlar ELLE eklenen kurallarla AYNI tabloda tutulur. Böylece:
--   - panel güvenlik duvarı ekranında oldukları gibi görünür/silinebilirler,
--   - mevcut whitelist kuralları onları da kapsar (ruleset'te whitelist önce
--     gelir; ayrıca izleyici whitelist'teki IP'yi hiç banlamaz),
--   - tek bir nft ruleset üretimi vardır, ikinci bir kural kaynağı yoktur.
ALTER TABLE firewall_kurallari
  -- 'elle' | 'otomatik' — otomatik olanlar izleyici tarafından eklenir ve
  -- süresi dolunca yine izleyici tarafından silinir.
  ADD COLUMN IF NOT EXISTS kaynak VARCHAR(10) NOT NULL DEFAULT 'elle',
  -- Banı tetikleyen servis ('ssh' | 'mail' | 'ftp'); elle kurallarda boştur.
  ADD COLUMN IF NOT EXISTS servis VARCHAR(20) NOT NULL DEFAULT '',
  -- NULL = süresiz (elle eklenen tüm kurallar böyledir). Dolu ise bu ana
  -- kadar geçerlidir; nft ruleset üretimi süresi geçmişleri zaten dışlar.
  ADD COLUMN IF NOT EXISTS bitis_at DATETIME NULL DEFAULT NULL;

-- Süresi dolmuş otomatik banların temizlik sorgusu bu indeksi kullanır.
CREATE INDEX IF NOT EXISTS ix_fw_bitis ON firewall_kurallari (bitis_at);

ALTER TABLE panel_ayarlari
  -- 0 = kapalı (varsayılan), 1 = açık.
  ADD COLUMN IF NOT EXISTS otoban_aktif TINYINT(1) NOT NULL DEFAULT 0,
  -- Kaç başarısız denemeden sonra banlanır.
  ADD COLUMN IF NOT EXISTS otoban_esik SMALLINT UNSIGNED NOT NULL DEFAULT 5,
  -- Denemelerin sayıldığı kayan pencere (dakika).
  ADD COLUMN IF NOT EXISTS otoban_pencere_dk SMALLINT UNSIGNED NOT NULL DEFAULT 10,
  -- Ban süresi (dakika). 0 kabul edilmez; uç 1-43200 (30 gün) arasını doğrular.
  ADD COLUMN IF NOT EXISTS otoban_sure_dk SMALLINT UNSIGNED NOT NULL DEFAULT 60;
