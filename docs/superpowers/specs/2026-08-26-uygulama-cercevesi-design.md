# Uygulama Kurulum Çerçevesi (1-Tık Uygulamalar) — Tasarım

**Tarih:** 2026-08-26
**Durum:** Onaylandı (brainstorming), uygulama planı bekleniyor

## Amaç

Panelde bugün sadece WordPress için 1-tık kurulum/güncelleme/silme var
(`internal/wordpress`, wp-cli tabanlı). Hedef: Joomla, Drupal, PrestaShop,
Nextcloud, OpenCart gibi popüler uygulamaları da aynı panelden kurulabilir
hale getirmek — ama her biri için WordPress'i tekrar tekrar yazmak yerine,
ortak bir "uygulama kur" çerçevesi kurup üzerine tek tek eklemek.

Bu spec **çerçevenin kendisini** ve **ilk somut uygulamayı (PrestaShop)**
kapsar. Joomla/Drupal/Nextcloud/OpenCart bu çerçeve oturduktan sonra ayrı
ayrı brainstorm→spec→plan döngüsünden geçecek alt-projelerdir (bkz. Sıra).

**Kapsam dışı (bilinçli):**
- Eklenti/tema/kullanıcı yönetimi derinliği (WordPress'teki gibi) — yeni
  uygulamalarda yalnız **kur/güncelle/sil**. Her uygulama için ayrı
  eklenti/tema API'si inşa etmek kapsamı çok büyütür.
- PrestaShop'ta **güncelleme** — resmi bir CLI güncelleyici yok (AutoUpgrade
  modülü CLI'dan güvenilir tetiklenemiyor). İlk sürümde yalnız kur+sil;
  arayüzde "bu uygulama için güncelleme desteklenmiyor" notu.
- DB tablosu / migration — tespit tamamen dosya sistemi taraması (mevcut WP
  deseniyle birebir aynı), stateless kalıyor.
- WordPress'in kendi `/wordpress/*` route'larının/handler'larının
  davranışını değiştirmek — bu spec onlara **dokunmaz**, sadece paralel bir
  yol açar (bkz. Alınan Kararlar).

## Mevcut Durum

`internal/wordpress/wordpress.go` içindeki `Kur`/`Guncelle`/`Sil` handler'ları
şu ortak iskeleti WordPress'e özel kodla iç içe barındırıyor:

- domain lookup + demo-abonelik reddi + `adlar.SKGecerli(sk)`
- alt dizin doğrulama (`reAltDizin`) + hedef yol çözümü
- eşzamanlı kurulum kilidi (`wpKurulumKilit sync.Map`, key=hedef dizin) +
  "zaten kurulu" guard (`kurulumZatenVar`, marker dosya taraması)
- hedef dizin oluştur + `chown`+`restorecon`
- DB oluştur (`hesaplar.MySQLCreateDB`)
- **WordPress'e özel:** `wp core download` → `wp config create` (parola
  stdin ile, `wpKomutStdin`) → `wp config` doğrulama (regex) → `wp core
  install` (admin parolası stdin ile) → admin parolasının fiilen
  kurulduğunu `wp eval` ile doğrulama
- hata durumunda temizlik (`basarisiz`: DB+kullanıcı DROP, kendi
  oluşturduğu alt dizini kaldır)
- final `chown`+`restorecon` + JSON yanıt (site/admin URL, admin
  kullanıcı/parola, sürüm)

`Sil` ayrıca kök-site koruması (public_html'in kendisi silinemez) ve
çapraz-tenant DROP koruması (DB adı regex + `db_accounts` sahiplik kontrolü)
içeriyor — bunlar da WordPress'e özel config dosyası formatından (wp-config.php
regex ile `DB_NAME` çekme) ayrıştırılabilir.

Tespit tamamen dosya sistemi taraması: `public_html` + 1 alt dizin seviyesinde
`wp-config.php` var mı bakılıyor (`Liste`/`TumListe`), DB tablosu yok.

Frontend: sol menüde tek "WordPress" satırı (`WordPressPage.tsx`, tüm
domainlerdeki WP kurulumlarını `TumListe`'den listeler); domain panelinde
(`DomainPano.tsx`) tek "WordPress" kartı → `DomainWordPressPage.tsx` (kurulum
formu + kurulu liste + eklenti/tema/kullanıcı/bakım sekmeleri).

## Alınan Kararlar

| Konu | Karar | Elenen alternatif |
|---|---|---|
| Navigasyon | Tek "Uygulamalar" menüsü + domain panelinde tek "Uygulamalar" kartı, kurulumda tür seçim sihirbazı | Her uygulama için ayrı menü satırı/kart (5+ türde kalabalıklaşır) |
| WordPress kodu | `internal/wordpress` **dokunulmadan** kalır, `/wordpress/*` route'ları aynen çalışmaya devam eder; yeni çerçeveye ince bir Adapter ile bağlanır | wp-cli mantığını yeni arayüze taşıyıp yeniden yazmak (gereksiz risk, çalışan koda dokunma) |
| İlk pilot uygulama | PrestaShop | Nextcloud (en karmaşık, çerçeveyi ilk kez onun üzerinden tasarlamak riskli) |
| Yönetim derinliği | Yeni uygulamalarda yalnız kur/güncelle/sil | WordPress'teki gibi tam eklenti/tema/kullanıcı yönetimi (kapsam çok büyür) |
| Tespit modeli | Dosya sistemi taraması (marker dosya), DB tablosu yok | `app_installs` tablosu (migration + tutarlılık riski, mevcut WP deseniyle tutarsız) |

## Mimari

### 1. `internal/apps` — ortak çerçeve (yeni paket)

**Registry arayüzü** — her uygulama türü bunu implement eder:

```go
package apps

type FormAlan struct {
    Anahtar   string // "site_basligi", "admin_email"...
    Etiket    string
    Tur       string // "text" | "email" | "password"
    Zorunlu   bool
    RegexAdi  string // frontend'de hangi doğrulama deseni kullanılacak (bkz. mevcut reAdmin/reEmail)
}

type KurulumIstek struct {
    DomainID       int64
    SK, AlanAdi    string
    SSL            bool
    Hedef          string            // mutlak dizin (zaten oluşturulmuş, chown'lanmış)
    URL            string            // scheme+alanadi(+altdizin) — ortak katman hazırlar
    DBAdi, DBKullanici, DBParola string // ortak katman DB'yi ÖNCEDEN oluşturur, driver'a hazır geçer
    Alanlar        map[string]string // FormAlanlari()'na göre doğrulanmış kullanıcı girdisi
}

type KurulumSonuc struct {
    SiteURL, AdminURL             string
    AdminKullanici, AdminParola   string // driver kendi üretir/doğrular (WP'deki randParola+eval deseni gibi)
    Surum                         string
    Ekstra                        map[string]string // türe özel ek bilgi (ör. PrestaShop admin dizini)
}

type Uygulama interface {
    Slug() string        // "wordpress", "prestashop"
    Ad() string           // görünen ad
    MarkerDosya() string  // "wp-config.php", "config/settings.inc.php" — kurulu tespiti
    FormAlanlari() []FormAlan
    GuncelleDesteklenir() bool

    Kur(ctx context.Context, i KurulumIstek) (KurulumSonuc, error)
    Bilgi(ctx context.Context, sk, dizin string) (Kurulum, error) // sürüm+site url+güncelleme durumu (liste taraması için)
    Guncelle(ctx context.Context, sk, dizin string) error          // GuncelleDesteklenir()==false ise hiç çağrılmaz
    DBAdiOku(dizin string) (dbAdi string, bulundu bool)             // silme sırasında DB temizliği için (WP'nin reDBName deseni gibi)
}

func Kaydet(u Uygulama)         // init() sırasında her tür kendini kaydeder
func Hepsi() []Uygulama
func Bul(slug string) (Uygulama, bool)
```

`internal/wordpress` ve `internal/prestashop` (ve ileride Joomla/Drupal/...)
kendi `init()`'lerinde `apps.Kaydet(...)` çağırır — `cmd/server/main.go`
sadece bu paketleri blank/normal import eder, registry kendi kendini
doldurur (mevcut `driver kaydı` desenleriyle tutarlı bir yaklaşım).

### 2. Ortak HTTP handler katmanı — `internal/apps/handlers.go`

WordPress'in `Kur`/`Sil` handler'larındaki **tür-bağımsız** iskeleti buraya
taşınır (WordPress'in kendi handler'ları bu koddan kopyalanmaz, aynı mantığı
tekrar implemente eder — ayrıntı aşağıda):

- `POST /domains/{id}/apps/{tur}/kur`
  1. domain lookup + demo reddi + `adlar.SKGecerli`
  2. `apps.Bul(tur)`, yoksa 404
  3. body decode → `FormAlanlari()`'na göre alan bazlı doğrulama (zorunlu/regex)
  4. alt dizin doğrulama + hedef yol + kilit (`sync.Map`, WP'deki
     `wpKurulumKilit` ile aynı desen, `apps` paketine taşınır) + "zaten
     kurulu" guard (`u.MarkerDosya()` dosya var mı)
  5. mkdir+chown+restorecon
  6. `hesaplar.MySQLCreateDB` ile DB oluştur (slug: `<tur kısaltması>_<rastgele>`)
  7. `u.Kur(ctx, KurulumIstek{...})` — driver'a özel kısım
  8. Hata → ortak temizlik (DB+kullanıcı DROP, kendi oluşturduğu alt dizini
     kaldır) — WP'deki `basarisiz` closure'ının tür-agnostik hali
  9. final chown+restorecon + JSON yanıt

- `DELETE /domains/{id}/apps/{tur}` — kök-site koruması + `u.DBAdiOku(dizin)`
  ile DB adı çıkar + WP'deki iki katmanlı DROP koruması (ad deseni +
  `db_accounts` sahiplik) tür-agnostik hale getirilip buraya taşınır +
  `os.RemoveAll`.

- `POST /domains/{id}/apps/{tur}/guncelle` — `GuncelleDesteklenir()==false`
  ise 400 ("bu uygulama için güncelleme desteklenmiyor"), aksi halde
  `u.Guncelle(ctx, sk, dizin)`.

- `GET /domains/{id}/apps` — `apps.Hepsi()` içindeki her tür için
  `public_html` + 1 alt dizin taraması (WP'nin `Liste` deseni), marker dosya
  eşleşen her dizin için `u.Bilgi(...)` çağrılır, birleşik liste döner.

- `GET /domains/{id}/apps/turler` — `apps.Hepsi()`'den `{slug, ad,
  form_alanlari}` listesi (sihirbaz dropdown + dinamik form şeması).

- `GET /apps/tumu` (global, `BayiVeUstu`) — WP'nin `TumListe`'sinin
  tür-agnostik hali: tüm domainler × `apps.Hepsi()` taraması, worker-pool(4)
  ile paralel `Bilgi()` çağrısı.

### 3. WordPress Adapter — `internal/wordpress/adapter.go` (yeni dosya, aynı paket)

Aynı pakette olduğu için `wpKomut`, `wpKomutStdin`, `wpConfigDBParolaDogrula`,
`randParola`, `reAdmin`/`reEmail`, `reDBName` gibi unexported yardımcılara
doğrudan erişir — **hiçbiri exported edilmez, kopyalanmaz.**

```go
type Adapter struct{}
func (Adapter) Slug() string { return "wordpress" }
func (Adapter) MarkerDosya() string { return "wp-config.php" }
func (Adapter) Kur(ctx, i apps.KurulumIstek) (apps.KurulumSonuc, error) {
    // wpKomut(core download) → wpKomutStdin(config create, i.DBParola) →
    // wpConfigDBParolaDogrula → wpKomutStdin(core install, adminParola) →
    // eval doğrulama — Kur()'daki WP'ye-özel gövdenin BİREBİR AYNISI,
    // sadece DB zaten ortak katmanda oluşturulduğu için o adım atlanır.
}
```

`internal/wordpress/wordpress.go`'daki `Kur`/`Guncelle`/`Sil` HTTP
handler'ları **silinmez, değişmez** — `/wordpress/*` route'ları aynen
çalışmaya devam eder (`WordPressPage.tsx`/`DomainWordPressPage.tsx`'in
eklenti/tema/kullanıcı/bakım sekmeleri hâlâ bu route'lara bağlı). Adapter
sadece `/apps/wordpress/*` için paralel bir yol açar; iki yol da aynı alt
seviye fonksiyonları (wpKomut vb.) çağırdığı için WP kurulum/silme mantığı
TEK yerde yaşamaya devam eder, çatallanma yalnız HTTP handler seviyesinde.

`init()` içinde `apps.Kaydet(Adapter{})`.

### 4. PrestaShop pilot — `internal/prestashop` (yeni paket)

`wordpress.go`'daki `runuser -u sk -- env HOME=/home/sk TMPDIR=/home/sk php
...` desenini birebir kopyalar (kendi `psKomut` fonksiyonu).

- **Kurulum:** `php install/index_cli.php --domain=<alanadi> --db_server=localhost
  --db_name=<i.DBAdi> --db_user=<i.DBKullanici> --db_password=<i.DBParola>
  --db_prefix=ps_ --name=<site_basligi> --email=<admin_email>
  --password=<admin_parola> --language=tr --country=tr --all_languages=0
  --newsletter=0` (tam bayrak listesi implementasyon sırasında kurulu
  PrestaShop sürümünün `install/index_cli.php --help` çıktısıyla
  doğrulanacak — sürümden sürüme değişebilir).
- **Marker/tespit:** `config/settings.inc.php` var mı.
- **DB adı okuma (silme için):** `config/settings.inc.php` içindeki
  `define('_DB_NAME_', '...')` regex ile (WP'nin `reDBName` desenine
  paralel).
- **Güncelleme:** `GuncelleDesteklenir()` → `false`.
- Kurulum sonrası admin dizini PrestaShop tarafından rastgele isimlendirilir
  (`admin<rastgele>/`) — bu, `KurulumSonuc.Ekstra["admin_dizini"]` ile
  frontend'e taşınır ve kurulum sonucu ekranında vurgulanır (silinmezse
  admin paneline erişilemez).

`init()` içinde `apps.Kaydet(Yeni())`.

### 5. Route kayıtları — `cmd/server/main.go`

```go
appsH := &apps.Handlers{DB: d}
r.With(middleware.MusteriScope).Get("/domains/{id}/apps", appsH.Liste)
r.With(middleware.MusteriScope).Get("/domains/{id}/apps/turler", appsH.Turler)
r.With(middleware.MusteriScope).Post("/domains/{id}/apps/{tur}/kur", appsH.Kur)
r.With(middleware.MusteriScope).Post("/domains/{id}/apps/{tur}/guncelle", appsH.Guncelle)
r.With(middleware.MusteriScope).Delete("/domains/{id}/apps/{tur}", appsH.Sil)
r.With(middleware.BayiVeUstu).Get("/apps/tumu", appsH.TumListe)
```

Mevcut `/wordpress/*` satırları **aynen kalır** (üstteki dosyada, değişmez).

### 6. Frontend

- Sol menü: `WordPress` satırı → `Uygulamalar` olarak yeniden adlandırılır,
  hedef route `/uygulamalar` (yeni `AppsPage.tsx`, `WordPressPage.tsx`'in
  yerini alır — tablo tür sütunu eklenmiş hali, `GET /apps/tumu` çeker).
  Eski `/wordpress` route'u `/uygulamalar`'a redirect eder (yer imi kırılmasın).
- `DomainPano.tsx`: "WordPress" kartı → "Uygulamalar" kartı, hedef yeni
  `DomainAppsPage.tsx` (route `abonelikler/:id/uygulamalar`).
  - Kurulu liste (tür ikonlu satırlar, `GET /domains/{id}/apps`) — her
    satırda "Yönet" (WordPress ise mevcut `DomainWordPressPage.tsx`'e route,
    diğer türlerde bu sayfada satır-içi Güncelle/Sil), "Sil".
  - "+ Yeni Uygulama Kur" → tür seçim kartları (`GET
    /domains/{id}/apps/turler`) → seçilen türün `form_alanlari`'na göre
    **dinamik** kurulum formu (tek generic form bileşeni, alan şeması
    API'den gelir — WordPress'in bugünkü sabit formu YERİNE bu kullanılır,
    ama WP'nin kendi eski formu `DomainWordPressPage.tsx` içinde referans
    olarak kalabilir ya da bu genel forma geçebilir; implementasyon planında
    netleştirilir).
- `DomainWordPressPage.tsx` (eklenti/tema/kullanıcı/bakım sekmeleri)
  **değişmeden kalır**, sadece `DomainAppsPage.tsx`'teki "Yönet" linkinin
  hedefi olur (bugün `DomainPano`'dan doğrudan gidiliyor, yarın
  `DomainAppsPage`'den).
- Yeni ikon: PrestaShop için `ICONS.prestashop` (`DomainPano.tsx`).
- i18n: `AppsPage.json`, `DomainAppsPage.json` (yeni, tr+en);
  `WordPressPage.json` içeriği `AppsPage.json`'a taşınır.

## Hata Davranışı

- Kurulum sırasında herhangi bir adım başarısız olursa (DB oluşturma,
  driver'ın `Kur()`'u, dosya işlemleri) ortak katman DB+kullanıcıyı DROP eder
  ve kendi oluşturduğu alt dizini kaldırır — WP'deki `basarisiz` deseniyle
  birebir, artık tüm türler için ortak.
- PrestaShop `install/index_cli.php` başarısız çıkış kodu dönerse çıktının
  son 600 karakteri hata mesajına eklenir (WP'nin `kisalt` deseniyle aynı).
- Güncelleme desteklenmeyen türde `POST .../guncelle` çağrılırsa 400 +
  "bu uygulama için güncelleme desteklenmiyor" — driver'a hiç girilmez.
- Silme: kök-site koruması ve çapraz-tenant DROP koruması tüm türlerde
  ortak katmanda uygulanır (`u.DBAdiOku`'nun döndürdüğü ad da WP'deki gibi
  ad-deseni + `db_accounts` sahiplik çift kontrolünden geçer).

## Test Planı

- `internal/apps`: ortak handler iskeleti için birim testler — kilit/eşzamanlı
  kurulum reddi, "zaten kurulu" guard, DB temizliği (hata enjekte edilen
  sahte `Uygulama` implementasyonuyla, gerçek DB/dosya sistemi gerektirmeyen
  testler mevcut `wpinstall_guard_test.go` desenindeki gibi).
- `internal/wordpress/adapter_test.go`: Adapter'ın `apps.Uygulama`
  arayüzünü doğru implement ettiğini + mevcut `wpKomut` tabanlı akışla aynı
  sonucu ürettiğini doğrulayan testler (mevcut `wpconfig_test.go` desenine
  paralel).
- `internal/prestashop`: `wpinstall_guard_test.go`/`wpconfig_test.go`
  desenlerinin PrestaShop karşılığı — marker tespiti, DB adı regex
  çıkarımı, kurulum guard'ları. Gerçek `install/index_cli.php` çağrısı
  gerektiren kısımlar (implementasyon planında) gerçek bir PrestaShop
  kurulumuna karşı manuel doğrulanacak (feedback: dış komut/CLI davranışı
  koddan değil gerçek çalıştırmadan doğrulanmalı).
- Frontend: `AppsPage`/`DomainAppsPage` için mevcut proje deseninde (birim
  testi yok) manuel smoke test — kurulum, silme, tür seçimi, WP'nin eski
  akışının (Yönet sekmeleri) kırılmadığının doğrulanması.

## Sıra (bu spec sonrası)

1. Bu spec → implementasyon planı → `internal/apps` + WP Adapter + PrestaShop.
2. Joomla — ayrı brainstorm (CLI kurulum deseni sürüme göre değişken,
   araştırma gerektirir).
3. Drupal — ayrı brainstorm (`drush` ile wp-cli'ye yakın, ama composer/PHP
   sürüm bağımlılığı incelenmeli).
4. OpenCart — ayrı brainstorm.
5. Nextcloud — ayrı brainstorm, en son (Redis/cron/depolama/PHP-FPM ayar
   farklılıkları nedeniyle en karmaşık).

## Değişecek/Eklenecek Dosyalar

- `internal/apps/apps.go` (yeni — registry arayüzü)
- `internal/apps/handlers.go` (yeni — ortak HTTP handler'lar)
- `internal/apps/apps_test.go` (yeni)
- `internal/wordpress/adapter.go` (yeni — `apps.Uygulama` implementasyonu)
- `internal/wordpress/adapter_test.go` (yeni)
- `internal/prestashop/prestashop.go` (yeni)
- `internal/prestashop/prestashop_test.go` (yeni)
- `cmd/server/main.go` (yeni route'lar, `internal/apps` + `internal/prestashop` import)
- `frontend/src/pages/AppsPage.tsx` (yeni, `WordPressPage.tsx`'in yerini alır)
- `frontend/src/pages/DomainAppsPage.tsx` (yeni, `DomainPano.tsx`'teki kartın hedefi)
- `frontend/src/App.tsx` (yeni route'lar + `/wordpress` redirect)
- `frontend/src/components/DomainPano.tsx` (kart adı/hedefi + `ICONS.prestashop`)
- `frontend/src/components/DashboardLayout.tsx` (menü etiketi/hedefi)
- `frontend/src/i18n/locales/{tr,en}/AppsPage.json` (yeni)
- `frontend/src/i18n/locales/{tr,en}/DomainAppsPage.json` (yeni)
