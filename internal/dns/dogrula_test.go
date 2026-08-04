package dns

import (
	"net"
	"testing"
)

// DKIM TXT'i çözümleyici parçalara bölerek döndürebilir ve bazı sağlayıcılar
// uzun anahtarı boşluklu saklar; p= değeri her iki durumda da panelinkiyle
// karşılaştırılabilecek şekilde normalize edilmeli.
func TestTxtdenP(t *testing.T) {
	const anahtar = "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA"
	testler := map[string]string{
		"v=DKIM1; k=rsa; p=" + anahtar:            anahtar,
		"v=DKIM1;k=rsa;p=" + anahtar:              anahtar,
		"v=DKIM1; k=rsa; p=MIIBIjAN Bgkq hkiG9w0": "MIIBIjANBgkqhkiG9w0",
		"v=DKIM1; k=rsa":                          "",
		"":                                        "",
	}
	for girdi, beklenen := range testler {
		if got := txtdenP(girdi); got != beklenen {
			t.Errorf("txtdenP(%q) = %q, beklenen %q", girdi, got, beklenen)
		}
	}
}

// SPF mekanizma araması TAM KELİME olmalı: "a" araması "ip4:1.2.3.4"
// içindeki harflere takılırsa yetkisiz bir SPF "ok" görünür.
func TestAlanKelimeIcerir(t *testing.T) {
	spf := "v=spf1 ip4:178.162.242.174 include:_spf.google.com ~all"
	if alanKelimeIcerir(spf, "a") {
		t.Error("'a' mekanizması yokken bulundu (ip4/all içindeki harfe takıldı)")
	}
	if alanKelimeIcerir(spf, "mx") {
		t.Error("'mx' mekanizması yokken bulundu")
	}

	spf2 := "v=spf1 a mx ip4:178.162.242.174 ~all"
	if !alanKelimeIcerir(spf2, "a") || !alanKelimeIcerir(spf2, "mx") {
		t.Error("gerçekten var olan a/mx mekanizmaları bulunamadı")
	}
	// Nitelikli (+a, -mx) ve parametreli (a:host, mx/24) biçimler.
	if !alanKelimeIcerir("v=spf1 +a -mx:mail.ornek.com ~all", "a") {
		t.Error("'+a' tanınmadı")
	}
	if !alanKelimeIcerir("v=spf1 mx:mail.ornek.com ~all", "mx") {
		t.Error("'mx:host' tanınmadı")
	}
}

func TestIpIcerir(t *testing.T) {
	adresler := []net.IPAddr{
		{IP: net.ParseIP("178.162.242.174")},
		{IP: net.ParseIP("2001:db8::1")},
	}
	if !ipIcerir(adresler, "178.162.242.174") {
		t.Error("mevcut IPv4 bulunamadı")
	}
	if !ipIcerir(adresler, "2001:db8::1") {
		t.Error("mevcut IPv6 bulunamadı")
	}
	if ipIcerir(adresler, "1.2.3.4") {
		t.Error("olmayan IP bulundu")
	}
	if ipIcerir(adresler, "") {
		t.Error("boş IP eşleşti")
	}
	// IPv4-mapped IPv6 gösterimi aynı adres sayılmalı (net.IP.Equal davranışı).
	if !ipIcerir([]net.IPAddr{{IP: net.ParseIP("::ffff:1.2.3.4")}}, "1.2.3.4") {
		t.Error("IPv4-mapped adres eşleşmedi")
	}
}

func TestKisaltAnahtar(t *testing.T) {
	if got := kisaltAnahtar("kisa"); got != "kisa" {
		t.Errorf("kısa değer kısaltılmamalı: %q", got)
	}
	uzun := "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAxuXbHrYVtDlkggbanjSdcvcj"
	got := kisaltAnahtar(uzun)
	if len(got) >= len(uzun) || !contains(got, "…") {
		t.Errorf("uzun anahtar kısaltılmadı: %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
