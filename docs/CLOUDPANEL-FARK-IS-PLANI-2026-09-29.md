# CloudPanel farkları: iş planı

Tarih: 2026-09-29. Kaynak: CloudPanel v2 değişiklik günlüğü ve SanalCP kod incelemesi.
Öncelik, SanalCP'nin PHP/e-ticaret odağı ve üretim güvenliği dikkate alınarak belirlenmiştir.

| Sıra | İş | Karar / başlatma koşulu | Çıkış ölçütü |
|---:|---|---|---|
| 1 | Let's Encrypt staging ön denemesi | Şimdi | Gerçek ACME isteği öncesinde aynı hostlar staging ile doğrulanır. Staging durumu gerçek sertifika deposundan ayrılır. Başarısızlık üretim CA isteğini durdurur, mevcut sertifika korunur ve kullanıcıya sebep gösterilir. Geçerli sertifika yeniden kullanılıyorsa iki CA'ya da istek gönderilmez. |
| 2 | Yerel Node.js ve Python uygulamaları | Genel amaçlı hosting hedefi kesinleşince | Tenant kimliğinde sürüm, süreç, dağıtım, log, yeniden başlatma, sağlık kontrolü ve kaldırma; kaynak sınırları ve geri alma. Önce Node.js, sonra Python teslim edilir. |
| 3 | MySQL 8.4 ve yeni MariaDB seçeneği | Uyumluluk gerektiren somut uygulama/taşıma talebi gelince | Kurulum ve yükseltme matrisi; yedek/restore, izolasyon, phpMyAdmin ve migration kabul testleri. |
| 4 | Bulut sağlayıcı snapshot yönetimi | Kullanıcıların yoğunlaştığı sağlayıcı belirlenince | Seçilen sağlayıcıda API kimliği, oluşturma, saklama/silme politikası ve felaket kurtarma tatbikatı. Önce tek sağlayıcıyla başlanır. |
| 5 | Rclone ile ek uzak yedek hedefleri | Mevcut S3/SFTP/B2 hedefleri kullanıcı ihtiyacını karşılamazsa | Kimliklerin güvenli saklanması, bağlantı testi, yükleme, indirme, saklama ve geri yükleme testi. |
| 6 | Parola güncelleme bağlantısı | Çok sayıda site kullanıcısı yönetimi talebi gelince | Tek kullanımlık, süreli ve yetki kapsamlı bağlantı; kullanım sonrası oturum iptali ve denetim kaydı. |

Varnish ve ek arayüz dilleri mevcut kapsamda iş kuyruğuna alınmadı. Varnish için
önce nginx cache/Valkey ölçümlerinde giderilemeyen bir performans sorunu;
ek diller için hedef pazar kararı gerekir.

## İlk işin doğrulaması

- Komut argümanları ve geçici depo için otomatik test.
- Staging hatasında üretim isteğinin çalışmadığına dair test.
- Geçerli sertifika yeniden kullanımının aynı kaldığına dair mevcut testler.
- Canlı alan adı/ACME doğrulaması test sunucusunda yapılmalı; yerel birim testi
  internette sertifika üretmez.

## Yerel uygulama desteği: teslimatlar

1. **Süreç temeli:** Reverse proxy hesabında Node.js/Python giriş dosyası,
   tenant kimliğinde systemd servisinin yönetimi, kaynak slice'ı, port sağlığı,
   günlükler, site silmede servis temizliği ve başarısız ayarda geri alma.
   Kodlandı; canlı systemd sunucusunda kabul testi bekliyor.
2. **Bağımlılıklar ve sürümler:** Kurulu Node.js/Python yorumlayıcısı seçimi,
   Python sanal ortamı, tenant kimliğiyle npm/pip bağımlılık kurulumu kodlandı.
   Canlı tenant kabul testi bekliyor.
3. **Dağıtım:** Mevcut Git Pull/webhook akışına ayrı Git worktree üzerinde
   Node.js derleme, Python gereksinim kurulumu, HTTP sağlık kapısı ve
   başarısızlıkta eski servis sürümüne dönüş kodlandı. Canlı tenant kabul
   testi bekliyor. Servis yeniden başladığı için sıfır kesinti garantisi yoktur.
