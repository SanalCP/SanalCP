package system

import (
	"context"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"sanalcp/internal/httpx"
)

// sshport.go — sshd'nin FİİLEN dinlediği portu tespit eder.
//
// 🔴 KAYNAK OLARAK `sshd -T` KULLANILIR, sshd_config'i elle ayrıştırmak DEĞİL.
// Modern OpenSSH kurulumlarında port `/etc/ssh/sshd_config.d/*.conf` altındaki
// bir Include dosyasında olabilir (AlmaLinux 10 varsayılanı böyle) ve ana
// dosyada hiç `Port` satırı bulunmayabilir. `sshd -T` tüm include'ları çözüp
// EFEKTİF yapılandırmayı basar — tek güvenilir kaynak budur.

const (
	// VarsayilanSSHPort: değiştirilmesi önerilen, internetteki otomatik tarama
	// ve kaba-kuvvet trafiğinin tamamına yakınının hedeflediği port.
	VarsayilanSSHPort = 22

	sshPortOnbellekOmru = 60 * time.Second
	sshdTZamanAsimi     = 5 * time.Second
)

var (
	sshPortMu      sync.Mutex
	sshPortCache   []int
	sshPortZamani  time.Time
	rePortSatiri   = regexp.MustCompile(`(?mi)^port\s+(\d{1,5})\s*$`)
	reSSListenPort = regexp.MustCompile(`:(\d{1,5})\s`)
)

// SSHPortlari: sshd'nin dinlediği portlar (artan sırada, tekrarsız).
// Tespit edilemezse varsayılan [22] döner — "bilinmiyor" durumunda uyarıyı
// göstermek, sessizce güvende sanmaktan iyidir.
func SSHPortlari(ctx context.Context) []int {
	sshPortMu.Lock()
	defer sshPortMu.Unlock()
	if time.Since(sshPortZamani) < sshPortOnbellekOmru && sshPortCache != nil {
		return sshPortCache
	}

	portlar := sshdTPortlari(ctx)
	if len(portlar) == 0 {
		portlar = ssDinlenenPortlar(ctx) // sshd -T başarısızsa fiilen dinlenene bak
	}
	if len(portlar) == 0 {
		portlar = []int{VarsayilanSSHPort}
	}
	sshPortCache, sshPortZamani = portlar, time.Now()
	return portlar
}

func sshdTPortlari(ctx context.Context) []int {
	c, iptal := context.WithTimeout(ctx, sshdTZamanAsimi)
	defer iptal()
	out, err := exec.CommandContext(c, "sshd", "-T").Output()
	if err != nil {
		// Bazı dağıtımlarda sshd PATH'te değil.
		c2, iptal2 := context.WithTimeout(ctx, sshdTZamanAsimi)
		defer iptal2()
		out, err = exec.CommandContext(c2, "/usr/sbin/sshd", "-T").Output()
		if err != nil {
			return nil
		}
	}
	var portlar []int
	for _, m := range rePortSatiri.FindAllStringSubmatch(string(out), -1) {
		if p, e := strconv.Atoi(m[1]); e == nil && p > 0 && p < 65536 {
			portlar = append(portlar, p)
		}
	}
	return benzersizSirali(portlar)
}

func ssDinlenenPortlar(ctx context.Context) []int {
	c, iptal := context.WithTimeout(ctx, sshdTZamanAsimi)
	defer iptal()
	out, err := exec.CommandContext(c, "ss", "-lntp").Output()
	if err != nil {
		return nil
	}
	var portlar []int
	for _, satir := range strings.Split(string(out), "\n") {
		if !strings.Contains(satir, "sshd") {
			continue
		}
		if m := reSSListenPort.FindStringSubmatch(satir); m != nil {
			if p, e := strconv.Atoi(m[1]); e == nil {
				portlar = append(portlar, p)
			}
		}
	}
	return benzersizSirali(portlar)
}

func benzersizSirali(in []int) []int {
	if len(in) == 0 {
		return nil
	}
	gorulen := map[int]bool{}
	out := make([]int, 0, len(in))
	for _, p := range in {
		if !gorulen[p] {
			gorulen[p] = true
			out = append(out, p)
		}
	}
	sort.Ints(out)
	return out
}

// VarsayilanPortKullaniliyor: sshd (aynı zamanda) 22'yi dinliyor mu?
func VarsayilanPortKullaniliyor(ctx context.Context) bool {
	for _, p := range SSHPortlari(ctx) {
		if p == VarsayilanSSHPort {
			return true
		}
	}
	return false
}

// SSHGuvenlikDurumu: panelin gösterdiği uyarı için.
type SSHGuvenlikDurumu struct {
	Portlar         []int `json:"portlar"`
	VarsayilanPort  bool  `json:"varsayilan_port"` // true → uyarı gösterilir
	VarsayilanDeger int   `json:"varsayilan_deger"`
}

// SSHGuvenlik — GET /system/ssh-guvenlik (AdminOnly)
func SSHGuvenlik(w http.ResponseWriter, r *http.Request) {
	portlar := SSHPortlari(r.Context())
	httpx.WriteJSON(w, http.StatusOK, SSHGuvenlikDurumu{
		Portlar:         portlar,
		VarsayilanPort:  VarsayilanPortKullaniliyor(r.Context()),
		VarsayilanDeger: VarsayilanSSHPort,
	})
}
