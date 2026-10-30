package uyari

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"sanalcp/internal/middleware"
)

func yeniHandlers(t *testing.T) (*Handlers, sqlmock.Sqlmock) {
	t.Helper()
	k := yeniTur(t)
	return &Handlers{DB: k.db, Gonderici: k.g}, k.mock
}

func TestAyarTokenDondurmez(t *testing.T) {
	h, mock := yeniHandlers(t)
	(&turKurulum{mock: mock}).ayar(t, 1)
	w := httptest.NewRecorder()
	h.Ayar(w, httptest.NewRequest(http.MethodGet, "/system/uyari-bildirim", nil))
	govde := w.Body.String()
	if w.Code != 200 || strings.Contains(govde, "TOK") || !strings.Contains(govde, `"telegram_token_ayarli":true`) {
		t.Fatalf("code=%d govde=%s", w.Code, govde)
	}
}

func TestKaydetBosTokenKorur(t *testing.T) {
	h, mock := yeniHandlers(t)
	mock.ExpectExec(`UPDATE panel_ayarlari SET bildirim_acik=\?, telegram_chat_id=\?, bildirim_eposta=\? WHERE id=1`).
		WithArgs(true, "42", "a@b.com").WillReturnResult(sqlmock.NewResult(0, 1))
	w := httptest.NewRecorder()
	h.AyarKaydet(w, httptest.NewRequest(http.MethodPut, "/system/uyari-bildirim",
		strings.NewReader(`{"acik":true,"telegram_token":"","telegram_chat_id":"42","eposta":"a@b.com"}`)))
	if w.Code != 200 {
		t.Fatalf("code=%d %s", w.Code, w.Body)
	}
}

func TestKaydetGecersizEposta(t *testing.T) {
	h, _ := yeniHandlers(t)
	w := httptest.NewRecorder()
	h.AyarKaydet(w, httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"eposta":"x@"}`)))
	if w.Code != 400 {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestKaydetGecersizChatID(t *testing.T) {
	h, _ := yeniHandlers(t)
	w := httptest.NewRecorder()
	h.AyarKaydet(w, httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"telegram_chat_id":"abc"}`)))
	if w.Code != 400 {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestKaydetGecersizToken(t *testing.T) {
	h, _ := yeniHandlers(t)
	w := httptest.NewRecorder()
	h.AyarKaydet(w, httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"telegram_token":"kisa"}`)))
	if w.Code != 400 {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestTestKanalYok(t *testing.T) {
	h, mock := yeniHandlers(t)
	mock.ExpectQuery("FROM panel_ayarlari").WillReturnRows(sqlmock.NewRows(
		[]string{"bildirim_acik", "telegram_token_enc", "telegram_chat_id", "bildirim_eposta", "varsayilan_dil"}).
		AddRow(1, "", "", "", "tr"))
	w := httptest.NewRecorder()
	h.Test(w, httptest.NewRequest(http.MethodPost, "/system/uyari-bildirim/test", nil))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "bildirim kanalı ayarlı değil") {
		t.Fatalf("code=%d %s", w.Code, w.Body)
	}
}

func TestAyarDemoModundaKisiselBilgiGizler(t *testing.T) {
	h, mock := yeniHandlers(t)
	mock.ExpectQuery("FROM panel_ayarlari").WillReturnRows(sqlmock.NewRows(
		[]string{"bildirim_acik", "telegram_token_enc", "telegram_chat_id", "bildirim_eposta", "varsayilan_dil"}).
		AddRow(1, "", "424242", "admin@ornek.com", "tr"))
	w := httptest.NewRecorder()
	h.Ayar(w, middleware.DemoIle(httptest.NewRequest(http.MethodGet, "/system/uyari-bildirim", nil), true))
	if govde := w.Body.String(); strings.Contains(govde, "424242") || strings.Contains(govde, "admin@ornek.com") {
		t.Fatalf("demo modunda kişisel bilgi döndü: %s", govde)
	}
}
