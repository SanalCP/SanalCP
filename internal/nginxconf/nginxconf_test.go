package nginxconf

import (
	"regexp"
	"strings"
	"testing"
)

func TestPanelConfGomulu(t *testing.T) {
	if len(PanelConf) < 1000 {
		t.Fatalf("gömülü _panel.conf beklenenden kısa (%d bayt) — embed başarısız olmuş olabilir", len(PanelConf))
	}
	for _, gerekli := range []string{
		"listen 8443 ssl http2 default_server;",
		"location /api/",
		"location ^~ /pma/",
		"location ^~ /webmail/",
	} {
		if !strings.Contains(PanelConf, gerekli) {
			t.Errorf("gömülü conf'ta %q yok", gerekli)
		}
	}
}

// Süslü parantez dengesi: bozuk bir conf'u binary'ye gömüp sunucuda nginx -t'ye
// yakalatmak yerine burada yakala.
func TestPanelConfParantezDengesi(t *testing.T) {
	derinlik := 0
	for i, satir := range strings.Split(PanelConf, "\n") {
		kod := satir
		if j := strings.Index(kod, "#"); j >= 0 && strings.Count(kod[:j], `"`)%2 == 0 {
			kod = kod[:j]
		}
		derinlik += strings.Count(kod, "{") - strings.Count(kod, "}")
		if derinlik < 0 {
			t.Fatalf("satır %d: fazla '}'", i+1)
		}
	}
	if derinlik != 0 {
		t.Fatalf("parantez dengesi bozuk: %d blok açık kaldı", derinlik)
	}
}

// 🔴 Bulgu 4'ün regresyon testi: script-src gevşetmesi SADECE pma/webmail
// bloklarında olmalı. Panel SPA'sına (server seviyesi + location /) sızarsa
// XSS savunması sessizce kaybolur.
func TestCSPGevsetmesiSadecePmaVeWebmailde(t *testing.T) {
	// Yalnız GERÇEK CSP satırları; yorumlarda geçen "script-src" sayılmamalı.
	re := regexp.MustCompile(`script-src [^;"]*`)
	var bulunan []string
	for _, s := range strings.Split(PanelConf, "\n") {
		if !strings.Contains(s, "add_header Content-Security-Policy") {
			continue
		}
		bulunan = append(bulunan, re.FindAllString(s, -1)...)
	}
	if len(bulunan) != 4 {
		t.Fatalf("4 script-src bekleniyordu (server, location /, pma, webmail), %d bulundu: %v",
			len(bulunan), bulunan)
	}

	var siki, gevsek int
	for _, s := range bulunan {
		switch {
		case strings.Contains(s, "unsafe-inline"), strings.Contains(s, "unsafe-eval"):
			gevsek++
		default:
			siki++
		}
	}
	if siki != 2 {
		t.Errorf("2 sıkı script-src bekleniyordu (server + location /), %d bulundu: %v", siki, bulunan)
	}
	if gevsek != 2 {
		t.Errorf("2 gevşek script-src bekleniyordu (pma + webmail), %d bulundu: %v", gevsek, bulunan)
	}
}

// nginx'in add_header mirası "hepsi ya da hiçbiri"dir: CSP yazan her blok
// diğer güvenlik header'larını da tekrar yazmalı, yoksa sessizce düşerler.
func TestCSPYazanHerBlokTumBaslikariTekrarliyor(t *testing.T) {
	esler := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Permissions-Policy",
		"Strict-Transport-Security",
	}
	satirlar := strings.Split(PanelConf, "\n")
	for i, s := range satirlar {
		if !strings.Contains(s, "add_header Content-Security-Policy") {
			continue
		}
		// CSP satırının ±10 satırlık komşuluğunda tüm eşlerin bulunması beklenir.
		alt, ust := max(0, i-10), min(len(satirlar), i+11)
		komsuluk := strings.Join(satirlar[alt:ust], "\n")
		for _, e := range esler {
			if !strings.Contains(komsuluk, e) {
				t.Errorf("satır %d'deki CSP bloğunda %s eksik — add_header mirası bu blokta üst seviyeyi düşürür", i+1, e)
			}
		}
	}
}

func TestElDegmemis(t *testing.T) {
	if !ElDegmemis([]byte(PanelConf)) {
		t.Error("kanonik sürümün kendisi el değmemiş sayılmalı")
	}
	if ElDegmemis([]byte("admin'in elle yazdığı bambaşka bir conf")) {
		t.Error("bilinmeyen içerik el değmemiş sayıldı — admin'in dosyası ezilirdi")
	}
}

// Yayınlanmış her sürümün hash'i listede kalmalı; biri düşerse o sürümü
// çalıştıran kurulumlar "özelleştirilmiş" sayılıp güncellemeyi ALAMAZ.
func TestBilinenHashler(t *testing.T) {
	const surum059 = "8455894d0e5f0fc47875bb9cdb53911220de1dfa1499c61519e109975bbd09f5"
	if _, ok := BilinenHashler[surum059]; !ok {
		t.Errorf("0.5.5-0.5.9 hash'i listeden düşmüş — o sürümdeki kurulumlar güncelleme alamaz")
	}
	if _, ok := BilinenHashler[KanonikHash()]; ok {
		t.Error("kanonik hash BilinenHashler'da olmamalı (orası ÖNCEKİ sürümler içindir)")
	}
	for h := range BilinenHashler {
		if len(h) != 64 {
			t.Errorf("geçersiz sha256 uzunluğu: %q", h)
		}
	}
}
