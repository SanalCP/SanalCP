package provisioner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Ek alan adı (addon/parked domain) vhost üretimi.
//
// 🔴 NEDEN AYRI BİR DOSYA: ek alan adları ana alan adıyla AYNI sistem kullanıcısını
// paylaşır. Normal vhost yolu dosya adını yalnızca sk'den türetir
// (/etc/nginx/conf.d/dom_<sk>.conf), dolayısıyla bir ek alan adı için normal yolu
// çağırmak ANA alan adının vhost'unu baştan yazar ve onu nginx'ten tamamen silerdi.
//
// Canlıda tam olarak bu yaşandı (2026-07-27, sille.tr / sille.org.tr): ek alan
// adına SSL alındığı anda dom_c_sille_tr.conf ek alan adının vhost'u hâline geldi,
// sille.tr'yi karşılayan hiçbir server bloğu kalmadı ve istekler default sunucuya
// düşüp panelin sertifikasını sundu ("sertifika geçersiz"). nginx o anda
// `conflicting server name "sille.org.tr"` uyarısı da bastı — iki blok birden aynı
// adı iddia ediyordu. Ek alan adına SSL alan her yol bu dosyadaki üreticiyi
// kullanmak ZORUNDA; dom_<sk>.conf'a asla dokunmamalı.

// EkConfPath: ek alan adının kendi nginx dosyası. Dosya adı hem sk hem alan adı
// içerir; aynı sk altında birden çok ek alan adı çakışmadan durabilsin diye.
func EkConfPath(sk, alanAdi string) string {
	return "/etc/nginx/conf.d/ek_" + sk + "_" + alanAdi + ".conf"
}

// EkVhostIcerik: ek alan adı vhost metni.
//
// certPath boşsa yalnız :80 bloğu (HTTP) üretilir. Doluysa :80 bloğu ACME
// challenge'ı açık bırakıp geri kalanı 443'e yönlendirir ve ayrıca bir :443
// bloğu üretilir — ana alan adının vhost'uyla aynı davranış.
//
// server_name'e www. DA eklenir: panel Let's Encrypt sertifikasını domain+www
// olarak alıyor (bkz. sslCertAl SAN listesi); www için server bloğu olmazsa
// sertifika www'yi kapsamasına rağmen istek default sunucuya düşer.
func EkVhostIcerik(alanAdi, docroot, socket, certPath, keyPath string) string {
	adlar := alanAdi + " www." + alanAdi
	log := "    access_log /var/log/nginx/" + alanAdi + ".access.log;\n" +
		"    error_log  /var/log/nginx/" + alanAdi + ".error.log warn;\n"
	basliklar := "    add_header X-Content-Type-Options \"nosniff\" always;\n" +
		"    add_header X-XSS-Protection \"1; mode=block\" always;\n" +
		framePolicyHeader("    ", certPath != "")

	govde := "    root " + docroot + ";\n" +
		"    index index.php index.html index.htm;\n" +
		"    # Sembolik baglanti saldirisi engeli: dosya, sahibi-farkli bir symlink uzerinden sunulmaz.\n" +
		"    disable_symlinks if_not_owner;\n\n" +
		log + "\n" + basliklar + "\n" +
		"    location / { try_files $uri $uri/ /index.php?$query_string; }\n\n" +
		"    location ~ \\.php$ {\n" +
		"        try_files $uri =404;\n" +
		"        fastcgi_split_path_info ^(.+\\.php)(/.+)$;\n" +
		"        fastcgi_pass unix:" + socket + ";\n" +
		"        fastcgi_index index.php;\n" +
		"        include fastcgi_params;\n" +
		"        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;\n" +
		"        fastcgi_read_timeout 60s;\n"
	if certPath != "" {
		// PHP tarafı isteğin şifreli geldiğini bilmeli (WordPress site_url vb.).
		govde += "        fastcgi_param HTTPS on;\n"
	}
	govde += "    }\n\n" +
		"    location ~* \\.(jpg|jpeg|png|gif|ico|css|js|woff2?|svg|webp|avif|pdf|zip|gz)$ {\n" +
		"        expires 30d;\n" +
		"        access_log off;\n" +
		"    }\n\n" +
		"    location ~ /\\.(?!well-known) { deny all; }\n"

	acme := "    location /.well-known/acme-challenge/ {\n" +
		"        root /var/www/_acme;\n" +
		"        auth_basic off;\n" +
		"        try_files $uri =404;\n" +
		"    }\n"

	if certPath == "" {
		return "server {\n" +
			"    listen 80;\n    listen [::]:80;\n" +
			"    server_name " + adlar + ";\n\n" +
			acme + "\n" +
			govde + "\n" +
			"    # SanalCP ek alan adı — " + alanAdi + "\n" +
			"}\n"
	}

	return "# " + alanAdi + " (ek alan adı) — 80 üzerinde HTTP-01 challenge için açık; geri kalan trafik 443'e yönlendirilir\n" +
		"server {\n" +
		"    listen 80;\n    listen [::]:80;\n" +
		"    server_name " + adlar + ";\n\n" +
		acme + "\n" +
		"    location / {\n        return 301 https://$host$request_uri;\n    }\n" +
		"}\n\n" +
		"server {\n" +
		"    listen 443 ssl http2;\n    listen [::]:443 ssl http2;\n" +
		"    server_name " + adlar + ";\n\n" +
		"    ssl_certificate     " + certPath + ";\n" +
		"    ssl_certificate_key " + keyPath + ";\n" +
		"    ssl_protocols TLSv1.2 TLSv1.3;\n" +
		"    ssl_ciphers HIGH:!aNULL:!MD5;\n" +
		"    ssl_prefer_server_ciphers on;\n" +
		"    ssl_session_cache shared:SSL:10m;\n" +
		"    ssl_session_timeout 1d;\n\n" +
		govde + "\n" +
		"    # SanalCP ek alan adı (SSL) — " + alanAdi + "\n" +
		"}\n"
}

// EkVhostYaz: ek alan adının vhost'unu diske yazar, nginx -t ile doğrular ve reload eder.
//
// renderAndReload'daki fail-safe ile aynı: doğrulama patlarsa eski içerik geri
// yüklenir. Bozuk bir dosyayı diskte bırakmak sonraki HER nginx -t'yi (dolayısıyla
// panelin tüm vhost işlemlerini) global olarak kırar.
func EkVhostYaz(alanAdi, sk, docroot, socket, certPath, keyPath string) error {
	if err := ValidateDomain(alanAdi); err != nil {
		return err
	}
	cfgPath := EkConfPath(sk, alanAdi)
	yedek, rerr := os.ReadFile(cfgPath)
	yedekVar := rerr == nil

	if err := os.WriteFile(cfgPath, []byte(EkVhostIcerik(alanAdi, docroot, socket, certPath, keyPath)), 0644); err != nil {
		return fmt.Errorf("ek vhost yaz: %w", err)
	}
	_ = exec.Command("restorecon", cfgPath).Run()

	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		if yedekVar {
			_ = os.WriteFile(cfgPath, yedek, 0644)
		} else {
			_ = os.Remove(cfgPath)
		}
		_ = exec.Command("nginx", "-t").Run()
		return fmt.Errorf("nginx -t başarısız: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx reload: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ekAlanAdiBilgi: alan adı bir ek alan adı mı? Öyleyse kendi docroot'unu döner.
//
// Karar DB'den okunur (domains.ana_domain_id), çünkü SSL yollarının hiçbiri bu
// bilgiyi parametre olarak taşımıyor — sslVhostYaz tek çıkış noktası olduğu için
// kararı orada vermek, yenileme/iyileştirme (ssl_heal) dahil TÜM çağıranları tek
// hamlede doğru dosyaya yönlendirir.
//
// FAIL-SAFE: DB yoksa/okunamazsa false döner, yani davranış eskisi gibi kalır.
// Burada yanlış pozitif üretmek ana alan adının vhost'unu yazmayı engellerdi.
func ekAlanAdiBilgi(alanAdi string) (docroot string, ekMi bool) {
	if pkgDB == nil {
		return "", false
	}
	var ana *int64
	var wr string
	err := pkgDB.QueryRow(
		`SELECT ana_domain_id, COALESCE(web_root,'') FROM domains WHERE alan_adi=?`,
		alanAdi).Scan(&ana, &wr)
	if err != nil || ana == nil {
		return "", false
	}
	return wr, true
}
