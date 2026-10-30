package mail

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"

	"sanalcp/internal/auth"
	"sanalcp/internal/kota"
	"sanalcp/internal/middleware"
)

func TestKotaIzinli(t *testing.T) {
	const gb = 1024
	for _, tc := range []struct {
		ad      string
		rol     string
		planMB  int64
		istenen int64
		izinli  bool
	}{
		{"müşteri plan altında", middleware.RolMusteri, gb, 500, true},
		{"müşteri tam plan", middleware.RolMusteri, gb, gb, true},
		{"müşteri plan üstü", middleware.RolMusteri, gb, gb + 1, false},
		{"müşteri limitli planda sınırsız (0) isteyemez", middleware.RolMusteri, gb, 0, false},
		{"müşteri sınırsız planda sınırsız", middleware.RolMusteri, 0, 0, true},
		{"müşteri sınırsız planda büyük değer", middleware.RolMusteri, 0, 100 * gb, true},
		{"admin plan üstü", middleware.RolAdmin, gb, 10 * gb, true},
		{"admin sınırsız", middleware.RolAdmin, gb, 0, true},
		{"bayi plan üstü", middleware.RolBayi, gb, 10 * gb, true},
		{"rolsüz plan üstü (fail-closed)", "", gb, gb + 1, false},
		{"tanınmayan rol sınırsız (fail-closed)", "operator", gb, 0, false},
		{"admin bile mutlak sınırı aşamaz", middleware.RolAdmin, 0, kota.MaxMailboxQuotaMB + 1, false},
		{"admin mutlak sınırda", middleware.RolAdmin, 0, kota.MaxMailboxQuotaMB, true},
		{"negatif", middleware.RolAdmin, 0, -1, false},
	} {
		t.Run(tc.ad, func(t *testing.T) {
			err := kotaIzinli(tc.rol, tc.planMB, tc.istenen)
			if (err == nil) != tc.izinli {
				t.Fatalf("kotaIzinli(%q, plan=%d, istenen=%d) err=%v, izinli=%v bekleniyordu", tc.rol, tc.planMB, tc.istenen, err, tc.izinli)
			}
		})
	}
}

func TestKotaBayt(t *testing.T) {
	if b, err := kotaBayt(0); err != nil || b != 0 {
		t.Errorf("0 MB: %d %v", b, err)
	}
	if b, err := kotaBayt(1); err != nil || b != 1<<20 {
		t.Errorf("1 MB: %d %v", b, err)
	}
	// Üst sınır bayta taşmadan çevrilir (1 TiB).
	if b, err := kotaBayt(kota.MaxMailboxQuotaMB); err != nil || b != 1<<40 {
		t.Errorf("max: %d %v", b, err)
	}
	for _, kotu := range []int64{-1, kota.MaxMailboxQuotaMB + 1, 1 << 62} {
		if _, err := kotaBayt(kotu); err == nil {
			t.Errorf("%d MB reddedilmeliydi", kotu)
		}
	}
}

func TestMaildirsizeParse(t *testing.T) {
	for _, tc := range []struct {
		ad    string
		giris string
		used  int64
		ok    bool
	}{
		{"limitli, birikimli satırlar", "1048576S,100C\n30213 1\n30213 1\n-10000 -1\n", 50426, true},
		{"sınırsız başlık", "0S\n150160 5\n", 150160, true},
		{"yalnız başlık", "1000S\n", 0, true},
		{"yeniden hesaplanmış", "100000S\n0 0\n30213 1\n", 30213, true},
		{"eksi toplam sıfırlanır", "1000S\n-500 -1\n", 0, true},
		{"boş dosya", "", 0, false},
		{"başlık yok", "30213 1\n", 0, false},
		{"bozuk satır", "1000S\n12 abc\n", 0, false},
		{"fazla alan", "1000S\n1 2 3\n", 0, false},
		{"kırpılmış/devasa dosya", "1000S\n" + strings.Repeat("1 1\n", maildirsizeMaxBayt), 0, false},
	} {
		t.Run(tc.ad, func(t *testing.T) {
			used, ok := maildirsizeParse(strings.NewReader(tc.giris))
			if ok != tc.ok || used != tc.used {
				t.Fatalf("used=%d ok=%v, beklenen used=%d ok=%v", used, ok, tc.used, tc.ok)
			}
		})
	}
}

// Geçersiz tenant/kutu adı diske hiç dokunmadan "bilinmiyor" döner.
func TestKutuKullanimiGecersizAdlariReddeder(t *testing.T) {
	for _, c := range [][2]string{{"", "bilgi"}, {"root", "bilgi"}, {"c_ornek", "../etc"}, {"c_ornek", "a/b"}, {"c_ornek", ""}} {
		if _, ok := kutuKullanimi(c[0], c[1]); ok {
			t.Errorf("kutuKullanimi(%q, %q) ok döndü", c[0], c[1])
		}
	}
}

// ---- HTTP: PUT /domains/{id}/mail/{mid}/quota ----

const (
	sqlDomainKota = `SELECT sistem_kullanici, COALESCE\(is_demo,0\) FROM domains WHERE id=\?`
	sqlKutuKota   = `SELECT email FROM mailboxes WHERE id=\? AND domain_id=\?`
	sqlPlanKota   = `FROM domains d LEFT JOIN service_plans p`
	sqlGuncelle   = `UPDATE mailboxes SET quota_bytes=\? WHERE id=\? AND domain_id=\?`
)

// kotaIstegi: handler'ı çağırır. guncellenecekBayt != nil ise UPDATE'in o değerle
// çalışması BEKLENİR; nil ise hiç UPDATE çalışmamalıdır (beklenmeyen Exec hata sayılır).
func kotaIstegi(t *testing.T, rol, govde string, planMB int64, guncellenecekBayt *int64) (*httptest.ResponseRecorder, sqlmock.Sqlmock, *[]string) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := &Handlers{DB: db}

	mock.ExpectQuery(sqlDomainKota).WillReturnRows(sqlmock.NewRows([]string{"sk", "demo"}).AddRow("c_ornek", 0))
	mock.ExpectQuery(sqlKutuKota).WithArgs(int64(9), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("bilgi@ornek.com"))
	mock.ExpectQuery(sqlPlanKota).WillReturnRows(
		sqlmock.NewRows([]string{"ana", "plan_id", "id", "kota"}).AddRow(nil, 3, 3, planMB))
	if guncellenecekBayt != nil {
		mock.ExpectExec(sqlGuncelle).WithArgs(*guncellenecekBayt, int64(9), int64(5)).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	bosaltilan := []string{}
	eski := authOnbellekBosalt
	authOnbellekBosalt = func(_ context.Context, email string) error {
		bosaltilan = append(bosaltilan, email)
		return nil
	}
	t.Cleanup(func() { authOnbellekBosalt = eski })

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "5")
	rctx.URLParams.Add("mid", "9")
	r := httptest.NewRequest(http.MethodPut, "/domains/5/mail/9/quota", strings.NewReader(govde))
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	if rol != "" {
		ctx = auth.ClaimsContext(ctx, &auth.Claims{UserID: 1, Username: "u", Role: rol})
	}
	w := httptest.NewRecorder()
	h.KotaGuncelle(w, r.WithContext(ctx))
	return w, mock, &bosaltilan
}

func bayt(mb int64) *int64 { b := mb << 20; return &b }

// 🔴 Yetki sınırı: müşteri plan sınırını aşamaz. UPDATE hiç çalışmamalı.
func TestKotaGuncelleMusteriPlaniAsamaz(t *testing.T) {
	for _, govde := range []string{`{"quota_mb":1025}`, `{"quota_mb":0}`} {
		w, mock, bosalt := kotaIstegi(t, middleware.RolMusteri, govde, 1024, nil)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s → %d %s, 403 bekleniyordu", govde, w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("%s: %v", govde, err)
		}
		if len(*bosalt) != 0 {
			t.Errorf("reddedilen istek önbellek boşalttı: %v", *bosalt)
		}
	}
}

func TestKotaGuncelleMusteriPlanIcindeUygulanirVeOnbellekBosalir(t *testing.T) {
	w, mock, bosalt := kotaIstegi(t, middleware.RolMusteri, `{"quota_mb":512}`, 1024, bayt(512))
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
	// Limit değişince Dovecot'un userdb önbelleği o kutu için atılmalı; aksi
	// hâlde yeni kota bir saate kadar yürürlüğe girmez.
	if len(*bosalt) != 1 || (*bosalt)[0] != "bilgi@ornek.com" {
		t.Errorf("önbellek boşaltma: %v", *bosalt)
	}
}

func TestKotaGuncelleAdminVeBayiPlaniAsabilir(t *testing.T) {
	for _, rol := range []string{middleware.RolAdmin, middleware.RolBayi} {
		w, mock, _ := kotaIstegi(t, rol, `{"quota_mb":5000}`, 1024, bayt(5000))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", rol, w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("%s: %v", rol, err)
		}
	}
	// 0 = sınırsız yalnız yönetici/bayi.
	w, _, _ := kotaIstegi(t, middleware.RolAdmin, `{"quota_mb":0}`, 1024, bayt(0))
	if w.Code != http.StatusOK {
		t.Fatalf("admin sınırsız: %d %s", w.Code, w.Body.String())
	}
}

// Geçersiz gövde DB'ye gitmeden 400 almalı (kutu/plan sorgusu bile yapılmaz).
func TestKotaGuncelleGecersizGovde(t *testing.T) {
	for _, govde := range []string{``, `{}`, `{"quota_mb":null}`, `{"quota_mb":"5"}`, `{"quota_mb":1.5}`, `{"quota_mb":-1}`, `{"quota_mb":1048577}`, `{"quota_mb":99999999999999999999}`} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectQuery(sqlDomainKota).WillReturnRows(sqlmock.NewRows([]string{"sk", "demo"}).AddRow("c_ornek", 0))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "5")
		rctx.URLParams.Add("mid", "9")
		r := httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(govde))
		r = r.WithContext(auth.ClaimsContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx),
			&auth.Claims{Role: middleware.RolAdmin}))
		w := httptest.NewRecorder()
		(&Handlers{DB: db}).KotaGuncelle(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%q → %d, 400 bekleniyordu", govde, w.Code)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("%q: %v", govde, err)
		}
		db.Close()
	}
}

// ---- yeni kutu varsayılanı ----

func yeniKutuKotasiDene(t *testing.T, rol string, planMB int64, istenen *int64) (int64, error) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(sqlPlanKota).WillReturnRows(
		sqlmock.NewRows([]string{"ana", "plan_id", "id", "kota"}).AddRow(nil, 3, 3, planMB))
	return (&Handlers{DB: db}).yeniKutuKotasi(context.Background(), rol, 5, istenen)
}

func TestYeniKutuKotasiPlanVarsayilaniniDevralir(t *testing.T) {
	if b, err := yeniKutuKotasiDene(t, middleware.RolMusteri, 1024, nil); err != nil || b != 1024<<20 {
		t.Fatalf("plan varsayılanı: %d %v", b, err)
	}
	// Sınırsız plan → sınırsız kutu.
	if b, err := yeniKutuKotasiDene(t, middleware.RolMusteri, 0, nil); err != nil || b != 0 {
		t.Fatalf("sınırsız plan: %d %v", b, err)
	}
	// Bozuk DB verisi (plan kotası mutlak sınırın üstünde) taşma üretmez.
	if b, err := yeniKutuKotasiDene(t, middleware.RolAdmin, 1<<40, nil); err != nil || b != int64(kota.MaxMailboxQuotaMB)<<20 {
		t.Fatalf("sınırlı varsayılan: %d %v", b, err)
	}
}

func TestYeniKutuKotasiAcikDegerYetkiKurali(t *testing.T) {
	ver := func(v int64) *int64 { return &v }
	if b, err := yeniKutuKotasiDene(t, middleware.RolMusteri, 1024, ver(256)); err != nil || b != 256<<20 {
		t.Fatalf("müşteri plan içi: %d %v", b, err)
	}
	if _, err := yeniKutuKotasiDene(t, middleware.RolMusteri, 1024, ver(2048)); err == nil {
		t.Fatal("müşteri plan üstü kabul edildi")
	}
	if b, err := yeniKutuKotasiDene(t, middleware.RolAdmin, 1024, ver(2048)); err != nil || b != 2048<<20 {
		t.Fatalf("admin plan üstü: %d %v", b, err)
	}
}

func TestYeniKutuKotasiPlanOkunamazsaKapaliKalir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(sqlPlanKota).WillReturnError(errors.New("db down"))
	if _, err := (&Handlers{DB: db}).yeniKutuKotasi(context.Background(), middleware.RolMusteri, 5, nil); !errors.Is(err, errPlanOkunamadi) {
		t.Fatalf("err=%v, errPlanOkunamadi bekleniyordu", err)
	}
}
