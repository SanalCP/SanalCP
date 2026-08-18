-- 0068 — Yönetim API'si için kişisel erişim token'ları.
--
-- TASARIM: Token AYRI BİR İZİN SİSTEMİ DEĞİLDİR. Her token bir panel hesabına
-- (users.id) bağlıdır ve istek anında o hesabın rolüyle çalışır. Böylece mevcut
-- yetki katmanının tamamı (AdminOnly / BayiVeUstu / MusteriScope / KapsamSQL)
-- API istekleri için de aynen geçerlidir; ikinci bir yetki matrisi yoktur.
--
-- Sonuç: bayinin token'ı bayinin görebildiğinden fazlasını göremez, hesabın
-- rolü değişince token'ın yetkisi de anında değişir, hesap askıya alınınca
-- token da çalışmaz.
--
-- Ham token SAKLANMAZ; yalnız SHA-256 özeti tutulur (cli_tokens ile aynı desen).
-- Oluşturma yanıtında bir kez gösterilir, sonra bir daha elde edilemez.
CREATE TABLE IF NOT EXISTS api_tokenlari (
  id INT AUTO_INCREMENT PRIMARY KEY,
  -- Yöneticinin tanıyabilmesi için etiket ("yedekleme scripti", "izleme" vb.).
  ad VARCHAR(64) NOT NULL,
  -- SHA-256(ham token), hex. UNIQUE: aynı token iki kez kayıtlı olamaz.
  token_hash CHAR(64) NOT NULL,
  -- Listede gösterilecek ön ek ("scp_1a2b3c…") — ham token değildir, yalnız
  -- yöneticinin hangi token olduğunu ayırt etmesi içindir.
  onek VARCHAR(24) NOT NULL DEFAULT '',
  -- Sahibi. Hesap silinince token'ları da silinir (yetim token kalmaz).
  user_id BIGINT NOT NULL,
  -- NULL = süresiz. Dolu ise bu andan sonra token kabul edilmez.
  bitis_at DATETIME NULL DEFAULT NULL,
  -- 0 = iptal edilmiş (silmeden devre dışı bırakma).
  aktif TINYINT(1) NOT NULL DEFAULT 1,
  son_kullanim_at DATETIME NULL DEFAULT NULL,
  son_kullanim_ip VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_api_token_hash (token_hash),
  KEY ix_api_token_user (user_id),
  CONSTRAINT fk_api_token_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
