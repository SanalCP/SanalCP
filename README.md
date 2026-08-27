<p align="center">
  <a href="https://github.com/sanalcp/sanalcp"><b>🌐 GitHub</b></a> &nbsp;·&nbsp;
  <a href="README.md">Türkçe</a> &nbsp;·&nbsp;
  <a href="README.en.md">English</a>
</p>

# SanalCP

**Ücretsiz ve açık kaynak hosting kontrol paneli.** Boş bir **AlmaLinux, Debian veya Ubuntu** sunucuyu tek komutla komple bir barındırma sistemine çevirir — nginx + MariaDB + çok sürümlü PHP + Valkey (Redis) + e-posta + phpMyAdmin + güvenlik duvarı, hepsi otomatik kurulur ve ayarlanır.

> **Her zaman ücretsiz.** Pro sürüm, özellik kilidi, lisans anahtarı, kullanıcı/domain
> sınırı yoktur. Panelin tamamı **MIT** lisanslıdır — ticari kullanım dahil, kısıtsız.
> Ücretli bir sürüm çıkarmayı planlamıyoruz.

## Neden bir panel daha?

Ücretsiz paneller (HestiaCP, CloudPanel, CWP, FastPanel) barındırma işini büyük ölçüde
çözüyor. Ama paylaşımlı barındırmanın asıl derdi olan **kaynak izolasyonunu** çözmüyorlar:
tek bir müşterinin sitesi CPU'yu, diski veya veritabanını tüketince sunucudaki herkes
etkileniyor. Bunun ticari çözümü CloudLinux (CageFS + LVE + MySQL Governor) ve ücretli.

SanalCP bu katmanı **çekirdeğin kendi araçlarıyla, ücretsiz** kuruyor:

| Katman | Nasıl | CloudLinux muadili |
|---|---|---|
| **CPU / RAM / görev limiti** | domain başına systemd slice — `CPUQuota`, `MemoryMax`, `TasksMax` | LVE |
| **Disk G/Ç limiti** | slice içinde mutlak okuma/yazma MB/s + IOPS throttle | LVE (io/iops) |
| **Veritabanı limiti** | native MariaDB `MAX_USER_CONNECTIONS`, `MAX_QUERIES_PER_HOUR`, `MAX_UPDATES_PER_HOUR` | MySQL Governor |
| **Disk kotası** | per-tenant XFS disk + inode kotası | — |
| **Dosya sistemi izolasyonu** | domain başına ayrı Linux kullanıcısı + ayrı PHP-FPM havuzu + `open_basedir` + kiracıya ait `session.save_path` / `upload_tmp_dir` | CageFS |

Limitler hizmet planına bağlıdır: plan üzerinde değeri değiştirirsiniz, panel ilgili
kiracılara yeniden uygular. **Kernel modülü, yamalı kernel veya ek lisans gerekmez.**

Bunun yanında:

- **Otomatik saldırı engelleme** dahildir — SSH, e-posta ve FTP'ye yapılan kimlik
  doğrulama saldırıları izlenir, eşiği aşan IP nftables tarafında süreli engellenir
  (fail2ban muadili, ayrı bir servis kurmadan). Ayrıntı için aşağıya bakın.
- **Tam yönetim API'si** — panelin yaptığı her şey HTTP üzerinden de yapılabilir;
  arayüzün kullandığı API'nin aynısıdır, kısıtlı bir alt küme değil. Token sahibinin
  rolüyle çalışır. Bkz. **[docs/API.md](docs/API.md)**.
- **SELinux `Enforcing` ile çalışır** — kapatmanızı istemez.
- **Tam e-posta yığını** dahildir (Postfix + Dovecot + OpenDKIM + Roundcube, otomatik DKIM/SPF/DMARC).
- **Tek statik Go binary** + React arayüz; mobil uyumlu.
- **Güncelleme geri alınabilir**: migration öncesi zorunlu veritabanı yedeği, sağlıksız
  başlarsa binary **ve** veritabanı otomatik eski hâline döner.

> ### ⚠️ Bu bir 0.x (beta) sürümüdür — okumadan kurmayın
>
> SanalCP genç bir projedir (ilk yayın: Temmuz 2026) ve iki kişilik bir ekiple geliştirilir.
> Kod açık, testli ve güvenlik tarafı ciddiye alınmıştır; ancak yıllardır sahada olan
> panellerin saha tecrübesine **henüz** sahip değildir.
>
> - **Önerilen kullanım:** test/geliştirme sunucuları, kendi projeleriniz, değerlendirme.
> - **Önerilmeyen kullanım:** yedeği olmayan, gelir getiren müşteri yükünü doğrudan
>   taşımak. Böyle bir kullanım için önce ayrı bir sunucuda deneyin.
> - **Kurulum yıkıcıdır:** betik boş bir sunucu varsayar; üzerinde çalışan bir servis
>   olan makinede çalıştırmayın.
>
> Hata bildirimi ve eleştiri açıkça beklenir: [issue açın](https://github.com/sanalcp/sanalcp/issues).

## Şu an yapmadıklarımız

Dürüst olmak, sonradan hayal kırıklığı yaşatmaktan iyidir. Bu maddeler bugün **yoktur**;
yol haritasındadırlar:

| Eksik | Durum |
|---|---|
| **Node.js / Python uygulama desteği** | Yalnızca PHP (7.4 – 8.5) ve statik site. |
| **Slave / cluster DNS** | Tek sunucu üzerinde birincil DNS. `allow-transfer` kapalıdır (zone enumerasyonuna karşı). |
| **Geniş uygulama kataloğu** | Tek tık kurulum WordPress, Joomla, PrestaShop, Drupal, OpenCart ve Matomo için; diğer PHP uygulamaları sırada. |

> **WHMCS modülü yok ve planlanmıyor** — WHMCS ücretli bir üründür, biz ücretsiz kalmayı
> tercih ediyoruz. Panel MIT lisanslı olduğu ve tam bir yönetim API'si sunduğu için
> ([docs/API.md](docs/API.md)), bu entegrasyona ihtiyacı olan kendisi yazabilir.

## Tek satır kurulum

Temiz bir sunucuda (min. 2 GB RAM) **root** olarak:

```bash
curl -fsSL https://sanalcp.com/kur | bash
```

Bu adres, yayınlanmış son sürümün **commit'ine ve arşiv SHA-256'sına sabitlenmiş**
bir betik sunar; indirilen paket doğrulanmadan kurulum başlamaz.

`main` dalını doğrudan çeken tek satır, **bütünlük doğrulaması olmadan artık
çalışmaz** — `install.sh` SHA-256 verilmeden kurulumu reddeder. Sabitlenmiş
kurulum için hash'i bu depodan bağımsız, güvendiğiniz bir kanaldan alın:

```bash
curl -fsSL https://raw.githubusercontent.com/sanalcp/sanalcp/main/install.sh \
  | SANALCP_REF=<commit-sha> SANALCP_SHA256=<hash> bash
```

Doğrulamayı bilinçli olarak atlamak (yalnızca GitHub'ın TLS'ine güvenmek)
istiyorsanız `SANALCP_INSECURE=1` vermeniz gerekir; üretim sunucularında
önerilmez.

Kurulum ~5-10 dakika sürer (paket indirmeleri). Bittiğinde panel adresi + giriş bilgileri ekrana yazılır.

## Kurulum sonrası

- **Panel:** `https://SUNUCU_IP:8443` (self-signed sertifika — tarayıcı uyarısını geçin)
- **İlk giriş:** kurulumun sonunda **ekrana basılan** yönetici kullanıcı adı ve parolası
  (varsayılan kullanıcı adı `admin`; `--admin-kullanici` / `--admin-parola` ile değiştirilebilir)

> ⚠️ **Bu parola bir kez gösterilir ve hiçbir dosyaya yazılmaz.** Kurulum çıktısındaki
> kutuyu kaydedin. Kaybederseniz aşağıdaki "Kurtarma" bölümüne bakın.

İlk girişten sonra **Profil → İki Adımlı Doğrulama (2FA)** açmanız önerilir.

## Kimlik doğrulama ve hesap modeli

Panelde **iki ayrı parola dünyası** vardır; bunu bilerek kullanmak güvenliğiniz açısından önemlidir.

### 1. Panel hesapları — birincil giriş yolu

Panele giriş, `users` tablosundaki **gerçek panel hesaplarıyla** yapılır. Bu hesapların
parolası panel veritabanında **bcrypt (cost 12)** ile saklanır ve sunucunun root
parolasıyla **hiçbir ilgisi yoktur.**

Kurulum betiği bu hesabı sizin için oluşturur ve kimlik bilgilerini **yalnız ekrana**
basar (diske yazmaz). Kullanıcı adı varsayılan olarak `admin`'dir; `--admin-kullanici`
ile değiştirebilirsiniz — ancak `root` adı kabul edilmez (aşağıdaki 2. maddeye ait
ayrı bir yoldur).

Yeni yönetici/bayi hesaplarını **Kullanıcılar → Yeni hesap** ekranından açarsınız.

### 2. `root` — break-glass (acil kurtarma) yolu, **varsayılan kapalı**

Panel, `root` kullanıcı adıyla girişte parolayı veritabanında değil `/etc/shadow`'da
arar (yescrypt native Go ile; eski sha512/sha256/md5crypt formatları için yedek yol).
Kilitli (`!`, `!!`, `*`) veya parolasız hesaplar **asla** kabul edilmez.

Bu yol **yeni kurulumlarda kapalı gelir** — panel girişi sunucunun root parolasına
bağlı olmasın, güvenlik günlüğünde kimin ne yaptığı ayırt edilebilsin diye. Mevcut
kurulumlarda ise güncelleme sonrası **açık kalır**, hiçbir davranış değişmez;
kapatma kararı sizindir.

Açıp kapatma yeri: **Araçlar ve Ayarlar → "Sunucu root parolasıyla panel girişi".**

- Kapatırken panel, root dışında **aktif bir yönetici hesabı** olmasını şart koşar —
  tek adımda kendinizi dışarı kilitleyemezsiniz.
- Kapatma, o ana kadar açılmış root **oturumlarını ve API token'larını da iptal eder.**

> 🔵 **SSH root erişiminiz bu ayardan HİÇ etkilenmez.** Bayrak yalnız `:8443` panel
> girişini ilgilendirir; sunucunun root parolası, `sshd` yapılandırması ve SSH ile
> root girişi olduğu gibi kalır. Yani sunucuya erişiminiz olduğu sürece kurtarma
> yolunuz kapanmaz (bkz. "Kurtarma").

### Kurtarma — panele giremiyorsanız

Her iki yol da SSH'tan çözülür; sunucuya root olarak bağlanın:

```bash
# DSN'i ortam dosyasından al (parola içerir, ekrana basmayın)
DSN=$(grep -m1 '^PANEL_DB_DSN=' /etc/sanalcp/env | cut -d= -f2-)

# 1) Yönetici parolasını sıfırla (hesap yoksa oluşturur)
/opt/sanalcp/bin/sanalcp-seed-admin -dsn "$DSN" -kullanici admin -parola 'YENI_PAROLA'

# 2) 2FA cihazınızı kaybettiyseniz — seed-admin 2FA'ya DOKUNMAZ, ayrıca temizlenmeli
mysql panel -e "UPDATE users SET totp_enabled=0, totp_secret='' WHERE username='admin';"

# 3) Break-glass: panelin root girişini geri aç (panele hiç giremiyorsanız)
mysql panel -e "UPDATE panel_ayarlari SET root_girisi_acik=1 WHERE id=1;"
```

> ⚠️ `sanalcp-seed-admin` **yalnız parolayı** sıfırlar. 2FA açıksa giriş yine 2FA
> adımında takılır; kaybolmuş bir TOTP için 2. komut da gereklidir.

Roller:

| Rol | Kapsam |
|---|---|
| **Yönetici (admin)** | Sunucunun tamamı. Birden fazla olabilir; son aktif yönetici rolünden düşürülemez. |
| **Bayi (reseller)** | Yalnızca kendi müşterileri ve onların domainleri. Kendi kotaları vardır, yalnız müşteri hesabı açabilir. |
| **Müşteri (user)** | Tek domain. Yönetim paneline giriş yapamaz; `/cp` üzerinden kendi domain paneline girer. |

### Giriş güvenliği

- **2FA (TOTP)** — tüm panel hesapları için; QR kod ile kurulur, kod tekrar kullanımına
  (replay) karşı korumalıdır. 2FA durumu okunamazsa giriş **reddedilir** (fail-closed).
- **Kaba kuvvet koruması** — IP başına kayan pencere + kademeli gecikme + kilit.
  İstemci IP'si ters proxy başlıklarıyla spoof edilemez.
- **Denetim kaydı (audit log)** — başarılı/başarısız girişler, hesap işlemleri ve
  yetki değişiklikleri kaydedilir.
- **Boşta oturum zaman aşımı** — `Araçlar ve Ayarlar → Sunucu Bakımı` (varsayılan kapalı).
- Kullanıcı adı sızdırmamak için başarısız girişte her iki dal da aynı yanıtı döner.

## Ne kurar?

| Bileşen | Detay |
|---|---|
| **Web** | nginx (panel :8443 + müşteri siteleri :80/:443) |
| **PHP** | 7.4 / 8.2 / 8.3 / 8.4 / 8.5 (remi) — her domain bağımsız sürüm seçer, per-domain FPM havuzu |
| **Veritabanı** | MariaDB 10.11 (`panel` DB) + phpMyAdmin (`/pma/`) |
| **Cache** | Valkey (Redis) — per-tenant izole object cache (WordPress'e otomatik bağlanır) |
| **E-posta** | Postfix + Dovecot + OpenDKIM — SMTP AUTH (587), IMAP, otomatik DKIM/SPF/DMARC; webmail (Roundcube, `/webmail/`) |
| **Güvenlik** | nftables güvenlik duvarı, SELinux uyumlu, ClamAV |
| **Performans** | MariaDB + nginx + OPcache otomatik tuning (`sanalcp-optimize`) |

## Panel özellikleri

- Domain / subdomain yönetimi, DNS düzenleme, toplu işlemler
- Tek-tık **WordPress**, **Joomla**, **PrestaShop**, **Drupal**, **OpenCart** ve **Matomo** kurulumu; sürüm algılama ve desteklenen uygulamalarda panelden çekirdek güncelleme
- Per-tenant **Redis object cache** (tek tıkla aç/kapa, WP'ye otomatik bağlama)
- **E-posta barındırma**: domain başına posta kutusu, SMTP AUTH ile kimlik doğrulamalı gönderim (PHPMailer/uygulama entegrasyonu için), DKIM/SPF/DMARC otomatik DNS kaydı, webmail arayüzü — ayrıntı için aşağıya bakın
- **Özel vhost modu** (admin): şablonun tek-root sınırını aşan yönlendirme ihtiyaçları için domain başına tam nginx vhost düzenleme — ayrıntı için aşağıya bakın
- **Güvenlik duvarı** arayüzü (IP ban / whitelist / port kapatma + hazır şablonlar)
- **Otomatik saldırı engelleme** (fail2ban muadili) — aşağıya bakın
- **Yönetim API'si** — otomasyon için kişisel erişim token'ları ([docs/API.md](docs/API.md))
- Backup yöneticisi, izleme/loglar, istatistikler
- Hizmet planları ve kaynak limitleri (domain oluştururken varsayılan **Başlangıç**)

## Otomatik Saldırı Engelleme

**Araçlar ve Ayarlar → Güvenlik Duvarı → Otomatik Saldırı Engelleme.**
**Varsayılan olarak KAPALIDIR** — açmadığınız sürece hiçbir davranış değişmez.

Panel, sistem servislerinin kimlik doğrulama hatalarını journald üzerinden izler.
Bir IP belirlediğiniz pencere içinde eşiği aşarsa, nftables tarafında belirlediğiniz
süre boyunca engellenir. Ayrı bir servis (fail2ban, Python) kurulmaz — izleyici panelin
kendi binary'sinin içindedir.

| İzlenen | Yakalanan |
|---|---|
| **sshd** | hatalı parola, geçersiz kullanıcı, azami deneme aşımı, preauth kopmaları |
| **dovecot** (IMAP/POP3) | başarısız giriş denemeleri |
| **postfix** | SASL (SMTP AUTH) kimlik doğrulama hataları |
| **pure-ftpd** | başarısız FTP girişleri |

Varsayılan: **10 dakika içinde 5 başarısız deneme → 60 dakika ban.** Üçü de ayarlanabilir.

### Kilitlenmeye karşı korumalar

Yanlış bir kural sizi kendi sunucunuzdan atabilir; bu yüzden şu güvenceler vardır:

- **İzin listesindeki (whitelist) IP ve CIDR'ler asla engellenmez.** Kendi sabit IP'nizi
  izin listesine eklemeniz önerilir.
- **Sunucunun kendi adresleri, loopback ve link-local asla engellenmez.**
- **Banlar süreldir** ve süresi dolan kural anında etkisiz kalır; panel kapalı olsa bile
  bir ban sonsuza kadar sürmez.
- **Açık oturumlar kopmaz**: nftables kural setinde `ct state established,related accept`
  en üstte olduğu için ban yalnızca **yeni** bağlantıları etkiler. Kendi IP'niz engellense
  bile o an açık olan SSH oturumunuz düşmez.
- Otomatik banlar güvenlik duvarı ekranında **"Otomatik"** rozetiyle ve bitiş saatiyle
  listelenir; tek tek ya da **"Otomatik banları temizle"** ile topluca kaldırılabilir.

> Sayaçlar bellekte tutulur: panel yeniden başlarsa sıfırlanır. Bu bilinçli bir tercihtir —
> hata durumunda fazladan ban yerine eksik ban yönünde davranır.

## E-posta (Mail Hosting)

Her domain için panelden posta kutusu açabilirsiniz — Postfix + Dovecot + OpenDKIM üzerine kurulu, tamamen kendi altyapınızda barınan bir e-posta sistemi (üçüncü taraf bir SMTP servisine bağımlı değil).

- **Domain sayfası → E-posta** sekmesinden önce domain için maili etkinleştirin (MX/SPF/DKIM/DMARC kayıtları DNS'e otomatik eklenir), sonra kutu oluşturun.
- **SMTP AUTH (587, STARTTLS)** — PHPMailer, Nodemailer gibi uygulama kütüphanelerinin doğrudan bağlanabileceği kimlik doğrulamalı gönderim uç noktası. Açık relay değildir; yalnızca kendi kutunuzun kimlik bilgileriyle gönderim yapılabilir.
- **DKIM imzası otomatiktir** — kutu oluşturduğunuz an giden postalar imzalanır, ayrıca bir şey yapmanıza gerek yoktur.
- **Webmail**: `https://SUNUCU_IP:8443/webmail/` üzerinden Roundcube ile kutunuza tarayıcıdan erişebilirsiniz (kutu e-postanız + parolanızla giriş yapın).
- Kötüye kullanıma karşı hız sınırlama (bağlantı/mesaj başına) ve SASL kaba-kuvvet koruması dahildir.
- Not: gelen posta (port 25) bazı barındırma sağlayıcıları tarafından varsayılan olarak ağ seviyesinde kapatılır (spam önleme amaçlı) — sunucunuzda gelen posta çalışmıyorsa sağlayıcınızdan port 25'i açmasını isteyin. Giden SMTP AUTH (587) bundan etkilenmez.

## Özel Vhost Modu

Panelin standart ayarları (güvenlik başlıkları, cache, "ek direktifler" alanı) çoğu site için yeterlidir. Ama bazen tek bir domain'in kökünde bir uygulama, bir alt dizininde (ör. `/blog`) başka bir uygulama çalıştırmak gibi, tek-`root` şablonunun ifade edemeyeceği bir yapı gerekir.

**Özel Vhost Modu** (Domain → Barındırma ve DNS → Apache ve nginx → "Özel Vhost Modu", yalnızca admin) bu durumlar için tam nginx vhost dosyasını görüntüleyip düzenlemenizi sağlar:

- Açtığınızda, panelin o an gerçekten sunduğu **çalışan dosyadan** başlarsınız (boş bir kutudan değil).
- Kaydettiğinizde `nginx -t` ile doğrulanır — geçersiz bir yapılandırma asla canlıya uygulanmaz, hem veritabanı hem çalışan dosya güvenli kalır.
- **Siz açtıktan sonra panel bu dosyaya bir daha dokunmaz** — SSL yenileme, PHP sürüm değişimi gibi otomatik işlemler bu domain için artık şablonu değil sizin kaydettiğiniz içeriği kullanır. Bu yüzden Let's Encrypt doğrulama bloğunu (`/.well-known/acme-challenge/`) dosyada tutmazsanız sertifika yenilemesi 90 gün sonra başarısız olur.
- Domain askıya alınırsa özel vhost modunda olsa bile her zaman "askıya alındı" sayfası gösterilir — bu güvenlik davranışı bypass edilemez.
- Kapatırsanız içerik silinmez; tekrar açarsanız kaldığınız yerden devam edersiniz.

## Sistem gereksinimleri

- **AlmaLinux 10** veya **AlmaLinux 9.4+** (aynı sürümdeki RHEL / Rocky de çalışır)
- veya **Debian 13 / 12**, **Ubuntu 26.04 LTS / 24.04 LTS** — dördü de canlı test edildi
- En az **2 GB RAM**, 2 vCPU (5 PHP sürümü + MariaDB + Valkey için)
- Root erişimi + internet bağlantısı
- Disk kotası hem **XFS** hem **ext4** kök dosya sisteminde çalışır. ext4'te kota,
  çekirdek komut satırına `rootflags=usrquota` eklenmesini ve **tek seferlik bir
  yeniden başlatmayı** gerektirir; kurulum bunu hazırlar ve ekranda bildirir.

## Kurulum sonrası yardımcı araçlar

Kurulumla birlikte `/usr/local/bin`'e şu araçlar gelir:

```bash
sanalcp-update        # paneli GitHub'dan güvenli güncelle (aşağıya bak)
sanalcp-optimize      # MariaDB/nginx/PHP'yi sunucu kaynaklarına göre yeniden ayarla
sanalcp-redis-setup   # Valkey (Redis) altyapısını kur/onar
sanalcp-wp-redis <sk> # bir domainin WordPress'ine Redis cache bağla/çöz
sanalcp-repair        # izin / SELinux / sahiplik onarımı (idempotent)
sanalcp-db-backup     # panel DB'sinin sıkıştırılmış dump'ını al (aşağıya bak)
```

## Yedekleme

### Panel veritabanı (`panel`)

Kurulumla birlikte **günlük otomatik yedek** gelir — ayrı bir şey yapmanız gerekmez:

| | |
|---|---|
| **Ne zaman** | Her gün **03:30** (`sanalcp-db-backup.timer`, ±5 dk rastgele gecikme) |
| **Nereye** | `/var/backups/sanalcp/db/panel-<TARİH>.sql.gz` (dizin `0700`, dump `0600`) |
| **Saklama** | **14 gün** — daha eskiler otomatik silinir |
| **Kapsam** | `panel` şeması + routine/trigger/event'ler (`mysqldump --single-transaction` → kilitsiz tutarlı anlık görüntü) |

Elle yedek almak için (üretilen dosyanın yolunu ekrana basar):

```bash
sanalcp-db-backup
# /var/backups/sanalcp/db/panel-2026-07-17-143052.sql.gz
```

Timer'ın durumunu görmek / bir sonraki çalışmayı öğrenmek için:

```bash
systemctl list-timers sanalcp-db-backup.timer
systemctl status sanalcp-db-backup.timer
journalctl -u sanalcp-db-backup -n 20    # son yedeklerin logu
```

Bir yedeği geri yüklemek için:

```bash
systemctl stop sanalcp
zcat /var/backups/sanalcp/db/panel-2026-07-17-143052.sql.gz | mysql
systemctl start sanalcp
```

> Yedek **fail-closed**'dır: gzip bütünlüğü doğrulanmadan veya dosya şüpheli derecede küçükse dump `panel-*.sql.gz` adını **almaz** — yarım bir dump asla geçerli yedek gibi görünmez.

### Güncelleme öncesi otomatik yedek

`sanalcp-update`, **migration'ları uygulamadan önce** panel DB'sinin tam dump'ını alır. Dump alınamazsa **güncelleme hiç başlamaz** (yedeksiz migration reddedilir). Ayrıntı için aşağıdaki "Güncelleme" bölümüne bakın.

### Müşteri siteleri

Müşteri siteleri + veritabanları ayrı bir işle yedeklenir: `sanalcp-backup-all` (cron, her gün 03:00 UTC, `/var/backups/sanalcp/<sistem_kullanıcı>/`, 14 gün saklama). Panel DB yedeği bu dizinlere **dokunmaz**.

## Güncelleme (SSH / CLI)

Kurulu bir panelde, SSH ile root olarak tek komut:

```bash
sanalcp-update            # son sürümü GitHub'dan çek → binary+frontend+migration değiştir → yeniden başlat
sanalcp-update --dry-run  # önce ne yapacağını göster (dokunmadan)
sanalcp-update --force    # binary aynı olsa bile yeniden uygula
sanalcp-update --branch X # farklı dal
```

- **Güvenli & veri-korumalı:** `/etc/sanalcp/env` (JWT/DB/Redis secret), MariaDB `panel` veritabanı ve `/home/c_*` müşteri siteleri **asla silinmez**. `install.sh`'in aksine yeni secret üretmez.
- Yeni migration'lar servis yeniden başlarken **otomatik + idempotent** uygulanır.
- Binary değişmemişse (sha eşleşir) hiçbir şey yapmaz.
- **Migration'lardan önce panel DB'sinin tam dump'ı alınır** → `/var/backups/sanalcp/db/`.
- **Fail-closed:** dump alınamazsa güncelleme **hiç başlamaz** — binary'ye, frontend'e ve migration'lara dokunulmaz. Yedeksiz migration kabul edilmez.
- Yeni sürüm sağlıklı başlamazsa **otomatik olarak eski binary'ye _ve_ güncelleme öncesi DB'ye geri döner** (rollback). Panel o sırada zaten durmuş olduğu için yazma kaybı olmaz.

> Kendi fork'unu deploy ediyorsan: kaynağı derle (`GOAMD64=v1 go build` + `npm run build`), `assets/sanalcp-server` + `assets/frontend-dist.tar.gz`'i güncelle, repona push et — sunucularda `sanalcp-update` yeni sürümü çeker. **Binary'yi mutlaka `GOAMD64=v1` ile derle** (bkz. "Backend (Go)" altındaki uyarı) — aksi halde eski CPU'lu müşteri sunucularında panel açılmaz.

## Notlar

- Kurulum **idempotent** değildir; her çalıştırma yeni secret (JWT/DB parola) üretir. Yeniden çalıştırma yerine `sanalcp-repair` / `sanalcp-optimize` kullanın.
- Panel HTTP/2 + self-signed SSL ile :8443'te yayınlanır; gerçek alan adı için Let's Encrypt panel üzerinden eklenebilir.

---

## Kaynaktan derleme ve geliştirme

Bu proje **tamamen açık kaynaktır** (MIT). İstersen hazır binary'yi kurmak yerine kaynağı kendin derleyip geliştirebilirsin — katkılar açıktır.

### Gereksinimler

- **Go 1.26+** (backend — go.mod dil sürümü 1.25, yayın binary'si 1.26 hattıyla derlenir; 1.24 ve 1.25 hatları 1.27.0 çıkınca destekten düştü)
- **Node.js 20+** ve **npm** (frontend)
- Çalıştırma için: MariaDB/MySQL erişimi (backend başlarken migration + admin seed uygular)

### Backend (Go)

> ⚠️ **Yayınlanacak binary `GOAMD64=v1` ile derlenmelidir.** AlmaLinux 10 (go1.26+) varsayılan olarak `GOAMD64=v3` üretir; v3 ile derlenen binary eski/yaygın müşteri CPU'larında
> `"This program can only be run on AMD64 processors with v3 microarchitecture support"` verip **çalışmaz**. `assets/sanalcp-server` daima `GOAMD64=v1` ile derlenmelidir
> (kolaylık için `scripts/build-assets.sh` kullan — bunu zaten sabitler).

```bash
# tek statik binary derle (eski CPU uyumu için GOAMD64=v1 ZORUNLU)
CGO_ENABLED=0 GOAMD64=v1 go build -o sanalcp-server ./cmd/server

# çalıştır (ortam değişkenleriyle)
PANEL_JWT_SECRET="$(openssl rand -hex 32)" \
PANEL_DB_DSN="root@unix(/var/lib/mysql/mysql.sock)/panel" \
./sanalcp-server
```

Backend API `/api/v1` altında; sağlık kontrolü `/healthz`. Panel girişi `users` tablosundaki gerçek hesaplarla yapılır; `root`/`/etc/shadow` yolu bayrakla kapatılabilen bir break-glass yoludur (bkz. "Kimlik doğrulama ve hesap modeli"). Geliştirmede admin hesabını `scripts/seed_admin.go` ile tohumlayabilirsin:

```bash
go run scripts/seed_admin.go -dsn '<DSN>' -kullanici admin -parola 'SECELECEGIN_PAROLA'
# ya da: PANEL_SEED_PAROLA env değişkeni
```

### Frontend (React + Vite + TypeScript)

```bash
cd frontend
npm install
npm run dev        # geliştirme sunucusu :5185 (proxy /api → VITE_API_PROXY)
npm run build      # üretim derlemesi → frontend/dist/
```

Dev sunucusunun backend'i nereye proxy'leyeceğini `VITE_API_PROXY` ile ayarla (varsayılan `http://localhost:8080`):

```bash
VITE_API_PROXY=http://localhost:8080 npm run dev
```

### Depo yapısı

```
cmd/server/       Go giriş noktası (main)
internal/         Backend paketleri (domains, wordpress, dns, redis, guvenlikduvari, github, backups, ...)
frontend/src/     React arayüzü (pages/, components/, lib/)
migrations/       SQL şema migration'ları (başlangıçta uygulanır)
scripts/          Ops yardımcıları (optimize, repair, redis-setup, seed_admin, ...)
assets/           Kurulum için hazır (prebuilt) release çıktıları — installer bunları kullanır
install.sh        Tek satır bootstrap (repoyu indirir → sanalcp-install.sh)
```

> `assets/` içindeki hazır binary + `frontend-dist.tar.gz`, `curl | bash` kurulumunun kaynağı derlemeden çalışması içindir. Kendi değişikliklerini yayınlarken bunları yukarıdaki `go build` / `npm run build` çıktısıyla güncelle.

## Katkı & lisans

- Katkılar (issue / PR) açıktır.
- Lisans: **MIT** — bkz. [LICENSE](LICENSE). Kullanabilir, değiştirebilir, dağıtabilir ve kendi ürününde kullanabilirsin.


## Güncelleme

Paneli son sürüme güncellemek için sunucuda:

```bash
sanalcp-update              # son sürümü kur
sanalcp-update --dry-run    # sadece ne yapacağını göster
sanalcp-update --force      # aynı sürüm olsa bile yeniden uygula
```

Panel içinden de güncelleyebilirsiniz: **Araçlar ve Ayarlar → Panel Güncellemesi → "Güncellemeleri denetle ve kur"**.

Güncelleme **korur** (asla dokunmaz): `/etc/sanalcp/env` (JWT/DB/Redis secret), MariaDB `panel` veritabanı + tüm müşteri verisi, `/home/c_*` siteleri.

Güncelleme, **migration'ları uygulamadan önce** panel DB'sinin tam dump'ını `/var/backups/sanalcp/db/` altına alır. Dump alınamazsa güncelleme **hiç başlamaz** (yedeksiz migration reddedilir). Yeni sürüm sağlıklı başlamazsa otomatik olarak **eski binary'ye + güncelleme öncesi DB'ye geri döner**.

### "sanalcp-update: command not found" alıyorsanız

Panelinizi, güncelleme aracı dağıtıma eklenmeden **önce** kurmuşsanız bu komut sunucunuzda bulunmaz. Aracı almanın tek yolu yine kendisi olduğu için kısır döngüye girersiniz. Tek seferlik şu komutla kurun:

```bash
curl -fsSL https://raw.githubusercontent.com/sanalcp/sanalcp/main/assets/ops/sanalcp-update \
  -o /usr/local/bin/sanalcp-update && chmod +x /usr/local/bin/sanalcp-update

sanalcp-update
```

Bunu **bir kez** yapmanız yeterlidir: `sanalcp-update` her çalıştığında `assets/ops/` altındaki tüm araçları `/usr/local/bin`'e yeniden kurar, dolayısıyla kendini de güncel tutar. Bundan sonra panel içindeki **Panel Güncellemesi** butonunu da kullanabilirsiniz.

> Panel içi güncelleme butonu, aracı eksikse **otomatik indirir** — yani butona basmanız da yeterlidir; yukarıdaki komut yalnızca panele hiç erişemediğiniz durumlar için.
