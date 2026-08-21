package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"sanalcp/internal/auth"
)

// gecerliOturumBekle: RequireAuth'un tek doğrulama sorgusunu ve ardından gelen
// aktivite güncellemesini başarıyla yanıtlayacak şekilde mock kurar.
func gecerliOturumBekle(t *testing.T, uid int64, rol string, surum uint64) sqlmock.Sqlmock {
	t.Helper()
	mock := mockDB(t)
	mock.ExpectQuery(requireAuthSorgu.String()).WithArgs(uid).
		WillReturnRows(sqlmock.NewRows(
			[]string{"status", "role", "auth_version", "bosta_saniye", "oturum_bosta_dakika"}).
			AddRow("active", rol, surum, nil, 0))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE users SET last_activity_at = NOW()")).
		WithArgs(uid).WillReturnResult(sqlmock.NewResult(0, 1))
	return mock
}

func cerezliRequireAuth(t *testing.T, secret []byte, kur func(*http.Request)) (int, *auth.Claims) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	kur(req)
	rec := httptest.NewRecorder()
	var gorulen *auth.Claims
	RequireAuth(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gorulen = ClaimsFrom(r)
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)
	return rec.Code, gorulen
}

// TestRequireAuthOturumCereziniKabulEder: oturumun asıl taşıyıcısı artık
// HttpOnly çerezdir; Authorization başlığı olmadan da kimlik çözülmeli.
func TestRequireAuthOturumCereziniKabulEder(t *testing.T) {
	secret := []byte("test-secret-0123456789-0123456789")
	gecerliOturumBekle(t, 42, RolBayi, 7)

	token, err := auth.Issue(secret, 3600, 42, "bayi", RolBayi, 7)
	if err != nil {
		t.Fatal(err)
	}
	kod, c := cerezliRequireAuth(t, secret, func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: auth.OturumCerezAdi, Value: token})
	})
	if kod != http.StatusOK {
		t.Fatalf("çerezli istek kod=%d, 200 bekleniyordu", kod)
	}
	if c == nil || c.UserID != 42 || c.Role != RolBayi {
		t.Fatalf("claim'ler çerezden çözülemedi: %+v", c)
	}
}

// TestRequireAuthCerezYokBaslikVar: eski akış (Authorization: Bearer <JWT>)
// çalışmaya devam etmeli — geçiş sırasında ve başlıkla kimlik taşıyan
// otomasyon için gerekli.
func TestRequireAuthCerezYokBaslikVar(t *testing.T) {
	secret := []byte("test-secret-0123456789-0123456789")
	gecerliOturumBekle(t, 42, RolBayi, 7)

	token, err := auth.Issue(secret, 3600, 42, "bayi", RolBayi, 7)
	if err != nil {
		t.Fatal(err)
	}
	kod, c := cerezliRequireAuth(t, secret, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	})
	if kod != http.StatusOK {
		t.Fatalf("başlıklı istek kod=%d, 200 bekleniyordu", kod)
	}
	if c == nil || c.UserID != 42 {
		t.Fatalf("claim'ler başlıktan çözülemedi: %+v", c)
	}
}

// TestRequireAuthCerezBaslikOncelikli: ikisi de varsa kimliği ÇEREZ belirler.
//
// Aksi hâlde bir saldırgan sayfa, kurbanın tarayıcısından giden isteğe kendi
// API token'ını Authorization başlığıyla ekleyip kurbanın oturumunun üstüne
// yazabilirdi. Burada çerez uid=42, başlık uid=99 taşır; handler'ın 42'yi
// görmesi gerekir ve başlık için HİÇ sorgu yapılmamalıdır.
func TestRequireAuthCerezBaslikOncelikli(t *testing.T) {
	secret := []byte("test-secret-0123456789-0123456789")
	mock := gecerliOturumBekle(t, 42, RolBayi, 7)

	cerezTok, err := auth.Issue(secret, 3600, 42, "cerez-sahibi", RolBayi, 7)
	if err != nil {
		t.Fatal(err)
	}
	baslikTok, err := auth.Issue(secret, 3600, 99, "baslik-sahibi", RolAdmin, 1)
	if err != nil {
		t.Fatal(err)
	}
	kod, c := cerezliRequireAuth(t, secret, func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: auth.OturumCerezAdi, Value: cerezTok})
		r.Header.Set("Authorization", "Bearer "+baslikTok)
	})
	if kod != http.StatusOK {
		t.Fatalf("kod=%d, 200 bekleniyordu", kod)
	}
	if c == nil || c.UserID != 42 {
		t.Fatalf("kimlik başlıktan çözülmüş: %+v — çerez öncelikli olmalı", c)
	}
	if c.Role != RolBayi {
		t.Fatalf("rol=%q, başlıktaki admin rolü sızmış olabilir", c.Role)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRequireAuthCerezdekiAPITokenReddedilir: API token'ı (scp_…) yalnız
// Authorization başlığından kabul edilir. Çerezde kabul etmek, tarayıcı çerezi
// kendiliğinden gönderdiği için API token'larını CSRF'e açardı.
//
// Çerez bir API token'ı taşıdığında akış JWT çözümlemesine düşer ve orada
// başarısız olur; DB'ye hiç gidilmez.
func TestRequireAuthCerezdekiAPITokenReddedilir(t *testing.T) {
	secret := []byte("test-secret-0123456789-0123456789")
	mock := mockDB(t)

	kod, c := cerezliRequireAuth(t, secret, func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: auth.OturumCerezAdi, Value: auth.APITokenOnEk + "sahte-token-degeri"})
	})
	if kod != http.StatusUnauthorized {
		t.Fatalf("çerezdeki API token'ı kod=%d, 401 bekleniyordu", kod)
	}
	if c != nil {
		t.Fatalf("reddedilmesi gereken istekte claim üretilmiş: %+v", c)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRequireAuthAsiriUzunCerezReddedilir: CVE-2025-30204 savunması çerez
// yolunda da geçerli olmalı — uzunluk kontrolü doğrulamadan ÖNCE yapılır.
func TestRequireAuthAsiriUzunCerezReddedilir(t *testing.T) {
	secret := []byte("test-secret-0123456789-0123456789")
	mock := mockDB(t)

	uzun := make([]byte, 8193)
	for i := range uzun {
		uzun[i] = 'a'
	}
	kod, _ := cerezliRequireAuth(t, secret, func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: auth.OturumCerezAdi, Value: string(uzun)})
	})
	if kod != http.StatusUnauthorized {
		t.Fatalf("aşırı uzun çerez kod=%d, 401 bekleniyordu", kod)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRequireAuthBosCerezBasligaDuser: değeri boş bir oturum çerezi, oturum
// yokmuş gibi ele alınmalı ve Authorization başlığına düşülmeli. Aksi hâlde
// tarayıcıda kalmış boş bir çerez, API token'lı otomasyonu kilitlerdi.
func TestRequireAuthBosCerezBasligaDuser(t *testing.T) {
	secret := []byte("test-secret-0123456789-0123456789")
	gecerliOturumBekle(t, 42, RolBayi, 7)

	token, err := auth.Issue(secret, 3600, 42, "bayi", RolBayi, 7)
	if err != nil {
		t.Fatal(err)
	}
	kod, c := cerezliRequireAuth(t, secret, func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: auth.OturumCerezAdi, Value: ""})
		r.Header.Set("Authorization", "Bearer "+token)
	})
	if kod != http.StatusOK {
		t.Fatalf("boş çerez + geçerli başlık kod=%d, 200 bekleniyordu", kod)
	}
	if c == nil || c.UserID != 42 {
		t.Fatalf("claim'ler çözülemedi: %+v", c)
	}
}

// TestRequireAuthKimligiAuthPaketineTasiyor: bu değişikliğin kaçırdığı hatayı
// yakalayan regresyon testi.
//
// internal/auth altındaki yedi uç (profile.go, dashboard.go) kimliği
// auth.ClaimsFrom ile okur. Middleware kendi özel context anahtarını
// kullandığı sürece o okuma daima nil dönüyordu ve handler'lar kimliği
// Authorization başlığından yeniden ayrıştırmak zorundaydı. Oturum çereze
// taşınınca başlık ortadan kalktı, yedi uç birden 401 "oturum yok" döndü;
// biri (GET /dashboard-duzen) anasayfa açılışında çağrıldığı için istemcinin
// 401 yakalayıcısı kullanıcıyı giriş yapar yapmaz çıkartıyordu.
//
// Hiçbir test RequireAuth'u gerçek bir handler'a zincirlemediği için kimse
// görmedi. Bu test tam o zinciri kurar.
func TestRequireAuthKimligiAuthPaketineTasiyor(t *testing.T) {
	secret := []byte("test-secret-0123456789-0123456789")
	gecerliOturumBekle(t, 42, RolBayi, 7)

	token, err := auth.Issue(secret, 3600, 42, "bayi", RolBayi, 7)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard-duzen", nil)
	req.AddCookie(&http.Cookie{Name: auth.OturumCerezAdi, Value: token})
	rec := httptest.NewRecorder()

	var handlerGordu *auth.Claims
	RequireAuth(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// auth paketindeki handler'ların kullandığı erişimcinin TA KENDİSİ.
		handlerGordu = auth.ClaimsFrom(r)
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("kod=%d, 200 bekleniyordu", rec.Code)
	}
	if handlerGordu == nil {
		t.Fatal("auth.ClaimsFrom nil döndü — auth paketindeki uçlar 401 \"oturum yok\" verir")
	}
	if handlerGordu.UserID != 42 || handlerGordu.Role != RolBayi {
		t.Fatalf("handler yanlış kimlik gördü: %+v", handlerGordu)
	}
	// middleware.ClaimsFrom ile auth.ClaimsFrom aynı değeri görmeli; ikisi
	// ayrışırsa yetki katmanı ile handler'lar farklı kullanıcı üzerinde
	// çalışıyor demektir.
	if mw := ClaimsFrom(req.WithContext(
		auth.ClaimsContext(req.Context(), handlerGordu))); mw == nil || mw.UserID != handlerGordu.UserID {
		t.Fatalf("middleware.ClaimsFrom ile auth.ClaimsFrom ayrışıyor: %+v vs %+v", mw, handlerGordu)
	}
}

// TestRequireAuthAPITokenKimligiDeTasiniyor: `scp_…` API token'ıyla gelen
// istekte de kimlik auth paketine ulaşmalı.
//
// Bu, çerezden ÖNCE de bozuktu: RequireAuth kimliği veritabanından çözüp
// context'e koyuyordu, ama auth handler'ları Authorization başlığındaki
// `scp_…` değerini JWT sanıp ayrıştırmaya çalışıyor ve nil alıyordu. Yani API
// token'ları /me, 2FA ve dashboard uçlarına hiçbir zaman erişemiyordu.
func TestRequireAuthAPITokenKimligiDeTasiniyor(t *testing.T) {
	secret := []byte("test-secret-0123456789-0123456789")
	// API token yolu DB'den çözülür; burada yalnız context aktarımını
	// sınadığımız için claim'i doğrudan yerleştirip erişimciyi doğruluyoruz.
	c := &auth.Claims{UserID: 7, Username: "otomasyon", Role: RolBayi}
	req := ClaimsIle(httptest.NewRequest(http.MethodGet, "/api/v1/me", nil), c)

	got := auth.ClaimsFrom(req)
	if got == nil || got.UserID != 7 {
		t.Fatalf("auth.ClaimsFrom API token kimliğini görmüyor: %+v", got)
	}
	_ = secret
}
