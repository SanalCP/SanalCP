# Değişiklik Günlüğü

Bu dosya SanalCP'nin sürüm geçmişini özetler. Ayrıntı için `git log`'a bakabilirsiniz.

Sürümleme `BÜYÜK.KÜÇÜK.YAMA` biçimindedir. Panel **1.0 öncesi (beta)** olduğu için
`0.x` serisinde küçük sürümler arasında davranış değişikliği olabilir; her sürüm
`sanalcp-update` ile güvenli (yedekli + geri dönüşlü) şekilde uygulanır.

Panelin kurulu sürümünü `Araçlar ve Ayarlar → Panel Güncellemesi` ekranından veya
sayfa altbilgisinden görebilirsiniz.

---

## 0.9.x — Lisans numarası

**0.9.48** (2026-09-03)

Özel PHP uygulamalarının kök `config.php` dosyasında kullandığı `DB_HOST`,
`DB_NAME`, `DB_USER` ve `DB_PASS` sabitleri canlı hesap aktarımı sırasında
hedefte oluşturulan veritabanı kimliklerine göre güncellenir. Keşif bu uygulama
biçimini tanır ve PDO/MySQL modülü gereksinimlerini hedefle karşılaştırır.

**0.9.47** (2026-09-03)

Canlı hesap aktarımında domain keşfi artık kaynak PHP sürümünü, uygulama türünü
ve uygulamanın kullandığı PHP modüllerini hedef sunucuyla karşılaştırır. Eksik
PHP sürümü veya modülü bulunan hesaplar nedenleriyle birlikte uyumsuz gösterilir
ve toplu seçime dahil edilmez.

WordPress'e ek olarak Laravel, Symfony ve PrestaShop veritabanı yapılandırmaları
hedefte oluşturulan yeni veritabanı adı ve kimlik bilgilerine göre güncellenir.
Taşınmış framework cache'leri güvenle temizlenir. Sağlık kontrolü HTTP 500 ile
sonuçlanırsa rollback öncesindeki güncel PHP-FPM/nginx hata özeti aktarım işine
kaydedilir.

**0.9.46** (2026-09-02)

Üst çubuktaki İşlemler ve Bildirimler menüsüne `Temizle` işlemi eklendi.
Tamamlanmış ve başarısız işler artık kaynak işlem geçmişi silinmeden listeden
kalıcı olarak gizlenebilir; temizlenen başarısız işler bildirim rozetinden de
düşer. Bekleyen ve çalışan işler yanlışlıkla temizlenmez, yeni işler normal
şekilde görünmeye devam eder.

**0.9.45** (2026-09-02)

SanalCP sunucuları arasında native domain aktarımı eklendi. Hedef panelden
kaynak sunucuya doğrulanmış SSH anahtarıyla bağlanılarak ana domainler
keşfedilebilir ve seçilen hesaplar toplu aktarılabilir. Web dosyaları,
veritabanları, DNS kayıtları, SSL sertifikaları, posta kutuları ve Maildir
mesajları, alias/otomatik yanıt/filtre/spam ayarları, cron görevleri, alt ve ek
alan adları ile taşınabilir güvenlik/nginx ayarları korunur.

Aktarım işleri sınırlı eşzamanlılıkla ayrı ayrı izlenir; tek domainin hatası
diğer işleri etkilemez. Kaynağa göre bozulan hedef otomatik geri alınır.
Yönlendirmeli sitelerde sağlık kontrolü artık `www` hedefini yerel sunucuda
takip eder. Kaynak zaten sağlıksızsa bu durum yanıltıcı başarı metni yerine
açıkça bildirilir.

Debian/Ubuntu hedeflerde PHP-FPM soket yolu işletim sistemi eşlemesinden
çözülür ve domain hazır olmadan önce tenant FPM gerçekten doğrulanır. Native
aktarımı kullanan WordPress sitelerinde varsayılan/çoklu veritabanı eşlemesi
korunur; `wp-config.php` hedefte üretilen güvenli DB kimlik bilgilerine göre
uyarlanır. Web kökü dışındaki uygulama yapılandırma dosyaları taşınırken SSH
anahtarları, kabuk geçmişi ve sistem profilleri özellikle dışarıda bırakılır.

Canlı kabul testlerinde küçük/toplu domainler, 560 MiB ve 9.779 dosyalı çoklu
veritabanı kullanan WordPress sitesi, SSL/DNS ve tam posta senaryosu (IMAPS,
SMTP AUTH, alias, Sieve klasörü ve ekli mesaj) kaynak/hedef karşılaştırmasıyla
doğrulandı. Frontend `browserslist` güvenlik bildirimleri de giderildi.

**0.9.44** (2026-09-01)

Profil ve Tercihler ekranındaki parola açıklaması oturum açan hesaba göre
düzeltildi. Panel yöneticisi (`admin`) artık açıkça yalnız kendi panel hesabının
parolasını değiştirdiğini görür; root/SSH uyarısı yalnız panelde root hesabıyla
oturum açılmışsa gösterilir. Başarılı parola değişikliğinde tüm mevcut oturumlar
geçersizleştirilir, tarayıcıdaki oturum çerezi silinir ve kullanıcı yeni
parolasıyla yeniden giriş yapmak üzere giriş ekranına gönderilir. Davranış
backend regresyon testiyle korunur.

**0.9.43** (2026-09-01)

`v0.9.42` release assetleri yerel makinedeki eski Go 1.25.0 toolchain'iyle
derlenmişti. Kaynak kod `govulncheck` taraması temizdi; ancak dağıtılan binary
Go 1.25.0 standard library'sini içine gömdüğü için CI binary taraması 47 eski
stdlib kaydı buldu. Binary'ler yamalı Go 1.26.7 ile yeniden üretildi. Release
paketleme betiği artık Go 1.26.7 altındaki toolchain'leri reddediyor ve üretilen
binary'nin gerçekten beklenen toolchain'i taşıdığını doğruluyor.

**0.9.42** (2026-09-01)

Debian ve Ubuntu sunucularda dashboard güvenlik denetimi etkinleştirildi. Panel,
`apt` simülasyonu üzerinden yalnız dağıtımın `security` depolarından gelen bekleyen
paketleri listeliyor; apt taşınabilir bir CVE/önem akışı sunmadığı için bunları
yanıltıcı biçimde CVE veya kritik/önemli diye etiketlemiyor. Dashboard'daki tek
tık güncelleme ve Araçlar'daki günlük otomatik güvenlik güncellemesi artık
Debian/Ubuntu'da da çalışıyor; reboot gereksinimi `/var/run/reboot-required`
üzerinden algılanıyor. Yeni kurulumlar `unattended-upgrades` paketini getiriyor,
mevcut kurulumlarda paket yoksa panel güvenli `apt-get --only-upgrade` yoluna düşüyor.

**0.9.41** (2026-09-01)

Kurulum artık sunucuyu kendisi yeniden başlatıyor. Yeni kurulumda ilk yeniden
başlatma zaten zorunluydu — disk kotası GRUB'a yazılan `rootflags=usrquota`
(XFS'te `uquota`) ile geliyor ve o bayrak çekirdeğe ancak açılışta ulaşıyor, yani
reboot edilmeden panel plan limitlerini gösterir ama sistem hiçbirini uygulamaz —
fakat bu yeniden başlatma yalnızca bir uyarı satırıyla operatöre bırakılıyordu.
Kurulumun sonunda artık "yeniden başlatmak için bir tuşa basın" istemi çıkıyor ve
tuşa basılınca sunucu yeniden başlatılıyor. İstem, panel yönetici parolasının
basıldığı kutudan sonra geldiği için parola ekranda kaybolmuyor. Terminali
olmayan (otomasyon/CI) kurulumlarda istem beklemeye takılmaz: 10 saniyelik
uyarıdan sonra yeniden başlatılır. `--no-reboot` bayrağı veya `SANALCP_REBOOT=0`
ortam değişkeni ile tamamen kapatılabilir.

Kurulum sonundaki özet ekranı düzeltildi. Panel girişi 2026-08-20'de root'tan
ayrıldığından beri özet eski metni basmaya devam ediyordu: kullanıcı adı olarak
`root`, parola olarak "sunucunun root parolası" gösteriyordu. Oysa aynı kurulum
panel root girişini kapatıyor (`root_girisi_acik=0`) ve rastgele parolalı gerçek
bir admin hesabı üretiyor; doğru bilgi ekranın epey yukarısındaki kutuda kalıyor
ve özet onunla çelişiyordu. Özet artık üretilen admin kullanıcı adını ve
parolasını gösteriyor, panel root girişinin kapalı olduğunu (SSH root erişiminin
etkilenmediğini) belirtiyor.

**0.9.40** (2026-08-31)

Domain/abonelik detayına yöneticiye özel hizmet paketi değiştirme kontrolü
eklendi. Değişiklik eski ve yeni paket adlarını gösteren onaydan sonra uygulanır;
aynı paket seçiliyken işlem kapalıdır.

Paket değişikliği sonrasında kaynak limitleri ve WAF uygulaması artık gerçek
backend aşamalarından izlenir. Panel plan kaydı, kaynak limitleri, WAF ve
tamamlanma adımlarını canlı yüzde çubuğuyla gösterir; arka plan hataları başarılı
gibi gösterilmek yerine kullanıcıya iletilir. Türkçe ve İngilizce arayüzler
desteklenir, migration gerekmez.

**0.9.39** (2026-08-30)

İngilizce panelde Güvenlik Açıkları (CVE) kartında sabit kalan dört metin
çeviriye bağlandı: "Toplam N benzersiz CVE", "· son tarama {tarih}", "Güvenlik
güncellemelerini kur" düğmesi ve temiz-sistem durumundaki "Sistem güncel" /
"Bilinen bir güvenlik açığı yok". Çeviri anahtarları zaten mevcuttu; yalnız
kullanılmıyordu. Türkçe çıktı değişmez, migration gerektirmez.

**0.9.38** (2026-08-30)

Boşta oturum zaman aşımı etkin olduğunda başarılı yeni girişin önceki oturumun
`last_activity_at` değerini miras alması giderildi. Parola doğrulaması ve çerez
yazımı başarılı olsa bile ilk API isteğinin anında 401 dönmesine yol açan sayaç
artık her başarılı girişte sıfırlanır; davranış regresyon testiyle korunur.

**0.9.37** (2026-08-30)

Üst çubuğa merkezi İşlemler ve Bildirimler görünümü eklendi. İçe aktarım,
uzak hesap taşıma, Laravel deploy ve zararlı yazılım taramaları ortak durum ve
ilerleme modeliyle tek yerden izlenebilir; aktif işler otomatik yenilenir ve
her kayıt ilgili ayrıntı ekranına bağlanır. Güvenlik korelasyonu bu özet
üzerinden de düzenli çalışmaya devam eder.

Panel güncellemesi artık başlamadan önce veritabanı, boş disk alanı, yedek
aracı, mevcut frontend ve nginx yapılandırmasını denetler. Engelleyici bir
kontrol başarısızsa hem arayüzden hem doğrudan API çağrısından güncelleme
reddedilir. Mevcut binary, frontend, migration ve DB otomatik geri dönüş
mekanizması korunmuştur.

Frontend'e gerçek React render test altyapısı eklendi. Panel erişimi, panel hız
limiti ve domain rate-limit ekranları boş veya `null` API listeleriyle sınanır.
Ayar kartları yüklenme, hata ve tekrar-dene durumlarını görünür gösterir;
profil tercihi kaydetme hataları artık sessizce yutulmaz. Domain rate-limit
olay listesindeki ek `null` çökme olasılığı da giderildi.

**0.9.36** (2026-08-29)

Araçlar ve Ayarlar ile domain Rate Limit sayfalarında görülen "Bu sayfa
yüklenemedi" hatası giderildi. Boş liste durumunda API yanıtları `[]` yerine
`null` üretiyordu; panel bu alanlarda doğrudan `.join()` çağırdığı için sayfa
render sırasında çöküyordu. Hata, hiç panel erişim kısıtı veya IP istisnası
tanımlanmamış kurulumlarda — yani varsayılan durumda — her zaman oluşuyordu.

Panel erişim CIDR'leri, panel hız limiti IP istisnaları, domain rate limit IP ve
yol istisnaları ile rate limit olay listesi artık boşken de dizi olarak
kodlanıyor. Kaydetme yanıtı da isteğin ham hâlini değil gerçekten saklanan
normalize listeyi döndürüyor. Panel tarafında bu alanlar ayrıca null'a karşı
korumaya alındı.

**0.9.35** (2026-08-29)

Panel sürüm güncellemesi sırasında beyaz sayfa sorunu giderildi. Yeni sürüm
yayınlandığında `frontend-dist` yeni içerik hash'leriyle değiştiği için, o
sırada açık kalan sekmeler artık var olmayan JavaScript parçalarını istiyor ve
404 alıyordu; panelde hata sınırı bulunmadığından React ağacı tümüyle çöküp
kullanıcıya boş beyaz sayfa gösteriliyordu.

Sayfa parçaları, çeviri dosyaları ve Vite ön-yükleme hataları artık tanınıyor ve
sayfa bir kez otomatik yenileniyor; `index.html` no-store ile sunulduğu için
yenileme taze dosyalarla açılır. Yenileme sorunu çözmezse sonsuz döngü yerine
yenileme ve anasayfa seçenekleri sunan bir hata kartı gösterilir. Panele ayrıca
genel bir hata sınırı eklendi: bir sayfa render sırasında hata verdiğinde menü ve
üst çubuk ayakta kalır, başka bir sayfaya geçiş normal çalışır.

**0.9.34** (2026-08-29)

Gelişmiş zararlı yazılım ve bütünlük motoru eklendi. PHP webshell içerik
sinyalleri ve yazılabilir konum sezgileri açıklanabilir 0–100 risk puanında
birleştirilir; zayıf tek sinyaller bulgu üretmez. WordPress çekirdek dosyaları
resmî checksum'larla tenant kimliğinde doğrulanır. Yanlış-pozitif istisnaları
yalnız tam dosya yolu, imza ve SHA-256 özeti değişmediği sürece geçerlidir.
Karantina ve geri alma işlemleri symlink takip etmez, değişmiş kaynağı veya dolu
hedefi reddeder ve veritabanı hatasında dosya hareketini geri alır.

Güvenlik bildirim merkezi; giriş, ModSecurity/WAF, zararlı dosya ve başarısız
deploy/taşıma olaylarını zaman pencerelerinde ilişkilendirir. Tekrarlanan olaylar
birleştirilir, önem seviyesi atanır ve açık/çözüldü durumu panelden yönetilir.

HTTP/3 desteği nginx yetenek algılamasıyla güvenli biçimde açılabilir. Genel PHP,
WordPress ve PrestaShop cache profilleri ile oturum/sepet/ödeme bypass kuralları
eklendi; yerel HTTPS üzerinden ardışık TTFB ve cache HIT/MISS ölçümü yapılabilir.

**0.9.33** (2026-08-29)

Panel güvenliği ve operasyon araçları genişletildi. Panel API'si IPv4/IPv6
CIDR allowlist'i, geçici erişim ve root kurtarma komutuyla sınırlandırılabilir.
Panel ve domain trafiğine bot/rate-limit profilleri, istisnalar, gözlem kayıtları
ve nginx doğrulamalı geri alma eklendi. cPanel, Plesk ve DirectAdmin'den SSH
üzerinden site taşıma arka plan işleriyle desteklendi. Laravel keşif, maskeli
`.env`, izin listeli Artisan, deploy sağlık kapısı ve otomatik geri alma
özellikleri eklendi.

**0.9.32** (2026-08-28)

Domain yedekleri için doğrulanmış felaket kurtarma katmanı eklendi. Yeni
yedekler güvenli arşiv çıkarma, manifest/domain eşleşmesi, dosya ağacı ve
SHA-256 bütünlüğüyle doğrulanır; SQL dump'ları yalnız rastgele geçici şemaya
yetkili tek kullanımlık MariaDB hesabıyla gerçekten geri yüklenir. En yeni
yedekler periyodik olarak yeniden sınanır. Panel doğrulama durumunu, zamanı ve
hata ayrıntısını gösterir. Her geri yükleme öncesinde aynı kanıt tekrar üretilir;
özet değişmiş veya yedek bozuksa işlem fail-closed engellenir. Yıkıcı iç
işlemlerin kurtarma noktaları doğrulanmadan işlem başlamaz.

**0.9.31** (2026-08-28)

PrestaShop operasyon araç seti eklendi. Panel kurulu mağazayı güvenli biçimde
keşfeder; sürüm, PHP, modül, cache, SSL, veritabanı ve kurulum dizini sağlığını
özetler. Bakım modu açılıp kapatılabilir, bilinen PrestaShop cache dizinleri
temizlenebilir ve en güncel uygulama günlüğü görüntülenebilir. Bakım modu
değişikliklerinden önce tam kurtarma noktası zorunludur; ayrıca elle kurtarma
noktası oluşturulabilir. Yapılandırma ve log erişimleri tenant jail ve symlink
kontrollerinden geçer, mağaza DB işlemleri kendi düşük yetkili hesabıyla yapılır.

**0.9.30** (2026-08-28)

Mevcut Web Sitesi İçe Aktarım aracı güvenli taşıma iş akışına yükseltildi.
Dosya çıkarma ve SQL içe aktarma işlemleri kalıcı arka plan işi olarak çalışır;
yüzde, işlem adımı, hata ve kurtarma dosyası panelde izlenebilir. Her uygulama
öncesinde tam kurtarma noktası zorunludur. Kısmi dosya veya SQL hatasında ilgili
bölüm otomatik geri yüklenir; aynı domain için çakışan aktarım engellenir.
Aktarım sonrası dosya, veritabanı kaydı ve yerel HTTP sağlık kontrolü eklendi.

**0.9.29** (2026-08-28)

Gerçek staging ortamı eklendi. Her staging sitesi ayrı Linux kullanıcısı,
PHP-FPM/vhost ve MariaDB kullanıcı/veritabanıyla sağlanır; WordPress, Laravel ve
PrestaShop bağlantı ayarları staging kimliklerine çevrilir ve arama motorları
`X-Robots-Tag` ile engellenir. Dosyalar ve veritabanı ayrı ayrı canlıya
gönderilebilir. Her gönderimden önce canlı sitenin tam kurtarma yedeğinin
başarıyla oluşturulması zorunludur; yedek alınamazsa işlem başlamaz.

**0.9.28** (2026-08-28)

Cloudflare entegrasyonu eklendi. Yönetici Cloudflare API token'ını bağlantı
doğrulamasından sonra AES-256-GCM ile şifreli saklayabilir. Domainler hesapta
adı birebir eşleşen aktif zone'a bağlanabilir; Cloudflare DNS kayıtları
listelenebilir, oluşturulabilir, güncellenebilir ve silinebilir. A/AAAA/CNAME
kayıtlarında turuncu bulut proxy durumu yönetilebilir ve zone cache'i tek
tıkla tamamen temizlenebilir.

Token tarayıcıya geri gönderilmez. Domain işlemleri mevcut müşteri/bayi kapsam
kontrolünden geçer ve kayıt uçları yalnız domain için sunucu tarafında saklanan
zone kimliğini kullanır. Cloudflare API istemcisi için Bearer doğrulama, zone
eşleme, hata mesajında token sızdırmama ve zone-sınırlı kayıt testleri eklendi.

**0.9.27** (2026-08-28)

Aylık kurtarma tatbikatı servisinin systemd sandbox'ı, `nginx -t` komutunun
`/var/log/nginx` hedeflerini açabilmesine izin verecek şekilde düzeltildi. İlk
0.9.26 kurulumunda restore tatbikatı 47 tabloyu başarıyla açsa da bu sandbox
kısıtı nginx kontrolünü yanlış-negatif gösteriyordu.

**0.9.26** (2026-08-28)

Kurtarma ve regresyon güvenlik ağı eklendi. Son panel veritabanı yedeğini
salt-okunur sağlık kontrolleriyle doğrulayan ve isteğe bağlı olarak rastgele
geçici bir veritabanına gerçekten geri açan `sanalcp-recovery-check` aracı
eklendi. Aylık systemd timer'ı restore tatbikatını otomatik çalıştırıp sonucu
journald'a kaydeder.

Core onarımının sağlık kontrolü başarısız olduğunda rollback artık yalnız
binary ve veritabanını değil frontend ile migration dizinini de onarım öncesi
duruma döndürüyor. Canonical restore assetleri SHA-256 manifestiyle doğrulanıyor;
DB yedekleri de geçerli gzip olmanın yanında `panel` şeması ve tablo tanımı
içermek zorunda. Sağlıksız release senaryosunu gerçek servislerden izole şekilde
sınayan kabul testleri CI'a eklendi.

**0.9.25** (2026-08-28)

Güncelleme betiğinin disk kotası onarımı ext2/3/4 sistemlerini de kapsayacak
şekilde düzeltildi. `/etc/default/grub` doğru olsa bile yeni bir BLS kernel
kaydında eksik kalabilen `rootflags=usrquota` artık tüm kernel kayıtlarına
yeniden uygulanır. Eski ext kurulumlar için kalıcı `quotacheck + quotaon`
systemd birimi de updater tarafından oluşturulup etkinleştirilir.

**0.9.24** (2026-08-28)

Sunucu Bakımı bölümündeki 15 kart, üçlü satırlar halinde beş ayrı renk
ailesiyle gruplandırıldı. Açık ve koyu tema için yeşil, mavi, amber, mor ve
gül tonları eklendi.

**0.9.23** (2026-08-28)

Swap bulunmayan sunucularda boş kaynak listesinin `null` dönmesi nedeniyle
Sunucu Ayarları sayfasını düşüren frontend hatası giderildi. Sistem bilgi
yanıtlarındaki liste alanları artık boşken de tutarlı biçimde `[]` döner.

**0.9.22** (2026-08-28)

Sunucu bakımı bölümüne yedi yeni yönetim ve sağlık kartı eklendi:

- Günlük systemd zamanlayıcısıyla otomatik güvenlik güncellemeleri ve isteğe bağlı
  yeniden başlatma,
- hostname, sunucu IP'leri, A/AAAA ve PTR uyumluluk denetimi,
- disk ve inode uyarı eşikleri ile büyük sistem dizinlerinin görünümü,
- çalışan/kurulu kernel karşılaştırması, son açılış ve reboot gereksinimi,
- journald disk kullanımı, kalıcı boyut sınırı ve güvenli vacuum işlemi,
- NetworkManager üzerinden kesintisiz DNS çözümleyici ayarı ve çözümleme testi,
- swap durumu, kalıcı swap dosyası oluşturma ve swappiness yönetimi.

Tüm değiştiren uçlar yalnız yöneticilere açıktır. DNS ayarı IP doğrulaması ve
çözümleme sonrası geri alma koruması kullanır; swap oluşturma boş disk alanını
önceden denetler ve kullanıcı onayı gerektirir.

**0.9.21** (2026-08-28)

Sunucu Saati kartındaki `Yerel` etiketi daha anlaşılır olması için
`Sunucu saati` olarak değiştirildi ve UTC satırı arayüzden kaldırıldı.

**0.9.20** (2026-08-28)

`Araçlar ve Ayarlar → Sunucu bakımı` bölümüne **Sunucu Saati** kartı eklendi.
Kart yerel ve UTC saati, etkin saat dilimini ve NTP senkronizasyon durumunu
gösterir. Yönetici, sistemin desteklediği saat dilimleri arasından seçim yapabilir
ve otomatik NTP eşitlemesini açıp kapatabilir. Saat dilimi yalnız
`timedatectl list-timezones` çıktısına göre doğrulanır; NTP uygulaması başarısız
olursa önceki saat dilimi geri yüklenir. Manuel saat girişi güvenlik ve zaman
tutarlılığı nedeniyle sunulmaz.

**0.9.19** (2026-08-27)

Yeni domain oluşturma akışına **Reverse Proxy** site tipi eklendi:

- Sunucuda `127.0.0.1` üzerinde çalışan Node.js, Next.js ve benzeri uygulamalar
  alan adına HTTP veya HTTPS upstream ile bağlanabilir.
- WebSocket yükseltmesi, gerçek istemci IP'si ve `X-Forwarded-*` başlıkları
  desteklenir.
- Hedef protokolü, portu ve WebSocket ayarı domainin Web Sunucusu sayfasından
  sonradan değiştirilebilir; `nginx -t` başarısız olursa eski hedef korunur.
- Proxy hedefi iç ağ erişimi ve SSRF riskine karşı yalnız loopback ile sınırlıdır;
  panelin kullandığı `8080`, `8443` ve `10080` portları engellenir.
- Reverse Proxy hesaplarında gereksiz MySQL veritabanı oluşturulmaz. Migration
  `0070_reverse_proxy.sql` mevcut domainleri değiştirmeden yeni proxy alanlarını ekler.

**0.9.18** (2026-08-27)

Uygulama kartlarında jenerik yer tutucu yerine her uygulamanın kendi ikonu
gösteriliyor. Domain alt sayfalarında breadcrumb'ın domain bağlantısını
kaybetmesi giderildi.

**0.9.17** (2026-08-27)

Farklı uygulamaların tanıma (marker) dosyalarının çakışması giderildi: aynı
dizinde birden fazla uygulama tanınabildiğinde liste yanlış uygulamayı
gösterebiliyordu. Çakışma regresyon testiyle korunuyor.

**0.9.16** (2026-08-27)

Uygulama kataloğuna **MediaWiki, Nextcloud ve phpBB** eklendi. Her sürücü kendi
minimum/maksimum PHP sürümünü, veritabanı ön ekini ve tanıma dosyasını
tanımlar; çerçevenin ortak kur/güncelle/sil akışı değişmedi.

**0.9.15** (2026-08-27)

Uygulama kataloğu **Joomla, Drupal, OpenCart, Matomo ve Grav** ile genişletildi.
0.9.14'te CDN kaynağı HTTP 522 döndüğü için gizlenen **PrestaShop** sürücüsü de
bu sürümde yeniden açıldı. Grav veritabanı gerektirmediği için kurulumunda
MySQL veritabanı oluşturulmaz.

**0.9.14** (2026-08-26)

Uygulama kurulum çerçevesi eklendi. Tek bir **Uygulamalar** menüsü altından
1-tık kur / güncelle / sil akışı çalışır; her uygulama `apps.Uygulama` arayüzünü
uygulayıp registry'ye kendini kaydeder, ortak HTTP uçları bu registry üzerinden
çalışır. İlk sürücüler WordPress (mevcut wp-cli akışına dokunmadan adapter ile)
ve PrestaShop'tur.

Tüm veritabanı işlemleri root yetki yoluna (`hesaplar.MySQLDropDB` /
`MySQLDropDBKeepUser`) taşındı; kurulum ve silme geri alma yollarındaki orphan
veritabanı riski giderildi. Arşiv açma zip-slip'e karşı korumalıdır.

PrestaShop, ilk canlı smoke testte `download.prestashop.com` HTTP 522 döndüğü
için bu sürümde panelde gizlendi (0.9.15'te geri açıldı).

**0.9.13** (2026-08-26)

WordPress güncellemesinin `TMPDIR` izin hatasıyla başarısız olması giderildi;
wp-cli artık tenant'ın erişebildiği bir geçici dizinle çalıştırılıyor.

**0.9.12** (2026-08-25)

0.9.11'deki Veritabanı Yönet sayfasında kod incelemesinden kalan küçük
tutarlılık ve sağlamlaştırma düzeltmeleri: isim değiştirmede güvenli sıra
(metadata güncellemesi geri dönüşü olmayan silmeden önce), yedek geri
yüklemede eşzamanlılık sınırı ve büyük dosya kesme davranışı, birkaç
sessizce yutulan hata artık düzgün yanıt dönüyor. Kullanıcı akışlarında
davranış değişikliği yok, migration gerektirmez.

**0.9.11** (2026-08-25)

Domain > Veritabanları ekranına, her veritabanı için ayrı bir "Yönet" sayfası
eklendi:

- **Genel bilgi**: boyut, karakter seti/sıralama, isim değiştirme (uygulama
  yapılandırma dosyalarınızı elle güncellemeniz gerekebileceğine dair uyarıyla).
- **Kullanıcılar**: aynı veritabanına birden fazla kullanıcı bağlanabilir —
  ekleme, şifre değiştirme, kaldırma (tek kullanıcıysa korumalı).
- **Bakım**: tek tıkla gzip yedekleme, .sql/.sql.gz'den geri yükleme, optimize
  ve onar.

phpMyAdmin erişimi bu yeni sayfaya taşındı. Kullanıcı akışlarında geriye dönük
kırılma yok, migration gerektirmez.

**0.9.10** (2026-08-25)

Domain detayında "Paket ve Kaynaklar" kartına inode kullanım/limit göstergesi
eklendi. Kullanıcı akışlarında davranış değişikliği yok, migration gerektirmez.

**0.9.9** (2026-08-25)

Arka plan ağ isteklerinde küçük bir düzeltme. Kullanıcı akışlarında davranış
değişikliği yok, migration gerektirmez.

**0.9.8** (2026-08-24)

Araçlar ve Ayarlar sayfasına, kurulumu tanımlayan bir lisans numarası kartı eklendi.

- **Araçlar ve Ayarlar**: "Sunucu bakımı" bölümüne yeni bir kart eklendi — kurulumun
  benzersiz kimliğini gösterir, kopyala düğmesi vardır. Destek talebinde paylaşmak
  için kullanılabilir.
- Kullanıcı akışlarında davranış değişikliği yok, migration gerektirmez. Mevcut
  kurulumlarda tek `sanalcp-update` yeterli.

## 0.9.x — Panel girişi root'tan ayrıldı, güvenlik sertleştirme

**0.9.7** (2026-08-23)

Hesap parolası, veritabanı parola sıfırlama, e-posta kutusu ve dizin şifre
koruma ekranlarına ortak güvenli parola üreteci ve göster/gizle düğmesi eklendi.
Kullanıcı akışlarında davranış değişikliği yok, migration gerektirmez.

**0.9.6** (2026-08-23)

Frontend'e ESLint (flat config) kuruldu ve CI'a lint job'ı eklendi. Süreçte 33
gerçek `jsx-a11y` hatası düzeltildi (klavye desteği, label-input eşleştirmesi).
Kullanıcı akışlarında davranış değişikliği yok.

**0.9.5** (2026-08-23)

`ACME_STAGING=1` ortam değişkeniyle Let's Encrypt `--issue` çağrılarına staging
sunucusu eklenebiliyor; manuel ve CI testlerinde üretim rate-limit'ine
takılmadan doğrulama yapılabilir. Varsayılan davranış (prod API) değişmedi.

**0.9.4** (2026-08-23)

`/admin/backups/temizle` artık domain başına ayrı `SELECT`/`DELETE` yerine tek
`SELECT ... IN (...)` ve tek `DELETE ... IN (...)` kullanıyor; çok domainli
kurulumlarda yedek temizliğindeki N+1 sorgu giderildi.

**0.9.3** (2026-08-21)

`/api/v1/eklenti-bundle/{ad}/app.js` ucu `RequireAuth` + `BayiVeUstu` zincirine
alındı. Uç HttpOnly çerez taşıdığı için kimlik doğrulamasız erişime açık
kalmamalıydı; güvenlik denetimindeki 4.4 maddesi kapandı.

**0.9.2** (2026-08-21)

Güvenlik güncellemesi:

- Panel oturumu **HttpOnly çerezine** taşındı — tarayıcıda çalışan bir XSS ile
  oturum çalınamaz.
- Antivirüs taramasına eşzamanlılık sınırı eklendi
  (`PANEL_AV_MAX_CONCURRENT`, varsayılan 1).
- Ortak `WriteExecError` yardımcısı ve onu kullanan 9 uç ile uzun komut
  çıktılarının hata yanıtıyla sızması ve DoS yüzeyi kapatıldı.

Mevcut kurulumlarda tek `sanalcp-update` yeterli, migration yok.

**0.9.1** (2026-08-20)

Arayüz düzeni ve yeni bir sunucu kapatma düğmesi.

- **Araçlar ve Ayarlar**: sunucu bakımı kartları tek sütunda tam genişlik
  duruyordu, artık geniş ekranda satırda üç, tablette iki kart. "Sunucu root
  parolasıyla panel girişi" kartı tam satır kaldı.
- **Sunucuyu Kapat** düğmesi eklendi (sayfa başlığında, turuncuya alınan
  yeniden başlat düğmesinin yanında, kırmızı). Yazılımsal kapatmadır:
  `systemctl poweroff` ile systemd servisleri düzenli durdurur, dosya
  sistemlerini senkronize edip söker, sonra gücü keser. **Yeniden başlatmanın
  aksine bu işlem kendiliğinden geri gelmez** — sunucuyu ancak sağlayıcı
  panelinden veya fiziksel olarak açabilirsiniz. Bu yüzden onay adımı daha
  ağırdır: kutuya `KAPAT` yazmak gerekir. Uç yalnız admin rolüne açıktır.
- **Profil**: kartlar geniş ekranda ikişerli; Tercihler tam satır.
- **Listeler ilk sütuna göre alfabetik sıralanıyor** — domainler, DNS, e-posta,
  veritabanları, SSL, kullanıcılar, müşteriler. Sıralama Türkçe harf sırasına
  göre yapılır (ç, ğ, ı, ö, ş, ü) ve sayı içeren adlarda doğal sıra korunur
  (`db2` < `db10`). Not: SSL listesi önceden en yakın sertifika bitişine göre
  sıralıydı; süresi dolan/dolmak üzere olanlar artık listenin başında değil ama
  üstteki özet rozetlerinde ve satır rozetlerinde görünmeye devam ediyor.
- Panel favicon'una `.ico` yedeği eklendi.

**0.9.0** (2026-08-20)

Panel girişi artık sunucunun root parolasına bağlı değil.

İlk sürümden bu yana panele `root` kullanıcı adı ve **sunucunun root parolasıyla**
giriliyordu; panel bu parolayı doğrudan `/etc/shadow`'dan okuyup doğruluyordu.
Bunun iki sonucu vardı:

- Sunucuda `passwd` çalıştırmak panel girişini **haber vermeden** bozuyordu.
  Panel başarısız girişleri `audit_log` tablosuna yazdığı için journald sessiz
  kalıyor, sebep de görünmüyordu.
- Panelin web arayüzü, sunucunun en yetkili kimlik bilgisini kabul eden bir
  saldırı yüzeyi haline geliyordu.

Bu sürümde panelin kendi yönetici hesabı var:

- **Yeni kurulumlar** kurulum sırasında bir admin hesabı üretir
  (`--admin-kullanici` ile ad verilebilir, varsayılan `admin`). Parola yalnız
  ekrana basılır, diske yazılmaz. Kurulum root girişini kapalı başlatır.
- **Mevcut kurulumlarda hiçbir şey bozulmaz.** Migration `root_girisi_acik`
  sütununu `1` varsayılanıyla ekler; bugüne kadar nasıl giriyorsanız öyle
  girmeye devam edersiniz.
- Geçiş üç adım: Kullanıcılar ekranından admin rolünde bir hesap açın, o
  hesapla girip çalıştığını doğrulayın, sonra **Araçlar ve Ayarlar → Sunucu
  Bakımı → "Sunucu root parolasıyla panel girişi"** kartından root girişini
  kapatın.

İki bağımsız kilitlenme koruması var. Root dışında aktif bir yönetici hesabı
yoksa panel root girişini kapatmayı reddeder; ayrıca root girişi kapalıyken
sistemdeki son gerçek yönetici hesabı silinemez ya da askıya alınamaz (root
satırı bu sayıma dahil edilmez, çünkü o hesapla girilemez — sayılsaydı tek
adımda panele kimsenin giremeyeceği bir duruma düşülebilirdi).

Kapatmak yalnız yeni girişleri durdurmakla kalmaz: **root'un canlı oturumları
düşürülür** (`auth_version` artırılır) ve **API token'ları iptal edilir**. Üç
yazma tek transaction'da yapılır — yarım uygulanmış bir kapatma (bayrak kapalı,
token'lar canlı) mümkün değil.

**SSH root erişimi bu değişiklikten hiç etkilenmez**; sunucunun root parolası
iptal edilmez. Panelden kilitlenirseniz SSH ile geri açabilirsiniz:

```
mysql panel -e "UPDATE panel_ayarlari SET root_girisi_acik=1 WHERE id=1;"
```

---

## 0.8.x — Debian desteği

**0.8.0** (2026-08-19)

Panel artık **Debian 12 (bookworm)** ve **Debian 13 (trixie)** üzerinde kuruluyor.
AlmaLinux/RHEL desteği aynen sürüyor; tek bir kurulum betiği iki aileyi de kuruyor.

- PHP paketleri Debian'da **deb.sury.org** deposundan gelir (7.4 – 8.5).
  Depo anahtarı `signed-by` ile yalnız o depoya kapsanır, `apt-key` kullanılmaz.
- İşletim sistemi farkları tek bir tabloda toplandı: Go tarafında
  `internal/osfam`, shell tarafında `assets/ops/sanalcp-ortak.sh`. İkisinin
  tutarlı kalmasını bir eşlik testi zorluyor.
- **Dovecot 2.4** için ayrı yapılandırma şablonu var (2.4 yapılandırma dilini
  kırdı: `mail_location` kalktı, `passdb`/`userdb` adlandırılmış blok istiyor,
  SQL bağlantısı ayrı dosyadan değil conf'un içinden geliyor). Şablon dovecot
  sürümüne göre otomatik seçilir.
- BIND, Pure-FTPd, OpenDKIM ve PHP-FPM için Debian'a özgü yol/servis/paket
  adları ve AppArmor kısıtları karşılandı.

### 🔴 Disk kotası düzeltmesi — Debian 12 ve 13'ü etkiliyordu

Kota, **ikinci yeniden başlatmadan itibaren sessizce devre dışı kalıyordu.**
Muhasebe çalışmaya devam ediyor, `repquota` doğru sayılar gösteriyor, panel
kullanımı doğru raporluyor — ama **hiçbir limit uygulanmıyordu**. Tek bir
yeniden başlatma sonrası her şey sağlıklı görünüyordu; hata ancak ikinci
yeniden başlatmada ortaya çıkıyor.

Nedeni, kota biriminin `quotacheck` (bir kez gerekli) ile `quotaon` (her
açılışta gerekli) işlerini tek bir "yalnızca ilk açılışta çalış" koşulunun
altında birleştirmesiydi.

Bu güncelleme kotayı **yeniden başlatma gerekmeden** onarır. Güncelleme
sonrasında teyit etmek için:

```
quotaon -p -u /        # "user quota on / (...) is on" demeli
```

### Diğer düzeltmeler

- Postfix `main.cf`'te tekrar eden anahtarlar temizleniyor. Bunlar postfix'i
  bozmuyordu (sonuncusu kazanır) ama her `postfix`, `postconf`, `sendmail` ve
  `mailq` çağrısında `overriding earlier entry` uyarısı bastırıp gerçek
  uyarıları gömüyordu.
- Kullanılmayan PHP-FPM master süreçleri durduruluyor. Debian'da paket
  kurulumu her PHP sürümünün servisini başlattığı için taze bir sunucuda yedi
  master birden çalışıyordu (ölçüldü: ~197 MB). Bir sürümün master'ı artık
  yalnız o sürümde site varsa çalışır; gerektiğinde otomatik açılır.
- Sanal posta kutularına teslimat Dovecot 2.4'te hiç çalışmıyordu (iki ayrı
  neden: INBOX'ın mbox yolunda aranması ve LMTP'nin adresten alan adını
  kırpması). Her ikisi de düzeltildi.

### Bu sürümde Debian/Ubuntu'da kapalı olan iki özellik

Yarım çalışan bir ekran yerine, çalışmayan özellik kapatıldı ve **nedeni panelde
yazıyor**:

- **Apache backend** (nginx önde, Apache arkada). Yapılandırma düzeni Debian'da
  RHEL'dekinden tamamen farklı. Seçenek artık menüde devre dışı görünüyor ve
  gerekçesi yazıyor. Bu sürümden önce seçilebiliyordu ve seçilince **site 502
  veriyordu**; güncelleme, bu duruma düşmüş siteleri açılışta otomatik onarır.
- **Güvenlik açığı (CVE) taraması.** RHEL'in `dnf updateinfo --security`
  çıktısının Debian'da doğrudan karşılığı yok. Panel "0 açık" gibi yanıltıcı bir
  sayı göstermek yerine taramanın desteklenmediğini söylüyor.

### Ubuntu

**Ubuntu 26.04 LTS ve Ubuntu 24.04 LTS** canlı test edildi ve desteklenmektedir.

Ubuntu 24.04'e özgü bir düzeltme — **disk kotası çalışmıyordu.** Ubuntu, ext4
kotasının ihtiyaç duyduğu çekirdek modülünü ayrı bir pakette
(`linux-modules-extra-*`) gönderiyor ve bulut sunucu imajları bu paketi
kurmuyor. Modül olmadan kota hiç açılmıyor, üstelik her şey sağlıklı görünüyor:
disk kotalı bağlanmış, muhasebe dosyası üretilmiş, servis çalışıyor — ama tek
bir limit uygulanmıyor. Kurulum artık modülü tespit edip gerekiyorsa kuruyor,
her açılışta yüklenmesini sağlıyor ve sağlayamazsa bunu açıkça bildiriyor.

Ubuntu'ya özgü bir düzeltme: Ubuntu'nun rspamd paketi `redis-server`'ı
Recommends olarak getirir (Debian'da bu satır `valkey-server | redis-server`
alternatifli olduğu için sorun çıkmaz). Sonuçta hem valkey hem redis kurulu ve
etkin kalıyor, açılışta 6379 portunu hangisinin kapacağı yarışa dönüşüyordu —
kaybeden servis başlamıyor, panelin önbellek ayarları (bellek limiti, tahliye
politikası, parola) uygulanmamış hâlde kalıyordu. Kurulum artık kullanılmayan
önbellek servisini devre dışı bırakıyor.

---

## 0.7.x — Otomatik saldırı engelleme ve yönetim API'si

**0.7.1** (2026-08-18) — **acil yama, 0.7.0 kullanılmamalıdır**

0.7.0 ile gelen `0068_api_tokenlari.sql`, `api_tokenlari.user_id` kolonunu `BIGINT`
(signed) tanımlıyordu; oysa referans verdiği `users.id` panelin ilk sürümünden beri
`BIGINT UNSIGNED`. InnoDB, foreign key'de imza uyuşmazlığını reddeder:

```
ERROR 1005 (HY000): Can't create table `panel`.`api_tokenlari`
(errno: 150 "Foreign key constraint is incorrectly formed")
```

Migration başarısız olunca panel açılışta duruyor ve **hiç ayağa kalkmıyordu**. Bu,
0.7.0'a güncelleyen ve sıfırdan kuran tüm sunucuları etkiliyordu. 0.7.1 yalnızca bu
kolonu `BIGINT UNSIGNED` yapar; başka davranış değişikliği yoktur.

- **Geri dönüş (rollback) güçlendirildi.** `sanalcp-update` başarısız bir güncellemede
  binary'yi ve veritabanını güncelleme öncesine döndürüyor, ancak migration
  **dosyalarını** diskte yeni sürümden bırakıyordu. Migration çalıştırıcısı uygulanmış
  her dosyanın sha256'sını tuttuğu için, veritabanı geri alınıp diskteki dosya yeni
  kalınca eski binary de açılamıyor ve kurtarılabilir bir geri dönüş tam kesintiye
  dönüşüyordu. Artık migration dizininin tam kopyası güncelleme öncesinde alınıyor ve
  geri dönüşte yerine konuyor. Bu iyileştirme, sunucudaki güncelleme aracı yenilendiği
  için **bir sonraki** güncellemeden itibaren geçerlidir.
- **Migration smoke testi eklendi.** Depodaki tüm migration'lar boş bir veritabanına
  baştan sona uygulanıp idempotentlikleri doğrulanıyor. Bu hata hiçbir teste
  takılmamıştı çünkü migration'lar test sırasında hiç çalıştırılmıyordu.

**0.7.0** (2026-08-18)

**Otomatik saldırı engelleme (fail2ban muadili) — YENİ.** Panel, sistem servislerinin
kimlik doğrulama hatalarını journald üzerinden izler; bir IP belirlenen pencerede eşiği
aşarsa nftables tarafında süreli engellenir. Ayrı bir servis (fail2ban, Python) kurulmaz —
izleyici panelin kendi binary'sindedir.

- İzlenen: **sshd** (hatalı parola, geçersiz kullanıcı, azami deneme aşımı, preauth
  kopmaları), **dovecot** (başarısız giriş), **postfix** (SASL / SMTP AUTH), **pure-ftpd**.
- Varsayılan: 10 dakikada 5 başarısız deneme → 60 dakika ban. Üçü de ayarlanabilir.
- **Varsayılan KAPALI** — açmadığınız sürece hiçbir davranış değişmez.
- Otomatik banlar elle eklenen kurallarla aynı tabloda tutulur: güvenlik duvarı ekranında
  "Otomatik" rozetiyle ve bitiş saatiyle görünür, tek tek veya topluca kaldırılabilir.
- Kilitlenme korumaları: izin listesindeki IP/CIDR'ler, sunucunun kendi adresleri,
  loopback ve link-local **asla** engellenmez; banlar sürelidir ve süresi dolan kural
  anında etkisiz kalır; nftables'ta `established,related accept` en üstte olduğu için
  açık SSH oturumları kopmaz.

**Yönetim API'si — YENİ.** Panelin yaptığı her şey HTTP üzerinden de yapılabilir; arayüzün
kullandığı API'nin aynısıdır, kısıtlı bir alt küme değil. Belgeler: **`docs/API.md`**.

- Kişisel erişim token'ları: **Profil ve Tercihler → API Token'ları**. Ham token yalnız bir
  kez gösterilir; sunucuda SHA-256 özeti saklanır.
- **Token ayrı bir izin sistemi değildir, sahibinin kimliğidir.** İstek, sahibi oturum
  açmış gibi işlenir ve mevcut yetki katmanından (AdminOnly / BayiVeUstu / MusteriScope /
  KapsamSQL) geçer. Bayinin token'ı bayiden fazlasını yapamaz; rol değişince yetki de
  değişir; hesap askıya alınırsa token çalışmaz; token iptal edilirse anında geçersizdir.
- Token başına son kullanım zamanı ve IP kaydedilir; isteğe bağlı son kullanma tarihi
  verilebilir. Hesap başına en fazla 20 token.
- API token'larına 2FA ve boşta oturum zaman aşımı uygulanmaz (ikisi de interaktif oturum
  kuralıdır) — bu, `docs/API.md` içinde açıkça belirtilmiştir.

migrations: **0067** (otomatik ban), **0068** (api_tokenlari).

## 0.6.x — Yedekleme, oturum güvenliği ve arayüz sadeleştirme

**0.6.5** (2026-08-18)
- **Düzeltme — plan kaynak limitleri artık anında uygulanıyor.** Bir hizmet planının
  CPU/RAM/görev, disk G/Ç, XFS kota veya MySQL Governor limitlerini değiştirdiğinizde bu
  değerler o plandaki mevcut domainlere **yalnızca panel yeniden başlarken** iniyordu
  (`HealTenantFPM`); yönetici planı kaydedip "hiçbir şey değişmedi" görüyordu. Artık plan
  kaydedilir kaydedilmez arka planda uygulanıyor — WAF ayarında zaten var olan desenin aynısı.
  Uygulama `LimitleriReAssert` üzerinden yapılır: slice'a `systemctl set-property --runtime`
  ile canlı yazar, **çalışan süreçleri öldürmez**, idempotenttir.
- README (tr+en) yeniden konumlandırıldı: **ücretsiz paneller arasındaki yeri** ve asıl
  farklılaştırıcısı olan **kaynak izolasyonu katmanı** (CloudLinux CageFS/LVE/MySQL Governor
  muadilleri) en üste taşındı; hangi katmanın hangi çekirdek aracıyla kurulduğu tabloya döküldü.
- README'ye **"Şu an yapmadıklarımız"** bölümü eklendi: Debian/Ubuntu, otomatik saldırı
  engelleme, kapsamlı REST API, Node.js/Python, slave DNS, WordPress dışı uygulama kurucu.
- **"Her zaman ücretsiz"** taahhüdü yazıldı: pro sürüm, özellik kilidi, lisans anahtarı ve
  kullanıcı/domain sınırı yok; MIT. WHMCS modülünün neden planlanmadığı açıklandı.
- AlmaLinux 9.4+ desteği README'de görünür hâle getirildi (0.5.24'ten beri vardı, yalnız
  "AlmaLinux 10" yazıyordu). Disk kotası için XFS gereksinimi sistem gereksinimlerine eklendi.

**0.6.4** (2026-08-18)
- README'ye **"Kimlik doğrulama ve hesap modeli"** bölümü eklendi: `root` parolasının neden
  `/etc/shadow`'da tutulduğu (kilitlenmeye karşı kurtarma yolu) ve günlük yönetim için
  root'tan bağımsız, kendi bcrypt parolası olan **yönetici hesabı** açmanın nasıl yapıldığı anlatıldı.
- `root` ile giriş yapan yöneticiye **Kullanıcılar** ekranında kendi panel hesabını açmasını
  öneren, kapatılabilir bilgi kutusu eklendi (özellik zaten vardı, keşfedilebilir değildi).
- README'deki hatalı **"PAM ile doğrulanır"** ifadesi düzeltildi — `root` girişi PAM ile değil,
  `/etc/shadow` hash'i üzerinden (yescrypt native Go) doğrulanıyor.
- Panelin **0.x beta** durumu ve eksik özellikleri (WHMCS modülü, kapsamlı REST API,
  Debian/Ubuntu desteği, fail2ban, slave DNS) README'de açıkça belirtildi.
- Bu dosya (CHANGELOG.md) eklendi.

**0.6.3** (2026-08-18)
- Dosya Yöneticisi'nde üstteki yol satırı klasör gezinirken güncellenmiyordu; artık
  bulunduğunuz dizini canlı gösteriyor.
- Domain sayfasındaki "Yapılandırma" kartına varsayılan dosya yolu eklendi.
- **SSH Erişimi'ne anahtar çifti üretme geldi**: panel ed25519 anahtarı üretir, genel
  anahtarı `authorized_keys`'e ekler; özel anahtar sunucuda saklanmaz, yalnızca bir kez
  indirmeniz için gösterilir.
- Domain sayfasındaki sekme çubuğu kaldırıldı, yerine "Hızlı Linkler" çubuğu eklendi.
  Ana panoya eksik olan Apache & Nginx kartı eklendi.

**0.6.2** (2026-08-18)
- Sürüm güncelleme bildirimi tek bir başlık çubuğuna indirildi (önem derecesine göre
  renklenir, uzun duyuru metni okla açılır). Mobilde ekranı kaplama sorunu giderildi.

**0.6.1** (2026-08-17)
- **Güvenlik:** panel vhost'u elle özelleştirilmiş kurulumlarda "no-cache heal" bloğu
  CSP'ye `unsafe-inline`/`unsafe-eval` geri sokuyordu; düzeltildi ve CI testine bağlandı.
- **Güvenlik:** subdomain / ek alan adı / webmail vhost'ları artık ana domainle aynı
  güvenlik header setini alıyor (önceden CSP/HSTS/Referrer-Policy yoktu).
- `sanalcp-optimize` sysctl ayarı yapıyor; MariaDB `buffer_pool` ve nginx
  `worker_connections` elle yükseltilmiş değerin altına düşürülmüyor.
- Boşta oturum zaman aşımı eklendi (varsayılan kapalı).

**0.6.0** (2026-08-12)
- Yedekleme: toplu yedekleme düğmesi düzeltildi, manuel saklama süresi ve toplu
  temizlik eklendi.

---

## 0.5.x — Çok kullanıcılı model, e-posta olgunlaşması, AlmaLinux 9 desteği

- **0.5.29** — Go 1.24 ile derlenmiş release.
- **0.5.28** — WordPress kurulumunda parolaların komut satırına (process listesi) düşmesi engellendi.
- **0.5.24 – 0.5.27** — AlmaLinux 9.4 / 9.8 kurulum desteği; canlı testte bulunan kurulum
  hataları ve reboot sonrası panele erişilememe sorunu düzeltildi.
- **0.5.21 – 0.5.23** — TailAdmin tabanlı arayüz yenilemesi, admin veritabanı işlemleri,
  2FA kodunda otomatik gönderim, altbilgide sürüm bilgisi.
- **0.5.14 – 0.5.20** — Sahiplik zinciri (hesap anında oluşur, domain devri arayüzden),
  site tipi seçimi, SSL rozetinin üç duruma ayrılması (self-signed artık kırmızı),
  domain oluştururken bayi seçimi, alt alan adı iyileştirmeleri.
- **0.5.6 – 0.5.13** — Dosya yöneticisi symlink saldırılarına karşı sertleştirildi;
  panel nginx yapılandırması kendi kendini güncelleyebilir hale getirildi; e-posta
  hizmeti için ayrı "geçici kapat" ve "kapat ve sil" akışları.
- **0.5.0 – 0.5.5** — Webmail müşterinin kendi alan adından sunulmaya başlandı,
  Dovecot PAM passdb kapatılarak webmail yavaşlığı giderildi; SSH port 22, Redis,
  yüksek paranoyalı WAF ve antivirüs taraması için kaynak/güvenlik uyarıları.

---

## 0.4.x — E-posta barındırma ve DNS olgunlaşması

- Webmail (Roundcube) erişimi ve DNS doğrulama paneli; webmail'den gönderim sorunları
  giderildi, SMTP onarımı panel açılışına taşındı.
- DNS: zone içi nameserver'lar için glue A kayıtlarının korunması/üretilmesi, domain
  oluşturma penceresinde nameserver gösterimi.

---

## 0.3.x ve öncesi — Temel altyapı, güvenlik sertleştirme ve izolasyon

Bu dönem panelin çekirdeğinin kurulduğu ve en yoğun güvenlik çalışmasının yapıldığı
aşamadır (14 Temmuz 2026 – ilk yayın).

**Kiracı (tenant) izolasyonu — CloudLinux muadili katman**
- Domain başına ayrı Linux kullanıcısı, ayrı PHP-FPM havuzu, `open_basedir`,
  `disable_functions`, kiracıya ait `session.save_path` / `upload_tmp_dir` (CageFS eşdeğeri).
- Plan tabanlı kaynak limitleri: CPU/bellek, mutlak disk G/Ç ve IOPS sınırları (LVE eşdeğeri).
- MySQL Governor eşdeğeri veritabanı limitleri (kernel modülü gerektirmeden, native MariaDB).
- Per-tenant XFS disk + inode kotası.
- "Ek direktifler" alanı allowlist'ten geçer: `php_admin_*`, `user`, `group`,
  `open_basedir`, `disable_functions` müşteri tarafından ezilemez.

**Güvenlik sertleştirme turları**
- phpMyAdmin IDOR / kırık nesne yetkilendirmesi (OWASP A01) düzeltmesi.
- nginx yapılandırma enjeksiyonu, DNS ve FTP satır enjeksiyonu, JWT CVE'si.
- `unzip`/`tar`/geri yükleme jail-escape kapatıldı; dosya yöneticisinde TOCTOU
  symlink güvenliği (`openat2`/`*at` çağrıları).
- WordPress veritabanı işlemlerinde kiracılar arası drop koruması.
- Askıya alma (suspend) bypass'ı, yükleme kaynaklı DoS, DNS AXFR kilidi, opt-in DNSSEC,
  FTP-TLS sertifika zorunluluğu.
- Giriş kaba-kuvvet koruması (IP başına kayan pencere + kademeli gecikme + kilit),
  native yescrypt doğrulama, 2FA replay koruması, spoof edilemez istemci IP tespiti.

**Çekirdek özellikler**
- Tek satır kurulum (`curl | bash`), tam otomatik AlmaLinux kurulumu.
- Rol tabanlı yetkilendirme (yönetici / bayi / müşteri), çok kullanıcılı giriş,
  bayi kotaları ve zincirleme askıya alma; yatay/dikey yetki regresyon test paketi.
- Native e-posta barındırma: Postfix + Dovecot + OpenDKIM + Roundcube, otomatik
  DKIM/SPF/DMARC, yönlendirme (forwarder) ve catch-all.
- WAF: ModSecurity v3 + OWASP CRS (plan ve domain bazlı, opt-in).
- cPanel hesap aktarımı (`cpmove` arşivi) ve çoklu veritabanı aktarımı.
- Site kullanıcısı için CLI: `db:export`, `db:import`, `cache:purge` (yalnızca
  127.0.0.1 dinleyen ikinci listener + bearer token).
- Özel (ham) nginx vhost modu — `nginx -t` doğrulamasından geçmeden canlıya uygulanmaz.
- Panel için özel alan adı + otomatik Let's Encrypt; port yazmadan `https://<domain>` erişimi.
- Sürükle-bırak pano widget düzeni, CVE/KernelCare güvenlik açığı widget'ı,
  tam duyarlı (responsive) mobil arayüz.
- Günlük panel veritabanı yedeği ve güncellemede fail-closed dump; güncelleme
  başarısız olursa binary + veritabanı otomatik geri alınır.
