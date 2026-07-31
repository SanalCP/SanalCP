package system

// Sunucu optimizasyonu + sistem paket güncellemesi — panelden tetiklenen,
// UZUN SÜREN, servis-etkileyebilen bir bakım işi.
//
// GÜVENLİK: komut SABİT — hiçbir kullanıcı girdisi argümana geçmez. Panel root
// çalıştığı için ayrıcalık zaten var; iş, panelin systemd cgroup'unda DEĞİL,
// systemd-run ile PID 1 altında AYRI transient unit olarak koşar (panel restart
// olsa/güncellense bile iş ölmez). Çıktı systemd'nin StandardOutput=append: ile
// log dosyasına yazılır → shell string / kabuk yorumlaması YOKTUR (argv-only).
//
// AKIŞ (sabit wrapper script):
//   1) sistem paket güncellemesi: dnf -y update (yoksa yum -y update)
//   2) MariaDB/nginx/PHP performans ayarı: sanalcp-optimize

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"sanalcp/internal/httpx"
)

const (
	optimizeUnit    = "sanalcp-optimize-run"
	optimizeLogYol  = "/opt/sanalcp/logs/optimize.log"
	optimizeWrapper = "/opt/sanalcp/optimize-run.sh"
)

// optimizeWrapperIcerik — SABİT script. Kullanıcı girdisi İÇERMEZ; her başlatmada
// diske atomik yazılır (kaynak-doğruluğu Go tarafında tek yerde). dnf/yum -y update
// + sanalcp-optimize. Her adım kendi başına idempotent + güvenli.
// Panel diline göre TR/EN üretilir — script kendisi ayrı bir process olarak
// systemd-run ile çalıştığı için DB'ye erişemez, dil seçimi burada (yazılma anında)
// sabitlenir.
func optimizeWrapperIcerik(dil string) string {
	if dil == "en" {
		return `#!/usr/bin/env bash
set -uo pipefail
echo "════════ Server Optimization — $(date "+%Y-%m-%d %H:%M:%S") ════════"
echo
echo "▶ 1/2 · System package update (AlmaLinux)"
if command -v dnf >/dev/null 2>&1; then
  dnf -y update
elif command -v yum >/dev/null 2>&1; then
  yum -y update
else
  echo "  (dnf/yum not found — package update skipped)"
fi
echo
echo "▶ 2/2 · MariaDB / nginx / PHP performance tuning"
if command -v sanalcp-optimize >/dev/null 2>&1; then
  PANEL_LANG=en sanalcp-optimize
else
  echo "  (sanalcp-optimize not found — tuning skipped)"
fi
echo
echo "════════ ✓ Optimization complete ════════"
`
	}
	return `#!/usr/bin/env bash
set -uo pipefail
echo "════════ Sunucu Optimizasyonu — $(date "+%Y-%m-%d %H:%M:%S") ════════"
echo
echo "▶ 1/2 · Sistem paket güncellemesi (AlmaLinux)"
if command -v dnf >/dev/null 2>&1; then
  dnf -y update
elif command -v yum >/dev/null 2>&1; then
  yum -y update
else
  echo "  (dnf/yum bulunamadı — paket güncellemesi atlandı)"
fi
echo
echo "▶ 2/2 · MariaDB / nginx / PHP performans ayarı"
if command -v sanalcp-optimize >/dev/null 2>&1; then
  sanalcp-optimize
else
  echo "  (sanalcp-optimize bulunamadı — tuning atlandı)"
fi
echo
echo "════════ ✓ Optimizasyon tamamlandı ════════"
`
}

// optimizeCalisiyor — transient unit hâlâ çalışıyor mu.
func optimizeCalisiyor() (bool, string) {
	d := strings.TrimSpace(runOut("systemctl", "is-active", optimizeUnit))
	return d == "active" || d == "activating", d
}

// OptimizeDurum — GET /system/optimize.
func OptimizeDurum(w http.ResponseWriter, r *http.Request) {
	calisiyor, durum := optimizeCalisiyor()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"calisiyor": calisiyor,
		"durum":     durum,
	})
}

// optimizeWrapperYaz — sabit wrapper scriptini atomik yazar (0700, panel-özel).
func optimizeWrapperYaz(dil string) error {
	tmp := optimizeWrapper + ".tmp"
	if err := os.WriteFile(tmp, []byte(optimizeWrapperIcerik(dil)), 0o700); err != nil {
		return err
	}
	return os.Rename(tmp, optimizeWrapper) // atomik
}

// OptimizeBaslat — POST /system/optimize/baslat: optimizasyonu ayrı systemd
// unit'inde başlatır.
func OptimizeBaslat(w http.ResponseWriter, r *http.Request) {
	if c, _ := optimizeCalisiyor(); c {
		httpx.WriteError(w, http.StatusConflict, t("optimizasyon zaten çalışıyor", "optimization already running"))
		return
	}
	// Panel güncellemesiyle çakışmasın (ikisi de paket/servis dokunur).
	if c, _ := guncelleCalisiyor(); c {
		httpx.WriteError(w, http.StatusConflict, t("panel güncellemesi sürüyor — bitince tekrar deneyin", "a panel update is in progress — try again once it's done"))
		return
	}
	_ = os.MkdirAll("/opt/sanalcp/logs", 0o750)
	dil := panelDili()
	if err := optimizeWrapperYaz(dil); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, t("hazırlanamadı: ", "could not prepare: ")+err.Error())
		return
	}
	bas := fmt.Sprintf("%s\n", t(
		fmt.Sprintf("=== Optimizasyon başlatıldı: %s ===", time.Now().Format("2006-01-02 15:04:05")),
		fmt.Sprintf("=== Optimization started: %s ===", time.Now().Format("2006-01-02 15:04:05"))))
	if err := os.WriteFile(optimizeLogYol, []byte(bas), 0o640); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, t("log açılamadı: ", "could not open log: ")+err.Error())
		return
	}
	// systemd-run: PID 1 altında transient unit; çıktı append: ile log dosyasına
	// (shell string YOK — tüm argümanlar sabit).
	cmd := exec.Command("systemd-run",
		"--collect",
		"--unit", optimizeUnit,
		"--description", "SanalCP sunucu optimizasyonu",
		"-p", "StandardOutput=append:"+optimizeLogYol,
		"-p", "StandardError=append:"+optimizeLogYol,
		optimizeWrapper)
	if out, err := cmd.CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, t("başlatılamadı: ", "could not start: ")+strings.TrimSpace(string(out)))
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"baslatildi": true})
}

// OptimizeLog — GET /system/optimize/log: log kuyruğu + durum.
func OptimizeLog(w http.ResponseWriter, r *http.Request) {
	b, err := os.ReadFile(optimizeLogYol)
	if err != nil {
		b = nil
	}
	s := string(b)
	if len(s) > 60000 {
		s = s[len(s)-60000:]
	}
	calisiyor, durum := optimizeCalisiyor()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"log":       s,
		"calisiyor": calisiyor,
		"durum":     durum,
	})
}
