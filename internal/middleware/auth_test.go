package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"sanalpanel/internal/auth"
)

// istekRol: verilen rolde admin-tipi (auth.Claims) token taşıyan istek üretir.
func istekRol(rol string, uid int64) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	c := &auth.Claims{UserID: uid, Username: "t", Role: rol}
	return r.WithContext(context.WithValue(r.Context(), claimsKey, c))
}

// istekMusteri: müşteri (auth.MusteriClaims) token'ı taşıyan istek üretir.
func istekMusteri(domainID int64) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	mc := &auth.MusteriClaims{DomainID: domainID}
	return r.WithContext(context.WithValue(r.Context(), musteriClaimsKey, mc))
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
	if kod := sar(istekMusteri(5)); kod != http.StatusForbidden {
		t.Errorf("müşteri token'ı 403 almalıydı, kod=%d", kod)
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
	if kod := sar(istekMusteri(5)); kod != http.StatusForbidden {
		t.Errorf("müşteri token'ı 403 almalıydı, kod=%d", kod)
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

	// Müşteri: yalnız kendi domaini.
	kosul, arg = KapsamSQL(istekMusteri(42), "d")
	if kosul == "" || len(arg) != 1 || arg[0] != int64(42) {
		t.Errorf("müşteri kapsamı hatalı: kosul=%q arg=%v", kosul, arg)
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
	// Müşteri yalnız kendi domainine.
	if !DomainSahibiMi(istekMusteri(99), 99) {
		t.Error("müşteri kendi domainine erişmeliydi")
	}
	if DomainSahibiMi(istekMusteri(98), 99) {
		t.Error("müşteri başka domaine erişmemeliydi")
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
