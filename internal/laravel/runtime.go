package laravel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"sanalcp/internal/httpx"
)

type runtimeReq struct {
	Dizin   string `json:"dizin"`
	Aktif   bool   `json:"aktif"`
	Queue   string `json:"queue"`
	Deneme  int    `json:"tries"`
	Timeout int    `json:"timeout"`
}

func (h *Handlers) Scheduler(w http.ResponseWriter, r *http.Request) {
	var q runtimeReq
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&q) != nil {
		httpx.WriteError(w, 400, "geçersiz istek")
		return
	}
	d, p, e := h.kurulum(r, q.Dizin)
	if e != nil {
		httpx.WriteError(w, 400, e.Error())
		return
	}
	if d.Demo {
		httpx.WriteError(w, 403, "demo aboneliğinde kullanılamaz")
		return
	}
	old, _ := exec.Command("crontab", "-u", d.SK, "-l").Output()
	next := schedulerCron(string(old), p.Root, q.Aktif)
	cmd := exec.Command("crontab", "-u", d.SK, "-")
	cmd.Stdin = strings.NewReader(next)
	if out, e := cmd.CombinedOutput(); e != nil {
		httpx.WriteError(w, 500, "crontab güncellenemedi: "+strings.TrimSpace(string(out)))
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "aktif": q.Aktif})
}

func schedulerCron(old, root string, aktif bool) string {
	marker := "# sanalcp-laravel-scheduler " + root
	lines := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(old, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" || strings.Contains(line, marker) {
			continue
		}
		lines = append(lines, line)
	}
	if aktif {
		lines = append(lines, "* * * * * /usr/bin/php "+filepath.Join(root, "artisan")+" schedule:run >> /dev/null 2>&1 "+marker)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (h *Handlers) Queue(w http.ResponseWriter, r *http.Request) {
	var q runtimeReq
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&q) != nil {
		httpx.WriteError(w, 400, "geçersiz istek")
		return
	}
	d, p, e := h.kurulum(r, q.Dizin)
	if e != nil {
		httpx.WriteError(w, 400, e.Error())
		return
	}
	if d.Demo {
		httpx.WriteError(w, 403, "demo aboneliğinde kullanılamaz")
		return
	}
	if q.Queue == "" {
		q.Queue = "default"
	}
	if !queueRE.MatchString(q.Queue) {
		httpx.WriteError(w, 400, "geçersiz queue adı")
		return
	}
	if q.Deneme < 1 || q.Deneme > 20 {
		q.Deneme = 3
	}
	if q.Timeout < 10 || q.Timeout > 3600 {
		q.Timeout = 90
	}
	unit := queueUnitName(d.ID)
	unitPath := filepath.Join("/etc/systemd/system", unit)
	if q.Aktif {
		body := queueUnit(d, p, q.Queue, q.Deneme, q.Timeout)
		if e = atomicWrite(unitPath, []byte(body), 0644); e != nil {
			httpx.WriteError(w, 500, e.Error())
			return
		}
	} else {
		_, _ = exec.Command("systemctl", "disable", "--now", unit).CombinedOutput()
		_ = os.Remove(unitPath)
	}
	if out, e := exec.Command("systemctl", "daemon-reload").CombinedOutput(); e != nil {
		httpx.WriteError(w, 500, "systemd yenilenemedi: "+string(out))
		return
	}
	if q.Aktif {
		if out, e := exec.Command("systemctl", "enable", "--now", unit).CombinedOutput(); e != nil {
			_ = os.Remove(unitPath)
			_, _ = exec.Command("systemctl", "daemon-reload").CombinedOutput()
			httpx.WriteError(w, 500, "queue worker başlatılamadı: "+string(out))
			return
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "aktif": q.Aktif})
}

var queueRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func queueUnitName(id int64) string {
	return "sanalcp-laravel-queue-" + strconv.FormatInt(id, 10) + ".service"
}
func queueUnit(d domain, p install, queue string, tries, timeout int) string {
	return fmt.Sprintf(`[Unit]
Description=SanalCP Laravel queue worker for %s
After=network.target mariadb.service

[Service]
Type=simple
User=%s
Group=%s
WorkingDirectory=%s
ExecStart=/usr/bin/php %s queue:work --queue=%s --sleep=3 --tries=%d --timeout=%d --no-interaction
Restart=on-failure
RestartSec=5
KillSignal=SIGTERM
TimeoutStopSec=360
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=%s %s
ProtectHome=read-only

[Install]
WantedBy=multi-user.target
`, d.AlanAdi, d.SK, d.SK, p.Root, filepath.Join(p.Root, "artisan"), queue, tries, timeout, filepath.Join(p.Root, "storage"), filepath.Join(p.Root, "bootstrap", "cache"))
}
func atomicWrite(target string, b []byte, mode os.FileMode) error {
	tmp, e := os.CreateTemp(filepath.Dir(target), ".sanalcp-laravel-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if e = tmp.Chmod(mode); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	if x := tmp.Close(); e == nil {
		e = x
	}
	if e != nil {
		return e
	}
	return os.Rename(name, target)
}
