package appruntime

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"sanalcp/internal/httpx"
	"sanalcp/internal/jailpath"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sys/unix"
)

func venvPython(s site, interpreter string) string {
	sum := sha256.Sum256([]byte(interpreter))
	return filepath.Join("/home", s.sk, ".sanalcp", "venv-"+strconv.FormatInt(s.id, 10)+"-"+hex.EncodeToString(sum[:4]), "bin", "python")
}

type cappedOutput struct{ data []byte }

func (o *cappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if len(o.data) < 16<<10 {
		remain := (16 << 10) - len(o.data)
		if n < remain {
			remain = n
		}
		o.data = append(o.data, p[:remain]...)
	}
	return n, nil
}

// tenantCommand runs package hooks with only the tenant's rights and a clean
// environment. Killing the process group also stops npm/pip child processes.
func tenantCommand(parent context.Context, s site, cwd string, timeout time.Duration, argv ...string) (string, error) {
	home, err := jailpath.TenantHome(s.sk)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "runuser", append([]string{"-u", s.sk, "--"}, argv...)...)
	cmd.Dir = cwd
	path := "/usr/local/bin:/usr/bin:/bin"
	if len(argv) > 0 && filepath.IsAbs(argv[0]) {
		path = filepath.Dir(argv[0]) + ":" + path
	}
	cmd.Env = []string{"HOME=" + home, "PATH=" + path, "LANG=C.UTF-8", "PIP_DISABLE_PIP_VERSION_CHECK=1", "NPM_CONFIG_UPDATE_NOTIFIER=false"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	var output cappedOutput
	cmd.Stdout, cmd.Stderr = &output, &output
	err = cmd.Run()
	if ctx.Err() != nil {
		return string(output.data), ctx.Err()
	}
	return string(output.data), err
}

func ensureVenv(ctx context.Context, s site, interpreter string) error {
	return ensureVenvAt(ctx, s, interpreter, filepath.Dir(filepath.Dir(venvPython(s, interpreter))))
}

func ensureVenvAt(ctx context.Context, s site, interpreter, base string) error {
	python := filepath.Join(base, "bin", "python")
	if st, err := os.Stat(python); err == nil && st.Mode().IsRegular() {
		return nil
	}
	home, err := jailpath.TenantHome(s.sk)
	if err != nil {
		return err
	}
	_, err = tenantCommand(ctx, s, home, 2*time.Minute, "mkdir", "-p", filepath.Dir(base))
	if err != nil {
		return err
	}
	out, err := tenantCommand(ctx, s, home, 2*time.Minute, interpreter, "-m", "venv", base)
	if err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}

func regularManifest(home, path string) bool {
	f, err := jailpath.Ac(home, path, unix.O_RDONLY, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	return err == nil && st.Mode().IsRegular()
}

func (h *Handlers) InstallDependencies(w http.ResponseWriter, r *http.Request) {
	installCtx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	mutationMu.Lock()
	defer mutationMu.Unlock()
	s, err := h.lookup(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, 404, "site bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, 500, "site okunamadı")
		return
	}
	if s.backend != "reverse-proxy" || s.suspended || s.demo {
		httpx.WriteError(w, 409, "bu sitede bağımlılık kurulamaz")
		return
	}
	c, exists, err := h.read(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, 500, "uygulama okunamadı")
		return
	}
	if !exists {
		httpx.WriteError(w, 404, "uygulama yapılandırılmamış")
		return
	}
	if c.Interpreter == "" {
		httpx.WriteError(w, 409, "önce uygulama ayarını yeniden kaydedip yorumlayıcıyı sabitleyin")
		return
	}
	if c.ReleaseDir != "" {
		httpx.WriteError(w, 409, "Git sürümünün bağımlılıkları yerinde değiştirilemez; yeni sürüm için Git Pull kullanın")
		return
	}
	interpreter := selectedBinary(c.Runtime, c.Interpreter)
	if interpreter == "" {
		httpx.WriteError(w, 409, "seçili yorumlayıcı artık kurulu değil")
		return
	}
	home, err := jailpath.TenantHome(s.sk)
	if err != nil {
		httpx.WriteError(w, 500, "tenant dizini bulunamadı")
		return
	}
	rel := "public_html"
	if err := jailpath.DizinDogrula(home, rel); err != nil {
		httpx.WriteError(w, 409, "site dizini güvenli değil")
		return
	}
	cwd := filepath.Join(home, rel)
	var argv []string
	switch c.Runtime {
	case "node":
		if !regularManifest(home, filepath.Join(rel, "package.json")) || !regularManifest(home, filepath.Join(rel, "package-lock.json")) {
			httpx.WriteError(w, 400, "package.json ve package-lock.json gerekli")
			return
		}
		npm := filepath.Join(filepath.Dir(interpreter), "npm")
		if st, err := os.Stat(npm); err != nil || !st.Mode().IsRegular() || st.Mode()&0111 == 0 {
			httpx.WriteError(w, 409, "seçili Node.js için npm bulunamadı")
			return
		}
		argv = []string{npm, "ci", "--omit=dev", "--no-audit", "--no-fund"}
	case "python":
		if !regularManifest(home, filepath.Join(rel, "requirements.txt")) {
			httpx.WriteError(w, 400, "requirements.txt gerekli")
			return
		}
		venv := filepath.Dir(filepath.Dir(venvPython(s, interpreter)))
		if err := ensureVenvAt(installCtx, s, interpreter, venv); err != nil {
			httpx.WriteError(w, 500, "Python sanal ortamı oluşturulamadı: "+err.Error())
			return
		}
		argv = []string{filepath.Join(venv, "bin", "python"), "-m", "pip", "install", "--requirement", "requirements.txt"}
	default:
		httpx.WriteError(w, 400, "geçersiz çalışma türü")
		return
	}
	out, err := tenantCommand(installCtx, s, cwd, 5*time.Minute, argv...)
	if err != nil {
		httpx.WriteError(w, 500, fmt.Sprintf("bağımlılık kurulamadı: %v\n%s", err, out))
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "output": out})
}
