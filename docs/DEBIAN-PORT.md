# Debian / Ubuntu desteği — teknik plan

**Durum:** Faz 0-5b tamamlandı (2026-08-19). Debian 13 (trixie) canlı testten
43 kontrol / 0 hata ile geçti; testte bulunan 5 hata düzeltildi (§5f) — biri
(kotanın ikinci reboot'ta kaybolması) Debian 12'yi de etkiliyordu.
Sıradaki: Faz 5c — Ubuntu 26.04.
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

> ✅ **Çözüldü (2026-08-19): `scripts/` ↔ `assets/ops/` ikilemesi kaldırıldı.**
> Aynı betiklerin iki kopyası vardı ve sunuculara **yalnız `assets/ops/`**
> kuruluyordu; 0.7.0 kesintisinde rollback düzeltmesinin hiçbir sunucuya
> inmemesinin sebebi buydu. `scripts/` altındaki 12 kopya silindi (11 ops betiği
> + `50-sanal-jail.conf`); tek kaynak artık `assets/`. Silmeden önce ayrışmış
> beş dosya karşılaştırıldı: `scripts/` tarafındaki her fark, `assets/`
> kopyasının daha yeni/iki dilli sürümünde zaten karşılanıyordu — kaybolan
> içerik yok. `TestScriptsAltindaAssetKopyasiYok` kopyaların geri gelmesini
> engelliyor. `scripts/` altında artık yalnız gerçekten oraya ait olanlar var:
> `build-assets.sh`, `package-release.sh`, `seed_admin.go`, `yetki-testi.sh`.

## 5c. Faz 5a statik ön-uçuş ✅ (2026-08-19)

Canlı sunucu olmadan yapılabilecek her doğrulama önce yapıldı: **paket adları,
dosya düzenleri ve sürümler gerçek Debian indekslerinden ve gerçek `.deb`
içeriklerinden** okundu. Amaç, bir VPS'i yakmadan "paket yok / yol yanlış"
sınıfı hataları elemekti.

### Paket adı doğrulaması — 61 paket, iki dağıtım

Installer + ops betiklerinin Debian'da kuracağı TÜM paket adları
`deb.debian.org` `Packages.gz` indekslerine karşı tarandı:

| | bookworm (12) | trixie (13) |
|---|---|---|
| doğrulanan paket | 61 | 61 |
| **bulunamayan** | **1** (`valkey-server`) | 0 |

Tek eksik, `osfam`'ın zaten bildiği ve `redis-server`'a düştüğü durum
(§7.0.1) — yani soyutlama gerçek indeks karşısında doğrulandı.

### Gerçek `.deb` içeriğinden doğrulanan düzenler

`pure-ftpd-common` + `pure-ftpd-mysql` paketleri indirilip açıldı; §5b'deki
varsayımların hepsi **paket içeriğiyle** doğrulandı:

- `/etc/pure-ftpd/conf/` gerçekten direktif-başına-dosya (`NoAnonymous`,
  `TLSCipherSuite`, `PAMAuthentication`… hepsi ayrı dosya olarak geliyor).
- `/etc/pure-ftpd/db/mysql.conf` `pure-ftpd-mysql` ile geliyor.
- `/etc/pure-ftpd/auth/30mysql` **paketin kendisi tarafından** kuruluyor
  (bizim `ln -sf`'imiz gereksiz ama idempotent/zararsız).
- `/etc/pure-ftpd/auth/65unix` ve `70pam` `pure-ftpd-common` ile geliyor →
  bizim silme adımımız GEREKLİ (yoksa sistem kullanıcıları FTP'ye girebilirdi).
- `pure-ftpd-wrapper` kaynağı okundu: `/etc/pure-ftpd/conf` dizinini tarıyor ve
  `auth/` altında **yalnız sembolik linkleri** dikkate alıyor (`grep {-l ...}`).
  Debian'ın da içerdiği `/etc/pure-ftpd/pure-ftpd.conf` dosyası wrapper
  tarafından OKUNMUYOR — betiğin Debian kolunda ona yazmaması doğru.
- systemd unit dosyası YOK: `/etc/init.d/pure-ftpd-mysql` (SysV) → systemd
  jeneratörü `pure-ftpd-mysql.service` üretiyor; `osfam`'ın servis adı doğru.

### 🔴 Düzeltme: OpenDKIM socket tuzağı §5b'de yanlış anlatılmıştı

`opendkim` paketi açılıp birim dosyası okundu. Gerçek durum:

- Stok birim yalnızca `ExecStart=/usr/sbin/opendkim` — argümansız, yani
  `/etc/opendkim.conf` geçerli. `/etc/default/opendkim` dosyasının kendi notu:
  *"This is a legacy configuration file. It is not used by the opendkim systemd
  service."*
- Socket'i EZEN şey `/etc/systemd/system/opendkim.service.d/override.conf`
  drop-in'idir; onu `/lib/opendkim/opendkim.service.generate` üretir ve
  `ExecStart=… -p $SOCKET` yazar. Üretici kaynağını `/etc/default`'tan alır.

Betik buna göre düzeltildi: artık **önce drop-in'i** arıyor ve `-p` içeriyorsa
devre dışı bırakıyor (`.sanal-devredisi` olarak yeniden adlandırıp
`daemon-reload`), ayrıca üretici sonradan çalıştırılırsa aynı tuzağı kurmasın
diye `/etc/default/opendkim`'deki `SOCKET=` satırını yorumluyor.

### Sürüm matrisi (gerçek indekslerden)

| Bileşen | AlmaLinux | Debian 12 | Debian 13 |
|---|---|---|---|
| MariaDB | 10.11 | **10.11.18** | **11.8.6** |
| Dovecot | 2.3.x | **2.3.19** | **2.4.1** |
| nginx | 1.20/1.26 | 1.22.1 | 1.26.3 |
| rspamd | (rspamd.com deposu) | 3.4 | 3.12.1 |
| valkey | 8.x | — (redis 7.0) | 8.1.1 |

Faz 5a/5b ayrımının gerekçesi böylece sayıyla doğrulandı: **Debian 12, MariaDB
tarafında AlmaLinux ile aynı** (10.11), Debian 13'te 11.8 devreye giriyor.

### 🔴 Yeni bulgu — Dovecot 2.4, Faz 5b'yi bloke ediyor

Plan yalnız MariaDB sürümünün değişeceğini öngörüyordu. Debian 13'te **Dovecot
2.4** var ve 2.4 yapılandırma sözdizimini KIRDI. `dovecot-core` 2.4.1 paketi
açılıp örnek konfigleri okundu:

- `mail_location` KALKTI → yerine `mail_driver` + `mail_path`
  (paketin `10-mail.conf`'u: `mail_driver = mbox`, `mail_path = %{home}/mail`).
- `dovecot.conf` artık `dovecot_config_version = 2.4.0` bildirimi taşıyor.
- `passdb { driver = sql; args = … }` biçimi 2.4'te adlandırılmış bloklara
  taşındı; `auth-sql.conf.ext` örneği boşaltılmış.

Bizim `assets/mail/dovecot/10-sanalcp-mail.conf.tmpl` şablonumuz **2.3
sözdizimi** (`mail_location`, `passdb { driver = sql }`, `$mail_plugins`).
Yani Debian 13 ve Ubuntu 26.04'te dovecot açılışta yapılandırma hatası verir →
sanal posta kutuları hiç çalışmaz.

**İyi haber:** `!include conf.d/*.conf` 2.4'te de duruyor, yani drop-in
yaklaşımı geçerli — yalnız şablonun 2.4 varyantı ve `doveconf --version`'a göre
seçim gerekiyor. Bu, Faz 5b'nin iş kalemi olarak eklendi.

**Debian 12 (2.3.19) ETKİLENMİYOR** — Faz 5a bu bulgudan bağımsız ilerleyebilir.

### Canlı sunucu olmadan doğrulanamayanlar

Bunlar Faz 5a'nın asıl işi: kurulumun uçtan uca akması, `nginx -t`, migration'
ların MariaDB 10.11'de uygulanması, panelin ayağa kalkması, BIND'in
`/var/lib/bind`'e yazabilmesi (AppArmor), kota için GRUB + tek seferlik reboot,
`quotacheck` oneshot'ı, per-tenant PHP-FPM izolasyonu ve sanal FTP girişi.

## 5d. Faz 5a — canlı Debian 12 testi ✅ (2026-08-19)

**Ortam:** Debian 12 bookworm · kök fs **ext4** · MariaDB 10.11.18 · 2 vCPU / 3.8 GB.
Tertemiz sunucuya `sanalcp-install.sh` ile sıfırdan kurulum.

Kök fs'in ext4 olması şans eseri iyi denk geldi: Faz 3'ün **ext4 kota backend'i**
de böylece gerçek donanımda sınandı.

### Sonuç

Kabul testi (`scripts/debian-kabul-testi.sh`): **39 kontrol, 0 hata.**
Installer üç kez üst üste çalıştırıldı — **idempotent**.

Uçtan uca kanıtlananlar:

| Ne | Kanıt |
|---|---|
| Kurulum | `EXIT=0`, hiç `✗` yok |
| Panel | `/healthz` 200, 69 migration MariaDB 10.11'de uygulandı |
| Giriş | root + PAM ile API token alındı |
| Site oluşturma | `kotatest.example` → HTTP **200** |
| PHP | `PHPOK-8.3.33-c_kotatest_example` — kiracının kendi kullanıcısı |
| İzolasyon | FPM worker'ı `c_kotatest_example` (per-tenant, CageFS muadili) |
| **Disk kotası** | limitler gerçekten yazıldı: blok **995328/1048576 KiB**, inode **23750/25000** |
| DNS | zone `/var/lib/bind`'e `bind:bind` yazıldı, `dig` gerçek SOA döndürdü (AppArmor aşıldı) |
| Posta | postfix check temiz, OpenDKIM **8891** dinliyor, rspamd 11332, `doveconf -n` temiz |
| FTP | `auth/30mysql` sembolik link, unix/pam kolları kalkmış, port 21 dinliyor |

### Bulunan yedi hata

Statik ön-uçuş paket adlarını ve dosya düzenlerini doğrulamıştı; canlı testin
bulduğu hataların **hiçbiri o sınıftan değildi**. Hepsi "servis ayakta ama
sessizce işlevsiz" türü:

1. **MariaDB soketi sabitti** (`internal/hesaplar`) → panel Debian'da HİÇ
   açılmadı (crash-loop). `osfam.MariaDBSoket()` eklendi: önce var olan adaya
   bakar, yoksa aile varsayılanı.
2. 🔴 **`systemctl enable --now` Debian'da yetmiyor.** apt paketi kurarken
   servisi BAŞLATIR (dnf başlatmaz); biz config'i sonraki adımlarda yazdığımız
   için servis çoktan çalışıyor ve `enable --now` no-op oluyor. Kanıt: bind9
   23:54:30'da başlamış, `named.conf` 23:56:27'de yazılmış; php8.3-fpm
   23:56:02'de başlamış, havuz 23:56:22'de yazılmış. Sonuç: phpMyAdmin FPM
   soketi hiç oluşmadı, named include'u hiç okunmadı. → ortak kütüphaneye
   `svc_hazirla` (enable + **restart**).
3. **`sanalcp-optimize` iki yerden bozuktu:** MariaDB tuning'i Debian'da
   olmayan `/etc/my.cnf.d`'ye yazıyordu (dosya okunmadı ama "yazıldı" dendi;
   düzeltince buffer_pool 128M→768M, `skip_name_resolve` OFF→ON) ve nginx perf
   bloğundaki `gzip on;` Debian'ın stok `nginx.conf`'uyla çakışıp `nginx -t`'yi
   patlatarak **tüm** tuning'i geri aldırıyordu → artık `nginx.conf`'ta zaten
   tanımlı direktifler ad bazlı eleniyor.
4. **PHP sürüm döngüsü ya-hep-ya-hiç'ti:** sury/bookworm'da
   `php8.5-{intl,opcache,pgsql}` yok ve toplu kurulum 8.5'in diğer 12 paketini
   de düşürüyordu. Artık tek tek deneniyor, eksikler adıyla raporlanıyor.
   Ayrıca `update-alternatives` "en yeni"yi seçtiği için `php` CLI 8.4'e
   kayıyordu (wp-cli/composer onu kullanır) → 8.3'e sabitlendi.
5. **Kiracı FPM havuzu `listen.owner = nginx` sabitti** → Debian'da havuz
   "cannot get uid for user 'nginx'" ile reddediliyor, **hiçbir site
   oluşturulamıyordu**.
6. **ACL `u:nginx:rX` sabitti** ve `setfacl` hatası yutuluyordu → ACL hiç
   kurulmuyor, web kullanıcısı `public_html`'i okuyamıyor, **her site 404**.
   Artık başarısızlık loglanıyor.
7. **ext4 kota durumu yanlış okunuyordu:** `quotaon -p` kota AÇIKKEN de rc=1
   dönüyor (quota-tools 4.06); kod `err != nil` görüp stdout'u okumadan
   dönüyordu → panel çalışan kotayı "kapalı, reboot gerekli" sanıyordu.
   Faz 3'te "çıkış koduna güvenilemez" notu vardı ama yalnız KAPALI durum
   doğrulanmıştı. Karar artık saf `ext4AktifCoz`'da ve yalnız stdout'a bakıyor.
   **Aynı tuzak kabul testi betiğinde de vardı** (`pipefail` + `quotaon | grep`).

> **Ders:** yedi hatanın altısı, "aynı işi yapan ikinci bir yer" olduğu için
> vardı — soyutlama bir çağrı yerini atlamıştı. Statik testler bunları
> yakalayamaz; yalnız gerçek bir kurulum yakalar. Faz 5b/5c'de aynı beklenti
> geçerli.

### Kota etkinleştirme akışı — canlı doğrulandı

Reboot öncesi/sonrası ölçüldü. GRUB'a `rootflags=usrquota` yazıldı, tek seferlik
reboot sonrası:

```
mount:    rw,relatime,quota,usrquota,errors=remount-ro
quotaon:  user quota on / (/dev/sda1) is on      (rc=1 — çıkış kodu yanıltıcı!)
repquota: User,BlockStatus,FileStatus,BlockUsed,BlockSoftLimit,...
```

`sanalcp-quotacheck.service` oneshot'ı `/aquota.user`'ı üretti ve `quotaon`'u
açtı. §4'teki "Faz 5'e kalan canlı sorular" böylece yanıtlandı: **klasik
`quotacheck` yolu gerekli ve yeterli**, `tune2fs -O quota` denenmedi.

## 5e. Faz 5b hazırlığı — iki risk de sunucusuz kapatıldı ✅ (2026-08-19)

Geliştirme makinesi zaten Debian 13 trixie olduğu için Faz 5b'nin iki bilinen
riski canlı sunucu beklemeden **gerçek ikililerle** sınandı.

### Risk 1 — MariaDB 11.8: KAPANDI

Tek kullanımlık bir **MariaDB 11.8.6** örneği (Debian 13'ün birebir sürümü)
ayrı datadir + soketle kaldırıldı ve migration smoke testi ona karşı koşuldu:

- `TestMigrationlarSifirDBdeUygulanir` → **69 migration temiz uygulandı**
- `TestMigrationlarIdempotent` → **geçti**

§7.1'deki "67 migration'ın 11.8'de çalıştığı doğrulanmalı" maddesi böylece
yanıtlandı. Canlı testte tekrar doğrulanacak ama artık sürpriz beklenmiyor.

### Risk 2 — Dovecot 2.4: ŞABLON YAZILDI ve GERÇEK AYRIŞTIRICIYLA DOĞRULANDI

`dovecot-core` 2.4.1 + `dovecot-mysql` + `dovecot-sieve` paketleri **kurulmadan**
açıldı ve `doveconf` doğrudan çalıştırıldı; şablon hata hata düzeltilerek
yazıldı. Tespit edilen kırılmalar:

| 2.3 | 2.4 |
|---|---|
| `mail_location = maildir:~/` | `mail_driver = maildir` + `mail_path = %{home}` |
| `passdb { driver = sql; args = <dosya> }` | `passdb sql { query = … }` (adlandırılmış blok) |
| `userdb { driver = sql; args = <dosya> }` | `userdb sql { query = … }` |
| SQL bağlantısı `dovecot-sql.conf.ext` dosyasında | `sql_driver` + `mysql <host> { … }` conf'un İÇİNDE |
| `%u` | `%{user}` |
| `ssl_cert = </yol` | `ssl_server_cert_file = /yol` (`<` öneki kalktı) |
| `plugin { sieve = file:~/sieve;active=… }` | `sieve_script personal { driver = file; path; active_path }` |
| `mail_plugins = $mail_plugins sieve` | `mail_plugins { sieve = yes }` |

Yeni dosya: **`assets/mail/dovecot/10-sanalcp-mail-2.4.conf.tmpl`**.
`doveconf -n` ile doğrulandı — tek kalan hata `dovenull` sistem kullanıcısının
bu makinede olmaması (gerçek kurulumda paket onu oluşturur); ayar hatası YOK.
Ayrıştırılmış çıktı da gözle kontrol edildi (passdb/userdb sorguları, mysql
bloğu, service blokları yerinde).

**Seçim:** `sanalcp-mail-setup` artık `dovecot --version` okuyup şablonu seçiyor
(≥2.4 → yeni, aksi hâlde 2.3). Sürüm okunamazsa 2.3'e düşer — kurulu tabanın
tamamı 2.3. Karşılaştırma sayısal: `2.10` da doğru şekilde 2.4'ten büyük sayılır.
2.4 şablonu DB parolası içerdiği için `root:dovecot 0640` yazılıyor ve 2.3'ten
kalan `dovecot-sql.conf.ext` (artık okunmuyor) yanıltmasın diye
`.2.3-devredisi` olarak yeniden adlandırılıyor.

> ⚠️ `doveconf --version` diye bir seçenek YOK — 2.4 "invalid option" der.
> Sürüm `dovecot --version`'dan okunur. Kabul testi betiği de düzeltildi.

**Değişmeyen:** 2.4 hâlâ `/etc/dovecot/conf.d/10-auth.conf` içinde
`!include auth-system.conf.ext` satırını taşıyor → PAM kapatma kodu
(`HealDovecotAuth` + kurulum betiği) 2.4'te de olduğu gibi çalışır.

**Testler:** `internal/mail/dovecot_sablon_test.go` iki şablonun birbirine
karışmasını engelliyor (2.3'te `mail_driver` olmamalı, 2.4'te `mail_location`
olmamalı, `%u` kalmamalı vb.). Bu, `doveconf` doğrulamasının yerini tutmaz —
yalnız geriye kaymayı yakalar.

### Canlı testte hâlâ görülecekler

Kurulumun uçtan uca akışı, dovecot 2.4'ün gerçekten AÇILMASI (parse ≠ çalışma),
sanal kutuya IMAP girişi, LMTP teslimatı, MariaDB 11.8'de panelin çalışma-anı
sorguları ve Faz 5a'daki 39 kontrolün trixie'de tekrarı.


---

## 5f. Faz 5b — canlı Debian 13 (trixie) testi ✅ (2026-08-19)

Sunucu: Hetzner Debian 13.6, 2 vCPU / 3.8 GB / 38 GB ext4, çekirdek 6.12
(`6.12.101+deb13-cloud-amd64`). Kurulum sıfırdan, `--lang tr`.

### Sonuç

Kurulum **ilk denemede uçtan uca aktı** ve o günkü 40 kabul kontrolünün
tamamı geçti (kontrol sayısı bu fazda eklenenlerle 44'e çıktı).
Faz 5a'nın (Debian 12) aksine servis düzeyinde hiçbir şey kırılmadı: MariaDB
11.8, Dovecot 2.4, PHP 7.4-8.5 (sury), bind9, pure-ftpd, rspamd hepsi ilk
açılışta ayağa kalktı. §5e'deki iki büyük risk gerçekten kapanmıştı.

Buna rağmen **beş hata** çıktı ve hepsi aynı sınıftandı: *servis ayakta,
yapılandırma ayrıştırılıyor, ama iş görmüyor.* Statik kontrol ve `doveconf -n`
beşini de yeşil gösteriyordu.

### Bulunan beş hata

**1. Dovecot 2.4: INBOX mbox yolunda kaldı — teslimat hiç olmuyor**

Şablon `mail_driver = maildir` + `mail_path = %{home}` yazıyor ve Debian'ın stok
`conf.d/10-mail.conf` dosyasındaki `mail_driver = mbox` / `mail_path` satırlarını
eziyor. Ama 2.4'te INBOX'ın yeri **AYRI** bir ayar: `mail_inbox_path`. Stok
dosya onu `/var/mail/%{user}` yapıyor, biz hiç yazmadığımız için ayakta kalıyor.
2.3'te böyle bir ayar yoktu (`mail_location` tek başına her şeyi tarif ediyordu),
o yüzden 2.3 şablonundan çeviri yaparken görünmez kaldı.

Sonuç: dovecot açılır, `doveconf -n` temizdir, IMAP girişi çalışır — ama INBOX
`/var/mail` altında mbox olarak açılmaya çalışıldığı için sanal kutunun oraya
yazma izni yoktur ve teslimat `Failed to autocreate mailbox: Permission denied`
ile düşer.

Düzeltme: 2.4 şablonuna `mail_inbox_path = %{home}`. Bu değer 2.3'teki
`mail_location = maildir:~/` yerleşimini birebir tekrarlar (cur/new/tmp doğrudan
kutu kökünde) — `%{home}` yerine boş bırakmak da çalışıyor ama INBOX'ı
`~/.INBOX/` altına alır ve 2.3 sunucusundan göç ile yedek/geri yükleme
dağıtıma göre farklılaşırdı.

**2. Dovecot 2.4: LMTP alan adını kırpıyor — `550 User doesn't exist`**

Debian'ın stok `conf.d/20-lmtp.conf` dosyası şunu yazar:

```
protocol lmtp {
  auth_username_format = %{user | username | lower}
}
```

`username` filtresi adresin alan adını atar. Tek alan adlı klasik kurulumda
doğru, sanal alan adlarında yıkıcı: userdb sorgusu `email='test'` olarak koşar,
hiçbir satır dönmez, postfix `550 5.1.1 User doesn't exist` ile **bounce** eder.
IMAP girişi çalıştığı için hata ilk bakışta görünmez — kutuya girilir, posta hiç
düşmez.

🔴 Ayarı ana şablona yazmak İŞE YARAMAZ: dovecot `conf.d/*.conf` dosyalarını
alfabetik yükler ve `20-lmtp.conf`, bizim `10-sanalcp-mail.conf`'umuzdan SONRA
gelip değeri geri ezer. Bu yüzden override ayrı bir dosyada, `99-` önekiyle:
`assets/mail/dovecot/99-sanalcp-lmtp-2.4.conf.tmpl`. 2.3'e geri düşülürse
kaldırılıyor (2.3 bu ayarı tanımaz).

**3. `main.cf`'te tekrar eden anahtar — her postfix çağrısında uyarı**

`sanalcp-mail-setup` kendi bloğunu eklemeden önce stok tanımları `postconf -X`
ile siliyordu, ama silinecek anahtar listesi **elle dört tane** yazılmıştı
(RHEL'in stok `main.cf`'ine bakılarak). Debian'ın stok dosyası
`smtpd_relay_restrictions`'ı da tanımlıyor, dolayısıyla her `postfix`,
`postconf`, `sendmail`, `mailq` çağrısı `overriding earlier entry` uyarısı
bastırıyordu. Değer aynı olduğu için güvenlik etkisi yok — ama gerçek uyarılar
bu gürültünün içinde kaybolur.

Düzeltme: liste artık `main.cf.append` şablonunun kendisinden türetiliyor
(31 anahtar). Şablona yeni anahtar eklendiğinde burasının unutulması mümkün
değil. Faz 5a'daki nginx `gzip` tekrarıyla aynı sınıf hata.

**4. Yedi PHP-FPM master'ı birden çalışıyor — ~197 MB boşa**

apt, her PHP sürümünün FPM servisini kurulumda **başlatır ve enable eder**;
kurulum betiği de üstüne her sürüm için `svc_hazirla` çağırıyordu. Taze kurulan
sunucuda 7.4-8.5 arası yedi master ayaktaydı, altısının tek bir havuzu bile
yoktu (kiracılar kendi `php-fpm-<kullanıcı>.service` birimlerini kullanıyor).
Ölçüm: 197 MB, ve her açılışta geri geliyor. RHEL'de dnf servisleri
başlatmadığı için bu tamamen Debian'a özgü bir regresyon.

Düzeltme: bir sürümün master'ı **yalnız o sürümde kiracı havuzu varsa**
(`c_*.conf`) başlatılıyor; yoksa `disable --now`. Kontrol "Debian mi" değil
"havuz var mı" — mevcut sunucularda 8.1'de canlı kiracı varsa servis
durdurulmamalı ve güncelleme sırasında installer'ın yeniden koşması siteleri
düşürmemeli. Panel, paylaşılan havuza geri düşen bir kiracı için o sürümü
talep üzerine geri açıyor (`provisioner.paylasilanFPMHazirla`: enable +
reload-or-restart). `enable`'ın oraya eklenmesi bu düzeltmenin ön koşuluydu —
`reload-or-restart` durmuş servisi başlatır ama disable kalırsa ilk reboot'ta
site 502 verirdi.

**5. 🔴 Disk kotası ikinci reboot'ta sessizce kayboluyor**

En ciddi bulgu. `sanalcp-quotacheck.service` birimi şu koşulu taşıyordu:

```
ConditionPathExists=!/aquota.user
```

Koşul birimin **tamamına** uygulanıyor, oysa birim İKİ ayrı iş yapıyor:

| İş | Ne sıklıkta gerekli |
|---|---|
| `quotacheck -cum /` | bir kez (dosyayı üretir, büyük diskte dakikalar) |
| `quotaon -u /` | **her açılışta** |

İlk reboot'ta `/aquota.user` yok, birim koşar, kota açılır — her şey sağlıklı
görünür. İkinci reboot'ta dosya artık vardır, koşul tutmaz, systemd birimi
atlar ve `quotaon` hiç çağrılmaz. Sunucu, **kota muhasebesi açık ama
enforcement kapalı** halde gelir: `repquota` çalışır, sayılar doğrudur, hiçbir
limit uygulanmaz.

Dağıtımın kendi birimi bizi kurtarmıyor: `systemd-fstab-generator` quotaon
bağımlılığını **`/etc/fstab`'daki kota seçeneklerinden** kurar, kök dosya
sistemi ise `usrquota`'yı çekirdek komut satırından (`rootflags=`) alıyor —
dolayısıyla `quotaon-root.service`'i hiçbir şey çekmiyor. Ayrıca Debian'da
`quotaon.service` diye bir birim yok (paket `quota.service` / `quotaon@.service`
gönderiyor), yani installer'daki `systemctl enable quotaon.service` çağrısı da
sessizce başarısız oluyordu.

Düzeltme: koşul birimden kaldırıldı; `quotacheck` kendi `ExecStart`'ı içinde
dosya yokluğuna göre korunuyor, `quotaon` her açılışta koşuyor.

🔴 **İkinci katman:** ilk düzeltme denendiğinde birim sunucuda DEĞİŞMEDİ. Birimi
yazan blok, "kota henüz kurulu değil" dalının içindeydi; kök fs'te kota zaten
aktifse installer `root fs user quota already active` deyip bloğu tamamen
atlıyordu. Yani düzeltme, kurulu hiçbir sunucuya `sanalcp-update` ile **asla**
ulaşmayacaktı — hata sonsuza kadar yaşardı. Birim artık her koşuda yazılıyor;
mount'ta kota zaten varsa birim hemen başlatılıp enforcement **reboot
gerekmeden** geri getiriliyor (mevcut sunucuların onarım yolu). Kurulum
çıktısında yeni satır: `✓ quota enforcement active (quotaon)`.

**Bu hata Debian 12'yi de aynı şekilde etkiliyor.** Faz 5a'da tek reboot
yaptığım için görünmedi — tek reboot bu hatayı kusursuz sağlıklı gösterir.

### Testlere eklenenler

| Test | Ne yakalar |
|---|---|
| `TestDovecot24SablonuInboxYolunuEziyor` | `mail_inbox_path` şablondan düşerse |
| `TestDovecot24LMTPKullaniciBicimiOverride` | `\| username` filtresi geri gelirse; ayrıca ayarın ana şablona (ezilir) yazılmasını engeller |
| kabul: `mail_inbox_path` **etkin** değeri | şablon doğru ama stok dosya eziyorsa |
| kabul: LMTP `auth_username_format` **etkin** değeri | aynı |
| kabul: `postconf -n` uyarı sayısı | main.cf'te tekrar eden anahtar |
| kabul: `sanalcp-quotacheck` bu açılışta koştu mu | kota biriminin tek seferlik koşula geri dönmesi |

🔴 İlk iki kabul kontrolü **etkin değere** bakıyor, dosya içeriğine değil. Bu
faz bunun neden şart olduğunu iki kez gösterdi: dosyamız doğru yazılmışken stok
`conf.d` dosyaları değeri geri ezdi.

Kota kalıcılık kontrolü de aynı mantıkta: "quotaon şu an açık" ile "sonraki
açılışta da açık olacak" farklı iddialar. Birim `RemainAfterExit=yes` taşıdığı
için, bu açılışta koştuysa `active` görünür; koşul yüzünden atlandıysa
`inactive` kalır ve kontrol düşer.

### Doğrulanan davranışlar

- MariaDB **11.8.6** — 69 migration diskte/DB'de eşit, panel sorunsuz
- Dovecot **2.4.1** — 2.4 şablonu birebir kuruldu, `doveconf -n` temiz
- Sanal kutuya **IMAPS girişi** (993) ve mesaj okuma
- Postfix → LMTP → maildir **teslimat**, bounce yok, yerleşim 2.3 ile aynı
- Kiracı sağlama: site + Linux kullanıcı + per-tenant FPM + nginx vhost + DB
- nginx → `php-fpm-<kullanıcı>.sock` üzerinden **PHP 8.3.33 çalıştırma**
- ext4 kotası: GRUB + tek seferlik reboot + `quotacheck` + `quotaon`, `repquota` okunur
- kota **enforcement'ı gerçekten uyguluyor**: 5 MB sınırın üstündeki kiracıda iki
  ayrı `dd` yazması 0 bayt kopyaladı (sınır aşımı bloklandı)
- **iki ardışık reboot** boyunca kota, site, posta ve panel ayakta kaldı
- PHP 7.4 · 8.0 · 8.1 · 8.2 · 8.3 · 8.4 · 8.5 sury'den kurulu (8.5 dahil)

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
| ~~**5a**~~ | ✅ Canlı test: **Debian 12** — 39 kontrol/0 hata, 7 hata bulundu ve düzeltildi (§5d) | — |
| ~~**5b**~~ | ✅ Canlı test: **Debian 13** — 44 kontrol/0 hata, 5 hata bulundu ve düzeltildi (§5f) | — |
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
