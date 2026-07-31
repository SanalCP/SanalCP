package users

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"

	"sanalcp/internal/auth"
	"sanalcp/internal/middleware"
)

// Panel hesabı yönetiminin yetki regresyon paketi (Faz 5F).
//
// Kilitlenen davranışlar — hepsi panelden KALICI KİLİTLENMEYE ya da yetki
// yükseltmeye yol açabilecek durumlar:
//
//   - bayi kendi altında olmayan hesaba dokunamaz (yatay)
//   - bayi admin/bayi hesabı açamaz (dikey)
//   - root (id=1) silinemez, askıya alınamaz, parolası sıfırlanamaz
//   - kendi hesabını silme/askıya alma engellidir
//   - son aktif admin silinemez / askıya alınamaz

func istekRol(rol string, uid int64, gövde string, urlID string) *http.Request {
	var okuyucu *strings.Reader
	if gövde == "" {
		okuyucu = strings.NewReader("{}")
	} else {
		okuyucu = strings.NewReader(gövde)
	}
	r := httptest.NewRequest(http.MethodPost, "/", okuyucu)

	// chi URL parametresi
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", urlID)
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)

	c := &auth.Claims{UserID: uid, Username: "test", Role: rol}
	return r.WithContext(middleware.ClaimsContext(ctx, c))
}

func kur(t *testing.T) (*Handlers, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &Handlers{DB: db}, mock
}

func TestRootHesabiKorumali(t *testing.T) {
	tablo := []struct {
		ad    string
		calis func(h *Handlers, w http.ResponseWriter, r *http.Request)
		govde string
	}{
		{"root silinemez", (*Handlers).Sil, ""},
		{"root askıya alınamaz", (*Handlers).DurumDegistir, `{"durum":"suspended"}`},
		{"root parolası sıfırlanamaz", (*Handlers).ParolaSifirla, `{"yeni":"UzunParola123"}`},
	}

	for _, tc := range tablo {
		t.Run(tc.ad, func(t *testing.T) {
			h, _ := kur(t)
			rec := httptest.NewRecorder()
			// Admin olarak dener — yetki değil, root'un dokunulmazlığı sınanıyor.
			tc.calis(h, rec, istekRol(middleware.RolAdmin, 1, tc.govde, "1"))

			if rec.Code != http.StatusForbidden {
				t.Errorf("kod = %d, 403 bekleniyordu (gövde: %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestBayiYatayErisimEngelli(t *testing.T) {
	h, mock := kur(t)
	// Hedef hesabın bayisi BAŞKASI (reseller_id = 99, çağıran 7).
	mock.ExpectQuery(regexp.QuoteMeta("SELECT reseller_id FROM users")).
		WillReturnRows(sqlmock.NewRows([]string{"reseller_id"}).AddRow(99))

	rec := httptest.NewRecorder()
	h.Sil(rec, istekRol(middleware.RolBayi, 7, "", "42"))

	if rec.Code != http.StatusForbidden {
		t.Errorf("bayi başkasının hesabını silememeliydi, kod = %d", rec.Code)
	}
}

func TestBayiSahipsizHesabaDokunamaz(t *testing.T) {
	h, mock := kur(t)
	// reseller_id NULL = admin'e ait hesap.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT reseller_id FROM users")).
		WillReturnRows(sqlmock.NewRows([]string{"reseller_id"}).AddRow(nil))

	rec := httptest.NewRecorder()
	h.Sil(rec, istekRol(middleware.RolBayi, 7, "", "42"))

	if rec.Code != http.StatusForbidden {
		t.Errorf("bayi sahipsiz hesaba dokunamamalıydı, kod = %d", rec.Code)
	}
}

// TestBayiRolYukseltemez: dikey yetki yükseltmesi. Bayi yalnız 'user' açabilir.
func TestBayiRolYukseltemez(t *testing.T) {
	for _, rol := range []string{middleware.RolAdmin, middleware.RolBayi} {
		t.Run("bayi "+rol+" acamaz", func(t *testing.T) {
			h, _ := kur(t)
			govde := `{"kullanici_adi":"yeni_hesap","parola":"UzunParola123","rol":"` + rol + `"}`
			rec := httptest.NewRecorder()
			h.Olustur(rec, istekRol(middleware.RolBayi, 7, govde, ""))

			if rec.Code != http.StatusForbidden {
				t.Errorf("kod = %d, 403 bekleniyordu (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRootKullaniciAdiAyrilmis(t *testing.T) {
	h, _ := kur(t)
	rec := httptest.NewRecorder()
	h.Olustur(rec, istekRol(middleware.RolAdmin, 1,
		`{"kullanici_adi":"root","parola":"UzunParola123","rol":"admin"}`, ""))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("root adıyla hesap açılamamalıydı, kod = %d", rec.Code)
	}
}

func TestKullaniciAdiDogrulama(t *testing.T) {
	// NOT: Büyük harf burada YOK — Olustur kullanıcı adını önce ToLower ile
	// normalize ediyor, yani "Buyuk" geçerli bir ada dönüşüp kabul ediliyor.
	// Bu bilinçli bir kolaylık; normalizasyon ayrıca sınanıyor
	// (TestKullaniciAdiNormalizasyonu).
	gecersiz := []string{"ab", "1abc", "bo şluk", "cok-uzun-bir-kullanici-adi-32-karakteri-asan", "türkçe"}
	for _, ad := range gecersiz {
		t.Run(ad, func(t *testing.T) {
			h, _ := kur(t)
			rec := httptest.NewRecorder()
			govde, _ := json.Marshal(map[string]string{
				"kullanici_adi": ad, "parola": "UzunParola123", "rol": "user",
			})
			h.Olustur(rec, istekRol(middleware.RolAdmin, 1, string(govde), ""))

			if rec.Code != http.StatusBadRequest {
				t.Errorf("%q kabul edilmemeliydi, kod = %d", ad, rec.Code)
			}
		})
	}
}

// TestKullaniciAdiNormalizasyonu: girdi küçük harfe çevrilip kırpılıyor.
// "ROOT" gibi bir varyasyonun da ayrılmış ad kontrolüne takılması bu
// normalizasyona bağlı — sınanmazsa "Root" adıyla ikinci bir hesap açılabilirdi.
func TestKullaniciAdiNormalizasyonu(t *testing.T) {
	h, _ := kur(t)
	rec := httptest.NewRecorder()
	h.Olustur(rec, istekRol(middleware.RolAdmin, 1,
		`{"kullanici_adi":"  ROOT  ","parola":"UzunParola123","rol":"admin"}`, ""))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("'  ROOT  ' normalize edilip reddedilmeliydi, kod = %d (%s)",
			rec.Code, rec.Body.String())
	}
}

func TestKendiHesabinaYikiciIslemEngelli(t *testing.T) {
	tablo := []struct {
		ad    string
		calis func(h *Handlers, w http.ResponseWriter, r *http.Request)
		govde string
	}{
		{"kendini silemez", (*Handlers).Sil, ""},
		{"kendini askıya alamaz", (*Handlers).DurumDegistir, `{"durum":"suspended"}`},
	}

	for _, tc := range tablo {
		t.Run(tc.ad, func(t *testing.T) {
			h, _ := kur(t)
			rec := httptest.NewRecorder()
			// Admin (id=5) kendi id'si üzerinde işlem deniyor.
			tc.calis(h, rec, istekRol(middleware.RolAdmin, 5, tc.govde, "5"))

			if rec.Code != http.StatusForbidden {
				t.Errorf("kod = %d, 403 bekleniyordu (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestSonAdminKorumasi: sistemde başka aktif admin kalmadığında silme/askı
// reddedilmeli — yoksa panele girecek kimse kalmaz.
func TestSonAdminKorumasi(t *testing.T) {
	t.Run("silme", func(t *testing.T) {
		h, mock := kur(t)
		// sonAdminMi: başka aktif admin sayısı 0
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM users WHERE role='admin'")).
			WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))

		rec := httptest.NewRecorder()
		h.Sil(rec, istekRol(middleware.RolAdmin, 5, "", "9"))

		if rec.Code != http.StatusForbidden {
			t.Errorf("son admin silinememeliydi, kod = %d (%s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("askıya alma", func(t *testing.T) {
		h, mock := kur(t)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM users WHERE role='admin'")).
			WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))

		rec := httptest.NewRecorder()
		h.DurumDegistir(rec, istekRol(middleware.RolAdmin, 5, `{"durum":"suspended"}`, "9"))

		if rec.Code != http.StatusForbidden {
			t.Errorf("son admin askıya alınamamalıydı, kod = %d (%s)", rec.Code, rec.Body.String())
		}
	})
}

func TestGecersizDurumReddedilir(t *testing.T) {
	h, _ := kur(t)
	rec := httptest.NewRecorder()
	h.DurumDegistir(rec, istekRol(middleware.RolAdmin, 1, `{"durum":"silinmis"}`, "9"))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("geçersiz durum reddedilmeliydi, kod = %d", rec.Code)
	}
}
