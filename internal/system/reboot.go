package system

// Sunucuyu Yeniden Başlat — /araclar-ayarlar sayfasındaki kırmızı buton.
//
// GÜVENLİK: argüman YOK, komut tamamen SABİT — hiçbir kullanıcı girdisi geçmez.
// Reboot, HTTP yanıtı istemciye ulaştıktan SONRA gerçekleşsin diye systemd-run ile
// birkaç saniye geciktirilerek zamanlanır (aynı desen: internal/system/optimize.go'daki
// OptimizeBaslat — transient unit PID 1 altında, panelin kendi cgroup'unda DEĞİL, bu
// yüzden panel süreci öldüğünde iş yarıda kesilmez).

import (
	"net/http"
	"os/exec"
	"strings"

	"sanalcp/internal/httpx"
)

// Reboot — POST /system/reboot: sunucuyu ~5sn sonra yeniden başlatır.
func Reboot(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("systemd-run",
		"--on-active=5",
		"--unit=sanalcp-reboot",
		"--description=SanalCP: sunucu yeniden başlatma",
		"--", "systemctl", "reboot")
	if out, err := cmd.CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "başlatılamadı: "+strings.TrimSpace(string(out)))
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"ok":    true,
		"mesaj": "Sunucu birkaç saniye içinde yeniden başlatılacak.",
	})
}

// Kapat — POST /system/kapat: sunucuyu ~5sn sonra YAZILIMSAL (graceful) kapatır.
//
// `systemctl poweroff` kullanılır: systemd önce tüm unit'leri düzenli durdurur
// (SIGTERM, sonra gerekirse SIGKILL), dosya sistemlerini senkronize edip söker,
// ardından ACPI ile gücü keser. `poweroff --force` gibi sert bir yol DEĞİLDİR.
//
// Reboot ile aynı desen: argüman yok, komut sabit, transient unit PID 1 altında,
// böylece HTTP yanıtı istemciye ulaştıktan sonra kapanma başlar.
//
// DİKKAT: reboot'un tersine bu işlem kendi kendine geri dönmez — sunucu ancak
// sağlayıcı panelinden / fiziksel olarak yeniden açılabilir. UI'da bu yüzden
// ayrı bir kırmızı buton ve kendi onay adımı vardır.
func Kapat(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("systemd-run",
		"--on-active=5",
		"--unit=sanalcp-kapat",
		"--description=SanalCP: sunucu kapatma",
		"--", "systemctl", "poweroff")
	if out, err := cmd.CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kapatılamadı: "+strings.TrimSpace(string(out)))
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"ok":    true,
		"mesaj": "Sunucu birkaç saniye içinde kapatılacak.",
	})
}
