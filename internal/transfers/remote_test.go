package transfers

import (
	"bytes"
	"testing"
)

func TestSinirliKesifCiktisiSiniriAsincaKeser(t *testing.T) {
	w := &sinirliKesifCiktisi{max: maxKesifCiktisi}
	n, err := w.Write(bytes.Repeat([]byte{'x'}, maxKesifCiktisi+1))
	if err == nil {
		t.Fatal("keşif çıktısı sınırı aşınca hata dönmedi")
	}
	if n != maxKesifCiktisi || w.buf.Len() != maxKesifCiktisi {
		t.Fatalf("çıktı %d byte yazdı, buffer %d byte; beklenen %d", n, w.buf.Len(), maxKesifCiktisi)
	}
}

func TestUzakHostDogrulama(t *testing.T) {
	for _, s := range []string{"203.0.113.5", "server.example.com", "2001:db8::1"} {
		if !uzakHostGecerli(s) {
			t.Errorf("geçerli reddedildi: %s", s)
		}
	}
	for _, s := range []string{"-oProxyCommand=x", "host name", "a..b", ""} {
		if uzakHostGecerli(s) {
			t.Errorf("geçersiz kabul edildi: %s", s)
		}
	}
}
func TestDomainKesifGecerli(t *testing.T) {
	if !domainKesifGecerli("example.com") {
		t.Fatal("domain reddedildi")
	}
	if domainKesifGecerli("-x.example") || domainKesifGecerli("localhost") {
		t.Fatal("geçersiz domain kabul edildi")
	}
}

func TestOnKontrolEksikPHPModulleriniBildirir(t *testing.T) {
	s := UzakSite{Domain: "example.com", PHPSurum: "8.2", KaynakModuller: []string{"curl", "gd"}, Uyarilar: []string{"requires:curl", "requires:gd"}}
	onKontrol(&s, map[string]phpHedef{"8.2": {yuklu: true, moduller: map[string]bool{"curl": true}}})
	if s.Tasinabilir || len(s.EksikModuller) != 1 || s.EksikModuller[0] != "gd" {
		t.Fatalf("beklenmeyen ön kontrol: %+v", s)
	}
}

func TestOnKontrolUyumluSiteyiTasınabilirYapar(t *testing.T) {
	s := UzakSite{Domain: "example.com", PHPSurum: "8.3", KaynakModuller: []string{"curl"}, Uyarilar: []string{"requires:curl"}}
	onKontrol(&s, map[string]phpHedef{"8.3": {yuklu: true, moduller: map[string]bool{"curl": true}}})
	if !s.Tasinabilir || s.KontrolDurumu != "compatible" || len(s.Engeller) != 0 {
		t.Fatalf("beklenmeyen ön kontrol: %+v", s)
	}
}
