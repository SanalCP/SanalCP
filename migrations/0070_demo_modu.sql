-- 0070 — Demo panel modu.
--
-- Açıkken tüm yazan istekler (auth/login, auth/cikis hariç) middleware
-- katmanında 403 ile reddedilir (bkz. internal/middleware/demo.go). Sır
-- döndüren birkaç GET ucu (DB parolası, mail parolası, dosya içeriği) değeri
-- maskeler. Sadece demo.sanalcp.com gibi ayrı, bağımsız bir VPS'te elle
-- açılır (bkz. scripts/demo_seed.go) — normal kurulumlarda hiç dokunulmaz.
--
-- VARSAYILAN 0 — mevcut kurulumlarda davranış birebir korunur.
ALTER TABLE panel_ayarlari
  ADD COLUMN IF NOT EXISTS demo_modu_acik TINYINT(1) NOT NULL DEFAULT 0;
