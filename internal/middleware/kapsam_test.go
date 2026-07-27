package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Sahiplik zinciri ve kapsam denetiminin regresyon paketi (Faz 5F).
//
// Buradaki testler DB'yi sqlmock ile taklit eder; amaç sorgunun ne döndürdüğü
// değil, DÖNEN SONUCA GÖRE ERİŞİM KARARININ ne olduğudur. Özellikle iki
// davranış kilitlenir:
//
//   - fail-closed: sahiplik sorgusu hata verirse erişim REDDEDİLİR.
//   - rol ayrımı: admin serbest, bayi/müşteri yalnız kendi zincirinde.
//
// Bu dosya, Faz 5A/5C/5D'de canlı olarak doğrulanan davranışları kalıcı hâle
// getirir — canlı test bir kez çalışır, bu paket her derlemede çalışır.

// mockDB: scopeDB'yi sahte bir DB'ye bağlar ve testin sonunda geri alır.
func mockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	eski := scopeDB
	scopeDB = db
	t.Cleanup(func() {
		scopeDB = eski
		db.Close()
	})
	return mock
}

func TestBayiDomainiMi(t *testing.T) {
	tablo := []struct {
		ad      string
		sayim   int
		hata    bool
		beklfen bool
	}{
		{"kendi müşterisinin domaini", 1, false, true},
		{"başkasının domaini", 0, false, false},
		{"DB hatası → fail-closed", 0, true, false},
	}

	for _, tc := range tablo {
		t.Run(tc.ad, func(t *testing.T) {
			mock := mockDB(t)
			q := mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*)"))
			if tc.hata {
				q.WillReturnError(http.ErrNotSupported)
			} else {
				q.WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(tc.sayim))
			}

			r := istekRol(RolBayi, 7)
			if got := BayiDomainiMi(r, 7, 42); got != tc.beklfen {
				t.Errorf("BayiDomainiMi = %v, beklenen %v", got, tc.beklfen)
			}
		})
	}
}

func TestMusteriKullanicisininDomainiMi(t *testing.T) {
	t.Run("kendi domaini", func(t *testing.T) {
		mock := mockDB(t)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*)")).
			WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
		if !MusteriKullanicisininDomainiMi(istekRol(RolMusteri, 5), 5, 9) {
			t.Error("kendi domainine erişebilmeliydi")
		}
	})

	t.Run("başkasının domaini", func(t *testing.T) {
		mock := mockDB(t)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*)")).
			WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
		if MusteriKullanicisininDomainiMi(istekRol(RolMusteri, 5), 5, 9) {
			t.Error("başkasının domainine erişmemeliydi")
		}
	})

	t.Run("geçersiz kullanıcı id", func(t *testing.T) {
		mockDB(t) // sorgu beklenmiyor: uid<=0 erken döner
		if MusteriKullanicisininDomainiMi(istekRol(RolMusteri, 0), 0, 9) {
			t.Error("uid=0 için erişim reddedilmeliydi")
		}
	})
}

// TestMusteriScopeRolMatrisi: kapsam middleware'inin tüm rol × sahiplik
// kombinasyonlarındaki HTTP sonucunu kilitler.
func TestMusteriScopeRolMatrisi(t *testing.T) {
	tablo := []struct {
		ad       string
		rol      string
		sahipMi  int  // sahiplik sorgusunun döndüreceği sayım
		sorguVar bool // bu rol için sahiplik sorgusu bekleniyor mu
		askiVar  bool // askı sorgusu bekleniyor mu
		askida   int
		beklenen int
	}{
		{"admin her domaine girer", RolAdmin, 0, false, false, 0, http.StatusOK},
		{"bayi kendi domainine girer", RolBayi, 1, true, false, 0, http.StatusOK},
		{"bayi başkasının domainine giremez", RolBayi, 0, true, false, 0, http.StatusForbidden},
		{"müşteri kendi domainine girer", RolMusteri, 1, true, true, 0, http.StatusOK},
		{"müşteri başkasının domainine giremez", RolMusteri, 0, true, false, 0, http.StatusForbidden},
		{"müşteri askıdaki domaine giremez", RolMusteri, 1, true, true, 1, http.StatusForbidden},
	}

	for _, tc := range tablo {
		t.Run(tc.ad, func(t *testing.T) {
			mock := mockDB(t)
			if tc.sorguVar {
				mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*)")).
					WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(tc.sahipMi))
			}
			if tc.askiVar {
				mock.ExpectQuery(regexp.QuoteMeta("COALESCE(askida,0)")).
					WillReturnRows(sqlmock.NewRows([]string{"askida"}).AddRow(tc.askida))
			}

			rec := httptest.NewRecorder()
			MusteriScope(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})).ServeHTTP(rec, istekRol(tc.rol, 7))

			if rec.Code != tc.beklenen {
				t.Errorf("kod = %d, beklenen %d", rec.Code, tc.beklenen)
			}
		})
	}
}

// TestKapsamSQLIzolasyon: iki farklı bayinin ürettiği koşulun aynı olmadığını
// ve her birinin kendi kullanıcı kimliğini taşıdığını doğrular. Faz 5D'de
// canlı testte görülen izolasyonun birim karşılığı.
func TestKapsamSQLIzolasyon(t *testing.T) {
	kosulA, argA := KapsamSQL(istekRol(RolBayi, 10), "d")
	kosulB, argB := KapsamSQL(istekRol(RolBayi, 11), "d")

	if kosulA != kosulB {
		t.Errorf("aynı rol için koşul metni farklı: %q vs %q", kosulA, kosulB)
	}
	if len(argA) != 1 || argA[0] != int64(10) {
		t.Errorf("bayi A argümanı yanlış: %v", argA)
	}
	if len(argB) != 1 || argB[0] != int64(11) {
		t.Errorf("bayi B argümanı yanlış: %v", argB)
	}
	// Koşul, sahiplik zincirini (customers.owner_user_id) kullanmalı; yalnız
	// domain tablosuna bakan bir filtre bayi izolasyonunu sağlamaz.
	if !regexp.MustCompile(`owner_user_id`).MatchString(kosulA) {
		t.Errorf("bayi koşulu sahiplik zincirini kullanmıyor: %q", kosulA)
	}
}

// TestKapsamSQLMusteriKendiDomaini: müşteri koşulu da sahiplik zincirinden
// (customers.user_id) türer — domain kimliği token'a gömülmez, çünkü bir
// müşterinin birden çok domaini olabilir.
func TestKapsamSQLMusteriKendiDomaini(t *testing.T) {
	kosul, arg := KapsamSQL(istekRol(RolMusteri, 42), "d")
	if len(arg) != 1 || arg[0] != int64(42) {
		t.Fatalf("müşteri argümanı yanlış: %v", arg)
	}
	if !regexp.MustCompile(`kc\.user_id = \?`).MatchString(kosul) {
		t.Errorf("müşteri koşulu sahiplik zincirini kullanmıyor: %q", kosul)
	}
	// Bayi sütunuyla karışmamalı: müşteri owner_user_id ile daralırsa kendi
	// domainlerini göremez, başkasınınkini görebilir.
	if regexp.MustCompile(`owner_user_id`).MatchString(kosul) {
		t.Errorf("müşteri koşulu bayi sütununu kullanıyor: %q", kosul)
	}
}
