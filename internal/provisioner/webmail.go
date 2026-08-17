package provisioner

import "os"

// webmail.go — Müşterinin KENDİ alan adı üzerinden webmail erişimi.
//
// SORUN: Roundcube yalnız panelin 8443 vhost'unda (/webmail/) servis ediliyordu.
// Müşteriye verilebilecek adres ya panelin hostname'i ya da — panele IP ile
// girilmişse — çıplak sunucu IP'siydi. Hosting müşterisinin beklentisi ise
// kendi alan adıdır: https://musterim.com/webmail/ (cPanel/Plesk deseni).
//
// ÇÖZÜM: aynı Roundcube kurulumu her domainin kendi vhost'una `/webmail/`
// yolundan bağlanır. Bu yol BİLEREK seçildi:
//
//   - webmail.<domain> ALT ALAN ADI yerine yol kullanmak, ek DNS kaydı ve
//     sertifika SAN'ı gerektirmez. Alt alan adı yaklaşımında her domain için
//     hem A kaydı hem LE sertifikasına yeni bir isim eklemek gerekir; ikisinden
//     biri eksikse tarayıcı sertifika uyarısı verir.
//   - Domainin mevcut sertifikası (apex + www) bu yolu zaten kapsar.
//
// TAKAS: müşterinin sitesinde gerçek bir /webmail dizini varsa `^~` önek
// eşleşmesi onu gölgeler. cPanel de aynı adı rezerve eder; kabul edilen davranış.

// roundcubeKok: Roundcube'un web köküne çıkan dizin. Kurulu değilse blok
// üretilmez — aksi hâlde her vhost'a 404 veren ölü bir yol eklenirdi.
const roundcubeKok = "/opt/roundcube/public_html"

// webmailNginx: vhost'a eklenen blok.
//
// 🔴 Roundcube 1.7 ayrıntıları (assets/nginx/_panel.conf'ta aynısı, gerçek
// sunucuda bulunmuştu): web kökü public_html/'dir ve skin/plugin varlıkları
// doğrudan dosya olarak DEĞİL static.php üzerinden PATH_INFO ile sunulur.
// Bu yüzden static.php, uzantı-bazlı statik bloktan ÖNCE kendi
// fastcgi_split mantığıyla eşlenmelidir; aksi hâlde nginx onu "var olmayan bir
// .css dosyası" sanıp 404 verir.
//
// PHP-FPM havuzu tenant'ın DEĞİL roundcube havuzudur (apache kullanıcısı):
// webmail tüm domainler için ortak bir kurulumdur, müşteri kimliğiyle
// çalıştırılamaz.
// var (const değil): BaselineSecurityHeaders çağrısı sabit ifade değildir.
var webmailNginx = `
    # ---- Webmail (Roundcube) — tüm domainler için ortak kurulum ----
    location ^~ /webmail/ {
        alias ` + roundcubeKok + `/;
        index index.php;
` + BaselineSecurityHeaders("        ", false) + `
        location ~ ^/webmail/static\.php(/.+)$ {
            fastcgi_pass unix:/run/php-fpm/roundcube.sock;
            fastcgi_param SCRIPT_FILENAME ` + roundcubeKok + `/static.php;
            fastcgi_param PATH_INFO $1;
            fastcgi_param SCRIPT_NAME /webmail/static.php;
            include fastcgi_params;
            fastcgi_read_timeout 60s;
        }

        location ~ ^/webmail/(.+\.php)$ {
            alias ` + roundcubeKok + `/$1;
            fastcgi_pass unix:/run/php-fpm/roundcube.sock;
            fastcgi_param SCRIPT_FILENAME ` + roundcubeKok + `/$1;
            fastcgi_param SCRIPT_NAME /webmail/$1;
            include fastcgi_params;
            fastcgi_read_timeout 60s;
        }

        location ~ ^/webmail/(.+\.(jpg|jpeg|gif|css|png|js|ico|html|xml|txt|svg|woff2?|map))$ {
            alias ` + roundcubeKok + `/$1;
            expires 7d;
            add_header Cache-Control "public, immutable";
` + BaselineSecurityHeaders("            ", false) + `
        }

        # Roundcube'un kendi hassas dizinleri — alias .htaccess uygulamaz, elle kapat.
        location ~ ^/webmail/(config|temp|logs|SQL|bin|tests)/ { deny all; return 404; }
    }

    # /webmail (sondaki eğik çizgi olmadan) → /webmail/
    location = /webmail { return 301 /webmail/; }
`

// roundcubeKokTest: testlerin geçici dizine yönlendirmesi için (üretimde
// roundcubeKok kullanılır).
var roundcubeKokTest = roundcubeKok

// webmailBloku: Roundcube kuruluysa nginx bloğunu, değilse boş dize döner.
func webmailBloku() string { return webmailBlokuYol(roundcubeKok) }

func webmailBlokuYol(kok string) string {
	if fi, err := os.Stat(kok); err != nil || !fi.IsDir() {
		return ""
	}
	return webmailNginx
}

// WebmailKurulu: panel API'si müşteriye webmail bağlantısı gösterip
// göstermeyeceğine bunun üzerinden karar verir.
func WebmailKurulu() bool {
	fi, err := os.Stat(roundcubeKok)
	return err == nil && fi.IsDir()
}
