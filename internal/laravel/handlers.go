package laravel

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"sanalcp/internal/adlar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/jailpath"
)

type Handlers struct{ DB *sql.DB }
type domain struct {
	ID                     int64
	AlanAdi, SK, Home, PHP string
	Demo                   bool
}
type install struct{ Rel, Root string }

var relRE = regexp.MustCompile(`^public_html(?:/[A-Za-z0-9._-]+)*$`)
var envKeyRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)
var secretKeyRE = regexp.MustCompile(`(?i)(KEY|SECRET|PASSWORD|PASS|TOKEN|CREDENTIAL|PRIVATE)`)
var publicEnvKey = map[string]bool{"APP_NAME": true, "APP_ENV": true, "APP_DEBUG": true, "APP_URL": true, "LOG_CHANNEL": true, "LOG_LEVEL": true}

func (h *Handlers) domain(r *http.Request) (domain, error) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var d domain
	var demo int
	err := h.DB.QueryRowContext(r.Context(), `SELECT id,alan_adi,sistem_kullanici,php_surum,COALESCE(is_demo,0) FROM domains WHERE id=?`, id).Scan(&d.ID, &d.AlanAdi, &d.SK, &d.PHP, &demo)
	if err != nil {
		return d, os.ErrNotExist
	}
	d.Demo = demo == 1
	if !adlar.SKGecerli(d.SK) {
		return d, errors.New("geçersiz sistem kullanıcısı")
	}
	d.Home, err = jailpath.TenantHome(d.SK)
	return d, err
}

func temizRel(v string) (string, error) {
	v = strings.Trim(strings.TrimSpace(strings.ReplaceAll(v, "\\", "/")), "/")
	if v == "" {
		v = "public_html"
	}
	v = path.Clean(v)
	if !relRE.MatchString(v) {
		return "", errors.New("geçersiz Laravel dizini")
	}
	return v, nil
}

func (h *Handlers) kurulum(r *http.Request, relRaw string) (domain, install, error) {
	d, err := h.domain(r)
	if err != nil {
		return d, install{}, err
	}
	rel, err := temizRel(relRaw)
	if err != nil {
		return d, install{}, err
	}
	if err = jailpath.DizinDogrula(d.Home, rel); err != nil {
		return d, install{}, errors.New("Laravel dizini bulunamadı veya güvenli değil")
	}
	root := filepath.Join(d.Home, filepath.FromSlash(rel))
	for _, f := range []string{"artisan", "composer.json", "bootstrap/app.php"} {
		st, e := os.Stat(filepath.Join(root, f))
		if e != nil || !st.Mode().IsRegular() {
			return d, install{}, errors.New("Laravel kurulumu bulunamadı")
		}
	}
	return d, install{Rel: rel, Root: root}, nil
}

type Kontrol struct {
	Ad    string `json:"ad"`
	OK    bool   `json:"ok"`
	Detay string `json:"detay"`
}

func (h *Handlers) Durum(w http.ResponseWriter, r *http.Request) {
	d, p, err := h.kurulum(r, r.URL.Query().Get("dizin"))
	if err != nil {
		httpx.WriteError(w, 400, err.Error())
		return
	}
	version, verr := calistir(r.Context(), d, p, 20*time.Second, "artisan", "--version", "--no-ansi")
	_, maintErr := os.Stat(filepath.Join(p.Root, "storage", "framework", "down"))
	storage := yazilabilir(filepath.Join(p.Root, "storage"))
	cache := yazilabilir(filepath.Join(p.Root, "bootstrap", "cache"))
	httpx.WriteJSON(w, 200, map[string]any{
		"kurulu": true, "dizin": p.Rel, "surum": strings.TrimSpace(version), "php": d.PHP, "bakim": maintErr == nil,
		"scheduler": schedulerVarMi(d.SK, p.Root), "queue_worker": queueAktif(d.ID),
		"kontroller": []Kontrol{{"artisan", verr == nil, hataDetayi(verr, "Artisan çalışıyor")}, {"env", dosyaVar(filepath.Join(p.Root, ".env")), ".env dosyası"}, {"storage", storage, "storage yazılabilir"}, {"cache", cache, "bootstrap/cache yazılabilir"}},
	})
}

var artisanAllowed = map[string][]string{
	"about": {"about", "--no-ansi"}, "cache-clear": {"cache:clear", "--no-ansi"}, "config-cache": {"config:cache", "--no-ansi"},
	"config-clear": {"config:clear", "--no-ansi"}, "route-cache": {"route:cache", "--no-ansi"}, "route-clear": {"route:clear", "--no-ansi"},
	"view-cache": {"view:cache", "--no-ansi"}, "view-clear": {"view:clear", "--no-ansi"}, "migrate-status": {"migrate:status", "--no-interaction", "--no-ansi"},
}

func (h *Handlers) Artisan(w http.ResponseWriter, r *http.Request) {
	var req struct{ Dizin, Komut string }
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req) != nil {
		httpx.WriteError(w, 400, "geçersiz istek")
		return
	}
	d, p, err := h.kurulum(r, req.Dizin)
	if err != nil {
		httpx.WriteError(w, 400, err.Error())
		return
	}
	if d.Demo {
		httpx.WriteError(w, 403, "demo aboneliğinde kullanılamaz")
		return
	}
	args, ok := artisanAllowed[req.Komut]
	if !ok {
		httpx.WriteError(w, 400, "izin verilmeyen Artisan komutu")
		return
	}
	out, runErr := calistir(r.Context(), d, p, 2*time.Minute, append([]string{"artisan"}, args...)...)
	httpx.WriteJSON(w, 200, map[string]any{"ok": runErr == nil, "komut": req.Komut, "cikti": son(out, 20000)})
}

func (h *Handlers) Bakim(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dizin string `json:"dizin"`
		Aktif bool   `json:"aktif"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req) != nil {
		httpx.WriteError(w, 400, "geçersiz istek")
		return
	}
	d, p, err := h.kurulum(r, req.Dizin)
	if err != nil {
		httpx.WriteError(w, 400, err.Error())
		return
	}
	if d.Demo {
		httpx.WriteError(w, 403, "demo aboneliğinde kullanılamaz")
		return
	}
	args := []string{"artisan", "up", "--no-ansi"}
	if req.Aktif {
		args = []string{"artisan", "down", "--no-ansi", "--retry=60"}
	}
	out, runErr := calistir(r.Context(), d, p, time.Minute, args...)
	if runErr != nil {
		httpx.WriteError(w, 500, son(out, 2000))
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "bakim": req.Aktif})
}

func (h *Handlers) EnvGet(w http.ResponseWriter, r *http.Request) {
	_, p, err := h.kurulum(r, r.URL.Query().Get("dizin"))
	if err != nil {
		httpx.WriteError(w, 400, err.Error())
		return
	}
	b, err := os.ReadFile(filepath.Join(p.Root, ".env"))
	if err != nil {
		httpx.WriteError(w, 404, ".env okunamadı")
		return
	}
	vals := parseEnv(b)
	out := map[string]any{}
	for k, v := range vals {
		// Fail-closed: yalnız açıkça zararsız sunum ayarlarını göster. Bilinmeyen
		// entegrasyon anahtarları adında SECRET geçmese de kimlik bilgisi olabilir.
		if secretKeyRE.MatchString(k) || !publicEnvKey[k] {
			out[k] = map[string]any{"gizli": true, "dolu": v != ""}
		} else {
			out[k] = map[string]any{"gizli": false, "deger": v}
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"dizin": p.Rel, "degiskenler": out})
}

func (h *Handlers) EnvPut(w http.ResponseWriter, r *http.Request) {
	var req struct{ Dizin, Anahtar, Deger string }
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req) != nil || !envKeyRE.MatchString(req.Anahtar) || strings.ContainsAny(req.Deger, "\r\n\x00") || len(req.Deger) > 8192 {
		httpx.WriteError(w, 400, "geçersiz ortam değişkeni")
		return
	}
	d, p, err := h.kurulum(r, req.Dizin)
	if err != nil {
		httpx.WriteError(w, 400, err.Error())
		return
	}
	if d.Demo {
		httpx.WriteError(w, 403, "demo aboneliğinde kullanılamaz")
		return
	}
	envPath := filepath.Join(p.Root, ".env")
	b, err := os.ReadFile(envPath)
	if err != nil {
		httpx.WriteError(w, 404, ".env okunamadı")
		return
	}
	updated := envSet(b, req.Anahtar, req.Deger)
	tmp, err := os.CreateTemp(p.Root, ".env.sanalcp-*")
	if err != nil {
		httpx.WriteError(w, 500, err.Error())
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	_ = tmp.Chmod(0600)
	_, e1 := tmp.Write(updated)
	e2 := tmp.Sync()
	e3 := tmp.Close()
	if e1 != nil || e2 != nil || e3 != nil {
		httpx.WriteError(w, 500, "geçici .env yazılamadı")
		return
	}
	if err = os.Chown(tmpName, uidOf(envPath), gidOf(envPath)); err != nil {
		httpx.WriteError(w, 500, ".env sahipliği korunamadı")
		return
	}
	if err = os.Rename(tmpName, envPath); err != nil {
		httpx.WriteError(w, 500, ".env atomik güncellenemedi")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "anahtar": req.Anahtar})
}

func calistir(parent context.Context, d domain, p install, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	argv := []string{"-u", d.SK, "--", "/usr/bin/php", filepath.Join(p.Root, args[0])}
	argv = append(argv, args[1:]...)
	cmd := exec.CommandContext(ctx, "runuser", argv...)
	cmd.Dir = p.Root
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + d.Home, "APP_ENV=production"}
	out, err := cmd.CombinedOutput()
	return string(out), err
}
func parseEnv(b []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		s := strings.TrimSpace(sc.Text())
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		k, v, ok := strings.Cut(s, "=")
		k = strings.TrimSpace(k)
		if ok && envKeyRE.MatchString(k) {
			v = strings.TrimSpace(v)
			if unquoted, err := strconv.Unquote(v); err == nil {
				v = unquoted
			} else {
				v = strings.Trim(v, `"'`)
			}
			out[k] = v
		}
	}
	return out
}
func envSet(b []byte, key, value string) []byte {
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	val := strconv.Quote(value)
	found := false
	for i, s := range lines {
		t := strings.TrimSpace(s)
		if strings.HasPrefix(t, key+"=") {
			lines[i] = key + "=" + val
			found = true
		}
	}
	if !found {
		lines = append(lines, key+"="+val)
	}
	return []byte(strings.Join(lines, "\n"))
}
func uidOf(p string) int {
	st, e := os.Stat(p)
	if e != nil {
		return -1
	}
	if x, ok := st.Sys().(*syscall.Stat_t); ok {
		return int(x.Uid)
	}
	return -1
}
func gidOf(p string) int {
	st, e := os.Stat(p)
	if e != nil {
		return -1
	}
	if x, ok := st.Sys().(*syscall.Stat_t); ok {
		return int(x.Gid)
	}
	return -1
}
func dosyaVar(p string) bool { st, e := os.Stat(p); return e == nil && st.Mode().IsRegular() }
func yazilabilir(p string) bool {
	st, e := os.Stat(p)
	return e == nil && st.IsDir() && st.Mode().Perm()&0200 != 0
}
func schedulerVarMi(sk, root string) bool {
	b, e := os.ReadFile("/var/spool/cron/" + sk)
	return e == nil && bytes.Contains(b, []byte(filepath.Join(root, "artisan")+" schedule:run"))
}
func queueAktif(id int64) bool {
	return exec.Command("systemctl", "is-active", "--quiet", queueUnitName(id)).Run() == nil
}
func hataDetayi(err error, ok string) string {
	if err != nil {
		return err.Error()
	}
	return ok
}
func son(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
