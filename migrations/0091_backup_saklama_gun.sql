-- 0091 — Otomatik yedekler için gün bazlı saklama süresi.
--
-- backup_retention ADET sınırıdır ("son 7 yedek"); frekansa göre anlamı
-- değişir (haftalıkta 7 = 7 hafta). Kullanıcının asıl sorusu çoğu zaman
-- "yedekler kaç gün dursun?"dur — bu sütun onu doğrudan karşılar.
--
-- İki sınır birlikte uygulanır, hangisi önce dolarsa o budar.
-- Yalnızca tip='oto' satırlarına uygulanır; manuel yedeklerin kendi sayacı var
-- (bkz. 0065).
--
-- 0 = gün sınırı yok (varsayılan): mevcut davranışı birebir korur.
-- Arayüz 3 / 5 / 7 / 30 seçeneklerini sunar; API 0-365 kabul eder.
ALTER TABLE domains
  ADD COLUMN IF NOT EXISTS backup_saklama_gun smallint(6) NOT NULL DEFAULT 0;
