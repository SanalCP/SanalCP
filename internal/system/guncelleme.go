package system

// Panel içi güncelleme — CLI'ya bağımlı olmadan panelden güncelleme.
//
// NEDEN: sanalcp-update scriptini dağıtan tek mekanizma yine kendisiydi;
// script eklenmeden önce kurulum yapan müşteriler kısır döngüye giriyordu
// ("command not found" → güncelleyemiyor → scripti alamıyor). Bu uç nokta
// scripti gerekirse repo'dan indirip (bootstrap) çalıştırır.
//
// 🔴 KRİTİK: Güncelleme panelin KENDİ binary'sini değiştirip servisi restart eder.
// Süreç panelin systemd cgroup'unda çalışsaydı, restart sırasında SIGKILL yerdi
// (KillMode=control-group varsayılanı tüm cgroup'u öldürür) → güncelleme yarıda
// kalır, panel bozulur. Bu yüzden `systemd-run` ile PID 1 altında AYRI transient
// unit olarak başlatılır; panel restart olurken süreç yaşamaya devam eder.

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"sanalcp/internal/httpx"
)

type GuncellemeKontrolu struct {
	Anahtar     string `json:"anahtar"`
	Basarili    bool   `json:"basarili"`
	Engelleyici bool   `json:"engelleyici"`
	Aciklama    string `json:"aciklama"`
}

// GuncellemeOnKontrol değişiklik yapmadan güncellemenin güvenli önkoşullarını
// sınar. Ayrıntılı komut çıktıları bilgi sızdırmamak için yanıta eklenmez.
func GuncellemeOnKontrol(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kontroller, uygun := guncellemeKontrolleri(r.Context(), db)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"uygun": uygun, "kontroller": kontroller, "otomatik_rollback": true})
	}
}

func guncellemeKontrolleri(parent context.Context, db *sql.DB) ([]GuncellemeKontrolu, bool) {
	kontroller := []GuncellemeKontrolu{}
	ekle := func(k string, ok, engelleyici bool, tr, en string) {
		kontroller = append(kontroller, GuncellemeKontrolu{Anahtar: k, Basarili: ok, Engelleyici: engelleyici, Aciklama: t(tr, en)})
	}
	ctxDB, cancelDB := context.WithTimeout(parent, 3*time.Second)
	ekle("veritabani", db.PingContext(ctxDB) == nil, true, "Veritabanı bağlantısı hazır", "Database connection is ready")
	cancelDB()
	var fs syscall.Statfs_t
	err := syscall.Statfs("/opt/sanalcp", &fs)
	bos := uint64(fs.Bavail) * uint64(fs.Bsize)
	ekle("disk", err == nil && bos >= 1<<30, true, "En az 1 GiB boş disk alanı var", "At least 1 GiB of disk space is available")
	_, backupErr := os.Stat("/usr/local/bin/sanalcp-db-backup")
	ekle("yedek", backupErr == nil, false, "DB yedek aracı hazır (yoksa güncelleyici kuracak)", "Database backup tool is ready (the updater will install it if missing)")
	_, frontErr := os.Stat("/opt/sanalcp/frontend-dist/index.html")
	ekle("frontend", frontErr == nil, true, "Mevcut panel arayüzü yedeklenebilir", "Current panel frontend can be backed up")
	ctxNginx, cancelNginx := context.WithTimeout(parent, 5*time.Second)
	nginxErr := exec.CommandContext(ctxNginx, "nginx", "-t").Run()
	cancelNginx()
	ekle("nginx", nginxErr == nil, true, "nginx yapılandırması geçerli", "nginx configuration is valid")
	uygun := true
	for _, k := range kontroller {
		if !k.Basarili && k.Engelleyici {
			uygun = false
			break
		}
	}
	return kontroller, uygun
}

const (
	guncelleScript = "/usr/local/bin/sanalcp-update"
	guncelleRawURL = "https://raw.githubusercontent.com/sanalcp/sanalcp/main/assets/ops/sanalcp-update"
	guncelleLogYol = "/opt/sanalcp/logs/guncelleme.log"
	guncelleUnit   = "sanalcp-guncelleme"
)

// guncelleCalisiyor — transient unit hâlâ çalışıyor mu.
func guncelleCalisiyor() (bool, string) {
	d := strings.TrimSpace(runOut("systemctl", "is-active", guncelleUnit))
	return d == "active" || d == "activating", d
}

// GuncellemeDurum — güncelleme aracı mevcut mu, çalışıyor mu.
func GuncellemeDurum(w http.ResponseWriter, r *http.Request) {
	_, serr := os.Stat(guncelleScript)
	calisiyor, durum := guncelleCalisiyor()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"arac_var":  serr == nil,
		"calisiyor": calisiyor,
		"durum":     durum,
	})
}

// guncelleAracIndir — eksik update scriptini repo'dan indirir (eski kurulum kurtarma).
func guncelleAracIndir() error {
	cli := &http.Client{Timeout: 30 * time.Second}
	resp, err := cli.Get(guncelleRawURL)
	if err != nil {
		return fmt.Errorf("%s: %w", t("indirilemedi", "download failed"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s HTTP %d", t("indirme", "download"), resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%s: %w", t("okunamadı", "could not read"), err)
	}
	// gelen şey gerçekten script mi (HTML hata sayfası vb. değil)
	if !strings.HasPrefix(string(b), "#!") {
		return fmt.Errorf("%s", t("beklenmeyen içerik — script değil", "unexpected content — not a script"))
	}
	tmp := guncelleScript + ".tmp"
	if err := os.WriteFile(tmp, b, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, guncelleScript) // atomik
}

// GuncellemeBaslat — aracı (gerekirse indirip) ayrı systemd unit'inde başlatır.
func GuncellemeBaslat(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, uygun := guncellemeKontrolleri(r.Context(), db); !uygun {
			httpx.WriteError(w, http.StatusPreconditionFailed, t("güncelleme ön kontrolleri başarısız", "update pre-checks failed"))
			return
		}
		if calisiyor, _ := guncelleCalisiyor(); calisiyor {
			httpx.WriteError(w, http.StatusConflict, t("güncelleme zaten çalışıyor", "update already running"))
			return
		}
		aracIndirildi := false
		if _, err := os.Stat(guncelleScript); err != nil {
			if err := guncelleAracIndir(); err != nil {
				httpx.WriteError(w, http.StatusBadGateway, t("güncelleme aracı alınamadı: ", "could not fetch update tool: ")+err.Error())
				return
			}
			aracIndirildi = true
		}

		_ = os.MkdirAll("/opt/sanalcp/logs", 0o750)
		bas := fmt.Sprintf("%s\n", t(
			fmt.Sprintf("=== Güncelleme başlatıldı: %s ===", time.Now().Format("2006-01-02 15:04:05")),
			fmt.Sprintf("=== Update started: %s ===", time.Now().Format("2006-01-02 15:04:05"))))
		if aracIndirildi {
			bas += t("(güncelleme aracı eksikti — repo'dan indirildi)\n", "(update tool was missing — fetched from the repo)\n")
		}
		if err := os.WriteFile(guncelleLogYol, []byte(bas), 0o640); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, t("log açılamadı: ", "could not open log: ")+err.Error())
			return
		}

		// systemd-run: PID 1 altında ayrı transient unit → panel restart'ında ÖLMEZ.
		cmd := exec.Command("systemd-run",
			"--collect", // bitince unit'i temizle (failed olsa da)
			"--unit", guncelleUnit,
			"--description", "SanalCP güncelleme",
			"/bin/bash", "-lc", fmt.Sprintf("%s >>%s 2>&1", guncelleScript, guncelleLogYol))
		if out, err := cmd.CombinedOutput(); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, t("başlatılamadı: ", "could not start: ")+strings.TrimSpace(string(out)))
			return
		}
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
			"baslatildi":     true,
			"arac_indirildi": aracIndirildi,
		})
	}
}

// GuncellemeLog — log kuyruğu + durum. Panel restart olsa da log dosyası diskte kalır.
func GuncellemeLog(w http.ResponseWriter, r *http.Request) {
	b, err := os.ReadFile(guncelleLogYol)
	if err != nil {
		b = nil
	}
	s := string(b)
	if len(s) > 60000 { // son 60KB yeter
		s = s[len(s)-60000:]
	}
	calisiyor, durum := guncelleCalisiyor()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"log":       s,
		"calisiyor": calisiyor,
		"durum":     durum,
	})
}
