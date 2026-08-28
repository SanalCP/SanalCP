# SanalCP Ürün Yol Haritası — CloudPanel ve gPanel Karşılaştırması

**Tarih:** 2026-08-28  
**Amaç:** SanalCP'yi üretim e-ticaret siteleri için CloudPanel seviyesine yaklaştıracak işleri, operasyonel risk ve kullanıcı değerine göre sıralamak.

## Tahmin varsayımları

- Süreler tek geliştiricinin odaklı çalışması içindir.
- Backend, arayüz, yetki kontrolleri, temel otomasyon testleri, paketleme ve test sunucusuna dağıtım dahildir.
- Gerçek müşteri verisiyle bekleme/gözlem süresi geliştirme süresine dahil değildir.
- Her faz ayrı sürüm olarak yayınlanabilir; büyük fazlar küçük teslimatlara bölünmelidir.

## Öncelik sırası

| Sıra | İş | Kaynak | Tahmin | Çıkış ölçütü |
|---:|---|---|---:|---|
| 0 | Kurtarma ve regresyon güvenlik ağı | Operasyon ihtiyacı | 2–3 gün | Güncelleme, geri alma ve yedekten dönüş otomatik kabul testinden geçer; planlı geri yükleme tatbikatı raporlanır. |
| 1 | Cloudflare entegrasyonu | CloudPanel farkı | 3–5 gün | API token ile zone doğrulama; DNS kayıtlarını yönetme; proxy durumu; cache temizleme; güvenli token saklama. |
| 2 | Gerçek staging ortamı | CloudPanel farkı | 5–8 gün | Site+DB klonlama, staging alan adı/SSL, arama motoru engeli, tek yönlü seçmeli canlıya aktarım ve işlem öncesi yedek. |
| 3 | PrestaShop operasyon araç seti | CloudPanel/e-ticaret ihtiyacı | 4–7 gün | Mevcut tek-tık kurulumun yanına bakım modu, cache temizleme, sürüm/modül sağlık özeti, log görüntüleme ve güvenli yedekleme noktası eklenir. |
| 4 | Doğrulanmış yedek ve felaket kurtarma | CloudPanel farkı | 4–6 gün | Yedekler bütünlük + geri yüklenebilirlik testinden geçer; son doğrulama durumu panelde görünür; periyodik restore drill bulunur. |
| 5 | Panel erişimini IP/CIDR ile sınırlandırma | CloudPanel güvenlik farkı | 2–3 gün | Allowlist, geçici erişim, kilitlenmeyi önleyen mevcut-IP kontrolü ve CLI kurtarma yolu bulunur. |
| 6 | Bot ve rate-limit yönetimi | CloudPanel güvenlik farkı | 3–5 gün | Panel ve site bazlı hazır profiller, özel limitler, istisnalar, gözlem logu ve güvenli nginx doğrulama/rollback bulunur. |
| 7 | Canlı sunucudan panel taşıma | gPanel farkı | 7–12 gün | cPanel/Plesk/DirectAdmin kaynağı SSH ile keşfedilir; seçili site dosya+DB+DNS+SSL taşınır; işler arka planda izlenir ve site bazlı geri alınır. |
| 8 | Laravel araç seti | gPanel farkı | 5–8 gün | Laravel otomatik keşif, güvenli deploy, `.env` yönetimi, artisan/composer, scheduler, queue worker, bakım modu ve deploy geçmişi bulunur. |
| 9 | Gelişmiş zararlı yazılım ve bütünlük motoru | gPanel farkı | 6–10 gün | Puanlı PHP webshell kuralları, konum sezgileri, WordPress çekirdek checksum kontrolü, karantina ve yanlış-pozitif güvenlik sınırları bulunur. |
| 10 | Güvenlik olay korelasyonu ve bildirim merkezi | gPanel farkı | 5–8 gün | Başarılı/başarısız giriş, WAF, dosya bulgusu ve süreç olayları zaman penceresinde ilişkilendirilir; panel içi bildirim ve çözüm durumu sunulur. |
| 11 | HTTP/3 ve uygulama cache profilleri | CloudPanel farkı | 3–5 gün | Ortam desteği algılanır; güvenli aç/kapat; WordPress/PrestaShop/genel profiller; ölçülebilir önce/sonra kontrolü bulunur. |

Toplam geliştirme tahmini: **49–80 iş günü**. İlk üretim eşiği olan 0–5 arası yaklaşık **20–32 iş günü** sürer. İşler paralel sürümlemeye uygun olsa da staging ve kurtarma doğrulaması tamamlanmadan kritik mağaza taşıması önerilmez.

## gPanel incelemesinden plana alınmayanlar

- **Güncelleme öncesi DB yedeği, health check ve otomatik rollback:** SanalCP'nin `assets/ops/sanalcp-update` aracı zaten bunları içeriyor.
- **Sunucu kaynaklarına göre optimizasyon:** SanalCP'de `sanalcp-optimize.sh` ve panel kartı mevcut.
- **Temel antivirüs/karantina:** SanalCP'de domain bazlı ClamAV + heuristik tarama var; yalnız gPanel'in puanlı motoru ve WordPress bütünlük katmanı yeni iş olarak alındı.
- **cPanel yedek arşivinden içe aktarma:** SanalCP'de mevcut. Plana alınan fark, uzak sunucuyu SSH ile keşfedip canlı taşıma ve Plesk/DirectAdmin adaptörleridir.
- **Boşta oturum süresi:** SanalCP'de mevcut.
- **Panel alan adı + Let's Encrypt yönetimi:** SanalCP'de mevcut.
- **Genel native uygulama kataloğu (Gitea/Grafana/Syncthing vb.):** Hosting panelinin saldırı yüzeyini ve bakım yükünü büyüttüğü için şimdilik kapsam dışı. Talep oluşursa ayrı ürün modülü olarak ele alınmalı.
- **Panel portunu arayüzden değiştirme ve geniş çoklu-IP yönetimi:** Hatalı kullanımda erişim kaybı riski yüksek, kullanıcı değeri ilk 12 işten düşük; CLI kurtarma ve güçlü testler olmadan eklenmemeli.

## Uygulama sırası ve sürüm kapıları

1. **Güven temeli:** 0 → 4. Her otomatik yedek için “dosya oluştu” değil, “geri açılabiliyor” kanıtı üret.
2. **Erişim güvenliği:** 5 → 6. Önce panelin kendisini sınırla, sonra müşteri sitelerine profil sun.
3. **Taşıma ve geliştirici deneyimi:** 7 → 8. Taşıma altyapısındaki iş kuyruğu ve geri alma mekanizmasını Laravel deploy işlerinde yeniden kullan.
4. **Derin güvenlik:** 9 → 10. Önce güvenilir olay üret, sonra olayları korele edip bildirimleştir.
5. **Protokol ve performans:** 11. HTTP/3 ve uygulama cache profillerini ortam desteği algılaması ve geri alma güvencesiyle sun.

## Kritik mağaza için kabul kapısı

`alsancakuniforma.com` gibi üretim PrestaShop mağazasını SanalCP'ye taşımadan önce en az şu koşullar sağlanmalı:

- Faz 0, 2, 3 ve 4 tamamlanmış olmalı.
- Kopya mağaza üzerinde ödeme, sipariş, e-posta, cron, cache ve admin senaryoları test edilmeli.
- En az 2–4 hafta staging/canary gözlemi yapılmalı.
- Belgelenmiş geri dönüş planı ve CloudPanel tarafında korunmuş son yedek bulunmalı.

## Gerçekleşme durumu

- **Faz 0 — tamamlandı (v0.9.26):** İzole sağlıksız-release kabul testi,
  binary+frontend+migration+DB tam rollback, restore asset SHA-256 kapısı,
  yapısal DB dump doğrulaması, salt-okunur kurtarma kontrolü ve aylık geçici-DB
  restore tatbikatı eklendi. 2026-08-28 canlı kabulünde son yedek 47 tabloyla
  geçici DB'ye açıldı; tatbikat sonunda geçici şema kalmadığı doğrulandı.
- **Faz 1 — tamamlandı (v0.9.28):** Şifreli ve doğrulanan Cloudflare API
  token'ı, domain-zone eşleştirmesi, DNS kayıt CRUD'u, proxy aç/kapat ve tüm
  zone cache'ini temizleme işlevleri eklendi.
- **Faz 2 — tamamlandı (v0.9.29):** Ayrı Linux kullanıcısı, PHP-FPM/vhost ve
  MariaDB hesabıyla gerçek staging; WordPress/Laravel/PrestaShop yapılandırma
  uyarlaması, arama motoru engeli, seçmeli dosya/DB canlıya gönderimi ve her
  gönderim öncesi zorunlu tam kurtarma noktası eklendi.
- **Faz 3 — tamamlandı (v0.9.31):** PrestaShop otomatik keşif ve sağlık özeti,
  düşük yetkili mağaza DB hesabıyla bakım modu yönetimi, güvenli cache temizliği,
  symlink-jail korumalı log görüntüleme ve bakım değişiklikleri öncesinde zorunlu
  tam kurtarma noktası eklendi.
- **Faz 4 — tamamlandı (v0.9.32):** Domain yedekleri SHA-256, güvenli çıkarma,
  manifest/dosya ağacı kontrolü ve izole geçici MariaDB şemasına gerçek restore
  tatbikatıyla doğrulanır. Durum, zaman ve hata panelde görünür; en yeni yedekler
  periyodik yeniden sınanır ve doğrulanamayan/değişmiş yedeğin geri yüklenmesi
  fail-closed engellenir.
- **Taşıma altyapısı ara teslimatı (v0.9.30):** Mevcut genel Web Sitesi İçe
  Aktarım aracı kalıcı arka plan işleri, canlı ilerleme/geçmiş, işlem öncesi
  zorunlu kurtarma noktası, hata halinde dosya/DB otomatik rollback ve aktarım
  sonrası dosya+DB+HTTP sağlık kontrolüyle güçlendirildi.
