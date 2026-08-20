-- 0069 — Panelin root/shadow giriş yolu bayrağı.
--
-- Panel girişi ilk sürümden beri sunucunun root parolasıdır: internal/auth/
-- handlers.go içindeki rootShadowHash() doğrudan /etc/shadow'u okur. Bu bayrak
-- o yolu kapatılabilir hale getirir; birincil giriş users tablosundaki gerçek
-- admin hesabına taşınır.
--
-- VARSAYILAN 1 (AÇIK) — bilinçli. Migration mevcut kurulumlarda çalıştığında
-- hiçbir davranış değişmez; root ile giren operatör kilitlenmez. Bayrağı 0'a
-- çeken tek yer sanalcp-install.sh'tir (yeni kurulumlar). Mevcut kurulumlar
-- admin hesabını oluşturduktan sonra Panel Ayarları'ndan kendileri kapatır.
--
-- SSH root erişimi bu bayraktan ETKİLENMEZ; yalnız :8443 panel girişi konudur.
ALTER TABLE panel_ayarlari
  ADD COLUMN IF NOT EXISTS root_girisi_acik TINYINT(1) NOT NULL DEFAULT 1;
