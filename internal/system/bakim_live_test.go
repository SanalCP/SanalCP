package system

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// TestBakimLiveSaltOkunur, yalnız açıkça SANALCP_LIVE_TEST=1 verilirse gerçek
// Linux sunucusundaki salt-okunur bakım uçlarını ve DNS sorgusunu sınar.
func TestBakimLiveSaltOkunur(t *testing.T) {
	if os.Getenv("SANALCP_LIVE_TEST") != "1" {
		t.Skip("canlı sistem testi kapalı")
	}
	tests := []struct {
		name string
		h    http.HandlerFunc
	}{
		{"guvenlik", GuvenlikGuncellemeDurum}, {"kimlik-dns", SunucuKimlikDNS},
		{"depolama", DepolamaDurumHandler}, {"reboot", RebootDurum},
		{"journal", JournalDurum}, {"dns", DNSCozumleyiciDurum}, {"swap", SwapDurumHandler},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			tc.h(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
			}
			if w.Body.Len() < 3 {
				t.Fatal("boş yanıt")
			}
		})
	}
	t.Run("dns-test", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"alan_adi":"cloud.sanalcp.com"}`))
		w := httptest.NewRecorder()
		DNSTest(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestBakimLiveGuvenliYazma(t *testing.T) {
	if os.Getenv("SANALCP_LIVE_WRITE_TEST") != "1" {
		t.Skip("canlı yazma testi kapalı")
	}
	call := func(name, method, body string, h http.HandlerFunc, want int) {
		t.Helper()
		r := httptest.NewRequest(method, "/", bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		h(w, r)
		if w.Code != want {
			t.Fatalf("%s: HTTP %d: %s", name, w.Code, w.Body.String())
		}
	}
	call("depolama", http.MethodPut, `{"disk_esik":80,"inode_esik":85}`, DepolamaAyarKaydet, 200)
	call("swappiness", http.MethodPut, `{"swappiness":60,"olustur_mb":0}`, SwapAyarKaydet, 200)
	call("guvenlik-kapali", http.MethodPut, `{"aktif":false,"otomatik_reboot":false}`, GuvenlikGuncellemeKaydet, 200)
	call("journal-limit", http.MethodPut, `{"maksimum_mb":500}`, JournalAyarKaydet, 200)
	call("journal-vacuum", http.MethodPost, `{"koru_mb":500}`, JournalTemizle, 200)
	call("dns-gecersiz", http.MethodPut, `{"sunucular":["8.8.8.8;id"]}`, DNSCozumleyiciKaydet, 400)
	call("swap-gecersiz", http.MethodPut, `{"swappiness":60,"olustur_mb":128}`, SwapAyarKaydet, 400)
	call("journal-gecersiz", http.MethodPut, `{"maksimum_mb":20}`, JournalAyarKaydet, 400)
}

func TestBakimLiveMevcutDNSYenidenUygula(t *testing.T) {
	if os.Getenv("SANALCP_LIVE_DNS_WRITE_TEST") != "1" {
		t.Skip("canlı DNS yazma testi kapalı")
	}
	d, err := resolvConfOku()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"sunucular": d.Sunucular})
	r := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(b))
	w := httptest.NewRecorder()
	DNSCozumleyiciKaydet(w, r)
	if w.Code != 200 {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
}
