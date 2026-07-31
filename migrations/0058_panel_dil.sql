-- Panelin sunucu-varsayılan dili (kullanıcı henüz giriş yapmadan önce, örn. login
-- ekranı, hangi dilde açılacağını belirler). Kurulum sırasında sorulup buraya yazılır;
-- admin daha sonra da değiştirebilir. Giriş yapmış kullanıcının kendi tercih_dil'i
-- (users.tercih_dil) bunu her zaman geçersiz kılar.
ALTER TABLE panel_ayarlari ADD COLUMN IF NOT EXISTS varsayilan_dil VARCHAR(8) NOT NULL DEFAULT 'tr';
