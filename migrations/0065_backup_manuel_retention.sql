-- 0065 — Manuel yedekler için ayrı saklama (retention) sınırı.
--
-- NEDEN: pruneOld() yalnızca tip='oto' satırlarını buduyordu; elle alınan
-- yedekler (tip='tam') hiç silinmiyordu. Backup Yöneticisi'ndeki "Yedek Sayısı"
-- kolonu ise diskteki TÜM .tar.gz dosyalarını saydığı için banner "7 yedek
-- saklanır" derken tablo 8-9 gösteriyordu — sayı yanlış değildi, etiket eksikti.
--
-- Neden otomatiklerle aynı sınır değil: manuel yedek genelde riskli bir işlem
-- (sürüm yükseltme, eklenti kurulumu) öncesi bilinçli alınır. Otomatiklerle
-- aynı havuzda dönseydi, gece çalışan planlı yedekler kullanıcının kasıtlı
-- aldığı yedeği birkaç gün içinde dışarı iterdi. Ayrı sayaç bunu engeller.
--
-- 0 = SINIRSIZ (varsayılan): mevcut davranışı birebir korur. Göç anında hiç
-- kimsenin manuel yedeği silinmez — sınırı domain sahibi bilinçli koyar.
-- 1-90 = en yeni N manuel yedek tutulur, gerisi budanır.
ALTER TABLE domains
  ADD COLUMN IF NOT EXISTS backup_manuel_retention tinyint(4) NOT NULL DEFAULT 0;
