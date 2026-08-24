package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"sanalcp/internal/panelbayrak"
)

func sqlmockRows(deger int) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"demo_modu_acik"}).AddRow(deger)
}

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
	panelbayrak.OnbellekSifirla() // DemoModuAcik'in 5sn'lik önbelleği testler arası sızmasın
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
	panelbayrak.OnbellekSifirla()
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
	panelbayrak.OnbellekSifirla()
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
		panelbayrak.OnbellekSifirla()
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
	panelbayrak.OnbellekSifirla()
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
