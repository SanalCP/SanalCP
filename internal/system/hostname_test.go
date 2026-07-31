package system

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostnameDogrula(t *testing.T) {
	gecerli := map[string]string{
		"panel.example.com":     "panel.example.com",
		" WEB-01.Example.COM. ": "web-01.example.com",
		"sunucu-1":              "sunucu-1",
	}
	for giris, beklenen := range gecerli {
		got, err := hostnameDogrula(giris)
		if err != nil || got != beklenen {
			t.Fatalf("%q: got=%q err=%v", giris, got, err)
		}
	}
	for _, giris := range []string{"", "localhost", "127.0.0.1", "-sunucu", "sunucu-", "a..b", "sunucu_1", strings.Repeat("a", 64)} {
		if _, err := hostnameDogrula(giris); err == nil {
			t.Fatalf("%q geçersiz kabul edilmeliydi", giris)
		}
	}
}

func TestHostnameKaydet(t *testing.T) {
	tmp := t.TempDir()
	eskiHostname, eskiCloud, eskiNM, eskiAyarla := hostnameDosyasi, cloudInitDosyasi, nmDosyasi, hostnameAyarla
	t.Cleanup(func() {
		hostnameDosyasi, cloudInitDosyasi, nmDosyasi, hostnameAyarla = eskiHostname, eskiCloud, eskiNM, eskiAyarla
	})
	hostnameDosyasi = filepath.Join(tmp, "etc/hostname")
	cloudInitDosyasi = filepath.Join(tmp, "etc/cloud/cloud.cfg.d/99-sanalcp-hostname.cfg")
	nmDosyasi = filepath.Join(tmp, "etc/NetworkManager/conf.d/99-sanalcp-hostname.conf")
	var uygulanan string
	hostnameAyarla = func(_ context.Context, ad string) error { uygulanan = ad; return nil }

	req := httptest.NewRequest(http.MethodPut, "/system/hostname", strings.NewReader(`{"hostname":"Panel.Example.COM"}`))
	rec := httptest.NewRecorder()
	HostnameKaydet(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if uygulanan != "panel.example.com" {
		t.Fatalf("uygulanan=%q", uygulanan)
	}
	for yol, parca := range map[string]string{
		hostnameDosyasi:  "panel.example.com\n",
		cloudInitDosyasi: "preserve_hostname: true",
		nmDosyasi:        "hostname-mode=none",
	} {
		b, err := os.ReadFile(yol)
		if err != nil || !strings.Contains(string(b), parca) {
			t.Fatalf("%s: içerik=%q err=%v", yol, b, err)
		}
	}
}

func TestHostnameKaydetGecersiziReddeder(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/system/hostname", strings.NewReader(`{"hostname":"bad_name"}`))
	rec := httptest.NewRecorder()
	HostnameKaydet(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
