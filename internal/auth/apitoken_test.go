package auth

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAPITokenMiJWTyiSecmez(t *testing.T) {
	// Gerçek bir JWT üç noktalı bölümden oluşur; API token dalına DÜŞMEMELİ,
	// aksi halde geçerli oturumlar "geçersiz API token" ile reddedilirdi.
	jwtOrnek := "eyJhbGciOiJIUzI1NiJ9.eyJ1aWQiOjF9.imzaimza"
	if APITokenMi(jwtOrnek) {
		t.Error("JWT, API token sanıldı")
	}
	if !APITokenMi("scp_deadbeef") {
		t.Error("API token tanınmadı")
	}
	if APITokenMi("") {
		t.Error("boş değer API token sanıldı")
	}
}

func TestAPITokenHashHamDegeriSizdirmaz(t *testing.T) {
	ham := "scp_0123456789abcdef"
	h := APITokenHash(ham)
	if strings.Contains(h, ham) || strings.Contains(h, "0123456789abcdef") {
		t.Fatalf("özet ham token'ı içeriyor: %q", h)
	}
	if len(h) != 64 {
		t.Fatalf("SHA-256 hex uzunluğu 64 olmalı, %d geldi", len(h))
	}
	// Aynı girdi aynı özeti vermeli (lookup bunun üzerine kurulu).
	if APITokenHash(ham) != h {
		t.Error("özet deterministik değil")
	}
	if APITokenHash(ham+"x") == h {
		t.Error("farklı token aynı özeti verdi")
	}
}

// 🔴 Yetkilendirmenin can damarı: sorgu token'ı YALNIZ aktif, süresi dolmamış
// ve sahibi 'active' olduğunda kabul etmeli. Bu koşullardan biri sorgudan
// düşerse iptal edilmiş bir token çalışmaya devam eder.
func TestAPITokenSahibiSorgusuIptalVeSureyiKontrolEder(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ham := "scp_" + strings.Repeat("a", 64)
	mock.ExpectQuery(`SELECT t\.id, u\.id, u\.username, u\.role, u\.status, u\.auth_version`).
		WithArgs(APITokenHash(ham)).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "id", "username", "role", "status", "auth_version"}).
			AddRow(7, 42, "bayi1", "reseller", "active", 3))

	c, tokenID, err := APITokenSahibi(db, ham)
	if err != nil {
		t.Fatalf("geçerli token reddedildi: %v", err)
	}
	if tokenID != 7 || c.UserID != 42 || c.Role != "reseller" || c.Version != 3 {
		t.Fatalf("claims yanlış çözüldü: %+v (tokenID=%d)", c, tokenID)
	}

	// Sorgunun kendisi iptal/süre koşullarını taşımalı.
	sorgu := mock.ExpectationsWereMet()
	if sorgu != nil {
		t.Fatalf("beklenen sorgu çalışmadı: %v", sorgu)
	}
}

// Sorgu metninin iptal (aktif=1) ve süre (bitis_at) koşullarını kaybetmediğini
// doğrular. Bu koşullar sessizce silinirse iptal edilmiş ya da süresi dolmuş
// bir token çalışmaya devam eder — sessiz ve tehlikeli bir regresyon.
func TestAPITokenSorgusuGuvenlikKosullariniTasir(t *testing.T) {
	for _, kosul := range []string{
		"t.aktif = 1",
		"t.bitis_at IS NULL OR t.bitis_at > NOW()",
		"u.status", // sahibin durumu okunmalı (aktif değilse reddedilir)
	} {
		if !strings.Contains(apiTokenSorgusu, kosul) {
			t.Errorf("sorgu %q koşulunu taşımıyor — iptal/süresi dolmuş token kabul edilebilir", kosul)
		}
	}
}

func TestAPITokenSahibiAktifOlmayanHesabiReddeder(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ham := "scp_" + strings.Repeat("c", 64)
	mock.ExpectQuery(`SELECT t\.id, u\.id`).
		WithArgs(APITokenHash(ham)).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "id", "username", "role", "status", "auth_version"}).
			AddRow(9, 43, "askidaki", "admin", "suspended", 1))

	if _, _, err := APITokenSahibi(db, ham); err == nil {
		t.Fatal("askıya alınmış hesabın token'ı kabul edildi")
	}
}

func TestAPITokenSahibiJWTyiReddeder(t *testing.T) {
	// Ön eki olmayan bir değer DB'ye hiç gitmemeli.
	if _, _, err := APITokenSahibi(nil, "eyJhbGciOiJIUzI1NiJ9.x.y"); err == nil {
		t.Fatal("JWT, API token olarak kabul edildi")
	}
}
