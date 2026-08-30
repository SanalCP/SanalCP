package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Kapı, parola DOĞRU olsa bile kapatmalı. Doğrulayıcı burada bilerek
// "her parola doğru" diye stub'lanıyor: test /etc/shadow'a bağımlı olmamalı,
// ve asıl iddia "yanlış parola reddedildi" değil, "bayrak kapalıyken DOĞRU
// parola bile reddedildi".
func TestLogin_RootGirisiKapaliykenDogruParolaBileReddedilir(t *testing.T) {
	eski := rootParolaDogrulaFn
	rootParolaDogrulaFn = func(string) bool { return true }
	defer func() { rootParolaDogrulaFn = eski }()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(0))
	// Reddedilen deneme audit_log'a YAZILMALI — kapalı bayrak, kaba-kuvvet
	// denemelerini görünmez yapmamalı.
	mock.ExpectExec(`INSERT INTO audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	h := &Handlers{DB: db, Secret: []byte("test"), LifetimeSec: 3600}
	govde := strings.NewReader(`{"kullanici":"root","parola":"dogru-parola"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", govde)
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("beklenen 401, gelen %d", w.Code)
	}
	// Mesaj hatalı-parola yanıtıyla BİREBİR aynı olmalı: root girişinin
	// kapalı olduğu sızarsa saldırgan hangi sunucuda root yolunun açık
	// olduğunu tarayarak bulabilir.
	var yanit map[string]any
	if err := json.NewDecoder(w.Body).Decode(&yanit); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	mesaj, _ := yanit["hata"].(string)
	if mesaj != "kullanıcı adı veya parola hatalı" {
		t.Fatalf("yanıt root girişinin kapalı olduğunu sızdırıyor: %q", mesaj)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}

// Bayrak açıkken doğrulayıcı ÇAĞRILMALI — kapı eski davranışı bozmamalı.
func TestLogin_RootGirisiAcikkenDogrulayiciCagrilir(t *testing.T) {
	cagrildi := false
	eski := rootParolaDogrulaFn
	rootParolaDogrulaFn = func(string) bool { cagrildi = true; return false }
	defer func() { rootParolaDogrulaFn = eski }()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	h := &Handlers{DB: db, Secret: []byte("test"), LifetimeSec: 3600}
	govde := strings.NewReader(`{"kullanici":"root","parola":"yanlis"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", govde)
	w := httptest.NewRecorder()

	h.Login(w, req)

	if !cagrildi {
		t.Fatal("bayrak açıkken parola doğrulayıcı hiç çağrılmadı")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("beklenen 401, gelen %d", w.Code)
	}
}

// Fix round 1: /me/parola da aynı kapıya tabi olmalı. Eskiden var olan bir
// root JWT'si, bayrak kapatıldıktan SONRA bile bu uçtan sistem root
// parolasını değiştirebiliyordu (chpasswd) — doğrulayıcı burada bilerek
// "her parola doğru" diye stub'lanıyor ki iddia "mevcut parola yanlıştı"
// değil, "bayrak kapalıyken shadow'a hiç dokunulmadı" olsun.
func TestParolaDegistir_RootGirisiKapaliykenShadowaDokunulmaz(t *testing.T) {
	eski := rootParolaDogrulaFn
	rootParolaDogrulaFn = func(string) bool { return true }
	defer func() { rootParolaDogrulaFn = eski }()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(0))
	// Reddedilen deneme burada da audit_log'a YAZILMALI.
	mock.ExpectExec(`INSERT INTO audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	secret := []byte("test")
	h := &Handlers{DB: db, Secret: secret, LifetimeSec: 3600}
	// Kimlik üretimdeki gibi context'ten gelir (middleware.RequireAuth koyar).
	govde := strings.NewReader(`{"mevcut":"dogru-parola","yeni":"yeni-parola-12"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/parola", govde)
	req = req.WithContext(ClaimsContext(req.Context(),
		&Claims{UserID: 1, Username: "root", Role: "admin"}))
	w := httptest.NewRecorder()

	h.ParolaDegistir(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("beklenen 403, gelen %d", w.Code)
	}
	var yanit map[string]any
	if err := json.NewDecoder(w.Body).Decode(&yanit); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	mesaj, _ := yanit["hata"].(string)
	beklenen := "panel root girişi kapalı; sunucu root parolası SSH üzerinden 'passwd' ile değiştirilir"
	if mesaj != beklenen {
		t.Fatalf("yanıt mesajı beklenmedik: %q (beklenen %q)", mesaj, beklenen)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}

// BAŞARI YOLU: bayrak AÇIK + parola doğru => 200 ve gerçek bir token.
//
// Global kısıt "mevcut kurulumlar etkilenmez"in koruduğu tek dal budur:
// migration bayrağı 1 ile eklediği için root ile giren operatörün girişi
// aynen çalışmaya devam etmeli. Diğer testler kapıyı reddetme yönünde
// kanıtlıyor; bu, kabul yönünü kanıtlar.
func TestLogin_RootGirisiAcikkenDogruParolaTokenUretir(t *testing.T) {
	// Giriş sonunda tetiklenen sürüm kontrolü ağ çağrısı yapmasın.
	t.Setenv("PANEL_SURUM_KONTROL", "0")

	eski := rootParolaDogrulaFn
	rootParolaDogrulaFn = func(string) bool { return true }
	defer func() { rootParolaDogrulaFn = eski }()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).
		WillReturnRows(sqlmock.NewRows([]string{"root_girisi_acik"}).AddRow(1))
	mock.ExpectQuery(`SELECT full_name FROM users WHERE id=1`).
		WillReturnRows(sqlmock.NewRows([]string{"full_name"}).AddRow("Sistem Yöneticisi"))
	// 2FA kapalı; auth_version=7 => token'a bu sürüm gömülmeli.
	mock.ExpectQuery(`SELECT totp_enabled, totp_secret, totp_last_step, auth_version FROM users WHERE id=\?`).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"totp_enabled", "totp_secret", "totp_last_step", "auth_version"}).
			AddRow(0, "", 0, 7))
	mock.ExpectExec(`INSERT INTO audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// Başarılı giriş, eski oturumun boşta kalma zamanını yeni oturuma
	// taşımamalı; aksi hâlde ilk yetkili istek anında 401 döner.
	mock.ExpectExec(`UPDATE users SET last_login_at=NOW\(\), last_login_ip=\?, last_activity_at=NOW\(\) WHERE id=\?`).
		WithArgs("192.0.2.1", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	secret := []byte("test")
	h := &Handlers{DB: db, Secret: secret, LifetimeSec: 3600}
	govde := strings.NewReader(`{"kullanici":"root","parola":"dogru-parola"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", govde)
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("beklenen 200, gelen %d (gövde: %s)", w.Code, w.Body.String())
	}
	var yanit struct {
		Token     string `json:"token"`
		Kullanici struct {
			ID  int64  `json:"id"`
			Adi string `json:"adi"`
			Rol string `json:"rol"`
		} `json:"kullanici"`
	}
	if err := json.NewDecoder(w.Body).Decode(&yanit); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	// Token yanıt GÖVDESİNDE dönmemeli — oturum yalnız HttpOnly çerezde taşınır.
	if yanit.Token != "" {
		t.Fatal("yanıt gövdesinde token var; oturum yalnız HttpOnly çerezde dönmeli")
	}
	if yanit.Kullanici.ID != 1 || yanit.Kullanici.Adi != "root" || yanit.Kullanici.Rol != "admin" {
		t.Fatalf("beklenen 1/root/admin, gelen %d/%s/%s",
			yanit.Kullanici.ID, yanit.Kullanici.Adi, yanit.Kullanici.Rol)
	}
	ham := oturumCerezindenToken(t, w)
	c, err := Parse(secret, ham)
	if err != nil {
		t.Fatalf("üretilen token çözülemedi: %v", err)
	}
	if c.UserID != 1 || c.Username != "root" || c.Role != "admin" {
		t.Fatalf("token içeriği beklenmedik: %d/%s/%s", c.UserID, c.Username, c.Role)
	}
	if c.Version != 7 {
		t.Fatalf("token'daki auth_version %d, beklenen 7", c.Version)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("beklenen DB çağrıları eksik: %v", err)
	}
}
