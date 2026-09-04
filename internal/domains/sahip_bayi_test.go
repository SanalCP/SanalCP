package domains

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"sanalcp/internal/auth"
	"sanalcp/internal/middleware"
)

// Sahip değiştirme, domain.customer_id değerini başka bir yönetici müşterisine
// bağlamak yerine mevcut müşterinin owner_user_id alanını değiştirmeli. Aksi
// hâlde bayi sütunu admin kalır ve hedef bayi domaini göremez.
func TestTopluSahipBayiKapsaminiGunceller(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT role, status FROM users WHERE id=?`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"role", "status"}).AddRow("reseller", "active"))
	mock.ExpectExec(`UPDATE customers c[[:space:]]+JOIN domains d ON d.customer_id = c.id[[:space:]]+SET c.owner_user_id=\?[[:space:]]+WHERE d.id IN \(\?\)`).
		WithArgs(int64(42), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM domains d JOIN customers c ON c.id=d.customer_id WHERE c.owner_user_id = \?[[:space:]]+AND d.id IN \(\?\)`).
		WithArgs(int64(42), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))

	h := &Handlers{DB: db}
	req := httptest.NewRequest(http.MethodPost, "/domains/toplu/sahip",
		bytes.NewBufferString(`{"ids":[7],"bayi_user_id":42}`))
	req = middleware.ClaimsIle(req, &auth.Claims{UserID: 1, Role: middleware.RolAdmin})
	w := httptest.NewRecorder()
	h.TopluSahip(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("kod=%d gövde=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// istekRol: verilen rol/kullanıcı ile kimliklendirilmiş bir istek üretir.
func istekRol(rol string, userID int64) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/domains", nil)
	return middleware.ClaimsIle(r, &auth.Claims{UserID: userID, Role: rol})
}

func i64(v int64) *int64 { return &v }

// 🔴 YETKİ: bayi, bayi_user_id göndererek domaini BAŞKA bir bayiye yazamamalı.
// Bu geçseydi bir bayi rakibinin kotasını harcayabilir ya da domainini onun
// kapsamına atabilirdi. Sahip daima çağıranın kendisi olmalı.
func TestSahipBayiCozBayiBaskasiniSecemez(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handlers{DB: db}

	// Bayi 7, domaini bayi 99'a yazmaya çalışıyor.
	sahip, hata := h.sahipBayiCoz(istekRol(middleware.RolBayi, 7), i64(99))
	if hata != "" {
		t.Fatalf("bayi için hata beklenmiyordu: %s", hata)
	}
	if sahip == nil || *sahip != 7 {
		t.Fatalf("sahip = %v, beklenen 7 (çağıranın kendisi) — istekteki 99 dikkate alınmamalı", sahip)
	}
	// DB'ye hiç gidilmemeli: bayi yolunda doğrulanacak bir şey yok.
}

// Admin bayi seçmezse sahip nil kalır = doğrudan admin'e ait.
func TestSahipBayiCozAdminVarsayilanNil(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handlers{DB: db}

	for _, istenen := range []*int64{nil, i64(0), i64(-1)} {
		sahip, hata := h.sahipBayiCoz(istekRol(middleware.RolAdmin, 1), istenen)
		if hata != "" || sahip != nil {
			t.Errorf("istenen=%v -> sahip=%v hata=%q; nil/nil bekleniyordu", istenen, sahip, hata)
		}
	}
}

// Admin geçerli, aktif bir bayi seçebilir.
func TestSahipBayiCozAdminGecerliBayi(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT role, status FROM users WHERE id=?`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"role", "status"}).AddRow("reseller", "active"))

	h := &Handlers{DB: db}
	sahip, hata := h.sahipBayiCoz(istekRol(middleware.RolAdmin, 1), i64(42))
	if hata != "" {
		t.Fatalf("beklenmeyen hata: %s", hata)
	}
	if sahip == nil || *sahip != 42 {
		t.Fatalf("sahip = %v, beklenen 42", sahip)
	}
}

// 🔴 Bayi OLMAYAN bir hesap seçilirse reddedilmeli. Kabul edilseydi domain,
// kapsam sorgularının (BayiDomainiMi) bulamayacağı bir hesaba bağlanır ve
// panelde kimsenin göremediği bir yere düşerdi.
func TestSahipBayiCozBayiOlmayanReddedilir(t *testing.T) {
	for _, d := range []struct{ rol, durum, bekleyenHata string }{
		{"user", "active", "seçilen hesap bayi değil"},
		{"admin", "active", "seçilen hesap bayi değil"},
		{"reseller", "suspended", "seçilen bayi hesabı askıya alınmış"},
	} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT role, status FROM users WHERE id=?`)).
			WithArgs(int64(5)).
			WillReturnRows(sqlmock.NewRows([]string{"role", "status"}).AddRow(d.rol, d.durum))

		h := &Handlers{DB: db}
		sahip, hata := h.sahipBayiCoz(istekRol(middleware.RolAdmin, 1), i64(5))
		if hata != d.bekleyenHata {
			t.Errorf("rol=%s durum=%s -> hata %q, beklenen %q", d.rol, d.durum, hata, d.bekleyenHata)
		}
		if sahip != nil {
			t.Errorf("rol=%s durum=%s -> sahip nil olmalıydı, %v geldi", d.rol, d.durum, sahip)
		}
		db.Close()
	}
}

// Var olmayan kullanıcı id'si de reddedilmeli.
func TestSahipBayiCozBulunamayanReddedilir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT role, status FROM users WHERE id=?`)).
		WithArgs(int64(1234)).
		WillReturnRows(sqlmock.NewRows([]string{"role", "status"}))

	h := &Handlers{DB: db}
	sahip, hata := h.sahipBayiCoz(istekRol(middleware.RolAdmin, 1), i64(1234))
	if hata != "seçilen bayi bulunamadı" {
		t.Errorf("hata = %q, beklenen \"seçilen bayi bulunamadı\"", hata)
	}
	if sahip != nil {
		t.Errorf("sahip nil olmalıydı, %v geldi", sahip)
	}
}
