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
	tok, err := Issue(secret, 3600, 1, "root", "admin", 0)
	if err != nil {
		t.Fatal(err)
	}

	govde := strings.NewReader(`{"mevcut":"dogru-parola","yeni":"yeni-parola-12"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/parola", govde)
	req.Header.Set("Authorization", "Bearer "+tok)
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
