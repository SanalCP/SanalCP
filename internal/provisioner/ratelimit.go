package provisioner

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var rateLimitConfPath = "/etc/nginx/conf.d/sanal_rate_limit.conf"

type rateLimitSatir struct {
	ID             int64
	Domain, Profil string
	Istek, Burst   int
	Bot            bool
	IPler, Yollar  string
}

// RateLimitGlobalYaz tüm etkin domainleri tek http-context dosyasında toplar.
// limit_req_zone server{} içinde geçerli olmadığından bu merkezi üretim zorunludur.
func RateLimitGlobalYaz(db *sql.DB) error {
	rows, err := db.Query(`SELECT r.domain_id,d.alan_adi,r.profil,r.istek_dakika,r.burst,r.bot_engelle,
		COALESCE(r.ip_istisnalari,''),COALESCE(r.yol_istisnalari,'')
		FROM domain_rate_limits r JOIN domains d ON d.id=r.domain_id WHERE d.ana_domain_id IS NULL ORDER BY r.domain_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var list []rateLimitSatir
	for rows.Next() {
		var x rateLimitSatir
		var bot int
		if err := rows.Scan(&x.ID, &x.Domain, &x.Profil, &x.Istek, &x.Burst, &bot, &x.IPler, &x.Yollar); err != nil {
			return err
		}
		x.Bot = bot == 1
		list = append(list, x)
	}
	var b strings.Builder
	b.WriteString("# SanalCP managed — bot/rate-limit\n")
	b.WriteString("map $http_user_agent $sanal_bad_bot { default 0; ~*(?:masscan|sqlmap|nikto|nmap|zgrab|gobuster|dirbuster|acunetix|nessus) 1; }\n")
	for _, x := range list {
		// Kapalı profilde de zone'u geçiş boyunca koru: global dosya önce
		// yazılırken eski vhost bu zone'a hâlâ başvuruyor olabilir.
		fmt.Fprintf(&b, "geo $sanal_rl_ip_%d { default 0;\n", x.ID)
		for _, ip := range rlSatirlar(x.IPler) {
			if net.ParseIP(ip) != nil {
				if strings.Contains(ip, ":") {
					ip += "/128"
				} else {
					ip += "/32"
				}
			}
			if _, _, e := net.ParseCIDR(ip); e == nil {
				fmt.Fprintf(&b, "    %s 1;\n", ip)
			}
		}
		b.WriteString("}\n")
		fmt.Fprintf(&b, "map \"$sanal_rl_ip_%d:$uri\" $sanal_rl_key_%d { default $binary_remote_addr; ~^1: \"\";\n", x.ID, x.ID)
		for _, p := range rlSatirlar(x.Yollar) {
			if strings.HasPrefix(p, "/") {
				son := "$"
				if strings.HasSuffix(p, "*") {
					son = ""
				}
				fmt.Fprintf(&b, "    ~^0:%s%s \"\";\n", regexp.QuoteMeta(strings.TrimSuffix(p, "*")), son)
			}
		}
		b.WriteString("}\n")
		fmt.Fprintf(&b, "map \"$sanal_bad_bot:$sanal_rl_key_%d\" $sanal_bot_block_%d { default 0; ~^1:.+ 1; }\n", x.ID, x.ID)
		rate := x.Istek
		if x.Profil == "dengeli" {
			rate = 120
		}
		if x.Profil == "siki" {
			rate = 30
		}
		fmt.Fprintf(&b, "limit_req_zone $sanal_rl_key_%d zone=sanal_rl_%d:64k rate=%dr/m;\n", x.ID, x.ID, rate)
	}
	return rateLimitDosyaDogrula([]byte(b.String()))
}

func rateLimitDosyaDogrula(yeni []byte) error {
	eski, okuErr := os.ReadFile(rateLimitConfPath)
	if err := os.WriteFile(rateLimitConfPath, yeni, 0644); err != nil {
		return err
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		if okuErr == nil {
			_ = os.WriteFile(rateLimitConfPath, eski, 0644)
		} else {
			_ = os.Remove(rateLimitConfPath)
		}
		return fmt.Errorf("nginx -t: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func rlSatirlar(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' })
}

func buildRateLimit(db *sql.DB, domainID int64) string {
	var profil string
	var burst, bot int
	if err := db.QueryRow(`SELECT profil,burst,bot_engelle FROM domain_rate_limits WHERE domain_id=?`, domainID).Scan(&profil, &burst, &bot); err != nil || (profil == "kapali" && bot != 1) {
		return ""
	}
	var b strings.Builder
	b.WriteString("    # ---- Bot ve istek hızı yönetimi ----\n")
	if bot == 1 {
		fmt.Fprintf(&b, "    if ($sanal_bot_block_%d) { return 403; }\n", domainID)
	}
	if profil != "kapali" {
		if burst > 0 {
			fmt.Fprintf(&b, "    limit_req zone=sanal_rl_%d burst=%s nodelay;\n", domainID, strconv.Itoa(burst))
		} else {
			fmt.Fprintf(&b, "    limit_req zone=sanal_rl_%d nodelay;\n", domainID)
		}
		b.WriteString("    limit_req_status 429;\n")
	}
	return b.String()
}
