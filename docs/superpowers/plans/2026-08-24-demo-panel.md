# demo.sanalcp.com Salt-Okunur Demo Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Panel'e `demo_modu_acik` bayrağı ekleyip, açıkken tüm yazan istekleri (login/logout hariç) tek bir middleware'de engelleyen, sır döndüren birkaç uçta değeri maskeleyen, ve ayrı bir VPS'te elle tohumlanıp gecelik sıfırlanan bir demo modu kurmak.

**Architecture:** Tek global chi middleware (`DemoSaltOkunur`) yazma isteklerini beyaz liste dışında 403'ler; context'e işlediği bayrağı 3 handler okuyup sır alanlarını maskeler. Tohumlama panelin kendi HTTP API'sini kullanan ayrı bir script'tir (ikinci bir oluşturma yolu açılmaz). Reset systemd timer + SQL dump geri yüklemesidir.

**Tech Stack:** Go 1.26, chi router, MySQL/MariaDB, sqlmock (test), systemd (demo VPS).

**Spec:** `docs/superpowers/specs/2026-08-24-demo-panel-design.md`

## Global Constraints

- Yeni migration dosyası `migrations/0070_demo_modu.sql` — `0069_panel_root_girisi.sql` ile birebir aynı desen (tek satırlık `panel_ayarlari`, `ADD COLUMN IF NOT EXISTS`, varsayılan 0).
- `demo_modu_acik` varsayılan **0** — mevcut/üretim kurulumlarında davranış birebir korunur.
- **Fail-safe yönü spec'ten DÜZELTİLDİ (bu planda netleşti):** spec "DB okunamazsa demo modu AÇIK sayılsın" diyordu. Bu, bayrağı hiç açmamış (yani dünyadaki HER normal SanalCP kurulumunun, `demo_modu_acik=0`) bir panelde tek bir geçici DB okuma hatasını "tüm yazmaları panel genelinde 403'le" haline getirir — blast radius'u demo VPS'inin çok ötesine taşır. Bunun yerine: DB okunamazsa/nil ise **demo modu KAPALI sayılır** (normal davranışa düşülür). Demo VPS'inde bu, bir DB blip'inde geçici olarak yazmanın açılması riskini taşır ama o sunucuda zaten gerçek müşteri verisi yok — kabul edilebilir takas. `panelbayrak.DemoModuAcik` yorumunda bu gerekçe açıkça yazılacak.
- Sır maskeleme tek bir paylaşılan yardımcıdan geçer: `middleware.Maskele(demoPanel bool, deger string) string`.
- Ham bayt akışı döndüren uçlar (`files.Download`, `backups.Download`) **maskelenmez, tamamen 403'lenir** — spec'in "mask" dediği JSON alan değeri içindir; gzip/octet-stream gövdesinde kısmi maskeleme anlamsızdır. Bu, spec'in "maskele" kararının bu iki uca uygulanışının somutlaşmasıdır, kararın kendisini değiştirmez.
- Türkçe değişken/fonksiyon adlandırma ve yorum stili, mevcut kod tabanıyla birebir aynı kalır (bkz. `panelbayrak/rootgirisi.go`, `middleware/csrf.go`).

---

### Task 1: Migration — `demo_modu_acik` bayrağı

**Files:**
- Create: `migrations/0070_demo_modu.sql`

**Interfaces:**
- Produces: `panel_ayarlari.demo_modu_acik TINYINT(1) NOT NULL DEFAULT 0` sütunu — Task 2 bunu okur.

- [ ] **Step 1: Migration dosyasını yaz**

```sql
-- 0070 — Demo panel modu.
--
-- Açıkken tüm yazan istekler (auth/login, auth/cikis hariç) middleware
-- katmanında 403 ile reddedilir (bkz. internal/middleware/demo.go). Sır
-- döndüren birkaç GET ucu (DB parolası, mail parolası, dosya içeriği) değeri
-- maskeler. Sadece demo.sanalcp.com gibi ayrı, bağımsız bir VPS'te elle
-- açılır (bkz. scripts/demo_seed.go) — normal kurulumlarda hiç dokunulmaz.
--
-- VARSAYILAN 0 — mevcut kurulumlarda davranış birebir korunur.
ALTER TABLE panel_ayarlari
  ADD COLUMN IF NOT EXISTS demo_modu_acik TINYINT(1) NOT NULL DEFAULT 0;
```

- [ ] **Step 2: Migration'ın uygulandığını doğrula**

Run: `go test ./cmd/server/... -run TestMigrations -v`
Expected: PASS (mevcut `migrations_test.go` her dosyayı sözdizimi/checksum açısından tarar; yeni dosya listeye otomatik girer, ek kod gerekmez).

- [ ] **Step 3: Commit**

```bash
git add migrations/0070_demo_modu.sql
git commit -m "feat(db): demo_modu_acik bayrağı ekle (panel_ayarlari)"
```

---

### Task 2: `panelbayrak.DemoModuAcik`

**Files:**
- Create: `internal/panelbayrak/demomodu.go`
- Test: `internal/panelbayrak/demomodu_test.go`

**Interfaces:**
- Consumes: Task 1'in `panel_ayarlari.demo_modu_acik` sütunu.
- Produces: `func DemoModuAcik(ctx context.Context, db *sql.DB) bool` — Task 3'ün `DemoSaltOkunur` middleware'i bunu çağırır.

- [ ] **Step 1: Başarısız testleri yaz**

```go
package panelbayrak

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const demoSorgu = `SELECT demo_modu_acik FROM panel_ayarlari WHERE id=1`

func TestDemoModuAcik_BayrakBirIseAcik(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(demoSorgu).
		WillReturnRows(sqlmock.NewRows([]string{"demo_modu_acik"}).AddRow(1))

	if !DemoModuAcik(context.Background(), db) {
		t.Fatal("bayrak 1 iken kapalı raporlandı")
	}
}

func TestDemoModuAcik_BayrakSifirIseKapali(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(demoSorgu).
		WillReturnRows(sqlmock.NewRows([]string{"demo_modu_acik"}).AddRow(0))

	if DemoModuAcik(context.Background(), db) {
		t.Fatal("bayrak 0 iken açık raporlandı")
	}
}

// FAIL-OPEN (bilinçli, RootGirisiAcik'in TERSİ): DB okunamıyorsa demo modu
// KAPALI sayılır. Aksi (fail-closed) olsaydı, demo_modu_acik=0 olan HER
// normal kurulumda geçici bir DB hatası tüm panel genelinde yazmayı
// kilitlerdi — blast radius'u demo VPS'inin çok ötesine taşırdı.
func TestDemoModuAcik_DBHatasindaKapali(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(demoSorgu).WillReturnError(errors.New("bağlantı koptu"))

	if DemoModuAcik(context.Background(), db) {
		t.Fatal("DB hatasında açık raporlandı (fail-open ihlali)")
	}
}

func TestDemoModuAcik_NilDBKapali(t *testing.T) {
	if DemoModuAcik(context.Background(), (*sql.DB)(nil)) {
		t.Fatal("nil db'de açık raporlandı")
	}
}

func TestDemoModuAcik_SatirYoksaKapali(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(demoSorgu).
		WillReturnRows(sqlmock.NewRows([]string{"demo_modu_acik"}))

	if DemoModuAcik(context.Background(), db) {
		t.Fatal("satır yokken açık raporlandı")
	}
}
```

- [ ] **Step 2: Testlerin derlenip başarısız olduğunu doğrula**

Run: `go test ./internal/panelbayrak/... -run TestDemoModuAcik -v`
Expected: FAIL (derleme hatası — `DemoModuAcik` henüz tanımlı değil)

- [ ] **Step 3: Uygulamayı yaz**

```go
package panelbayrak

import (
	"context"
	"database/sql"
)

// DemoModuAcik — panel demo modunda mı? (bkz. migrations/0070)
//
// FAIL-OPEN (RootGirisiAcik'in TERSİ yönde): db nil ise ya da sorgu
// başarısız olursa demo modu KAPALI (normal davranış) kabul edilir. Bu
// bayrak varsayılan 0'dır ve dünyadaki neredeyse tüm kurulumlarda hiç
// açılmaz; fail-closed olsaydı (demo=AÇIK varsayımı) geçici bir DB
// hatası, hiç demo modunda olmayan bir panelde TÜM yazma isteklerini
// 403'e çevirirdi. Yalnız demo_modu_acik'ı bilinçli açan tek kurulumda
// (demo.sanalcp.com) bu fail-open, DB blip'i sırasında yazmanın kısa
// süreliğine açık kalması riskini taşır — o sunucuda gerçek müşteri
// verisi olmadığı için kabul edilebilir.
func DemoModuAcik(ctx context.Context, db *sql.DB) bool {
	if db == nil {
		return false
	}
	var acik int
	if err := db.QueryRowContext(ctx,
		`SELECT demo_modu_acik FROM panel_ayarlari WHERE id=1`).Scan(&acik); err != nil {
		return false
	}
	return acik == 1
}
```

- [ ] **Step 4: Testlerin geçtiğini doğrula**

Run: `go test ./internal/panelbayrak/... -v`
Expected: PASS (tüm `TestDemoModuAcik_*` + mevcut `TestRootGirisiAcik_*` testleri)

- [ ] **Step 5: Commit**

```bash
git add internal/panelbayrak/demomodu.go internal/panelbayrak/demomodu_test.go
git commit -m "feat(panelbayrak): DemoModuAcik okuyucusu (fail-open)"
```

---

### Task 3: `middleware.DemoSaltOkunur` + `DemoPaneliMi` + `Maskele`

**Files:**
- Create: `internal/middleware/demo.go`
- Test: `internal/middleware/demo_test.go`

**Interfaces:**
- Consumes: `panelbayrak.DemoModuAcik(ctx, db) bool` (Task 2); paket-içi `scopeDB` (zaten `middleware.Init(db)` ile set edilir, `internal/middleware/auth.go:19`); paket-içi `mockDB(t) sqlmock.Sqlmock` test yardımcısı (`internal/middleware/kapsam_test.go:25`, aynı pakette, doğrudan kullanılabilir).
- Produces:
  - `func DemoSaltOkunur(next http.Handler) http.Handler` — Task 4 `cmd/server/main.go`'da `r.Use()` ile takar.
  - `func DemoPaneliMi(r *http.Request) bool` — Task 5/6/7 handler'ları bunu okur.
  - `func DemoIle(r *http.Request, acik bool) *http.Request` — yalnız testler için (`ClaimsIle` deseniyle aynı).
  - `func Maskele(demoPanel bool, deger string) string` — Task 5/6/7 sır alanlarını bununla maskeler.

- [ ] **Step 1: Başarısız testleri yaz**

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMaskele(t *testing.T) {
	if got := Maskele(true, "gizli-parola"); got != "••••••••" {
		t.Fatalf("demoPanel=true: got %q, want maskeli", got)
	}
	if got := Maskele(false, "gizli-parola"); got != "gizli-parola" {
		t.Fatalf("demoPanel=false: got %q, want değişmemiş değer", got)
	}
}

func TestDemoIleVeDemoPaneliMi(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	if DemoPaneliMi(r) {
		t.Fatal("işaretlenmemiş istekte demo=true")
	}
	r2 := DemoIle(r, true)
	if !DemoPaneliMi(r2) {
		t.Fatal("DemoIle(true) sonrası DemoPaneliMi false")
	}
}

func TestDemoSaltOkunur_BayrakKapaliYazmaSerbest(t *testing.T) {
	mock := mockDB(t)
	mock.ExpectQuery(demoModuSorgu).
		WillReturnRows(sqlmockRows(0))

	gecti := false
	h := DemoSaltOkunur(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gecti = true }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/domains", nil))

	if !gecti || w.Code != http.StatusOK {
		t.Fatalf("bayrak kapalıyken istek engellendi: code=%d gecti=%v", w.Code, gecti)
	}
}

func TestDemoSaltOkunur_BayrakAcikGetSerbest(t *testing.T) {
	mock := mockDB(t)
	mock.ExpectQuery(demoModuSorgu).
		WillReturnRows(sqlmockRows(1))

	gecti := false
	h := DemoSaltOkunur(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gecti = true }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/domains", nil))

	if !gecti {
		t.Fatal("bayrak açıkken GET engellendi")
	}
}

func TestDemoSaltOkunur_BayrakAcikYazmaEngellenir(t *testing.T) {
	mock := mockDB(t)
	mock.ExpectQuery(demoModuSorgu).
		WillReturnRows(sqlmockRows(1))

	gecti := false
	h := DemoSaltOkunur(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gecti = true }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/domains", nil))

	if gecti || w.Code != http.StatusForbidden {
		t.Fatalf("bayrak açıkken POST geçti: code=%d gecti=%v", w.Code, gecti)
	}
}

func TestDemoSaltOkunur_BeyazListeGecer(t *testing.T) {
	for _, yol := range []string{"/api/v1/auth/login", "/api/v1/auth/cikis"} {
		mock := mockDB(t)
		mock.ExpectQuery(demoModuSorgu).
			WillReturnRows(sqlmockRows(1))

		gecti := false
		h := DemoSaltOkunur(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gecti = true }))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, yol, nil))

		if !gecti {
			t.Fatalf("%s beyaz listede olmasına rağmen engellendi: code=%d", yol, w.Code)
		}
	}
}

func TestDemoSaltOkunur_ContextTasinir(t *testing.T) {
	mock := mockDB(t)
	mock.ExpectQuery(demoModuSorgu).
		WillReturnRows(sqlmockRows(1))

	var goruldu bool
	h := DemoSaltOkunur(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		goruldu = DemoPaneliMi(r)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/domains", nil))

	if !goruldu {
		t.Fatal("bayrak açıkken context'e demo=true işlenmedi")
	}
}
```

Not: `sqlmockRows` küçük bir test yardımcısıdır, aynı dosyaya eklenir:

```go
func sqlmockRows(deger int) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"demo_modu_acik"}).AddRow(deger)
}
```

(`"github.com/DATA-DOG/go-sqlmock"` import'u test dosyasına eklenmeli.)

- [ ] **Step 2: Testlerin derlenip başarısız olduğunu doğrula**

Run: `go test ./internal/middleware/... -run 'TestMaskele|TestDemoIleVeDemoPaneliMi|TestDemoSaltOkunur' -v`
Expected: FAIL (derleme hatası — `DemoSaltOkunur`, `DemoPaneliMi`, `DemoIle`, `Maskele`, `demoModuSorgu` henüz tanımlı değil)

- [ ] **Step 3: Uygulamayı yaz**

```go
package middleware

import (
	"context"
	"net/http"

	"sanalcp/internal/httpx"
	"sanalcp/internal/panelbayrak"
)

// demoModuSorgu: panelbayrak.DemoModuAcik'in çalıştırdığı sorgu — test
// dosyasında sqlmock.ExpectQuery eşleşmesi için burada da tutulur.
const demoModuSorgu = `SELECT demo_modu_acik FROM panel_ayarlari WHERE id=1`

type demoCtxKey struct{}

// DemoPaneliMi: bu istek demo-modu AÇIK bir panelden mi geliyor?
//
// Yalnız DemoSaltOkunur zincirdeyken güvenilirdir — o middleware bayrağı
// bir kez okuyup context'e işler, sır döndüren handler'lar (bkz.
// internal/domains, internal/mail, internal/files) her istekte ikinci bir
// DB sorgusu atmadan bunu okur.
func DemoPaneliMi(r *http.Request) bool {
	v, _ := r.Context().Value(demoCtxKey{}).(bool)
	return v
}

// DemoIle: isteğe demo-modu bayrağı iliştirilmiş bir kopyasını döner.
//
// Yalnızca TESTLER için (bkz. ClaimsIle'nin üstündeki aynı uyarı) — üretimde
// bunu DemoSaltOkunur yazar.
func DemoIle(r *http.Request, acik bool) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), demoCtxKey{}, acik))
}

// Maskele: demoPanel true ise sabit bir maske döner, değilse deger'i
// olduğu gibi bırakır. Sır döndüren tüm demo-farkında handler'lar bunu
// kullanır — tek yerde tanımlı, tek görünüm.
func Maskele(demoPanel bool, deger string) string {
	if demoPanel {
		return "••••••••"
	}
	return deger
}

// demoYazmaBeyazListesi: demo modunda dahi izinli kalan TEK state-changing
// uçlar — oturum açma/kapama. Başka HİÇBİR yazma demo modunda geçmez.
var demoYazmaBeyazListesi = map[string]bool{
	"POST /api/v1/auth/login": true,
	"POST /api/v1/auth/cikis": true,
}

// DemoSaltOkunur: panel_ayarlari.demo_modu_acik açıkken tüm yazan istekleri
// (beyaz liste hariç) 403 ile reddeder. GET/HEAD/OPTIONS her zaman serbest —
// tanım gereği state değiştirmezler (CSRFKoruma'daki muafiyetle tutarlı,
// bkz. internal/middleware/csrf.go).
//
// Global zincire CSRFKoruma'dan hemen sonra, TÜM route'lardan önce eklenir
// (bkz. cmd/server/main.go) — git-webhook ve pma-redeem gibi RequireAuth
// dışındaki uçlar da dahil olmak üzere hiçbir route'a tek tek dokunulmaz.
func DemoSaltOkunur(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acik := panelbayrak.DemoModuAcik(r.Context(), scopeDB)
		if acik {
			r = r.WithContext(context.WithValue(r.Context(), demoCtxKey{}, true))
		}
		if !acik {
			next.ServeHTTP(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if demoYazmaBeyazListesi[r.Method+" "+r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		httpx.WriteError(w, http.StatusForbidden, "demo modunda değişiklik yapılamaz")
	})
}
```

- [ ] **Step 4: Testlerin geçtiğini doğrula**

Run: `go test ./internal/middleware/... -v`
Expected: PASS (yeni testler + mevcut paket testleri, `mockDB` paylaşımlı kullanım nedeniyle sıra bağımsız çalışmalı — `t.Cleanup` her testte `scopeDB`'yi eski haline döndürür)

- [ ] **Step 5: Commit**

```bash
git add internal/middleware/demo.go internal/middleware/demo_test.go
git commit -m "feat(middleware): DemoSaltOkunur yazma engeli + Maskele yardımcısı"
```

---

### Task 4: Router'a bağlama — `cmd/server/main.go`

**Files:**
- Modify: `cmd/server/main.go:304` (civarı — `r.Use(middleware.CSRFKoruma)` satırının hemen altı)

**Interfaces:**
- Consumes: `middleware.DemoSaltOkunur` (Task 3).

- [ ] **Step 1: Middleware'i zincire ekle**

`cmd/server/main.go` içinde:

```go
	r.Use(middleware.CSRFKoruma)
```

satırının hemen altına ekle:

```go
	// 🔴 GÜVENLİK: demo.sanalcp.com gibi ayrı, salt-okunur bir demo kurulumunda
	// panel_ayarlari.demo_modu_acik=1 iken TÜM yazan istekleri (login/cikis
	// hariç) engeller. Route'lardan ÖNCE eklendiği için git-webhook/pma-redeem
	// gibi RequireAuth dışındaki uçlar da kapsanır (bkz. internal/middleware/demo.go).
	// Bayrak varsayılan kapalıdır; kapalıyken bu satır no-op'tur.
	r.Use(middleware.DemoSaltOkunur)
```

- [ ] **Step 2: Derlemenin/testlerin bozulmadığını doğrula**

Run: `go build ./... && go test ./cmd/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat(server): DemoSaltOkunur'u global middleware zincirine ekle"
```

---

### Task 5: DB parolası maskeleme — `internal/domains`

**Files:**
- Modify: `internal/domains/handlers.go:914-936` (`ListDatabases`)
- Test: Create `internal/domains/demo_maskeleme_test.go`

**Interfaces:**
- Consumes: `middleware.DemoPaneliMi(r)`, `middleware.Maskele(bool, string)` (Task 3), `middleware.DemoIle(r, bool)` (test).

- [ ] **Step 1: Başarısız testi yaz**

```go
package domains

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"sanalcp/internal/middleware"
)

func TestListDatabases_DemoModundaMaskelenir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handlers{DB: db}

	mock.ExpectQuery(`SELECT id, domain_id, db_name, db_user, db_host, db_pass_plain`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "domain_id", "db_name", "db_user", "db_host", "db_pass_plain", "created_at"}).
			AddRow(1, 5, "ornek_db", "ornek_user", "localhost", "sifrelenmis-deger", "2026-08-24 10:00"))

	r := middleware.DemoIle(httptest.NewRequest(http.MethodGet, "/domains/5/databases", nil), true)
	w := httptest.NewRecorder()
	h.ListDatabases(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d, body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); !contains(got, `"db_parola":"••••••••"`) {
		t.Fatalf("parola maskelenmedi: %s", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
```

- [ ] **Step 2: Testin derlenip başarısız olduğunu doğrula**

Run: `go test ./internal/domains/... -run TestListDatabases_DemoModundaMaskelenir -v`
Expected: FAIL (yanıt gerçek `"sifrelenmis-deger"`i çözmeye çalışıp farklı bir değer döner — maskeleme henüz yok)

- [ ] **Step 3: `ListDatabases`'i güncelle**

`internal/domains/handlers.go` içinde `ListDatabases` (satır 914-936):

```go
func (h *Handlers) ListDatabases(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, domain_id, db_name, db_user, db_host, db_pass_plain, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i')
		 FROM db_accounts WHERE domain_id=? ORDER BY id`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB sorgu: "+err.Error())
		return
	}
	defer rows.Close()
	demoPanel := middleware.DemoPaneliMi(r)
	out := make([]DBAccount, 0)
	for rows.Next() {
		var d DBAccount
		if err := rows.Scan(&d.ID, &d.DomainID, &d.DBAdi, &d.DBKullanici, &d.DBHost, &d.DBParola, &d.Olusturulma); err != nil {
			continue
		}
		if dec, err := hesaplar.DecryptDBPassword(d.DBParola); err == nil {
			d.DBParola = dec
		}
		// DEMO: gerçek DB parolası demo panelde asla dönmez.
		d.DBParola = middleware.Maskele(demoPanel, d.DBParola)
		out = append(out, d)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
```

(`middleware` paketi bu dosyada zaten import edilmiş — `internal/domains/handlers.go:27`.)

- [ ] **Step 4: Testin geçtiğini doğrula**

Run: `go test ./internal/domains/... -v`
Expected: PASS (yeni test + paketin geri kalanı)

- [ ] **Step 5: Commit**

```bash
git add internal/domains/handlers.go internal/domains/demo_maskeleme_test.go
git commit -m "feat(domains): demo modunda DB parolasını maskele"
```

---

### Task 6: Mail parolası maskeleme — `internal/mail`

**Files:**
- Modify: `internal/mail/mail.go:356-376` (`ParolaGoster`)
- Test: Create `internal/mail/demo_maskeleme_test.go`

**Interfaces:**
- Consumes: `middleware.DemoPaneliMi(r)`, `middleware.Maskele`, `middleware.DemoIle` (Task 3).

- [ ] **Step 1: Başarısız testi yaz**

```go
package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"

	"sanalcp/internal/middleware"
)

func TestParolaGoster_DemoModundaMaskelenir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handlers{DB: db}

	mock.ExpectQuery(`SELECT sistem_kullanici, COALESCE\(is_demo,0\) FROM domains`).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici", "is_demo"}).AddRow("c_ornek", 0))
	mock.ExpectQuery(`SELECT 1 FROM mailboxes WHERE id=\? AND domain_id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

	token := revealSakla(9, "gercek-parola")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "5")
	rctx.URLParams.Add("mid", "9")
	rctx.URLParams.Add("token", token)

	r := httptest.NewRequest(http.MethodGet, "/domains/5/mail/9/parola-reveal/"+token, nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = middleware.DemoIle(r, true)

	w := httptest.NewRecorder()
	h.ParolaGoster(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d, body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != `{"parola":"••••••••"}`+"\n" {
		t.Fatalf("beklenmeyen gövde: %q", got)
	}
}
```

(`internal/mail/mail.go`'daki `h.domain(r)` fonksiyonu `chi.URLParam(r, "id")` okuyor — `internal/mail/mail.go:49-59`'a bakarak sorgu metnini teyit et; yukarıdaki `SELECT sistem_kullanici, COALESCE(is_demo,0) FROM domains` regex'i o fonksiyonun gerçek sorgusuyla eşleşmelidir, birebir eşleşmiyorsa test yazarken `h.domain`'in tam SQL metnini oradan alıp regex'i güncelle.)

- [ ] **Step 2: Testin derlenip başarısız olduğunu doğrula**

Run: `go test ./internal/mail/... -run TestParolaGoster_DemoModundaMaskelenir -v`
Expected: FAIL (gövde gerçek `"gercek-parola"`yı içerir)

- [ ] **Step 3: `ParolaGoster`'ı güncelle**

`internal/mail/mail.go:356-376`:

```go
func (h *Handlers) ParolaGoster(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	mid, _ := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	var exists int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT 1 FROM mailboxes WHERE id=? AND domain_id=?`, mid, id).Scan(&exists); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "kutu bulunamadı")
		return
	}
	parola, ok := revealAl(chi.URLParam(r, "token"), mid)
	if !ok {
		httpx.WriteError(w, http.StatusGone, "gösterim süresi doldu veya parola zaten görüntülendi")
		return
	}
	// DEMO: gerçek posta kutusu parolası demo panelde asla dönmez.
	parola = middleware.Maskele(middleware.DemoPaneliMi(r), parola)
	w.Header().Set("Cache-Control", "no-store, private")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"parola": parola})
}
```

(`middleware` paketi bu dosyada zaten import edilmiş — `internal/mail/mail.go:22`.)

- [ ] **Step 4: Testin geçtiğini doğrula**

Run: `go test ./internal/mail/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/mail/mail.go internal/mail/demo_maskeleme_test.go
git commit -m "feat(mail): demo modunda posta kutusu parolasını maskele"
```

---

### Task 7: Dosya içeriği maskeleme + indirme engeli — `internal/files`, `internal/backups`

**Files:**
- Modify: `internal/files/files.go:176-210` (`Download`), `internal/files/files.go:230-255` (`Read`)
- Modify: `internal/backups/backups.go:321-354` (`Download`)
- Test: Create `internal/files/demo_maskeleme_test.go`, `internal/backups/demo_engeli_test.go`

**Interfaces:**
- Consumes: `middleware.DemoPaneliMi(r)`, `middleware.Maskele`, `middleware.DemoIle` (Task 3).

- [ ] **Step 1: Başarısız testleri yaz**

`internal/files/demo_maskeleme_test.go`:

```go
package files

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"

	"sanalcp/internal/middleware"
)

func TestDownload_DemoModundaEngellenir(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handlers{DB: db}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "5")
	r := httptest.NewRequest(http.MethodGet, "/domains/5/files/indir?yol=a.txt", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = middleware.DemoIle(r, true)

	w := httptest.NewRecorder()
	h.Download(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("demo modunda indirme engellenmedi: code=%d", w.Code)
	}
}
```

`internal/backups/demo_engeli_test.go`:

```go
package backups

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"

	"sanalcp/internal/middleware"
)

func TestDownload_DemoModundaEngellenir(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handlers{DB: db}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "5")
	rctx.URLParams.Add("bid", "9")
	r := httptest.NewRequest(http.MethodGet, "/domains/5/backups/9/indir", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = middleware.DemoIle(r, true)

	w := httptest.NewRecorder()
	h.Download(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("demo modunda indirme engellenmedi: code=%d", w.Code)
	}
}
```

`internal/files/files.go`'daki `h.home(r)`, `sistem_kullanici`'yi gerçek `/home/<sk>` yoluna çevirir (bkz. `internal/files/files.go:39-50`) — bu yolun fiilen var olması ve okunabilir olması gerekir, ki bu test ortamında (root olmayan, `/home` altına yazamayan CI) pratik değildir. Mevcut paket testleri (`safeio_okuma_test.go`) de bu yüzden `h.Read`/`h.Download`'ı hiç çağırmaz — doğrudan `readFileBeneath(home, rel, sinir)` gibi alt-seviye fonksiyonları `t.TempDir()` ile test eder. Aynı desen izlenir: maskeleme mantığı `Read`'den küçük, saf bir yardımcıya çıkarılır ve o yardımcı doğrudan test edilir.

- [ ] **Step 1b: `internal/files/files.go`'a saf bir maskeleme yardımcısı ekle, test dosyasına onu test eden fonksiyonu yaz**

`internal/files/demo_maskeleme_test.go`'ya (Step 1'deki `TestDownload_DemoModundaEngellenir`'in yanına) ekle:

```go
func TestIcerikGoster_DemoModundaMaskelenir(t *testing.T) {
	if got := icerikGoster(true, []byte("gizli içerik")); got != "••••••••" {
		t.Fatalf("demoPanel=true: got %q, want maskeli", got)
	}
	if got := icerikGoster(false, []byte("gizli içerik")); got != "gizli içerik" {
		t.Fatalf("demoPanel=false: got %q, want değişmemiş içerik", got)
	}
}
```

Bu test dosyasının importlarına `"sanalcp/internal/middleware"` zaten Step 1'deki `TestDownload_DemoModundaEngellenir`'den geliyor; `icerikGoster` `files` paketinin kendi fonksiyonu olduğu için ek import gerekmez.

- [ ] **Step 2: Testlerin derlenip başarısız olduğunu doğrula**

Run: `go test ./internal/files/... ./internal/backups/... -run 'DemoModunda|Demo' -v`
Expected: FAIL (`Download` hâlâ 200 döner / dosya akıtır, `Read` maskelemez)

- [ ] **Step 3: `files.Download`'ı güncelle**

`internal/files/files.go:176-181` başına ekle:

```go
func (h *Handlers) Download(w http.ResponseWriter, r *http.Request) {
	// DEMO: ham dosya akışı JSON alanı gibi kısmi maskelenemez — tamamen kapatılır.
	if middleware.DemoPaneliMi(r) {
		httpx.WriteError(w, http.StatusForbidden, "demo modunda dosya indirilemez")
		return
	}
	home, _, err := h.home(r)
	// ... (geri kalanı değişmez)
```

- [ ] **Step 4: `files.Read`'i güncelle + saf yardımcıyı ekle**

`internal/files/files.go:250-254`:

```go
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"yol":    rel,
		"icerik": icerikGoster(middleware.DemoPaneliMi(r), data),
		"boyut":  boyut,
	})
```

Aynı dosyaya, `Read` fonksiyonunun hemen altına (satır ~255'ten sonra) yeni bir yardımcı ekle:

```go
// icerikGoster: demo panelde dosya içeriği (kaynak kod, .env, wp-config.php
// vb. sır taşıyabilir) yerine sabit bir maske döner.
func icerikGoster(demoPanel bool, data []byte) string {
	return middleware.Maskele(demoPanel, string(data))
}
```

- [ ] **Step 5: `files.go` import listesine `middleware` ekle**

`internal/files/files.go` import bloğuna (satır ~10-23) ekle:

```go
	"sanalcp/internal/middleware"
```

(alfabetik sıra: `"sanalcp/internal/httpx"` ile `github.com/go-chi/chi/v5` arasına; mevcut dosyada bu iki import zaten yan yana, aralarına eklenir.)

- [ ] **Step 6: `backups.Download`'ı güncelle**

`internal/backups/backups.go:321-327` başına ekle:

```go
func (h *Handlers) Download(w http.ResponseWriter, r *http.Request) {
	// DEMO: yedek arşivi tüm tenant dosya ağacını (dolayıyla sırlarını) içerir
	// — ham gzip akışı JSON alanı gibi kısmi maskelenemez, tamamen kapatılır.
	if middleware.DemoPaneliMi(r) {
		httpx.WriteError(w, http.StatusForbidden, "demo modunda yedek indirilemez")
		return
	}
	// Büyük yedek indirmeleri sunucunun kısa varsayılan yazma zaman aşımını
	// (bkz. cmd/server/main.go) aşabilir — bu uç için istisna açılır.
	httpx.ExtendDeadline(w, 30*time.Minute)
	// ... (geri kalanı değişmez)
```

(`middleware` paketi bu dosyada zaten import edilmiş — `internal/backups/backups.go:19`.)

- [ ] **Step 7: Testlerin geçtiğini doğrula**

Run: `go test ./internal/files/... ./internal/backups/... -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/files/files.go internal/files/demo_maskeleme_test.go internal/backups/backups.go internal/backups/demo_engeli_test.go
git commit -m "feat(files,backups): demo modunda dosya/yedek indirmeyi kapat, dosya içeriğini maskele"
```

---

### Task 8: Tohum aracı — `scripts/demo_seed.go`

**Files:**
- Create: `scripts/demo_seed.go`

**Interfaces:**
- Consumes: panelin kendi HTTP API'si (`POST /api/v1/auth/login`, `POST /api/v1/domains`) — `internal/domains/handlers.go:261-276`'daki `createReq{AlanAdi string}` gövde şeklini kullanır; ikinci bir oluşturma yolu AÇILMAZ.
- Produces: demo VPS'inde çalıştırıldığında birkaç örnek domain + `demo_modu_acik=1` (script sonunda geri açılır).

- [ ] **Step 1: Script'i yaz**

```go
//go:build ignore

// Demo panel tohumlama — SADECE demo VPS'inde, kurulumdan sonra elle çalıştırılır:
//
//	go run scripts/demo_seed.go -dsn '...' -taban-url https://127.0.0.1:8443 \
//	  -kullanici demo -parola '...'
//
// Akış: demo_modu_acik'ı GEÇİCİ olarak kapatır (yazma engeli aradan çekilsin
// diye), panelin KENDİ HTTP API'sinden birkaç örnek domain oluşturur (gerçek
// nginx/php-fpm/MySQL kaynağı doğar — sahte satır değil), sonra bayrağı 1'e
// geri çevirir.
//
// Bu script internal/domains paketini DOĞRUDAN import ETMEZ: panel API'sinin
// dışında ikinci bir oluşturma yolu açmak şema/iş kuralı driftine yol açar;
// HTTP API zaten "panelin yaptığı her şeyin" tek gerçek kaynağı (bkz.
// docs/API.md, README.md "Tam yönetim API'si").
package main

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

var ornekDomainler = []string{"vitrin-magaza.demo", "blog-ornegi.demo", "kurumsal-site.demo"}

func main() {
	dsn := flag.String("dsn", "", "MySQL DSN (panel_ayarlari bayrağını geçici kapatmak/açmak için)")
	tabanURL := flag.String("taban-url", "https://127.0.0.1:8443", "panel API taban adresi")
	kullanici := flag.String("kullanici", "demo", "panel giriş kullanıcı adı")
	parola := flag.String("parola", "", "panel giriş parolası")
	flag.Parse()
	if *dsn == "" || *parola == "" {
		log.Fatalf("dsn ve parola zorunlu")
	}

	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		log.Fatalf("db aç: %v", err)
	}
	defer db.Close()

	if err := demoBayragiAyarla(db, 0); err != nil {
		log.Fatalf("bayrak kapatılamadı: %v", err)
	}
	defer func() {
		if err := demoBayragiAyarla(db, 1); err != nil {
			log.Printf("UYARI: bayrak geri açılamadı, elle kontrol et: UPDATE panel_ayarlari SET demo_modu_acik=1 WHERE id=1; (%v)", err)
		}
	}()

	jar, _ := cookiejar.New(nil)
	istemci := &http.Client{Jar: jar, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // self-signed, loopback
	}}

	if err := girisYap(istemci, *tabanURL, *kullanici, *parola); err != nil {
		log.Fatalf("giriş: %v", err)
	}

	for _, ad := range ornekDomainler {
		if err := domainOlustur(istemci, *tabanURL, ad); err != nil {
			log.Printf("UYARI: %s oluşturulamadı: %v", ad, err)
			continue
		}
		fmt.Printf("oluşturuldu: %s\n", ad)
	}

	fmt.Println("tohumlama tamam. Şimdi tek seferlik dump al:")
	fmt.Println("  mysqldump --single-transaction --databases panel | gzip -c > /var/backups/sanalcp/demo/demoseed.sql.gz")
}

func demoBayragiAyarla(db *sql.DB, deger int) error {
	_, err := db.Exec(`UPDATE panel_ayarlari SET demo_modu_acik=? WHERE id=1`, deger)
	return err
}

func girisYap(c *http.Client, tabanURL, kullanici, parola string) error {
	gov, _ := json.Marshal(map[string]string{"kullanici": kullanici, "parola": parola})
	req, err := http.NewRequest(http.MethodPost, tabanURL+"/api/v1/auth/login", bytes.NewReader(gov))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", tabanURL) // CSRFKoruma origin kontrolü için gerekli
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("giriş %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func domainOlustur(c *http.Client, tabanURL, alanAdi string) error {
	gov, _ := json.Marshal(map[string]string{"alan_adi": alanAdi})
	req, err := http.NewRequest(http.MethodPost, tabanURL+"/api/v1/domains", bytes.NewReader(gov))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", tabanURL)
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
```

- [ ] **Step 2: Derlendiğini doğrula (çalıştırma değil — üretim DB'si gerektirir)**

Run: `go vet ./scripts/... ` (not: `//go:build ignore` etiketi normal `go build ./...`'a dahil olmaz, bu yüzden ayrıca doğrula) `go build -o /dev/null scripts/demo_seed.go`
Expected: derleme hatası yok

- [ ] **Step 3: Commit**

```bash
git add scripts/demo_seed.go
git commit -m "feat(scripts): demo VPS için API üzerinden tohumlama aracı"
```

---

### Task 9: Gecelik reset — `assets/ops/sanalcp-demo-reset` + systemd timer

**Files:**
- Create: `assets/ops/sanalcp-demo-reset`
- Create: `assets/systemd/sanalcp-demo-reset.service`
- Create: `assets/systemd/sanalcp-demo-reset.timer`

**Interfaces:**
- Consumes: Task 8'in ürettiği `/var/backups/sanalcp/demo/demoseed.sql.gz` dump'ı (elle, bir kez alınır — bkz. Task 8 Step 1 çıktısı).

- [ ] **Step 1: Reset script'ini yaz**

```bash
#!/bin/bash
# SanalCP DEMO — gecelik sıfırlama.
#
# Yalnız demo_modu_acik=1 olan kurulumda systemd timer ile çalışır (bkz.
# sanalcp-demo-reset.timer). API yazmaları zaten middleware.DemoSaltOkunur
# tarafından bloklanır (bkz. internal/middleware/demo.go); bu script'in asıl
# işi zamanla biriken disk/log/sayaç durumunu (SSL süre sayacı, kota
# göstergesi, journal büyümesi) temizlemek ve tohum durumunu SQL dump'tan
# geri yüklemek.
#
# ÖN KOŞUL: scripts/demo_seed.go bir kez çalıştırılmış ve dump'ı alınmış
# olmalı (bkz. o script'in son satırdaki mysqldump komutu).
set -euo pipefail

DUMP=/var/backups/sanalcp/demo/demoseed.sql.gz
[ -f "$DUMP" ] || { echo "HATA: tohum dump'ı yok: $DUMP (önce scripts/demo_seed.go çalıştırılmalı)" >&2; exit 1; }

systemctl stop sanalcp

zcat "$DUMP" | mysql panel

journalctl --vacuum-time=1h >/dev/null 2>&1 || true
find /var/log/sanalcp -maxdepth 1 -type f -mtime +0 -delete 2>/dev/null || true

systemctl start sanalcp

echo "demo reset tamam: $(date -u +%FT%TZ)"
```

- [ ] **Step 2: `chmod +x` ve systemd birimlerini yaz**

Run: `chmod +x assets/ops/sanalcp-demo-reset`

`assets/systemd/sanalcp-demo-reset.service`:

```ini
[Unit]
Description=SanalCP demo panel gecelik sifirlama
After=network.target mariadb.service

[Service]
Type=oneshot
ExecStart=/usr/local/bin/sanalcp-demo-reset
```

`assets/systemd/sanalcp-demo-reset.timer`:

```ini
[Unit]
Description=SanalCP demo panel gecelik sifirlama zamanlayicisi

[Timer]
OnCalendar=*-*-* 04:00:00
RandomizedDelaySec=120
Persistent=true

[Install]
WantedBy=timers.target
```

(Desen `assets/systemd/sanalcp-db-backup.{service,timer}` ile birebir aynı — `ExecStart` yolu `/usr/local/bin/` altında, kurulum sırasında `assets/ops/*` oradan sembolik bağlanıyor; bu iki dosyanın kurulum script'ine eklenmesi bilinçli olarak bu planın kapsamı DIŞINDA — `sanalcp-install.sh` yalnız normal kurulumlar için çalışır, demo VPS bu timer'ı elle etkinleştirir: `systemctl enable --now sanalcp-demo-reset.timer`.)

- [ ] **Step 3: Commit**

```bash
git add assets/ops/sanalcp-demo-reset assets/systemd/sanalcp-demo-reset.service assets/systemd/sanalcp-demo-reset.timer
git commit -m "feat(ops): demo panel için gecelik sıfırlama script'i + systemd timer"
```

---

## Self-Review Notları (uygulama öncesi)

- **Spec kapsaması:** Spec'in A-F bölümlerinin hepsi bir task'a karşılık geliyor — A→Task 9 (kurulum notu, install.sh'a dokunulmuyor, bilinçli), B→Task 1/2/3/4, C→Task 5/6/7, D→Task 8, E→Task 8 (paylaşımlı hesap `-kullanici demo` parametresiyle), F→(kapsam dışı, kod değişikliği yok).
- **Bilinen sapma (yukarıda gerekçeli):** fail-safe yönü spec'in aksine çevrildi (demo=KAPALI varsayımı). Sır maskeleme somutlaştırılırken `files.Download`/`backups.Download` "maskele" yerine "kapat" oldu — ikisi de Global Constraints'te açıkça belirtildi, sürpriz değil.
- **Tip tutarlılığı:** `middleware.DemoPaneliMi`, `middleware.Maskele`, `middleware.DemoIle` imzaları Task 3'te tanımlanıp Task 5/6/7'de birebir aynı adlarla kullanıldı. `icerikGoster` (Task 7) yalnız `files` paketi içinde, `Maskele`'yi sarmalayan saf bir yardımcıdır.
- **Task 7 test tasarımı:** `files.Read`/`files.Download` gerçek `/home/<sk>` yoluna bağımlı olduğu için (root gerektirir, CI'da pratik değil) — mevcut paketin kendi konvansiyonuna uyularak (`safeio_okuma_test.go` da `h.Read`/`h.Download`'ı hiç çağırmaz) maskeleme mantığı saf bir yardımcıya (`icerikGoster`) çıkarılıp doğrudan test edildi; `Download`'ın demo-engeli ise DB sorgusuna ulaşmadan en baştan döndüğü için sqlmock'suz test edilebiliyor.
