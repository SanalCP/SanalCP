// Package appruntime manages one tenant-owned Node.js or Python process behind
// an existing reverse-proxy site. It never executes a user-supplied shell string.
package appruntime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"sanalcp/internal/adlar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/jailpath"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sys/unix"
)

type Handlers struct{ DB *sql.DB }

type Config struct {
	Runtime     string `json:"runtime"`
	Interpreter string `json:"interpreter"`
	Entrypoint  string `json:"entrypoint"`
	HealthPath  string `json:"health_path"`
	Port        int    `json:"port"`
	Enabled     bool   `json:"enabled"`
	ReleaseDir  string `json:"-"`
}

type site struct {
	id          int64
	domain      string
	sk          string
	backend     string
	proxyScheme string
	demo        bool
	suspended   bool
	proxyPort   int
}

var mutationMu sync.Mutex
var entryRE = regexp.MustCompile(`^public_html(?:/[A-Za-z0-9._-]+)*/[A-Za-z0-9._-]+\.(?:js|mjs|cjs|py)$`)
var healthRE = regexp.MustCompile(`^/[A-Za-z0-9/_-]*$`)
var releaseRE = regexp.MustCompile(`^\.sanalcp/releases/[0-9]+/release-[a-f0-9]{12}-[a-f0-9]{16}$`)

func releasePath(s site, rel string) (string, error) {
	if rel == "" {
		return filepath.Join("/home", s.sk, "public_html"), nil
	}
	prefix := ".sanalcp/releases/" + strconv.FormatInt(s.id, 10) + "/"
	if !releaseRE.MatchString(rel) || !strings.HasPrefix(rel, prefix) {
		return "", errors.New("geçersiz sürüm dizini")
	}
	home, err := jailpath.TenantHome(s.sk)
	if err != nil {
		return "", err
	}
	if err := jailpath.DizinDogrula(home, rel); err != nil {
		return "", err
	}
	return filepath.Join(home, rel), nil
}

func healthPath(path string) string {
	if path == "" {
		return "/"
	}
	return path
}

func unitName(id int64) string { return "sanalcp-app-" + strconv.FormatInt(id, 10) + ".service" }
func unitPath(id int64) string { return filepath.Join("/etc/systemd/system", unitName(id)) }

func (h *Handlers) lookup(ctx context.Context, id int64) (site, error) {
	var s site
	var demo, suspended int
	err := h.DB.QueryRowContext(ctx, `SELECT id,alan_adi,sistem_kullanici,COALESCE(web_backend,'php-fpm'),COALESCE(proxy_scheme,'http'),COALESCE(is_demo,0),COALESCE(askida,0),COALESCE(proxy_port,0) FROM domains WHERE id=?`, id).
		Scan(&s.id, &s.domain, &s.sk, &s.backend, &s.proxyScheme, &demo, &suspended, &s.proxyPort)
	if err != nil {
		return s, err
	}
	s.demo = demo == 1
	s.suspended = suspended == 1
	if !adlar.SKGecerli(s.sk) {
		return s, errors.New("geçersiz tenant kullanıcısı")
	}
	return s, nil
}

func validate(s site, c Config) (string, string, error) {
	if s.backend != "reverse-proxy" {
		return "", "", errors.New("yalnız reverse proxy sitelerinde uygulama çalıştırılabilir")
	}
	if s.demo {
		return "", "", errors.New("demo sitesinde uygulama çalıştırılamaz")
	}
	if s.suspended {
		return "", "", errors.New("askıdaki sitede uygulama başlatılamaz")
	}
	if s.proxyScheme != "http" {
		return "", "", errors.New("yerel uygulama için reverse proxy protokolü http olmalı")
	}
	if c.Port != s.proxyPort || c.Port < 1024 || c.Port > 65535 {
		return "", "", errors.New("uygulama portu reverse proxy portuyla aynı olmalı")
	}
	if !entryRE.MatchString(c.Entrypoint) || strings.Contains(c.Entrypoint, "..") {
		return "", "", errors.New("giriş dosyası public_html altında .js/.mjs/.cjs/.py olmalı")
	}
	if (c.Runtime == "node" && !strings.HasSuffix(c.Entrypoint, ".js") && !strings.HasSuffix(c.Entrypoint, ".mjs") && !strings.HasSuffix(c.Entrypoint, ".cjs")) ||
		(c.Runtime == "python" && !strings.HasSuffix(c.Entrypoint, ".py")) {
		return "", "", errors.New("çalışma türü ve giriş dosyası uyuşmuyor")
	}
	if c.Runtime != "node" && c.Runtime != "python" {
		return "", "", errors.New("çalışma türü node veya python olmalı")
	}
	if len(c.HealthPath) > 128 || !healthRE.MatchString(healthPath(c.HealthPath)) {
		return "", "", errors.New("sağlık yolu / ile başlamalı ve yalnız harf, rakam, /, _, - içermeli")
	}
	binary := selectedBinary(c.Runtime, c.Interpreter)
	if binary == "" {
		return "", "", fmt.Errorf("%s sunucuda kurulu değil", c.Runtime)
	}
	home, err := jailpath.TenantHome(s.sk)
	if err != nil {
		return "", "", err
	}
	if err := jailpath.DizinDogrula(home, "public_html"); err != nil {
		return "", "", errors.New("site dizini güvenli değil")
	}
	f, err := jailpath.Ac(home, c.Entrypoint, unix.O_RDONLY, 0)
	if err != nil {
		return "", "", errors.New("giriş dosyası yok veya güvenli değil")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return "", "", errors.New("giriş dosyası normal dosya olmalı")
	}
	return binary, filepath.Join(home, filepath.FromSlash(c.Entrypoint)), nil
}

func unit(s site, c Config, binary, entry string) string {
	home := filepath.Join("/home", s.sk)
	workDir := filepath.Join(home, "public_html")
	if c.ReleaseDir != "" {
		workDir = filepath.Join(home, c.ReleaseDir)
		entry = filepath.Join(workDir, strings.TrimPrefix(c.Entrypoint, "public_html/"))
	}
	if c.Runtime == "python" && (c.Interpreter != "" || c.ReleaseDir != "") {
		binary = venvPython(s, c.Interpreter)
		if c.ReleaseDir != "" {
			binary = filepath.Join(workDir, ".venv", "bin", "python")
		}
	}
	return fmt.Sprintf(`[Unit]
Description=SanalCP app for %s
After=network.target

[Service]
Type=simple
User=%s
Group=%s
Slice=sanal-%s.slice
WorkingDirectory=%s
ExecStart=%s %s
Environment=NODE_ENV=production
Environment=PORT=%d
Environment=HOST=127.0.0.1
Restart=on-failure
RestartSec=5
TimeoutStopSec=30
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=%s

[Install]
WantedBy=multi-user.target
`, s.domain, s.sk, s.sk, s.sk, workDir, binary, entry, c.Port, home)
}

func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".sanalcp-app-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func systemctl(ctx context.Context, args ...string) error {
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cmdCtx, "systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (h *Handlers) read(ctx context.Context, id int64) (Config, bool, error) {
	var c Config
	var enabled int
	err := h.DB.QueryRowContext(ctx, `SELECT runtime,COALESCE(interpreter,''),entrypoint,COALESCE(health_path,'/'),port,enabled,COALESCE(release_dir,'') FROM app_runtimes WHERE domain_id=?`, id).
		Scan(&c.Runtime, &c.Interpreter, &c.Entrypoint, &c.HealthPath, &c.Port, &enabled, &c.ReleaseDir)
	if errors.Is(err, sql.ErrNoRows) {
		return c, false, nil
	}
	if err != nil {
		return c, false, err
	}
	c.Enabled = enabled == 1
	if c.ReleaseDir != "" && (!releaseRE.MatchString(c.ReleaseDir) || !strings.HasPrefix(c.ReleaseDir, ".sanalcp/releases/"+strconv.FormatInt(id, 10)+"/")) {
		return c, false, errors.New("geçersiz sürüm dizini")
	}
	return c, true, nil
}

func active(ctx context.Context, id int64) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return exec.CommandContext(checkCtx, "systemctl", "is-active", "--quiet", unitName(id)).Run() == nil
}

func waitPort(ctx context.Context, port int) error {
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for {
		conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("127.0.0.1:%d dinlenmiyor", port)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func probeHTTP(ctx context.Context, host string, port int, path string) error {
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil}}
	return probeHTTPClient(ctx, client, host, port, path)
}

func probeHTTPClient(ctx context.Context, client *http.Client, host string, port int, path string) error {
	if port < 1024 || port > 65535 {
		return errors.New("geçersiz port")
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(checkCtx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+healthPath(path), nil)
	if err != nil {
		return err
	}
	request.Host = host
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("sağlık yanıtı HTTP %d", response.StatusCode)
	}
	return nil
}

func waitHTTP(ctx context.Context, id int64, host string, port int, path string) error {
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	var last error
	for {
		if active(ctx, id) {
			last = probeHTTP(ctx, host, port, path)
			if last == nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			if last != nil {
				return last
			}
			return errors.New("uygulama servisi çalışmıyor")
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	s, err := h.lookup(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, 404, "site bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, 500, "site okunamadı")
		return
	}
	if s.backend != "reverse-proxy" {
		httpx.WriteError(w, 409, "reverse proxy sitesi değil")
		return
	}
	c, exists, err := h.read(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, 500, "uygulama okunamadı")
		return
	}
	nodeBin, pythonBin := runtimeBinary("node"), runtimeBinary("python")
	isActive := exists && active(r.Context(), id)
	httpx.WriteJSON(w, 200, map[string]any{"configured": exists, "config": c, "active": isActive, "healthy": isActive && probeHTTP(r.Context(), s.domain, c.Port, healthPath(c.HealthPath)) == nil, "proxy_port": s.proxyPort, "active_release": c.ReleaseDir,
		"node_available": nodeBin != "", "python_available": pythonBin != "",
		"node_version": runtimeVersion(r.Context(), nodeBin), "python_version": runtimeVersion(r.Context(), pythonBin),
		"interpreters": availableInterpreters(r.Context())})
}

func runtimeBinary(runtime string) string {
	paths := binaryCandidates(runtime)
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

func binaryCandidates(runtime string) []string {
	var candidates []string
	switch runtime {
	case "node":
		candidates = []string{"/usr/bin/node", "/usr/local/bin/node"}
	case "python":
		candidates = []string{"/usr/bin/python3", "/usr/local/bin/python3"}
	default:
		return nil
	}
	var found []string
	for _, path := range candidates {
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() && st.Mode()&0111 != 0 {
			found = append(found, path)
		}
	}
	return found
}

func selectedBinary(runtime, path string) string {
	for _, candidate := range binaryCandidates(runtime) {
		if path == "" || path == candidate {
			return candidate
		}
	}
	return ""
}

type interpreterOption struct {
	Runtime string `json:"runtime"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

func availableInterpreters(ctx context.Context) []interpreterOption {
	options := make([]interpreterOption, 0)
	for _, kind := range []string{"node", "python"} {
		for _, path := range binaryCandidates(kind) {
			options = append(options, interpreterOption{kind, path, runtimeVersion(ctx, path)})
		}
	}
	return options
}

func runtimeVersion(ctx context.Context, binary string) string {
	if binary == "" {
		return ""
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(checkCtx, binary, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	version := strings.TrimSpace(string(out))
	if len(version) > 80 {
		version = version[:80]
	}
	return version
}

func (h *Handlers) Put(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var c Config
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&c); err != nil {
		httpx.WriteError(w, 400, "geçersiz istek")
		return
	}
	mutationMu.Lock()
	defer mutationMu.Unlock()
	s, err := h.lookup(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, 404, "site bulunamadı")
		return
	}
	binary, entry, err := validate(s, c)
	if err != nil {
		httpx.WriteError(w, 400, err.Error())
		return
	}
	if c.Interpreter == "" {
		c.Interpreter = binary
	}
	c.HealthPath = healthPath(c.HealthPath)
	old, existed, err := h.read(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, 500, "eski yapılandırma okunamadı")
		return
	}
	if existed && old.ReleaseDir != "" {
		if old.Runtime != c.Runtime || old.Interpreter != c.Interpreter || old.Entrypoint != c.Entrypoint {
			if old.Enabled {
				httpx.WriteError(w, 409, "aktif Git sürümünde çalışma ortamı veya giriş dosyası değiştirilemez; önce uygulamayı durdurun")
				return
			}
		} else {
			c.ReleaseDir = old.ReleaseDir
		}
	}
	if !existed || old.Port != c.Port {
		var used int
		if err := h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM app_runtimes WHERE port=? AND domain_id<>?`, c.Port, id).Scan(&used); err != nil || used != 0 {
			httpx.WriteError(w, 409, "port başka bir uygulamada kullanılıyor")
			return
		}
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Port)))
		if err != nil {
			httpx.WriteError(w, 409, "port zaten kullanımda")
			return
		}
		ln.Close()
	}
	if c.Runtime == "python" && c.ReleaseDir == "" {
		if err := ensureVenv(r.Context(), s, binary); err != nil {
			httpx.WriteError(w, 500, "Python sanal ortamı oluşturulamadı: "+err.Error())
			return
		}
	}
	path := unitPath(id)
	oldBody, oldErr := os.ReadFile(path)
	if oldErr != nil && !os.IsNotExist(oldErr) {
		httpx.WriteError(w, 500, "servis dosyası okunamadı")
		return
	}
	if err := atomicWrite(path, []byte(unit(s, c, binary, entry))); err != nil {
		httpx.WriteError(w, 500, "servis dosyası yazılamadı")
		return
	}
	rollback := func() {
		_ = systemctl(context.Background(), "disable", "--now", unitName(id))
		if oldErr == nil {
			_ = atomicWrite(path, oldBody)
		} else {
			_ = os.Remove(path)
		}
		_ = systemctl(context.Background(), "daemon-reload")
		if oldErr == nil && old.Enabled {
			_ = systemctl(context.Background(), "enable", "--now", unitName(id))
		}
	}
	if err := systemctl(r.Context(), "daemon-reload"); err != nil {
		rollback()
		httpx.WriteError(w, 500, err.Error())
		return
	}
	cmd := "disable"
	if c.Enabled {
		cmd = "enable"
	}
	if err := systemctl(r.Context(), cmd, "--now", unitName(id)); err != nil {
		rollback()
		httpx.WriteError(w, 500, err.Error())
		return
	}
	if c.Enabled && existed {
		if err := systemctl(r.Context(), "restart", unitName(id)); err != nil {
			rollback()
			httpx.WriteError(w, 500, err.Error())
			return
		}
	}
	if c.Enabled {
		if err := waitPort(r.Context(), c.Port); err != nil {
			rollback()
			httpx.WriteError(w, 500, "uygulama yerel portta başlatılamadı: "+err.Error())
			return
		}
		if !active(r.Context(), id) {
			rollback()
			httpx.WriteError(w, 500, "uygulama servisi çalışmıyor")
			return
		}
		if err := waitHTTP(r.Context(), id, s.domain, c.Port, c.HealthPath); err != nil {
			rollback()
			httpx.WriteError(w, 500, "uygulama sağlık kontrolünden geçemedi: "+err.Error())
			return
		}
	}
	_, err = h.DB.ExecContext(r.Context(), `INSERT INTO app_runtimes(domain_id,runtime,interpreter,entrypoint,health_path,port,enabled,release_dir) VALUES(?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE runtime=VALUES(runtime),interpreter=VALUES(interpreter),entrypoint=VALUES(entrypoint),health_path=VALUES(health_path),port=VALUES(port),enabled=VALUES(enabled),release_dir=VALUES(release_dir)`, id, c.Runtime, c.Interpreter, c.Entrypoint, c.HealthPath, c.Port, c.Enabled, c.ReleaseDir)
	if err != nil {
		rollback()
		httpx.WriteError(w, 500, "uygulama kaydı yazılamadı")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "active": c.Enabled && active(r.Context(), id)})
}

func (h *Handlers) Action(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	action := chi.URLParam(r, "action")
	if action != "start" && action != "stop" && action != "restart" {
		httpx.WriteError(w, 400, "geçersiz işlem")
		return
	}
	mutationMu.Lock()
	defer mutationMu.Unlock()
	s, err := h.lookup(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, 404, "site bulunamadı")
		return
	}
	if s.suspended && action != "stop" {
		httpx.WriteError(w, 409, "askıdaki sitede uygulama başlatılamaz")
		return
	}
	if s.proxyScheme != "http" && action != "stop" {
		httpx.WriteError(w, 409, "yerel uygulama için reverse proxy protokolü http olmalı")
		return
	}
	c, exists, err := h.read(r.Context(), id)
	if err != nil || !exists {
		httpx.WriteError(w, 404, "uygulama yapılandırılmamış")
		return
	}
	command := []string{"disable", "--now", unitName(id)}
	if action == "start" || action == "restart" {
		command = []string{"enable", "--now", unitName(id)}
	}
	restore := func() {
		if c.Enabled {
			_ = systemctl(context.Background(), "enable", "--now", unitName(id))
		} else {
			_ = systemctl(context.Background(), "disable", "--now", unitName(id))
		}
	}
	if err := systemctl(r.Context(), command...); err != nil {
		restore()
		httpx.WriteError(w, 500, err.Error())
		return
	}
	if action == "restart" {
		if err := systemctl(r.Context(), "restart", unitName(id)); err != nil {
			restore()
			httpx.WriteError(w, 500, err.Error())
			return
		}
	}
	if action == "start" || action == "restart" {
		if waitHTTP(r.Context(), id, s.domain, c.Port, c.HealthPath) != nil {
			restore()
			httpx.WriteError(w, 500, "uygulama sağlık kontrolünden geçemedi")
			return
		}
	}
	enabled := action != "stop"
	if _, err := h.DB.ExecContext(r.Context(), `UPDATE app_runtimes SET enabled=? WHERE domain_id=?`, enabled, id); err != nil {
		restore()
		httpx.WriteError(w, 500, "durum kaydedilemedi")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "active": active(r.Context(), id)})
}

func (h *Handlers) Logs(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := h.lookup(r.Context(), id); err != nil {
		httpx.WriteError(w, 404, "site bulunamadı")
		return
	}
	_, exists, err := h.read(r.Context(), id)
	if err != nil || !exists {
		httpx.WriteError(w, 404, "uygulama yapılandırılmamış")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "journalctl", "-u", unitName(id), "-n", "100", "--no-pager", "--output=short-iso").CombinedOutput()
	if err != nil {
		httpx.WriteError(w, 500, "uygulama günlüğü okunamadı")
		return
	}
	if len(out) > 64<<10 {
		out = out[len(out)-(64<<10):]
	}
	httpx.WriteJSON(w, 200, map[string]any{"logs": string(out)})
}

// Remove stops a managed process before the tenant account is deleted.
func Remove(ctx context.Context, id int64) error {
	mutationMu.Lock()
	defer mutationMu.Unlock()
	path := unitPath(id)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		_ = os.Remove(pendingPath(id))
		return nil
	} else if err != nil {
		return err
	}
	if err := systemctl(ctx, "disable", "--now", unitName(id)); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	if err := os.Remove(pendingPath(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return systemctl(ctx, "daemon-reload")
}

// Suspend disables the unit during site suspension without changing the
// user's saved enabled setting. Unsuspend restores only enabled applications.
func Suspend(ctx context.Context, db *sql.DB, id int64, suspended bool) error {
	mutationMu.Lock()
	defer mutationMu.Unlock()
	if _, err := os.Lstat(unitPath(id)); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if suspended {
		return systemctl(ctx, "disable", "--now", unitName(id))
	}
	var enabled int
	err := db.QueryRowContext(ctx, `SELECT enabled FROM app_runtimes WHERE domain_id=?`, id).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if enabled == 1 {
		return systemctl(ctx, "enable", "--now", unitName(id))
	}
	return nil
}
