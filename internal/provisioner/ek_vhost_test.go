package provisioner

import (
	"strings"
	"testing"
)

// Bu testler 2026-07-27'de canlıda yaşanan olayın birim karşılığıdır: ek alan
// adına SSL alınması ana alan adının vhost'unu ezmiş, ana alan adı nginx'ten
// tamamen düşmüş ve istekler default sunucunun sertifikasını yemişti.

// TestEkConfPathAnaVhostuEzmez: ek alan adının dosya yolu ASLA dom_<sk>.conf
// olmamalı — ana alan adının vhost'u tam olarak orada durur.
func TestEkConfPathAnaVhostuEzmez(t *testing.T) {
	yol := EkConfPath("c_sille_tr", "sille.org.tr")

	if yol == "/etc/nginx/conf.d/dom_c_sille_tr.conf" {
		t.Fatal("ek alan adı ana alan adının vhost dosyasına yazıyor")
	}
	if !strings.Contains(yol, "sille.org.tr") {
		t.Errorf("dosya adı alan adını içermeli, aksi halde aynı sk'deki ek alan adları çakışır: %q", yol)
	}
	// Aynı sk altındaki iki ayrı ek alan adı aynı dosyaya düşmemeli.
	if EkConfPath("c_sille_tr", "a.tr") == EkConfPath("c_sille_tr", "b.tr") {
		t.Error("aynı sk'deki farklı ek alan adları aynı dosyayı paylaşıyor")
	}
}

func TestEkVhostIcerikSSLsiz(t *testing.T) {
	c := EkVhostIcerik("sille.org.tr", "/home/c_sille_tr/domains/sille.org.tr", "/run/x.sock", "", "")

	if strings.Contains(c, "listen 443") {
		t.Error("sertifika verilmediği hâlde 443 bloğu üretildi")
	}
	if strings.Contains(c, "return 301 https://") {
		t.Error("SSL yokken HTTPS'e yönlendirme üretildi — site erişilemez olurdu")
	}
	if !strings.Contains(c, "acme-challenge") {
		t.Error("ACME challenge location'ı yok — ilk sertifika hiç alınamaz")
	}
}

func TestEkVhostIcerikSSLli(t *testing.T) {
	cert := "/etc/pki/sanalcp/sille.org.tr/sille.org.tr.crt"
	key := "/etc/pki/sanalcp/sille.org.tr/sille.org.tr.key"
	c := EkVhostIcerik("sille.org.tr", "/home/c_sille_tr/domains/sille.org.tr", "/run/x.sock", cert, key)

	if !strings.Contains(c, "listen 443 ssl") {
		t.Fatal("443 bloğu üretilmedi — sertifika alınsa bile sunulamaz")
	}
	if !strings.Contains(c, "ssl_certificate     "+cert) {
		t.Error("443 bloğu ek alan adının kendi sertifikasını göstermiyor")
	}
	// Docroot ana alan adınınki (public_html) DEĞİL, ek alan adınınki olmalı.
	if strings.Contains(c, "/public_html") {
		t.Error("ek alan adı ana alan adının docroot'unu sunuyor")
	}
	if !strings.Contains(c, "root /home/c_sille_tr/domains/sille.org.tr;") {
		t.Error("ek alan adının kendi docroot'u kullanılmamış")
	}
	// ACME challenge SSL açıkken de erişilebilir kalmalı, yoksa yenileme kırılır.
	if !strings.Contains(c, "acme-challenge") {
		t.Error("SSL'li vhost'ta ACME challenge yolu kapalı — 90 günde yenileme başarısız olur")
	}
	if !strings.Contains(c, "fastcgi_param HTTPS on;") {
		t.Error("PHP'ye HTTPS bildirilmiyor — uygulama kendini http sanır")
	}
}

// TestEkVhostWWWKapsar: panel sertifikayı domain+www olarak alıyor; server_name
// www'yi kapsamazsa istek default sunucuya düşer ve sertifika uyuşmaz.
func TestEkVhostWWWKapsar(t *testing.T) {
	for _, cert := range []string{"", "/tmp/c.crt"} {
		c := EkVhostIcerik("sille.org.tr", "/srv/x", "/run/x.sock", cert, "/tmp/c.key")
		for _, blok := range strings.Split(c, "server_name ")[1:] {
			ad := strings.SplitN(blok, ";", 2)[0]
			if !strings.Contains(ad, "www.sille.org.tr") {
				t.Errorf("server_name www'yi kapsamıyor (cert=%q): %q", cert, ad)
			}
		}
	}
}
