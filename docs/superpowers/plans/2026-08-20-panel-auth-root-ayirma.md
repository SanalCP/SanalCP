# Panel Girişini root'tan Ayırma — Uygulama Planı

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Panelin birincil giriş yolunu sunucunun root parolasından alıp `users` tablosundaki gerçek bir admin hesabına taşımak; root/shadow yolunu varsayılan kapalı bir break-glass mekanizmasına indirmek.

**Architecture:** `panel_ayarlari` tablosuna `root_girisi_acik` bayrağı eklenir (varsayılan `1`, mevcut kurulumlar etkilenmez). Bayrağı okuyan iç bağımlılığı sıfır bir yaprak paket (`internal/panelbayrak`) üç tüketiciye hizmet eder: `auth` (giriş kapısı), `users` (son-admin sayımı) ve `panelayarlari` (HTTP uçları). Installer yeni kurulumlarda gerçek bir admin oluşturup bayrağı `0`'a çeker.

**Tech Stack:** Go 1.24, chi router, MariaDB, `github.com/DATA-DOG/go-sqlmock` (testler), React + TypeScript + Tailwind (frontend), bash (installer).

**Spec:** `docs/superpowers/specs/2026-08-20-panel-auth-root-ayirma-design.md`

## Global Constraints

- SSH root erişimi **kapsam dışı**. `PermitRootLogin`, `/etc/shadow` içeriği, sshd yapılandırması değişmez.
- Migration varsayılanı **`1` (açık)** olmalı — mevcut kurulumlarda hiçbir davranış değişmez, kimse kilitlenmez. Bayrağı `0`'a çeken tek yer installer'dır.
- Bayrak okunamadığında davranış **fail-closed** (root reddedilir).
- Root girişi kapalıyken dönen HTTP yanıtı, hatalı parolayla dönenle **birebir aynı** olmalı: `401` + `"kullanıcı adı veya parola hatalı"`. Root girişinin kapalı olduğu sızmamalı.
- `KullaniciRootMu` (`internal/auth/parola.go:36`) saf fonksiyon kalır; DB I/O oraya girmez.
- Installer'ın ürettiği admin parolası **yalnız ekrana** basılır, hiçbir dosyaya yazılmaz.
- Installer kodu Debian/RHEL **ortak gövdede** kalır; dağıtıma özel dallanma eklenmez (`internal/osfam` paritesi testi bunu kolluyor).
- `set -o pipefail` altındaki bash betiklerinde `komut | grep -q desen` boru hattı yasak (`internal/osfam/pipefail_grep_test.go`). Çıktıyı değişkene alıp `case`/`=~` ile eşleştir.
- Panel metinleri Türkçe; frontend metinleri `react-i18next` ile `tr` + `en` dosyalarına eklenir.

**Zaten yapılmış — tekrar uygulama:** Spec'in "Birlikte Giden Değişiklik" bölümündeki `HISTCONTROL=ignoreboth` sertleştirmesi bu dalda **commit `ee9cc58` ile tamamlandı** (`sanalcp-install.sh` + `assets/ops/sanalcp-update`). Bu planın hiçbir task'ı ona dokunmaz.

---

### Task 1: Bayrak — migration + okuyucu paket

**Files:**
- Create: `migrations/0069_panel_root_girisi.sql`
- Create: `internal/panelbayrak/rootgirisi.go`
- Test: `internal/panelbayrak/rootgirisi_test.go`

**Interfaces:**
- Consumes: yok (ilk task)
- Produces: `panelbayrak.RootGirisiAcik(ctx context.Context, db *sql.DB) bool` — Task 2, 3 ve 4 bunu çağırır.

- [ ] **Step 1: Migration dosyasını yaz**

`migrations/0069_panel_root_girisi.sql`:

```sql
-- 0069 — Panelin root/shadow giriş yolu bayrağı.
--
-- Panel girişi ilk sürümden beri sunucunun root parolasıdır: internal/auth/
-- handlers.go içindeki rootShadowHash() doğrudan /etc/shadow'u okur. Bu bayrak
-- o yolu kapatılabilir hale getirir; birincil giriş users tablosundaki gerçek
-- admin hesabına taşınır.
--
-- VARSAYILAN 1 (AÇIK) — bilinçli. Migration mevcut kurulumlarda çalıştığında
-- hiçbir davranış değişmez; root ile giren operatör kilitlenmez. Bayrağı 0'a
-- çeken tek yer sanalcp-install.sh'tir (yeni kurulumlar). Mevcut kurulumlar
-- admin hesabını oluşturduktan sonra Panel Ayarları'ndan kendileri kapatır.
--
-- SSH root erişimi bu bayraktan ETKİLENMEZ; yalnız :8443 panel girişi konudur.
ALTER TABLE panel_ayarlari
  ADD COLUMN IF NOT EXISTS root_girisi_acik TINYINT(1) NOT NULL DEFAULT 1;
```

- [ ] **Step 2: Başarısız testi yaz**

`internal/panelbayrak/rootgirisi_test.go`:

```go
package panelbayrak

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const sorgu = `SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`

func TestRootGirisiAcik_BayrakBirIseAcik(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(sorgu).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(1))

	if !RootGirisiAcik(context.Background(), db) {
		t.Fatal("bayrak 1 iken kapalı raporlandı")
	}
}

func TestRootGirisiAcik_BayrakSifirIseKapali(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(sorgu).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(0))

	if RootGirisiAcik(context.Background(), db) {
		t.Fatal("bayrak 0 iken açık raporlandı")
	}
}

// FAIL-CLOSED: DB okunamıyorsa root girişi REDDEDİLİR. Fail-open olsaydı,
// DB'yi bozabilen bir saldırgan root giriş yolunu kendiliğinden açardı.
func TestRootGirisiAcik_DBHatasindaKapali(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(sorgu).WillReturnError(errors.New("bağlantı koptu"))

	if RootGirisiAcik(context.Background(), db) {
		t.Fatal("DB hatasında açık raporlandı (fail-open)")
	}
}

// panel_ayarlari satırı hiç yoksa (bozuk/yarım kurulum) da kapalı.
func TestRootGirisiAcik_SatirYoksaKapali(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(sorgu).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}))

	if RootGirisiAcik(context.Background(), db) {
		t.Fatal("satır yokken açık raporlandı")
	}
}
```

- [ ] **Step 3: Testi çalıştır, başarısız olduğunu doğrula**

Run: `go test ./internal/panelbayrak/ -v`
Expected: FAIL — paket derlenmez, `undefined: RootGirisiAcik`

- [ ] **Step 4: Okuyucuyu yaz**

`internal/panelbayrak/rootgirisi.go`:

```go
// Package panelbayrak: panel_ayarlari tablosundaki davranış bayraklarının
// salt-okunur erişimi.
//
// NEDEN AYRI PAKET: bu bayrağı üç yer okuyor — internal/auth (giriş kapısı),
// internal/users (son-admin sayımı) ve internal/panelayarlari (HTTP uçları).
// Okuyucu panelayarlari'nda dursaydı auth ve users o paketi import etmek
// zorunda kalırdı; panelayarlari ise provisioner'ı çekiyor (Let's Encrypt,
// os/exec) — auth gibi bir kimlik doğrulama paketine taşınacak bir yük değil,
// ayrıca provisioner ileride auth'a ihtiyaç duyarsa import cycle olurdu.
// Bu paketin iç bağımlılığı SIFIR ve öyle kalmalı.
package panelbayrak

import (
	"context"
	"database/sql"
)

// RootGirisiAcik — panelin root/shadow giriş yolu açık mı?
//
// FAIL-CLOSED: bayrak okunamıyorsa false (root reddedilir). Kaybedilen bir şey
// yok — panel_ayarlari okunamıyorsa users tablosu da okunamaz, yani alternatif
// giriş yolu zaten çalışmıyordur. Aynı handler'daki 2FA durumu okumasının
// fail-closed kararıyla tutarlı (bkz. internal/auth/handlers.go Login).
func RootGirisiAcik(ctx context.Context, db *sql.DB) bool {
	var acik int
	if err := db.QueryRowContext(ctx,
		`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).Scan(&acik); err != nil {
		return false
	}
	return acik == 1
}
```

- [ ] **Step 5: Testleri çalıştır, geçtiğini doğrula**

Run: `go test ./internal/panelbayrak/ -v`
Expected: PASS — dört test de geçer

- [ ] **Step 6: Commit**

```bash
git add migrations/0069_panel_root_girisi.sql internal/panelbayrak/
git commit -m "feat(auth): root_girisi_acik bayrağı + fail-closed okuyucu"
```

---

### Task 2: Giriş kapısı — Login root dalı

**Files:**
- Modify: `internal/auth/handlers.go:181-189`
- Test: `internal/auth/rootgirisi_kapisi_test.go` (yeni dosya)

**Interfaces:**
- Consumes: `panelbayrak.RootGirisiAcik(ctx, db) bool` (Task 1)
- Produces: `var rootParolaDogrulaFn = rootParolaDogrula` — paket içi, testlerin `/etc/shadow`'a bağımlı olmadan kapıyı doğrulamasını sağlar. Başka task tüketmez.

- [ ] **Step 1: Başarısız testi yaz**

`internal/auth/rootgirisi_kapisi_test.go`:

```go
package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Kapı, parola DOĞRU olsa bile kapatmalı. Doğrulayıcı burada bilerek
// "her parola doğru" diye stub'lanıyor: test /etc/shadow'a bağımlı olmamalı,
// ve asıl iddia "yanlış parola reddedildi" değil, "bayrak kapalıyken DOĞRU
// parola bile reddedildi".
func TestLogin_RootGirisiKapaliykenDogruParolaBileReddedilir(t *testing.T) {
	eski := rootParolaDogrulaFn
	rootParolaDogrulaFn = func(string) bool { return true }
	defer func() { rootParolaDogrulaFn = eski }()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(0))
	// Reddedilen deneme audit_log'a YAZILMALI — kapalı bayrak, kaba-kuvvet
	// denemelerini görünmez yapmamalı.
	mock.ExpectExec(`INSERT INTO audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	h := &Handlers{DB: db, Secret: []byte("test"), LifetimeSec: 3600}
	govde := strings.NewReader(`{"kullanici":"root","parola":"dogru-parola"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", govde)
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("beklenen 401, gelen %d", w.Code)
	}
	// Mesaj hatalı-parola yanıtıyla BİREBİR aynı olmalı: root girişinin
	// kapalı olduğu sızarsa saldırgan hangi sunucuda root yolunun açık
	// olduğunu tarayarak bulabilir.
	var yanit map[string]any
	if err := json.NewDecoder(w.Body).Decode(&yanit); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	mesaj, _ := yanit["hata"].(string)
	if mesaj != "kullanıcı adı veya parola hatalı" {
		t.Fatalf("yanıt root girişinin kapalı olduğunu sızdırıyor: %q", mesaj)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}

// Bayrak açıkken doğrulayıcı ÇAĞRILMALI — kapı eski davranışı bozmamalı.
func TestLogin_RootGirisiAcikkenDogrulayiciCagrilir(t *testing.T) {
	cagrildi := false
	eski := rootParolaDogrulaFn
	rootParolaDogrulaFn = func(string) bool { cagrildi = true; return false }
	defer func() { rootParolaDogrulaFn = eski }()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	h := &Handlers{DB: db, Secret: []byte("test"), LifetimeSec: 3600}
	govde := strings.NewReader(`{"kullanici":"root","parola":"yanlis"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", govde)
	w := httptest.NewRecorder()

	h.Login(w, req)

	if !cagrildi {
		t.Fatal("bayrak açıkken parola doğrulayıcı hiç çağrılmadı")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("beklenen 401, gelen %d", w.Code)
	}
}
```

> **Doğrulandı:** `httpx.WriteError` gövdeyi `httpx.ErrorBody` ile yazar ve alan etiketi `json:"hata"`'dır (`internal/httpx/httpx.go:18-24`). Testteki `yanit["hata"]` anahtarı doğrudur.

- [ ] **Step 2: Testi çalıştır, başarısız olduğunu doğrula**

Run: `go test ./internal/auth/ -run TestLogin_RootGirisi -v`
Expected: FAIL — `undefined: rootParolaDogrulaFn`

- [ ] **Step 3: Doğrulayıcıyı değiştirilebilir yap**

`internal/auth/handlers.go` içinde, `rootParolaDogrula` fonksiyonunun hemen altına:

```go
// rootParolaDogrulaFn — rootParolaDogrula'nın test edilebilir sarmalayıcısı.
// Gerçek doğrulama /etc/shadow'u okur; testler bu değişkeni geçici olarak
// değiştirerek Login'in bayrak kapısını dosya sistemine bağımlı olmadan
// doğrular. Üretimde ASLA değiştirilmez.
var rootParolaDogrulaFn = rootParolaDogrula
```

- [ ] **Step 4: Kapıyı Login'e bağla**

`internal/auth/handlers.go:181-186` bloğunu şununla değiştir:

```go
	if KullaniciRootMu(req.Kullanici) {
		// Root/shadow yolu artık BAYRAKLI (bkz. migrations/0069). Kapalıysa
		// parola DOĞRU olsa bile reddedilir — panel girişi sunucunun root
		// parolasıyla eşleşmeyi bıraksın diye. Kısa devre bilinçli: bayrak
		// kapalıyken /etc/shadow hiç okunmaz.
		//
		// Yanıt hatalı-parola dalıyla BİREBİR aynı: root yolunun kapalı
		// olduğu sızarsa, saldırgan hangi sunucularda bu yolun hâlâ açık
		// olduğunu tarayarak ayıklayabilirdi. 401 döndüğü için deneme
		// middleware.GirisLimiti sayacına da girer.
		if !panelbayrak.RootGirisiAcik(r.Context(), h.DB) || !rootParolaDogrulaFn(req.Parola) {
			WriteAudit(h.DB, 0, req.Kullanici, ip, "auth.login", req.Kullanici, false)
			httpx.WriteError(w, http.StatusUnauthorized, "kullanıcı adı veya parola hatalı")
			return
		}
		uid, kadi, rol = 1, "root", "admin"
		_ = h.DB.QueryRow(`SELECT full_name FROM users WHERE id=1`).Scan(&adSoyad)
	} else {
```

Import bloğuna ekle: `"sanalcp/internal/panelbayrak"`

- [ ] **Step 5: Testleri çalıştır, geçtiğini doğrula**

Run: `go test ./internal/auth/ -v`
Expected: PASS — yeni iki test geçer, mevcut auth testleri (`apitoken_test.go`, `legacycrypt_test.go`, `parola_test.go`, `qr_test.go`) bozulmaz

- [ ] **Step 6: Commit**

```bash
git add internal/auth/handlers.go internal/auth/rootgirisi_kapisi_test.go
git commit -m "feat(auth): root giriş yolunu bayrağa bağla, kapalıyken sızdırmadan reddet"
```

---

### Task 3: Son-admin sayımı root satırını dışlasın

**Files:**
- Modify: `internal/users/handlers.go:347-355`
- Test: `internal/users/sonadmin_test.go` (yeni dosya)

**Interfaces:**
- Consumes: `panelbayrak.RootGirisiAcik(ctx, db) bool` (Task 1)
- Produces: yok (mevcut `sonAdminMi` imzası değişmez: `func (h *Handlers) sonAdminMi(r *http.Request, id int64) (bool, error)`)

**Arka plan:** `sonAdminMi` zaten var ve üç handler'a bağlı — `Sil` (`:646`), `DurumDegistir` (`:416`), `Guncelle` (`:317`). Eklenecek koruma yok; düzeltilecek **sayım** var. Bugünkü sorgu `users` tablosundaki root satırını (`id=1`, `role='admin'`, `status='active'`) sayıyor. Root girişi kapalıyken o satırla panele girilemez, yani kurtarma değeri yoktur — sayıldığı için son gerçek admin silinebilir ve panele kimse giremez hale gelir.

- [ ] **Step 1: Başarısız testi yaz**

`internal/users/sonadmin_test.go`:

```go
package users

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Root girişi KAPALI: sayım root satırını dışlamalı. Dışlamazsa, geriye tek
// gerçek admin kaldığında koruma devreye girmez, o admin silinir ve panele
// kimse giremez.
func TestSonAdminMi_RootKapaliykenRootSayilmaz(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(0))
	// id<>? İKİ KEZ geçmeli: biri silinen hesap, biri root satırı.
	mock.ExpectQuery(`role='admin' AND status='active' AND id<>\? AND id<>\?`).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))

	h := &Handlers{DB: db}
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/users/7", nil)

	yalniz, err := h.sonAdminMi(r, 7)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if !yalniz {
		t.Fatal("root kapalıyken son gerçek admin korunmadı")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}

// Root girişi AÇIK: mevcut davranış birebir korunmalı — root gerçekten
// kullanılabilir bir kurtarma yolu olduğu için sayıma dahildir.
func TestSonAdminMi_RootAcikkenRootSayilir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(1))
	mock.ExpectQuery(`role='admin' AND status='active' AND id<>\?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))

	h := &Handlers{DB: db}
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/users/7", nil)

	yalniz, err := h.sonAdminMi(r, 7)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if yalniz {
		t.Fatal("root açıkken root sayılmadı, davranış değişti")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}
```

> **Doğrulandı:** `internal/users/handlers.go:23-25` — `type Handlers struct { DB *sql.DB }`. Başka zorunlu alan yok, `&Handlers{DB: db}` yeterli.

- [ ] **Step 2: Testi çalıştır, başarısız olduğunu doğrula**

Run: `go test ./internal/users/ -run TestSonAdminMi -v`
Expected: FAIL — `TestSonAdminMi_RootKapaliykenRootSayilmaz` düşer; mevcut sorgu tek `id<>?` içeriyor ve `panel_ayarlari` hiç sorgulanmıyor

- [ ] **Step 3: Sayımı düzelt**

`internal/users/handlers.go:347-355` bloğunu şununla değiştir:

```go
func (h *Handlers) sonAdminMi(r *http.Request, id int64) (bool, error) {
	// users tablosundaki root satırı (id=1) role='admin' + status='active'
	// olduğu için bu sayıma doğal olarak dahil. Ama root/shadow girişi
	// KAPALIYKEN o satırla panele GİRİLEMEZ — kurtarma değeri yoktur.
	// Sayılsaydı sistemdeki son gerçek admin silinebilir, ardından panele
	// kimse giremezdi (root da giremez). Bu yüzden bayrak kapalıyken root
	// dışlanır; açıkken mevcut davranış birebir korunur.
	sorgu := `SELECT COUNT(*) FROM users WHERE role='admin' AND status='active' AND id<>?`
	args := []any{id}
	if !panelbayrak.RootGirisiAcik(r.Context(), h.DB) {
		sorgu += ` AND id<>?`
		args = append(args, rootID)
	}
	var n int
	if err := h.DB.QueryRowContext(r.Context(), sorgu, args...).Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}
```

Import bloğuna ekle: `"sanalcp/internal/panelbayrak"`

- [ ] **Step 4: Testleri çalıştır, geçtiğini doğrula**

Run: `go test ./internal/users/ -v`
Expected: PASS — iki yeni test geçer, mevcut users testleri bozulmaz

- [ ] **Step 5: Commit**

```bash
git add internal/users/handlers.go internal/users/sonadmin_test.go
git commit -m "fix(users): root girişi kapalıyken son-admin sayımı root satırını dışlasın"
```

---

### Task 4: Ayarlar uçları — oku / yaz

**Files:**
- Create: `internal/panelayarlari/rootgirisi.go`
- Modify: `cmd/server/main.go:377-378` civarı (rota kaydı)
- Test: `internal/panelayarlari/rootgirisi_test.go`

**Interfaces:**
- Consumes: `panelbayrak.RootGirisiAcik(ctx, db) bool` (Task 1)
- Produces: `(*panelayarlari.Handlers).RootGirisi` ve `(*panelayarlari.Handlers).RootGirisiKaydet` — Task 6 (frontend) `GET/PUT /api/v1/system/root-girisi` uçlarını tüketir. İstek gövdesi `{"acik": bool}`, yanıt `{"acik": bool}`.

- [ ] **Step 1: Başarısız testi yaz**

`internal/panelayarlari/rootgirisi_test.go`:

```go
package panelayarlari

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Kilitlenme koruması: root girişi kapatılırken sistemde aktif ve root
// OLMAYAN bir admin yoksa istek reddedilmeli. Aksi halde tek adımda kendini
// dışarı kilitlemek mümkün olurdu — root kapanır, girebilecek hesap kalmaz.
func TestRootGirisiKaydet_BaskaAdminYokkenKapatilamaz(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`role='admin' AND status='active' AND id<>1`).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))

	h := &Handlers{DB: db}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/root-girisi",
		strings.NewReader(`{"acik":false}`))
	w := httptest.NewRecorder()

	h.RootGirisiKaydet(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("beklenen 400, gelen %d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}

func TestRootGirisiKaydet_BaskaAdminVarkenKapatilir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`role='admin' AND status='active' AND id<>1`).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectExec(`UPDATE panel_ayarlari SET root_girisi_acik=\?`).
		WithArgs(0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	h := &Handlers{DB: db}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/root-girisi",
		strings.NewReader(`{"acik":false}`))
	w := httptest.NewRecorder()

	h.RootGirisiKaydet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("beklenen 200, gelen %d", w.Code)
	}
	var yanit struct {
		Acik bool `json:"acik"`
	}
	if err := json.NewDecoder(w.Body).Decode(&yanit); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	if yanit.Acik {
		t.Fatal("kapatma sonrası açık raporlandı")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}

// AÇMA yönü admin sayımı gerektirmez — kilitlenme riski yaratmaz.
func TestRootGirisiKaydet_AcmaAdminSaymaz(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectExec(`UPDATE panel_ayarlari SET root_girisi_acik=\?`).
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	h := &Handlers{DB: db}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/root-girisi",
		strings.NewReader(`{"acik":true}`))
	w := httptest.NewRecorder()

	h.RootGirisiKaydet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("beklenen 200, gelen %d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}

func TestRootGirisi_DurumOkur(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(1))

	h := &Handlers{DB: db}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/root-girisi", nil)
	w := httptest.NewRecorder()

	h.RootGirisi(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("beklenen 200, gelen %d", w.Code)
	}
	var yanit struct {
		Acik bool `json:"acik"`
	}
	if err := json.NewDecoder(w.Body).Decode(&yanit); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	if !yanit.Acik {
		t.Fatal("bayrak 1 iken kapalı raporlandı")
	}
}
```

- [ ] **Step 2: Testi çalıştır, başarısız olduğunu doğrula**

Run: `go test ./internal/panelayarlari/ -run TestRootGirisi -v`
Expected: FAIL — `h.RootGirisi` / `h.RootGirisiKaydet` tanımlı değil

- [ ] **Step 3: Uçları yaz**

`internal/panelayarlari/rootgirisi.go`:

```go
package panelayarlari

// Panelin root/shadow giriş yolu anahtarı (bkz. migrations/0069).
//
// Bu bayrak YALNIZ :8443 panel girişini etkiler. Sunucunun SSH root erişimi,
// root parolası ve sshd yapılandırması bundan HİÇ etkilenmez.

import (
	"encoding/json"
	"net/http"

	"sanalcp/internal/httpx"
	"sanalcp/internal/panelbayrak"
)

// RootGirisi — GET /api/v1/system/root-girisi (AdminOnly).
func (h *Handlers) RootGirisi(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"acik": panelbayrak.RootGirisiAcik(r.Context(), h.DB),
	})
}

type rootGirisiKaydetReq struct {
	Acik bool `json:"acik"`
}

// RootGirisiKaydet — PUT /api/v1/system/root-girisi (AdminOnly).
//
// KİLİTLENME KORUMASI: kapatma yönünde, sistemde aktif ve root OLMAYAN bir
// admin hesabı yoksa istek reddedilir. Aksi halde tek istekle kendini dışarı
// kilitlemek mümkün olurdu — root kapanır, girebilecek başka hesap yoktur ve
// geri açacak oturum da açılamaz. Açma yönünde böyle bir risk yok, sayım
// yapılmaz.
//
// Bu kontrol internal/users.sonAdminMi'den BAĞIMSIZDIR: o, hesap silmeyi/
// pasifleştirmeyi korur; bu, bayrağın kendisini kapatmayı korur. İkisi de
// gerekli — biri olmadan diğeri kilitlenmeyi tek adımda mümkün bırakır.
func (h *Handlers) RootGirisiKaydet(w http.ResponseWriter, r *http.Request) {
	var req rootGirisiKaydetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}

	if !req.Acik {
		var n int
		if err := h.DB.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM users WHERE role='admin' AND status='active' AND id<>1`).
			Scan(&n); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "admin sayısı okunamadı")
			return
		}
		if n == 0 {
			httpx.WriteError(w, http.StatusBadRequest,
				"root girişi kapatılamaz: önce root dışında aktif bir yönetici hesabı oluşturun")
			return
		}
	}

	deger := 0
	if req.Acik {
		deger = 1
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE panel_ayarlari SET root_girisi_acik=? WHERE id=1`, deger); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "acik": req.Acik})
}
```

- [ ] **Step 4: Rotaları kaydet**

`cmd/server/main.go`, `oturum-bosta` satırlarının (`:377-378`) hemen altına:

```go
			// Panelin root/shadow giriş yolu anahtarı (bkz. migrations/0069).
			// SSH root erişimini ETKİLEMEZ — yalnız :8443 panel girişi.
			r.With(middleware.AdminOnly).Get("/system/root-girisi", panelAyarH.RootGirisi)
			r.With(middleware.AdminOnly).Put("/system/root-girisi", panelAyarH.RootGirisiKaydet)
```

- [ ] **Step 5: Testleri ve derlemeyi çalıştır**

Run: `go build ./... && go test ./internal/panelayarlari/ -v`
Expected: BUILD OK, dört test de PASS

- [ ] **Step 6: Commit**

```bash
git add internal/panelayarlari/rootgirisi.go internal/panelayarlari/rootgirisi_test.go cmd/server/main.go
git commit -m "feat(panel): root girişi anahtarı uçları + kilitlenme koruması"
```

---

### Task 5: Installer — gerçek admin hesabı üret

**Files:**
- Modify: `sanalcp-install.sh` (argüman çözümleme bloğu ~satır 26-32, adım 13 ~satır 915-930)

**Interfaces:**
- Consumes: `migrations/0069_panel_root_girisi.sql` (Task 1) — `root_girisi_acik` sütunu var olmalı
- Produces: yok (kabuk betiği)

- [ ] **Step 1: Yeni argümanı çözümleme bloğuna ekle**

`sanalcp-install.sh` içinde `ADMIN_PAROLA=""; ADMIN_EPOSTA="admin@local"; PANEL_LANG=""` satırını şununla değiştir:

```bash
ADMIN_PAROLA=""; ADMIN_EPOSTA="admin@local"; PANEL_LANG=""; ADMIN_KULLANICI="admin"
```

`while [ $# -gt 0 ]; do case "$1" in` bloğuna `--admin-parola` satırının yanına ekle:

```bash
  --admin-kullanici) shift; ADMIN_KULLANICI="$1" ;;
```

Dosya başındaki kullanım satırını da güncelle:

```bash
#   ./sanalcp-install.sh [--admin-kullanici <k>] [--admin-parola <p>] [--admin-eposta <e>] [--lang tr|en]
```

- [ ] **Step 2: Adım 13'ü genişlet**

`sanalcp-install.sh` adım 13'te, `ok "Login: username 'root' + this server's root password"` satırını şu blokla değiştir:

```bash
# Gerçek admin hesabı — panelin BİRİNCİL giriş yolu artık bu (bkz.
# docs/superpowers/specs/2026-08-20-panel-auth-root-ayirma-design.md).
# Yukarıdaki root tohumlaması yerinde kalıyor: root/shadow yolu Panel
# Ayarları'ndan tekrar açılabilir ve o yol users.id=1 satırına bağlı.
if [ -z "$ADMIN_PAROLA" ]; then
  ADMIN_PAROLA=$(openssl rand -base64 18 | tr -d '/+=' | cut -c1-20)
fi
if [ -x /opt/sanalcp/bin/sanalcp-seed-admin ]; then
  /opt/sanalcp/bin/sanalcp-seed-admin -dsn "$DSN" -kullanici "$ADMIN_KULLANICI" \
    -parola "$ADMIN_PAROLA" -eposta "$ADMIN_EPOSTA" -dil "$PANEL_LANG" >/dev/null 2>&1 \
    && ok "admin account created" || die "admin account could not be created"
fi

# Root/shadow giriş yolunu KAPAT. Migration bunu 1 (açık) olarak ekliyor ki
# mevcut kurulumlar kilitlenmesin; yeni kurulumda ise girecek gerçek bir admin
# zaten var, o yüzden kapalı başlıyoruz.
mysql panel -e "UPDATE panel_ayarlari SET root_girisi_acik=0 WHERE id=1;" >/dev/null 2>&1 \
  && ok "panel root login disabled (SSH root access is unaffected)" \
  || warn "could not disable panel root login"

echo
echo "  ╔══════════════════════════════════════════════════════════════╗"
echo "  ║  PANEL LOGIN — bu parola BİR KEZ gösterilir, kaydedin        ║"
echo "  ╚══════════════════════════════════════════════════════════════╝"
echo "    kullanıcı : $ADMIN_KULLANICI"
echo "    parola    : $ADMIN_PAROLA"
echo
echo "  Bu parola hiçbir dosyaya yazılmadı. Kaybederseniz SSH ile:"
echo "    /opt/sanalcp/bin/sanalcp-seed-admin -dsn '<DSN>' \\"
echo "      -kullanici $ADMIN_KULLANICI -parola '<yeni-parola>'"
echo
```

- [ ] **Step 3: Sözdizimi ve parite testlerini çalıştır**

Run: `bash -n sanalcp-install.sh && go test ./internal/osfam/ -v`
Expected: sözdizimi hatasız; `TestPipefailBetiklerindeGrepQBoruHattiYok` ve installer paritesi testleri PASS

- [ ] **Step 4: Argüman çözümlemesini elle doğrula**

Run:
```bash
bash -c 'set -- --admin-kullanici yusuf --lang tr
ADMIN_KULLANICI="admin"; PANEL_LANG=""
while [ $# -gt 0 ]; do case "$1" in
  --admin-kullanici) shift; ADMIN_KULLANICI="$1" ;;
  --lang) shift; PANEL_LANG="$1" ;;
esac; shift; done
echo "kullanici=$ADMIN_KULLANICI lang=$PANEL_LANG"'
```
Expected: `kullanici=yusuf lang=tr`

- [ ] **Step 5: Commit**

```bash
git add sanalcp-install.sh
git commit -m "feat(install): gerçek admin hesabı üret, panel root girişini kapat"
```

---

### Task 6: Frontend — root girişi anahtarı

**Files:**
- Create: `frontend/src/components/RootGirisiAyari.tsx`
- Create: `frontend/src/i18n/locales/tr/RootGirisiAyari.json`
- Create: `frontend/src/i18n/locales/en/RootGirisiAyari.json`
- Modify: `frontend/src/pages/AraclarAyarlarPage.tsx:226` civarı

**Interfaces:**
- Consumes: `GET/PUT /api/v1/system/root-girisi` (Task 4) — yanıt `{"acik": bool}`, istek `{"acik": bool}`
- Produces: yok

- [ ] **Step 1: i18n dosyalarını yaz**

`frontend/src/i18n/locales/tr/RootGirisiAyari.json`:

```json
{
  "label": "Sunucu root parolasıyla panel girişi",
  "desc": "Açıkken panele 'root' kullanıcı adı ve sunucunun root parolasıyla girilebilir. Kapatmanız önerilir: SSH root parolanızı değiştirmek panel girişini etkilemez ve güvenlik günlüğünde kimin giriş yaptığı ayırt edilebilir. SSH root erişiminiz bu ayardan etkilenmez.",
  "on": "Açık",
  "off": "Kapalı",
  "save": "Kaydet",
  "saving": "Kaydediliyor…",
  "saved_on": "Root ile panel girişi açıldı.",
  "saved_off": "Root ile panel girişi kapatıldı.",
  "load_failed": "Ayar okunamadı.",
  "save_failed": "Ayar kaydedilemedi."
}
```

`frontend/src/i18n/locales/en/RootGirisiAyari.json`:

```json
{
  "label": "Panel login with the server root password",
  "desc": "When enabled, the panel can be accessed with the username 'root' and the server's root password. Disabling is recommended: changing your SSH root password will no longer affect panel login, and the audit log can tell operators apart. Your SSH root access is unaffected by this setting.",
  "on": "Enabled",
  "off": "Disabled",
  "save": "Save",
  "saving": "Saving…",
  "saved_on": "Panel login with root enabled.",
  "saved_off": "Panel login with root disabled.",
  "load_failed": "Could not read the setting.",
  "save_failed": "Could not save the setting."
}
```

- [ ] **Step 2: Bileşeni yaz**

`frontend/src/components/RootGirisiAyari.tsx`:

```tsx
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'

type Durum = { acik: boolean }

export default function RootGirisiAyari() {
  const { t } = useTranslation(['RootGirisiAyari'])
  const [acik, setAcik] = useState(true)
  const [kayitli, setKayitli] = useState(true)
  const [kaydediliyor, setKaydediliyor] = useState(false)
  const [hata, setHata] = useState('')
  const [basari, setBasari] = useState('')

  const yukle = useCallback(async () => {
    try {
      const { data } = await api.get<Durum>('/system/root-girisi')
      setAcik(data.acik)
      setKayitli(data.acik)
    } catch (e) {
      setHata(apiHata(e, t('RootGirisiAyari:load_failed')))
    }
  }, [t])

  useEffect(() => { void yukle() }, [yukle])

  async function kaydet() {
    setHata('')
    setBasari('')
    setKaydediliyor(true)
    try {
      const { data } = await api.put<Durum>('/system/root-girisi', { acik })
      setKayitli(data.acik)
      setAcik(data.acik)
      setBasari(data.acik
        ? t('RootGirisiAyari:saved_on')
        : t('RootGirisiAyari:saved_off'))
    } catch (e) {
      // Sunucu, root dışında aktif admin yokken kapatmayı reddeder —
      // mesajı olduğu gibi göster, kullanıcı ne yapması gerektiğini öğrensin.
      setHata(apiHata(e, t('RootGirisiAyari:save_failed')))
      setAcik(kayitli)
    } finally {
      setKaydediliyor(false)
    }
  }

  const degisti = acik !== kayitli

  return (
    <div className="rounded-2xl border border-amber-200 bg-amber-50 p-4 dark:border-amber-800/50 dark:bg-amber-900/15">
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} className="h-5 w-5" aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 1 0-9 0v3.75M6.75 10.5h10.5a2.25 2.25 0 0 1 2.25 2.25v6a2.25 2.25 0 0 1-2.25 2.25H6.75a2.25 2.25 0 0 1-2.25-2.25v-6a2.25 2.25 0 0 1 2.25-2.25Z" />
          </svg>
        </div>
        <div className="min-w-0 flex-1">
          <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('RootGirisiAyari:label')}</span>
          <p className="mt-1 text-xs leading-relaxed text-slate-500 dark:text-slate-400">
            {t('RootGirisiAyari:desc')}
          </p>

          {hata && <div className="mt-2 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">{hata}</div>}
          {basari && <div className="mt-2 rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{basari}</div>}

          <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center">
            <label className="flex items-center gap-2 text-xs text-slate-700 dark:text-slate-300">
              <input
                type="checkbox"
                checked={acik}
                onChange={(e) => setAcik(e.target.checked)}
                className="h-4 w-4 rounded border-slate-300 text-amber-600 focus:ring-amber-500"
              />
              {acik ? t('RootGirisiAyari:on') : t('RootGirisiAyari:off')}
            </label>
            <button
              type="button"
              onClick={() => void kaydet()}
              disabled={!degisti || kaydediliyor}
              className="rounded-lg bg-amber-600 px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50"
            >
              {kaydediliyor ? t('RootGirisiAyari:saving') : t('RootGirisiAyari:save')}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 3: Ayarlar sayfasına bağla**

`frontend/src/pages/AraclarAyarlarPage.tsx` içinde `<OturumBostaAyari />` satırının hemen altına ekle:

```tsx
          <RootGirisiAyari />
```

Dosyanın import bloğuna, `OturumBostaAyari` importunun yanına:

```tsx
import RootGirisiAyari from '@/components/RootGirisiAyari'
```

- [ ] **Step 4: Frontend derlemesini doğrula**

Run: `cd frontend && npm run build`
Expected: derleme hatasız tamamlanır

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/RootGirisiAyari.tsx frontend/src/i18n/locales/tr/RootGirisiAyari.json frontend/src/i18n/locales/en/RootGirisiAyari.json frontend/src/pages/AraclarAyarlarPage.tsx
git commit -m "feat(frontend): panel root girişi anahtarı"
```

---

### Task 7: Uçtan uca doğrulama

**Files:**
- Değişiklik yok — yalnız doğrulama

**Interfaces:**
- Consumes: Task 1-6'nın tamamı
- Produces: yok

- [ ] **Step 1: Tüm testleri ve derlemeyi çalıştır**

Run: `go build ./... && go test ./... && bash -n sanalcp-install.sh && bash -n assets/ops/sanalcp-update`
Expected: hepsi başarılı

- [ ] **Step 2: Migration'ı test kurulumunda uygula ve varsayılanı doğrula**

Test sunucusunda (üretimde DEĞİL) panel yeniden başlatıldıktan sonra:

```bash
mysql -N -e "SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1;" panel
```
Expected: `1` — migration mevcut kurulumda bayrağı açık bırakır, giriş davranışı değişmez

- [ ] **Step 3: Kapatma korumasını doğrula**

Root dışında aktif admin YOKKEN:

```bash
curl -sk -X PUT -H "Authorization: Bearer <admin-jwt>" -H 'Content-Type: application/json' \
  --data '{"acik":false}' https://127.0.0.1:8443/api/v1/system/root-girisi
```
Expected: `400` + `"root girişi kapatılamaz: önce root dışında aktif bir yönetici hesabı oluşturun"`

- [ ] **Step 4: Kapalı bayrakla root girişinin reddedildiğini doğrula**

Bir admin hesabı oluşturduktan ve bayrağı kapattıktan sonra, DOĞRU root parolasıyla:

```bash
curl -sk -X POST -H 'Content-Type: application/json' \
  --data '{"kullanici":"root","parola":"<gerçek-root-parolası>"}' \
  https://127.0.0.1:8443/api/v1/auth/login
```
Expected: `401` + `"kullanıcı adı veya parola hatalı"` — mesaj hatalı parolayla birebir aynı

- [ ] **Step 5: SSH root erişiminin bozulmadığını doğrula**

Run: `ssh -p 7346 root@<sunucu> 'echo SSH-OK'`
Expected: `SSH-OK` — panel bayrağı SSH'ı etkilemez

- [ ] **Step 6: audit_log'da reddedilen denemenin göründüğünü doğrula**

```bash
mysql -t -e "SELECT ts, actor_username, ip, action, ok FROM audit_log WHERE action='auth.login' ORDER BY id DESC LIMIT 5;" panel
```
Expected: Step 4'teki denemenin `ok=0` satırı görünür

- [ ] **Step 7: Commit (varsa düzeltmeler)**

```bash
git add -A
git commit -m "test: uçtan uca doğrulama düzeltmeleri"
```

---

## Uygulama Sonrası

Bu dal (`auth-root-ayirma`) `main`'e birleştirilince tüm SanalCP kullanıcılarına yayın çıkar. Birleştirmeden önce:

1. Üretim sunucusunda (`cloud.sanalcp.com`) güncelleme yapılır, bayrağın `1` kaldığı ve root girişinin çalışmaya devam ettiği doğrulanır.
2. Panel Ayarları'ndan admin hesabı oluşturulur, o hesapla giriş doğrulanır.
3. Root girişi kapatılır, tekrar giriş doğrulanır.
4. Aynı üç adım ikinci kuruluma anlatılır.
