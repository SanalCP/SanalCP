package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sanalcp/internal/auth"
)

// istekRol: verilen rolde admin-tipi (auth.Claims) token taşıyan istek üretir.
func istekRol(rol string, uid int64) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	c := &auth.Claims{UserID: uid, Username: "t", Role: rol}
	return ClaimsIle(r, c)
}

func TestAdminOnly(t *testing.T) {
	sar := func(r *http.Request) int {
		rec := httptest.NewRecorder()
		AdminOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rec, r)
		return rec.Code
	}

	if kod := sar(istekRol(RolAdmin, 1)); kod != http.StatusOK {
		t.Errorf("admin geçmeliydi, kod=%d", kod)
	}
	// Asıl regresyon: bayi token'ı auth.Claims taşır; rol kontrolü olmadan
	// admin muamelesi görüyordu.
	if kod := sar(istekRol(RolBayi, 2)); kod != http.StatusForbidden {
		t.Errorf("bayi 403 almalıydı, kod=%d", kod)
	}
	if kod := sar(istekRol(RolMusteri, 3)); kod != http.StatusForbidden {
		t.Errorf("user rolü 403 almalıydı, kod=%d", kod)
	}
	if kod := sar(httptest.NewRequest(http.MethodGet, "/", nil)); kod != http.StatusForbidden {
		t.Errorf("kimliksiz 403 almalıydı, kod=%d", kod)
	}
}

func TestBayiVeUstu(t *testing.T) {
	sar := func(r *http.Request) int {
		rec := httptest.NewRecorder()
		BayiVeUstu(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rec, r)
		return rec.Code
	}

	if kod := sar(istekRol(RolAdmin, 1)); kod != http.StatusOK {
		t.Errorf("admin geçmeliydi, kod=%d", kod)
	}
	if kod := sar(istekRol(RolBayi, 2)); kod != http.StatusOK {
		t.Errorf("bayi geçmeliydi, kod=%d", kod)
	}
	if kod := sar(istekRol(RolMusteri, 3)); kod != http.StatusForbidden {
		t.Errorf("user rolü 403 almalıydı, kod=%d", kod)
	}
	if kod := sar(httptest.NewRequest(http.MethodGet, "/", nil)); kod != http.StatusForbidden {
		t.Errorf("kimliksiz 403 almalıydı, kod=%d", kod)
	}
}

func TestKapsamSQL(t *testing.T) {
	// Admin: daraltma yok.
	if kosul, arg := KapsamSQL(istekRol(RolAdmin, 1), "d"); kosul != "" || arg != nil {
		t.Errorf("admin daraltılmamalı, kosul=%q arg=%v", kosul, arg)
	}

	// Bayi: sahiplik zinciri üzerinden EXISTS + kendi user id'si.
	kosul, arg := KapsamSQL(istekRol(RolBayi, 7), "d")
	if kosul == "" || len(arg) != 1 || arg[0] != int64(7) {
		t.Errorf("bayi kapsamı hatalı: kosul=%q arg=%v", kosul, arg)
	}

	// Müşteri: aynı zincir, customers.user_id üzerinden.
	kosul, arg = KapsamSQL(istekRol(RolMusteri, 42), "d")
	if !strings.Contains(kosul, "kc.user_id") || len(arg) != 1 || arg[0] != int64(42) {
		t.Errorf("müşteri kapsamı hatalı: kosul=%q arg=%v", kosul, arg)
	}
	// Bayi koşulu müşterininkiyle karışmamalı — owner_user_id ile daralmalı.
	if kosulB, _ := KapsamSQL(istekRol(RolBayi, 7), "d"); !strings.Contains(kosulB, "kc.owner_user_id") {
		t.Errorf("bayi kapsamı owner_user_id kullanmalı: %q", kosulB)
	}

	// Kimliksiz: fail-closed — hiçbir satır eşleşmemeli.
	kosul, _ = KapsamSQL(httptest.NewRequest(http.MethodGet, "/", nil), "d")
	if kosul != " WHERE 1 = 0" {
		t.Errorf("kimliksiz istek fail-closed olmalı, kosul=%q", kosul)
	}
}

func TestDomainSahibiMiFailClosed(t *testing.T) {
	// scopeDB nil (bu testte DB yok) → bayi için sahiplik doğrulanamaz,
	// erişim REDDEDİLMELİ. Fail-open olsaydı DB hatasında bayi her domaine
	// erişirdi.
	if DomainSahibiMi(istekRol(RolBayi, 2), 99) {
		t.Error("DB yokken bayi erişimi reddedilmeliydi (fail-closed)")
	}
	// Admin DB'ye bakmadan geçer.
	if !DomainSahibiMi(istekRol(RolAdmin, 1), 99) {
		t.Error("admin her domaine erişmeliydi")
	}
	// Müşteri de zincirden çözülür; DB yoksa reddedilir.
	if DomainSahibiMi(istekRol(RolMusteri, 5), 99) {
		t.Error("DB yokken müşteri erişimi reddedilmeliydi (fail-closed)")
	}
	// Tanımsız rol hiçbir domaine erişemez.
	if DomainSahibiMi(istekRol("bilinmeyen", 5), 99) {
		t.Error("bilinmeyen rol reddedilmeliydi")
	}
}

func TestMusteriScopeBayiKapsamsizGecemez(t *testing.T) {
	// scopeDB nil → BayiDomainiMi false → bayi 403 almalı.
	// Regresyon koruması: eski kod ClaimsFrom != nil olduğu için bayiyi
	// doğrudan geçiriyordu.
	rec := httptest.NewRecorder()
	MusteriScope(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, istekRol(RolBayi, 2))

	if rec.Code != http.StatusForbidden {
		t.Errorf("kapsamı doğrulanamayan bayi 403 almalıydı, kod=%d", rec.Code)
	}
}
