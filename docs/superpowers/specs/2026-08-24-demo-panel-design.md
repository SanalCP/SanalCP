# demo.sanalcp.com — Salt-Okunur Demo Panel — Tasarım

**Tarih:** 2026-08-24
**Durum:** Onaylandı (brainstorming), uygulama planı bekleniyor

## Amaç

Paneli kurmayı düşünenlerin, kurulum yapmadan önce **tüm özellikleri** gezebileceği canlı bir demo. Piyasadaki panellerin (CloudPanel, HestiaCP) demo sayfalarıyla aynı model: gerçek arayüz, gerçek veri, **değişiklik yapılamaz**.

## Kapsam dışı (bilinçli)

- Ziyaretçi başına izole sandbox/VM — reddedildi (maliyet + karmaşıklık, bkz. "Alınan Kararlar").
- Herhangi bir alanda gerçek yazma izni ("karma model") — reddedildi, salt-okunur ihlali kanıtlamak zor.
- Sahte/statik frontend (backend'siz demo) — reddedildi, gerçek panel deneyimi vaadini karşılamıyor.
- Bu spec yalnız demo VPS'ini ve panelin demo moduna aldığı davranışı kapsar. Mevcut üretim sunucusuna (cloud.sanalcp.com) hiç dokunulmaz — ayrı, bağımsız bir kurulumdur.

## Alınan Kararlar

Brainstorming sırasında netleşen üç karar:

| Konu | Karar | Elenen alternatif |
|---|---|---|
| İzolasyon modeli | Ortak panel + sert salt-okunur (CloudPanel/HestiaCP modeli) | ziyaretçi başına geçici sandbox (cPanel modeli); sahte API'li salt-frontend |
| Sır döndüren GET'ler (SSH anahtarı, DB/mail parolası, dosya oku/indir, log/süreç) | Maskele, uç 200 dönmeye devam etsin | tamamen 403 kapat; serbest bırak |
| Barındırma | Ayrı, bağımsız VPS | mevcut üretim sunucusuna (18 gerçek kiracı) kurulum |

## Neden

- **Satış/değerlendirme aracı yok.** README'de "0.x beta, okumadan kurmayın" uyarısı var; adayların kurulum riskine girmeden özellikleri görmesi gerekiyor.
- **Üretim sunucusuna kurulamaz.** `sanalcp-install.sh` boş sunucu varsayar (yıkıcı), aynı süreç/DB'yi paylaşmak anonim demo trafiğini gerçek 18 kiracıyla aynı kaynak havuzuna sokar, tek bir middleware kaçağı gerçek müşteri sırrını sızdırabilir. (bkz. [[sunucu-sanalcp-uretim]])
- **`domains.SeedIfEmpty` bilerek no-op.** Geçmişte "boş tabloya 4 demo domain ekle" davranışı, domain silinip tablo boşalınca demo'ların sessizce geri türemesi bug'ına yol açmıştı (`internal/domains/seed.go` yorumu). Bu spec o fonksiyona DOKUNMAZ; demo tohumlama tamamen ayrı, elle tetiklenen bir araçtır.

## Mimari

### 1. Veri modeli

`migrations/00XX_demo_modu.sql` (mevcut `root_girisi_acik` şablonuyla birebir aynı desen):

```sql
-- Demo panel modu: açıkken tüm yazan istekler (beyaz liste hariç) 403 döner.
-- VARSAYILAN 0 — mevcut kurulumlarda davranış birebir korunur. Yalnız
-- sanalcp-demo-seed aracı tarafından demo VPS'inde elle açılır.
ALTER TABLE panel_ayarlari
  ADD COLUMN IF NOT EXISTS demo_modu_acik TINYINT(1) NOT NULL DEFAULT 0;
```

Okuma `internal/panelbayrak` paketine eklenir (`RootGirisiAcik` yanına `DemoModuAcik`) — aynı fail-closed desen: bayrak okunamazsa **açık kabul edilir mi, kapalı mı?** Burada `root_girisi_acik`'in tersine, fail-closed = **demo modu AÇIK** olmalı (okunamıyorsa yazmayı engellemek güvenli taraf; root girişinde fail-closed = erişimi reddetmekti, burada fail-closed = yazmayı reddetmek). Yani DB okunamazsa middleware yine de 403 üretir, sessizce normal moda düşmez.

### 2. Yazma engeli — `internal/middleware`

Yeni `DemoSaltOkunur` middleware, global zincire `CSRFKoruma`'dan hemen sonra eklenir (`cmd/server/main.go:304` civarı) — `r.Route("/api/v1", ...)`'dan ÖNCEKİ `git-webhook`/`pma-redeem` dahil TÜM route'ları kapsar, route bazlı değişiklik gerekmez.

Kural:
- `demo_modu_acik=0` → no-op, mevcut davranış korunur.
- `demo_modu_acik=1` → GET/HEAD serbest. Diğer metotlar, sabit bir beyaz listedeki path'ler hariç `403 {"hata":"demo modunda değişiklik yapılamaz"}` döner.
- Beyaz liste (sabit, kod içinde): `POST /api/v1/auth/login`, `POST /api/v1/auth/cikis`.

### 3. Sır maskeleme

Middleware, bayrak açıkken `context`'e `demo=true` işaretler (`middleware.DemoContext`, `ClaimsContext` deseniyle aynı). Sır döndüren handler'lar bu bayrağı okuyup değeri maskeler, 200 dönmeye devam eder:

- `internal/sshaccess` — özel anahtar görüntüleme
- `internal/hesaplar` / DB parola görüntüleme uçları
- `internal/mail` — posta hesabı parolası
- `internal/files` — `Read`/`Download` (içerik maskelenir, dosya listesi/meta AYNEN görünür)
- `internal/backups` — `Download`

Her değişiklik `// DEMO:` yorumuyla işaretlenir — ileride sır döndüren yeni bir handler eklenirse kod incelemesinde göze çarpsın diye.

### 4. Tohum veri

Yeni `cmd/demoseed` CLI (mevcut `scripts/seed_admin.go` deseniyle aynı: DSN argümanı, idempotent). Birkaç bayi, birkaç domain, örnek istatistik/log satırı, örnek eklenti kurulumu basar. Yalnız demo VPS'inde, kurulumdan sonra **elle** çalıştırılır — boot akışına (`SeedIfEmpty` gibi) hiç girmez.

### 5. Reset mekanizması

`systemd timer`, gecelik: servis durur → `mysql panel < demoseed-dump.sql` → tohum dosya ağacı geri kopyalanır → disk/log temizlenir → servis başlar. Yazma zaten API katmanında bloklu olduğu için reset'in asıl işi disk/log/metrik birikimi ve sayaçları (SSL süre sayacı, kota göstergesi) temizlemek.

### 6. Erişim

Paylaşımlı tek hesap (`demo` / sabit parola), login ekranında görünür/ön-doldurulmuş. `middleware.GirisLimiti` (kaba kuvvet koruması) aynen kalır.

## Testler

- `middleware.DemoSaltOkunur`: bayrak kapalıyken no-op; açıkken GET geçer, whitelist dışı POST/PUT/DELETE 403; whitelist'teki login/cikis geçer.
- `panelbayrak.DemoModuAcik`: DB hatasında `true` döner (fail-closed doğrulaması, mevcut `TestDomainSahibiMiFailClosed` deseniyle aynı).
- Maskeleme: her dokunulan handler için "demo=true iken maskeli, demo=false iken gerçek değer" test çifti.
- `cmd/demoseed`: idempotent çalıştırma (iki kez çalıştırınca hata/duplicate yok).

## Açık uçlar (uygulama planında netleşecek)

- `cmd/demoseed`'in tohum içeriği (kaç bayi/domain, örnek log hacmi) — plan aşamasında somutlaşacak.
- Reset script'inin tam yolu/adı ve dump dosyasının nerede tutulacağı.
