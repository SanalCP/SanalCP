package panelayarlari

// Panel özel domaini için port'suz erişim (443) — CloudPanel'deki gibi
// https://panel.ornek.com, https://panel.ornek.com:8443 YAZMADAN çalışsın diye.
//
// TASARIM: _panel.conf'un (8443, /api/, /pma/, /webmail/, güvenlik header'ları, statik
// frontend) zengin mantığını burada YENİDEN YAZMIYORUZ — panel domaini için 443'te
// SADECE küçük bir SNI bloğu açıp https://127.0.0.1:8443'e (değişmeyen _panel.conf'a)
// proxy_pass yapıyoruz. Böylece panel TEK yerde tanımlı kalıyor, bu dosya sadece
// "port'suz giriş kapısı". Çift TLS terminasyonu (443'te bir, loopback'te bir daha)
// düşük trafikli bir admin paneli için önemsiz bir maliyet.
//
// GÜVENLİK: bu dosya SADECE gerçek bir Let's Encrypt sertifikası kurulduğunda
// (ssl_durum='aktif') yazılır — self-signed fallback'te port'suz erişim AÇILMAZ, aksi
// halde "otomatik SSL" vaadiyle çelişen bir sertifika-uyarılı port'suz erişim olurdu.
// 443'ün eşleşmeyen-SNI güvenlik ağı assets/nginx/_default443.conf'tur (default_server
// AÇIKÇA işaretli — nginx conf.d dosyaları alfabetik yüklendiği için bu işaret olmasa
// bir tenant ya da panel vhost'u dosya sırasına göre YANLIŞLIKLA 443 varsayılanı olabilirdi).

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const panelDomainVhostYol = "/etc/nginx/conf.d/_panel_domain.conf"

const panelDomainVhostSablon = `# SanalCP — özel panel alan adı (internal/panelayarlari tarafından yazılır/silinir,
# Kaydet/Kaldır işlemleri üzerine yazar — elle düzenlemeyin).
server {
    listen 80;
    listen [::]:80;
    server_name %s;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name %s;

    ssl_certificate     ` + certYol + `;
    ssl_certificate_key ` + keyYol + `;
    ssl_protocols TLSv1.2 TLSv1.3;

    client_max_body_size 10240m;

    location / {
        proxy_pass https://127.0.0.1:8443;
        proxy_ssl_verify off;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        # _panel.conf istemci IP'sini YALNIZ nginx anahtarı doğruysa bu bloktan
        # kabul eder (bkz. internal/nginxconf/proxyanahtar.go); aksi hâlde tüm
        # istekler 127.0.0.1 görünürdü.
        proxy_set_header X-SanalCP-Proxy $sanalcp_proxy_anahtari;
        proxy_set_header X-SanalCP-Istemci $remote_addr;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto https;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
`

var panelDomainServerNameRE = regexp.MustCompile(`(?m)^\s*server_name\s+([A-Za-z0-9.-]+);`)

// HealPort443VhostOnStartup: anahtar başlığından önce yazılmış bir panel alan
// adı vhost'unu güncel şablonla yeniden yazar. Dosya yoksa ya da zaten
// günceldse no-op; nginx -t başarısız olursa port443VhostYaz eskisini geri koyar.
func HealPort443VhostOnStartup() {
	mevcut, err := os.ReadFile(panelDomainVhostYol)
	if err != nil || strings.Contains(string(mevcut), "X-SanalCP-Istemci") {
		return
	}
	m := panelDomainServerNameRE.FindStringSubmatch(string(mevcut))
	if m == nil {
		log.Printf("panel alan adı vhost'u: server_name okunamadı, güncellenmedi (%s)", panelDomainVhostYol)
		return
	}
	if err := port443VhostYaz(m[1]); err != nil {
		log.Printf("panel alan adı vhost'u güncellenemedi: %v", err)
	}
}

// port443VhostYaz: panel domaini için 443'te (port'suz) bir SNI bloğu yazar ve nginx'i
// güvenle yeniden yükler. nginx -t başarısız olursa ESKİ içerik geri yüklenir (veya
// dosya yoksa silinir) — panelin :8443 erişimi bu adımdan ETKİLENMEZ.
func port443VhostYaz(domain string) error {
	icerik := fmt.Sprintf(panelDomainVhostSablon, domain, domain)
	yedek, yedekErr := os.ReadFile(panelDomainVhostYol)
	yedekVar := yedekErr == nil

	if err := os.WriteFile(panelDomainVhostYol, []byte(icerik), 0644); err != nil {
		return err
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		if yedekVar {
			_ = os.WriteFile(panelDomainVhostYol, yedek, 0644)
		} else {
			_ = os.Remove(panelDomainVhostYol)
		}
		return fmt.Errorf("nginx -t: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx reload: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// port443VhostSil: panel domaini kaldırıldığında (Kaldır) veya SSL geçersiz hale
// geldiğinde 443'teki port'suz girişi kapatır. Dosya yoksa no-op.
func port443VhostSil() {
	if _, err := os.Stat(panelDomainVhostYol); err != nil {
		return
	}
	_ = os.Remove(panelDomainVhostYol)
	_, _ = exec.Command("nginx", "-t").CombinedOutput()
	_, _ = exec.Command("systemctl", "reload", "nginx").CombinedOutput()
}
