# Panel Girişini Sunucu root Hesabından Ayırma — Tasarım

**Tarih:** 2026-08-20
**Durum:** Onaylandı (brainstorming), uygulama planı bekleniyor

## Amaç

SanalCP paneline giriş, ilk sürümden beri **sunucunun root parolasıdır**. `internal/auth/handlers.go` içindeki `rootShadowHash()` doğrudan `/etc/shadow`'dan root hash'ini okur ve yescrypt ile doğrular. Panelin kendi kullanıcı tablosu (`users`, bcrypt) yıllardır var ama root bilinçli olarak bu tablonun dışında bırakılmıştı.

Bu tasarım, panelin birincil giriş yolunu `users` tablosundaki gerçek bir admin hesabına taşır ve root/shadow yolunu bir break-glass mekanizmasına indirger.

**Kapsam dışı (bilinçli):** SSH root erişimi. Sunucunun root parolası iptal edilmiyor, `PermitRootLogin` değişmiyor, SSH akışına hiç dokunulmuyor. Bu spec yalnız `:8443` panel girişini konu alır.

## Neden

Bu işi tetikleyen olay, 2026-08-19 tarihinde `passwd` ile değiştirilen root parolasının panel girişini sessizce kırmasıydı. Altında yatan yapısal sorunlar:

1. **Kimlik eşleşmesi.** SSH root parolasını döndürmek panel girişini kırar, tersi de geçerli. İki ayrı amaç tek sırra bağlı.
2. **Saldırı yüzeyi.** `:8443` internete açık. `internal/middleware/ratelimit.go` bunu kendi yorumunda kabul ediyor: panel girişindeki bir zafiyet doğrudan tam sunucu ele geçirilmesine çıkar, ara kademe yoktur.
3. **Hesap verebilirlik yok.** `audit_log`'da her giriş "root" görünür. Birden fazla operatör varken kimin ne yaptığı ayırt edilemez.
4. **Kök yetkili web daemon.** Panelin `/etc/shadow` okuyabilmesi root olarak çalışmasını zorunlu kılar.
5. **Kişi başına 2FA anlamsız.** Paylaşılan tek hesapta TOTP kişiyi değil hesabı korur.

## Alınan Kararlar

Brainstorming sırasında dört karar netleşti:

| Konu | Karar | Elenen alternatif |
|---|---|---|
| İlk kurulum admin'i | Installer üretir, tarayıcıda kurulum ekranı **yok** | CloudPanel tarzı ilk-çalıştırma sihirbazı (kapma yarışı riski), kurulum jetonu, loopback kısıtı |
| Panelin root girişi | Kalır, **varsayılan kapalı**, bayrakla açılabilir | Tamamen kaldırma (break-glass kaybı), açık + 2FA zorunlu, değiştirmeme |
| Mevcut kurulumların göçü | Migration bayrağı **açık** ekler, installer yeni kurulumlarda kapatır | Update'in otomatik admin üretmesi, tamamen elle |
| Üretilen admin parolası | **Yalnız ekrana**, diske yazılmaz | Root-only dosyaya da yazma, kurulumda interaktif sorma |

Kurtarma yolu her durumda `sanalcp-seed-admin`'dir: SSH root erişimi durduğu için kilitlenme kalıcı değildir.

## Mimari

### 1. Veri modeli

`migrations/0069_panel_root_girisi.sql`:

```sql
ALTER TABLE panel_ayarlari
  ADD COLUMN IF NOT EXISTS root_girisi_acik TINYINT(1) NOT NULL DEFAULT 1;
```

`panel_ayarlari` tek satırlık ayar tablosudur (`migrations/0044_panel_ayarlari.sql`, `id=1`).

Varsayılanın `1` olması göç stratejisinin tamamıdır: migration mevcut kurulumlarda çalıştığında hiçbir davranış değişmez, kimse kilitlenmez. Yeni kurulumlarda değeri `0`'a çeken taraf installer'dır.

### 2. Backend kapısı

`internal/auth/handlers.go`, `Login` içindeki root dalının başına bayrak kontrolü girer:

- Bayrak kapalıysa istek reddedilir. Yanıt **diğer başarısızlıklarla birebir aynı** genel mesajdır (`"kullanıcı adı veya parola hatalı"`) — root girişinin kapalı olduğu dışarıya sızmaz.
- Reddedilen istek `WriteAudit(..., "auth.login", ..., false)` ile kaydedilir ve 401 döndüğü için `middleware.GirisLimiti` sayacına da girer. Kapalı bayrak, kaba-kuvvet denemelerini görünmez yapmaz.
- Bayrak okunamazsa (DB hatası) **fail-closed**: root reddedilir. Bu, aynı handler'daki 2FA durumu okuma kararıyla tutarlıdır. Kaybedilen bir şey yoktur; `panel_ayarlari` okunamıyorsa `users` tablosu da okunamaz, yani alternatif giriş yolu zaten çalışmıyordur.

`KullaniciRootMu` (`internal/auth/parola.go:36`) saf fonksiyon olarak kalır. Aynı dosyanın yorumu onu "bu ayrımın tek karar noktası" diye tanımlıyor; bayrak okuma bir DB işlemidir ve oraya girmez, `Login` içinde kalır.

### 3. Installer

`sanalcp-install.sh` adım 13 (`Admin access (root + PAM)`) yeniden yazılır:

- Yeni argüman `--admin-kullanici` (varsayılan `admin`), mevcut `--admin-parola` / `--admin-eposta` / `--lang` ile aynı çözümleme bloğunda.
- `--admin-parola` verilmemişse rastgele üretilir. Üretilen parola **yalnız kurulum çıktısına** basılır; hiçbir dosyaya yazılmaz.
- Mevcut root satırı tohumlaması **olduğu gibi korunur**: adım 13 bugün `sanalcp-seed-admin -kullanici root -parola "$(openssl rand -hex 16)"` çağırıyor ve ardından `users` satırının placeholder e-posta/ad alanlarını temizliyor. Bu blok değişmez.
- Buna **ek olarak** ikinci bir çağrı yapılır: `sanalcp-seed-admin -kullanici "$ADMIN_KULLANICI" -parola "$ADMIN_PAROLA"`. Seed aracına değişiklik gerekmez — kaynağı `scripts/seed_admin.go` (`//go:build ignore`) zaten `-kullanici` bayrağı alır, `role='admin'` yazar, bcrypt cost 12 kullanır ve `ON DUPLICATE KEY UPDATE` ile idempotenttir.
- `users` tablosundaki root satırı böylece **yerinde kalır**. Root/shadow yolu tekrar açılırsa `Login` orada `uid, kadi, rol = 1, "root", "admin"` ataması yapıyor; satır silinirse profil/2FA sorguları kırılır.
- `UPDATE panel_ayarlari SET root_girisi_acik=0 WHERE id=1` ile yeni kurulumlarda root girişi kapatılır.
- Kapanış satırı değişir: `ok "Login: username 'root' + this server's root password"` yerine üretilen admin kullanıcı adı ve parolası basılır.

### 4. Panel arayüzü

Ayarlar sayfasına `AdminOnly` bir anahtar eklenir: "Sunucu root parolasıyla panel girişi". Bayrağı okuyup yazan uçlar mevcut `panel_ayarlari` handler desenini izler.

**Son-admin koruması.** Bu koruma **zaten mevcut** ve üç handler'a da bağlı: `usersH.Sil` (`internal/users/handlers.go:646`), `usersH.DurumDegistir` (`:416`) ve `usersH.Guncelle` (`:317`), hepsi `sonAdminMi` (`:347`) üzerinden geçiyor. Eklenecek bir koruma yok; **düzeltilecek bir sayım var**.

`sonAdminMi` bugün şunu soruyor:

```sql
SELECT COUNT(*) FROM users WHERE role='admin' AND status='active' AND id<>?
```

`users` tablosundaki root satırı (`id=1`) `role='admin'` ve `status='active'` olduğu için bu sayıma **her zaman dahil**. Root girişi kapatıldığında sonuç şu olur: sistemdeki son gerçek admin silinebilir, çünkü root kalan admin sanılır — ama root ile giriş yapılamadığı için panele kimse giremez. Kilitlenme deliği tam burada.

Düzeltme: root girişi kapalıyken sayım root satırını **dışlar**. Root açıkken mevcut davranış birebir korunur, çünkü o zaman root gerçekten kullanılabilir bir kurtarma yoludur.

**Bayrağı kapatan ucun kendi koruması.** Ayarlar'dan root girişi kapatılırken sistemde aktif ve root olmayan bir admin yoksa istek reddedilir. Aksi halde tek adımda kendini dışarı kilitlemek mümkün olurdu: root kapanır, girebilecek başka hesap yoktur. Bu kontrol `sonAdminMi` düzeltmesinden bağımsızdır — biri hesap silmeyi, diğeri bayrağı kapatmayı korur.

### 5. Göç

Kod tarafında göç işi yoktur. Mevcut iki kurulum (`cloud.sanalcp.com` ve ikinci kurulum) güncellemeden sonra üç adım izler:

1. Panel → Kullanıcılar'dan admin hesabı oluştur (veya SSH'tan `sanalcp-seed-admin`)
2. Yeni hesapla giriş yapıp doğrula
3. Ayarlar'dan root girişini kapat

## Test Stratejisi

`internal/auth` paketinde:

- Root girişi kapalıyken doğru parolayla bile 401 döner, `audit_log`'a `ok=0` kaydı yazılır ve yanıt mesajı diğer başarısızlıklarla aynıdır.
- Root girişi açıkken mevcut davranış bire bir korunur (yescrypt yolu, legacy crypt yolu, 2FA akışı).
- Bayrak okunamadığında root reddedilir (fail-closed).
- `users` tablosundaki admin hesabı, root girişi kapalıyken normal şekilde giriş yapabilir.

Kullanıcı yönetimi tarafında:

- `sonAdminMi`: root girişi kapalıyken root satırı sayılmaz — geriye tek gerçek admin kalmışsa silme/pasifleştirme/rol düşürme reddedilir.
- `sonAdminMi`: root girişi açıkken root satırı sayılır, mevcut davranış birebir korunur.

Ayarlar ucunda:

- Aktif ve root olmayan admin yokken root girişini kapatma isteği reddedilir.
- Böyle bir admin varken kapatma başarılı olur.

Installer tarafında `internal/osfam` altındaki mevcut testler korunur: installer paritesi (Debian/RHEL ortak gövde) ve `set -o pipefail` altında `| grep -q` boru hattı yasağı. Yeni installer kodu ortak gövdede kalır, dağıtıma özel dallanma eklemez.

## Birlikte Giden Değişiklik

Aynı dalda, bu işi tetikleyen olayın ikinci bulgusuna karşılık gelen bir sertleştirme de yer alır: `/etc/profile.d/sanalcp-history.sh` ile `HISTCONTROL=ignoreboth`. Hem `sanalcp-install.sh` (yeni kurulumlar) hem `assets/ops/sanalcp-update` (mevcut kurulumlar) bu dosyayı idempotent olarak yazar. Boşlukla başlayan komutlar kabuk geçmişine hiç yazılmaz; panel girişi sunucunun root parolası olduğu sürece operatörler bu parolayı root kabuğunda elleyeceği için doğrudan ilgilidir.
