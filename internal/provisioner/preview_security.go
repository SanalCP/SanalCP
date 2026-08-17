package provisioner

import (
	"net"
	"os"
	"strings"
)

// panelFrameAncestors tenant sitelerinin yalnız kendi origin'i ve SanalCP
// tarafından iframe içinde gösterilebilmesini sağlayan CSP kaynak listesidir.
// X-Frame-Options farklı bir origin'e güvenli izin veremediği için kullanılmaz.
func panelFrameAncestors() string {
	sources := []string{"'self'"}
	seen := map[string]bool{"'self'": true}
	add := func(source string) {
		if source != "" && !seen[source] {
			seen[source] = true
			sources = append(sources, source)
		}
	}

	ip := strings.TrimSpace(os.Getenv("PANEL_PUBLIC_IPV4"))
	if net.ParseIP(ip) == nil && pkgDB != nil {
		_ = pkgDB.QueryRow(`SELECT ipv4 FROM domains WHERE ipv4<>'' ORDER BY id LIMIT 1`).Scan(&ip)
		ip = strings.TrimSpace(ip)
	}
	if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
		add("https://" + parsed.String() + ":8443")
	}

	if pkgDB != nil {
		var domain string
		if err := pkgDB.QueryRow(
			`SELECT COALESCE(ozel_domain,'') FROM panel_ayarlari WHERE id=1 AND ssl_durum='aktif'`,
		).Scan(&domain); err == nil {
			domain = strings.ToLower(strings.TrimSpace(domain))
			if ValidateDomain(domain) == nil {
				add("https://" + domain)
				add("https://" + domain + ":8443")
			}
		}
	}
	return strings.Join(sources, " ")
}

func framePolicyHeader(indent string, upgradeHTTPS bool) string {
	policy := "frame-ancestors " + panelFrameAncestors()
	if upgradeHTTPS {
		policy += "; upgrade-insecure-requests"
	}
	return indent + `add_header Content-Security-Policy "` + policy + `" always;` + "\n"
}

// Tek kaynaktan üretilen temel güvenlik header değerleri — vhost üreten HER
// yer (ana domain, subdomain, ek alan adı, webmail) burayı çağırmalı. Değer
// buradan başka bir yerde elle kopyalanırsa politika değişikliği o kopyaya
// ULAŞMAZ (bkz. panelHealSecurityHeaders'ın CSP'yi bir süre eskide bıraktığı
// bug — aynı sınıf hatayı burada tekrarlamamak için tek fonksiyon).
const (
	hdrXContentTypeOptions = `add_header X-Content-Type-Options "nosniff" always;`
	hdrReferrerPolicy      = `add_header Referrer-Policy "strict-origin-when-cross-origin" always;`
	hdrPermissionsPolicy   = `add_header Permissions-Policy "geolocation=(), microphone=(), camera=(), interest-cohort=()" always;`
	hdrHSTS                = `add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;`
)

// BaselineSecurityHeaders: subdomain/ek-vhost/webmail gibi kendi header toggle
// UI'ı olmayan yüzeyler için sabit temel set. includeHSTS yalnız SSL aktifken
// true verilmeli (düz HTTP'de HSTS anlamsız/yanıltıcı).
func BaselineSecurityHeaders(indent string, includeHSTS bool) string {
	b := indent + hdrXContentTypeOptions + "\n" +
		indent + hdrReferrerPolicy + "\n" +
		indent + hdrPermissionsPolicy + "\n"
	if includeHSTS {
		b += indent + hdrHSTS + "\n"
	}
	return b
}
