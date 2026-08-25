# Veritabanı Yönet Sayfası Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Domain > Veritabanları ekranındaki her veritabanı için, adına göre gruplanmış bir "Yönet" sayfası eklemek: boyut/charset gösterimi, kullanıcı ekleme/silme/şifre değiştirme, isim değiştirme, tek tıkla gzip yedekleme, geri yükleme, optimize ve onar (repair).

**Architecture:** Backend'de yeni `internal/domains/handlers_dbyonet.go` dosyası, `db_accounts` tablosunu `db_name`'e göre gruplayan/işleyen 8 yeni uç açar (hepsi `MusteriScope` + `{id}` domain kapsamı). `internal/hesaplar`'a 4 yeni MySQL yardımcı fonksiyonu eklenir (grant/revoke/rename, hepsi mevcut `MySQLCreateDB`/`MySQLDropDB` deseniyle birebir — `rootExecAll` + kimlik doğrulama guard'ı). Var olan iki uç (`DeleteDatabase`, `SetDatabasePassword`) `AdminOnly`'den çıkarılıp `pma.TokenIste`'deki manuel `DomainSahibiMi` IDOR desenine geçirilir. Frontend'de liste sayfası `db_adi`'na göre gruplanır, yeni `DomainDatabaseYonetPage.tsx` 3 kartla (genel bilgi, kullanıcılar, bakım) eklenir.

**Tech Stack:** Go (chi router, database/sql, os/exec — mysqldump/mysql/mysqlcheck CLI), React + TypeScript (axios, react-i18next, Tailwind).

**Spec:** `docs/superpowers/specs/2026-08-25-veritabani-yonetim-design.md`

## Global Constraints

- DB/kullanıcı adı doğrulama: `hesaplar.GecerliDBKimlik` (`^[A-Za-z0-9_]{1,64}$`) — SQL/exec'e giden HER identifier bundan geçmeli.
- Müşteri girdisi (sonek) doğrulama: `hesaplar.GecerliDBSonek` (`^[a-z0-9_]{1,32}$`), panel `<sk>_` önekini zorunlu ekler.
- Parola gücü: `hesaplar.ParolaGucluMu` (≥12 karakter, harf+rakam karışık, tek satır).
- Restore yükleme üst sınırı: 200 MB (`maxDBRestoreBytes`).
- Uzun süren işlemler (rename/backup/restore/optimize/repair): `httpx.ExtendDeadline(w, 15*time.Minute)` (global `chi.Timeout(300s)`'u aşmak için, `files.Upload`'daki 30dk desenle aynı fikir).
- Yeni backend uçları `middleware.MusteriScope` ile korunur (URL'deki `{id}` domain param'ını otomatik doğrular — bkz. `internal/middleware/auth.go:204`). `{id}` içermeyen uçlarda (`/databases/{dbid}...`) manuel `middleware.DomainSahibiMi(r, domainID)` kullanılır (`internal/pma/pma.go:64-70` deseni), var-olmayan kayıtla AYNI 404 döner (varlık sızdırılmaz).
- Frontend i18n dosyaları `frontend/src/i18n/locales/{tr,en}/*.json` altına eklenir, `import.meta.glob` ile otomatik toplanır — elle kayıt gerekmez.
- UI: mevcut `ta-primary-button`, `ta-secondary-button`, `ta-danger-button`, `ta-input`, `ta-input-sm`, `ta-form-error`, `ta-form-success`, `ta-form-actions` sınıfları kullanılır, yeni sınıf icat edilmez.
- Frontend'de birim test altyapısı yok (proje genelinde) — frontend görevleri `npx tsc --noEmit` + `npm run build` ile doğrulanır, manuel smoke test notu düşülür.

---

## Backend

### Task 1: `hesaplar` — mevcut DB'ye kullanıcı ekleme yardımcıları

**Files:**
- Modify: `internal/hesaplar/hesaplar.go` (yeni fonksiyonlar, `MySQLDropDBKeepUser`'dan hemen sonra eklenir)
- Test: `internal/hesaplar/dbyonet_test.go` (yeni)

**Interfaces:**
- Produces: `MySQLGrantNewUser(db *sql.DB, domainID int64, dbName, dbUser, dbPass string) error`, `MySQLGrantExistingUser(db *sql.DB, domainID int64, dbName, dbUser string) error`

- [ ] **Step 1: Write the failing test**

```go
// internal/hesaplar/dbyonet_test.go
package hesaplar

import "testing"

func TestMySQLGrantNewUserGecersizKimlikleriReddeder(t *testing.T) {
	if err := MySQLGrantNewUser(nil, 1, "gecerli_db", "kotu ad", "Parola1234567!"); err == nil {
		t.Error("boşluklu kullanıcı adı reddedilmeliydi")
	}
	if err := MySQLGrantNewUser(nil, 1, "kötü-db", "gecerli_user", "Parola1234567!"); err == nil {
		t.Error("tire içeren DB adı reddedilmeliydi")
	}
}

func TestMySQLGrantExistingUserGecersizKimlikleriReddeder(t *testing.T) {
	if err := MySQLGrantExistingUser(nil, 1, "gecerli_db", "kotu;user"); err == nil {
		t.Error("noktalı virgüllü kullanıcı adı reddedilmeliydi")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hesaplar/... -run TestMySQLGrant -v`
Expected: derleme hatası (`MySQLGrantNewUser`/`MySQLGrantExistingUser` tanımlı değil)

- [ ] **Step 3: Write minimal implementation**

`internal/hesaplar/hesaplar.go` içine, `MySQLDropDBKeepUser` fonksiyonundan hemen sonra ekle:

```go
// MySQLGrantNewUser: var olan bir DB'ye YENİ bir kullanıcı olustur + GRANT ver
// (CREATE DATABASE YAPMAZ — DB zaten var olmalı). db_accounts'a yeni satır ekler.
func MySQLGrantNewUser(db *sql.DB, domainID int64, dbName, dbUser, dbPass string) error {
	if !GecerliDBKimlik(dbName) || !GecerliDBKimlik(dbUser) {
		return fmt.Errorf("güvenlik: geçersiz veritabanı adı veya kullanıcısı")
	}
	if err := rootExecAll(
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s'", dbUser, sqlKac(dbPass)),
		fmt.Sprintf("ALTER USER '%s'@'localhost' IDENTIFIED BY '%s'", dbUser, sqlKac(dbPass)),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'", dbName, dbUser),
		"FLUSH PRIVILEGES",
	); err != nil {
		return err
	}
	encPass, encErr := box.Encrypt(dbPass)
	if encErr != nil {
		return fmt.Errorf("db parola şifreleme: %w", encErr)
	}
	_, err := db.Exec(
		`INSERT INTO db_accounts(domain_id, db_name, db_user, db_pass_plain, db_host)
		 VALUES(?,?,?,?, 'localhost')`,
		domainID, dbName, dbUser, encPass)
	return err
}

// MySQLGrantExistingUser: var olan bir DB'ye, ZATEN var olan bir kullanıcıya GRANT ver
// (CREATE/ALTER USER YOK → parola korunur). Çağıran, dbUser'ın bu domaine ait olduğunu
// ÖNCEDEN doğrulamalıdır (sahiplik + önek). Çağıran ayrıca db_accounts'tan mevcut
// parolayı okuyup yanıtta gösterebilir (bu fonksiyon parolayı döndürmez).
func MySQLGrantExistingUser(db *sql.DB, domainID int64, dbName, dbUser string) error {
	if !GecerliDBKimlik(dbName) || !GecerliDBKimlik(dbUser) {
		return fmt.Errorf("güvenlik: geçersiz veritabanı adı veya kullanıcısı")
	}
	var pass string
	if err := db.QueryRow(
		`SELECT db_pass_plain FROM db_accounts WHERE db_user=? LIMIT 1`, dbUser).Scan(&pass); err != nil {
		return fmt.Errorf("mevcut kullanıcı parolası bulunamadı: %w", err)
	}
	if err := rootExecAll(
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'", dbName, dbUser),
		"FLUSH PRIVILEGES",
	); err != nil {
		return err
	}
	_, err := db.Exec(
		`INSERT INTO db_accounts(domain_id, db_name, db_user, db_pass_plain, db_host)
		 VALUES(?,?,?,?, 'localhost')`,
		domainID, dbName, dbUser, pass)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/hesaplar/... -run TestMySQLGrant -v`
Expected: PASS (guard clause `GecerliDBKimlik` kontrolünde döner, `rootDB`'ye hiç dokunmaz — `db *sql.DB` parametresi `nil` olsa da testler geçer)

- [ ] **Step 5: Commit**

```bash
git add internal/hesaplar/hesaplar.go internal/hesaplar/dbyonet_test.go
git commit -m "feat(hesaplar): var olan DB'ye kullanıcı ekleme (yeni/mevcut)"
```

---

### Task 2: `hesaplar` — kullanıcı erişimini kaldırma yardımcısı

**Files:**
- Modify: `internal/hesaplar/hesaplar.go`
- Test: `internal/hesaplar/dbyonet_test.go` (Task 1'de açılan dosyaya eklenir)

**Interfaces:**
- Produces: `MySQLRevokeUser(db *sql.DB, dbName, dbUser string, dropUser bool) error`

- [ ] **Step 1: Write the failing test**

`internal/hesaplar/dbyonet_test.go`'ya ekle:

```go
func TestMySQLRevokeUserGecersizKimlikleriReddeder(t *testing.T) {
	if err := MySQLRevokeUser(nil, "kötü-db", "gecerli_user", false); err == nil {
		t.Error("tire içeren DB adı reddedilmeliydi")
	}
	if err := MySQLRevokeUser(nil, "gecerli_db", "kotu user", true); err == nil {
		t.Error("boşluklu kullanıcı adı reddedilmeliydi")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hesaplar/... -run TestMySQLRevokeUser -v`
Expected: derleme hatası (`MySQLRevokeUser` tanımlı değil)

- [ ] **Step 3: Write minimal implementation**

`internal/hesaplar/hesaplar.go`'ya, Task 1'de eklenen fonksiyonlardan sonra:

```go
// MySQLRevokeUser: kullanicinin bir DB uzerindeki erisimini kaldirir (DB'nin
// kendisine DOKUNMAZ). dropUser=true ise kullanici MariaDB'den tamamen silinir
// (baska hicbir DB'de kullanilmiyorsa cagiran bunu ONCEDEN kontrol etmelidir);
// false ise yalniz bu DB uzerindeki GRANT geri alinir, kullanici yasamaya devam eder.
func MySQLRevokeUser(db *sql.DB, dbName, dbUser string, dropUser bool) error {
	if !GecerliDBKimlik(dbName) || !GecerliDBKimlik(dbUser) {
		return fmt.Errorf("güvenlik: geçersiz veritabanı adı veya kullanıcısı")
	}
	stmts := []string{fmt.Sprintf("REVOKE ALL PRIVILEGES ON `%s`.* FROM '%s'@'localhost'", dbName, dbUser)}
	if dropUser {
		stmts = append(stmts, fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost'", dbUser))
	}
	stmts = append(stmts, "FLUSH PRIVILEGES")
	if err := rootExecAll(stmts...); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM db_accounts WHERE db_name=? AND db_user=?`, dbName, dbUser)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/hesaplar/... -run TestMySQLRevokeUser -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/hesaplar/hesaplar.go internal/hesaplar/dbyonet_test.go
git commit -m "feat(hesaplar): kullanıcı erişimini kaldırma (revoke/drop)"
```

---

### Task 3: `hesaplar` — veritabanı isim değiştirme

**Files:**
- Modify: `internal/hesaplar/hesaplar.go`
- Test: `internal/hesaplar/dbyonet_test.go`

**Interfaces:**
- Consumes: `rootExecAll(stmts ...string) error` (mevcut, `internal/hesaplar/hesaplar.go:71`)
- Produces: `MySQLRenameDB(ctx context.Context, db *sql.DB, domainID int64, eskiAd, yeniAd string, kullanicilar []string) error`

- [ ] **Step 1: Write the failing test**

`internal/hesaplar/dbyonet_test.go`'ya ekle:

```go
import "context"

func TestMySQLRenameDBGecersizKimlikleriReddeder(t *testing.T) {
	ctx := context.Background()
	if err := MySQLRenameDB(ctx, nil, 1, "kötü-db", "yeni_db", []string{"kullanici1"}); err == nil {
		t.Error("tire içeren eski DB adı reddedilmeliydi")
	}
	if err := MySQLRenameDB(ctx, nil, 1, "eski_db", "kötü-db", []string{"kullanici1"}); err == nil {
		t.Error("tire içeren yeni DB adı reddedilmeliydi")
	}
	if err := MySQLRenameDB(ctx, nil, 1, "eski_db", "yeni_db", []string{"kotu user"}); err == nil {
		t.Error("boşluklu kullanıcı adı reddedilmeliydi")
	}
}
```

(Not: dosyanın başındaki `import` bloğuna `"context"` bu adımda eklenir — dosya zaten Task 1/2'de var, tek bir import bloğu paylaşılır.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hesaplar/... -run TestMySQLRenameDB -v`
Expected: derleme hatası (`MySQLRenameDB` tanımlı değil)

- [ ] **Step 3: Write minimal implementation**

`internal/hesaplar/hesaplar.go`'nun import bloğuna `"context"` ve `"os/exec"` (zaten var) ekli olduğundan emin ol, sonra Task 2'nin fonksiyonundan sonra ekle:

```go
// MySQLRenameDB: MySQL'de native RENAME DATABASE yok — CREATE + mysqldump|mysql
// taşıma (view/trigger/routine/event de taşınır, RENAME TABLE döngüsünün aksine)
// + grant taşıma + DROP ile taklit edilir. Herhangi bir adım basarisiz olursa
// yeni DB best-effort silinir, eski DB adim 4'e kadar DOKUNULMAMIŞ kalır.
func MySQLRenameDB(ctx context.Context, db *sql.DB, domainID int64, eskiAd, yeniAd string, kullanicilar []string) error {
	if !GecerliDBKimlik(eskiAd) || !GecerliDBKimlik(yeniAd) {
		return fmt.Errorf("güvenlik: geçersiz veritabanı adı")
	}
	for _, u := range kullanicilar {
		if !GecerliDBKimlik(u) {
			return fmt.Errorf("güvenlik: geçersiz kullanıcı adı")
		}
	}

	temizle := func() { _ = rootExecAll(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", yeniAd)) }

	if err := rootExecAll(
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", yeniAd),
	); err != nil {
		return err
	}

	dump := exec.CommandContext(ctx, "mysqldump", "--single-transaction", "--routines", "--triggers", "--events", eskiAd)
	restore := exec.CommandContext(ctx, "mysql", yeniAd)
	pipe, err := dump.StdoutPipe()
	if err != nil {
		temizle()
		return fmt.Errorf("pipe: %w", err)
	}
	restore.Stdin = pipe
	var dumpErr, restoreErr strings.Builder
	dump.Stderr = &dumpErr
	restore.Stderr = &restoreErr

	if err := restore.Start(); err != nil {
		temizle()
		return fmt.Errorf("mysql start: %w", err)
	}
	if err := dump.Run(); err != nil {
		_ = restore.Wait()
		temizle()
		return fmt.Errorf("mysqldump %s: %s: %w", eskiAd, strings.TrimSpace(dumpErr.String()), err)
	}
	if err := restore.Wait(); err != nil {
		temizle()
		return fmt.Errorf("mysql %s: %s: %w", yeniAd, strings.TrimSpace(restoreErr.String()), err)
	}

	grants := make([]string, 0, len(kullanicilar)*2+1)
	for _, u := range kullanicilar {
		grants = append(grants,
			fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'", yeniAd, u),
			fmt.Sprintf("REVOKE ALL PRIVILEGES ON `%s`.* FROM '%s'@'localhost'", eskiAd, u))
	}
	grants = append(grants, "FLUSH PRIVILEGES")
	if err := rootExecAll(grants...); err != nil {
		temizle()
		return fmt.Errorf("grant/revoke: %w", err)
	}

	if err := rootExecAll(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", eskiAd)); err != nil {
		return fmt.Errorf("yeni veritabanı aktif ama eski silinemedi (elle temizleyin): %w", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE db_accounts SET db_name=? WHERE db_name=? AND domain_id=?`, yeniAd, eskiAd, domainID); err != nil {
		return fmt.Errorf("metadata güncelleme: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/hesaplar/... -run TestMySQLRenameDB -v`
Expected: PASS

- [ ] **Step 5: Run full package test + vet**

Run: `go build ./... && go vet ./... && go test ./internal/hesaplar/...`
Expected: hepsi PASS/temiz

- [ ] **Step 6: Commit**

```bash
git add internal/hesaplar/hesaplar.go internal/hesaplar/dbyonet_test.go
git commit -m "feat(hesaplar): veritabanı isim değiştirme (dump|restore taşıma)"
```

---

### Task 4: `internal/domains` — grup detay ucu (GET)

**Files:**
- Create: `internal/domains/handlers_dbyonet.go`
- Test: `internal/domains/handlers_dbyonet_test.go` (yeni)

**Interfaces:**
- Consumes: `hesaplar.DecryptDBPassword(enc string) (string, error)` (mevcut)
- Produces: `Handlers.DatabaseGrupDetay(w http.ResponseWriter, r *http.Request)`, tip `dbKullaniciSatiri`, `dbGrupDetay`, yardımcı `dbKullanicilariGetir(ctx context.Context, db *sql.DB, domainID int64, dbAdi string) ([]string, error)` (Task 5-7'de de kullanılacak)

- [ ] **Step 1: Write the failing test**

```go
// internal/domains/handlers_dbyonet_test.go
package domains

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

func TestDatabaseGrupDetayBirdenFazlaKullaniciyiGruplar(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "db_user", "db_host", "db_pass_plain", "olusturulma"}).
		AddRow(int64(1), "sk_blog", "localhost", "", "2026-08-25 10:00").
		AddRow(int64(2), "sk_ikinci", "localhost", "", "2026-08-25 11:00")
	mock.ExpectQuery(`SELECT id, db_user, db_host, db_pass_plain, DATE_FORMAT\(created_at,'%Y-%m-%d %H:%i'\)\s+FROM db_accounts WHERE domain_id=\? AND db_name=\? ORDER BY id`).
		WithArgs(int64(7), "sk_blog").
		WillReturnRows(rows)
	mock.ExpectQuery(`SELECT SUM\(data_length\+index_length\)/1024/1024 FROM information_schema.tables WHERE table_schema=\?`).
		WithArgs("sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"boyut"}).AddRow(2.5))
	mock.ExpectQuery(`SELECT default_character_set_name, default_collation_name FROM information_schema.schemata WHERE schema_name=\?`).
		WithArgs("sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"cs", "co"}).AddRow("utf8mb4", "utf8mb4_unicode_ci"))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Get("/domains/{id}/databases/{dbAdi}", h.DatabaseGrupDetay)

	req := httptest.NewRequest(http.MethodGet, "/domains/7/databases/sk_blog", nil)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("200 bekleniyordu, %d geldi: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !contains(body, `"db_adi":"sk_blog"`) || !contains(body, "sk_ikinci") {
		t.Errorf("iki kullanıcı da yanıtta olmalıydı: %s", body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestDatabaseGrupDetayBulunamayanDB404Doner(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, db_user, db_host, db_pass_plain, DATE_FORMAT\(created_at,'%Y-%m-%d %H:%i'\)\s+FROM db_accounts WHERE domain_id=\? AND db_name=\? ORDER BY id`).
		WithArgs(int64(7), "yok_boyle_db").
		WillReturnRows(sqlmock.NewRows([]string{"id", "db_user", "db_host", "db_pass_plain", "olusturulma"}))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Get("/domains/{id}/databases/{dbAdi}", h.DatabaseGrupDetay)

	req := httptest.NewRequest(http.MethodGet, "/domains/7/databases/yok_boyle_db", nil)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("404 bekleniyordu, %d geldi", w.Code)
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

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domains/... -run TestDatabaseGrupDetay -v`
Expected: derleme hatası (`Handlers.DatabaseGrupDetay` tanımlı değil)

- [ ] **Step 3: Write minimal implementation**

```go
// internal/domains/handlers_dbyonet.go
package domains

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"

	"sanalcp/internal/hesaplar"
	"sanalcp/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// dbKullanicilariGetir: bir domain+db_name icin GRANT'li tum kullanicilari
// dondurur (Task 4-7'de paylasilan yardimci).
func dbKullanicilariGetir(ctx context.Context, db *sql.DB, domainID int64, dbAdi string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT db_user FROM db_accounts WHERE domain_id=? AND db_name=?`, domainID, dbAdi)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

type dbKullaniciSatiri struct {
	ID          int64  `json:"id"`
	DBKullanici string `json:"db_kullanici"`
	DBParola    string `json:"db_parola"`
	Olusturulma string `json:"olusturulma"`
}

type dbGrupDetay struct {
	DBAdi        string              `json:"db_adi"`
	DBHost       string              `json:"db_host"`
	Charset      string              `json:"charset"`
	Collation    string              `json:"collation"`
	BoyutMB      float64             `json:"boyut_mb"`
	Kullanicilar []dbKullaniciSatiri `json:"kullanicilar"`
}

// DatabaseGrupDetay: GET /domains/{id}/databases/{dbAdi}
func (h *Handlers) DatabaseGrupDetay(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	dbAdi := chi.URLParam(r, "dbAdi")

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, db_user, db_host, db_pass_plain, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i')
		 FROM db_accounts WHERE domain_id=? AND db_name=? ORDER BY id`, id, dbAdi)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB sorgu: "+err.Error())
		return
	}
	defer rows.Close()

	var out dbGrupDetay
	out.DBAdi = dbAdi
	for rows.Next() {
		var k dbKullaniciSatiri
		var host string
		if err := rows.Scan(&k.ID, &k.DBKullanici, &host, &k.DBParola, &k.Olusturulma); err != nil {
			continue
		}
		out.DBHost = host
		if dec, err := hesaplar.DecryptDBPassword(k.DBParola); err == nil {
			k.DBParola = dec
		}
		out.Kullanicilar = append(out.Kullanicilar, k)
	}
	if len(out.Kullanicilar) == 0 {
		httpx.WriteError(w, http.StatusNotFound, "veritabanı bulunamadı")
		return
	}

	var boyutMB sql.NullFloat64
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT SUM(data_length+index_length)/1024/1024 FROM information_schema.tables WHERE table_schema=?`,
		dbAdi).Scan(&boyutMB)
	out.BoyutMB = boyutMB.Float64

	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT default_character_set_name, default_collation_name FROM information_schema.schemata WHERE schema_name=?`,
		dbAdi).Scan(&out.Charset, &out.Collation)

	httpx.WriteJSON(w, http.StatusOK, out)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domains/... -run TestDatabaseGrupDetay -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domains/handlers_dbyonet.go internal/domains/handlers_dbyonet_test.go
git commit -m "feat(domains): veritabanı grup detay ucu (GET)"
```

---

### Task 5: `internal/domains` — isim değiştirme ucu (PUT)

**Files:**
- Modify: `internal/domains/handlers_dbyonet.go`
- Test: `internal/domains/handlers_dbyonet_test.go`

**Interfaces:**
- Consumes: `dbKullanicilariGetir` (Task 4), `hesaplar.GecerliDBSonek`, `hesaplar.GecerliDBKimlik`, `hesaplar.MySQLRenameDB` (Task 3), `httpx.ExtendDeadline`
- Produces: `Handlers.DatabaseIsimDegistir(w http.ResponseWriter, r *http.Request)`

- [ ] **Step 1: Write the failing test**

`internal/domains/handlers_dbyonet_test.go`'ya ekle:

```go
func TestDatabaseIsimDegistirGecersizSonekiReddeder(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT sistem_kullanici, is_demo FROM domains WHERE id=\?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici", "is_demo"}).AddRow("sk", 0))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Put("/domains/{id}/databases/{dbAdi}/isim", h.DatabaseIsimDegistir)

	body := strings.NewReader(`{"yeni_sonek":"Buyuk Harf Ve Bosluk"}`)
	req := httptest.NewRequest(http.MethodPut, "/domains/7/databases/sk_blog/isim", body)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("400 bekleniyordu, %d geldi: %s", w.Code, w.Body.String())
	}
}

func TestDatabaseIsimDegistirCakismaVarsa409Doner(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT sistem_kullanici, is_demo FROM domains WHERE id=\?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici", "is_demo"}).AddRow("sk", 0))
	mock.ExpectQuery(`SELECT DISTINCT db_user FROM db_accounts WHERE domain_id=\? AND db_name=\?`).
		WithArgs(int64(7), "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"db_user"}).AddRow("sk_blog"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE db_name=\?`).
		WithArgs("sk_yeni").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Put("/domains/{id}/databases/{dbAdi}/isim", h.DatabaseIsimDegistir)

	body := strings.NewReader(`{"yeni_sonek":"yeni"}`)
	req := httptest.NewRequest(http.MethodPut, "/domains/7/databases/sk_blog/isim", body)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("409 bekleniyordu, %d geldi: %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domains/... -run TestDatabaseIsimDegistir -v`
Expected: derleme hatası (`Handlers.DatabaseIsimDegistir` tanımlı değil, `strings` kullanılmayan import kalır)

- [ ] **Step 3: Write minimal implementation**

`internal/domains/handlers_dbyonet.go`'nun import bloğunu tam listeye genişlet ve fonksiyonu ekle:

```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"sanalcp/internal/hesaplar"
	"sanalcp/internal/httpx"

	"github.com/go-chi/chi/v5"
)
```

```go
type dbIsimDegistirReq struct {
	YeniSonek string `json:"yeni_sonek"`
}

// DatabaseIsimDegistir: PUT /domains/{id}/databases/{dbAdi}/isim
func (h *Handlers) DatabaseIsimDegistir(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	eskiAd := chi.URLParam(r, "dbAdi")

	var sk string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, is_demo FROM domains WHERE id=?`, id).Scan(&sk, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "domain sorgu: "+err.Error())
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğin veritabanı adı değiştirilemez")
		return
	}

	var req dbIsimDegistirReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	if !hesaplar.GecerliDBSonek(req.YeniSonek) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz veritabanı soneki (yalnız küçük harf/rakam/alt-çizgi, 1-32 karakter)")
		return
	}
	yeniAd := sk + "_" + req.YeniSonek
	if !hesaplar.GecerliDBKimlik(yeniAd) {
		httpx.WriteError(w, http.StatusBadRequest, "veritabanı adı çok uzun (önek + sonek ≤64 karakter olmalı)")
		return
	}

	kullanicilar, err := dbKullanicilariGetir(r.Context(), h.DB, id, eskiAd)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı sorgu: "+err.Error())
		return
	}
	if len(kullanicilar) == 0 {
		httpx.WriteError(w, http.StatusNotFound, "veritabanı bulunamadı")
		return
	}

	var cakisma int
	_ = h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM db_accounts WHERE db_name=?`, yeniAd).Scan(&cakisma)
	if cakisma > 0 {
		httpx.WriteError(w, http.StatusConflict, "bu isimde bir veritabanı zaten var: "+yeniAd)
		return
	}

	httpx.ExtendDeadline(w, 15*time.Minute)
	if err := hesaplar.MySQLRenameDB(r.Context(), h.DB, id, eskiAd, yeniAd, kullanicilar); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "isim değiştirme: "+err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "eski_ad": eskiAd, "yeni_ad": yeniAd,
		"uyari": "Veritabanı adını kullanan uygulama ayar dosyalarınızı (örn. wp-config.php) elle güncellemeniz gerekir.",
	})
}
```

(`"strings"` test dosyasında `strings.NewReader` için gerekli — `handlers_dbyonet_test.go`'nun import bloğuna ekle.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domains/... -run TestDatabaseIsimDegistir -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domains/handlers_dbyonet.go internal/domains/handlers_dbyonet_test.go
git commit -m "feat(domains): veritabanı isim değiştirme ucu (PUT)"
```

---

### Task 6: `internal/domains` — kullanıcı ekleme ucu (POST)

**Files:**
- Modify: `internal/domains/handlers_dbyonet.go`
- Test: `internal/domains/handlers_dbyonet_test.go`

**Interfaces:**
- Consumes: `hesaplar.MySQLGrantNewUser`, `hesaplar.MySQLGrantExistingUser` (Task 1), `hesaplar.RandomParola`, `hesaplar.ParolaGucluMu`, `hesaplar.DecryptDBPassword`
- Produces: `Handlers.DatabaseKullaniciEkle(w http.ResponseWriter, r *http.Request)`

- [ ] **Step 1: Write the failing test**

```go
func TestDatabaseKullaniciEkleMevcutKullaniciDomaineAitDegilse400(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT sistem_kullanici, is_demo FROM domains WHERE id=\?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici", "is_demo"}).AddRow("sk", 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE domain_id=\? AND db_name=\?`).
		WithArgs(int64(7), "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE domain_id=\? AND db_user=\?`).
		WithArgs(int64(7), "sk_baska").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Post("/domains/{id}/databases/{dbAdi}/kullanicilar", h.DatabaseKullaniciEkle)

	body := strings.NewReader(`{"kullanici_tipi":"mevcut","mevcut_kullanici":"sk_baska"}`)
	req := httptest.NewRequest(http.MethodPost, "/domains/7/databases/sk_blog/kullanicilar", body)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("400 bekleniyordu, %d geldi: %s", w.Code, w.Body.String())
	}
}

func TestDatabaseKullaniciEkleZatenErisimiVarsa409(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT sistem_kullanici, is_demo FROM domains WHERE id=\?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"sistem_kullanici", "is_demo"}).AddRow("sk", 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE domain_id=\? AND db_name=\?`).
		WithArgs(int64(7), "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE domain_id=\? AND db_user=\?`).
		WithArgs(int64(7), "sk_baska").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE domain_id=\? AND db_name=\? AND db_user=\?`).
		WithArgs(int64(7), "sk_blog", "sk_baska").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Post("/domains/{id}/databases/{dbAdi}/kullanicilar", h.DatabaseKullaniciEkle)

	body := strings.NewReader(`{"kullanici_tipi":"mevcut","mevcut_kullanici":"sk_baska"}`)
	req := httptest.NewRequest(http.MethodPost, "/domains/7/databases/sk_blog/kullanicilar", body)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("409 bekleniyordu, %d geldi: %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domains/... -run TestDatabaseKullaniciEkle -v`
Expected: derleme hatası (`Handlers.DatabaseKullaniciEkle` tanımlı değil)

- [ ] **Step 3: Write minimal implementation**

`internal/domains/handlers_dbyonet.go`'ya ekle:

```go
type dbKullaniciEkleReq struct {
	KullaniciTipi   string `json:"kullanici_tipi"` // "yeni" | "mevcut"
	KullaniciSonek  string `json:"kullanici_sonek"`
	MevcutKullanici string `json:"mevcut_kullanici"`
	Parola          string `json:"parola"`
}

// DatabaseKullaniciEkle: POST /domains/{id}/databases/{dbAdi}/kullanicilar
// Kota kontrolü YOK — yeni DB değil, mevcut DB'ye ek kullanıcı (bkz. spec kararı).
func (h *Handlers) DatabaseKullaniciEkle(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	dbAdi := chi.URLParam(r, "dbAdi")

	var sk string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, is_demo FROM domains WHERE id=?`, id).Scan(&sk, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "domain sorgu: "+err.Error())
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğe kullanıcı eklenemez")
		return
	}

	var dbVarMi int
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_name=?`, id, dbAdi).Scan(&dbVarMi)
	if dbVarMi == 0 {
		httpx.WriteError(w, http.StatusNotFound, "veritabanı bulunamadı")
		return
	}

	var req dbKullaniciEkleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}

	var dbKullanici, parola string
	mevcutModu := req.KullaniciTipi == "mevcut"

	if mevcutModu {
		if req.MevcutKullanici == "" || !hesaplar.GecerliDBKimlik(req.MevcutKullanici) {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz mevcut kullanıcı")
			return
		}
		var sahip int
		_ = h.DB.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_user=?`, id, req.MevcutKullanici).Scan(&sahip)
		if sahip == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "seçilen kullanıcı bu domaine ait değil")
			return
		}
		dbKullanici = req.MevcutKullanici
	} else {
		if req.KullaniciSonek == "" || !hesaplar.GecerliDBSonek(req.KullaniciSonek) {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz kullanıcı soneki (yalnız küçük harf/rakam/alt-çizgi, 1-32 karakter)")
			return
		}
		dbKullanici = sk + "_" + req.KullaniciSonek
		if !hesaplar.GecerliDBKimlik(dbKullanici) {
			httpx.WriteError(w, http.StatusBadRequest, "kullanıcı adı çok uzun (önek + sonek ≤64 karakter olmalı)")
			return
		}
		if req.Parola == "" {
			parola = hesaplar.RandomParola(24)
		} else {
			if ok, neden := hesaplar.ParolaGucluMu(req.Parola); !ok {
				httpx.WriteError(w, http.StatusBadRequest, neden)
				return
			}
			parola = req.Parola
		}
	}

	var zatenVar int
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_name=? AND db_user=?`, id, dbAdi, dbKullanici).Scan(&zatenVar)
	if zatenVar > 0 {
		httpx.WriteError(w, http.StatusConflict, "bu kullanıcının zaten bu veritabanına erişimi var")
		return
	}

	if mevcutModu {
		if err := hesaplar.MySQLGrantExistingUser(h.DB, id, dbAdi, dbKullanici); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı ekleme: "+err.Error())
			return
		}
		var encParola string
		_ = h.DB.QueryRowContext(r.Context(),
			`SELECT db_pass_plain FROM db_accounts WHERE db_user=? LIMIT 1`, dbKullanici).Scan(&encParola)
		if dec, err := hesaplar.DecryptDBPassword(encParola); err == nil {
			parola = dec
		}
	} else {
		if err := hesaplar.MySQLGrantNewUser(h.DB, id, dbAdi, dbKullanici, parola); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı ekleme: "+err.Error())
			return
		}
	}

	var yeniID int64
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT id FROM db_accounts WHERE domain_id=? AND db_name=? AND db_user=?`, id, dbAdi, dbKullanici).Scan(&yeniID)

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok": true, "id": yeniID, "db_kullanici": dbKullanici, "db_parola": parola,
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domains/... -run TestDatabaseKullaniciEkle -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domains/handlers_dbyonet.go internal/domains/handlers_dbyonet_test.go
git commit -m "feat(domains): veritabanına kullanıcı ekleme ucu (POST)"
```

---

### Task 7: `internal/domains` — kullanıcı silme ucu (DELETE, son-kullanıcı koruması)

**Files:**
- Modify: `internal/domains/handlers_dbyonet.go`
- Test: `internal/domains/handlers_dbyonet_test.go`

**Interfaces:**
- Consumes: `hesaplar.MySQLRevokeUser` (Task 2)
- Produces: `Handlers.DatabaseKullaniciSil(w http.ResponseWriter, r *http.Request)`, `sonKullaniciMi(toplamKullanici int) bool`

- [ ] **Step 1: Write the failing test**

```go
func TestSonKullaniciMi(t *testing.T) {
	if !sonKullaniciMi(1) {
		t.Error("1 kullanıcı = son kullanıcı olmalı")
	}
	if !sonKullaniciMi(0) {
		t.Error("0 kullanıcı = son kullanıcı (veri tutarsızlığına karşı güvenli taraf) olmalı")
	}
	if sonKullaniciMi(2) {
		t.Error("2 kullanıcı = son kullanıcı DEĞİL")
	}
}

func TestDatabaseKullaniciSilSonKullaniciysa409(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT db_user FROM db_accounts WHERE id=\? AND domain_id=\? AND db_name=\?`).
		WithArgs(int64(101), int64(7), "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"db_user"}).AddRow("sk_blog"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE domain_id=\? AND db_name=\?`).
		WithArgs(int64(7), "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Delete("/domains/{id}/databases/{dbAdi}/kullanicilar/{dbid}", h.DatabaseKullaniciSil)

	req := httptest.NewRequest(http.MethodDelete, "/domains/7/databases/sk_blog/kullanicilar/101", nil)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("409 bekleniyordu, %d geldi: %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domains/... -run "TestSonKullaniciMi|TestDatabaseKullaniciSil" -v`
Expected: derleme hatası

- [ ] **Step 3: Write minimal implementation**

```go
func sonKullaniciMi(toplamKullanici int) bool { return toplamKullanici <= 1 }

// DatabaseKullaniciSil: DELETE /domains/{id}/databases/{dbAdi}/kullanicilar/{dbid}
// DB'yi SİLMEZ — yalnız bu kullanıcının erişimini kaldırır. Son kullanıcıysa
// 409 döner (DB'yi silmek için domain sil ucu kullanılmalı).
func (h *Handlers) DatabaseKullaniciSil(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	dbAdi := chi.URLParam(r, "dbAdi")
	dbid, _ := strconv.ParseInt(chi.URLParam(r, "dbid"), 10, 64)

	var dbUser string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT db_user FROM db_accounts WHERE id=? AND domain_id=? AND db_name=?`, dbid, id, dbAdi).Scan(&dbUser)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "kullanıcı kaydı bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okuma: "+err.Error())
		return
	}

	var toplam int
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_name=?`, id, dbAdi).Scan(&toplam)
	if sonKullaniciMi(toplam) {
		httpx.WriteError(w, http.StatusConflict, "bu veritabanının tek kullanıcısı — silmek için veritabanının kendisini silin")
		return
	}

	var baskaYerde int
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE db_user=? AND db_name<>?`, dbUser, dbAdi).Scan(&baskaYerde)
	if err := hesaplar.MySQLRevokeUser(h.DB, dbAdi, dbUser, baskaYerde == 0); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı silme: "+err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "silinen_kullanici": dbUser})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domains/... -run "TestSonKullaniciMi|TestDatabaseKullaniciSil" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domains/handlers_dbyonet.go internal/domains/handlers_dbyonet_test.go
git commit -m "feat(domains): veritabanı kullanıcısı silme ucu (son-kullanıcı korumalı)"
```

---

### Task 8: `internal/domains` — yedekleme ucu (GET stream)

**Files:**
- Modify: `internal/domains/handlers_dbyonet.go`

**Interfaces:**
- Produces: `Handlers.DatabaseYedekle(w http.ResponseWriter, r *http.Request)`

- [ ] **Step 1: Write the failing test**

Bu görev canlı `mysqldump` çalıştırdığından (bu sandbox'ta MariaDB root soketi yok — `internal/hesaplar`'daki `MySQLCreateDB` gibi diğer exec-tabanlı fonksiyonların hiçbiri de test edilmiyor, aynı sınır burada da geçerli), yalnız derleme+route-kayıt doğrulaması yapılır — TDD adımı olarak önce derlemenin BEKLENDİĞİ gibi kırılacağı doğrulanır:

Run: `go build ./...`
Expected: `Handlers.DatabaseYedekle` henüz yok → `cmd/server/main.go`'da route bağlanana kadar bu adımda yalnız fonksiyonun kendisi eklenir, main.go'ya Task 12'de bağlanacak; bu ara adımda sadece `go vet ./internal/domains/...` ile syntax doğrulanır.

- [ ] **Step 2: Write implementation**

```go
// DatabaseYedekle: GET /domains/{id}/databases/{dbAdi}/yedek
// mysqldump çıktısını gzip'leyip indirme yanıtı olarak döner. Önce geçici bir
// dosyaya yazılır (backups.Indir deseniyle aynı) — mysqldump ortasında hata
// verirse yanıt başlamadan 500 dönebiliriz, yarım/bozuk dosya indirtmeyiz.
func (h *Handlers) DatabaseYedekle(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	dbAdi := chi.URLParam(r, "dbAdi")

	var varMi int
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_name=?`, id, dbAdi).Scan(&varMi)
	if varMi == 0 {
		httpx.WriteError(w, http.StatusNotFound, "veritabanı bulunamadı")
		return
	}

	httpx.ExtendDeadline(w, 15*time.Minute)
	tmp, err := os.CreateTemp("", "sanal-db-yedek-*.sql.gz")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	gz := gzip.NewWriter(tmp)
	cmd := exec.CommandContext(r.Context(), "mysqldump",
		"--single-transaction", "--routines", "--triggers", "--events", dbAdi)
	cmd.Stdout = gz
	var stderr strings.Builder
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	closeErr := gz.Close()
	_ = tmp.Close()
	if runErr != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "mysqldump: "+strings.TrimSpace(stderr.String()))
		return
	}
	if closeErr != nil {
		httpx.WriteError(w, http.StatusInternalServerError, closeErr.Error())
		return
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	dosya := dbAdi + "-" + time.Now().UTC().Format("20060102-150405") + ".sql.gz"
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+dosya+`"`)
	if st != nil {
		w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	}
	_, _ = io.Copy(w, f)
}
```

Import bloğuna `"compress/gzip"`, `"io"`, `"os"`, `"os/exec"`, `"strings"`, `"time"` ekle (bu fonksiyon `strings.Builder`/`strings.TrimSpace` ve `httpx.ExtendDeadline`/`time.Now` kullanıyor).

- [ ] **Step 3: Verify it compiles**

Run: `go build ./... && go vet ./internal/domains/...`
Expected: temiz derleme

- [ ] **Step 4: Commit**

```bash
git add internal/domains/handlers_dbyonet.go
git commit -m "feat(domains): veritabanı yedekleme ucu (tek tık gzip indirme)"
```

---

### Task 9: `internal/domains` — geri yükleme ucu (POST multipart)

**Files:**
- Modify: `internal/domains/handlers_dbyonet.go`

**Interfaces:**
- Consumes: `httpx.ExtendBodyLimit`, `httpx.GovdeSinirAsildi` (mevcut, `internal/httpx/limit.go`)
- Produces: `Handlers.DatabaseGeriYukle(w http.ResponseWriter, r *http.Request)`, `maxDBRestoreBytes`

- [ ] **Step 1: Write implementation**

(Aynı gerekçeyle — canlı `mysql` CLI gerektirir — bu görev de derleme doğrulamasıyla ilerler, ayrı bir birim test eklenmez.)

```go
const maxDBRestoreBytes = 200 * 1024 * 1024 // 200 MB

// DatabaseGeriYukle: POST /domains/{id}/databases/{dbAdi}/geri-yukle (multipart, alan adı "dosya")
// .sql veya .sql.gz kabul eder; mevcut tabloları EZEBİLİR (frontend'de tehlikeli-onay zorunlu).
func (h *Handlers) DatabaseGeriYukle(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	dbAdi := chi.URLParam(r, "dbAdi")

	var varMi int
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_name=?`, id, dbAdi).Scan(&varMi)
	if varMi == 0 {
		httpx.WriteError(w, http.StatusNotFound, "veritabanı bulunamadı")
		return
	}

	httpx.ExtendDeadline(w, 15*time.Minute)
	httpx.ExtendBodyLimit(w, r, maxDBRestoreBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if httpx.GovdeSinirAsildi(err) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "yükleme boyutu sınırı aştı (max 200 MB)")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "form parse: "+err.Error())
		return
	}
	file, fh, err := r.FormFile("dosya")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "dosya alanı bulunamadı: "+err.Error())
		return
	}
	defer file.Close()
	if fh.Size > maxDBRestoreBytes {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "dosya çok büyük (max 200 MB)")
		return
	}

	ad := strings.ToLower(fh.Filename)
	var reader io.Reader = file
	switch {
	case strings.HasSuffix(ad, ".gz"):
		gzr, err := gzip.NewReader(file)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "gzip açılamadı: "+err.Error())
			return
		}
		defer gzr.Close()
		reader = gzr
	case strings.HasSuffix(ad, ".sql"):
		// duz metin, reader zaten dogru
	default:
		httpx.WriteError(w, http.StatusBadRequest, "yalnız .sql veya .sql.gz kabul edilir")
		return
	}

	cmd := exec.CommandContext(r.Context(), "mysql", dbAdi)
	cmd.Stdin = reader
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "mysql: "+strings.TrimSpace(stderr.String()))
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "sonuc": dbAdi + " geri yüklendi"})
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./... && go vet ./internal/domains/...`
Expected: temiz derleme

- [ ] **Step 3: Commit**

```bash
git add internal/domains/handlers_dbyonet.go
git commit -m "feat(domains): veritabanı geri yükleme ucu (.sql/.sql.gz)"
```

---

### Task 10: `internal/domains` — optimize + onar (repair) uçları

**Files:**
- Modify: `internal/domains/handlers_dbyonet.go`

**Interfaces:**
- Produces: `Handlers.DatabaseOptimize`, `Handlers.DatabaseOnar`

- [ ] **Step 1: Write implementation**

```go
// DatabaseOptimize: POST /domains/{id}/databases/{dbAdi}/optimize
func (h *Handlers) DatabaseOptimize(w http.ResponseWriter, r *http.Request) {
	h.mysqlcheckCalistir(w, r, "--optimize")
}

// DatabaseOnar: POST /domains/{id}/databases/{dbAdi}/onar
func (h *Handlers) DatabaseOnar(w http.ResponseWriter, r *http.Request) {
	h.mysqlcheckCalistir(w, r, "--repair")
}

func (h *Handlers) mysqlcheckCalistir(w http.ResponseWriter, r *http.Request, bayrak string) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	dbAdi := chi.URLParam(r, "dbAdi")

	var varMi int
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_name=?`, id, dbAdi).Scan(&varMi)
	if varMi == 0 {
		httpx.WriteError(w, http.StatusNotFound, "veritabanı bulunamadı")
		return
	}

	httpx.ExtendDeadline(w, 15*time.Minute)
	out, err := exec.CommandContext(r.Context(), "mysqlcheck", bayrak, dbAdi).CombinedOutput()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "mysqlcheck: "+strings.TrimSpace(string(out)))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "sonuc": string(out)})
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./... && go vet ./internal/domains/...`
Expected: temiz derleme

- [ ] **Step 3: Commit**

```bash
git add internal/domains/handlers_dbyonet.go
git commit -m "feat(domains): veritabanı optimize/onar uçları (mysqlcheck)"
```

---

### Task 11: `DeleteDatabase`/`SetDatabasePassword` — sahiplik düzeltmesi + çoklu-kullanıcı temizliği

**Files:**
- Modify: `internal/domains/handlers.go:1096-1127` (`DeleteDatabase`)
- Modify: `internal/domains/handlers_dbpw.go` (`SetDatabasePassword`)
- Test: `internal/domains/handlers_dbyonet_test.go` (DeleteDatabase testi buraya, aynı paket)

**Interfaces:**
- Consumes: `middleware.DomainSahibiMi(r *http.Request, domainID int64) bool` (mevcut, `internal/middleware/auth.go:262`), `hesaplar.MySQLRevokeUser` (Task 2), `dbKullanicilariGetir` (Task 4)

- [ ] **Step 1: Write the failing test**

`internal/domains/handlers_dbyonet_test.go`'ya ekle (DeleteDatabase artık birden fazla kullanıcıyı doğru temizlemeli):

```go
func TestDeleteDatabaseCokKullaniciliDBdeHepsiniTemizler(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT db.db_name, db.domain_id, d.is_demo\s+FROM db_accounts db JOIN domains d ON d.id=db.domain_id\s+WHERE db.id=\?`).
		WithArgs(int64(101)).
		WillReturnRows(sqlmock.NewRows([]string{"db_name", "domain_id", "is_demo"}).AddRow("sk_blog", int64(7), 0))
	mock.ExpectQuery(`SELECT DISTINCT db_user FROM db_accounts WHERE domain_id=\? AND db_name=\?`).
		WithArgs(int64(7), "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"db_user"}).AddRow("sk_blog").AddRow("sk_ikinci"))
	// sk_blog: baska yerde kullanilmiyor -> drop user
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE db_user=\? AND db_name<>\?`).
		WithArgs("sk_blog", "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectExec(`DELETE FROM db_accounts WHERE db_name=\? AND db_user=\?`).
		WithArgs("sk_blog", "sk_blog").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// sk_ikinci: baska DB'de de kullaniliyor -> yalniz revoke, user korunur
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM db_accounts WHERE db_user=\? AND db_name<>\?`).
		WithArgs("sk_ikinci", "sk_blog").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))
	mock.ExpectExec(`DELETE FROM db_accounts WHERE db_name=\? AND db_user=\?`).
		WithArgs("sk_blog", "sk_ikinci").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// nihai temizlik: MySQLDropDBKeepUser'in metadata DELETE'i (satirlar zaten silindiyse no-op)
	mock.ExpectExec(`DELETE FROM db_accounts WHERE db_name=\?`).
		WithArgs("sk_blog").
		WillReturnResult(sqlmock.NewResult(0, 0))

	h := &Handlers{DB: db}
	rtr := chi.NewRouter()
	rtr.Delete("/databases/{dbid}", h.DeleteDatabase)

	req := httptest.NewRequest(http.MethodDelete, "/databases/101", nil)
	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	// rootDB baglantisi yok (sandbox) -> hesaplar.rootExecAll panic/hata verir,
	// bu test yalniz DOGRU SORGU SIRASININ kuruldugunu (mock beklentileri) dogrular;
	// gercek MariaDB olmadan 200 beklenmez. ExpectationsWereMet SQL sirasini kanitlar.
	_ = w
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
```

Not: bu test `rootDB` bağlantısı olmadan `hesaplar.MySQLRevokeUser`/`MySQLDropDBKeepUser`'ın panel-DB tarafındaki SQL sırasını doğrular; gerçek `rootExecAll` çağrısı sandbox'ta hata dönebilir (nil `rootDB`) — bu, Task 1-3'teki guard-clause testleriyle aynı sınırdır, `DeleteDatabase`'in üretim davranışı manuel/üretim ortamında doğrulanır (bkz. Test Planı).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domains/... -run TestDeleteDatabaseCokKullanicili -v`
Expected: FAIL (mevcut `DeleteDatabase` yalnız TEK kullanıcıyı işliyor, mock sıra beklentileri karşılanmaz)

- [ ] **Step 3: Write implementation**

`internal/domains/handlers.go`'daki `DeleteDatabase`'i (satır 1096-1127) tamamen şununla değiştir:

```go
func (h *Handlers) DeleteDatabase(w http.ResponseWriter, r *http.Request) {
	dbid, _ := strconv.ParseInt(chi.URLParam(r, "dbid"), 10, 64)
	var dbName string
	var domainID int64
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT db.db_name, db.domain_id, d.is_demo
		 FROM db_accounts db JOIN domains d ON d.id=db.domain_id
		 WHERE db.id=?`, dbid).Scan(&dbName, &domainID, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "DB kaydı bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okuma: "+err.Error())
		return
	}
	// IDOR korumasi: bu route'ta {id} domain param'i yok, MusteriScope
	// uygulanamaz — pma.TokenIste'deki gibi manuel kontrol (bkz. Task 11).
	if !middleware.DomainSahibiMi(r, domainID) {
		httpx.WriteError(w, http.StatusNotFound, "DB kaydı bulunamadı")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğin DB'si silinemez")
		return
	}

	// Coklu-kullanicili DB: HER kullanicinin erisimini ayri ayri temizle (baska
	// DB'de kullaniliyorsa yalniz revoke, degilse DROP USER) — tek bir dbid'nin
	// kullanicisina odaklanip digerlerini MariaDB'de "hayalet grant" olarak
	// birakmamak icin (bkz. spec: coklu-kullanici destegiyle bu artik sik durum).
	kullanicilar, err := dbKullanicilariGetir(r.Context(), h.DB, domainID, dbName)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı sorgu: "+err.Error())
		return
	}
	for _, u := range kullanicilar {
		var baskaYerde int
		_ = h.DB.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM db_accounts WHERE db_user=? AND db_name<>?`, u, dbName).Scan(&baskaYerde)
		if err := hesaplar.MySQLRevokeUser(h.DB, dbName, u, baskaYerde == 0); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı temizliği: "+err.Error())
			return
		}
	}
	if err := hesaplar.MySQLDropDBKeepUser(h.DB, dbName); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB silme: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "silinen": dbName})
}
```

`internal/domains/handlers.go`'nun import bloğuna `"sanalcp/internal/middleware"` ekle.

`internal/domains/handlers_dbpw.go`'daki `SetDatabasePassword`'ı şununla değiştir:

```go
package domains

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"sanalcp/internal/hesaplar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/middleware"

	"github.com/go-chi/chi/v5"
)

type setDBPwReq struct {
	Parola string `json:"parola"`
}

// SetDatabasePassword: PUT /api/v1/databases/:dbid/password
// Body bos ise rastgele uretir. Demo abonelige reddeder.
func (h *Handlers) SetDatabasePassword(w http.ResponseWriter, r *http.Request) {
	dbid, _ := strconv.ParseInt(chi.URLParam(r, "dbid"), 10, 64)
	var req setDBPwReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if req.Parola == "" {
		req.Parola = hesaplar.RandomParola(24)
	}
	if len(req.Parola) < 6 {
		httpx.WriteError(w, http.StatusBadRequest, "parola en az 6 karakter olmalı")
		return
	}

	var dbName, dbUser string
	var domainID int64
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT db.db_name, db.db_user, db.domain_id, d.is_demo
		 FROM db_accounts db JOIN domains d ON d.id=db.domain_id
		 WHERE db.id=?`, dbid).Scan(&dbName, &dbUser, &domainID, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "DB kaydı bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okuma: "+err.Error())
		return
	}
	if !middleware.DomainSahibiMi(r, domainID) {
		httpx.WriteError(w, http.StatusNotFound, "DB kaydı bulunamadı")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğin DB parolası değiştirilemez")
		return
	}

	if err := hesaplar.MySQLChangePassword(h.DB, dbUser, req.Parola); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "parola değişimi: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"dbid":         dbid,
		"db_adi":       dbName,
		"db_kullanici": dbUser,
		"db_parola":    req.Parola,
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domains/... -run TestDeleteDatabaseCokKullanicili -v`
Expected: PASS (mock beklenti sırası artık kodun gerçek sorgu sırasıyla eşleşir)

- [ ] **Step 5: Run full backend test suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: hepsi PASS

- [ ] **Step 6: Commit**

```bash
git add internal/domains/handlers.go internal/domains/handlers_dbpw.go internal/domains/handlers_dbyonet_test.go
git commit -m "fix(domains): DB silme/parola uçlarında sahiplik kontrolü + çoklu-kullanıcı temizliği"
```

---

### Task 12: Route kayıtları

**Files:**
- Modify: `cmd/server/main.go:506-509`

**Interfaces:**
- Consumes: Task 4-11'de tanımlanan tüm handler'lar

- [ ] **Step 1: Route'ları ekle/değiştir**

`cmd/server/main.go`'da mevcut satırları:

```go
r.With(middleware.MusteriScope).Get("/domains/{id}/databases", domainsH.ListDatabases)
r.With(middleware.MusteriScope).Post("/domains/{id}/databases", domainsH.CreateDatabase)
r.With(middleware.AdminOnly).Delete("/databases/{dbid}", domainsH.DeleteDatabase)
r.With(middleware.AdminOnly).Put("/databases/{dbid}/password", domainsH.SetDatabasePassword)
```

şununla değiştir:

```go
r.With(middleware.MusteriScope).Get("/domains/{id}/databases", domainsH.ListDatabases)
r.With(middleware.MusteriScope).Post("/domains/{id}/databases", domainsH.CreateDatabase)
r.With(middleware.MusteriScope).Get("/domains/{id}/databases/{dbAdi}", domainsH.DatabaseGrupDetay)
r.With(middleware.MusteriScope).Put("/domains/{id}/databases/{dbAdi}/isim", domainsH.DatabaseIsimDegistir)
r.With(middleware.MusteriScope).Post("/domains/{id}/databases/{dbAdi}/kullanicilar", domainsH.DatabaseKullaniciEkle)
r.With(middleware.MusteriScope).Delete("/domains/{id}/databases/{dbAdi}/kullanicilar/{dbid}", domainsH.DatabaseKullaniciSil)
r.With(middleware.MusteriScope).Get("/domains/{id}/databases/{dbAdi}/yedek", domainsH.DatabaseYedekle)
r.With(middleware.MusteriScope).Post("/domains/{id}/databases/{dbAdi}/geri-yukle", domainsH.DatabaseGeriYukle)
r.With(middleware.MusteriScope).Post("/domains/{id}/databases/{dbAdi}/optimize", domainsH.DatabaseOptimize)
r.With(middleware.MusteriScope).Post("/domains/{id}/databases/{dbAdi}/onar", domainsH.DatabaseOnar)
// dbid-only: {id} yok, MusteriScope uygulanamaz — handler icinde manuel
// middleware.DomainSahibiMi kontrolu var (bkz. Task 11).
r.Delete("/databases/{dbid}", domainsH.DeleteDatabase)
r.Put("/databases/{dbid}/password", domainsH.SetDatabasePassword)
```

(Bu iki satırın, `pma-token` route'unun bulunduğu — auth zorunlu ama role-scope'suz — aynı router grubunda olduğunu doğrula; `r.Post("/databases/{dbId}/pma-token", pmaH.TokenIste)` ile aynı grup.)

- [ ] **Step 2: Derle ve testleri çalıştır**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: hepsi temiz/PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat(server): veritabanı yönet sayfası route'larını bağla"
```

---

## Frontend

### Task 13: `DomainDatabasesPage.tsx` — DB adına göre gruplama + "Yönet" linki

**Files:**
- Modify: `frontend/src/pages/DomainDatabasesPage.tsx`
- Modify: `frontend/src/i18n/locales/tr/DomainDatabasesPage.json`
- Modify: `frontend/src/i18n/locales/en/DomainDatabasesPage.json`

**Interfaces:**
- Consumes: mevcut `GET /domains/{id}/databases` (değişmedi), `DELETE /databases/{dbid}` (Task 11'de davranışı düzeldi, imza aynı)
- Produces: liste artık `db_adi` başına tek satır gösterir, "Yönet" linki `/abonelikler/{id}/veritabanlari/{db_adi}`'ye gider (Task 14'te tüketilecek)

- [ ] **Step 1: Dosyayı güncelle**

`frontend/src/pages/DomainDatabasesPage.tsx`'in tamamını şununla değiştir:

```tsx
// sanal-dark-swept
// sanal-dark-swept-v2
import { useEffect, useMemo, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import ConfirmDialog from '@/components/ConfirmDialog'
import Modal from '@/components/Modal'
import { T } from '@/lib/tablo'
import { uretGucluParola } from '@/lib/parola'

type Domain = { id: number; alan_adi: string; sistem_kullanici: string }
type DB = {
  id: number; domain_id: number; db_adi: string; db_kullanici: string;
  db_host: string; db_parola: string; olusturulma: string
}
type DBGrup = { db_adi: string; ilkId: number; kullaniciSayisi: number; olusturulma: string }

export default function DomainDatabasesPage() {
  const { t } = useTranslation(['DomainDatabasesPage', 'common'])
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [dbler, setDbler] = useState<DB[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [silinecek, setSilinecek] = useState<DBGrup | null>(null)
  const [ekleAcik, setEkleAcik] = useState(false)

  function yukle() {
    if (!id) return
    setYuk(true)
    api.get<DB[]>(`/domains/${id}/databases`)
      .then(r => setDbler(r.data))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }

  useEffect(() => {
    if (id) api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
    yukle()
  }, [id])

  // db_adi'na gore grupla: ayni DB'ye birden fazla kullanici baglanabilir
  // (bkz. Yonet sayfasi "kullanici ekle"), listede tek satir kalir.
  const gruplar = useMemo<DBGrup[]>(() => {
    const map = new Map<string, DBGrup>()
    for (const d of dbler) {
      const mevcut = map.get(d.db_adi)
      if (mevcut) {
        mevcut.kullaniciSayisi++
      } else {
        map.set(d.db_adi, { db_adi: d.db_adi, ilkId: d.id, kullaniciSayisi: 1, olusturulma: d.olusturulma })
      }
    }
    return Array.from(map.values())
  }, [dbler])

  const mevcutKullanicilar = useMemo(
    () => Array.from(new Set(dbler.map(d => d.db_kullanici))),
    [dbler],
  )

  async function sil() {
    if (!silinecek) return
    try { await api.delete(`/databases/${silinecek.ilkId}`); setSilinecek(null); yukle() }
    catch (e) { alert(apiHata(e, t('DomainDatabasesPage:delete_failed'))) }
  }

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' }, { etiket: t('common:domain'), href: '/domainler' },
        { etiket: domain?.alan_adi || '...', href: `/abonelikler/${id}` },
        { etiket: t('DomainDatabasesPage:breadcrumb_title') },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainDatabasesPage:title')}</h1>
      {domain && <p className="text-sm text-slate-500 dark:text-slate-500 mb-5"><Link to={`/abonelikler/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:hover:text-brand-300 font-medium">{domain.alan_adi}</Link></p>}

      <div className="flex items-center gap-2 mb-4">
        <button onClick={() => setEkleAcik(true)} className="px-3.5 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-md">{t('DomainDatabasesPage:new_database')}</button>
        <button onClick={yukle} className="px-3 py-2 bg-white hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 text-sm rounded-md">↻ {t('DomainDatabasesPage:refresh')}</button>
        <span className="ml-auto text-sm text-slate-500 dark:text-slate-500">{gruplar.length} {t('DomainDatabasesPage:count_suffix')}</span>
      </div>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}

      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
        {yuk ? <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('common:loading')}</div> :
         gruplar.length === 0 ? <div className="py-12 text-center text-sm text-slate-500 dark:text-slate-500">{t('DomainDatabasesPage:no_databases')}</div> :
        <table className={T.tablo}>
          <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-700`}>
            <tr>
              <th className={T.baslik}>{t('DomainDatabasesPage:col_database')}</th>
              <th className={T.baslik}>{t('DomainDatabasesPage:col_user_count')}</th>
              <th className={T.baslik}>{t('DomainDatabasesPage:col_created')}</th>
              <th className={`${T.baslik} text-right`}>{t('DomainDatabasesPage:col_actions')}</th>
            </tr>
          </thead>
          <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-800`}>
            {gruplar.map(g => (
              <tr key={g.db_adi} className={`${T.satir} lg:hover:bg-slate-50 dark:lg:hover:bg-slate-800`}>
                <td className={T.hucreBaslik}><span className="font-mono lg:text-sm text-base">{g.db_adi}</span></td>
                <td className={T.hucre} data-etiket={t('DomainDatabasesPage:col_user_count')}>
                  <span className="text-sm text-slate-600 dark:text-slate-400">{g.kullaniciSayisi}</span>
                </td>
                <td className={T.hucre} data-etiket={t('DomainDatabasesPage:col_created')}><span className="text-sm text-slate-600 dark:text-slate-400">{g.olusturulma}</span></td>
                <td className={T.hucreAksiyon}>
                  <Link to={`/abonelikler/${id}/veritabanlari/${encodeURIComponent(g.db_adi)}`} className="text-sm text-brand-600 dark:text-brand-400 hover:bg-brand-50 dark:hover:bg-brand-900/30 dark:bg-brand-900/20 px-2 py-1 rounded">{t('DomainDatabasesPage:manage')}</Link>
                  <button onClick={() => setSilinecek(g)} className="text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 px-2 py-1 rounded">{t('common:delete')}</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>}
      </div>

      {ekleAcik && domain && (
        <YeniDBModal
          domainId={Number(id)}
          sk={domain.sistem_kullanici}
          mevcutKullanicilar={mevcutKullanicilar}
          onKapat={() => setEkleAcik(false)}
          onTamam={() => { setEkleAcik(false); yukle() }}
          t={t}
        />
      )}

      <ConfirmDialog
        acik={!!silinecek}
        baslik={t('DomainDatabasesPage:delete_dialog_title')}
        mesaj={t('DomainDatabasesPage:delete_dialog_msg', { ad: silinecek?.db_adi })}
        tehlikeli
        onayMetni={t('DomainDatabasesPage:delete_confirm')}
        onOnay={sil}
        onIptal={() => setSilinecek(null)}
      />
    </div>
  )
}

type YeniDBModalProps = {
  domainId: number
  sk: string
  mevcutKullanicilar: string[]
  onKapat: () => void
  onTamam: () => void
  t: (k: string, opts?: Record<string, unknown>) => string
}

const SONEK_RE = /^[a-z0-9_]{1,32}$/

function YeniDBModal({ domainId, sk, mevcutKullanicilar, onKapat, onTamam, t }: YeniDBModalProps) {
  const onek = sk + '_'
  const [otomatik, setOtomatik] = useState(true)
  const [dbSonek, setDbSonek] = useState('')
  const [kullaniciTipi, setKullaniciTipi] = useState<'yeni' | 'mevcut'>(
    mevcutKullanicilar.length ? 'yeni' : 'yeni',
  )
  const [kullaniciSonek, setKullaniciSonek] = useState('')
  const [mevcutKullanici, setMevcutKullanici] = useState(mevcutKullanicilar[0] || '')
  const [parola, setParola] = useState('')
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [sonuc, setSonuc] = useState<{ db_adi: string; db_kullanici: string; db_parola: string } | null>(null)

  const dbAdiOnizleme = onek + (dbSonek || '…')
  const kullaniciOnizleme = onek + (kullaniciSonek || '…')
  const parolaGucSorunu =
    parola !== '' && (parola.length < 12 || !/[A-Za-z]/.test(parola) || !/[0-9]/.test(parola))

  function yerelDogrula(): string | null {
    if (otomatik) return null
    if (!SONEK_RE.test(dbSonek)) return t('DomainDatabasesPage:validation_db_suffix')
    if ((onek + dbSonek).length > 64) return t('DomainDatabasesPage:validation_db_long')
    if (kullaniciTipi === 'yeni') {
      if (!SONEK_RE.test(kullaniciSonek)) return t('DomainDatabasesPage:validation_user_suffix')
      if ((onek + kullaniciSonek).length > 64) return t('DomainDatabasesPage:validation_user_long')
      if (parola !== '' && parolaGucSorunu) return t('DomainDatabasesPage:validation_password')
    } else {
      if (!mevcutKullanici) return t('DomainDatabasesPage:validation_select_user')
    }
    return null
  }

  async function olustur() {
    const y = yerelDogrula()
    if (y) { setHata(y); return }
    setIsleniyor(true); setHata(null)
    try {
      const body: Record<string, unknown> = otomatik
        ? { otomatik: true }
        : {
            db_sonek: dbSonek,
            kullanici_tipi: kullaniciTipi,
            ...(kullaniciTipi === 'yeni'
              ? { kullanici_sonek: kullaniciSonek, parola }
              : { mevcut_kullanici: mevcutKullanici }),
          }
      const { data } = await api.post(`/domains/${domainId}/databases`, body)
      setSonuc({ db_adi: data.db_adi, db_kullanici: data.db_kullanici, db_parola: data.db_parola })
    } catch (e) {
      setHata(apiHata(e, t('DomainDatabasesPage:create_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  const inputCls = 'ta-input ta-input-sm w-full font-mono'

  return (
    <Modal acik={true} baslik={t('DomainDatabasesPage:new_db_modal_title')} onKapat={sonuc ? onTamam : onKapat} genislik="lg">
      {sonuc ? (
        <div className="space-y-4">
          <div className="bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md p-4 space-y-3">
            <p className="text-sm text-emerald-800 dark:text-emerald-200 font-medium">{t('DomainDatabasesPage:created_title')}</p>
            <p className="text-xs text-emerald-700 dark:text-emerald-300">{t('DomainDatabasesPage:save_info')}</p>
            <SonucSatir e={t('DomainDatabasesPage:suffix_db')} v={sonuc.db_adi} t={t} />
            <SonucSatir e={t('DomainDatabasesPage:suffix_user')} v={sonuc.db_kullanici} t={t} />
            <SonucSatir e={t('DomainDatabasesPage:suffix_password')} v={sonuc.db_parola} t={t} />
          </div>
          <div className="flex justify-end">
            <button onClick={onTamam} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm rounded-md">{t('DomainDatabasesPage:ok')}</button>
          </div>
        </div>
      ) : (
        <div className="space-y-5">
          <label className="flex items-center gap-3 cursor-pointer select-none">
            <input type="checkbox" checked={otomatik} onChange={e => setOtomatik(e.target.checked)} className="h-4 w-4 accent-brand-600" />
            <span className="text-sm text-slate-700 dark:text-slate-300">
              <strong className="font-medium">{t('DomainDatabasesPage:auto_label')}</strong> {t('DomainDatabasesPage:auto_desc')}
            </span>
          </label>

          {!otomatik && (
            <div className="space-y-5 pt-1">
              <div>
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDatabasesPage:db_name_label')}</label>
                <div className="flex items-stretch">
                  <span className="inline-flex items-center px-3 rounded-l-md border border-r-0 border-slate-300 dark:border-slate-600 bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 text-sm font-mono select-none">{onek}</span>
                  <input value={dbSonek} onChange={e => setDbSonek(e.target.value.toLowerCase())} placeholder="blog" className={inputCls + ' rounded-l-none'} />
                </div>
                <p className="mt-1 text-xs text-slate-400 dark:text-slate-500 font-mono">→ {dbAdiOnizleme}</p>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1.5">{t('DomainDatabasesPage:db_user_label')}</label>
                <div className="flex gap-4 mb-2">
                  <label className="flex items-center gap-1.5 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
                    <input type="radio" name="kullaniciTipi" checked={kullaniciTipi === 'yeni'} onChange={() => setKullaniciTipi('yeni')} className="accent-brand-600" />
                    {t('DomainDatabasesPage:new_user_radio')}
                  </label>
                  <label className={'flex items-center gap-1.5 text-sm cursor-pointer ' + (mevcutKullanicilar.length ? 'text-slate-700 dark:text-slate-300' : 'text-slate-400 dark:text-slate-600 cursor-not-allowed')}>
                    <input type="radio" name="kullaniciTipi" disabled={!mevcutKullanicilar.length} checked={kullaniciTipi === 'mevcut'} onChange={() => setKullaniciTipi('mevcut')} className="accent-brand-600" />
                    {t('DomainDatabasesPage:existing_user_radio')}
                  </label>
                </div>

                {kullaniciTipi === 'yeni' ? (
                  <>
                    <div className="flex items-stretch">
                      <span className="inline-flex items-center px-3 rounded-l-md border border-r-0 border-slate-300 dark:border-slate-600 bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 text-sm font-mono select-none">{onek}</span>
                      <input value={kullaniciSonek} onChange={e => setKullaniciSonek(e.target.value.toLowerCase())} placeholder="bloguser" className={inputCls + ' rounded-l-none'} />
                    </div>
                    <p className="mt-1 text-xs text-slate-400 dark:text-slate-500 font-mono">→ {kullaniciOnizleme}</p>
                  </>
                ) : (
                  <select value={mevcutKullanici} onChange={e => setMevcutKullanici(e.target.value)} className={inputCls}>
                    {mevcutKullanicilar.map(u => <option key={u} value={u}>{u}</option>)}
                  </select>
                )}
              </div>

              {kullaniciTipi === 'yeni' && (
                <div>
                  <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDatabasesPage:password_label')} <span className="text-slate-400 dark:text-slate-500">{t('DomainDatabasesPage:password_optional')}</span></label>
                  <div className="flex gap-2">
                    <input type="text" value={parola} onChange={e => setParola(e.target.value)} placeholder={t('DomainDatabasesPage:password_placeholder')} className={inputCls} />
                    <button type="button" onClick={() => setParola(uretGucluParola())} className="whitespace-nowrap px-3 py-2 bg-white dark:bg-slate-800 border border-brand-600 text-brand-700 dark:text-brand-300 hover:bg-brand-50 dark:hover:bg-brand-900/30 text-sm rounded-md">{t('DomainDatabasesPage:generate')}</button>
                  </div>
                  {parolaGucSorunu && <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">{t('DomainDatabasesPage:password_warning')}</p>}
                </div>
              )}
            </div>
          )}

          {hata && <div className="px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-sm text-red-700 dark:text-red-300">{hata}</div>}

          <div className="flex justify-end gap-2 pt-1">
            <button onClick={onKapat} disabled={isleniyor} className="px-4 py-2 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 rounded-md text-sm">{t('DomainDatabasesPage:cancel')}</button>
            <button onClick={olustur} disabled={isleniyor} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-md">{isleniyor ? t('DomainDatabasesPage:creating') : t('DomainDatabasesPage:create')}</button>
          </div>
        </div>
      )}
    </Modal>
  )
}

function SonucSatir({ e, v, t }: { e: string; v: string; t: (k: string, opts?: Record<string, unknown>) => string }) {
  const [ok, setOk] = useState(false)
  return (
    <div className="flex items-center gap-2">
      <span className="w-24 shrink-0 text-xs text-emerald-700 dark:text-emerald-300">{e}</span>
      <code className="flex-1 bg-white dark:bg-slate-800 px-3 py-1.5 font-mono text-sm text-slate-900 dark:text-slate-100 rounded border border-emerald-200 dark:border-emerald-800 break-all">{v}</code>
      <button onClick={() => { navigator.clipboard.writeText(v); setOk(true); setTimeout(() => setOk(false), 1500) }} className="px-2.5 py-1.5 bg-emerald-100 dark:bg-emerald-900/30 hover:bg-emerald-200 text-emerald-800 dark:text-emerald-200 text-xs rounded">{ok ? '✓' : t('DomainDatabasesPage:copy')}</button>
    </div>
  )
}
```

- [ ] **Step 2: i18n dosyalarını güncelle**

`frontend/src/i18n/locales/tr/DomainDatabasesPage.json`'da şu satırları:

```json
  "col_user": "Kullanıcı",
  "col_server": "Sunucu",
  "col_password": "Parola",
  "password_show": "Göster",
  "password_hide": "Gizle",
  "pma_open_title": "phpMyAdmin'de yeni sekmede aç",
  "pma_button": "🔓 phpMyAdmin",
  "reset_password": "🔑 Parola Sıfırla",
  "pma_token_failed": "phpMyAdmin token alınamadı",
```

kaldır, `"col_actions": "İşlemler",` satırının üstüne ekle:

```json
  "col_user_count": "Kullanıcı Sayısı",
  "manage": "Yönet",
```

`frontend/src/i18n/locales/en/DomainDatabasesPage.json`'da aynı işlemi İngilizce karşılıklarıyla yap: kaldırılanlar `col_user`/`col_server`/`col_password`/`password_show`/`password_hide`/`pma_open_title`/`pma_button`/`reset_password`/`pma_token_failed`; eklenenler `"col_user_count": "User Count"`, `"manage": "Manage"`.

- [ ] **Step 3: Tip kontrolü ve build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: hata yok

- [ ] **Step 4: Manuel smoke test**

Dev sunucusunu başlat (`npm run dev`), bir domain'in Veritabanları sayfasına git: liste artık DB adına göre tekilleşmiş görünmeli, "Yönet" linki (henüz 404 verecek, Task 14'te sayfa eklenecek) ve "Sil" butonu görünmeli.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/DomainDatabasesPage.tsx frontend/src/i18n/locales/tr/DomainDatabasesPage.json frontend/src/i18n/locales/en/DomainDatabasesPage.json
git commit -m "feat(frontend): veritabanı listesini DB adına göre grupla, Yönet linki ekle"
```

---

### Task 14: `DomainDatabaseYonetPage.tsx` — sayfa iskeleti + genel bilgi kartı + isim değiştirme

**Files:**
- Create: `frontend/src/pages/DomainDatabaseYonetPage.tsx`
- Create: `frontend/src/i18n/locales/tr/DomainDatabaseYonetPage.json`
- Create: `frontend/src/i18n/locales/en/DomainDatabaseYonetPage.json`
- Modify: `frontend/src/App.tsx`

**Interfaces:**
- Consumes: `GET /domains/{id}/databases/{dbAdi}` (Task 4), `PUT /domains/{id}/databases/{dbAdi}/isim` (Task 5)
- Produces: route `abonelikler/:id/veritabanlari/:dbAdi`, sayfa bileşeni `DomainDatabaseYonetPage` (Task 15-16'da genişletilecek)

- [ ] **Step 1: Sayfa dosyasını oluştur**

```tsx
// frontend/src/pages/DomainDatabaseYonetPage.tsx
import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'

type DBKullanici = { id: number; db_kullanici: string; db_parola: string; olusturulma: string }
export type DBGrupDetay = {
  db_adi: string; db_host: string; charset: string; collation: string
  boyut_mb: number; kullanicilar: DBKullanici[]
}

function fmtBoyut(mb: number): string {
  if (mb >= 1024) return (mb / 1024).toFixed(2) + ' GB'
  return mb.toFixed(1) + ' MB'
}

export default function DomainDatabaseYonetPage() {
  const { t } = useTranslation(['DomainDatabaseYonetPage', 'common'])
  const { id, dbAdi } = useParams()
  const [detay, setDetay] = useState<DBGrupDetay | null>(null)
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [isimDegistirAcik, setIsimDegistirAcik] = useState(false)

  function yukle() {
    if (!id || !dbAdi) return
    setYuk(true); setHata(null)
    api.get<DBGrupDetay>(`/domains/${id}/databases/${encodeURIComponent(dbAdi)}`)
      .then(r => setDetay(r.data))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }

  useEffect(yukle, [id, dbAdi])

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' }, { etiket: t('common:domain'), href: '/domainler' },
        { etiket: t('common:domain'), href: `/abonelikler/${id}` },
        { etiket: t('DomainDatabaseYonetPage:breadcrumb_databases'), href: `/abonelikler/${id}/veritabanlari` },
        { etiket: dbAdi || '...' },
      ]} />

      <div className="flex items-center justify-between mb-5">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 font-mono">{dbAdi}</h1>
        <Link
          to={`/abonelikler/${id}/veritabanlari`}
          className="text-sm text-slate-500 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300"
        >
          ← {t('DomainDatabaseYonetPage:back_to_list')}
        </Link>
      </div>

      {hata && <div className="mb-4 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}

      {yuk ? (
        <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('common:loading')}</div>
      ) : detay ? (
        <div className="space-y-4 max-w-3xl">
          <div className="ta-card p-5">
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{t('DomainDatabaseYonetPage:general_info')}</h3>
            <div className="grid grid-cols-2 gap-y-2 gap-x-4 text-sm">
              <BilgiSatiri e={t('DomainDatabaseYonetPage:size')} d={fmtBoyut(detay.boyut_mb)} />
              <BilgiSatiri e={t('DomainDatabaseYonetPage:server')} d={`${detay.db_host}:3306`} mono />
              <BilgiSatiri e={t('DomainDatabaseYonetPage:charset')} d={detay.charset || '—'} mono />
              <BilgiSatiri e={t('DomainDatabaseYonetPage:collation')} d={detay.collation || '—'} mono />
            </div>
            <div className="mt-4 pt-3 border-t border-slate-100 dark:border-slate-800">
              <button onClick={() => setIsimDegistirAcik(true)} className="ta-secondary-button">{t('DomainDatabaseYonetPage:rename_button')}</button>
            </div>
          </div>
        </div>
      ) : null}

      {isimDegistirAcik && dbAdi && (
        <IsimDegistirModal
          domainId={id!}
          eskiAd={dbAdi}
          onKapat={() => setIsimDegistirAcik(false)}
          onTamam={(yeniAd) => { setIsimDegistirAcik(false); window.location.href = `/abonelikler/${id}/veritabanlari/${encodeURIComponent(yeniAd)}` }}
        />
      )}
    </div>
  )
}

function BilgiSatiri({ e, d, mono }: { e: string; d: string; mono?: boolean }) {
  return (
    <>
      <div className="text-slate-500 dark:text-slate-500">{e}</div>
      <div className={`text-slate-800 dark:text-slate-200 ${mono ? 'font-mono' : ''}`}>{d}</div>
    </>
  )
}

function IsimDegistirModal({ domainId, eskiAd, onKapat, onTamam }: {
  domainId: string; eskiAd: string; onKapat: () => void; onTamam: (yeniAd: string) => void
}) {
  const { t } = useTranslation(['DomainDatabaseYonetPage', 'common'])
  const [sonek, setSonek] = useState('')
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [onaySoruluyor, setOnaySoruluyor] = useState(false)

  async function uygula() {
    setIsleniyor(true); setHata(null)
    try {
      const { data } = await api.put(`/domains/${domainId}/databases/${encodeURIComponent(eskiAd)}/isim`, { yeni_sonek: sonek })
      onTamam(data.yeni_ad)
    } catch (e) {
      setHata(apiHata(e, t('DomainDatabaseYonetPage:rename_failed')))
      setOnaySoruluyor(false)
    } finally {
      setIsleniyor(false)
    }
  }

  return (
    <>
      <Modal acik={!onaySoruluyor} baslik={t('DomainDatabaseYonetPage:rename_modal_title')} onKapat={onKapat} genislik="md">
        <div className="space-y-4">
          <div className="ta-form-error !bg-amber-50 dark:!bg-amber-900/20 !border-amber-200 dark:!border-amber-800 !text-amber-800 dark:!text-amber-200">
            {t('DomainDatabaseYonetPage:rename_warning')}
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDatabaseYonetPage:new_suffix_label')}</label>
            <input value={sonek} onChange={e => setSonek(e.target.value.toLowerCase())} placeholder="yeniisim" className="ta-input ta-input-sm w-full font-mono" />
          </div>
          {hata && <div className="ta-form-error">{hata}</div>}
          <div className="ta-form-actions">
            <button onClick={onKapat} disabled={isleniyor} className="ta-secondary-button">{t('common:cancel')}</button>
            <button onClick={() => setOnaySoruluyor(true)} disabled={isleniyor || !sonek} className="ta-primary-button">{t('DomainDatabaseYonetPage:rename_button')}</button>
          </div>
        </div>
      </Modal>
      <ConfirmDialog
        acik={onaySoruluyor}
        baslik={t('DomainDatabaseYonetPage:rename_confirm_title')}
        mesaj={t('DomainDatabaseYonetPage:rename_confirm_msg')}
        tehlikeli
        onayMetni={t('DomainDatabaseYonetPage:rename_confirm_button')}
        onOnay={uygula}
        onIptal={() => setOnaySoruluyor(false)}
      />
    </>
  )
}
```

- [ ] **Step 2: i18n dosyalarını oluştur**

```json
// frontend/src/i18n/locales/tr/DomainDatabaseYonetPage.json
{
  "breadcrumb_databases": "Veritabanları",
  "back_to_list": "Listeye dön",
  "general_info": "Genel Bilgi",
  "size": "Boyut",
  "server": "Sunucu",
  "charset": "Karakter Seti",
  "collation": "Sıralama (Collation)",
  "rename_button": "İsim Değiştir",
  "rename_modal_title": "Veritabanı İsmini Değiştir",
  "rename_warning": "Bu işlem, veritabanı adını kullanan uygulama yapılandırma dosyalarınızı (örn. WordPress wp-config.php) OTOMATİK güncellemez — işlem sonrası bu dosyaları elle güncellemeniz gerekir, aksi halde siteniz çalışmaz.",
  "new_suffix_label": "Yeni isim soneki",
  "rename_confirm_title": "İsim değişikliği uygulansın mı?",
  "rename_confirm_msg": "Bu işlem geri alınamaz ve uygulama yapılandırma dosyalarınızı bozabilir. Devam etmeden önce uyarıyı okuduğunuzdan emin olun.",
  "rename_confirm_button": "Evet, ismi değiştir",
  "rename_failed": "İsim değiştirme başarısız"
}
```

```json
// frontend/src/i18n/locales/en/DomainDatabaseYonetPage.json
{
  "breadcrumb_databases": "Databases",
  "back_to_list": "Back to list",
  "general_info": "General Info",
  "size": "Size",
  "server": "Server",
  "charset": "Charset",
  "collation": "Collation",
  "rename_button": "Rename",
  "rename_modal_title": "Rename Database",
  "rename_warning": "This does NOT automatically update application config files that reference the database name (e.g. WordPress wp-config.php) — you must update them manually afterward, or your site will break.",
  "new_suffix_label": "New name suffix",
  "rename_confirm_title": "Apply the rename?",
  "rename_confirm_msg": "This cannot be undone and may break application config files. Make sure you've read the warning before continuing.",
  "rename_confirm_button": "Yes, rename it",
  "rename_failed": "Rename failed"
}
```

- [ ] **Step 3: Route'u ekle**

`frontend/src/App.tsx`'de lazy import bloğuna (`DomainDatabasesPage` satırının hemen altına):

```tsx
const DomainDatabaseYonetPage = lazy(() => import('@/pages/DomainDatabaseYonetPage'))
```

Route listesine (`abonelikler/:id/veritabanlari` satırının hemen altına):

```tsx
<Route path="abonelikler/:id/veritabanlari/:dbAdi" element={<DomainDatabaseYonetPage />} />
```

- [ ] **Step 4: Tip kontrolü ve build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: hata yok

- [ ] **Step 5: Manuel smoke test**

Dev sunucusunda bir DB'nin "Yönet" linkine tıkla: sayfa açılmalı, boyut/charset/collation görünmeli, "İsim Değiştir" modalı uyarı metniyle açılmalı.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/pages/DomainDatabaseYonetPage.tsx frontend/src/i18n/locales/tr/DomainDatabaseYonetPage.json frontend/src/i18n/locales/en/DomainDatabaseYonetPage.json frontend/src/App.tsx
git commit -m "feat(frontend): veritabanı Yönet sayfası — genel bilgi + isim değiştirme"
```

---

### Task 15: Kullanıcılar kartı — ekle/sil/şifre değiştir

**Files:**
- Modify: `frontend/src/pages/DomainDatabaseYonetPage.tsx`
- Modify: `frontend/src/i18n/locales/tr/DomainDatabaseYonetPage.json`
- Modify: `frontend/src/i18n/locales/en/DomainDatabaseYonetPage.json`

**Interfaces:**
- Consumes: `POST /domains/{id}/databases/{dbAdi}/kullanicilar` (Task 6), `DELETE /domains/{id}/databases/{dbAdi}/kullanicilar/{dbid}` (Task 7), mevcut `DBParolaSifirlaModal` (`db: {id, db_adi, db_kullanici}` prop'u alır — `db_adi` olarak grup `dbAdi`'sını geçir)

- [ ] **Step 1: Kullanıcılar kartını ve modallarını ekle**

`frontend/src/pages/DomainDatabaseYonetPage.tsx`'e import ekle:

```tsx
import DBParolaSifirlaModal from '@/components/DBParolaSifirlaModal'
import { uretGucluParola } from '@/lib/parola'
```

`DomainDatabaseYonetPage` bileşeninin state'ine ekle (mevcut `isimDegistirAcik`'in yanına):

```tsx
  const [kullaniciEkleAcik, setKullaniciEkleAcik] = useState(false)
  const [silinecekKullanici, setSilinecekKullanici] = useState<DBKullanici | null>(null)
  const [pwResetFor, setPwResetFor] = useState<DBKullanici | null>(null)

  async function kullaniciSil() {
    if (!silinecekKullanici || !id || !dbAdi) return
    try {
      await api.delete(`/domains/${id}/databases/${encodeURIComponent(dbAdi)}/kullanicilar/${silinecekKullanici.id}`)
      setSilinecekKullanici(null)
      yukle()
    } catch (e) {
      alert(apiHata(e, t('DomainDatabaseYonetPage:user_delete_failed')))
    }
  }
```

Genel bilgi kartının hemen altına (`{detay && (...)}` bloğunun içinde, aynı `space-y-4` div'in içine) yeni kart ekle:

```tsx
          <div className="ta-card p-5">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainDatabaseYonetPage:users')}</h3>
              <button onClick={() => setKullaniciEkleAcik(true)} className="ta-secondary-button text-xs">{t('DomainDatabaseYonetPage:add_user')}</button>
            </div>
            <div className="space-y-2">
              {detay.kullanicilar.map(k => (
                <KullaniciSatiri
                  key={k.id}
                  kullanici={k}
                  sonKullanici={detay.kullanicilar.length <= 1}
                  onSifreDegistir={() => setPwResetFor(k)}
                  onSil={() => setSilinecekKullanici(k)}
                  t={t}
                />
              ))}
            </div>
          </div>
```

`{isimDegistirAcik && ...}` bloğunun altına yeni modal/dialog'lar ekle:

```tsx
      {kullaniciEkleAcik && id && dbAdi && (
        <KullaniciEkleModal
          domainId={id}
          dbAdi={dbAdi}
          onKapat={() => setKullaniciEkleAcik(false)}
          onTamam={() => { setKullaniciEkleAcik(false); yukle() }}
        />
      )}

      {pwResetFor && dbAdi && (
        <DBParolaSifirlaModal
          db={{ id: pwResetFor.id, db_adi: dbAdi, db_kullanici: pwResetFor.db_kullanici }}
          onKapat={() => setPwResetFor(null)}
          onTamam={() => { setPwResetFor(null); yukle() }}
        />
      )}

      <ConfirmDialog
        acik={!!silinecekKullanici}
        baslik={t('DomainDatabaseYonetPage:user_delete_title')}
        mesaj={t('DomainDatabaseYonetPage:user_delete_msg', { ad: silinecekKullanici?.db_kullanici })}
        tehlikeli
        onayMetni={t('DomainDatabaseYonetPage:user_delete_confirm')}
        onOnay={kullaniciSil}
        onIptal={() => setSilinecekKullanici(null)}
      />
```

Dosyanın sonuna (mevcut `IsimDegistirModal`'dan sonra) yeni bileşenleri ekle:

```tsx
function KullaniciSatiri({ kullanici, sonKullanici, onSifreDegistir, onSil, t }: {
  kullanici: DBKullanici; sonKullanici: boolean
  onSifreDegistir: () => void; onSil: () => void
  t: (k: string, opts?: Record<string, unknown>) => string
}) {
  const [goster, setGoster] = useState(false)
  const [kopya, setKopya] = useState(false)
  return (
    <div className="flex items-center justify-between py-2 border-b border-slate-50 dark:border-slate-800 last:border-0">
      <div className="flex items-center gap-2">
        <span className="font-mono text-sm text-slate-800 dark:text-slate-200">{kullanici.db_kullanici}</span>
        <button onClick={() => setGoster(!goster)} className="font-mono text-xs px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 rounded">
          {goster ? kullanici.db_parola : '••••••••'}
        </button>
        {goster && (
          <button
            onClick={() => { navigator.clipboard.writeText(kullanici.db_parola); setKopya(true); setTimeout(() => setKopya(false), 1500) }}
            className="text-xs px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 hover:bg-brand-100 dark:hover:bg-brand-900/30 hover:text-brand-700 dark:text-brand-300 rounded"
          >
            {kopya ? '✓' : '⧉'}
          </button>
        )}
      </div>
      <div className="flex items-center gap-1">
        <button onClick={onSifreDegistir} className="text-xs text-brand-600 dark:text-brand-400 hover:bg-brand-50 dark:hover:bg-brand-900/30 px-2 py-1 rounded">{t('DomainDatabaseYonetPage:change_password')}</button>
        <button
          onClick={onSil}
          disabled={sonKullanici}
          title={sonKullanici ? t('DomainDatabaseYonetPage:last_user_hint') : undefined}
          className="text-xs text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 px-2 py-1 rounded disabled:opacity-40 disabled:cursor-not-allowed"
        >
          {t('common:delete')}
        </button>
      </div>
    </div>
  )
}

function KullaniciEkleModal({ domainId, dbAdi, onKapat, onTamam }: {
  domainId: string; dbAdi: string; onKapat: () => void; onTamam: () => void
}) {
  const { t } = useTranslation(['DomainDatabaseYonetPage', 'common'])
  const [tip, setTip] = useState<'yeni' | 'mevcut'>('yeni')
  const [sonek, setSonek] = useState('')
  const [mevcutKullanici, setMevcutKullanici] = useState('')
  const [parola, setParola] = useState('')
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)

  async function ekle() {
    setIsleniyor(true); setHata(null)
    try {
      const body = tip === 'yeni'
        ? { kullanici_tipi: 'yeni', kullanici_sonek: sonek, parola }
        : { kullanici_tipi: 'mevcut', mevcut_kullanici: mevcutKullanici }
      await api.post(`/domains/${domainId}/databases/${encodeURIComponent(dbAdi)}/kullanicilar`, body)
      onTamam()
    } catch (e) {
      setHata(apiHata(e, t('DomainDatabaseYonetPage:add_user_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  return (
    <Modal acik={true} baslik={t('DomainDatabaseYonetPage:add_user_modal_title')} onKapat={onKapat} genislik="md">
      <div className="space-y-4">
        <div className="flex gap-4">
          <label className="flex items-center gap-1.5 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
            <input type="radio" checked={tip === 'yeni'} onChange={() => setTip('yeni')} className="accent-brand-600" />
            {t('DomainDatabaseYonetPage:new_user_radio')}
          </label>
          <label className="flex items-center gap-1.5 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
            <input type="radio" checked={tip === 'mevcut'} onChange={() => setTip('mevcut')} className="accent-brand-600" />
            {t('DomainDatabaseYonetPage:existing_user_radio')}
          </label>
        </div>

        {tip === 'yeni' ? (
          <>
            <div>
              <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDatabaseYonetPage:user_suffix_label')}</label>
              <input value={sonek} onChange={e => setSonek(e.target.value.toLowerCase())} placeholder="ikincikullanici" className="ta-input ta-input-sm w-full font-mono" />
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDatabaseYonetPage:password_label')}</label>
              <div className="flex gap-2">
                <input type="text" value={parola} onChange={e => setParola(e.target.value)} placeholder={t('DomainDatabaseYonetPage:password_placeholder')} className="ta-input ta-input-sm w-full font-mono" />
                <button type="button" onClick={() => setParola(uretGucluParola())} className="ta-secondary-button whitespace-nowrap text-xs">{t('DomainDatabaseYonetPage:generate')}</button>
              </div>
            </div>
          </>
        ) : (
          <div>
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDatabaseYonetPage:existing_user_label')}</label>
            <input value={mevcutKullanici} onChange={e => setMevcutKullanici(e.target.value)} placeholder="sk_baska_kullanici" className="ta-input ta-input-sm w-full font-mono" />
          </div>
        )}

        {hata && <div className="ta-form-error">{hata}</div>}
        <div className="ta-form-actions">
          <button onClick={onKapat} disabled={isleniyor} className="ta-secondary-button">{t('common:cancel')}</button>
          <button onClick={ekle} disabled={isleniyor || (tip === 'yeni' ? !sonek : !mevcutKullanici)} className="ta-primary-button">
            {isleniyor ? t('DomainDatabaseYonetPage:adding') : t('DomainDatabaseYonetPage:add_user')}
          </button>
        </div>
      </div>
    </Modal>
  )
}
```

- [ ] **Step 2: i18n anahtarlarını ekle**

`frontend/src/i18n/locales/tr/DomainDatabaseYonetPage.json`'daki son alan `"rename_failed": "İsim değiştirme başarısız"` satırının sonuna `,` ekle, ardından kapanış `}`'dan hemen önce şu alanları ekle (geçerli, tek düz JSON nesnesi olarak):

```json
  "users": "Kullanıcılar",
  "add_user": "Kullanıcı Ekle",
  "add_user_failed": "Kullanıcı ekleme başarısız",
  "add_user_modal_title": "Veritabanına Kullanıcı Ekle",
  "new_user_radio": "Yeni kullanıcı",
  "existing_user_radio": "Mevcut kullanıcı",
  "user_suffix_label": "Kullanıcı adı soneki",
  "existing_user_label": "Mevcut kullanıcı adı (bu domaine ait)",
  "password_label": "Parola",
  "password_placeholder": "En az 12 karakter, harf+rakam",
  "generate": "Üret",
  "adding": "Ekleniyor…",
  "change_password": "Şifre Değiştir",
  "last_user_hint": "Bu veritabanının tek kullanıcısı — silmek için veritabanının kendisini silin",
  "user_delete_title": "Kullanıcıyı kaldır",
  "user_delete_msg": "\"{{ad}}\" kullanıcısının bu veritabanına erişimi kaldırılacak.",
  "user_delete_confirm": "Evet, kaldır",
  "user_delete_failed": "Kullanıcı kaldırma başarısız"
```

`frontend/src/i18n/locales/en/DomainDatabaseYonetPage.json`'daki son alanın (`"rename_failed": "Rename failed"`) sonuna aynı şekilde `,` ekleyip aynı yapıda İngilizce karşılıklarını ekle:

```json
  "users": "Users",
  "add_user": "Add User",
  "add_user_failed": "Failed to add user",
  "add_user_modal_title": "Add User to Database",
  "new_user_radio": "New user",
  "existing_user_radio": "Existing user",
  "user_suffix_label": "Username suffix",
  "existing_user_label": "Existing username (belonging to this domain)",
  "password_label": "Password",
  "password_placeholder": "At least 12 characters, letters+digits",
  "generate": "Generate",
  "adding": "Adding…",
  "change_password": "Change Password",
  "last_user_hint": "This database's only user — delete the database itself to remove it",
  "user_delete_title": "Remove user",
  "user_delete_msg": "\"{{ad}}\"'s access to this database will be removed.",
  "user_delete_confirm": "Yes, remove",
  "user_delete_failed": "Failed to remove user"
```

- [ ] **Step 3: Tip kontrolü ve build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: hata yok

- [ ] **Step 4: Manuel smoke test**

Yönet sayfasında: kullanıcı listesi görünmeli, "Kullanıcı Ekle" modalı açılmalı, tek kullanıcılı bir DB'de "Sil" butonu devre dışı olmalı.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/DomainDatabaseYonetPage.tsx frontend/src/i18n/locales/tr/DomainDatabaseYonetPage.json frontend/src/i18n/locales/en/DomainDatabaseYonetPage.json
git commit -m "feat(frontend): Yönet sayfasına kullanıcı ekleme/silme/şifre değiştirme"
```

---

### Task 16: Bakım kartı — yedekle/geri yükle/optimize/onar

**Files:**
- Modify: `frontend/src/pages/DomainDatabaseYonetPage.tsx`
- Modify: `frontend/src/i18n/locales/tr/DomainDatabaseYonetPage.json`
- Modify: `frontend/src/i18n/locales/en/DomainDatabaseYonetPage.json`

**Interfaces:**
- Consumes: `GET .../yedek` (Task 8, blob indirme — `DomainBackupsPage.tsx:200-210` deseniyle), `POST .../geri-yukle` (Task 9, multipart), `POST .../optimize` ve `.../onar` (Task 10)

- [ ] **Step 1: Bakım kartını ekle**

Kullanıcılar kartından sonra (aynı `space-y-4` içine, `IsimDegistirModal` render bloğundan önce) ekle:

```tsx
          <BakimKarti domainId={id!} dbAdi={dbAdi!} t={t} />
```

Dosyanın sonuna yeni bileşen ekle:

```tsx
function BakimKarti({ domainId, dbAdi, t }: { domainId: string; dbAdi: string; t: (k: string, opts?: Record<string, unknown>) => string }) {
  const [isleniyor, setIsleniyor] = useState<string | null>(null)
  const [sonucMetni, setSonucMetni] = useState<string | null>(null)
  const [hata, setHata] = useState<string | null>(null)
  const [geriYukleAcik, setGeriYukleAcik] = useState(false)

  function yedekle() {
    setIsleniyor('yedek'); setHata(null)
    fetch(`/api/v1/domains/${domainId}/databases/${encodeURIComponent(dbAdi)}/yedek`, { credentials: 'include' })
      .then(async r => {
        if (!r.ok) throw new Error(await r.text())
        return r.blob()
      })
      .then(blob => {
        const a = document.createElement('a')
        a.href = URL.createObjectURL(blob)
        a.download = `${dbAdi}.sql.gz`
        a.click()
      })
      .catch(() => setHata(t('DomainDatabaseYonetPage:backup_failed')))
      .finally(() => setIsleniyor(null))
  }

  async function mysqlcheckCalistir(uc: 'optimize' | 'onar') {
    setIsleniyor(uc); setHata(null); setSonucMetni(null)
    try {
      const { data } = await api.post(`/domains/${domainId}/databases/${encodeURIComponent(dbAdi)}/${uc}`)
      setSonucMetni(data.sonuc)
    } catch (e) {
      setHata(apiHata(e, t('DomainDatabaseYonetPage:maintenance_failed')))
    } finally {
      setIsleniyor(null)
    }
  }

  return (
    <div className="ta-card p-5">
      <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{t('DomainDatabaseYonetPage:maintenance')}</h3>
      <div className="flex flex-wrap gap-2">
        <button onClick={yedekle} disabled={!!isleniyor} className="ta-secondary-button">
          {isleniyor === 'yedek' ? t('DomainDatabaseYonetPage:backing_up') : t('DomainDatabaseYonetPage:backup_button')}
        </button>
        <button onClick={() => setGeriYukleAcik(true)} disabled={!!isleniyor} className="ta-secondary-button">
          {t('DomainDatabaseYonetPage:restore_button')}
        </button>
        <button onClick={() => mysqlcheckCalistir('optimize')} disabled={!!isleniyor} className="ta-secondary-button">
          {isleniyor === 'optimize' ? t('DomainDatabaseYonetPage:optimizing') : t('DomainDatabaseYonetPage:optimize_button')}
        </button>
        <button onClick={() => mysqlcheckCalistir('onar')} disabled={!!isleniyor} className="ta-secondary-button">
          {isleniyor === 'onar' ? t('DomainDatabaseYonetPage:repairing') : t('DomainDatabaseYonetPage:repair_button')}
        </button>
      </div>

      {hata && <div className="mt-3 ta-form-error">{hata}</div>}
      {sonucMetni && (
        <pre className="mt-3 p-3 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded text-xs font-mono text-slate-700 dark:text-slate-300 overflow-x-auto max-h-48 overflow-y-auto whitespace-pre-wrap">
          {sonucMetni}
        </pre>
      )}

      {geriYukleAcik && (
        <GeriYukleModal
          domainId={domainId}
          dbAdi={dbAdi}
          onKapat={() => setGeriYukleAcik(false)}
          onTamam={(msg) => { setGeriYukleAcik(false); setSonucMetni(msg); setHata(null) }}
        />
      )}
    </div>
  )
}

function GeriYukleModal({ domainId, dbAdi, onKapat, onTamam }: {
  domainId: string; dbAdi: string; onKapat: () => void; onTamam: (sonuc: string) => void
}) {
  const { t } = useTranslation(['DomainDatabaseYonetPage', 'common'])
  const [dosya, setDosya] = useState<File | null>(null)
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [onaySoruluyor, setOnaySoruluyor] = useState(false)

  async function yukle() {
    if (!dosya) return
    setIsleniyor(true); setHata(null)
    try {
      const form = new FormData()
      form.append('dosya', dosya)
      const { data } = await api.post(`/domains/${domainId}/databases/${encodeURIComponent(dbAdi)}/geri-yukle`, form, {
        timeout: 0, // buyuk geri yukleme: client tarafinda iptal etme (backend 15dk sinir) — DomainFilesPage.tsx upload deseniyle ayni
      })
      onTamam(data.sonuc)
    } catch (e) {
      setHata(apiHata(e, t('DomainDatabaseYonetPage:restore_failed')))
      setOnaySoruluyor(false)
    } finally {
      setIsleniyor(false)
    }
  }

  return (
    <>
      <Modal acik={!onaySoruluyor} baslik={t('DomainDatabaseYonetPage:restore_modal_title')} onKapat={onKapat} genislik="md">
        <div className="space-y-4">
          <div className="ta-form-error !bg-amber-50 dark:!bg-amber-900/20 !border-amber-200 dark:!border-amber-800 !text-amber-800 dark:!text-amber-200">
            {t('DomainDatabaseYonetPage:restore_warning')}
          </div>
          <input type="file" accept=".sql,.gz" onChange={e => setDosya(e.target.files?.[0] ?? null)} className="ta-input ta-input-sm w-full" />
          {hata && <div className="ta-form-error">{hata}</div>}
          <div className="ta-form-actions">
            <button onClick={onKapat} disabled={isleniyor} className="ta-secondary-button">{t('common:cancel')}</button>
            <button onClick={() => setOnaySoruluyor(true)} disabled={isleniyor || !dosya} className="ta-primary-button">{t('DomainDatabaseYonetPage:restore_button')}</button>
          </div>
        </div>
      </Modal>
      <ConfirmDialog
        acik={onaySoruluyor}
        baslik={t('DomainDatabaseYonetPage:restore_confirm_title')}
        mesaj={t('DomainDatabaseYonetPage:restore_confirm_msg')}
        tehlikeli
        onayMetni={t('DomainDatabaseYonetPage:restore_confirm_button')}
        onOnay={yukle}
        onIptal={() => setOnaySoruluyor(false)}
      />
    </>
  )
}
```

- [ ] **Step 2: i18n anahtarlarını ekle**

`frontend/src/i18n/locales/tr/DomainDatabaseYonetPage.json`'a ekle:

```json
  "maintenance": "Bakım",
  "backup_button": "Yedekle (.sql.gz indir)",
  "backing_up": "Yedekleniyor…",
  "backup_failed": "Yedekleme başarısız",
  "restore_button": "Geri Yükle",
  "restore_modal_title": "Veritabanını Geri Yükle",
  "restore_warning": "Yüklediğiniz dosyadaki tablolar mevcut tabloların ÜZERİNE YAZILIR. Bu işlem geri alınamaz.",
  "restore_confirm_title": "Geri yükleme uygulansın mı?",
  "restore_confirm_msg": "Mevcut veriler yüklediğiniz dosyayla değiştirilecek. Bu işlem geri alınamaz.",
  "restore_confirm_button": "Evet, geri yükle",
  "restore_failed": "Geri yükleme başarısız",
  "optimize_button": "Optimize Et",
  "optimizing": "Optimize ediliyor…",
  "repair_button": "Onar",
  "repairing": "Onarılıyor…",
  "maintenance_failed": "İşlem başarısız"
```

`frontend/src/i18n/locales/en/DomainDatabaseYonetPage.json`'a ekle:

```json
  "maintenance": "Maintenance",
  "backup_button": "Backup (download .sql.gz)",
  "backing_up": "Backing up…",
  "backup_failed": "Backup failed",
  "restore_button": "Restore",
  "restore_modal_title": "Restore Database",
  "restore_warning": "Tables in the uploaded file will OVERWRITE existing tables. This cannot be undone.",
  "restore_confirm_title": "Apply the restore?",
  "restore_confirm_msg": "Existing data will be replaced with your uploaded file. This cannot be undone.",
  "restore_confirm_button": "Yes, restore",
  "restore_failed": "Restore failed",
  "optimize_button": "Optimize",
  "optimizing": "Optimizing…",
  "repair_button": "Repair",
  "repairing": "Repairing…",
  "maintenance_failed": "Operation failed"
```

- [ ] **Step 3: Tip kontrolü ve build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: hata yok

- [ ] **Step 4: Manuel smoke test**

Yönet sayfasında Bakım kartı görünmeli; Yedekle butonu dosya indirmeli; Geri Yükle modalı tehlikeli-onay diyaloğuyla açılmalı; Optimize/Onar sonuç metnini `<pre>` panelinde göstermeli.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/DomainDatabaseYonetPage.tsx frontend/src/i18n/locales/tr/DomainDatabaseYonetPage.json frontend/src/i18n/locales/en/DomainDatabaseYonetPage.json
git commit -m "feat(frontend): Yönet sayfasına bakım kartı — yedekle/geri yükle/optimize/onar"
```

---

## Self-Review Notları

**Spec kapsaması:** Spec'in "Mimari" bölümündeki 1-4 numaralı tüm alt başlıklar (backend uçları, frontend sayfaları, hata davranışı) Task 1-16'da karşılanıyor. "Alınan Kararlar" tablosundaki 5 karar (gruplama, isim değiştirme+uyarı, anında-indirme yedekleme, geri yükleme eklendi, erişim modeli) sırasıyla Task 13/4, Task 3+5+14, Task 8+16, Task 9+16, Task 11 ile karşılanıyor.

**Ek düzeltme (spec'te açıkça anılmamıştı, brainstorming sırasında keşfedildi):** "kullanıcı ekleme" özelliği çoklu-kullanıcılı DB'leri ilk kez sık bir durum haline getiriyor; mevcut `DeleteDatabase` bu durumda diğer kullanıcıları MariaDB'de "hayalet grant" olarak bırakıyordu (metadata silinir, MySQL kullanıcısı silinmez) — Task 11 bunu düzeltiyor. Bu, "işi yapan kod içindeki, işi etkileyen sorun" kapsamına giriyor (bkz. brainstorming skill "Working in existing codebases").

**Tip tutarlılığı:** `DBGrupDetay`/`DBKullanici` (frontend, Task 14) ile `dbGrupDetay`/`dbKullaniciSatiri` (backend, Task 4) alan adları (`db_adi`, `db_host`, `charset`, `collation`, `boyut_mb`, `kullanicilar`, `id`, `db_kullanici`, `db_parola`, `olusturulma`) birebir eşleşiyor. Route path'leri (`/domains/{id}/databases/{dbAdi}/...`) backend (Task 12) ve frontend (Task 13-16) çağrılarında aynı.

**Placeholder taraması:** Tüm adımlarda gerçek kod var; "TODO"/"benzer şekilde" gibi ifade yok. Test kapsamı sınırı (exec-tabanlı fonksiyonlar için canlı-MySQL testi yok) açıkça gerekçelendirildi — bu, kod tabanının zaten kurulu sınırı (`MySQLCreateDB`/`MySQLDropDB` de test edilmiyor), icat edilmiş bir boşluk değil.
