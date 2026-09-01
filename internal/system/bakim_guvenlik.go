package system

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"sanalcp/internal/httpx"
	"sanalcp/internal/osfam"
)

const (
	guvenlikAyarYolu   = "/etc/sanalcp/guvenlik-guncelleme.json"
	guvenlikBetikYolu  = "/opt/sanalcp/bin/sanalcp-security-update"
	guvenlikServisYolu = "/etc/systemd/system/sanalcp-security-update.service"
	guvenlikTimerYolu  = "/etc/systemd/system/sanalcp-security-update.timer"
)

type guvenlikAyari struct {
	Aktif          bool `json:"aktif"`
	OtomatikReboot bool `json:"otomatik_reboot"`
}

type guvenlikDurum struct {
	Aktif          bool   `json:"aktif"`
	OtomatikReboot bool   `json:"otomatik_reboot"`
	Bekleyen       int    `json:"bekleyen"`
	SonCalisma     string `json:"son_calisma"`
	SonrakiCalisma string `json:"sonraki_calisma"`
	Calisiyor      bool   `json:"calisiyor"`
	Destekli       bool   `json:"destekli"`
}

func guvenlikAyariOku() guvenlikAyari {
	var a guvenlikAyari
	b, err := os.ReadFile(guvenlikAyarYolu)
	if err == nil {
		_ = json.Unmarshal(b, &a)
	}
	return a
}

func guvenlikBekleyen(ctx context.Context) int {
	if osfam.Mevcut().DebianMi() {
		b, _ := exec.CommandContext(ctx, "apt-get", "-s", "-o", "Debug::NoLocking=true", "dist-upgrade").CombinedOutput()
		return len(aptGuvenlikPaketleriniAyristir(string(b)))
	}
	b, _ := exec.CommandContext(ctx, "dnf", "-q", "updateinfo", "list", "--security", "updates").Output()
	n := 0
	for _, s := range strings.Split(string(b), "\n") {
		f := strings.Fields(s)
		if len(f) >= 3 && (strings.Contains(f[1], "/Sec.") || strings.HasPrefix(f[0], "ALSA-")) {
			n++
		}
	}
	return n
}

func systemctlOzellik(ctx context.Context, unit, alan string) string {
	b, _ := exec.CommandContext(ctx, "systemctl", "show", unit, "--property="+alan, "--value").Output()
	return strings.TrimSpace(string(b))
}

// GuvenlikGuncellemeDurum — günlük yalnız-güvenlik güncelleme zamanlayıcısı.
func GuvenlikGuncellemeDurum(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	a := guvenlikAyariOku()
	d := guvenlikDurum{OtomatikReboot: a.OtomatikReboot, Destekli: osfam.GuvenlikGuncellemeDestekli()}
	d.Aktif = systemctlOzellik(ctx, "sanalcp-security-update.timer", "UnitFileState") == "enabled"
	d.Calisiyor = systemctlOzellik(ctx, "sanalcp-security-update.service", "ActiveState") == "active"
	d.SonCalisma = systemctlOzellik(ctx, "sanalcp-security-update.service", "ExecMainExitTimestamp")
	d.SonrakiCalisma = systemctlOzellik(ctx, "sanalcp-security-update.timer", "NextElapseUSecRealtime")
	if d.Destekli {
		d.Bekleyen = guvenlikBekleyen(ctx)
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

func guvenlikDosyalariYaz(a guvenlikAyari) error {
	if err := atomikYaz(guvenlikAyarYolu, fmt.Sprintf("{\n  \"aktif\": %t,\n  \"otomatik_reboot\": %t\n}\n", a.Aktif, a.OtomatikReboot), 0600); err != nil {
		return err
	}
	reboot := "false"
	if a.OtomatikReboot {
		reboot = "true"
	}
	betik := `#!/usr/bin/env bash
set -euo pipefail
/usr/bin/dnf -y --refresh upgrade --security
if ` + reboot + `; then
  set +e
  reboot_check=$(/usr/bin/dnf -q needs-restarting -r 2>&1)
  reboot_rc=$?
  set -e
  if [ "$reboot_rc" -eq 1 ] && grep -Eqi 'reboot (is )?required|reboot should be performed' <<<"$reboot_check"; then
    /usr/bin/systemctl reboot
  fi
fi
`
	if osfam.Mevcut().DebianMi() {
		betik = `#!/usr/bin/env bash
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a
/usr/bin/apt-get update
if command -v unattended-upgrade >/dev/null 2>&1; then
  unattended-upgrade -d
else
  mapfile -t security_packages < <(/usr/bin/apt-get -s -o Debug::NoLocking=true dist-upgrade | awk '/^Inst / { p=index($0,"("); if (p && tolower(substr($0,p)) ~ /security/) print $2 }' | sort -u)
  if ((${#security_packages[@]})); then
    /usr/bin/apt-get install -y --only-upgrade "${security_packages[@]}"
  fi
fi
if ` + reboot + ` && [ -f /var/run/reboot-required ]; then
  /usr/bin/systemctl reboot
fi
`
	}
	if err := atomikYaz(guvenlikBetikYolu, betik, 0700); err != nil {
		return err
	}
	servis := "[Unit]\nDescription=SanalCP automatic security updates\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=oneshot\nExecStart=" + guvenlikBetikYolu + "\n"
	timer := "[Unit]\nDescription=SanalCP daily security updates\n\n[Timer]\nOnCalendar=daily\nPersistent=true\nRandomizedDelaySec=1h\n\n[Install]\nWantedBy=timers.target\n"
	if err := atomikYaz(guvenlikServisYolu, servis, 0644); err != nil {
		return err
	}
	return atomikYaz(guvenlikTimerYolu, timer, 0644)
}

func GuvenlikGuncellemeKaydet(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var a guvenlikAyari
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		httpx.WriteError(w, 400, "geçersiz istek gövdesi")
		return
	}
	guncelleyici := "dnf"
	if osfam.Mevcut().DebianMi() {
		guncelleyici = "apt-get"
	}
	if !osfam.GuvenlikGuncellemeDestekli() {
		httpx.WriteError(w, 400, "otomatik güvenlik güncellemesi bu sistemde desteklenmiyor")
		return
	}
	if _, err := exec.LookPath(guncelleyici); err != nil {
		httpx.WriteError(w, 400, "otomatik güvenlik güncellemesi bu sistemde desteklenmiyor")
		return
	}
	if !a.Aktif {
		a.OtomatikReboot = false
	}
	if err := guvenlikDosyalariYaz(a); err != nil {
		httpx.WriteError(w, 500, "güncelleme ayarı yazılamadı")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if b, err := exec.CommandContext(ctx, "systemctl", "daemon-reload").CombinedOutput(); err != nil {
		httpx.WriteError(w, 500, strings.TrimSpace(string(b)))
		return
	}
	islem := "disable"
	if a.Aktif {
		islem = "enable"
	}
	args := []string{islem, "--now", "sanalcp-security-update.timer"}
	if b, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput(); err != nil {
		httpx.WriteError(w, 500, strings.TrimSpace(string(b)))
		return
	}
	httpx.WriteJSON(w, 200, a)
}

type rebootDurum struct {
	Gerekli             bool   `json:"gerekli"`
	CalisanKernel       string `json:"calisan_kernel"`
	SonKernel           string `json:"son_kernel"`
	AcilisZamani        string `json:"acilis_zamani"`
	CalismaSuresiSaniye int64  `json:"calisma_suresi_saniye"`
}

func RebootDurum(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	calisan := strings.TrimSpace(runOut("uname", "-r"))
	son := calisan
	for _, ln := range strings.Split(runOut("rpm", "-q", "--last", "kernel"), "\n") {
		if f := strings.Fields(ln); len(f) > 0 {
			son = strings.TrimPrefix(f[0], "kernel-")
			break
		}
	}
	b, _ := exec.CommandContext(ctx, "cat", "/proc/uptime").Output()
	sec := float64(0)
	if f := strings.Fields(string(b)); len(f) > 0 {
		sec, _ = strconv.ParseFloat(f[0], 64)
	}
	boot := time.Now().Add(-time.Duration(sec) * time.Second).Format(time.RFC3339)
	httpx.WriteJSON(w, 200, rebootDurum{Gerekli: son != "" && son != calisan, CalisanKernel: calisan, SonKernel: son, AcilisZamani: boot, CalismaSuresiSaniye: int64(sec)})
}
