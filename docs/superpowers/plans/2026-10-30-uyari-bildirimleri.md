# Uyarı Bildirimleri (Telegram + E-posta) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Panel, domain sorunlarını (site erişilemiyor, SSL bitiyor, yedek doğrulaması başarısız) kimse panele girmeden admin'e Telegram ve/veya e-posta ile bildirsin.

**Architecture:** Yeni `internal/uyari` paketi 5 dakikada bir tüm aktif domain'leri kontrol eder, her (domain, kontrol) için son durumu `uyari_durumlari` tablosunda tutar ve yalnız durum *değiştiğinde* (ok→uyari/hata, hata→ok) mesaj gönderir. Ayarlar `panel_ayarlari` tekil satırında; Telegram token'ı `secretcrypt` ile şifreli. Web kontrolü `monitor` paketindeki mevcut SSRF korumalı `probe`'u yeniden kullanır.

**Tech Stack:** Go 1.26 (stdlib `net/http`, `net/smtp`), MariaDB, go-sqlmock, React + react-i18next, Vitest.

**Spec:** Bu sohbette kararlaştırılan öneri (ayrı spec dosyası yok): ayarlar (Telegram token + chat ID + e-posta), 5 dk arka plan kontrolü, yalnız durum geçişinde bildirim, "test mesajı gönder" butonu.

## Global Constraints

- Telegram token asla API yanıtında, logda ya da düz metin DB'de görünmez; GET yalnız `telegram_token_ayarli: bool` döner. Boş token gönderilen PUT mevcut token'ı korur.
- Tüm yeni uçlar `middleware.AdminOnly`. Demo modunda yazma uçları mevcut `DemoSaltOkunur` ile zaten engelli; ek kod yazılmaz.
- Kontrol kapsamı: `domains WHERE askida=0 AND is_demo=0`.
- Kontrol aralığı sabit `5 * time.Minute`; eşzamanlı web probu en fazla `8`.
- Migration numarası: `0096_uyari_bildirimleri.sql`.
- Mesaj dili `panel_ayarlari.varsayilan_dil` (`tr`|`en`, diğer her şey `tr`).
- Kapsam dışı (YAGNI): DNS/posta kotası/disk kontrolleri, müşteri/bayi bazlı bildirim, webhook/Slack, yeniden deneme kuyruğu, `DomainHealthCard`'ın backend'e taşınması.

## Review Focus

1. **Tek seferlik ağ hıçkırığı gece 3'te mesaj atar** → web `hata` durumu ancak art arda 2 başarısız kontrolde kaydedilir (Task 2 testi `TestWebIkiArdisikHatadaHata`).
2. **Panel ilk açıldığında/ilk kurulumda mevcut tüm sorunlar için mesaj seli** → durumu hiç kaydedilmemiş bir kontrol için ilk sonuç sessizce kaydedilir, bildirim gitmez (Task 3 `TestIlkGozlemSessiz`).
3. **Telegram/SMTP çökük, gönderim hata veriyor** → durum yine kaydedilir (aksi halde her 5 dakikada aynı mesaj yeniden denenir), hata `log.Printf` ile yazılır (Task 3 `TestGonderimHatasindaDurumYineKaydedilir`).
4. **Domain silinir** → `uyari_durumlari` satırları FK `ON DELETE CASCADE` ile gider, hayalet kurtarma mesajı yok (Task 1 migration).
5. **Bildirim kapalı ya da hiçbir kanal ayarlı değil** → döngü hiçbir şey göndermez ama durum yine güncellenir; açıldığında geçmiş sorunlar için sel olmaz (Task 3 `TestKapaliykenGondermez`).

---

## File Structure

| Dosya | Sorumluluk |
|---|---|
| `migrations/0096_uyari_bildirimleri.sql` | `panel_ayarlari` kolonları + `uyari_durumlari` tablosu |
| `internal/monitor/monitor.go` | `Probe` dışa açılır, `Health` onu kullanır |
| `internal/uyari/degerlendir.go` | Saf fonksiyonlar: ham veri → `Durum` |
| `internal/uyari/gonder.go` | Telegram + SMTP gönderimi |
| `internal/uyari/dongu.go` | `Run`, `Tur` (bir tur kontrol + geçiş + gönderim) |
| `internal/uyari/handlers.go` | Ayar GET/PUT + test mesajı |
| `cmd/server/main.go` | `uyari.Init`, `go uyari.Run`, route'lar |
| `frontend/src/components/UyariBildirimAyari.tsx` (+ test, i18n tr/en) | Ayar kartı |
| `frontend/src/pages/AraclarAyarlarPage.tsx:248` | Kartı ekle |

---

### Task 1: Migration + `monitor.Probe`

**Files:**
- Create: `migrations/0096_uyari_bildirimleri.sql`
- Modify: `internal/monitor/monitor.go` (`Health` ~satır 107, `probe` ~satır 160)
- Test: `internal/monitor/monitor_test.go` (yoksa oluştur)

**Interfaces:**
- Produces: `func Probe(alanAdi string) DomainHealth`: önce `https://`, erişilemezse `http://` dener; `Sema` alanını doldurur. `Health` handler'ı bunu çağırır, davranışı değişmez.
- Produces (DB):
  - `panel_ayarlari`: `bildirim_acik TINYINT(1) NOT NULL DEFAULT 0`, `telegram_token_enc TEXT NULL`, `telegram_chat_id VARCHAR(64) NOT NULL DEFAULT ''`, `bildirim_eposta VARCHAR(320) NOT NULL DEFAULT ''`
  - `uyari_durumlari(domain_id BIGINT UNSIGNED NOT NULL, kontrol VARCHAR(16) NOT NULL, durum VARCHAR(8) NOT NULL, ardisik_hata INT NOT NULL DEFAULT 0, degisti_at DATETIME NOT NULL, PRIMARY KEY(domain_id,kontrol), FK domain_id → domains(id) ON DELETE CASCADE)`. `ADD COLUMN IF NOT EXISTS` / `CREATE TABLE IF NOT EXISTS` (0095 stili).

- [ ] **Step 1:** Migration'ı yaz. `go test ./cmd/server -run Migration` → PASS (mevcut `migrations_test.go` sıra/sözdizimi kontrolü).
- [ ] **Step 2: Failing test** `TestProbe_SSRFEngelli`: `Probe("127.0.0.1")` → `Erisilebilir == false`, `Hata` "engellendi" içerir.
- [ ] **Step 3:** `go test ./internal/monitor -run TestProbe` → FAIL (`Probe` tanımsız).
- [ ] **Step 4:** `Health` içindeki https→http düşme mantığını `Probe`'a taşı; `Health` = DB okuma + `httpx.WriteJSON(w, 200, Probe(alanAdi))`.
- [ ] **Step 5:** `go test ./internal/monitor` → PASS (demo engeli testleri dahil).
- [ ] **Step 6:** Commit `feat(monitor): Probe dışa açıldı; uyarı bildirimleri migration'ı`

---

### Task 2: Saf değerlendirme fonksiyonları

**Files:**
- Create: `internal/uyari/degerlendir.go`
- Test: `internal/uyari/degerlendir_test.go`

**Interfaces:**
- Produces:
  ```go
  type Durum string // "ok" | "uyari" | "hata"
  func SSLDurum(aktif bool, kaynak string, bitis *time.Time, simdi time.Time) (Durum, bool) // bool=false → kontrol uygulanmaz
  func WebDurum(h monitor.DomainHealth, oncekiArdisik int) (d Durum, ardisik int)
  func YedekDurum(sonDogrulama string) (Durum, bool) // "" → uygulanmaz
  ```
- Kurallar:
  - SSL: `!aktif` ya da `kaynak=="self-signed"` ya da `bitis==nil` → uygulanmaz. Kalan gün `≤3` (geçmiş dahil) → `hata`; `≤14` → `uyari`; aksi `ok`.
  - Web: başarısız = `!Erisilebilir || DurumKodu >= 500`. Başarısızsa `ardisik = onceki+1`; `ardisik >= 2` → `hata`, değilse `ok` (henüz). Başarılıysa `ok, 0`. 4xx = `ok` (bakım sayfası/403 bilerek olabilir).
  - Yedek: domain'in en son yedeğinin `dogrulama_durum`'u; `basarisiz` → `hata`, `dogrulandi` → `ok`, `bekliyor`/`dogrulaniyor` → uygulanmaz.

- [ ] **Step 1: Failing tests** (tablo testleri):
  - `TestSSLDurum`: 30 gün → ok; 14 → uyari; 4 → uyari; 3 → hata; -1 → hata; self-signed → uygulanmaz; aktif=false → uygulanmaz.
  - `TestWebIkiArdisikHatadaHata`: (erişilemez, 0) → (ok, 1); (erişilemez, 1) → (hata, 2); (200, 5) → (ok, 0); (503, 1) → (hata, 2); (404, 0) → (ok, 0).
  - `TestYedekDurum`: basarisiz→hata, dogrulandi→ok, dogrulaniyor/""→uygulanmaz.
- [ ] **Step 2:** `go test ./internal/uyari` → FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** `go test ./internal/uyari` → PASS.
- [ ] **Step 5:** Commit `feat(uyari): kontrol değerlendirme kuralları`

---

### Task 3: Gönderim + döngü

**Files:**
- Create: `internal/uyari/gonder.go`, `internal/uyari/dongu.go`
- Test: `internal/uyari/gonder_test.go`, `internal/uyari/dongu_test.go`

**Interfaces:**
- Consumes: Task 1 `monitor.Probe`, Task 2 fonksiyonları.
- Produces:
  ```go
  var box *secretcrypt.Box
  func Init(b *secretcrypt.Box)                       // cloudflare.Init kalıbı
  type Ayar struct { Acik bool; TelegramToken, TelegramChatID, Eposta, Dil string }
  func AyarOku(ctx context.Context, db *sql.DB) (Ayar, error) // token'ı çözer
  type Gonderici struct { TelegramURL string; SMTPAddr string; HTTP *http.Client } // üretim: "https://api.telegram.org", "127.0.0.1:25"
  func (g Gonderici) Gonder(ctx context.Context, a Ayar, metin string) error // ayarlı her kanala; hataları errors.Join
  func Tur(ctx context.Context, db *sql.DB, g Gonderici, probe func(string) monitor.DomainHealth, simdi time.Time) error
  func Run(ctx context.Context, db *sql.DB) // 5 dk ticker, ilk tur 1 dk gecikmeli; Tur hatasını loglar
  ```
- Telegram: `POST {TelegramURL}/bot{token}/sendMessage`, form `chat_id`, `text`; 2xx dışı → hata (gövde loglanmaz, token URL'de olduğundan `url.Error` mesajı da token'sız sarılır).
- E-posta: `smtp.SendMail(SMTPAddr, nil, "sanalcp@"+os.Hostname(), []string{Eposta}, msg)`, konu `[SanalCP] Uyarı`/`[SanalCP] Alert`.
- `Tur`: domain'leri okur → kontrol başına yeni durum hesaplar (web probları 8'lik semafor ile paralel) → `uyari_durumlari` ile karşılaştırır. Kayıt yoksa: sessizce INSERT. Durum farklıysa: UPSERT + mesaj listesine ekle. Aynıysa: yalnız `ardisik_hata`'yı güncelle. Tur sonunda `Acik` ve en az bir kanal varsa tüm mesajları **tek** gönderimde birleştir (satır başına: `🔴/🟡/🟢 alan.com — SSL 3 gün içinde bitiyor` vb.; tr/en metinler `dongu.go` içinde küçük bir map).

- [ ] **Step 1: Failing tests** `gonder_test.go` (`httptest.Server`):
  - `TestTelegramGonderir`: istek yolu `/botTOK/sendMessage`, `chat_id=42`, `text` beklenen metin.
  - `TestTelegramHataTokenSizlar`: sunucu 401 döner → hata döner ve `err.Error()` "TOK" içermez.
  - `TestKanalYoksaHicbirSeyYapmaz`: boş Ayar → nil, sunucuya istek yok.
- [ ] **Step 2: Failing tests** `dongu_test.go` (sqlmock + sahte probe + sahte Telegram):
  - `TestIlkGozlemSessiz`: kayıt yok, SSL 3 gün → INSERT beklenir, Telegram'a istek yok.
  - `TestGecisteBildirir`: kayıt `ok`, SSL 3 gün → UPSERT `hata` + 1 Telegram isteği, metinde alan adı.
  - `TestKurtarmadaBildirir`: kayıt `hata`, web 200 → `ok` + 🟢 mesaj.
  - `TestGonderimHatasindaDurumYineKaydedilir`: Telegram 500 → UPSERT yine beklenir, `Tur` nil döner.
  - `TestKapaliykenGondermez`: `bildirim_acik=0`, geçiş var → UPSERT var, istek yok.
- [ ] **Step 3:** `go test ./internal/uyari` → FAIL.
- [ ] **Step 4:** Implement.
- [ ] **Step 5:** `go test ./internal/uyari -race` → PASS.
- [ ] **Step 6:** Commit `feat(uyari): durum geçişinde Telegram/e-posta bildirimi`

---

### Task 4: Ayar uçları + bağlama

**Files:**
- Create: `internal/uyari/handlers.go`
- Test: `internal/uyari/handlers_test.go`
- Modify: `cmd/server/main.go` (Init ~satır 194 yanına; `go uyari.Run` ~satır 327 yanına; route'lar ~satır 480 yanına)

**Interfaces:**
- Consumes: Task 3 `AyarOku`, `Gonderici`, `box`.
- Produces:
  ```go
  type Handlers struct{ DB *sql.DB; Gonderici Gonderici }
  func (h *Handlers) Ayar(w, r)        // GET  /system/uyari-bildirim
  func (h *Handlers) AyarKaydet(w, r)  // PUT  /system/uyari-bildirim
  func (h *Handlers) Test(w, r)        // POST /system/uyari-bildirim/test
  ```
- GET yanıtı: `{"acik":bool,"telegram_token_ayarli":bool,"telegram_chat_id":string,"eposta":string}`
- PUT gövdesi: `{"acik":bool,"telegram_token":string,"telegram_chat_id":string,"eposta":string}`. Doğrulama: `telegram_chat_id` boş ya da `^-?\d{1,20}$|^@[A-Za-z0-9_]{5,32}$`; `eposta` boş ya da `net/mail.ParseAddress` geçer ve ≤320; `telegram_token` boş (=koru), `"-"` (=sil) ya da `^\d+:[A-Za-z0-9_-]{30,}$`. Hata → 400 `httpx.WriteError`.
- Test: mevcut ayarla `"SanalCP test mesajı"` gönderir; kanal yoksa 400 `"bildirim kanalı ayarlı değil"`; gönderim hatası → 502 + kanal bazlı token'sız hata metni.

- [ ] **Step 1: Failing tests:**
  - `TestAyarTokenDondurmez`: DB'de şifreli token var → yanıt gövdesi token'ı içermez, `telegram_token_ayarli:true`.
  - `TestKaydetBosTokenKorur`: PUT boş token → UPDATE token kolonuna dokunmaz.
  - `TestKaydetGecersizEposta`: `"x@"` → 400.
  - `TestKaydetGecersizChatID`: `"abc"` → 400.
  - `TestTestKanalYok`: → 400.
- [ ] **Step 2:** `go test ./internal/uyari -run 'Ayar|Kaydet|TestTest'` → FAIL.
- [ ] **Step 3:** Implement; `main.go`'ya `uyari.Init(secretBox)`, `go uyari.Run(context.Background(), d)`, 3 route `r.With(middleware.AdminOnly)` ile.
- [ ] **Step 4:** `go test ./internal/uyari ./cmd/server && go build ./...` → PASS.
- [ ] **Step 5:** Commit `feat(uyari): bildirim ayar uçları`

---

### Task 5: Ayar kartı (frontend)

**Files:**
- Create: `frontend/src/components/UyariBildirimAyari.tsx`, `frontend/src/components/UyariBildirimAyari.test.tsx`, `frontend/src/i18n/locales/tr/UyariBildirimAyari.json`, `frontend/src/i18n/locales/en/UyariBildirimAyari.json`
- Modify: `frontend/src/pages/AraclarAyarlarPage.tsx:19,248` (import + `<UyariBildirimAyari />` `PanelHizLimitiAyari`'nin altına)

**Interfaces:**
- Consumes: Task 4 uçları.
- `PanelHizLimitiAyari.tsx` kalıbı: `api`, `apiHata`, `AyarKartDurumu`, yanıt doğrulayıcı fonksiyon. Alanlar: açık/kapalı anahtarı, token (password input, placeholder `telegram_token_ayarli` ise "•••• kayıtlı"), chat ID, e-posta, Kaydet, "Test mesajı gönder". Kaydette token alanı boşsa `""` gönderilir.
- Test sözleşmesi `AyarSozlesmesi.test.tsx` kalıbına uyar (dosya o testi kapsıyorsa listeye ekle).

- [ ] **Step 1: Failing tests** (`UyariBildirimAyari.test.tsx`, api mock):
  - `kayıtlı token'ı göstermez, boş bırakılınca "" gönderir`: GET `telegram_token_ayarli:true` → input değeri `""`; Kaydet → PUT gövdesi `telegram_token: ""`.
  - `test butonu hatayı gösterir`: POST 400 `{"hata":"bildirim kanalı ayarlı değil"}` → metin ekranda.
- [ ] **Step 2:** `cd frontend && npx vitest run src/components/UyariBildirimAyari.test.tsx` → FAIL.
- [ ] **Step 3:** Implement + i18n (tr/en anahtarları birebir aynı).
- [ ] **Step 4:** `npx vitest run && npx tsc --noEmit` → PASS.
- [ ] **Step 5:** Commit `feat(uyari): bildirim ayar kartı`

---

### Task 6: Uçtan uca doğrulama (sunucuda)

- [ ] **Step 1:** Build + deploy (projenin mevcut release akışı); migration 0096 uygulandı mı: `SHOW COLUMNS FROM panel_ayarlari LIKE 'bildirim%'`.
- [ ] **Step 2:** Araçlar & Ayarlar → Telegram bot token + chat ID gir, "Test mesajı gönder" → Telegram'a mesaj geldi.
- [ ] **Step 3:** `SELECT COUNT(*) FROM uyari_durumlari` ilk turdan (~1 dk) sonra > 0 ve Telegram'a mesaj **gelmedi** (ilk gözlem sessiz).
- [ ] **Step 4:** Test domain'inde `UPDATE uyari_durumlari SET durum='hata' WHERE domain_id=<test> AND kontrol='web'` → sonraki turda 🟢 kurtarma mesajı geldi. (Canlı DB'de tek satır, test domain'i; onay alınarak.)
- [ ] **Step 5:** Panel servis logunda (`journalctl -u <panel servisi> | grep -i uyari`) token görünmüyor.
