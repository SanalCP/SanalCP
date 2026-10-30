package monitor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbe_SSRFEngelli(t *testing.T) {
	// Dinleyen bir yerel sunucu: bağlantı kurulur, ardından iç IP olduğu için kesilmeli.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	h := Probe(strings.TrimPrefix(srv.URL, "http://"))
	if h.Erisilebilir || !strings.Contains(h.Hata, "engellendi") {
		t.Fatalf("loopback probu engellenmedi: %+v", h)
	}
}
