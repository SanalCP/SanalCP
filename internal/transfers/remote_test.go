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
