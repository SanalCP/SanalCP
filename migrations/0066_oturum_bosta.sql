-- 0066 — Oturum boşta zaman aşımı (idle session timeout).
--
-- oturum_bosta_dakika = 0 (VARSAYILAN): kapalı, mevcut kurulumlarda davranış
-- birebir korunur. Admin bilinçli olarak açar (Panel Ayarları). >0 ise, bir
-- hesap bu kadar dakika hiç istek atmazsa bir sonraki istekte 401 alır ve
-- yeniden giriş yapması gerekir — token süresi dolmamış olsa bile.
--
-- last_activity_at RequireAuth tarafından THROTTLE'LI güncellenir (30sn'de
-- bir) — her istekte yazım baskısı oluşturmasın diye. NULL = hiç istek
-- atmamış (yeni hesap ya da bu sütun eklenmeden önceki son giriş); bu durumda
-- zaman aşımı UYGULANMAZ (ilk istekte last_activity_at hemen set edilir).
ALTER TABLE panel_ayarlari
  ADD COLUMN IF NOT EXISTS oturum_bosta_dakika SMALLINT UNSIGNED NOT NULL DEFAULT 0;

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS last_activity_at DATETIME NULL DEFAULT NULL;
