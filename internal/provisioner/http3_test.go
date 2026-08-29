package provisioner

import (
	"bytes"
	"strings"
	"testing"
)

func TestVhostHTTP3YalnizAcikkenUretilir(t *testing.T) {
	o := VhostOpts{AlanAdi: "example.com", WebRoot: "/home/c_x/public_html", CertPath: "/cert", KeyPath: "/key", Backend: "static", HTTP3: true}
	var b bytes.Buffer
	if err := vhostTmpl.Execute(&b, o); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	if !strings.Contains(s, "listen 443 quic;") || !strings.Contains(s, "Alt-Svc") {
		t.Fatalf("HTTP/3 blok eksik: %s", s)
	}
	o.HTTP3 = false
	b.Reset()
	_ = vhostTmpl.Execute(&b, o)
	if strings.Contains(b.String(), "listen 443 quic") {
		t.Fatal("kapalı HTTP/3 render edildi")
	}
}

func TestCacheAtlamaWordPressVePrestaShop(t *testing.T) {
	o := VhostOpts{AlanAdi: "example.com", WebRoot: "/home/c_x/public_html", Backend: "php-fpm", PHPSocket: "/run/x.sock", FastCgiCache: true, FastCgiCacheDakika: 30, CacheProfili: "prestashop"}
	var b bytes.Buffer
	_ = vhostTmpl.Execute(&b, o)
	s := b.String()
	for _, want := range []string{"PrestaShop-", "/checkout/", "/admin[^/]*/"} {
		if !strings.Contains(s, want) {
			t.Errorf("cache bypass eksik: %s", want)
		}
	}
	if strings.Contains(s, "wordpress_logged_in") {
		t.Fatal("PrestaShop profiline WordPress cookie kuralı karıştı")
	}
}
