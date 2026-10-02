package eklenti

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"sanalcp/internal/auth"
	"sanalcp/internal/middleware"
)

type gorulen struct {
	Yol, Cerez, Yetki, Uid, Rol string
}

func sahteEklenti(t *testing.T) string {
	t.Helper()
	soket := filepath.Join(t.TempDir(), "e.sock")
	ln, err := net.Listen("unix", soket)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(gorulen{
			Yol: r.URL.Path, Cerez: r.Header.Get("Cookie"), Yetki: r.Header.Get("Authorization"),
			Uid: r.Header.Get("X-Sanal-Uid"), Rol: r.Header.Get("X-Sanal-Rol"),
		})
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return soket
}

func TestEklentiProxyKimlikVeCerez(t *testing.T) {
	soket := sahteEklenti(t)
	r := httptest.NewRequest("GET", "/api/v1/eklenti/ai/sohbetler", nil)
	r.Header.Set("Cookie", auth.OturumCerezAdi+"=jwt-degeri; eklenti_tercih=koyu")
	r.Header.Set("Authorization", "Bearer scp_gizli")
	// İstemci kimlik başlıklarını taklit etmeye ve Connection ile düşürmeye çalışır.
	r.Header.Set("X-Sanal-Rol", "admin")
	r.Header.Set("Connection", "X-Sanal-Rol, X-Sanal-Uid")
	r = middleware.ClaimsIle(r, &auth.Claims{UserID: 42, Username: "musteri1", Role: "user"})

	rec := httptest.NewRecorder()
	eklentiProxy("ai", soket).ServeHTTP(rec, r)
	var g gorulen
	if err := json.Unmarshal(rec.Body.Bytes(), &g); err != nil {
		t.Fatalf("yanıt çözülemedi (%d): %s", rec.Code, rec.Body.String())
	}
	if g.Yol != "/sohbetler" {
		t.Errorf("yol %q, beklenen /sohbetler", g.Yol)
	}
	if g.Uid != "42" || g.Rol != "user" {
		t.Errorf("kimlik başlıkları yanlış: uid=%q rol=%q", g.Uid, g.Rol)
	}
	if g.Yetki != "" {
		t.Errorf("Authorization eklentiye sızdı: %q", g.Yetki)
	}
	if g.Cerez != "eklenti_tercih=koyu" {
		t.Errorf("çerez başlığı %q, beklenen yalnız eklenti çerezi", g.Cerez)
	}
}

func TestSoketTransportYenidenKullanilir(t *testing.T) {
	a, b := soketTransport("/tmp/x.sock"), soketTransport("/tmp/x.sock")
	if a != b {
		t.Fatal("aynı soket için her çağrıda yeni Transport oluşturuluyor")
	}
	if a.IdleConnTimeout == 0 {
		t.Fatal("IdleConnTimeout tanımsız — boşta bağlantılar hiç kapanmaz")
	}
}
