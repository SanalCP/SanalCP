package transfers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"sanalcp/internal/httpx"
	"sanalcp/internal/phpsurum"
)

var sshBin = "ssh"
var uzakHostRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
var uzakUserRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

type uzakKesifReq struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Kullanici string `json:"kullanici"`
}
type UzakEnvanter struct {
	Provider  string     `json:"provider"`
	Surum     string     `json:"surum"`
	Domainler []string   `json:"domainler"`
	Siteler   []UzakSite `json:"siteler"`
}
type UzakSite struct {
	Domain         string   `json:"domain"`
	Hesap          string   `json:"hesap"`
	PHPSurum       string   `json:"php_version,omitempty"`
	Uygulama       string   `json:"application,omitempty"`
	KaynakModuller []string `json:"source_modules,omitempty"`
	EksikModuller  []string `json:"missing_modules,omitempty"`
	Tasinabilir    bool     `json:"transferable"`
	KontrolDurumu  string   `json:"check_status"`
	Engeller       []string `json:"blockers,omitempty"`
	Uyarilar       []string `json:"warnings,omitempty"`
	HedefteVarMi   bool     `json:"target_exists,omitempty"`
}

// RemoteDiscover yalnız anahtar tabanlı, BatchMode ve doğrulanmış known_hosts ile
// bağlanır. Parola API'ye hiç girmez; ilk bağlantıda host anahtarı körlemesine kabul edilmez.
func (h *Handlers) RemoteDiscover(w http.ResponseWriter, r *http.Request) {
	var req uzakKesifReq
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		httpx.WriteError(w, 400, "geçersiz gövde")
		return
	}
	req.Host = strings.TrimSpace(req.Host)
	req.Kullanici = strings.TrimSpace(req.Kullanici)
	if req.Kullanici == "" {
		req.Kullanici = "root"
	}
	if req.Port == 0 {
		req.Port = 22
	}
	if !uzakHostGecerli(req.Host) || !uzakUserRe.MatchString(req.Kullanici) || req.Port < 1 || req.Port > 65535 {
		httpx.WriteError(w, 400, "geçersiz SSH hedefi")
		return
	}
	// Uygulama ve PHP modülü ön kontrolü çok sayıda hesapta salt domain
	// listesinden daha uzun sürebilir.
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	env, err := uzakKesfet(ctx, req)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		httpx.WriteError(w, status, "SSH keşfi başarısız: "+err.Error())
		return
	}
	rows, dbErr := h.DB.QueryContext(r.Context(), `SELECT LOWER(alan_adi) FROM domains`)
	if dbErr != nil {
		httpx.WriteError(w, 500, "hedef domain envanteri okunamadı")
		return
	}
	hedefler := map[string]bool{}
	for rows.Next() {
		var domain string
		if rows.Scan(&domain) == nil {
			hedefler[domain] = true
		}
	}
	_ = rows.Close()
	for i := range env.Siteler {
		env.Siteler[i].HedefteVarMi = hedefler[env.Siteler[i].Domain]
	}
	hedefPHP := hedefPHPEnvanteri(r.Context())
	for i := range env.Siteler {
		onKontrol(&env.Siteler[i], hedefPHP)
	}
	httpx.WriteJSON(w, 200, env)
}

// PREFLIGHT satırları kaynak sunucuda yalnızca salt-okunur dosya/PHP sorguları ile
// üretilir. Eski sanalcp-transfer-export sürümleri de bu sarmalayıcı sayesinde
// ön kontrol bilgisi sağlayabilir.
const uzakKesifKomutu = `t=$(mktemp /tmp/sanalcp-discover.XXXXXX) || exit 1; trap 'rm -f "$t"' EXIT; if command -v sanalcp-transfer-export >/dev/null 2>&1; then sanalcp-transfer-export inventory >"$t"; elif [ -d /var/cpanel/users ]; then echo 'PROVIDER=cpanel' >"$t"; /usr/local/cpanel/cpanel -V 2>/dev/null | head -1 | sed 's/^/VERSION=/' >>"$t"; find /var/cpanel/users -maxdepth 1 -type f -print 2>/dev/null | while read f; do u=${f##*/}; sed -n 's/^DNS=//p' "$f" | while read d; do pv=$(sed -n 's/^[[:space:]]*phpversion:[[:space:]]*//p' "/var/cpanel/userdata/$u/$d" 2>/dev/null | head -1 | sed -E 's/[^0-9]*([0-9])([0-9]).*/\1.\2/'); echo "SITE=$d|$u|$pv"; done; done >>"$t"; elif command -v plesk >/dev/null 2>&1; then echo 'PROVIDER=plesk' >"$t"; plesk version 2>/dev/null | head -1 | sed 's/^/VERSION=/' >>"$t"; plesk bin domain --list 2>/dev/null | while read d; do echo "SITE=$d|$d"; done >>"$t"; elif [ -x /usr/local/directadmin/directadmin ]; then echo 'PROVIDER=directadmin' >"$t"; /usr/local/directadmin/directadmin v 2>/dev/null | head -1 | sed 's/^/VERSION=/' >>"$t"; find /usr/local/directadmin/data/users -name domains.list -type f -print 2>/dev/null | while read f; do u=${f%/domains.list}; u=${u##*/}; while read d; do echo "SITE=$d|$u"; done < "$f"; done >>"$t"; else echo 'PROVIDER=unknown' >"$t"; fi; cat "$t"; while IFS='|' read -r tag u pv rest; do case "$tag" in SITE=*) d=${tag#SITE=};; *) continue;; esac; root="/home/$u/public_html"; app=unknown; req="curl,fileinfo,json,mbstring,openssl"; if [ -f "$root/wp-config.php" ]; then app=wordpress; req="$req,dom,exif,gd,intl,mysqli,zip"; elif [ -f "$root/artisan" ]; then app=laravel; req="$req,ctype,dom,pdo,pdo_mysql,tokenizer,xml"; elif [ -f "$root/bin/console" ]; then app=symfony; req="$req,ctype,dom,intl,pdo,pdo_mysql,tokenizer,xml"; elif [ -f "$root/config/settings.inc.php" ] || [ -f "$root/app/config/parameters.php" ]; then app=prestashop; req="$req,dom,gd,intl,mysqli,zip"; elif [ -f "$root/config.php" ] && grep -qE 'const[[:space:]]+DB_(NAME|USER|PASS)' "$root/config.php"; then app=custom-php; req="$req,pdo,pdo_mysql"; fi; bin=""; n=$(printf %s "$pv" | tr -d .); for b in "/opt/remi/php$n/root/usr/bin/php" "/usr/local/bin/ea-php$n" "/usr/bin/php$pv"; do [ -x "$b" ] && bin=$b && break; done; [ -n "$bin" ] || { command -v php >/dev/null 2>&1 && bin=$(command -v php); }; mods=""; [ -n "$bin" ] && mods=$($bin -m 2>/dev/null | tr '[:upper:]' '[:lower:]' | sed '/^\[/d;/^$/d' | sort -u | paste -sd, -); echo "PREFLIGHT=$d|$pv|$app|$req|$mods"; done <"$t"`

const maxKesifCiktisi = 2 << 20

func uzakKesfet(ctx context.Context, req uzakKesifReq) (UzakEnvanter, error) {
	var out sinirliKesifCiktisi
	out.max = maxKesifCiktisi
	args := append(uzakSSHArgs(req.Port), req.Kullanici+"@"+req.Host, uzakKesifKomutu)
	cmd := exec.CommandContext(ctx, sshBin, args...)
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if out.asildi {
		return UzakEnvanter{}, errors.New("keşif çıktısı çok büyük")
	}
	if err != nil {
		msg := strings.TrimSpace(out.buf.String())
		if len(msg) > 500 {
			msg = msg[:500]
		}
		if msg == "" {
			msg = err.Error()
		}
		return UzakEnvanter{}, errors.New(msg)
	}
	raw := out.buf.Bytes()
	env := UzakEnvanter{Domainler: []string{}, Siteler: []UzakSite{}}
	seen := map[string]bool{}
	preflight := map[string][]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "PROVIDER=") {
			env.Provider = strings.TrimSpace(strings.TrimPrefix(line, "PROVIDER="))
		}
		if strings.HasPrefix(line, "VERSION=") {
			env.Surum = strings.TrimSpace(strings.TrimPrefix(line, "VERSION="))
		}
		if strings.HasPrefix(line, "SITE=") {
			parca := strings.SplitN(strings.TrimPrefix(line, "SITE="), "|", 3)
			if len(parca) < 2 {
				continue
			}
			d := strings.ToLower(strings.TrimSpace(parca[0]))
			hesap := strings.TrimSpace(parca[1])
			if domainKesifGecerli(d) && uzakUserRe.MatchString(hesap) && !seen[d] {
				seen[d] = true
				env.Domainler = append(env.Domainler, d)
				phpSurum := ""
				if len(parca) == 3 && regexp.MustCompile(`^[0-9]+\.[0-9]+$`).MatchString(strings.TrimSpace(parca[2])) {
					phpSurum = strings.TrimSpace(parca[2])
				}
				env.Siteler = append(env.Siteler, UzakSite{Domain: d, Hesap: hesap, PHPSurum: phpSurum})
			}
		}
		if strings.HasPrefix(line, "PREFLIGHT=") {
			p := strings.SplitN(strings.TrimPrefix(line, "PREFLIGHT="), "|", 5)
			if len(p) == 5 && domainKesifGecerli(strings.ToLower(strings.TrimSpace(p[0]))) {
				preflight[strings.ToLower(strings.TrimSpace(p[0]))] = p[1:]
			}
		}
	}
	for i := range env.Siteler {
		if p, ok := preflight[env.Siteler[i].Domain]; ok {
			if regexp.MustCompile(`^[0-9]+\.[0-9]+$`).MatchString(strings.TrimSpace(p[0])) {
				env.Siteler[i].PHPSurum = strings.TrimSpace(p[0])
			}
			env.Siteler[i].Uygulama = temizUygulama(p[1])
			env.Siteler[i].KaynakModuller = temizModuller(p[3])
			// Uygulamanın ihtiyaç listesi yalnız kaynakta gerçekten yüklüyse talep
			// edilir; böylece tahmini uygulama tespiti yanlış engel üretmez.
			yuklu := modulKumesi(env.Siteler[i].KaynakModuller)
			for _, m := range temizModuller(p[2]) {
				if yuklu[m] {
					env.Siteler[i].Uyarilar = append(env.Siteler[i].Uyarilar, "requires:"+m)
				}
			}
		}
	}
	sort.Strings(env.Domainler)
	sort.Slice(env.Siteler, func(i, j int) bool { return env.Siteler[i].Domain < env.Siteler[j].Domain })
	if env.Provider == "" || env.Provider == "unknown" {
		return env, errors.New("desteklenen panel bulunamadı")
	}
	return env, nil
}

type phpHedef struct {
	yuklu    bool
	moduller map[string]bool
}

func hedefPHPEnvanteri(ctx context.Context) map[string]phpHedef {
	sonuc := map[string]phpHedef{}
	for _, s := range phpsurum.TumSurumler() {
		if !s.Yuklu || s.PHPBin == "" {
			continue
		}
		if _, varMi := sonuc[s.Surum]; varMi {
			continue
		}
		mods := map[string]bool{}
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		out, err := exec.CommandContext(cctx, s.PHPBin, "-m").Output()
		cancel()
		if err == nil {
			mods = modulKumesi(temizModuller(string(out)))
		}
		sonuc[s.Surum] = phpHedef{yuklu: true, moduller: mods}
	}
	return sonuc
}

func onKontrol(s *UzakSite, hedef map[string]phpHedef) {
	s.KontrolDurumu = "incompatible"
	if s.HedefteVarMi {
		s.Engeller = append(s.Engeller, "Domain hedef sunucuda zaten var")
	}
	if s.PHPSurum == "" {
		s.Engeller = append(s.Engeller, "Kaynak PHP sürümü belirlenemedi")
	} else if h, ok := hedef[s.PHPSurum]; !ok || !h.yuklu {
		s.Engeller = append(s.Engeller, "Hedefte PHP "+s.PHPSurum+" kurulu değil")
	} else {
		for _, u := range s.Uyarilar {
			if strings.HasPrefix(u, "requires:") {
				m := strings.TrimPrefix(u, "requires:")
				if !h.moduller[m] {
					s.EksikModuller = append(s.EksikModuller, m)
				}
			}
		}
	}
	if len(s.EksikModuller) > 0 {
		s.Engeller = append(s.Engeller, "Eksik PHP modülleri: "+strings.Join(s.EksikModuller, ", "))
	}
	if len(s.KaynakModuller) == 0 {
		s.Engeller = append(s.Engeller, "Kaynak PHP modülleri doğrulanamadı")
	}
	filtre := s.Uyarilar[:0]
	for _, u := range s.Uyarilar {
		if !strings.HasPrefix(u, "requires:") {
			filtre = append(filtre, u)
		}
	}
	s.Uyarilar = filtre
	if len(s.Engeller) == 0 {
		s.Tasinabilir = true
		s.KontrolDurumu = "compatible"
	}
}

func temizUygulama(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "wordpress", "laravel", "symfony", "prestashop", "custom-php":
		return s
	}
	return "unknown"
}
func temizModuller(s string) []string {
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n", ",")
	seen := map[string]bool{}
	out := []string{}
	for _, m := range strings.Split(s, ",") {
		m = strings.ToLower(strings.TrimSpace(m))
		if regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(m) && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}
func modulKumesi(ms []string) map[string]bool {
	r := map[string]bool{}
	for _, m := range ms {
		r[m] = true
	}
	return r
}

type sinirliKesifCiktisi struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	max    int
	asildi bool
}

func (s *sinirliKesifCiktisi) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.asildi {
		return 0, errors.New("keşif çıktısı çok büyük")
	}
	kalan := s.max - s.buf.Len()
	if len(p) > kalan {
		if kalan > 0 {
			_, _ = s.buf.Write(p[:kalan])
		}
		s.asildi = true
		return kalan, errors.New("keşif çıktısı çok büyük")
	}
	return s.buf.Write(p)
}
func uzakHostGecerli(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	return len(s) <= 253 && uzakHostRe.MatchString(s) && !strings.Contains(s, "..")
}
func domainKesifGecerli(s string) bool {
	return len(s) > 3 && len(s) <= 253 && strings.Contains(s, ".") && uzakHostRe.MatchString(s)
}
