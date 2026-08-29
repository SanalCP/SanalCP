package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPanelErisimKisiti_IzinliVeYasakIP(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	eski := scopeDB
	scopeDB = db
	t.Cleanup(func() { scopeDB = eski })

	h := PanelErisimKisiti(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for _, tc := range []struct {
		ip   string
		want int
	}{{"192.0.2.8:1234", 204}, {"[2001:db8::8]:1234", 204}, {"198.51.100.8:1234", 403}} {
		mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"erisim_cidrleri", "gecici", "aktif"}).AddRow("192.0.2.0/24\n2001:db8::/48", "", 0))
		r := httptest.NewRequest(http.MethodGet, "/api/v1/public/dil", nil)
		r.RemoteAddr = tc.ip
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("ip=%s status=%d, want %d", tc.ip, w.Code, tc.want)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPanelErisimKisiti_BosListeGecirir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	eski := scopeDB
	scopeDB = db
	t.Cleanup(func() { scopeDB = eski })
	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"erisim_cidrleri", "gecici", "aktif"}).AddRow("", "", 0))
	h := PanelErisimKisiti(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestPanelErisimKisiti_AktifGeciciCIDR(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	eski := scopeDB
	scopeDB = db
	t.Cleanup(func() { scopeDB = eski })
	h := PanelErisimKisiti(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	for _, tc := range []struct{ aktif, want int }{{1, 204}, {0, 403}} {
		mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"kalici", "gecici", "aktif"}).AddRow("192.0.2.0/24", "198.51.100.8/32", tc.aktif))
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "198.51.100.8:1234"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("aktif=%d status=%d, want %d", tc.aktif, w.Code, tc.want)
		}
	}
}
