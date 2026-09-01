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
	Domain       string `json:"domain"`
	Hesap        string `json:"hesap"`
	PHPSurum     string `json:"php_version,omitempty"`
	HedefteVarMi bool   `json:"target_exists,omitempty"`
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
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
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
	httpx.WriteJSON(w, 200, env)
}

const uzakKesifKomutu = `if command -v sanalcp-transfer-export >/dev/null 2>&1; then sanalcp-transfer-export inventory; elif [ -d /var/cpanel/users ]; then echo 'PROVIDER=cpanel'; /usr/local/cpanel/cpanel -V 2>/dev/null | head -1 | sed 's/^/VERSION=/'; find /var/cpanel/users -maxdepth 1 -type f -print 2>/dev/null | while read f; do u=${f##*/}; sed -n 's/^DNS=//p' "$f" | while read d; do echo "SITE=$d|$u"; done; done; elif command -v plesk >/dev/null 2>&1; then echo 'PROVIDER=plesk'; plesk version 2>/dev/null | head -1 | sed 's/^/VERSION=/'; plesk bin domain --list 2>/dev/null | while read d; do echo "SITE=$d|$d"; done; elif [ -x /usr/local/directadmin/directadmin ]; then echo 'PROVIDER=directadmin'; /usr/local/directadmin/directadmin v 2>/dev/null | head -1 | sed 's/^/VERSION=/'; find /usr/local/directadmin/data/users -name domains.list -type f -print 2>/dev/null | while read f; do u=${f%/domains.list}; u=${u##*/}; while read d; do echo "SITE=$d|$u"; done < "$f"; done; else echo 'PROVIDER=unknown'; fi`

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
	}
	sort.Strings(env.Domainler)
	sort.Slice(env.Siteler, func(i, j int) bool { return env.Siteler[i].Domain < env.Siteler[j].Domain })
	if env.Provider == "" || env.Provider == "unknown" {
		return env, errors.New("desteklenen panel bulunamadı")
	}
	return env, nil
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
