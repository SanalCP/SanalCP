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
