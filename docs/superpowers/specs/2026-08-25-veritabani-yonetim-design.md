# Veritabanı Yönet Sayfası — Tasarım

**Tarih:** 2026-08-25
**Durum:** Onaylandı (brainstorming), uygulama planı bekleniyor

## Amaç

Domain > Veritabanları ekranındaki satır aksiyonları (phpMyAdmin, şifre sıfırla,
sil) yetersiz kalıyor. Her veritabanı için tek bir "Yönet" sayfası açılacak;
bu sayfada boyut, kullanıcı yönetimi (ekle/sil/şifre değiştir), isim değiştirme,
tek tıkla gzip yedekleme, geri yükleme, optimize ve onar (repair) işlemleri
yapılabilecek.

**Kapsam dışı (bilinçli):** Tablo bazlı yönetim (tekil tablo optimize/repair/
drop) yok — phpMyAdmin zaten bunu karşılıyor, buton sayfada korunuyor. Yedekler
sunucuda saklanmıyor/listelenmiyor (anında indirme, bkz. Alınan Kararlar).

## Mevcut Durum

`db_accounts` tablosu satır = (db_name, db_user) çifti. Bir veritabanı adı
birden fazla satırda görünebilir — "mevcut kullanıcı" modunda
(`CreateDatabase`, `internal/domains/handlers.go:1010-1026`) var olan bir
kullanıcıya yeni bir DB için GRANT verilebiliyor; bu, aynı `db_user`'ın farklı
`db_name`'lerde satır açmasına yol açıyor. Ancak bugün **aynı DB'ye ikinci bir
kullanıcı ekleme** akışı yok — `MySQLCreateDBForUser` her zaman `CREATE
DATABASE` da yapıyor.

`DomainDatabasesPage.tsx` bugün her `db_accounts` satırını ayrı bir tablo
satırı olarak listeliyor (db_adi tekilleştirilmiyor), satır başına phpMyAdmin
aç / şifre sıfırla / sil butonları var.

`DELETE /databases/{dbid}` ve `PUT /databases/{dbid}/password` bugün
`AdminOnly` — muhtemelen URL'de `domain_id` olmadığı için sahiplik kontrolü
atlanmış (bkz. `pma.TokenIste`'deki `middleware.DomainSahibiMi` IDOR deseni,
`internal/pma/pma.go:64-70`). Liste/oluşturma (`ListDatabases`,
`CreateDatabase`) zaten `MusteriScope`.

## Alınan Kararlar

Brainstorming sırasında netleşen kararlar:

| Konu | Karar | Elenen alternatif |
|---|---|---|
| Sayfa granülerliği | Veritabanı **adına** göre grupla (bir DB, birden çok kullanıcı) | Mevcut satır (db+kullanıcı çifti) başına ayrı sayfa |
| İsim değiştirme | Uygulanır, belirgin uyarı gösterilir ("uygulama config dosyanızı elle güncelleyin") | Bu turda atlanır |
| Yedekleme | mysqldump\|gzip anında tarayıcıya indirilir, sunucuda saklanmaz | `backups` tablosuna kaydedip liste/retention kurmak |
| Geri yükleme | Eklenir (yedeklemenin doğal karşılığı) | Eklenmez |
| Erişim modeli | Yeni uçlar `MusteriScope` + `DomainSahibiMi`; mevcut iki `AdminOnly` uç da aynı korumaya çekilir | Tümü `AdminOnly` kalır |

## Mimari

### 1. Backend — veri modeli değişikliği yok

`db_accounts` şeması aynen kalıyor. Gruplama sorgu zamanında yapılıyor
(`GROUP BY db_name` / `WHERE db_name=?`), yeni kolon/migration gerekmiyor.

### 2. Backend — yeni dosya `internal/domains/handlers_dbyonet.go`

Route öneki: `/api/v1/domains/{id}/databases/{dbAdi}/...` (hepsi
`MusteriScope` + handler girişinde `middleware.DomainSahibiMi(r, domainID)`
kontrolü — yok/yetkisizse var-olmayan DB ile aynı 404, varlık sızdırılmaz,
`pma.TokenIste` deseniyle birebir).

- `GET /domains/{id}/databases/{dbAdi}` — grup detayı:
  ```
  {
    db_adi, db_host, charset, collation, boyut_mb,
    kullanicilar: [{id, db_kullanici, db_parola, olusturulma}]
  }
  ```
  Boyut+charset tek sorgudan:
  `SELECT COALESCE(SUM(data_length+index_length),0)/1024/1024, default_character_set_name, default_collation_name FROM information_schema.tables t JOIN information_schema.schemata s ON s.schema_name=t.table_schema WHERE t.table_schema=? GROUP BY s.default_character_set_name, s.default_collation_name`
  (DB boşsa fallback: `information_schema.schemata`'dan tek satır, boyut 0.)

- `PUT /domains/{id}/databases/{dbAdi}/isim` body `{yeni_sonek}` — isim
  değiştirme. `yeni_sonek` mevcut `GecerliDBSonek`/`<sk>_` önek kuralıyla
  doğrulanır (create ile aynı kısıtlar). Adım adım:
  1. `CREATE DATABASE <yeni>`
  2. `mysqldump --single-transaction --routines --triggers --events <eski> | mysql <yeni>`
     (RENAME TABLE döngüsü YERİNE — view/trigger/routine/event'i de taşımak
     için; `internal/backups/create.go`'daki mysqldump kalıbıyla tutarlı)
  3. Bu DB'ye grant'lı her `db_user` için `GRANT ALL PRIVILEGES ON <yeni>.* TO ...`
     + `REVOKE ALL PRIVILEGES ON <eski>.* FROM ...`
  4. `DROP DATABASE <eski>`
  5. `UPDATE db_accounts SET db_name=<yeni> WHERE db_name=<eski>`
  Herhangi bir adım başarısızsa `<yeni>` DB'si best-effort `DROP` edilir
  (temizlik), hata döner; `<eski>` adım 4'e kadar dokunulmamış durumda kalır
  (adım 4 en son, geri dönüşü olmayan adım).
  Yanıtta `"uyari"` alanı sabit metinle döner, frontend modalde gösterir.

- `POST /domains/{id}/databases/{dbAdi}/kullanicilar` body
  `{kullanici_tipi: "yeni"|"mevcut", kullanici_sonek?, parola?, mevcut_kullanici?}`
  — `CreateDatabase`'in kullanıcı alt-akışıyla aynı doğrulama. "mevcut" seçilirse
  seçilen kullanıcının bu domain'e ait olduğu kontrol edilir (mevcut
  `paylasim`/sahiplik sorgusuyla aynı desen) ve zaten bu DB'de olmadığı
  kontrol edilir. **Kota kontrolü yok** (`kota.CheckDBEklenebilir` çağrılmaz —
  yeni DB değil, mevcut DB'ye ek kullanıcı).
  Yeni `hesaplar.MySQLGrantOnly(db, domainID, dbName, dbUser, dbPass string, yeniKullanici bool) error`:
  `yeniKullanici` true ise önce `CREATE USER`, sonra `GRANT ALL PRIVILEGES ON db.* TO user@localhost`,
  sonra `db_accounts` satırı eklenir.

- `DELETE /domains/{id}/databases/{dbAdi}/kullanicilar/{dbid}` — kullanıcı
  çıkar (DB'yi silmez). Bu DB'nin **son** kullanıcısıysa 409 döner ("bu DB'nin
  tek kullanıcısı, silmek yerine veritabanını silin"). Değilse:
  - `db_user` başka bir `db_name`'de de kullanılıyor mu (global, tüm
    domain'ler) → `SELECT COUNT(*) FROM db_accounts WHERE db_user=? AND id<>?`.
    Varsa yalnız `REVOKE ALL PRIVILEGES ON dbAdi.* FROM user@localhost` + satır
    silinir (kullanıcı yaşamaya devam eder). Yoksa `DROP USER` + satır silinir.
  Yeni `hesaplar.MySQLRevokeUser(db, dbName, dbUser string, dropUser bool) error`.

- `GET /domains/{id}/databases/{dbAdi}/yedek` — `mysqldump --single-transaction
  --routines --triggers --events dbAdi | gzip`, `Content-Disposition:
  attachment; filename="dbAdi-YYYYMMDD-HHMMSS.sql.gz"` ile stream edilir.
  Sunucuya yazılmaz (pipe doğrudan response'a).

- `POST /domains/{id}/databases/{dbAdi}/geri-yukle` — multipart dosya (`.sql`
  veya `.sql.gz`, üst sınır 200MB). `.gz` ise `gzip.NewReader` ile açılıp
  `mysql dbAdi` komutunun stdin'ine pipe edilir. Frontend'de tehlikeli-onay
  diyaloğu zorunlu (mevcut tabloları ezebilir).

- `POST /domains/{id}/databases/{dbAdi}/optimize` ve
  `POST /domains/{id}/databases/{dbAdi}/onar` — `mysqlcheck --optimize dbAdi`
  / `mysqlcheck --repair dbAdi` (tüm tablolar, tek komut), çıktı satır satır
  `{"sonuc": "tablo1 OK\ntablo2 OK\n..."}` olarak döner. Senkron (mevcut
  yedekleme/temizleme uçlarıyla aynı desen — arka planda değil, istek boyunca
  bekler).

Var olan `DELETE /databases/{dbid}` ve `PUT /databases/{dbid}/password`
uçları `AdminOnly` → `MusteriScope` + `DomainSahibiMi(r, domainID)` (join
üzerinden `domain_id` çekilip kontrol edilir) olarak güncellenir.

### 3. Frontend

**`DomainDatabasesPage.tsx`** — `dbler` listesi `db_adi`'na göre
`useMemo` ile gruplanır (`{db_adi, kullanici_sayisi, ilk_olusturulma}[]`).
Aksiyon sütunu: yalnız **"Yönet"** linki
(`/abonelikler/{id}/veritabanlari/{db_adi}`). phpMyAdmin/şifre
sıfırla/sil butonları kaldırılır (Yönet sayfasına taşınır).

**Yeni sayfa `DomainDatabaseYonetPage.tsx`**
(route `abonelikler/:id/veritabanlari/:dbAdi`, `App.tsx`'e eklenir):

1. Başlık satırı: DB adı (mono) + "phpMyAdmin'de Aç" butonu (mevcut
   `pmaAc` mantığı, `db_accounts.id`'lerden herhangi biriyle — hangi
   kullanıcıyla açıldığı önemli değil, aynı DB'ye bakıyor).
2. **Genel bilgi kartı** — boyut (MB, `fmt` yardımcı fonksiyonuyla),
   charset/collation, sunucu (`host:3306`), "İsim Değiştir" (modal:
   yeni sonek input + sabit uyarı metni + `ConfirmDialog tehlikeli`).
3. **Kullanıcılar kartı** — satır başına: kullanıcı adı, parola
   göster/kopyala (mevcut desen), "Şifre Değiştir" (var olan
   `DBParolaSifirlaModal` yeniden kullanılır), "Sil" (son kullanıcıysa
   `disabled` + tooltip). Üstte "Kullanıcı Ekle" (yeni `YeniKullaniciModal`
   — `YeniDBModal`'ın kullanıcı alt-formuyla aynı UI, DB adı sabit).
4. **Bakım kartı** — 4 buton: Yedekle (indirme tetikler, `window.location`
   veya `<a>` ile — auth cookie zaten gidiyor), Geri Yükle (dosya seç +
   `ConfirmDialog tehlikeli`), Optimize Et, Onar. Her biri kendi
   yükleniyor durumunu tutar, sonuç `<pre>` benzeri küçük panelde
   (mono, kaydırılabilir) gösterilir.

i18n: `frontend/src/i18n/locales/{tr,en}/DomainDatabaseYonetPage.json`
(yeni), `DomainDatabasesPage.json`'a yalnız "Yönet" butonu metni eklenir,
kaldırılan buton anahtarları silinir.

### 4. Hata davranışı

- Yedekleme/geri yükleme/optimize/repair sırasında `exec` hatası → 500 +
  `stderr` metni (mevcut backup deseniyle aynı, `strings.Builder` stderr
  yakalama).
- İsim değiştirme yarıda kalırsa (adım 2-3 arası hata) `<yeni>` DB'si
  best-effort silinir, `<eski>` bozulmamış kalır — kullanıcıya net hata
  mesajıyla "değişiklik uygulanmadı" bildirilir.
- Kullanıcı silme: son kullanıcı korumasıyla veri kaybı/erişilemez-DB
  durumu engellenir.

## Test Planı

- `internal/hesaplar`: `MySQLGrantOnly`, `MySQLRevokeUser` için mevcut
  `MySQLCreateDB`/`MySQLDropDBKeepUser` testleriyle aynı desende testler
  (gerçek MySQL bağlantısı gerektiren entegrasyon testleri, mevcut dosyada
  nasıl ele alınıyorsa öyle).
- `internal/domains/handlers_dbyonet.go`: saf karar fonksiyonları (son
  kullanıcı kontrolü, sonek doğrulama, sahiplik) birim test edilir.
- İsim değiştirme: mysqldump/mysql komutlarının çağrıldığı entegrasyon
  testi (varsa mevcut test-MySQL altyapısı kullanılır) + yarıda kalma
  senaryosunun temizlik davranışı.
- Frontend: `DomainDatabasesPage` gruplama mantığı (`useMemo`) için birim
  test yoksa bile manuel smoke test yeterli (proje genelinde frontend
  birim testi yok, mevcut desen).

## Değişecek/Eklenecek Dosyalar

- `internal/domains/handlers_dbyonet.go` (yeni)
- `internal/domains/handlers.go` (DELETE/PUT rol değişikliği: `AdminOnly` → `MusteriScope`+ownership)
- `internal/hesaplar/hesaplar.go` (`MySQLGrantOnly`, `MySQLRevokeUser`, rename için mysqldump|mysql yardımcıları)
- `cmd/server/main.go` (yeni route'lar)
- `frontend/src/pages/DomainDatabasesPage.tsx` (gruplama, Yönet linki)
- `frontend/src/pages/DomainDatabaseYonetPage.tsx` (yeni)
- `frontend/src/App.tsx` (yeni route)
- `frontend/src/i18n/locales/{tr,en}/DomainDatabaseYonetPage.json` (yeni)
- `frontend/src/i18n/locales/{tr,en}/DomainDatabasesPage.json` (buton metinleri)
