-- Apex domain (ör. sanalcp.com) ziyaret edilince www alt alan adına (www.sanalcp.com)
-- 301 yönlendirilsin mi? SSL/TLS sayfasından yönetilir. Yalnızca ana domain satırı
-- (ana_domain_id IS NULL) için anlamlıdır — ek/parked alan adları ayrı vhost dosyasında
-- render edildiği için bu ayarı okumaz (bkz. domain_redirects ile aynı kısıtlama).
ALTER TABLE domains ADD COLUMN IF NOT EXISTS www_yonlendir TINYINT(1) NOT NULL DEFAULT 0;
