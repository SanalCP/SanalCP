package provisioner

import (
	"strings"
	"testing"
)

func TestTenantFramePolicyPanelOnizlemesiniCSPileKorur(t *testing.T) {
	headers := buildSecurityHeaders(VhostOpts{})
	if strings.Contains(headers, "X-Frame-Options") {
		t.Fatal("X-Frame-Options farklı panel origin'ini engeller; üretilmemeli")
	}
	if !strings.Contains(headers, `Content-Security-Policy "frame-ancestors 'self'`) {
		t.Fatalf("enforce edilen frame-ancestors politikası yok:\n%s", headers)
	}
}

func TestEkVhostFramePolicyUretir(t *testing.T) {
	vhost := EkVhostIcerik("ornek.test", "/srv/ornek", "/run/ornek.sock", "", "")
	if strings.Contains(vhost, "X-Frame-Options") {
		t.Fatal("ek vhost panel önizlemesini X-Frame-Options ile engelliyor")
	}
	if !strings.Contains(vhost, `Content-Security-Policy "frame-ancestors 'self'`) {
		t.Fatal("ek vhost enforce edilen frame-ancestors politikası üretmiyor")
	}
}
