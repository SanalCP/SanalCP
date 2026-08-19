# Değişiklik Günlüğü

Bu dosya SanalCP'nin sürüm geçmişini özetler. Ayrıntı için `git log`'a bakabilirsiniz.

Sürümleme `BÜYÜK.KÜÇÜK.YAMA` biçimindedir. Panel **1.0 öncesi (beta)** olduğu için
`0.x` serisinde küçük sürümler arasında davranış değişikliği olabilir; her sürüm
`sanalcp-update` ile güvenli (yedekli + geri dönüşlü) şekilde uygulanır.

Panelin kurulu sürümünü `Araçlar ve Ayarlar → Panel Güncellemesi` ekranından veya
sayfa altbilgisinden görebilirsiniz.

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

