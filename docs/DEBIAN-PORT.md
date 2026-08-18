# Debian / Ubuntu desteği — teknik plan

**Durum:** planlama. Kod yazılmadı.
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
| kota araçları | `xfsprogs`, `quota` | `quota`, `e2fsprogs` | |
| certbot | `certbot`, `python3-certbot-nginx` | aynı | |
| diğer | `policycoreutils-python-utils`, `setools-console` | **yok** (SELinux) | Debian'da hiç kurulmaz |

### 3.4 Apache backend (~42 satır)

Panel opsiyonel Apache backend'i destekliyor (`Backend == "apache"`, 127.0.0.1:10080 proxy).
Debian'da conf düzeni tamamen farklı (`/etc/apache2/sites-available` + `a2ensite`, modül
adları farklı).

**Öneri: v1'de Debian'da Apache backend'i kapatalım.** Kullanım oranı düşük, iş yükü
yüksek. UI'da "bu sunucuda kullanılamıyor" denir. Sonraki sürüme bırakılır.

---

## 4. Disk kotası — işin tek gerçek zor kısmı

Kaynak izolasyonunun disk ayağı şu an **tamamen `xfs_quota`** üzerine kurulu
(`internal/kaynaklimit/kaynaklimit.go`). Debian/Ubuntu bulut imajlarında kök dosya
sistemi neredeyse her zaman **ext4**.

### Mevcut yapı (iyi haber: temiz kapsüllenmiş)

| Fonksiyon | Görev |
|---|---|
| `mountKotaAktif()` | `xfs_quota -x -c 'state -u' /` → accounting/enforcement |
| `kotaLimitArgs()` | **saf fonksiyon**, birim testli |
| `KotaUygula()` | limiti yaz |
| `kotaReportSatir()` | `xfs_quota report -u -N` → kullanım oku |
| `KotaDurum()` | UI için durum |

### Yapılacak: kota backend arayüzü

```go
type KotaBackend interface {
    Aktif() (accounting, enforcement bool)
    Uygula(sk string, diskMB, inode int) error
    Durum(sk string) (kullanimMB, limitMB, kullanilanInode, limitInode int, err error)
}
```

- **XFS backend:** mevcut kod, olduğu gibi taşınır (regresyon riski sıfır).
- **ext4 backend:** `setquota` / `repquota` (`quota` paketi).

```bash
setquota -u c_ornek <bsoft> <bhard> <isoft> <ihard> /
repquota -u -O csv /
```

### Etkinleştirme akışı — mevcut UX birebir örtüşüyor

XFS'te kota yalnız **mount anında** açılabildiği için panel `rootflags=uquota` yazıp
tek seferlik reboot istiyor ve bunu bir "reboot gerekli" sentinel'iyle UI'da gösteriyor
(`kotaRebootSentinel`).

ext4'te de aynı mekanizma çalışıyor — yalnız flag ve GRUB komutu değişiyor:

| | RHEL / XFS | Debian / ext4 |
|---|---|---|
| kernel flag | `rootflags=uquota` | `rootflags=usrquota` |
| grub üret | `grub2-mkconfig -o /boot/grub2/grub.cfg` | `grub-mkconfig -o /boot/grub/grub.cfg` |
| BLS güncelle | `grubby --update-kernel=ALL` | yok (gerekmez) |
| reboot sonrası | otomatik aktif | `quotacheck -cum /` + `quotaon /` gerekebilir |

**Sentinel + "reboot gerekli" arayüzü aynen kullanılır.** Yeni UX yazılmayacak.

### Canlı doğrulanacaklar

- `tune2fs -O quota` bağlıyken çalışıyor mu, yoksa klasik `quotacheck` yolu mu gerekli?
- Bulut imajlarında `/` gerçekten ext4 mi (bazıları btrfs olabilir → kota desteklenmez,
  dürüstçe "kota kapalı" denir; panelin geri kalanı çalışır).

---

## 5. Installer (`sanalcp-install.sh`, 587 satır)

Tespit edilen RHEL bağımlılığı: 14 `dnf` çağrısı, 12 SELinux çağrısı, 8 firewalld,
7 remi/epel.

**Yaklaşım:** tek dosyayı ikiye bölmek yerine, üstte bir `OS_AILE` tespiti + ince
sarmalayıcı fonksiyonlar (`pkg_install`, `svc_enable`, `repo_php_ekle`) tanımlanacak;
gövde ortak kalacak. Böylece iki ayrı installer'ın zamanla birbirinden ayrışması
(en sık görülen bakım hatası) önlenir.

Debian tarafında ek adımlar:
- `apt-get update` + `DEBIAN_FRONTEND=noninteractive`
- sury deposu: anahtar + `deb https://packages.sury.org/php/ <codename> main`
- `firewalld` yok → doğrudan `nftables` (panel zaten nftables kullanıyor, kolaylaşıyor)

---

## 6. Sıralama

| Faz | İş | Risk |
|---|---|---|
| **0** | `internal/osfam` + tespit + birim testler | düşük |
| **1** | phpMap/PHPSurumler ikinci tablo, nginx kullanıcısı, servis adı haritası | düşük |
| **2** | 6 dosyadaki paket yöneticisi çağrılarını soyutlamaya taşı | düşük |
| **3** | Kota backend arayüzü + ext4 uygulaması | **yüksek** |
| **4** | Installer soyutlaması + sury deposu | orta |
| **5a** | Canlı test: **Debian 12** (MariaDB 10.11 — bilinen DB, yalnız dağıtım farkı test edilir) | — |
| **5b** | Canlı test: **Debian 13** (MariaDB 11.8 devreye girer) | — |
| **5c** | Canlı test: **Ubuntu 26.04**, ardından 24.04 | — |
| **6** | Apache backend + CVE ekranı: Debian'da kapat, dürüstçe belirt | düşük |

Faz 0-2 birbirini besliyor ve tek oturumda bitebilir. Faz 3 ayrı ve dikkat isteyen iş.

Faz 5 sırası bilinçlidir: **5a'da yalnız "dağıtım ailesi" değişkeni**, 5b'de üzerine
"MariaDB sürümü" değişkeni biner. Tersi sırada bir hata çıksa hangisinden geldiği
belirsiz kalırdı.

Faz 5 olmadan **hiçbir şey "destekleniyor" diye ilan edilmeyecek.**

---

## 7. Karara bağlanacaklar

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
