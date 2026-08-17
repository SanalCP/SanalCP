package provisioner

import (
	"strings"
	"testing"

	"sanalcp/internal/nginxconf"
)

// panelConfFirstLineContaining: nginxconf.PanelConf içinde (tek kaynak,
// SANAL-PANEL-SEC bloğu) verilen alt dizeyi içeren İLK satırı, baştaki/sondaki
// boşluk kırpılmış hâlde döner. Server-seviyesi bloğu bulmak için kullanılır —
// dosyada aynı direktif nested location'larda tekrar edebiliyor, ilk satır
// her zaman server-seviyesi (SANAL-PANEL-SEC) satırıdır.
func panelConfFirstLineContaining(alt string) string {
	for _, satir := range strings.Split(nginxconf.PanelConf, "\n") {
		if strings.Contains(satir, alt) {
			return strings.TrimSpace(satir)
		}
	}
	return ""
}

// TestPanelHealCSPMatchesPanelConf: HealPanelIndexNoCacheOnStartup'ın enjekte
// ettiği CSP/HSTS, tek kaynak _panel.conf'un (internal/nginxconf/_panel.conf,
// SANAL-PANEL-SEC v3 bloğu) sunduğu politikayla BİREBİR aynı olmalı.
//
// Bu test, canlıda gerçekten yaşanan bir bug'ı bir daha imkansız kılmak için
// var: heal bloğu bir süre v2'nin script-src'sinde kalan 'unsafe-inline'
// 'unsafe-eval'i taşıyordu — v3'ün kasıtlı olarak kaldırdığı XSS gevşetmesini
// heal her tetiklendiğinde GERİ SOKUYORDU. _panel.conf güncellenip bu sabit
// unutulursa (ya da tam tersi), bu test kırılır.
func TestPanelHealCSPMatchesPanelConf(t *testing.T) {
	kanonikCSP := panelConfFirstLineContaining("Content-Security-Policy")
	if kanonikCSP == "" {
		t.Fatal("_panel.conf içinde CSP satırı bulunamadı — dosya mı değişti?")
	}
	if panelHealCSP != kanonikCSP {
		t.Errorf("heal bloğunun CSP'si _panel.conf'un kanonik SANAL-PANEL-SEC CSP'siyle uyuşmuyor:\nheal:     %s\n_panel.conf: %s", panelHealCSP, kanonikCSP)
	}
	if strings.Contains(panelHealCSP, "unsafe-inline' 'unsafe-eval") || strings.Contains(panelHealCSP, "'unsafe-eval'") {
		t.Error("heal CSP'sinde script-src icin unsafe-eval var — v3'un kasitli kaldirdigi XSS gevsetmesi geri sokulmus")
	}

	kanonikHSTS := panelConfFirstLineContaining("Strict-Transport-Security")
	if kanonikHSTS == "" {
		t.Fatal("_panel.conf içinde HSTS satırı bulunamadı")
	}
	if hdrHSTS != kanonikHSTS {
		t.Errorf("paylaşılan hdrHSTS sabiti _panel.conf'un kanonik HSTS satırıyla uyuşmuyor:\nhdrHSTS:     %s\n_panel.conf: %s", hdrHSTS, kanonikHSTS)
	}
}
