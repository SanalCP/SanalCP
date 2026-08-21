package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWriteExecErrorKirpma: ExecOutMax'ı aşan çıktı kırpılır + toplam bayt
// sayısı sonda görünür (operatör neyin kesildiğini bilir).
func TestWriteExecErrorKirpma(t *testing.T) {
	uzun := strings.Repeat("a", ExecOutMax+500)
	w := httptest.NewRecorder()
	WriteExecError(w, http.StatusInternalServerError, "işlem", []byte(uzun))

	var b ErrorBody
	if err := json.NewDecoder(w.Result().Body).Decode(&b); err != nil {
		t.Fatalf("yanıt parse edilemedi: %v", err)
	}
	if !strings.Contains(b.Hata, "kırpıldı") {
		t.Errorf("kırpma işareti yok: %q", b.Hata)
	}
	if !strings.Contains(b.Hata, "toplam") {
		t.Errorf("toplam bayt bilgisi yok: %q", b.Hata)
	}
	// Mesaj + kırpılmış gövde birleşik bayttan küçük olmalı (marker hariç)
	if len(b.Hata) >= len(uzun) {
		t.Errorf("kırpılmadı: %d >= %d", len(b.Hata), len(uzun))
	}
}

// TestWriteExecErrorKucuk: sınırın altındaki çıktı aynen geçer.
func TestWriteExecErrorKucuk(t *testing.T) {
	w := httptest.NewRecorder()
	WriteExecError(w, http.StatusInternalServerError, "işlem", []byte("kısa hata"))

	var b ErrorBody
	_ = json.NewDecoder(w.Result().Body).Decode(&b)
	if b.Hata != "işlem: kısa hata" {
		t.Errorf("b.Hata = %q, beklenen \"işlem: kısa hata\"", b.Hata)
	}
}

// TestWriteExecErrorBos: çıktı boşsa yalnız msg döner (sonda iki nokta olmamalı).
func TestWriteExecErrorBos(t *testing.T) {
	w := httptest.NewRecorder()
	WriteExecError(w, http.StatusInternalServerError, "başarısız", []byte("   \n  "))

	var b ErrorBody
	_ = json.NewDecoder(w.Result().Body).Decode(&b)
	if b.Hata != "başarısız" {
		t.Errorf("b.Hata = %q, beklenen \"başarısız\" (sonda \":\" olmamalı)", b.Hata)
	}
}

// TestWriteExecErrorContentType: JSON Content-Type set edilmeli — XSS
// olmamasının güvencesi (tarayıcı application/json gövdelerini HTML olarak
// yorumlamaz).
func TestWriteExecErrorContentType(t *testing.T) {
	w := httptest.NewRecorder()
	WriteExecError(w, http.StatusInternalServerError, "x", []byte("<script>alert(1)</script>"))
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, application/json bekleniyordu", ct)
	}
}