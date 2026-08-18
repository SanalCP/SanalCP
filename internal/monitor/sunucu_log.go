package monitor

import (
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"sanalcp/internal/httpx"
	"sanalcp/internal/osfam"
)

// logKaynaklari: kaynak anahtarı → systemd unit. Allowlist — komut enjeksiyonu YOK
// (kullanıcı girdisi doğrudan komuta gitmez, sadece bu haritadan geçer).
// Unit adları aileye göre değişir (named/bind9, crond/cron) — sabit RHEL adıyla
// bırakılırsa Debian'da o kaynak boş log döner.
var logKaynaklari = logKaynaklariSec(osfam.Mevcut())

func logKaynaklariSec(b osfam.Bilgi) map[string]string {
	return map[string]string{
		"panel":   "sanalcp.service",
		"mariadb": b.Servis(osfam.PaketDB) + ".service",
		"named":   b.Servis(osfam.PaketDNS) + ".service",
		"sshd":    b.Servis(osfam.PaketSSH) + ".service",
		"cron":    b.Servis(osfam.PaketCron) + ".service",
	}
}

// nginx journald'a yazmaz (dosyaya loglar) → dosya-tabanlı kaynak.
var dosyaKaynaklari = map[string]string{
	"nginx": "/var/log/nginx/error.log",
}

var logKaynakSira = []string{"panel", "nginx", "mariadb", "named", "sshd", "cron", "sistem"}

// SunucuLog: GET /admin/system/loglar?kaynak=panel&son=200 — journald sunucu günlükleri.
func (h *Handlers) SunucuLog(w http.ResponseWriter, r *http.Request) {
	kaynak := r.URL.Query().Get("kaynak")
	if kaynak == "" {
		kaynak = "panel"
	}
	son, _ := strconv.Atoi(r.URL.Query().Get("son"))
	if son < 50 {
		son = 200
	}
	if son > 1000 {
		son = 1000
	}
	var out []byte
	if dosya, ok := dosyaKaynaklari[kaynak]; ok {
		// dosya-tabanlı (nginx error.log gibi) — tail
		out, _ = exec.Command("tail", "-n", strconv.Itoa(son), dosya).CombinedOutput()
	} else {
		args := []string{"--no-pager", "-o", "short-iso", "-n", strconv.Itoa(son)}
		if kaynak != "sistem" {
			unit, ok := logKaynaklari[kaynak]
			if !ok {
				httpx.WriteError(w, http.StatusBadRequest, "geçersiz log kaynağı")
				return
			}
			args = append(args, "-u", unit)
		}
		out, _ = exec.Command("journalctl", args...).CombinedOutput()
	}
	metin := strings.TrimRight(string(out), "\n")
	satirlar := []string{}
	if metin != "" {
		satirlar = strings.Split(metin, "\n")
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"kaynak":    kaynak,
		"satirlar":  satirlar,
		"kaynaklar": logKaynakSira,
	})
}
