# Uygulama Kurulum Çerçevesi (1-Tık Uygulamalar) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** WordPress'e özel `internal/wordpress` kodunu değiştirmeden, Joomla/Drupal/Nextcloud/OpenCart gibi uygulamaları da kurulabilir kılacak ortak bir `internal/apps` çerçevesi kurmak; WordPress'i bu çerçeveye bir Adapter ile bağlamak; ilk yeni pilot uygulama olarak PrestaShop'u eklemek; frontend'de tek "Uygulamalar" navigasyonuna geçmek.

**Architecture:** `internal/apps` paketi bir `Uygulama` arayüzü (registry) ve bu arayüzü kullanan ortak HTTP handler'lar (kur/sil/güncelle/listele) tanımlar. `internal/wordpress/adapter.go` (aynı pakette, `wpKomut` gibi unexported yardımcılara doğrudan erişerek) ve yeni `internal/prestashop` paketi bu arayüzü implement edip `init()` içinde kendilerini kaydeder. WordPress'in mevcut `/wordpress/*` route'ları ve handler'ları **hiç değişmez** — `/apps/*` route'ları paralel bir yol açar.

**Tech Stack:** Go 1.25 (backend, chi router, database/sql/MySQL), React+TypeScript+Vite (frontend, react-router-dom v6, react-i18next, axios tabanlı `@/lib/api`), stdlib `archive/zip`/`net/http` (PrestaShop indirme).

**Spec:** `docs/superpowers/specs/2026-08-26-uygulama-cercevesi-design.md`

## Global Constraints

- Go toolchain: bu makinede `go` PATH'i eski (1.24.4) — tüm `go build`/`go test`/`go vet` komutları `/usr/local/go/bin/go` (1.26.7) ile çalıştırılmalı (bkz. proje hafızası "SanalCP Go toolchain").
- `internal/wordpress`'in mevcut dosyalarında (`wordpress.go`, `toolkit.go`, `bakim.go`) **hiçbir satır değiştirilmez** — yalnız yeni `adapter.go`/`adapter_test.go` eklenir. Bu, spec'in "Alınan Kararlar" tablosundaki en kritik karar.
- Yeni Go kodu projenin Türkçe tanımlayıcı kuralına uyar (mevcut `internal/wordpress`, `internal/hesaplar` vb. ile aynı isimlendirme dili).
- Kullanıcı girdisi güvenlik desenleri (alt dizin regex, DB adı guard, SKGecerli, demo-abonelik reddi) `internal/wordpress/wordpress.go`'daki karşılıklarıyla **birebir aynı davranışta** olmalı — bunlar güvenlik kritik, sadeleştirilmeden taşınır.
- Commit her görev sonunda; `go vet ./...` ve ilgili paketin testleri her commit'ten önce yeşil olmalı.
- Release/version bump ve `git push` bu planın kapsamında DEĞİL — kullanıcı ayrıca isteyecek.

---

## Task 1: `internal/apps` — türler ve registry

**Files:**
- Create: `internal/apps/apps.go`
- Test: `internal/apps/apps_test.go`

**Interfaces:**
- Produces: `apps.FormAlan{Anahtar, Etiket, Tur, Zorunlu, YerTutucu}`, `apps.KurulumIstek{DomainID, SK, AlanAdi, SSL, Hedef, URL, DBAdi, DBKullanici, DBParola, Alanlar}`, `apps.KurulumSonuc{SiteURL, AdminURL, AdminKullanici, AdminParola, Surum, Ekstra}`, `apps.Kurulum{Dizin, Surum, SonSurum, Durum, SiteURL, AdminURL, KurulumTarihi}`, `apps.Uygulama` arayüzü, `apps.Kaydet(u Uygulama)`, `apps.Bul(slug string) (Uygulama, bool)`, `apps.Hepsi() []Uygulama`.

- [ ] **Step 1: Write the failing test**

`internal/apps/apps_test.go`:
```go
package apps

import (
	"context"
	"testing"
)

type sahteUygulama struct {
	slug string
}

func (s sahteUygulama) Slug() string                { return s.slug }
func (s sahteUygulama) Ad() string                  { return "Sahte " + s.slug }
func (s sahteUygulama) DBOnEki() string              { return s.slug }
func (s sahteUygulama) MarkerDosya() string          { return "marker.txt" }
func (s sahteUygulama) FormAlanlari() []FormAlan     { return nil }
func (s sahteUygulama) GuncelleDesteklenir() bool    { return false }
func (s sahteUygulama) Kur(ctx context.Context, i KurulumIstek) (KurulumSonuc, error) {
	return KurulumSonuc{}, nil
}
func (s sahteUygulama) Bilgi(ctx context.Context, sk, dizin, url string) (Kurulum, error) {
	return Kurulum{}, nil
}
func (s sahteUygulama) Guncelle(ctx context.Context, sk, dizin string) error { return nil }
func (s sahteUygulama) DBAdiOku(dizin string) (string, bool)                { return "", false }

func TestKaydetBulHepsi(t *testing.T) {
	// Registry paket-seviyesi global olduğu için test isimlerini benzersiz
	// slug'larla izole ediyoruz (paralel testlerde çakışma olmasın diye t.Parallel YOK).
	Kaydet(sahteUygulama{slug: "test-tur-a"})
	Kaydet(sahteUygulama{slug: "test-tur-b"})

	if _, ok := Bul("test-tur-yok"); ok {
		t.Fatal("kayıtlı olmayan tür bulunmamalı")
	}
	u, ok := Bul("test-tur-a")
	if !ok || u.Slug() != "test-tur-a" {
		t.Fatalf("test-tur-a bulunamadı veya yanlış: %+v ok=%v", u, ok)
	}

	var gorulenA, gorulenB bool
	for _, u := range Hepsi() {
		switch u.Slug() {
		case "test-tur-a":
			gorulenA = true
		case "test-tur-b":
			gorulenB = true
		}
	}
	if !gorulenA || !gorulenB {
		t.Fatalf("Hepsi() her iki kayıtlı türü de içermeli (a=%v b=%v)", gorulenA, gorulenB)
	}
}

func TestKaydetIdempotent(t *testing.T) {
	Kaydet(sahteUygulama{slug: "test-tur-c"})
	oncekiUzunluk := len(Hepsi())
	Kaydet(sahteUygulama{slug: "test-tur-c"}) // aynı slug'ı ikinci kez kaydetmek listeyi büyütmemeli
	if len(Hepsi()) != oncekiUzunluk {
		t.Fatalf("aynı slug'ın tekrar kaydı Hepsi() uzunluğunu değiştirmemeli: önce=%d sonra=%d", oncekiUzunluk, len(Hepsi()))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/usr/local/go/bin/go test ./internal/apps/... -run TestKaydet -v`
Expected: FAIL (derleme hatası — `apps` paketi/tipler henüz yok).

- [ ] **Step 3: Write minimal implementation**

`internal/apps/apps.go`:
```go
// Package apps: 1-tık uygulama kurulum çerçevesi (WordPress, PrestaShop, ...).
// Her uygulama türü Uygulama arayüzünü implement edip init() içinde Kaydet ile
// kendini kaydeder; ortak HTTP handler'lar (handlers.go) bu registry üzerinden çalışır.
package apps

import (
	"context"
	"sync"
)

// FormAlan: bir uygulama türünün kurulum formunda istediği tek bir alan
// (frontend'e /domains/{id}/apps/turler'dan dinamik form şeması olarak gider).
type FormAlan struct {
	Anahtar   string `json:"anahtar"`
	Etiket    string `json:"etiket"`
	Tur       string `json:"tur"` // "text" | "email" | "password"
	Zorunlu   bool   `json:"zorunlu"`
	YerTutucu string `json:"yer_tutucu,omitempty"`
}

// KurulumIstek: ortak katman (handlers.go) hedef dizini oluşturup DB'yi
// hazırladıktan SONRA Uygulama.Kur'a geçirdiği girdi. Hedef zaten var + chown'lanmış,
// DBAdi/DBKullanici/DBParola zaten oluşturulmuş bir MySQL veritabanına ait.
type KurulumIstek struct {
	DomainID    int64
	SK          string
	AlanAdi     string
	SSL         bool
	Hedef       string
	URL         string
	DBAdi       string
	DBKullanici string
	DBParola    string
	Alanlar     map[string]string
}

// KurulumSonuc: Uygulama.Kur'un başarılı dönüşü.
type KurulumSonuc struct {
	SiteURL        string
	AdminURL       string
	AdminKullanici string
	AdminParola    string
	Surum          string
	Ekstra         map[string]string
}

// Kurulum: tek bir kurulumun anlık durumu (liste taramalarında döner).
// Dizin alanı ORTAK KATMAN tarafından doldurulur — Uygulama.Bilgi implementasyonları
// bu alanı boş bırakmalı, dönen değer yok sayılır.
type Kurulum struct {
	Dizin         string `json:"dizin"`
	Surum         string `json:"surum"`
	SonSurum      string `json:"son_surum"`
	Durum         string `json:"durum"` // "guncel" | "eski" | "bilinmiyor"
	SiteURL       string `json:"site_url"`
	AdminURL      string `json:"admin_url"`
	KurulumTarihi string `json:"kurulum_tarihi"`
}

// Uygulama: her 1-tık uygulama türünün implement ettiği arayüz.
type Uygulama interface {
	Slug() string      // "wordpress", "prestashop" — route parametresi
	Ad() string         // görünen ad ("WordPress", "PrestaShop")
	DBOnEki() string     // db_accounts'ta kullanılacak DB adı/kullanıcı öneki (ör. "wp" → wp_xxxx / wpu_xxxx)
	MarkerDosya() string // kurulu tespiti için hedef dizindeki göreli yol (ör. "wp-config.php")
	FormAlanlari() []FormAlan
	GuncelleDesteklenir() bool

	Kur(ctx context.Context, i KurulumIstek) (KurulumSonuc, error)

	// Bilgi: url ortak katmanca hesaplanmış site kök adresidir (scheme+alanadi+altdizin);
	// driver kendi admin yolunu buna ekler (ör. url+"/wp-admin").
	Bilgi(ctx context.Context, sk, dizin, url string) (Kurulum, error)

	Guncelle(ctx context.Context, sk, dizin string) error // GuncelleDesteklenir()==false ise hiç çağrılmaz

	// DBAdiOku: dizin'deki config dosyasından DB adını okur VE bu ismin driver'ın
	// kendi ad-deseniyle (DBOnEki()+"_"+...) uyuştuğunu doğrular; ikisi de
	// sağlanmazsa bulundu=false döner (silmede yanlış DB'nin DROP edilmesine karşı
	// ilk katman — ikincisi apps.Handlers.Sil'deki db_accounts sahiplik kontrolü).
	DBAdiOku(dizin string) (dbAdi string, bulundu bool)
}

var (
	kayitliMu sync.RWMutex
	kayitli   = map[string]Uygulama{}
	sira      []string // eklenme sırası — Hepsi() deterministik dönsün diye
)

func Kaydet(u Uygulama) {
	kayitliMu.Lock()
	defer kayitliMu.Unlock()
	slug := u.Slug()
	if _, varMi := kayitli[slug]; !varMi {
		sira = append(sira, slug)
	}
	kayitli[slug] = u
}

func Bul(slug string) (Uygulama, bool) {
	kayitliMu.RLock()
	defer kayitliMu.RUnlock()
	u, ok := kayitli[slug]
	return u, ok
}

func Hepsi() []Uygulama {
	kayitliMu.RLock()
	defer kayitliMu.RUnlock()
	out := make([]Uygulama, 0, len(sira))
	for _, s := range sira {
		out = append(out, kayitli[s])
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `/usr/local/go/bin/go test ./internal/apps/... -v`
Expected: PASS (`TestKaydetBulHepsi`, `TestKaydetIdempotent`).

- [ ] **Step 5: Commit**

```bash
git add internal/apps/apps.go internal/apps/apps_test.go
git commit -m "feat(apps): uygulama kurulum çerçevesi — registry ve tipler"
```

---

## Task 2: `internal/apps` — ortak yardımcılar

**Files:**
- Create: `internal/apps/helpers.go`
- Test: `internal/apps/helpers_test.go`

**Interfaces:**
- Consumes: (yok — bu görev saf yardımcı fonksiyonlar)
- Produces: `zatenKuruluMu(hedef, marker string) (string, bool)`, `cozDizin(sk, dizinStr, marker string) (string, error)`, `randSlug() string`, paket seviyesi `reAltDizin`, `reEmail *regexp.Regexp`.

- [ ] **Step 1: Write the failing test**

`internal/apps/helpers_test.go` (WordPress'teki `wpinstall_guard_test.go` deseninin genellenmiş hali):
```go
package apps

import (
	"os"
	"path/filepath"
	"testing"
)

func TestZatenKuruluMu(t *testing.T) {
	t.Run("olmayan dizin temiz", func(t *testing.T) {
		yol := filepath.Join(t.TempDir(), "yok")
		if _, kurulu := zatenKuruluMu(yol, "marker.txt"); kurulu {
			t.Fatal("olmayan dizin temiz sayılmalı")
		}
	})

	t.Run("bos dizin temiz", func(t *testing.T) {
		if _, kurulu := zatenKuruluMu(t.TempDir(), "marker.txt"); kurulu {
			t.Fatal("boş dizin temiz sayılmalı")
		}
	})

	t.Run("sadece placeholder temiz", func(t *testing.T) {
		d := t.TempDir()
		for _, f := range []string{"index.html", "favicon.ico", ".htaccess", "robots.txt"} {
			if err := os.WriteFile(filepath.Join(d, f), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if _, kurulu := zatenKuruluMu(d, "marker.txt"); kurulu {
			t.Fatal("yalnız placeholder dosyalı dizin temiz sayılmalı")
		}
	})

	t.Run("marker dosyasi bloklar", func(t *testing.T) {
		d := t.TempDir()
		if err := os.WriteFile(filepath.Join(d, "marker.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		msg, kurulu := zatenKuruluMu(d, "marker.txt")
		if !kurulu || msg == "" {
			t.Fatalf("marker dosyası mevcut → kurulu:true beklenir (msg=%q kurulu=%v)", msg, kurulu)
		}
	})

	t.Run("baska icerik bloklar", func(t *testing.T) {
		d := t.TempDir()
		if err := os.WriteFile(filepath.Join(d, "baska.php"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, kurulu := zatenKuruluMu(d, "marker.txt"); !kurulu {
			t.Fatal("dolu dizin kurulu:true olmalı — mevcut içerik ezilmemeli")
		}
	})
}

func TestCozDizin(t *testing.T) {
	sk := "test_sk_apps"
	root := "/home/" + sk + "/public_html"
	_ = root // gerçek dosya sistemi kullanılmıyor — sadece yol-dışına-çıkma testleri

	t.Run("kok disina cikma engellenir", func(t *testing.T) {
		if _, err := cozDizin(sk, "../../etc/passwd", "marker.txt"); err == nil {
			t.Fatal("kök dizin dışına çıkan yol reddedilmeli")
		}
	})
}

func TestRandSlug(t *testing.T) {
	a := randSlug()
	b := randSlug()
	if len(a) != 8 || len(b) != 8 {
		t.Fatalf("randSlug 8 hex karakter dönmeli: a=%q(%d) b=%q(%d)", a, len(a), b, len(b))
	}
	if a == b {
		t.Fatal("iki ardışık randSlug çağrısı aynı değeri dönmemeli (rastgelelik bozuk)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/usr/local/go/bin/go test ./internal/apps/... -run "TestZatenKuruluMu|TestCozDizin|TestRandSlug" -v`
Expected: FAIL (derleme hatası — fonksiyonlar henüz yok).

- [ ] **Step 3: Write minimal implementation**

`internal/apps/helpers.go`:
```go
package apps

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reAltDizin = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]{0,30}[a-z0-9])?$`)
	reEmail    = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

// zatenKuruluMu: hedef dizinde zaten bir kurulum/içerik var mı? Varsa (mesaj, true)
// döner ve kurulum DURDURULUR (mevcut içerik asla ezilmez). WordPress'teki
// kurulumZatenVar ile birebir aynı mantık, marker dosya adı parametreleştirilmiş.
func zatenKuruluMu(hedef, marker string) (string, bool) {
	if _, err := os.Stat(filepath.Join(hedef, marker)); err == nil {
		return "bu dizinde zaten bir kurulum var (mevcut kurulum korunuyor)", true
	}
	entries, err := os.ReadDir(hedef)
	if err != nil {
		return "", false // dizin yok = temiz
	}
	for _, e := range entries {
		switch strings.ToLower(e.Name()) {
		case "index.html", "index.htm", "favicon.ico", "robots.txt",
			"error_log", ".user.ini", ".well-known", "cgi-bin",
			".ftpquota", ".htaccess", ".git", ".gitkeep":
			continue
		}
		return "hedef dizin boş değil — mevcut içerik/kurulum korunuyor (üzerine yazılmaz). Boş bir alt dizin seçin", true
	}
	return "", false
}

// cozDizin: {dizin} kullanıcı girdisini domain'in public_html'i İÇİNDE güvenli
// bir mutlak yola çözer + marker dosyasının orada var olduğunu doğrular.
func cozDizin(sk, dizinStr, marker string) (string, error) {
	root := "/home/" + sk + "/public_html"
	d := strings.TrimPrefix(strings.TrimSpace(dizinStr), "/ (kök)")
	rel := strings.Trim(strings.TrimSpace(d), "/")
	dir := root
	if rel != "" && rel != "(kök)" {
		dir = filepath.Join(root, rel)
	}
	clean := filepath.Clean(dir)
	if clean != root && !strings.HasPrefix(clean, root+"/") {
		return "", fmt.Errorf("yol domain dizini dışında")
	}
	if _, err := os.Stat(filepath.Join(clean, marker)); err != nil {
		return "", fmt.Errorf("bu dizinde kurulum bulunamadı")
	}
	return clean, nil
}

// randSlug: DB adı/kullanıcı adı için 8 hex karakterlik rastgele parça
// (WordPress'teki randSlug ile birebir aynı).
func randSlug() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand okunamadı, slug üretilemiyor: " + err.Error())
	}
	return hex.EncodeToString(b)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `/usr/local/go/bin/go test ./internal/apps/... -v`
Expected: PASS (tüm testler, `TestCozDizin`'in "kök disina cikma engellenir" alt testi dahil).

- [ ] **Step 5: Commit**

```bash
git add internal/apps/helpers.go internal/apps/helpers_test.go
git commit -m "feat(apps): ortak yardımcılar (zatenKuruluMu, cozDizin, randSlug)"
```

---

## Task 3: `internal/apps` — `Handlers` + `Kur`

**Files:**
- Create: `internal/apps/handlers.go`
- Test: `internal/apps/handlers_test.go`

**Interfaces:**
- Consumes: `apps.Uygulama`, `apps.Bul`, `zatenKuruluMu`, `reAltDizin`, `reEmail`, `randSlug` (Task 1-2), `hesaplar.MySQLCreateDB(db *sql.DB, domainID int64, dbName, dbUser, dbPass string) error`, `hesaplar.RandomParola(n int) string`, `adlar.SKGecerli(sk string) bool`, `httpx.WriteJSON`/`WriteError`, `middleware.MusteriScope`.
- Produces: `apps.Handlers{DB *sql.DB}`, `(h *Handlers) Kur(w, r)`, `(h *Handlers) domain(r) (id int64, sk, alanAdi string, ssl, demo, ok bool)`, `scheme(ssl bool) string`.

- [ ] **Step 1: Write the failing test**

Bu görevde gerçek MySQL bağlantısı gerektiren `MySQLCreateDB` çağrısı olduğu için, `Kur` handler'ının **tam HTTP akışı** (WordPress modülündeki gibi) burada birim test edilmiyor — proje genelinde bu tür uçlar manuel/entegrasyon testiyle doğrulanıyor (bkz. spec Test Planı, `internal/wordpress`'in kendi `Kur`/`Sil` handler'ları da birim test edilmemiş, sadece alt-fonksiyonları). Bu görevde birim testi, **alan doğrulama mantığının saf kısmını** (`FormAlanlari` zorunlu/email kontrolü) ayıklayıp test edilebilir kılıyoruz:

`internal/apps/handlers_test.go`:
```go
package apps

import "testing"

func TestAlanlariDogrula(t *testing.T) {
	alanlar := []FormAlan{
		{Anahtar: "ad", Etiket: "Ad", Tur: "text", Zorunlu: true},
		{Anahtar: "eposta", Etiket: "E-posta", Tur: "email", Zorunlu: true},
		{Anahtar: "notlar", Etiket: "Notlar", Tur: "text", Zorunlu: false},
	}

	t.Run("zorunlu alan eksikse hata doner", func(t *testing.T) {
		girdi := map[string]string{"eposta": "a@b.com"}
		if _, hata := alanlariDogrula(alanlar, girdi); hata == "" {
			t.Fatal("zorunlu 'ad' eksik — hata dönmeli")
		}
	})

	t.Run("gecersiz email hata doner", func(t *testing.T) {
		girdi := map[string]string{"ad": "x", "eposta": "gecersiz"}
		if _, hata := alanlariDogrula(alanlar, girdi); hata == "" {
			t.Fatal("geçersiz e-posta — hata dönmeli")
		}
	})

	t.Run("opsiyonel alan bos gecebilir", func(t *testing.T) {
		girdi := map[string]string{"ad": "x", "eposta": "a@b.com"}
		temiz, hata := alanlariDogrula(alanlar, girdi)
		if hata != "" {
			t.Fatalf("geçerli girdi hata dönmemeli: %q", hata)
		}
		if temiz["notlar"] != "" {
			t.Fatalf("boş opsiyonel alan boş kalmalı, geldi: %q", temiz["notlar"])
		}
	})

	t.Run("bosluklar kirpilir", func(t *testing.T) {
		girdi := map[string]string{"ad": "  x  ", "eposta": " a@b.com "}
		temiz, hata := alanlariDogrula(alanlar, girdi)
		if hata != "" {
			t.Fatalf("geçerli girdi hata dönmemeli: %q", hata)
		}
		if temiz["ad"] != "x" || temiz["eposta"] != "a@b.com" {
			t.Fatalf("boşluklar kırpılmalı: %+v", temiz)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/usr/local/go/bin/go test ./internal/apps/... -run TestAlanlariDogrula -v`
Expected: FAIL (derleme hatası — `alanlariDogrula` ve `Handlers` henüz yok).

- [ ] **Step 3: Write minimal implementation**

`internal/apps/handlers.go`:
```go
package apps

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"sanalcp/internal/adlar"
	"sanalcp/internal/hesaplar"
	"sanalcp/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Handlers struct{ DB *sql.DB }

// kurulumKilit: eşzamanlı kurulumları hedef-yola göre serileştirir (çift-tık koruması).
// Değer önemsiz; anahtar = mutlak hedef dizin.
var kurulumKilit sync.Map

func scheme(ssl bool) string {
	if ssl {
		return "https://"
	}
	return "http://"
}

func (h *Handlers) domain(r *http.Request) (id int64, sk, alanAdi string, ssl, demo, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var cert string
	var isDemo int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, alan_adi, COALESCE(cert_path,''), COALESCE(is_demo,0) FROM domains WHERE id=?`, id).
		Scan(&sk, &alanAdi, &cert, &isDemo); err != nil {
		return id, "", "", false, false, false
	}
	return id, sk, alanAdi, cert != "", isDemo == 1, true
}

// alanlariDogrula: FormAlanlari şemasına göre girdiyi doğrular + kırpar. Hata boşsa geçerli.
func alanlariDogrula(alanlar []FormAlan, girdi map[string]string) (map[string]string, string) {
	temiz := map[string]string{}
	for _, fa := range alanlar {
		v := strings.TrimSpace(girdi[fa.Anahtar])
		temiz[fa.Anahtar] = v
		if fa.Zorunlu && v == "" {
			return nil, fa.Etiket + " gerekli"
		}
		if fa.Tur == "email" && v != "" && !reEmail.MatchString(v) {
			return nil, "geçersiz e-posta: " + fa.Etiket
		}
	}
	return temiz, ""
}

// POST /domains/{id}/apps/{tur}/kur
func (h *Handlers) Kur(w http.ResponseWriter, r *http.Request) {
	tur := chi.URLParam(r, "tur")
	u, bulunduTur := Bul(tur)
	if !bulunduTur {
		httpx.WriteError(w, http.StatusNotFound, "bilinmeyen uygulama türü")
		return
	}
	id, sk, alanAdi, ssl, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	if !adlar.SKGecerli(sk) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz kullanıcı")
		return
	}
	var req struct {
		AltDizin string            `json:"alt_dizin"`
		Alanlar  map[string]string `json:"alanlar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	req.AltDizin = strings.Trim(strings.TrimSpace(req.AltDizin), "/")
	if req.AltDizin != "" && !reAltDizin.MatchString(req.AltDizin) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz alt dizin (küçük harf/rakam/-)")
		return
	}
	temizAlanlar, hataMsg := alanlariDogrula(u.FormAlanlari(), req.Alanlar)
	if hataMsg != "" {
		httpx.WriteError(w, http.StatusBadRequest, hataMsg)
		return
	}

	root := "/home/" + sk + "/public_html"
	hedef := root
	if req.AltDizin != "" {
		hedef = filepath.Join(root, req.AltDizin)
	}
	if _, sur := kurulumKilit.LoadOrStore(hedef, struct{}{}); sur {
		httpx.WriteError(w, http.StatusConflict, "bu dizine kurulum zaten sürüyor — lütfen bekleyin")
		return
	}
	defer kurulumKilit.Delete(hedef)
	if msg, kurulu := zatenKuruluMu(hedef, u.MarkerDosya()); kurulu {
		httpx.WriteError(w, http.StatusConflict, msg)
		return
	}
	if err := os.MkdirAll(hedef, 0o755); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "hedef dizin oluşturulamadı")
		return
	}
	_ = exec.Command("chown", "-R", sk+":"+sk, hedef).Run()
	_ = exec.Command("restorecon", "-R", hedef).Run()

	slug := randSlug()
	dbOnEk := u.DBOnEki()
	dbName := dbOnEk + "_" + slug
	dbUser := dbOnEk + "u_" + slug
	dbPass := hesaplar.RandomParola(24)
	if err := hesaplar.MySQLCreateDB(h.DB, id, dbName, dbUser, dbPass); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "veritabanı oluşturulamadı: "+err.Error())
		return
	}
	temizle := func(asama string, err error) {
		_, _ = h.DB.Exec("DROP DATABASE IF EXISTS `" + dbName + "`")
		_, _ = h.DB.Exec("DROP USER IF EXISTS '" + dbUser + "'@'localhost'")
		if req.AltDizin != "" {
			_ = os.RemoveAll(hedef)
		}
		msg := err.Error()
		if len(msg) > 600 {
			msg = msg[len(msg)-600:]
		}
		httpx.WriteError(w, http.StatusInternalServerError, asama+" başarısız: "+msg)
	}

	url := scheme(ssl) + alanAdi
	if req.AltDizin != "" {
		url += "/" + req.AltDizin
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	sonuc, err := u.Kur(ctx, KurulumIstek{
		DomainID: id, SK: sk, AlanAdi: alanAdi, SSL: ssl,
		Hedef: hedef, URL: url,
		DBAdi: dbName, DBKullanici: dbUser, DBParola: dbPass,
		Alanlar: temizAlanlar,
	})
	if err != nil {
		temizle(u.Ad()+" kurulumu", err)
		return
	}
	_ = exec.Command("chown", "-R", sk+":"+sk, hedef).Run()
	_ = exec.Command("restorecon", "-R", hedef).Run()

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "tur": u.Slug(),
		"site_url": sonuc.SiteURL, "admin_url": sonuc.AdminURL,
		"admin_kullanici": sonuc.AdminKullanici, "admin_parola": sonuc.AdminParola,
		"surum": sonuc.Surum, "db_adi": dbName, "ekstra": sonuc.Ekstra,
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `/usr/local/go/bin/go test ./internal/apps/... -v`
Expected: PASS (tüm önceki + `TestAlanlariDogrula`). Ayrıca: `/usr/local/go/bin/go build ./...` hatasız derlenmeli (henüz hiçbir yerden çağrılmasa da `Handlers.Kur` derlenebilir olmalı).

- [ ] **Step 5: Commit**

```bash
git add internal/apps/handlers.go internal/apps/handlers_test.go
git commit -m "feat(apps): Handlers + Kur (ortak kurulum akışı)"
```

---

## Task 4: `internal/apps` — `Sil` + `Guncelle`

**Files:**
- Modify: `internal/apps/handlers.go`
- Test: `internal/apps/handlers_test.go`

**Interfaces:**
- Consumes: `cozDizin` (Task 2), `apps.Uygulama.DBAdiOku`/`GuncelleDesteklenir`/`Guncelle`/`Bilgi` (Task 1).
- Produces: `(h *Handlers) Sil(w, r)`, `(h *Handlers) Guncelle(w, r)`, `(h *Handlers) dbSahipMi(ctx, dbName string, domainID int64) (bool, error)`.

- [ ] **Step 1: Write the failing test**

`dbSahipMi` gerçek DB gerektirdiği için burada birim testi yok (WP'deki `dbSahipMi` de test edilmiyor) — bu görevde test edilebilir tek saf parça yok, bu yüzden **doğrudan minimal implementasyona geçiyoruz** (TDD "test önce" kuralı burada anlamlı bir saf-fonksiyon üretmiyor; bu, mevcut `internal/wordpress` modülündeki `Sil`/`Guncelle` handler'larının da test edilmeme gerekçesiyle aynı — gerçek dosya sistemi + gerçek MySQL bağlantısı gerektiriyorlar). Bunun yerine bu adımda **derleme + `go vet` temizliğini** doğrulama adımı olarak kullanıyoruz.

- [ ] **Step 2: (atlandı — bu görevde saf/izole test edilebilir birim yok, gerekçe yukarıda)**

- [ ] **Step 3: Write implementation**

`internal/apps/handlers.go`'ya ekle:
```go
// DELETE /domains/{id}/apps/{tur}  {dizin, db_sil}
func (h *Handlers) Sil(w http.ResponseWriter, r *http.Request) {
	tur := chi.URLParam(r, "tur")
	u, bulunduTur := Bul(tur)
	if !bulunduTur {
		httpx.WriteError(w, http.StatusNotFound, "bilinmeyen uygulama türü")
		return
	}
	domID, sk, _, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	if !adlar.SKGecerli(sk) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz kullanıcı")
		return
	}
	var sreq struct {
		Dizin string `json:"dizin"`
		DBSil bool   `json:"db_sil"`
	}
	_ = json.NewDecoder(r.Body).Decode(&sreq)
	dir, err := cozDizin(sk, sreq.Dizin, u.MarkerDosya())
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	root := "/home/" + sk + "/public_html"
	if dir == root {
		httpx.WriteError(w, http.StatusBadRequest, "kök dizindeki kurulum panelden silinemez (tüm site gider); Dosya Yöneticisi'nden kaldırın")
		return
	}
	if sreq.DBSil {
		if dbName, bulundu := u.DBAdiOku(dir); bulundu {
			if ok, err := h.dbSahipMi(r.Context(), dbName, domID); err == nil && ok {
				// h.DB panel bağlantısı yalnız GRANT ALL ON panel.* yetkisine sahip —
				// gerçek DROP DATABASE yetkisi yalnız hesaplar paketinin root
				// bağlantısında (rootExecAll). hesaplar.MySQLDropDBKeepUser bunu doğru
				// yapar + db_accounts satırını temizler (kullanıcıya dokunmaz — "mevcut
				// kullanıcı" modunda aynı kullanıcı başka DB'de de olabilir; bu ihtimal
				// Kur akışında tek DB'ye tek özel kullanıcı oluşturulduğu için bu türde
				// pratikte oluşmaz, ama fonksiyon davranışı bilinçli olarak muhafazakâr).
				_ = hesaplar.MySQLDropDBKeepUser(h.DB, dbName)
			}
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silinemedi")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// dbSahipMi: dbName GERÇEKTEN bu domain'e ait mi? (db_accounts sahiplik kontrolü)
func (h *Handlers) dbSahipMi(ctx context.Context, dbName string, domainID int64) (bool, error) {
	var n int
	err := h.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM db_accounts WHERE db_name=? AND domain_id=?`, dbName, domainID).Scan(&n)
	return n > 0, err
}

// POST /domains/{id}/apps/{tur}/guncelle  {dizin}
func (h *Handlers) Guncelle(w http.ResponseWriter, r *http.Request) {
	tur := chi.URLParam(r, "tur")
	u, bulunduTur := Bul(tur)
	if !bulunduTur {
		httpx.WriteError(w, http.StatusNotFound, "bilinmeyen uygulama türü")
		return
	}
	if !u.GuncelleDesteklenir() {
		httpx.WriteError(w, http.StatusBadRequest, "bu uygulama için güncelleme desteklenmiyor")
		return
	}
	_, sk, _, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	var greq struct {
		Dizin string `json:"dizin"`
	}
	_ = json.NewDecoder(r.Body).Decode(&greq)
	dir, err := cozDizin(sk, greq.Dizin, u.MarkerDosya())
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := u.Guncelle(ctx, sk, dir); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "güncelleme: "+err.Error())
		return
	}
	bilgi, _ := u.Bilgi(ctx, sk, dir, "")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "surum": bilgi.Surum})
}
```

- [ ] **Step 4: Run to verify it compiles + existing tests still pass**

Run: `/usr/local/go/bin/go build ./... && /usr/local/go/bin/go vet ./internal/apps/... && /usr/local/go/bin/go test ./internal/apps/... -v`
Expected: derleme temiz, `go vet` sessiz, tüm önceki testler PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/apps/handlers.go
git commit -m "feat(apps): Sil + Guncelle (ortak silme/güncelleme akışı)"
```

---

## Task 5: `internal/apps` — `Liste` + `Turler` + `TumListe`

**Files:**
- Modify: `internal/apps/handlers.go`
- Test: `internal/apps/handlers_test.go`

**Interfaces:**
- Produces: `(h *Handlers) Liste(w, r)`, `(h *Handlers) Turler(w, r)`, `(h *Handlers) TumListe(w, r)`, `TumKurulum{DomainID, AlanAdi, Tur, TurAdi, Dizin, Surum, SonSurum, Durum, KurulumTarihi, SiteURL, AdminURL}` (JSON tag'leri ile), `(h *Handlers) incele(ctx, a aday) TumKurulum`.

- [ ] **Step 1: Write the failing test**

Bu üç handler da dosya sistemi taraması + registry + (TumListe için) DB sorgusu gerektiriyor — WP'nin `Liste`/`TumListe` handler'ları da birim test edilmemiş (yalnız manuel/entegrasyon). Test edilebilir tek saf parça `Turler`'ın döndürdüğü JSON şeklinin registry içeriğini doğru yansıtmasıdır — bunu bir HTTP-seviyesinde-olmayan doğrudan çağrı testiyle kapsıyoruz (gerçek DB gerektirmiyor çünkü `Turler` `h.DB`'ye hiç dokunmuyor):

`internal/apps/handlers_test.go`'ya ekle:
```go
func TestTurlerFormSemasiIcerir(t *testing.T) {
	Kaydet(sahteUygulama{slug: "test-tur-form"})
	u, ok := Bul("test-tur-form")
	if !ok {
		t.Fatal("kayıt bulunamadı")
	}
	if u.Ad() != "Sahte test-tur-form" {
		t.Fatalf("Ad() beklenmedik: %q", u.Ad())
	}
	// Turler handler'ının ürettiği türBilgi şeklini burada, HTTP'siz, doğrudan
	// registry üzerinden doğruluyoruz (Handlers.Turler'ın DB'ye dokunmadığını
	// ve yalnız Hepsi()'yi map'lediğini garanti eden regresyon testi).
	var bulunduSlug bool
	for _, uu := range Hepsi() {
		if uu.Slug() == "test-tur-form" {
			bulunduSlug = true
		}
	}
	if !bulunduSlug {
		t.Fatal("Hepsi() kayıtlı türü içermeli")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/usr/local/go/bin/go test ./internal/apps/... -run TestTurlerFormSemasiIcerir -v`
Expected: FAIL only if compile error exists (bu test aslında Task 1'in registry'sini kullanıyor, bu yüzden asıl amacı `Liste`/`Turler`/`TumListe` eklendikten sonra da derlemenin bozulmadığını garanti etmek — adım 3'te asıl handler kodu eklenecek).

- [ ] **Step 3: Write implementation**

`internal/apps/handlers.go`'ya ekle:
```go
// GET /domains/{id}/apps
func (h *Handlers) Liste(w http.ResponseWriter, r *http.Request) {
	_, sk, alanAdi, ssl, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	root := "/home/" + sk + "/public_html"
	dizinler := []string{root}
	if entries, err := os.ReadDir(root); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				dizinler = append(dizinler, filepath.Join(root, e.Name()))
			}
		}
	}
	type satir struct {
		Tur string `json:"tur"`
		Ad  string `json:"ad"`
		Kurulum
	}
	out := []satir{}
	for _, u := range Hepsi() {
		for _, dir := range dizinler {
			if _, err := os.Stat(filepath.Join(dir, u.MarkerDosya())); err != nil {
				continue
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(dir, root), "/")
			url := scheme(ssl) + alanAdi
			if rel != "" {
				url += "/" + rel
			}
			k, err := u.Bilgi(ctx, sk, dir, url)
			if err != nil {
				continue
			}
			k.Dizin = "/" + rel
			if k.Dizin == "/" {
				k.Dizin = "/ (kök)"
			}
			out = append(out, satir{Tur: u.Slug(), Ad: u.Ad(), Kurulum: k})
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// GET /domains/{id}/apps/turler
func (h *Handlers) Turler(w http.ResponseWriter, r *http.Request) {
	type turBilgi struct {
		Slug    string     `json:"slug"`
		Ad      string     `json:"ad"`
		Alanlar []FormAlan `json:"form_alanlari"`
	}
	out := []turBilgi{}
	for _, u := range Hepsi() {
		out = append(out, turBilgi{Slug: u.Slug(), Ad: u.Ad(), Alanlar: u.FormAlanlari()})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// TumKurulum: tüm domainlerdeki tek bir kurulumun özeti (aggregate tablo satırı).
type TumKurulum struct {
	DomainID      int64  `json:"domain_id"`
	AlanAdi       string `json:"alan_adi"`
	Tur           string `json:"tur"`
	TurAdi        string `json:"tur_adi"`
	Dizin         string `json:"dizin"`
	Surum         string `json:"surum"`
	SonSurum      string `json:"son_surum"`
	Durum         string `json:"durum"`
	KurulumTarihi string `json:"kurulum_tarihi"`
	SiteURL       string `json:"site_url"`
	AdminURL      string `json:"admin_url"`
}

type aday struct {
	domainID    int64
	sk, alanAdi string
	ssl         bool
	u           Uygulama
	dir, root   string
}

// GET /apps/tumu — TÜM domainlerdeki kurulu uygulamaları tarar. BayiVeUstu.
func (h *Handlers) TumListe(w http.ResponseWriter, r *http.Request) {
	kosul, arg := middleware.KapsamSQL(r, "d")
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT d.id, d.sistem_kullanici, d.alan_adi, COALESCE(d.cert_path,'') FROM domains d`+kosul+` ORDER BY d.alan_adi`, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "domainler listelenemedi")
		return
	}
	var adaylar []aday
	for rows.Next() {
		var id int64
		var sk, alanAdi, cert string
		if err := rows.Scan(&id, &sk, &alanAdi, &cert); err != nil {
			continue
		}
		if !adlar.SKGecerli(sk) {
			continue
		}
		root := "/home/" + sk + "/public_html"
		dizinler := []string{root}
		if entries, err := os.ReadDir(root); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					dizinler = append(dizinler, filepath.Join(root, e.Name()))
				}
			}
		}
		for _, u := range Hepsi() {
			for _, dir := range dizinler {
				if _, err := os.Stat(filepath.Join(dir, u.MarkerDosya())); err != nil {
					continue
				}
				adaylar = append(adaylar, aday{id, sk, alanAdi, cert != "", u, dir, root})
			}
		}
	}
	_ = rows.Err()
	rows.Close()

	out := make([]TumKurulum, len(adaylar))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range adaylar {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, a aday) {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = h.incele(r.Context(), a)
		}(i, adaylar[i])
	}
	wg.Wait()
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) incele(ctx context.Context, a aday) TumKurulum {
	rel := strings.TrimPrefix(strings.TrimPrefix(a.dir, a.root), "/")
	dizinEt := "/" + rel
	if dizinEt == "/" {
		dizinEt = "/ (kök)"
	}
	tk := TumKurulum{DomainID: a.domainID, AlanAdi: a.alanAdi, Tur: a.u.Slug(), TurAdi: a.u.Ad(), Dizin: dizinEt, Durum: "bilinmiyor"}
	url := scheme(a.ssl) + a.alanAdi
	if rel != "" {
		url += "/" + rel
	}
	k, err := a.u.Bilgi(ctx, a.sk, a.dir, url)
	if err != nil {
		return tk
	}
	tk.Surum, tk.SonSurum, tk.Durum, tk.KurulumTarihi, tk.SiteURL, tk.AdminURL =
		k.Surum, k.SonSurum, k.Durum, k.KurulumTarihi, k.SiteURL, k.AdminURL
	if tk.Durum == "" {
		tk.Durum = "bilinmiyor"
	}
	return tk
}
```

`internal/apps/handlers.go`'nun import bloğuna `"sanalcp/internal/middleware"` ekle.

- [ ] **Step 4: Run test to verify it passes**

Run: `/usr/local/go/bin/go build ./... && /usr/local/go/bin/go vet ./internal/apps/... && /usr/local/go/bin/go test ./internal/apps/... -v`
Expected: derleme temiz, tüm testler PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/apps/handlers.go internal/apps/handlers_test.go
git commit -m "feat(apps): Liste + Turler + TumListe (domain ve global tarama)"
```

---

## Task 6: `internal/wordpress` — Adapter

**Files:**
- Create: `internal/wordpress/adapter.go`
- Test: `internal/wordpress/adapter_test.go`

**Interfaces:**
- Consumes (aynı paketten, unexported): `wpKomut`, `wpKomutStdin`, `wpStdout`, `wpConfigDBParolaDogrula`, `wpParolaDogrulaPHP`, `reAdmin`, `dbAdiWPGuard`.
- Produces: `wordpress.Adapter{}` implement eden `apps.Uygulama`, `init()` içinde `apps.Kaydet(Adapter{})`.

- [ ] **Step 1: Write the failing test**

`internal/wordpress/adapter_test.go`:
```go
package wordpress

import "testing"

func TestAdapterTemelBilgiler(t *testing.T) {
	a := Adapter{}
	if a.Slug() != "wordpress" {
		t.Fatalf("Slug() = %q, beklenen wordpress", a.Slug())
	}
	if a.DBOnEki() != "wp" {
		t.Fatalf("DBOnEki() = %q, beklenen wp (mevcut wp_/wpu_ DB adlandırma deseniyle uyumlu olmalı)", a.DBOnEki())
	}
	if a.MarkerDosya() != "wp-config.php" {
		t.Fatalf("MarkerDosya() = %q, beklenen wp-config.php", a.MarkerDosya())
	}
	if a.GuncelleDesteklenir() != true {
		t.Fatal("WordPress güncelleme desteklemeli (wp core update mevcut)")
	}
	alanlar := a.FormAlanlari()
	beklenenAnahtarlar := map[string]bool{"site_basligi": false, "admin_kullanici": false, "admin_email": false}
	for _, fa := range alanlar {
		if _, var_ := beklenenAnahtarlar[fa.Anahtar]; var_ {
			beklenenAnahtarlar[fa.Anahtar] = true
		}
	}
	for k, bulundu := range beklenenAnahtarlar {
		if !bulundu {
			t.Errorf("form alanlarında %q eksik", k)
		}
	}
}

func TestAdapterDBAdiOkuGecersizDosya(t *testing.T) {
	a := Adapter{}
	if _, bulundu := a.DBAdiOku(t.TempDir()); bulundu {
		t.Fatal("wp-config.php olmayan dizinde bulundu=false olmalı")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/usr/local/go/bin/go test ./internal/wordpress/... -run TestAdapter -v`
Expected: FAIL (derleme hatası — `Adapter` henüz yok).

- [ ] **Step 3: Write minimal implementation**

`internal/wordpress/adapter.go`:
```go
// adapter.go — apps.Uygulama arayüzünün WordPress implementasyonu. wordpress.go/
// toolkit.go/bakim.go'daki HİÇBİR SATIRA dokunulmaz; bu dosya onlardaki unexported
// yardımcıları (aynı pakette olduğu için) doğrudan çağırarak apps çerçevesine
// paralel bir yol açar. /wordpress/* route'ları bu dosyadan tamamen bağımsız çalışmaya
// devam eder.
package wordpress

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sanalcp/internal/apps"
)

func init() {
	apps.Kaydet(Adapter{})
}

type Adapter struct{}

func (Adapter) Slug() string       { return "wordpress" }
func (Adapter) Ad() string         { return "WordPress" }
func (Adapter) DBOnEki() string    { return "wp" }
func (Adapter) MarkerDosya() string { return "wp-config.php" }
func (Adapter) GuncelleDesteklenir() bool { return true }

func (Adapter) FormAlanlari() []apps.FormAlan {
	return []apps.FormAlan{
		{Anahtar: "site_basligi", Etiket: "Site Başlığı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_kullanici", Etiket: "Admin Kullanıcı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_email", Etiket: "Admin E-posta", Tur: "email", Zorunlu: true},
	}
}

func (Adapter) DBAdiOku(dizin string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dizin, "wp-config.php"))
	if err != nil {
		return "", false
	}
	m := reDBName.FindSubmatch(b)
	if m == nil {
		return "", false
	}
	dbName := string(m[1])
	if !dbAdiWPGuard(dbName) {
		return "", false
	}
	return dbName, true
}

// Kur: wordpress.go'daki Kur handler'ının WordPress'e-özel gövdesinin adaptasyonu
// (DB oluşturma ortak apps katmanında zaten yapıldığı için burada tekrar edilmez).
func (Adapter) Kur(ctx context.Context, i apps.KurulumIstek) (apps.KurulumSonuc, error) {
	if out, err := wpKomut(ctx, i.SK, "core", "download", "--path="+i.Hedef, "--locale=tr_TR"); err != nil {
		return apps.KurulumSonuc{}, wpHataOlustur("WordPress indirme", out)
	}
	if out, err := wpKomutStdin(ctx, i.SK, i.DBParola+"\n", "config", "create", "--dbname="+i.DBAdi,
		"--dbuser="+i.DBKullanici, "--prompt=dbpass", "--dbhost=localhost", "--locale=tr_TR",
		"--path="+i.Hedef, "--skip-check", "--quiet"); err != nil {
		return apps.KurulumSonuc{}, wpHataOlustur("wp-config oluşturma", out)
	}
	if err := wpConfigDBParolaDogrula(i.Hedef, i.DBParola); err != nil {
		return apps.KurulumSonuc{}, err
	}
	adminParola := randParola()
	if out, err := wpKomutStdin(ctx, i.SK, adminParola+"\n", "core", "install", "--url="+i.URL,
		"--title="+i.Alanlar["site_basligi"], "--admin_user="+i.Alanlar["admin_kullanici"],
		"--prompt=admin_password", "--admin_email="+i.Alanlar["admin_email"],
		"--skip-email", "--path="+i.Hedef, "--quiet"); err != nil {
		return apps.KurulumSonuc{}, wpHataOlustur("WordPress kurulum", out)
	}
	out, err := wpKomutStdin(ctx, i.SK, i.Alanlar["admin_kullanici"]+"\n"+adminParola+"\n",
		"eval", wpParolaDogrulaPHP, "--path="+i.Hedef, "--quiet")
	if err != nil || !bytes.Contains(out, []byte("PAROLA_OK")) {
		return apps.KurulumSonuc{}, wpHataOlustur("admin parolası doğrulama", out)
	}
	surum := ""
	if b, err := wpKomut(ctx, i.SK, "core", "version", "--path="+i.Hedef); err == nil {
		surum = strings.TrimSpace(string(b))
	}
	return apps.KurulumSonuc{
		SiteURL: i.URL, AdminURL: i.URL + "/wp-admin",
		AdminKullanici: i.Alanlar["admin_kullanici"], AdminParola: adminParola,
		Surum: surum,
	}, nil
}

func wpHataOlustur(asama string, out []byte) error {
	msg := strings.TrimSpace(string(out))
	if len(msg) > 600 {
		msg = msg[len(msg)-600:]
	}
	return &wpAdapterHata{asama: asama, mesaj: msg}
}

type wpAdapterHata struct{ asama, mesaj string }

func (e *wpAdapterHata) Error() string { return e.asama + ": " + e.mesaj }

func (Adapter) Bilgi(ctx context.Context, sk, dizin, url string) (apps.Kurulum, error) {
	k := apps.Kurulum{SiteURL: url, AdminURL: url + "/wp-admin", Durum: "bilinmiyor"}
	c1, cancel1 := context.WithTimeout(ctx, 15*time.Second)
	if b, err := wpStdout(c1, sk, "core", "version", "--path="+dizin); err == nil {
		k.Surum = strings.TrimSpace(string(b))
	}
	cancel1()
	c2, cancel2 := context.WithTimeout(ctx, 25*time.Second)
	if b, err := wpStdout(c2, sk, "core", "check-update", "--path="+dizin, "--format=json"); err == nil {
		bt := bytes.TrimSpace(b)
		if len(bt) == 0 || string(bt) == "[]" {
			k.Durum = "guncel"
		} else {
			var ups []struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(bt, &ups) == nil {
				if len(ups) > 0 {
					k.Durum = "eski"
					k.SonSurum = ups[0].Version
				} else {
					k.Durum = "guncel"
				}
			}
		}
	}
	cancel2()
	if fi, err := os.Stat(filepath.Join(dizin, "wp-config.php")); err == nil {
		k.KurulumTarihi = fi.ModTime().Format("2006-01-02")
	}
	return k, nil
}

func (Adapter) Guncelle(ctx context.Context, sk, dizin string) error {
	out1, e1 := wpKomut(ctx, sk, "core", "update", "--path="+dizin)
	_, _ = wpKomut(ctx, sk, "core", "update-db", "--path="+dizin)
	if e1 != nil {
		return wpHataOlustur("güncelleme", out1)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `/usr/local/go/bin/go test ./internal/wordpress/... -v`
Expected: PASS — `TestAdapterTemelBilgiler`, `TestAdapterDBAdiOkuGecersizDosya` dahil, **ve mevcut tüm eski testler (wpconfig_test.go, wpdrop_test.go, wpinstall_guard_test.go) hâlâ PASS** (hiçbiri değişmedi, bu bir regresyon kontrolüdür).

- [ ] **Step 5: Commit**

```bash
git add internal/wordpress/adapter.go internal/wordpress/adapter_test.go
git commit -m "feat(wordpress): apps.Uygulama Adapter — mevcut wp-cli akışına dokunmadan"
```

---

## Task 7: `cmd/server/main.go` — `apps` route'ları

**Files:**
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: `apps.Handlers{DB *sql.DB}`, `apps.Handlers.{Liste,Turler,Kur,Guncelle,Sil,TumListe}` (Task 3-5), mevcut `middleware.MusteriScope`/`middleware.BayiVeUstu`.

- [ ] **Step 1: (TDD atlanır — bu görev yalnız route wiring, davranışı Task 3-5'in testleri zaten kapsıyor)**

- [ ] **Step 2: Implementation**

`cmd/server/main.go` import bloğuna, satır 20 (`"sanalcp/internal/antivirus"`) ile satır 21 (`"sanalcp/internal/auth"`) arasına ekle:
```go
	"sanalcp/internal/apps"
```

`wpH := &wordpress.Handlers{DB: d}` satırının (mevcut satır 266) hemen altına ekle:
```go
	appsH := &apps.Handlers{DB: d}
```

Mevcut WordPress route bloğunun (satır 461-475) hemen altına, aynı girinti seviyesinde ekle:
```go
	// Uygulama Kurulum Çerçevesi — WordPress dahil tüm türler için ortak uçlar.
	// /wordpress/* uçları YUKARIDA aynen kalır, bu bloğa dokunmaz.
	r.With(middleware.MusteriScope).Get("/domains/{id}/apps", appsH.Liste)
	r.With(middleware.MusteriScope).Get("/domains/{id}/apps/turler", appsH.Turler)
	r.With(middleware.MusteriScope).Post("/domains/{id}/apps/{tur}/kur", appsH.Kur)
	r.With(middleware.MusteriScope).Post("/domains/{id}/apps/{tur}/guncelle", appsH.Guncelle)
	r.With(middleware.MusteriScope).Delete("/domains/{id}/apps/{tur}", appsH.Sil)
	r.With(middleware.BayiVeUstu).Get("/apps/tumu", appsH.TumListe)
```

- [ ] **Step 3: Run to verify it compiles**

Run: `/usr/local/go/bin/go build ./... && /usr/local/go/bin/go vet ./...`
Expected: hatasız derleme, `go vet` sessiz.

- [ ] **Step 4: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat(apps): /domains/{id}/apps ve /apps/tumu route'ları"
```

---

## Task 8: `internal/prestashop` — indirme + zip açma altyapısı

**Files:**
- Create: `internal/prestashop/indirici.go`
- Test: `internal/prestashop/indirici_test.go`

**Interfaces:**
- Produces: `psSonSurum(ctx context.Context) (string, error)`, `psIndirVeAc(ctx context.Context, surum, hedef string) error`, `psZipAc(zipYolu, hedef string) error`, `ortakUstDizin(files []*zip.File) string`, `psKomut(ctx context.Context, sk string, args ...string) ([]byte, error)`.

🔴 **Doğrulanmamış dış bağımlılık:** PrestaShop'un WP-CLI benzeri resmi bir CLI indirme aracı yok. Aşağıdaki `download.prestashop.com/download/releases/prestashop_<sürüm>.zip` URL kalıbı üçüncü taraf belgelerden (hosting sağlayıcı kılavuzu) doğrulandı, ama bu ortamdan canlı test edilemedi (Cloudflare 522 / bağlantı hatası — sandbox kısıtı mı gerçek kesinti mi belirsiz). **Task 13'teki manuel doğrulama adımında gerçek bir SanalCP sunucusundan bu URL'nin çalıştığı doğrulanmalı; çalışmazsa yalnız bu dosyadaki `psIndirVeAc`'in URL'si değişir, başka hiçbir yer etkilenmez.**

- [ ] **Step 1: Write the failing test**

`internal/prestashop/indirici_test.go`:
```go
package prestashop

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func zipOlustur(t *testing.T, dosyalar map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for ad, icerik := range dosyalar {
		f, err := zw.Create(ad)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(icerik)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	yol := filepath.Join(t.TempDir(), "test.zip")
	if err := os.WriteFile(yol, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return yol
}

func TestPsZipAcKokte(t *testing.T) {
	zipYolu := zipOlustur(t, map[string]string{
		"index.php":         "<?php",
		"install/index.php": "<?php",
	})
	hedef := t.TempDir()
	if err := psZipAc(zipYolu, hedef); err != nil {
		t.Fatalf("psZipAc: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hedef, "index.php")); err != nil {
		t.Fatalf("kökteki index.php açılmalıydı: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hedef, "install", "index.php")); err != nil {
		t.Fatalf("install/index.php açılmalıydı: %v", err)
	}
}

func TestPsZipAcTekUstDizinliSoyulur(t *testing.T) {
	zipYolu := zipOlustur(t, map[string]string{
		"prestashop-9.1.5/index.php":         "<?php",
		"prestashop-9.1.5/install/index.php": "<?php",
	})
	hedef := t.TempDir()
	if err := psZipAc(zipYolu, hedef); err != nil {
		t.Fatalf("psZipAc: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hedef, "prestashop-9.1.5")); err == nil {
		t.Fatal("üst dizin soyulmalıydı, ama hâlâ mevcut")
	}
	if _, err := os.Stat(filepath.Join(hedef, "index.php")); err != nil {
		t.Fatalf("üst dizin soyulduktan sonra index.php kökte olmalıydı: %v", err)
	}
}

func TestPsZipAcZipSlipEngellenir(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("../../etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("kotu"))
	_ = zw.Close()
	yol := filepath.Join(t.TempDir(), "kotu.zip")
	if err := os.WriteFile(yol, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	hedef := t.TempDir()
	if err := psZipAc(yol, hedef); err == nil {
		t.Fatal("zip-slip girişimi reddedilmeliydi")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/usr/local/go/bin/go test ./internal/prestashop/... -v`
Expected: FAIL (paket/fonksiyonlar henüz yok).

- [ ] **Step 3: Write minimal implementation**

`internal/prestashop/indirici.go`:
```go
// Package prestashop: apps.Uygulama arayüzünün PrestaShop implementasyonu.
package prestashop

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// psSonSurum: PrestaShop/PrestaShop reposunun GitHub API'sindeki en güncel
// release tag'ini çeker (ör. "9.1.5"). WP-CLI'nin "core download"unun her
// zaman en güncel sürümü indirmesiyle aynı felsefe.
func psSonSurum(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/PrestaShop/PrestaShop/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("PrestaShop sürüm bilgisi alınamadı: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PrestaShop sürüm bilgisi alınamadı: GitHub API HTTP %d", resp.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.TagName == "" {
		return "", fmt.Errorf("PrestaShop sürüm bilgisi ayrıştırılamadı")
	}
	return body.TagName, nil
}

// psIndirVeAc: PrestaShop kurulum paketini indirir ve hedef dizine açar.
//
// 🔴 URL KALIBI DOĞRULANMADI (bkz. Task 8 notu, plan dosyası) — ilk canlı
// kurulumda başarısız olursa TEK değişecek yer burasıdır.
func psIndirVeAc(ctx context.Context, surum, hedef string) error {
	url := fmt.Sprintf("https://download.prestashop.com/download/releases/prestashop_%s.zip", surum)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("PrestaShop kaynağı indirilemedi (%s): %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PrestaShop kaynağı indirilemedi: %s → HTTP %d", url, resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "prestashop-*.zip")
	if err != nil {
		return fmt.Errorf("geçici dosya oluşturulamadı: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return fmt.Errorf("indirilen dosya yazılamadı: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return psZipAc(tmp.Name(), hedef)
}

// psZipAc: zip'i hedef dizine açar. Zip içeriği ya doğrudan kökte (index.php,
// install/, ...) ya da tek bir ortak üst dizin altında paketlenmiş olabilir —
// ikinci durumda o üst dizin soyulur. zip-slip'e karşı korumalıdır.
func psZipAc(zipYolu, hedef string) error {
	zr, err := zip.OpenReader(zipYolu)
	if err != nil {
		return fmt.Errorf("zip açılamadı: %w", err)
	}
	defer zr.Close()

	onEk := ortakUstDizin(zr.File)
	hedefTemiz := filepath.Clean(hedef)

	for _, f := range zr.File {
		rel := strings.TrimPrefix(f.Name, onEk)
		if rel == "" {
			continue
		}
		hedefYol := filepath.Join(hedef, rel)
		if hedefYol != hedefTemiz && !strings.HasPrefix(hedefYol, hedefTemiz+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz zip girdisi: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(hedefYol, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(hedefYol), 0o755); err != nil {
			return err
		}
		if err := psDosyaCikar(f, hedefYol); err != nil {
			return err
		}
	}
	return nil
}

func psDosyaCikar(f *zip.File, hedefYol string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(hedefYol, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

// ortakUstDizin: tüm zip girdileri tek bir ortak üst dizin altındaysa o dizin
// adını ("ad/" formatında) döner, yoksa boş string (kökte açılır) döner.
func ortakUstDizin(files []*zip.File) string {
	if len(files) == 0 {
		return ""
	}
	ilkParca := strings.SplitN(files[0].Name, "/", 2)
	if len(ilkParca) != 2 {
		return ""
	}
	aday := ilkParca[0] + "/"
	for _, f := range files {
		if !strings.HasPrefix(f.Name, aday) {
			return ""
		}
	}
	return aday
}

// psKomut: PHP'yi domain kullanıcısı olarak, WordPress modülündeki wpKomut ile
// birebir aynı runuser+env+TMPDIR deseniyle çalıştırır (bkz. internal/wordpress/
// wordpress.go — TMPDIR=/home/sk, root'a ait /var/lib/sanalcp/tmp izin hatasından
// kaçınmak için).
func psKomut(ctx context.Context, sk string, args ...string) ([]byte, error) {
	full := append([]string{"-u", sk, "--", "env", "HOME=/home/" + sk, "TMPDIR=/home/" + sk,
		"/usr/bin/php"}, args...)
	cmd := exec.CommandContext(ctx, "runuser", full...)
	return cmd.CombinedOutput()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `/usr/local/go/bin/go test ./internal/prestashop/... -v`
Expected: PASS — `TestPsZipAcKokte`, `TestPsZipAcTekUstDizinliSoyulur`, `TestPsZipAcZipSlipEngellenir`.

- [ ] **Step 5: Commit**

```bash
git add internal/prestashop/indirici.go internal/prestashop/indirici_test.go
git commit -m "feat(prestashop): indirme + zip açma altyapısı (zip-slip korumalı)"
```

---

## Task 9: `internal/prestashop` — `Surucu` (apps.Uygulama implementasyonu)

**Files:**
- Create: `internal/prestashop/prestashop.go`
- Test: `internal/prestashop/prestashop_test.go`

**Interfaces:**
- Consumes: `psSonSurum`, `psIndirVeAc`, `psKomut` (Task 8), `hesaplar.GecerliDBKimlik(s string) bool`, `hesaplar.RandomParola(n int) string`.
- Produces: `prestashop.Surucu{}` implement eden `apps.Uygulama`, `init()` içinde `apps.Kaydet(Surucu{})`.

- [ ] **Step 1: Write the failing test**

`internal/prestashop/prestashop_test.go`:
```go
package prestashop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSurucuTemelBilgiler(t *testing.T) {
	s := Surucu{}
	if s.Slug() != "prestashop" {
		t.Fatalf("Slug() = %q, beklenen prestashop", s.Slug())
	}
	if s.DBOnEki() != "prestashop" {
		t.Fatalf("DBOnEki() = %q, beklenen prestashop", s.DBOnEki())
	}
	if s.MarkerDosya() != filepath.Join("config", "settings.inc.php") {
		t.Fatalf("MarkerDosya() = %q", s.MarkerDosya())
	}
	if s.GuncelleDesteklenir() != false {
		t.Fatal("PrestaShop güncelleme DESTEKLENMEMELİ (resmi CLI güncelleyici yok — spec kararı)")
	}
	var eposta bool
	for _, fa := range s.FormAlanlari() {
		if fa.Anahtar == "admin_email" && fa.Tur == "email" && fa.Zorunlu {
			eposta = true
		}
	}
	if !eposta {
		t.Error("form alanlarında zorunlu email tipli admin_email eksik")
	}
}

func TestSurucuDBAdiOkuGecersizDosya(t *testing.T) {
	s := Surucu{}
	if _, bulundu := s.DBAdiOku(t.TempDir()); bulundu {
		t.Fatal("config/settings.inc.php olmayan dizinde bulundu=false olmalı")
	}
}

func TestSurucuDBAdiOkuGecerliDosya(t *testing.T) {
	s := Surucu{}
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	icerik := "<?php\ndefine('_DB_NAME_', 'prestashop_a1b2c3d4');\ndefine('_DB_PREFIX_', 'ps_');\n"
	if err := os.WriteFile(filepath.Join(d, "config", "settings.inc.php"), []byte(icerik), 0o644); err != nil {
		t.Fatal(err)
	}
	dbAdi, bulundu := s.DBAdiOku(d)
	if !bulundu {
		t.Fatal("geçerli dosyada bulundu=true olmalı")
	}
	if dbAdi != "prestashop_a1b2c3d4" {
		t.Fatalf("dbAdi = %q, beklenen prestashop_a1b2c3d4", dbAdi)
	}
}

func TestSurucuDBAdiOkuYanlisOnekReddedilir(t *testing.T) {
	s := Surucu{}
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	// başka bir kiracının/uygulamanın DB'sine benzer isim — DBOnEki ("prestashop_")
	// önekini taşımıyor, guard reddetmeli.
	icerik := "<?php\ndefine('_DB_NAME_', 'wp_baskatenant');\n"
	if err := os.WriteFile(filepath.Join(d, "config", "settings.inc.php"), []byte(icerik), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, bulundu := s.DBAdiOku(d); bulundu {
		t.Fatal("prestashop_ önekini taşımayan DB adı reddedilmeli")
	}
}

func TestPsAdminDizinBul(t *testing.T) {
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "adminxyz123"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := psAdminDizinBul(d); got != "adminxyz123" {
		t.Fatalf("psAdminDizinBul = %q, beklenen adminxyz123", got)
	}
}

func TestPsAdminDizinBulBulunamazsaVarsayilan(t *testing.T) {
	d := t.TempDir()
	if got := psAdminDizinBul(d); got != "admin" {
		t.Fatalf("psAdminDizinBul boş dizinde 'admin' dönmeli, döndü: %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/usr/local/go/bin/go test ./internal/prestashop/... -run "TestSurucu|TestPsAdminDizinBul" -v`
Expected: FAIL (derleme hatası — `Surucu` henüz yok).

- [ ] **Step 3: Write minimal implementation**

`internal/prestashop/prestashop.go`:
```go
package prestashop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"sanalcp/internal/apps"
	"sanalcp/internal/hesaplar"
)

func init() {
	apps.Kaydet(Surucu{})
}

type Surucu struct{}

func (Surucu) Slug() string             { return "prestashop" }
func (Surucu) Ad() string               { return "PrestaShop" }
func (Surucu) DBOnEki() string          { return "prestashop" }
func (Surucu) MarkerDosya() string      { return filepath.Join("config", "settings.inc.php") }
func (Surucu) GuncelleDesteklenir() bool { return false } // spec kararı: resmi CLI güncelleyici yok

func (Surucu) FormAlanlari() []apps.FormAlan {
	return []apps.FormAlan{
		{Anahtar: "magaza_adi", Etiket: "Mağaza Adı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_email", Etiket: "Admin E-posta", Tur: "email", Zorunlu: true},
		{Anahtar: "admin_ad", Etiket: "Admin Ad", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_soyad", Etiket: "Admin Soyad", Tur: "text", Zorunlu: true},
	}
}

var reDBNamePS = regexp.MustCompile(`define\(\s*'_DB_NAME_',\s*'([^']+)'\s*\)`)

func (Surucu) DBAdiOku(dizin string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dizin, "config", "settings.inc.php"))
	if err != nil {
		return "", false
	}
	m := reDBNamePS.FindSubmatch(b)
	if m == nil {
		return "", false
	}
	dbName := string(m[1])
	if !hesaplar.GecerliDBKimlik(dbName) || !strings.HasPrefix(dbName, "prestashop_") {
		return "", false
	}
	return dbName, true
}

// psAdminDizinBul: PrestaShop kurulum sonrası admin/ dizinini güvenlik için
// rastgele isimlendirir (adminXXXXXXXXXX/). Stdout ayrıştırmak yerine hedef
// dizini tarayarak buluyoruz — sürüme göre değişebilecek stdout metnine
// bağımlı olmadan güvenilir.
func psAdminDizinBul(hedef string) string {
	entries, err := os.ReadDir(hedef)
	if err != nil {
		return "admin"
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != "admin" && strings.HasPrefix(e.Name(), "admin") {
			return e.Name()
		}
	}
	return "admin"
}

var rePSVersion = regexp.MustCompile(`define\(\s*'_PS_VERSION_',\s*'([^']+)'\s*\)`)

// psSurumDosyadanOku: kurulu sürümü config dosyalarından okumaya çalışır
// (dosya adı/konumu sürüme göre değişebilir — best-effort, bulunamazsa "").
func psSurumDosyadanOku(dizin string) string {
	for _, yol := range []string{
		filepath.Join(dizin, "config", "defines.inc.php"),
		filepath.Join(dizin, "config", "settings.inc.php"),
	} {
		b, err := os.ReadFile(yol)
		if err != nil {
			continue
		}
		if m := rePSVersion.FindSubmatch(b); m != nil {
			return string(m[1])
		}
	}
	return ""
}

func (Surucu) Kur(ctx context.Context, i apps.KurulumIstek) (apps.KurulumSonuc, error) {
	surum, err := psSonSurum(ctx)
	if err != nil {
		return apps.KurulumSonuc{}, err
	}
	if err := psIndirVeAc(ctx, surum, i.Hedef); err != nil {
		return apps.KurulumSonuc{}, err
	}

	adminParola := hesaplar.RandomParola(18)
	args := []string{
		filepath.Join(i.Hedef, "install", "index_cli.php"),
		"--domain=" + i.AlanAdi,
		"--db_server=localhost",
		"--db_name=" + i.DBAdi,
		"--db_user=" + i.DBKullanici,
		"--db_password=" + i.DBParola,
		"--prefix=ps_",
		"--name=" + i.Alanlar["magaza_adi"],
		"--email=" + i.Alanlar["admin_email"],
		"--password=" + adminParola,
		"--firstname=" + i.Alanlar["admin_ad"],
		"--lastname=" + i.Alanlar["admin_soyad"],
		"--language=tr",
		"--country=tr",
		"--all_languages=0",
		"--fixtures=0",
		"--license=0",
	}
	out, err := psKomut(ctx, i.SK, args...)
	if err != nil || !strings.Contains(string(out), "Installation successful") {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 600 {
			msg = msg[len(msg)-600:]
		}
		return apps.KurulumSonuc{}, fmt.Errorf("PrestaShop kurulum: %s", msg)
	}

	adminDizin := psAdminDizinBul(i.Hedef)
	return apps.KurulumSonuc{
		SiteURL:        i.URL,
		AdminURL:       i.URL + "/" + adminDizin,
		AdminKullanici: i.Alanlar["admin_email"],
		AdminParola:    adminParola,
		Surum:          surum,
		Ekstra:         map[string]string{"admin_dizini": adminDizin},
	}, nil
}

func (Surucu) Bilgi(ctx context.Context, sk, dizin, url string) (apps.Kurulum, error) {
	adminDizin := psAdminDizinBul(dizin)
	kurulumTarihi := ""
	if fi, err := os.Stat(filepath.Join(dizin, "config", "settings.inc.php")); err == nil {
		kurulumTarihi = fi.ModTime().Format("2006-01-02")
	}
	return apps.Kurulum{
		Surum: psSurumDosyadanOku(dizin), Durum: "bilinmiyor",
		SiteURL: url, AdminURL: url + "/" + adminDizin,
		KurulumTarihi: kurulumTarihi,
	}, nil
}

func (Surucu) Guncelle(ctx context.Context, sk, dizin string) error {
	return fmt.Errorf("PrestaShop için otomatik güncelleme desteklenmiyor")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `/usr/local/go/bin/go test ./internal/prestashop/... -v`
Expected: PASS — tüm testler (Task 8 + Task 9).

- [ ] **Step 5: Commit**

```bash
git add internal/prestashop/prestashop.go internal/prestashop/prestashop_test.go
git commit -m "feat(prestashop): apps.Uygulama Surucu — kur/bilgi (güncelleme desteklenmiyor)"
```

---

## Task 10: `cmd/server/main.go` — PrestaShop kaydı

**Files:**
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: `internal/prestashop` paketinin `init()` yan etkisi (`apps.Kaydet(Surucu{})`).

- [ ] **Step 1-2: Implementation**

`cmd/server/main.go` import bloğuna, `"sanalcp/internal/pma"` ile `"sanalcp/internal/provisioner"` arasına (alfabetik: plans, pma, prestashop, provisioner) ekle:
```go
	_ "sanalcp/internal/prestashop" // yalnız init()'teki apps.Kaydet için — hiçbir sembolü doğrudan kullanılmıyor
```

- [ ] **Step 3: Run to verify it compiles**

Run: `/usr/local/go/bin/go build ./... && /usr/local/go/bin/go vet ./...`
Expected: hatasız derleme (blank import unused-import hatası vermez).

- [ ] **Step 4: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat(prestashop): registry'ye kayıt (blank import)"
```

---

## Task 11: Frontend — `AppsPage.tsx` (global liste)

**Files:**
- Create: `frontend/src/pages/AppsPage.tsx`
- Create: `frontend/src/i18n/locales/tr/AppsPage.json`
- Create: `frontend/src/i18n/locales/en/AppsPage.json`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/components/DashboardLayout.tsx`
- Delete: `frontend/src/pages/WordPressPage.tsx`
- Delete: `frontend/src/i18n/locales/tr/WordPressPage.json`
- Delete: `frontend/src/i18n/locales/en/WordPressPage.json`

**Interfaces:**
- Consumes: `GET /apps/tumu` (Task 5), `api`/`apiHata` (`@/lib/api`), `Breadcrumb`, `T` (`@/lib/tablo`).

- [ ] **Step 1-2: (frontend'de proje genelinde birim testi yok — TDD adımı atlanır, spec Test Planı'ndaki desenle tutarlı; doğrulama Task 13'te manuel)**

- [ ] **Step 3: Write implementation**

`frontend/src/i18n/locales/tr/AppsPage.json`:
```json
{
  "breadcrumb_title": "Uygulamalar",
  "title": "Uygulamalar",
  "subtitle": "Sunucudaki tüm 1-tık uygulama kurulumlarını görüntüleyin ve yönetin.",
  "update_warning_title": "{{count}} kurulumda güncelleme mevcut.",
  "update_warning_desc": "Eski sürümler bilinen güvenlik açıkları içerebilir — en kısa sürede güncelleyin.",
  "installed_title": "Kurulu Uygulamalar",
  "refresh": "↻ Yenile",
  "scanning": "Kurulumlar taranıyor…",
  "no_installations": "Sunucuda hiç uygulama kurulumu bulunamadı.",
  "col_domain": "Domain",
  "col_app": "Uygulama",
  "col_path": "Dizin",
  "col_version": "Sürüm",
  "col_status": "Durum",
  "col_actions": "İşlemler",
  "status_update_to": "Güncelleme var → v{{version}}",
  "status_current": "Güncel",
  "status_unknown": "Bilinmiyor",
  "admin_link": "Yönetim",
  "manage_link": "Yönet"
}
```

`frontend/src/i18n/locales/en/AppsPage.json`:
```json
{
  "breadcrumb_title": "Apps",
  "title": "Apps",
  "subtitle": "View and manage all 1-click app installations on the server.",
  "update_warning_title": "{{count}} installation(s) have an update available.",
  "update_warning_desc": "Older versions may contain known security vulnerabilities — please update as soon as possible.",
  "installed_title": "Installed Apps",
  "refresh": "↻ Refresh",
  "scanning": "Scanning installations…",
  "no_installations": "No app installations found on the server.",
  "col_domain": "Domain",
  "col_app": "App",
  "col_path": "Path",
  "col_version": "Version",
  "col_status": "Status",
  "col_actions": "Actions",
  "status_update_to": "Update available → v{{version}}",
  "status_current": "Up to date",
  "status_unknown": "Unknown",
  "admin_link": "Admin",
  "manage_link": "Manage"
}
```

`frontend/src/pages/AppsPage.tsx`:
```tsx
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import { T } from '@/lib/tablo'

type TumKurulum = {
  domain_id: number; alan_adi: string; tur: string; tur_adi: string; dizin: string
  surum: string; son_surum: string; durum: 'guncel' | 'eski' | 'bilinmiyor'
  kurulum_tarihi: string; site_url: string; admin_url: string
}

export default function AppsPage() {
  const { t } = useTranslation(['AppsPage', 'common'])
  const [tum, setTum] = useState<TumKurulum[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)

  function listele() {
    setYuk(true)
    api.get<TumKurulum[]>('/apps/tumu')
      .then(r => setTum(r.data || []))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(() => { listele() }, [])

  const eskiler = useMemo(() => tum.filter(tk => tk.durum === 'eski'), [tum])

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: t('AppsPage:breadcrumb_title') }]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">🧩</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{t('AppsPage:title')}</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">{t('AppsPage:subtitle')}</p>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}

      {!yuk && eskiler.length > 0 && (
        <div className="mb-4 px-4 py-3 rounded-2xl border border-amber-300 dark:border-amber-800 bg-amber-50 dark:bg-amber-900/20 flex items-start gap-3">
          <span className="text-lg leading-none">⚠️</span>
          <div className="text-sm text-amber-800 dark:text-amber-200">
            <strong>{t('AppsPage:update_warning_title', { count: eskiler.length })}</strong> {t('AppsPage:update_warning_desc')}
          </div>
        </div>
      )}

      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden">
        <div className="flex items-center justify-between px-4 py-3 border-b border-slate-100 dark:border-slate-700/60">
          <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">{t('AppsPage:installed_title')} {!yuk && <span className="text-slate-400 font-normal">· {tum.length}</span>}</h3>
          <button onClick={listele} disabled={yuk} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-50">{t('AppsPage:refresh')}</button>
        </div>
        <div className="lg:overflow-x-auto">
          <table className={T.tablo}>
            <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900/50 border-b border-slate-200 dark:border-slate-700/60`}>
              <tr>
                <th className={T.baslik}>{t('AppsPage:col_domain')}</th>
                <th className={T.baslik}>{t('AppsPage:col_app')}</th>
                <th className={T.baslik}>{t('AppsPage:col_path')}</th>
                <th className={T.baslik}>{t('AppsPage:col_version')}</th>
                <th className={T.baslik}>{t('AppsPage:col_status')}</th>
                <th className={`${T.baslik} text-right`}>{t('AppsPage:col_actions')}</th>
              </tr>
            </thead>
            <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-700/60`}>
              {yuk ? (
                <tr><td colSpan={6} className={T.hucreDurum}>{t('AppsPage:scanning')}</td></tr>
              ) : tum.length === 0 ? (
                <tr><td colSpan={6} className={T.hucreDurum}>
                  <div className="text-2xl mb-1">🧩</div>
                  <p className="text-sm text-slate-500 dark:text-slate-400">{t('AppsPage:no_installations')}</p>
                </td></tr>
              ) : (
                tum.map(tk => {
                  const key = tk.domain_id + tk.tur + tk.dizin
                  const eski = tk.durum === 'eski'
                  return (
                    <tr key={key} className={`${T.satir} ${eski ? 'lg:bg-amber-50/50 dark:lg:bg-amber-900/10' : 'lg:hover:bg-slate-50 dark:lg:hover:bg-slate-800/40'}`}>
                      <td className={T.hucreBaslik}>
                        <a href={tk.site_url} target="_blank" rel="noreferrer" className="font-medium text-slate-800 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400">{tk.alan_adi}</a>
                      </td>
                      <td className={T.hucre} data-etiket={t('AppsPage:col_app')}>
                        <span className="text-xs px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 font-medium">{tk.tur_adi}</span>
                      </td>
                      <td className={T.hucre} data-etiket={t('AppsPage:col_path')}>
                        <span className="font-mono text-xs text-slate-500 dark:text-slate-400 whitespace-nowrap">{tk.dizin}</span>
                      </td>
                      <td className={T.hucre} data-etiket={t('AppsPage:col_version')}>
                        <span className="text-xs px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 font-mono font-semibold">{tk.surum ? `v${tk.surum}` : '—'}</span>
                      </td>
                      <td className={T.hucre} data-etiket={t('AppsPage:col_status')}>
                        {tk.durum === 'eski' ? (
                          <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-amber-100 dark:bg-amber-900/40 text-amber-800 dark:text-amber-200 font-medium">{t('AppsPage:status_update_to', { version: tk.son_surum })}</span>
                        ) : tk.durum === 'guncel' ? (
                          <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300 font-medium">{t('AppsPage:status_current')}</span>
                        ) : (
                          <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-slate-100 dark:bg-slate-700 text-slate-500 dark:text-slate-400 font-medium">{t('AppsPage:status_unknown')}</span>
                        )}
                      </td>
                      <td className={T.hucreAksiyon}>
                        <div className="flex items-center flex-wrap gap-1.5 lg:justify-end">
                          <a href={tk.admin_url} target="_blank" rel="noreferrer" className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">{t('AppsPage:admin_link')}</a>
                          <a href={`/abonelikler/${tk.domain_id}/uygulamalar`} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">{t('AppsPage:manage_link')}</a>
                        </div>
                      </td>
                    </tr>
                  )
                })
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
```

`frontend/src/App.tsx` değişiklikleri:
1. Satır 50'deki `const WordPressPage = lazy(() => import('@/pages/WordPressPage'))` satırını **sil**, yerine ekle: `const AppsPage = lazy(() => import('@/pages/AppsPage'))`.
2. Satır 154'teki `<Route path="wordpress" element={<WordPressPage />} />` satırını **şu ikisiyle değiştir**:
```tsx
        <Route path="uygulamalar" element={<AppsPage />} />
        <Route path="wordpress" element={<Navigate to="/uygulamalar" replace />} />
```

`frontend/src/components/DashboardLayout.tsx` değişiklikleri (3 yerde, `nav(t)` ve `bayiNav(t)` fonksiyonlarında):
- `{ to: '/wordpress', etiket: t('DashboardLayout:items.wordpress'), ikon: ICONS.wp }` satırlarını (satır ~82 ve ~128) şuna değiştir: `{ to: '/uygulamalar', etiket: t('DashboardLayout:items.apps'), ikon: ICONS.wp }`.
- `DashboardLayout.json` (tr/en) dosyalarında `items.apps` anahtarı yoksa ekle (`"apps": "Uygulamalar"` / `"apps": "Apps"`), `items.wordpress` anahtarını KALDIRMA (Task 12'de `domainNav`'da hâlâ kullanılacak farklı bir anahtar için referans — aslında Task 12'de o da `uygulamalar`'a döneceği için burada temizlenebilir; iki görev sırayla ilerlediği için bu adımda `items.wordpress` anahtarını dokunmadan bırak, Task 12 temizler).

- [ ] **Step 4: Delete dead files**

```bash
git rm frontend/src/pages/WordPressPage.tsx frontend/src/i18n/locales/tr/WordPressPage.json frontend/src/i18n/locales/en/WordPressPage.json
```

- [ ] **Step 5: Verify build**

Run: `cd frontend && npm run lint && npm run build`
Expected: lint temiz (kullanılmayan `WordPressPage` importu/route kalmamalı), build başarılı.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/pages/AppsPage.tsx frontend/src/i18n/locales/tr/AppsPage.json frontend/src/i18n/locales/en/AppsPage.json frontend/src/App.tsx frontend/src/components/DashboardLayout.tsx frontend/src/i18n/locales/tr/DashboardLayout.json frontend/src/i18n/locales/en/DashboardLayout.json
git add -u frontend/src/pages/WordPressPage.tsx frontend/src/i18n/locales/tr/WordPressPage.json frontend/src/i18n/locales/en/WordPressPage.json
git commit -m "feat(frontend): AppsPage — global 'Uygulamalar' listesi, WordPressPage'in yerini alır"
```

---

## Task 12: Frontend — `DomainAppsPage.tsx` (domain bazlı kur/yönet)

**Files:**
- Create: `frontend/src/pages/DomainAppsPage.tsx`
- Create: `frontend/src/i18n/locales/tr/DomainAppsPage.json`
- Create: `frontend/src/i18n/locales/en/DomainAppsPage.json`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/components/DomainPano.tsx`
- Modify: `frontend/src/components/DashboardLayout.tsx`
- Modify: `frontend/src/i18n/locales/{tr,en}/DomainPano.json`
- Modify: `frontend/src/i18n/locales/{tr,en}/DashboardLayout.json`

**Interfaces:**
- Consumes: `GET /domains/{id}/apps`, `GET /domains/{id}/apps/turler`, `POST /domains/{id}/apps/{tur}/kur`, `POST /domains/{id}/apps/{tur}/guncelle`, `DELETE /domains/{id}/apps/{tur}` (Task 3-5).

- [ ] **Step 1-2: (frontend TDD atlanır — Task 11 ile aynı gerekçe)**

- [ ] **Step 3: Write implementation**

`frontend/src/i18n/locales/tr/DomainAppsPage.json`:
```json
{
  "breadcrumb_subscription": "Abonelik",
  "breadcrumb_apps": "Uygulamalar",
  "title": "Uygulamalar",
  "new_install_button": "+ Yeni Uygulama Kur",
  "install_failed": "Kurulum başarısız",
  "update_failed": "Güncellenemedi",
  "delete_failed": "Silinemedi",
  "confirm_delete": "{{ad}} ({{yol}}) silinsin mi?\nBu dizindeki tüm dosyalar ve veritabanı kaldırılır. Geri alınamaz.",
  "installed_title": "Kurulu Uygulamalar",
  "scanning": "Kurulumlar taranıyor…",
  "no_installations": "Bu domain üzerinde henüz kurulu uygulama yok.",
  "version_label": "Sürüm",
  "status_update_to": "Güncelleme var → v{{version}}",
  "admin_link": "Yönetim",
  "manage_link": "Yönet",
  "update_link": "Güncelle",
  "delete": "Sil",
  "installed_ok": "✅ {{version}} kuruldu",
  "result_site": "Site",
  "result_admin": "Yönetim",
  "result_user": "Kullanıcı",
  "result_password": "Parola",
  "password_warning": "⚠ Parolayı şimdi kaydedin — tekrar gösterilmez.",
  "pick_app_title": "Kurulacak Uygulamayı Seçin",
  "pick_different": "← Farklı uygulama seç",
  "install_title": "{{ad}} Kurulumu",
  "subdir_label": "Alt Dizin (isteğe bağlı)",
  "subdir_placeholder": "boş = kök · örn: magaza",
  "install_button": "Kur",
  "installing_button": "Kuruluyor… (~1-2 dk)"
}
```

`frontend/src/i18n/locales/en/DomainAppsPage.json`:
```json
{
  "breadcrumb_subscription": "Subscription",
  "breadcrumb_apps": "Apps",
  "title": "Apps",
  "new_install_button": "+ New App Install",
  "install_failed": "Installation failed",
  "update_failed": "Could not update",
  "delete_failed": "Could not delete",
  "confirm_delete": "Delete {{ad}} ({{yol}})?\nAll files in this directory and the database are removed. This cannot be undone.",
  "installed_title": "Installed Apps",
  "scanning": "Scanning installations…",
  "no_installations": "No apps installed on this domain yet.",
  "version_label": "Version",
  "status_update_to": "Update available → v{{version}}",
  "admin_link": "Admin",
  "manage_link": "Manage",
  "update_link": "Update",
  "delete": "Delete",
  "installed_ok": "✅ {{version}} installed",
  "result_site": "Site",
  "result_admin": "Admin",
  "result_user": "User",
  "result_password": "Password",
  "password_warning": "⚠ Save the password now — it will not be shown again.",
  "pick_app_title": "Choose an App to Install",
  "pick_different": "← Choose a different app",
  "install_title": "{{ad}} Installation",
  "subdir_label": "Subdirectory (optional)",
  "subdir_placeholder": "empty = root · e.g. shop",
  "install_button": "Install",
  "installing_button": "Installing… (~1-2 min)"
}
```

`frontend/src/pages/DomainAppsPage.tsx`:
```tsx
import { useCallback, useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Kurulu = {
  tur: string; ad: string; dizin: string; surum: string; son_surum: string
  durum: 'guncel' | 'eski' | 'bilinmiyor'; kurulum_tarihi: string
  site_url: string; admin_url: string
}
type FormAlan = { anahtar: string; etiket: string; tur: 'text' | 'email' | 'password'; zorunlu: boolean; yer_tutucu?: string }
type TurBilgi = { slug: string; ad: string; form_alanlari: FormAlan[] }
type Sonuc = {
  tur: string; site_url: string; admin_url: string
  admin_kullanici: string; admin_parola: string; surum: string; ekstra?: Record<string, string>
}

const ICONS: Record<string, string> = {
  wordpress: 'M12 21a9 9 0 100-18 9 9 0 000 18zm0 0c2.5-2.5 3-6 3-9s-.5-6.5-3-9m0 18c-2.5-2.5-3-6-3-9s.5-6.5 3-9M3.6 9h16.8M3.6 15h16.8',
  prestashop: 'M12 3l7 4v10l-7 4-7-4V7l7-4z',
}

export default function DomainAppsPage() {
  const { t } = useTranslation(['DomainAppsPage', 'common'])
  const { id } = useParams()
  const navigate = useNavigate()
  const [alanAdi, setAlanAdi] = useState('')
  const [liste, setListe] = useState<Kurulu[]>([])
  const [yuk, setYuk] = useState(true)
  const [turler, setTurler] = useState<TurBilgi[]>([])
  const [hata, setHata] = useState<string | null>(null)
  const [mesgul, setMesgul] = useState<string | null>(null)

  const [sihirbazAcik, setSihirbazAcik] = useState(false)
  const [seciliTur, setSeciliTur] = useState<TurBilgi | null>(null)
  const [altDizin, setAltDizin] = useState('')
  const [alanlar, setAlanlar] = useState<Record<string, string>>({})
  const [kuruyor, setKuruyor] = useState(false)
  const [sonuc, setSonuc] = useState<Sonuc | null>(null)

  useEffect(() => {
    if (!id) return
    api.get<{ alan_adi: string }>(`/domains/${id}`).then(r => setAlanAdi(r.data.alan_adi || '')).catch(() => {})
    api.get<TurBilgi[]>(`/domains/${id}/apps/turler`).then(r => setTurler(r.data || [])).catch(() => {})
  }, [id])

  const listele = useCallback(() => {
    if (!id) return
    setYuk(true)
    api.get<Kurulu[]>(`/domains/${id}/apps`).then(r => setListe(r.data || [])).catch(() => setListe([])).finally(() => setYuk(false))
  }, [id])
  useEffect(() => { listele() }, [listele])

  function turSec(tb: TurBilgi) {
    setSeciliTur(tb)
    const bos: Record<string, string> = {}
    tb.form_alanlari.forEach(fa => { bos[fa.anahtar] = '' })
    setAlanlar(bos)
    setAltDizin('')
    setHata(null)
  }

  async function kur(e: React.FormEvent) {
    e.preventDefault()
    if (!seciliTur) return
    setHata(null); setSonuc(null); setKuruyor(true)
    try {
      const { data } = await api.post<Sonuc>(`/domains/${id}/apps/${seciliTur.slug}/kur`, {
        alt_dizin: altDizin.trim(), alanlar,
      })
      setSonuc(data); setSihirbazAcik(false); setSeciliTur(null)
      listele()
    } catch (err) { setHata(apiHata(err, t('DomainAppsPage:install_failed'))) }
    finally { setKuruyor(false) }
  }

  async function guncelle(k: Kurulu) {
    const key = k.tur + k.dizin
    setMesgul(key); setHata(null)
    try { await api.post(`/domains/${id}/apps/${k.tur}/guncelle`, { dizin: k.dizin }); listele() }
    catch (err) { setHata(apiHata(err, t('DomainAppsPage:update_failed'))) }
    finally { setMesgul(null) }
  }

  async function sil(k: Kurulu) {
    if (!confirm(t('DomainAppsPage:confirm_delete', { ad: k.ad, yol: k.dizin }))) return
    const key = k.tur + k.dizin
    setMesgul(key); setHata(null)
    try {
      await api.delete(`/domains/${id}/apps/${k.tur}`, { data: { dizin: k.dizin, db_sil: true } })
      listele()
    } catch (err) { setHata(apiHata(err, t('DomainAppsPage:delete_failed'))) }
    finally { setMesgul(null) }
  }

  function yonetHedefi(k: Kurulu): string | null {
    if (k.tur === 'wordpress') return `/abonelikler/${id}/wordpress`
    return null
  }

  return (
    <div className="w-full px-6 py-6">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' },
        { etiket: alanAdi || t('DomainAppsPage:breadcrumb_subscription'), href: `/abonelikler/${id}` },
        { etiket: t('DomainAppsPage:breadcrumb_apps') },
      ]} />
      <div className="flex items-center justify-between gap-4 mb-6 flex-wrap">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{t('DomainAppsPage:title')}</h1>
        <button onClick={() => { setSihirbazAcik(true); setSeciliTur(null) }} className="ta-primary-button">
          {t('DomainAppsPage:new_install_button')}
        </button>
      </div>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}

      {sonuc && (
        <div className="mb-4 rounded-2xl border border-emerald-200 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/15 p-4">
          <div className="text-sm font-semibold text-emerald-700 dark:text-emerald-300 mb-2">
            {t('DomainAppsPage:installed_ok', { version: sonuc.surum })}
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-1.5 text-sm">
            <div><span className="text-[11px] uppercase text-slate-400 font-semibold">{t('DomainAppsPage:result_site')}</span> <a href={sonuc.site_url} target="_blank" rel="noreferrer" className="text-brand-600 dark:text-brand-400 hover:underline font-mono text-xs">{sonuc.site_url}</a></div>
            <div><span className="text-[11px] uppercase text-slate-400 font-semibold">{t('DomainAppsPage:result_admin')}</span> <a href={sonuc.admin_url} target="_blank" rel="noreferrer" className="text-brand-600 dark:text-brand-400 hover:underline font-mono text-xs">{sonuc.admin_url}</a></div>
            <div><span className="text-[11px] uppercase text-slate-400 font-semibold">{t('DomainAppsPage:result_user')}</span> <span className="font-mono text-xs">{sonuc.admin_kullanici}</span></div>
            <div><span className="text-[11px] uppercase text-slate-400 font-semibold">{t('DomainAppsPage:result_password')}</span> <span className="font-mono text-xs">{sonuc.admin_parola}</span></div>
          </div>
          <p className="text-[11px] text-amber-700 dark:text-amber-400 mt-2">{t('DomainAppsPage:password_warning')}</p>
        </div>
      )}

      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden mb-6">
        <div className="px-4 py-3 border-b border-slate-100 dark:border-slate-700/60">
          <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">{t('DomainAppsPage:installed_title')}</h3>
        </div>
        {yuk ? (
          <div className="px-4 py-8 text-center text-sm text-slate-500 dark:text-slate-400">{t('DomainAppsPage:scanning')}</div>
        ) : liste.length === 0 ? (
          <div className="px-4 py-8 text-center text-sm text-slate-500 dark:text-slate-400">{t('DomainAppsPage:no_installations')}</div>
        ) : (
          <div className="divide-y divide-slate-100 dark:divide-slate-700/60">
            {liste.map(k => {
              const key = k.tur + k.dizin
              const yonet = yonetHedefi(k)
              return (
                <div key={key} className="flex items-center justify-between gap-4 px-4 py-3 flex-wrap">
                  <div className="flex items-center gap-3 min-w-0">
                    <svg viewBox="0 0 24 24" className="w-6 h-6 text-slate-400 shrink-0" fill="none" stroke="currentColor" strokeWidth={1.5}><path d={ICONS[k.tur] || ICONS.wordpress} /></svg>
                    <div className="min-w-0">
                      <div className="font-medium text-slate-800 dark:text-slate-100">{k.ad} <span className="text-xs text-slate-400 font-mono">{k.dizin}</span></div>
                      <div className="text-xs text-slate-500 dark:text-slate-400">
                        {t('DomainAppsPage:version_label')} {k.surum || '—'}
                        {k.durum === 'eski' && <span className="text-amber-600 dark:text-amber-400 font-medium ml-1">{t('DomainAppsPage:status_update_to', { version: k.son_surum })}</span>}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-1.5 flex-wrap">
                    <a href={k.admin_url} target="_blank" rel="noreferrer" className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">{t('DomainAppsPage:admin_link')}</a>
                    {yonet && <button onClick={() => navigate(yonet)} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">{t('DomainAppsPage:manage_link')}</button>}
                    <button disabled={!!mesgul} onClick={() => guncelle(k)} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-50">
                      {mesgul === key ? '…' : t('DomainAppsPage:update_link')}
                    </button>
                    <button disabled={!!mesgul} onClick={() => sil(k)} className="text-xs px-2.5 py-1 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50">{t('DomainAppsPage:delete')}</button>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {sihirbazAcik && !seciliTur && (
        <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4">
          <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">{t('DomainAppsPage:pick_app_title')}</h3>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            {turler.map(tb => (
              <button key={tb.slug} onClick={() => turSec(tb)} className="flex flex-col items-center gap-2 p-4 border border-slate-200 dark:border-slate-700 rounded-xl hover:border-brand-400 dark:hover:border-brand-500 hover:bg-slate-50 dark:hover:bg-slate-800">
                <svg viewBox="0 0 24 24" className="w-8 h-8 text-slate-500" fill="none" stroke="currentColor" strokeWidth={1.5}><path d={ICONS[tb.slug] || ICONS.wordpress} /></svg>
                <span className="text-sm font-medium text-slate-700 dark:text-slate-200">{tb.ad}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {sihirbazAcik && seciliTur && (
        <form onSubmit={kur} className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 max-w-2xl">
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">{t('DomainAppsPage:install_title', { ad: seciliTur.ad })}</h3>
            <button type="button" onClick={() => setSeciliTur(null)} className="text-xs text-slate-500 hover:text-slate-700 dark:hover:text-slate-300">{t('DomainAppsPage:pick_different')}</button>
          </div>
          <div className="mb-3">
            <label className="ta-label-sm">{t('DomainAppsPage:subdir_label')}</label>
            <input value={altDizin} onChange={e => setAltDizin(e.target.value)} placeholder={t('DomainAppsPage:subdir_placeholder')} className="ta-input ta-input-sm w-full font-mono" />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            {seciliTur.form_alanlari.map(fa => (
              <label key={fa.anahtar} className="block">
                <span className="ta-label-sm">{fa.etiket}</span>
                <input
                  value={alanlar[fa.anahtar] || ''}
                  onChange={e => setAlanlar(a => ({ ...a, [fa.anahtar]: e.target.value }))}
                  required={fa.zorunlu}
                  placeholder={fa.yer_tutucu}
                  type={fa.tur === 'password' ? 'password' : fa.tur === 'email' ? 'email' : 'text'}
                  className="ta-input ta-input-sm w-full"
                />
              </label>
            ))}
          </div>
          <button disabled={kuruyor} className="ta-primary-button mt-3 w-full sm:w-auto">
            {kuruyor ? t('DomainAppsPage:installing_button') : t('DomainAppsPage:install_button')}
          </button>
        </form>
      )}
    </div>
  )
}
```

`frontend/src/App.tsx` değişiklikleri:
1. `const DomainWordPressPage = lazy(...)` satırının altına ekle: `const DomainAppsPage = lazy(() => import('@/pages/DomainAppsPage'))`.
2. `<Route path="abonelikler/:id/wordpress" element={<DomainWordPressPage />} />` satırını (mevcut, DEĞİŞMEDEN kalır) hemen altına ekle:
```tsx
        <Route path="abonelikler/:id/uygulamalar" element={<DomainAppsPage />} />
```

`frontend/src/components/DomainPano.tsx` değişiklikleri:
- `ICONS` objesine ekle: `prestashop: 'M12 3l7 4v10l-7 4-7-4V7l7-4z',` (mevcut `wordpress` satırının hemen altına).
- İlk `Grup` bloğunu:
```tsx
      <Grup baslik={t('DomainPano:groups.apps')}>
        <ToolCard etiket={t('DomainPano:items.wordpress')} aciklama={t('DomainPano:items.wordpress_desc')} ikon={ICONS.wordpress} renk="sky" onClick={git('wordpress')} />
      </Grup>
```
şuna değiştir:
```tsx
      <Grup baslik={t('DomainPano:groups.apps')}>
        <ToolCard etiket={t('DomainPano:items.apps')} aciklama={t('DomainPano:items.apps_desc')} ikon={ICONS.wordpress} renk="sky" onClick={git('uygulamalar')} />
      </Grup>
```

`frontend/src/i18n/locales/tr/DomainPano.json` `items` bloğuna ekle (mevcut `wordpress`/`wordpress_desc` anahtarlarının yerine, çünkü artık kart tek "Uygulamalar" kartı):
```json
    "apps": "Uygulamalar",
    "apps_desc": "1-tıkla kurulum · yönetim",
```
`wordpress`/`wordpress_desc` anahtarlarını **kaldır** (artık hiçbir yerden çağrılmıyor). `frontend/src/i18n/locales/en/DomainPano.json`'a aynı değişikliği İngilizce karşılıklarıyla uygula (`"apps": "Apps"`, `"apps_desc": "1-click install · management"`).

`frontend/src/components/DashboardLayout.tsx` — `domainNav(id, t)` içindeki (Task 11'de dokunulmayan) satırı:
```ts
  { to: y('/wordpress'),     etiket: t('DashboardLayout:items.wordpress'),    ikon: ICONS.wp },
```
şuna değiştir:
```ts
  { to: y('/uygulamalar'),   etiket: t('DashboardLayout:items.apps'),         ikon: ICONS.wp },
```
Bu değişiklikle `DashboardLayout.json`'daki `items.wordpress` anahtarı artık hiçbir yerden (ne `nav`, ne `bayiNav` — Task 11'de değişti —, ne `domainNav`) kullanılmıyor; `items.wordpress` anahtarını `tr`/`en` `DashboardLayout.json`'dan **kaldır**.

- [ ] **Step 4: Verify build**

Run: `cd frontend && npm run lint && npm run build`
Expected: lint temiz, build başarılı.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/DomainAppsPage.tsx frontend/src/i18n/locales/tr/DomainAppsPage.json frontend/src/i18n/locales/en/DomainAppsPage.json frontend/src/App.tsx frontend/src/components/DomainPano.tsx frontend/src/components/DashboardLayout.tsx frontend/src/i18n/locales/tr/DomainPano.json frontend/src/i18n/locales/en/DomainPano.json frontend/src/i18n/locales/tr/DashboardLayout.json frontend/src/i18n/locales/en/DashboardLayout.json
git commit -m "feat(frontend): DomainAppsPage — domain bazlı kur/yönet, tek 'Uygulamalar' kartı"
```

---

## Task 13: Doğrulama — build/test/vet + manuel smoke test

**Files:** (yok — yalnız doğrulama)

- [ ] **Step 1: Backend tam derleme + testler + vet**

```bash
/usr/local/go/bin/go build ./...
/usr/local/go/bin/go vet ./...
/usr/local/go/bin/go test ./...
```
Expected: hepsi hatasız/PASS. `internal/wordpress`'in ESKİ testlerinin (wpconfig_test.go, wpdrop_test.go, wpinstall_guard_test.go) hâlâ PASS olduğunu özellikle doğrula — bunlar hiç değişmedi, regresyon sinyali.

- [ ] **Step 2: Frontend tam derleme**

```bash
cd frontend && npm ci && npm run lint && npm run build
```
Expected: hatasız.

- [ ] **Step 3: Manuel smoke test — gerçek sunucuda**

Bu proje dev server'la yerel test edilemiyor (SSH/runuser/gerçek domain kullanıcıları gerektiriyor) — gerçek bir SanalCP sunucusuna (üretim ya da test) deploy edilip aşağıdaki akış elle doğrulanmalı:

1. `/uygulamalar` sayfası açılıyor, mevcut WordPress kurulumları (varsa) tür sütunuyla listede görünüyor (regresyon: eski `/wordpress` URL'sine giden biri `/uygulamalar`'a yönlendiriliyor mu?).
2. Bir domain panelinde "Uygulamalar" kartına tıklanınca `DomainAppsPage` açılıyor, "+ Yeni Uygulama Kur" ile tür seçim ızgarası (WordPress + PrestaShop) görünüyor.
3. WordPress seçilip kurulum yapılıyor — **kritik regresyon kontrolü:** DB adı `wp_xxxxxxxx` / kullanıcı `wpu_xxxxxxxx` deseninde mi (yeni `/apps/wordpress/kur` yolu üzerinden bile eski deseni koruyor mu)? Kurulum sonrası eski `/abonelikler/{id}/wordpress` sayfasından "Yönet" ile açılıp eklenti/tema sekmelerinin hâlâ çalıştığı doğrulanıyor.
4. PrestaShop seçilip kurulum deneniyor — 🔴 **bu adım Task 8'deki doğrulanmamış indirme URL'sini test eder.** `download.prestashop.com/download/releases/prestashop_<sürüm>.zip` 200 dönüyor mu? Dönmüyorsa (404/522/vb.) `internal/prestashop/indirici.go`'daki `psIndirVeAc` fonksiyonundaki URL kalıbı gerçek çalışan adresle güncellenip Task 8'in commit'i düzeltme commit'iyle takip edilmeli — bu, planın geri kalanını etkilemez (tek dosya, tek fonksiyon).
5. PrestaShop kurulumu başarılıysa: admin URL'sinin rastgele isimlendirilmiş `adminXXXXXXXXXX/` dizinine doğru işaret ettiği, `install/index_cli.php`'nin başarı çıktısının (`Installation successful`) doğru yakalandığı doğrulanıyor.
6. PrestaShop satırında "Güncelle" butonuna basılınca 400 + "bu uygulama için güncelleme desteklenmiyor" mesajı geldiği doğrulanıyor (spec'in bilinçli kapsam-dışı kararı).
7. Hem WordPress hem PrestaShop için "Sil" (DB dahil) çalışıyor, ardından `/uygulamalar` ve `DomainAppsPage` listelerinden kayboluyor.
8. `/apps/tumu` (global liste) bayi/admin rolüyle açılıp her iki türün de doğru `tur_adi` ile göründüğü doğrulanıyor; müşteri rolüyle sadece kendi domainleri görünüyor (KapsamSQL regresyonu).

- [ ] **Step 4: Sonuçları raporla**

Adım 3'teki her maddenin sonucunu (✅/❌ + varsa hata mesajı) kullanıcıya raporla. PrestaShop indirme URL'si çalışmadıysa Task 8'e dönüp düzelt (ayrı commit).

- [ ] **Step 5: Final commit (yalnızca Step 3'te bir düzeltme gerektiyse)**

```bash
git add internal/prestashop/indirici.go
git commit -m "fix(prestashop): indirme URL'sini gerçek sunucuda doğrulanan adresle güncelle"
```
