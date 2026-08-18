# Debian / Ubuntu desteği — teknik plan

**Durum:** Faz 0-4b tamamlandı (2026-08-19). Sıradaki: Faz 5 — canlı test.
**Hedef:** birincil Debian 13 (trixie) + Ubuntu 26.04 LTS (resolute);
ikincil Ubuntu 24.04 (noble) + Debian 12 (bookworm).
**Tarih:** 2026-08-18

---

## 1. Hedef sürümler ve gerekçe

### Birincil

| Dağıtım | Durum | MariaDB |
|---|---|---|
| **Debian 13 "trixie"** | Ağustos 2025'ten beri stable, şu an 13.6 | 11.8 |
| **Ubuntu 26.04 LTS "resolute"** | 23 Nisan 2026, destek Nisan 2031 | doğrulanacak |

### İkincil

| Dağıtım | Durum | MariaDB | Neden dahil |
|---|---|---|---|
| **Ubuntu 24.04 LTS "noble"** | destek Nisan 2029 | 10.11 | 26.04 henüz 4 aylık; birçok VPS sağlayıcısı varsayılan imaj olarak hâlâ 24.04 veriyor |
| **Debian 12 "bookworm"** | tam destek 11 Tem 2026'da bitti, **LTS ile 30 Haz 2028'e kadar** güvenlik desteği | **10.11** | Kurulu tabanı hâlâ çok geniş; ayrıca aşağıdaki teşhis avantajı |

> **Debian 12'nin gizli faydası — değişken izolasyonu.** Debian 12 ve Ubuntu 24.04,
> panelin bugün AlmaLinux'ta kullandığı **MariaDB 10.11**'i getiriyor. Debian 13 ise
> 11.8'e atlıyor. Yani port sırasında bir şey bozulduğunda "dağıtım farkı mı, MariaDB
> sürüm farkı mı?" sorusunu **aynı ailede iki sürümü karşılaştırarak** yanıtlayabiliriz.
> Bu, MariaDB 11.8 doğrulamasının (§7.1) riskini gözle görülür biçimde düşürür.

> **LTS uyarısı.** Debian LTS, normal stable ile birebir aynı kapsamda değildir: bazı
> paketler LTS kapsamı dışında kalabilir ve yalnız belirli mimariler desteklenir.
> Debian 12 bu yüzden **ikincil** hedeftir — kurulum çalışır ve desteklenir, ama yeni
> kurulumlar için Debian 13 önerilir. Kurulum betiği Debian 12'de bunu bir kez uyarır.

Çok sürümlü PHP tamamen **`deb.sury.org`**'a bağlı. Depo indeksi doğrulandı — mevcut
dağıtımlar: `bullseye bookworm trixie | jammy noble resolute`. **Dört hedefimiz de var**
(`bookworm`, `trixie`, `noble`, `resolute`).

---

## 2. İyi haber: sanılandan çok daha küçük bir iş

Kod tabanı denetlendi. Üç büyük engel sanılan şey aslında engel değil:

### 2.1 SELinux — sıfır kod değişikliği

23 Go dosyası `restorecon` çağırıyor, ama **istisnasız hepsi hatayı yok sayıyor**:

```go
_ = exec.Command("restorecon", conf).Run()
_, _ = exec.Command("restorecon", "-R", dir).CombinedOutput()
```

`semanage` / `setsebool` çağrıları ise zaten `exec.LookPath` ile korumalı ve yoksa
sessizce atlıyor. Debian'da bu ikililer yok → çağrılar no-op olur, akış devam eder.

**Yapılacak:** hiçbir şey. (İsteğe bağlı: AppArmor profili — v1 için gerekmez.)

### 2.2 PHP düzeni — veri değişikliği, kod değil

`internal/provisioner/provisioner.go` içindeki `phpMap` zaten tablo tabanlı:

```go
"8.3": {PoolDir: "...", SockDir: "...", Service: "...", FpmBin: "..."}
```

Debian için ikinci bir tablo yeterli (sury yolları doğrulandı):

| | RHEL / remi | Debian / sury |
|---|---|---|
| PoolDir | `/etc/opt/remi/php83/php-fpm.d` | `/etc/php/8.3/fpm/pool.d` |
| SockDir | `/var/opt/remi/php83/run/php-fpm` | `/run/php` |
| Service | `php83-php-fpm` | `php8.3-fpm` |
| FpmBin | `/opt/remi/php83/root/usr/sbin/php-fpm` | `/usr/sbin/php-fpm8.3` |
| Eklenti ini | `/etc/opt/remi/php83/php.d` | `/etc/php/8.3/fpm/conf.d` |

`internal/php/php.go` içindeki `PHPSurumler` tablosu da aynı şekilde ikizlenecek.

### 2.3 Kaynak izolasyonu — systemd ve MariaDB tarafı aynen çalışır

En güçlü özelliğimizin üç ayağından **ikisi hiç etkilenmiyor**:

- **systemd slice** (`CPUQuota`, `MemoryMax`, `TasksMax`, IO throttle) — systemd her iki
  ailede de aynı. Değişiklik yok.
- **MariaDB Governor** (`MAX_USER_CONNECTIONS`, `MAX_QUERIES_PER_HOUR`) — standart SQL,
  değişiklik yok.
- **Disk kotası** — tek etkilenen ayak, bkz. §4.

---

## 3. Gerçek iş kalemleri

### 3.1 OS aile soyutlaması (yeni paket: `internal/osfam`)

Açılışta bir kez tespit, sonra her yerden okunur:

```go
type Aile int // AileRHEL | AileDebian

func Tespit() Aile          // /etc/os-release ID/ID_LIKE
func PaketKur(...) error    // dnf install -y  |  apt-get install -y
func PaketVarMi(ad string) bool
func Servis(mantiksal string) string // "dns" -> "named" | "bind9"
func NginxKullanici() string          // "nginx" | "www-data"
func PHPTablo() map[string]phpAyar
```

**Kural:** `exec.Command("dnf", ...)` çağrısı kodun içinde kalmayacak; hepsi buradan geçecek.

### 3.2 Paket yöneticisi çağrıları (6 Go dosyası)

| Dosya | Ne yapıyor |
|---|---|
| `internal/phpsurum/phpsurum.go` | PHP sürümü kur/kaldır/listele (`dnf module` dahil) |
| `internal/phpext/phpext.go` | PHP eklentisi kur |
| `internal/paketler/paketler.go` | Paket yöneticisi ekranı |
| `internal/provisioner/provisioner.go` | Eksik paket tamamlama |
| `internal/system/optimize.go` | `dnf -y` çağrıları |
| `internal/system/cve.go` | Güvenlik güncellemesi denetimi (`dnf updateinfo` → Debian'da karşılığı farklı) |

`cve.go` en zoru: RHEL'in `dnf updateinfo --security` çıktısının Debian'da doğrudan
karşılığı yok. Debian tarafında `apt list --upgradable` + `debian-security-support` ya da
`unattended-upgrades` çıktısı kullanılacak; **v1'de bu ekran Debian'da "desteklenmiyor"
diye kapatılabilir** (dürüst ve düşük riskli).

### 3.3 Paket ve servis adı haritası

| Mantıksal | RHEL | Debian/Ubuntu | Not |
|---|---|---|---|
| web | `nginx` | `nginx` | aynı |
| web kullanıcısı | `nginx` | **`www-data`** | FPM pool `listen.owner/group` |
| apache backend | `httpd`, `httpd-tools` | `apache2`, `apache2-utils` | conf düzeni de farklı, §3.4 |
| db | `mariadb-server` | `mariadb-server` | sürüm farkı: 10.11 → **11.8** |
| dns | `bind`, `bind-utils` / servis `named` | `bind9`, `bind9-utils` / servis `bind9` | `named-checkconf/checkzone` ikisinde de aynı ad |
| ftp | `pure-ftpd` | `pure-ftpd-mysql` | Debian'da MySQL desteği ayrı pakette |
| antivirüs | `clamav`, `clamav-update` / `clamd` | `clamav-daemon`, `clamav-freshclam` | servis adları farklı |
| cache | `valkey` | `valkey-server` (doğrulanacak) | Debian 13'te valkey var mı, yoksa `redis-server`'a mı düşülecek |
| cron | `cronie` | `cron` | |
| kota araçları | `xfsprogs` + `quota` | `xfsprogs` + `quota` | Backend fs tipine göre seçilir (aileye göre DEĞİL) → her iki araç seti her iki ailede kurulur; `osfam.PaketKotaXFS` / `PaketKotaExt` |
| certbot | `certbot`, `python3-certbot-nginx` | aynı | |
| diğer | `policycoreutils-python-utils`, `setools-console` | **yok** (SELinux) | Debian'da hiç kurulmaz |

### 3.4 Apache backend (~42 satır)

Panel opsiyonel Apache backend'i destekliyor (`Backend == "apache"`, 127.0.0.1:10080 proxy).
Debian'da conf düzeni tamamen farklı (`/etc/apache2/sites-available` + `a2ensite`, modül
adları farklı).

**Öneri: v1'de Debian'da Apache backend'i kapatalım.** Kullanım oranı düşük, iş yükü
yüksek. UI'da "bu sunucuda kullanılamıyor" denir. Sonraki sürüme bırakılır.

---

## 4. Disk kotası — Faz 3 ✅ (2026-08-18)

Kaynak izolasyonunun disk ayağı XFS'e (`xfs_quota`) bağlıydı; Debian/Ubuntu bulut
imajlarında kök dosya sistemi neredeyse her zaman **ext4**. Backend arayüzü yazıldı.

### Yapılan

`internal/kaynaklimit/kaynaklimit.go` içindeki kota bloğu üç dosyaya ayrıldı:

| Dosya | İçerik |
|---|---|
| `kota.go` | Ortak katman: `kotaBackend` arayüzü, fs tespiti, sk allowlist, sentinel, plan çözümlemesi, `HealKotaOnStartup` |
| `kota_xfs.go` | `xfsKota` — mevcut `xfs_quota` kodu olduğu gibi taşındı (regresyon riski sıfır) |
| `kota_ext4.go` | `ext4Kota` — `setquota` / `repquota` / `quotaon` (`quota` paketi) |

```go
type kotaBackend interface {
    Ad() string            // "xfs" | "ext4" — log/teşhis
    KernelBayragi() string // "rootflags=uquota" | "rootflags=usrquota"
    Aktif() (accounting, enforcement bool)
    Uygula(ctx context.Context, sk string, diskMB, inode int) error
    Durum(sk string) (kullanilanMB, limitMB, kullanilanInode, limitInode int)
}
```

Backend seçimi **aileye değil, kök fs'in `statfs(2)` magic'ine** bakar
(`unix.XFS_SUPER_MAGIC` / `unix.EXT4_SUPER_MAGIC`; ext2/3/4 aynı magic'i paylaşır).
Böylece AlmaLinux-üstü-ext4 ve Debian-üstü-XFS de doğru çalışır. Tanınmayan fs
(btrfs, zfs, overlay) → backend **yok**, kota dürüstçe "desteklenmiyor" denir;
panelin geri kalanı çalışır. `KotaFSUyumlu()` artık "XFS mi" değil, "backend var mı"
sorusunu yanıtlıyor; UI metni de buna göre güncellendi.

Dışa açık API (`KotaUygula`, `KotaDurum`, `KotaFSUyumlu`, `KotaRebootGerekli`,
`DomainKotaUygula`, `HealKotaOnStartup`) **değişmedi** — çağıran paketlere dokunulmadı.

### Etkinleştirme akışı — mevcut UX birebir korundu

Kota her iki fs'te de yalnız mount anında açılabildiği için panel GRUB'a kernel
bayrağı yazıp tek seferlik reboot istiyor ve bunu `kotaRebootSentinel` ile UI'da
gösteriyor. Sentinel metni artık bayrağı **backend'den** alıyor:

| | RHEL / XFS | Debian / ext4 |
|---|---|---|
| kernel flag | `rootflags=uquota` | `rootflags=usrquota` |
| grub üret | `grub2-mkconfig -o /boot/grub2/grub.cfg` | `grub-mkconfig -o /boot/grub/grub.cfg` |
| BLS güncelle | `grubby --update-kernel=ALL` | yok (gerekmez) |
| reboot sonrası | otomatik aktif | `quotacheck -cum /` + `quotaon /` gerekebilir (Faz 5) |

### Canlı doğrulanan komut sözleşmeleri

Bu makine Debian 13 trixie / ext4 kök; **quota-tools 4.09** ile doğrulandı.
Ayrıştırıcılar bu gözlemlere göre yazıldı ve birim testli:

- **`quotaon -p -u /`** — kota KAPALIYKEN tanı mesajını **stderr**'e yazar ve yine
  **rc=0** döner. → Çıkış koduna güvenilemez. Kota açıkken **stdout**'a tek satır
  gelir: `user quota on / (/dev/vda1) is on`. Ayrıştırıcı yalnız bu satıra bakar.
- **`repquota -u -O csv /`** — kota kapalıyken **rc=1** (quotaon'un aksine güvenilir).
  Başlık (ikilinin format string'inden birebir):
  `User,BlockStatus,FileStatus,BlockUsed,BlockSoftLimit,BlockHardLimit,BlockGrace,FileUsed,FileSoftLimit,FileHardLimit,FileGrace`.
  Sütunlar **sabit indeksle değil, başlık adıyla** bulunur (blok sütunları sürüme
  göre `Block*` yerine `Space*` adlanabilir; ikili her iki dizgeyi de taşıyor).
  Değerler ham kalsın diye `-s`/human-readable **asla** verilmez; blok değerleri KiB.
- **`setquota -u <ad> <bsoft> <bhard> <isoft> <ihard> <fs>`** — blok limitlerinde
  `M` soneki kabul edilir (varsayılan birim KiB blok), `0M` dahil; inode limitleri
  çıplak sayı (`k/m/g` sonekleri 10³ katları demek olduğu için kullanılmaz).
  **0 = sınırsız** — XFS ile aynı semantik. Arg biçimi gerçek ikili ile doğrulandı.
- **accounting / enforcement ayrımı**: ext4'te `quotaon` yalnız *enforcement*'ı
  söyler. quotaon kapalı ama kök mount `usrquota` / `quota` / `usrjquota=` ile
  bağlıysa kullanım sayılıyor ama limitler uygulanmıyor demektir — XFS'teki
  `uqnoenforce` durumunun karşılığı. `/proc/self/mounts` okunarak ayırt edilir.

### Testler

`kota_ext4_test.go`: arg-slice, CSV ayrıştırıcı (karışık sütun sırası + `Space*`
adlandırması dahil), quotaon/mount durum ayrıştırıcıları, ve sahte backend ile
guard testleri (backend yok / enforcement kapalı / geçersiz sk → **asla hata,
asla limit yazma**). Ayrıca **XFS-ext4 parite testi**: iki backend aynı disk/inode
girdisi için aynı soft/hard sayılarını üretmeli — aksi halde dağıtım değişince
tenant'ın efektif limiti sessizce kayardı.

`KOTA_LIVE=1 go test` bu makinede (ext4, kota kapalı) çalıştırıldı: backend
otomatik `ext4` seçildi, doğru kernel bayrağı loglandı, `KotaUygula` hata dönmeden
graceful-skip yaptı.

### Faz 5'e kalan canlı sorular

- `tune2fs -O quota` bağlıyken çalışıyor mu, yoksa klasik `quotacheck` yolu mu gerekli?
- Reboot sonrası `quotaon` kendiliğinden gelir mi (`quotaon.service`), yoksa
  `quotacheck -cum /` şart mı?
- Bulut imajlarında `/` gerçekten ext4 mi (bazıları btrfs olabilir → kota desteklenmez,
  dürüstçe "kota kapalı" denir; panelin geri kalanı çalışır).

## 5. Installer — Faz 4 ✅ (2026-08-18)

Tespit edilen RHEL bağımlılığı (başlangıç): 14 `dnf` çağrısı, 12 SELinux çağrısı,
8 firewalld, 7 remi/epel.

**Uygulanan yaklaşım (planlandığı gibi):** dosya ikiye BÖLÜNMEDİ. Üstte bir aile
tespiti + ince sarmalayıcılar var, gövde ortak kaldı. (Faz 4b'de bu blok
`assets/ops/sanalcp-ortak.sh`'a taşındı; installer artık onu source ediyor —
ops betikleriyle tek tablo paylaşılıyor, bkz. §5b.)

| Sarmalayıcı | İşi |
|---|---|
| `pkg_kur` / `pkg_kurulu` / `depo_yenile` | `dnf` ↔ `apt-get` |
| `paket_ad` / `servis_ad` | mantıksal ad → gerçek paket / systemd unit |
| `php_pkg <sürüm> <eklenti>` | `php83-php-fpm` ↔ `php8.3-fpm` |
| `havuz_kur` | panel-içi FPM havuzunu doğru dizine + doğru kullanıcıyla kurar |
| `debian_mi` / `rhel_mi` | okunabilirlik |

Artık betikte `dnf`/`rpm` çağrısı YALNIZCA `rhel_mi` kolunda ya da sarmalayıcının
içinde geçiyor.

### Debian tarafında eklenen adımlar

- `apt-get update` + `DEBIAN_FRONTEND=noninteractive` **ve** `NEEDRESTART_MODE=a`
  — needrestart'ın servis seçici ekranı gözetimsiz kurulumu sonsuza kadar
  bekletirdi.
- **sury deposu:** anahtar `/usr/share/keyrings/deb.sury.org-php.gpg` +
  `signed-by`. 🔴 `apt-key` KULLANILMADI: oraya eklenen anahtar sistemdeki HER
  depo için güvenilir olur; `signed-by` güveni tek depoya hapseder.
- `firewalld` yoksa da iş bitmiyor: Ubuntu imajlarında aynı tuzağın adı **ufw**.
  İkisi de kapatılıyor (panel kendi nftables'ını yönetir).
- Debian'ın **etkin varsayılan nginx sitesi** siliniyor — `default_server`
  talebi `_default80.conf` ile çakışır ve `nginx -t` "duplicate default server"
  ile patlar, hiçbir şey ayağa kalkmaz.
- `unattended-upgrades` + `apt-daily*.timer` kapatılıyor (dnf-automatic'in
  karşılığı; kurulum ortasında dpkg kilidi için yarışırdı).

### Canlı doğrulanan depo sözleşmeleri

Bu makinede (Debian 13 trixie) ağ üzerinden doğrulandı:

- `https://packages.sury.org/php/apt.gpg` → HTTP 200, 3182 bayt, **ikili** OpenPGP
  anahtarı (`DEB.SURY.ORG Automatic Signing Key <deb@sury.org>`, RSA 3072).
  `signed-by` ikili keyring'i doğrudan kabul eder.
- Dört hedefin de dist'i var: `bookworm` · `trixie` · `noble` · `resolute` →
  `Release` ve `main/binary-amd64/Packages.gz` hepsi HTTP 200.
- trixie'de sury'nin sunduğu FPM sürümleri: **5.6 → 8.6**. Bizim kurduğumuz
  aralık 7.4–8.5 (Remi listesiyle parite).
- 8.3 için ihtiyaç duyulan 15 paketin tamamı mevcut (`fpm cli mysql mbstring
  bcmath intl gd soap opcache xml zip pgsql ldap curl redis`).

### Planda olmayan, kurulumu sessizce bozan üç boşluk

Bunlar Faz 4 sırasında ortaya çıktı ve **Go tarafında** düzeltildi — installer tek
başına çözemezdi:

1. **BIND yolları (`internal/dns/yollar.go` — yeni).** `zone_writer.go`
   `/var/named`, `/etc/named/…`, `chown named:named` ve `systemctl reload named`
   değerlerini sabit tutuyordu. Debian'da servis `bind9`, kullanıcı `bind` ve
   🔴 **bind9 bir AppArmor profiliyle gelir**: named `/var/named`'e YAZAMAZ.
   Zone dizini bu yüzden `/var/lib/bind` (profilde yazılabilir tanımlı), include
   `/etc/bind/sanalcp-zones.conf`. Yanlış bırakılsa DNS hatasız ama tamamen
   işlevsiz olurdu.
2. **Sistem PHP'si (`provisioner.SistemPHPHavuzDizin/Servis`).** phpMyAdmin ve
   webmail onarımları `/etc/php-fpm.d/…` ve `systemctl reload php-fpm` diyordu;
   Debian'da havuz yanlış dizine yazılır, reload var olmayan bir birime giderdi.
3. **UI servis listeleri.** `system/servis.go` (yeniden başlat düğmeleri) ve
   `monitor/sunucu_log.go` (journal kaynakları) RHEL unit adlarıyla sabitti.
   Debian'da her satır "absent" görünür, log ekranı boş dönerdi. Ayrıca
   `sshd.service` Debian'da yalnız bir ALIAS'tır — journald kayıtları gerçek
   unit adıyla (`ssh.service`) tutulduğu için `journalctl -u sshd` BOŞ döner.

### Kota bölümü artık iki backend'i de kuruyor

Faz 3'ün ayrımına uygun olarak seçim **aileye değil kök fs'e** bakıyor:

| kök fs | kernel bayrağı | grub üretimi |
|---|---|---|
| xfs | `rootflags=uquota` | `grub2-mkconfig` + `grubby --update-kernel=ALL` (BLS) |
| ext2/3/4 | `rootflags=usrquota` | `update-grub` / `grub-mkconfig` |
| diğer (btrfs…) | — | kota desteklenmiyor, dürüstçe söyleniyor |

ext tarafında reboot sonrası muhasebe dosyası gerektiği için bir kerelik
`sanalcp-quotacheck.service` (oneshot, `ConditionPathExists=!/aquota.user`,
`Before=quotaon.service`) kuruluyor. §4'teki "Faz 5'e kalan canlı sorular" hâlâ
geçerli; bu servis klasik `quotacheck` yolunu kapsıyor.

### Parite testi (installer ↔ osfam)

`internal/osfam/installer_paritesi_test.go`: betik `SANALCP_TANIM_TESTI=1` ile
source edilip (sisteme dokunmadan) **sahte bir `/etc/os-release`** verilir; ürettiği
her paket/servis/kullanıcı adı `osfam`'ın aynı girdi için ürettiğiyle karşılaştırılır.
Altı dağıtım kapsanıyor: AlmaLinux 9/10, Debian 12/13, Ubuntu 24.04/26.04.

**Neden gerekli:** installer adları kurulum anında kendi tablosundan, panel ise
çalışma anında `osfam`'dan çözüyor. İki tablo ayrışırsa installer bir şeyi kurar,
panel başka bir adı arar — ve bu yalnız gerçek bir müşteri sunucusunda görülür.
Test, tablolardan biri değiştirilerek doğrulandı (mutasyon: `bind9` → `bind9x`;
test beklendiği gibi kırmızıya döndü).

## 5b. Ops betikleri — Faz 4b ✅ (2026-08-19)

Kurulumun son adımları ops betiklerini çağırır; bunlar RHEL-özgü kaldığı sürece
Debian'da kurulum "tamam" der ama posta, FTP ve cache kurulmamış olurdu.

### Tek shell tablosu: `assets/ops/sanalcp-ortak.sh`

Aile tespiti ve tüm paket/servis/yol çözümlemesi **tek dosyaya** taşındı. Hem
installer (`. "$A/ops/sanalcp-ortak.sh"`) hem her ops betiği
(`. /usr/local/bin/sanalcp-ortak`) onu source ediyor. Böylece shell tarafında
tek bir tablo var ve `internal/osfam` ile paritesi test ediliyor.

Kütüphane ayrıca şunları taşıyor: `php_kurulu_surumler` (kurulu PHP sürümlerini
aileye uygun **token** biçiminde döner), `selinux_var` (Debian'da daima false),
`WEB_USER`, `DNS_*`, `SYS_PHP_POOL_DIR` / `SYS_PHP_SVC` / `MYSQL_SOCK`.

### Betik betik yapılanlar

| Betik | Debian tarafında değişen |
|---|---|
| `sanalcp-redis-setup` | paket/birim `valkey-server`\|`redis-server`; php eklentisi `php8.3-redis` (Remi'de `php-pecl-redis6`); SELinux bloğu atlanıyor |
| `sanalcp-mail-setup` | dovecot **tek paket değil**; rspamd deposu gerekmiyor; opendkim SOCKET tuzağı; `php8.3-intl`; FPM reload birimi |
| `sanalcp-ftp-setup` | 🔴 tamamen ayrı yapılandırma düzeni (aşağıda) |
| `sanalcp-repair` | phpMyAdmin havuz yolu + havuz grubu; SELinux zaten `SEL=0` ile no-op |
| `sanalcp-waf-setup` | `-devel` → `-dev` derleme bağımlılıkları; nginx modül dizini `/usr/lib/nginx/modules` |
| `sanalcp-optimize` | PHP ini drop-in dizini sürüm başına (`/etc/php/*/fpm/conf.d`); FPM birim adları |
| `sanalcp-update` | paket kurulumları `pkg_kur`'a taşındı; kütüphaneyi **indirilen release'ten** source ediyor |

### Yol boyunca çıkan dört tuzak

1. 🔴 **Pure-FTPd düzeni.** Debian'da tek `pure-ftpd.conf` YOKTUR:
   `/etc/pure-ftpd/conf/` altında **direktif başına bir dosya** (ad = direktif,
   içerik = değer) ve `pure-ftpd-wrapper` bunları komut satırına çevirir. Dahası
   MySQL doğrulaması conf dosyası yazmakla AÇILMAZ — `/etc/pure-ftpd/auth/`
   altındaki sıralı sembolik link gerekir (`30mysql`). `CertFile` direktifi de
   tanınmaz: sertifika sabit `/etc/ssl/private/pure-ftpd.pem` yolundan okunur.
   Betik artık iki düzeni de yazıyor ve Debian'da unix/pam auth linklerini
   kaldırıyor (yoksa sistem kullanıcıları da FTP'ye girebilirdi).
2. 🔴 **OpenDKIM sessiz devre dışı kalması.** Debian'da `/etc/default/opendkim`
   içindeki `SOCKET=` satırı systemd birimine `DAEMON_OPTS` olarak geçer ve
   `opendkim.conf`'taki `Socket` satırını EZER. Postfix `smtpd_milters` inet'i
   gösterdiği için DKIM imzalama hiç çalışmazdı; satır artık yorumlanıyor.
3. **Dovecot Debian'da tek paket değil.** `dovecot-core` + `imapd` + `lmtpd` +
   `mysql` + `sieve` + `managesieved` ayrı ayrı gerekiyor; `pigeonhole` adı
   Debian'da yok. Eksik `dovecot-imapd` = IMAP hiç yok.
4. **Rspamd Debian'da dağıtımın kendi deposunda** (trixie: 3.12.1) — üçüncü
   parti depo eklemeye gerek yok, yalnız RHEL'de ekleniyor. Doğrulandı:
   `apt-cache policy rspamd` → trixie/main.

### Test

`TestOpsBetikleriOrtakKutuphaneyiSourceEder`: paket yöneticisine dokunan her ops
betiği ortak kütüphaneyi source etmek ZORUNDA. Kendi `dnf install` satırını
taşıyan bir betik, Debian portunun tam olarak gözden kaçtığı yerdi.

> ⚠️ **Açık kalan bakım tuzağı: `scripts/` ↔ `assets/ops/` ikilemesi.** Aynı
> betiklerin iki kopyası var ve **yalnız `assets/ops/` sunuculara kuruluyor**.
> Beşi zaten ayrışmış durumda (`mail-setup`, `optimize`, `redis-setup`,
> `update`, `wp-redis` — `assets/` kopyaları iki dilli ve daha yeni). 0.7.0
> kesintisinde rollback düzeltmesinin yanlış dosyaya yazılmasının sebebi tam
> olarak buydu. Faz 4b yeni ayrışma EKLEMEDİ (aynı olan üç dosya elle
> eşitlendi), ama kalıcı çözüm `scripts/` kopyalarının silinmesi ya da
> `package-release.sh`'ın senkronu zorunlu kılmasıdır. Karar bekliyor.

---

## 6. Sıralama

| Faz | İş | Risk |
|---|---|---|
| ~~**0**~~ | ✅ `internal/osfam` + tespit + birim testler | düşük |
| ~~**1**~~ | ✅ phpMap/KurulSurumler ikinci tablo, web kullanıcısı, servis adı haritası | düşük |
| ~~**2**~~ | ✅ 6 dosyadaki paket yöneticisi çağrıları soyutlamaya taşındı | düşük |
| ~~**3**~~ | ✅ Kota backend arayüzü + ext4 uygulaması | **yüksek** |
| ~~**4**~~ | ✅ Installer soyutlaması + sury deposu (+ planda olmayan 3 Go boşluğu) | orta |
| ~~**4b**~~ | ✅ Ops betikleri + tek shell tablosu (`sanalcp-ortak.sh`) | orta |
| **5a** | Canlı test: **Debian 12** (MariaDB 10.11 — bilinen DB, yalnız dağıtım farkı test edilir) | — |
| **5b** | Canlı test: **Debian 13** (MariaDB 11.8 devreye girer) | — |
| **5c** | Canlı test: **Ubuntu 26.04**, ardından 24.04 | — |
| **6** | Apache backend + CVE ekranı: Debian'da kapat, dürüstçe belirt | düşük |

**Faz 0-4b tamamlandı (2026-08-19).** Go tarafında artık doğrudan `dnf`/`yum`/`rpm`
çağrısı YOKTUR; hepsi `internal/osfam` üzerinden geçer. Kalan tek istisna
`internal/system/cve.go` içindeki `cveRun`'dır ve o da yalnız RHEL'de erişilebilir
(`GuvenlikGuncellemeDestekli` kapısı).

Faz 2'de ayrıca yapılanlar:
- PHP sürüm yönetimi (`phpsurum`) üçüncü kaynağı tanıyor: `remi` · `appstream` · **`sury`**.
  sury'de paket adları `php<sürüm>-<eklenti>`; eklenti demeti Remi'nin birebir çevirisi
  değil (`mysqlnd`→`mysql`, `pdo` ve `json` ayrı paket değil).
- PHP eklenti kurulumu (`phpext`) sury paket adlarını ve `conf.d` ini dizinini tanıyor.
- Hata özetleyici apt çıktısını da tanıyor (`E: Unable to locate package…`).
- `sanalcp-optimize` sarmalayıcısı apt dalını içeriyor.

Faz 3'te disk kotası XFS/ext4 backend'lerine ayrıldı; ayrıntısı §4'te.
Faz 4'te installer soyutlandı ve sury deposu eklendi; ayrıntısı §5'te.

**Sıradaki iş Faz 4b: ops betikleri.** Installer bittiğinde kurulum Debian'da
sonuna kadar akıyor, ama son adımlarda çağırdığı ops betikleri hâlâ RHEL-özgü.
Denetim sonucu (RHEL-özgü çağrı sayısı / satır):

| Betik | RHEL-özgü | Zorluk |
|---|---|---|
| `sanalcp-repair` | 16 / 275 | orta — çoğu SELinux, Debian'da atlanacak |
| `sanalcp-mail-setup.sh` | 13 / 277 | orta — postfix/dovecot yolları büyük ölçüde ortak |
| `sanalcp-update` | 9 / 402 | düşük |
| `sanalcp-ftp-setup` | 6 / 120 | 🔴 **yüksek** — ad değişimi YETMEZ |
| `sanalcp-redis-setup.sh` | 4 / 73 | düşük — paket/birim/conf dizini adı |
| `sanalcp-waf-setup` | 3 / 261 | düşük |

> 🔴 **Pure-FTPd tuzağı.** Debian'da `pure-ftpd-mysql` yapılandırması RHEL'deki
> tek `pure-ftpd.conf` dosyasıyla DEĞİL, `/etc/pure-ftpd/conf/` altında **direktif
> başına bir dosya** düzeniyle çalışır (`pure-ftpd-wrapper` bunları komut satırı
> argümanına çevirir) ve MySQL yapılandırması `/etc/pure-ftpd/db/mysql.conf`
> yolundadır. Bu, isim haritasıyla çözülecek bir fark değil; ayrı bir kol gerekir.
> FTP olmadan panelin geri kalanı çalışır, bu yüzden Faz 4b'nin son maddesi olabilir.

Faz 5 sırası bilinçlidir: **5a'da yalnız "dağıtım ailesi" değişkeni**, 5b'de üzerine
"MariaDB sürümü" değişkeni biner. Tersi sırada bir hata çıksa hangisinden geldiği
belirsiz kalırdı.

Faz 5 olmadan **hiçbir şey "destekleniyor" diye ilan edilmeyecek.**

---

## 7. Kararlar

**Hepsi karara bağlandı (2026-08-18).** Aşağıdaki maddeler geçmiş kaydı olarak duruyor.

### 7.0 Verilen kararlar — özet

| # | Karar | Sonuç |
|---|---|---|
| 1 | MariaDB 11.8 doğrulaması | **Faz 5b'de.** Önce Debian 12 (10.11) ile soyutlama doğrulanır |
| 2 | Valkey mi Redis mi | **Çözüldü, aşağıya bakın** — sürüm bazlı |
| 3 | Apache backend Debian'da | **v1'de kapalı.** UI "bu sunucuda kullanılamıyor" der |
| 4 | CVE ekranı Debian'da | **v1'de kapalı.** Yarım çalışan ekran yerine dürüst kapatma |
| 5 | Ubuntu 24.04 + Debian 12 | **İkincil hedef olarak dahil** |

### 7.0.1 Valkey/Redis — doğrulandı, tasarımı etkiledi

Paket arşivleri denetlendi:

| Dağıtım | `valkey-server` |
|---|---|
| Debian 13 (trixie) | ✅ 8.1.1 |
| Ubuntu 24.04 / 25.10 / 26.04 | ✅ var |
| **Debian 12 (bookworm)** | ❌ **yok** → `redis-server`'a düşülecek |

> **Tasarım sonucu:** paket adı çözümlemesi **yalnız aileye bakarak yapılamaz.**
> Valkey örneği gösteriyor ki bazı paketler dağıtım *sürümüne* bağlı. `osfam` bu yüzden
> yalnız `Aile` değil, `ID` + `Surum` + `KodAdi` de tutacak ve çözümleyici bu üçünü
> görebilecek. Bu, ileride çıkacak benzer durumlar için de tek doğru yer olacak.

---

## 7.1 Kararların ayrıntısı

1. **MariaDB 10.11 → 11.8.** 67 migration'ın 11.8'de sorunsuz çalıştığı doğrulanmalı.
   Kullanılan özellikler (`CREATE INDEX IF NOT EXISTS`, `ALTER USER ... MAX_QUERIES_PER_HOUR`)
   standart ve uzun süredir mevcut; yine de canlı doğrulama şart.
   **Sıralama önerisi:** önce Debian 12 (MariaDB 10.11 — bilinen değer) üzerinde soyutlamayı
   doğrula, sonra Debian 13'e (11.8) geç. Böylece dağıtım farkı ile DB sürüm farkı aynı anda
   devreye girmez; bir sorun çıktığında hangisinden geldiği tek testle ayrılır.
2. **Valkey mi Redis mi?** Debian 13'te `valkey-server` var mı, yoksa `redis-server`'a mı
   düşülecek — paket adı ve servis adı buna göre.
3. **Apache backend** Debian'da v1'de kapatılsın mı? (öneri: evet)
4. **CVE/güvenlik güncellemesi ekranı** Debian'da v1'de kapatılsın mı? (öneri: evet)
5. ~~**Ubuntu 24.04** ilk turda mı?~~ **Karar verildi:** Ubuntu 24.04 ve Debian 12 ikincil
   hedef olarak dahil. Soyutlama bittikten sonra ikisi de neredeyse bedava; Debian 12
   ayrıca §7.1'deki MariaDB doğrulamasında teşhis aracı olarak kullanılacak.

---

## 8. Doğrulama kaynakları

- Debian sürüm durumu: <https://www.debian.org/releases/>
- Debian 12 güvenlik desteğinin LTS ekibine devri: <https://www.debian.org/News/2026/20260712>
- Ubuntu 26.04 duyurusu: <https://canonical.com/blog/canonical-releases-ubuntu-26-04-lts-resolute-raccoon>
- sury dist listesi: <https://packages.sury.org/php/dists/>
