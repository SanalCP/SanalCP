-- 0090 - Webhook HMAC anahtarı URL'deki yönlendirme anahtarından ayrıldı.
-- webhook_secret URL'de taşınır (erişim günlüklerine düşebilir); imza artık
-- yalnız GitHub'ın "Secret" alanına girilen bu ayrı değerle doğrulanır.
-- Boş = eski kayıt: geriye uyum için URL anahtarıyla doğrulanır.
ALTER TABLE git_repos ADD COLUMN IF NOT EXISTS webhook_imza VARCHAR(64) NOT NULL DEFAULT '' AFTER webhook_secret;
