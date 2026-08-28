package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"sanalcp/internal/httpx"
)

const depolamaAyarYolu = "/etc/sanalcp/depolama-uyari.json"

type kimlikDNSDurum struct {
	Hostname          string              `json:"hostname"`
	Adresler          []string            `json:"adresler"`
	HostnameAdresleri []string            `json:"hostname_adresleri"`
	PTR               map[string][]string `json:"ptr"`
	IleriEslesiyor    bool                `json:"ileri_eslesiyor"`
	PTREslesiyor      bool                `json:"ptr_eslesiyor"`
}

func SunucuKimlikDNS(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	host, _ := os.Hostname()
	d := kimlikDNSDurum{Hostname: host, PTR: map[string][]string{}}
	ifaces, _ := net.Interfaces()
	for _, in := range ifaces {
		addrs, _ := in.Addrs()
		for _, a := range addrs {
			ip := net.ParseIP(strings.Split(a.String(), "/")[0])
			if ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				d.Adresler = append(d.Adresler, ip.String())
			}
		}
	}
	ips, _ := net.DefaultResolver.LookupIPAddr(ctx, host)
	for _, ip := range ips {
		d.HostnameAdresleri = append(d.HostnameAdresleri, ip.IP.String())
	}
	for _, ip := range d.Adresler {
		ptr, _ := net.DefaultResolver.LookupAddr(ctx, ip)
		d.PTR[ip] = ptr
		for _, p := range ptr {
			if strings.EqualFold(strings.TrimSuffix(p, "."), strings.TrimSuffix(host, ".")) {
				d.PTREslesiyor = true
			}
		}
		for _, hip := range d.HostnameAdresleri {
			if hip == ip {
				d.IleriEslesiyor = true
			}
		}
	}
	sort.Strings(d.Adresler)
	sort.Strings(d.HostnameAdresleri)
	httpx.WriteJSON(w, 200, d)
}

type depolamaAyar struct {
	DiskEsik  int `json:"disk_esik"`
	InodeEsik int `json:"inode_esik"`
}
type depolamaDurum struct {
	depolamaAyar
	DiskYuzde     float64           `json:"disk_yuzde"`
	InodeYuzde    float64           `json:"inode_yuzde"`
	DiskToplam    uint64            `json:"disk_toplam"`
	DiskBos       uint64            `json:"disk_bos"`
	InodeToplam   uint64            `json:"inode_toplam"`
	InodeBos      uint64            `json:"inode_bos"`
	DiskUyari     bool              `json:"disk_uyari"`
	InodeUyari    bool              `json:"inode_uyari"`
	BuyukDizinler map[string]uint64 `json:"buyuk_dizinler"`
}

func depolamaAyariOku() depolamaAyar {
	a := depolamaAyar{DiskEsik: 80, InodeEsik: 85}
	if b, err := os.ReadFile(depolamaAyarYolu); err == nil {
		_ = json.Unmarshal(b, &a)
	}
	return a
}

func DepolamaDurumHandler(w http.ResponseWriter, r *http.Request) {
	var s syscall.Statfs_t
	if err := syscall.Statfs("/", &s); err != nil {
		httpx.WriteError(w, 500, "disk durumu okunamadı")
		return
	}
	a := depolamaAyariOku()
	top := s.Blocks * uint64(s.Bsize)
	bos := s.Bavail * uint64(s.Bsize)
	itop := s.Files
	ibos := s.Ffree
	d := depolamaDurum{depolamaAyar: a, DiskToplam: top, DiskBos: bos, InodeToplam: itop, InodeBos: ibos, BuyukDizinler: map[string]uint64{}}
	if top > 0 {
		d.DiskYuzde = float64(top-bos) * 100 / float64(top)
	}
	if itop > 0 {
		d.InodeYuzde = float64(itop-ibos) * 100 / float64(itop)
	}
	d.DiskUyari = d.DiskYuzde >= float64(a.DiskEsik)
	d.InodeUyari = d.InodeYuzde >= float64(a.InodeEsik)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	for _, p := range []string{"/home", "/var", "/opt", "/usr"} {
		b, err := exec.CommandContext(ctx, "du", "-sx", "--block-size=1", p).Output()
		if err == nil {
			f := strings.Fields(string(b))
			if len(f) > 0 {
				n, _ := strconv.ParseUint(f[0], 10, 64)
				d.BuyukDizinler[p] = n
			}
		}
	}
	httpx.WriteJSON(w, 200, d)
}

func DepolamaAyarKaydet(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var a depolamaAyar
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&a) != nil || a.DiskEsik < 50 || a.DiskEsik > 99 || a.InodeEsik < 50 || a.InodeEsik > 99 {
		httpx.WriteError(w, 400, "eşikler 50–99 arasında olmalıdır")
		return
	}
	if err := atomikYaz(depolamaAyarYolu, fmt.Sprintf("{\n  \"disk_esik\": %d,\n  \"inode_esik\": %d\n}\n", a.DiskEsik, a.InodeEsik), 0600); err != nil {
		httpx.WriteError(w, 500, "eşikler kaydedilemedi")
		return
	}
	httpx.WriteJSON(w, 200, a)
}

type journalDurum struct {
	KullanimByte uint64 `json:"kullanim_byte"`
	MaksimumMB   int    `json:"maksimum_mb"`
	Kalici       bool   `json:"kalici"`
}

var boyutRe = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*([KMGT]?)(?:i?B)?`)

func boyutByte(s string) uint64 {
	m := boyutRe.FindStringSubmatch(s)
	if len(m) < 3 {
		return 0
	}
	n, _ := strconv.ParseFloat(m[1], 64)
	carp := float64(1)
	switch strings.ToUpper(m[2]) {
	case "K":
		carp = 1 << 10
	case "M":
		carp = 1 << 20
	case "G":
		carp = 1 << 30
	case "T":
		carp = 1 << 40
	}
	return uint64(n * carp)
}
func JournalDurum(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	b, _ := exec.CommandContext(ctx, "journalctl", "--disk-usage").CombinedOutput()
	max := 0
	if c, err := os.ReadFile("/etc/systemd/journald.conf.d/90-sanalcp.conf"); err == nil {
		for _, ln := range strings.Split(string(c), "\n") {
			if strings.HasPrefix(ln, "SystemMaxUse=") {
				max = int(boyutByte(strings.TrimPrefix(ln, "SystemMaxUse=")) / (1 << 20))
			}
		}
	}
	_, err := os.Stat("/var/log/journal")
	httpx.WriteJSON(w, 200, journalDurum{KullanimByte: boyutByte(string(b)), MaksimumMB: max, Kalici: err == nil})
}
func JournalAyarKaydet(w http.ResponseWriter, r *http.Request) {
	var q struct {
		MaksimumMB int `json:"maksimum_mb"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&q) != nil || q.MaksimumMB < 100 || q.MaksimumMB > 10240 {
		httpx.WriteError(w, 400, "journal sınırı 100–10240 MB olmalıdır")
		return
	}
	icerik := fmt.Sprintf("[Journal]\nStorage=persistent\nSystemMaxUse=%dM\n", q.MaksimumMB)
	if err := atomikYaz("/etc/systemd/journald.conf.d/90-sanalcp.conf", icerik, 0644); err != nil {
		httpx.WriteError(w, 500, "journal ayarı yazılamadı")
		return
	}
	_ = os.MkdirAll("/var/log/journal", 0755)
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	if b, err := exec.CommandContext(ctx, "systemctl", "restart", "systemd-journald").CombinedOutput(); err != nil {
		httpx.WriteError(w, 500, strings.TrimSpace(string(b)))
		return
	}
	httpx.WriteJSON(w, 200, q)
}
func JournalTemizle(w http.ResponseWriter, r *http.Request) {
	var q struct {
		KoruMB int `json:"koru_mb"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if json.NewDecoder(r.Body).Decode(&q) != nil || q.KoruMB < 50 || q.KoruMB > 10240 {
		httpx.WriteError(w, 400, "korunacak boyut 50–10240 MB olmalıdır")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "journalctl", "--vacuum-size="+strconv.Itoa(q.KoruMB)+"M").CombinedOutput()
	if err != nil {
		httpx.WriteError(w, 500, strings.TrimSpace(string(b)))
		return
	}
	httpx.WriteJSON(w, 200, map[string]string{"sonuc": strings.TrimSpace(string(b))})
}

type dnsDurum struct {
	Sunucular     []string `json:"sunucular"`
	AramaAlanlari []string `json:"arama_alanlari"`
	Yonetici      string   `json:"yonetici"`
	Baglanti      string   `json:"baglanti"`
}

func aktifNMBaglanti(ctx context.Context) (string, string) {
	b, _ := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME,DEVICE", "connection", "show", "--active").Output()
	for _, ln := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		ad, cihaz, ok := strings.Cut(ln, ":")
		if ok && ad != "" && cihaz != "" && cihaz != "lo" {
			return ad, cihaz
		}
	}
	return "", ""
}

func resolvConfOku() (dnsDurum, error) {
	b, e := os.ReadFile("/etc/resolv.conf")
	if e != nil {
		return dnsDurum{}, e
	}
	d := dnsDurum{Yonetici: "resolv.conf"}
	for _, ln := range strings.Split(string(b), "\n") {
		f := strings.Fields(ln)
		if len(f) > 1 && f[0] == "nameserver" {
			d.Sunucular = append(d.Sunucular, f[1])
		}
		if len(f) > 1 && f[0] == "search" {
			d.AramaAlanlari = append(d.AramaAlanlari, f[1:]...)
		}
	}
	return d, nil
}
func DNSCozumleyiciDurum(w http.ResponseWriter, r *http.Request) {
	d, e := resolvConfOku()
	if e != nil {
		httpx.WriteError(w, 500, "DNS yapılandırması okunamadı")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	ad, _ := aktifNMBaglanti(ctx)
	if ad != "" {
		d.Yonetici = "NetworkManager"
		d.Baglanti = ad
	}
	httpx.WriteJSON(w, 200, d)
}
func alanAdiDogrula(s string) error {
	s = strings.TrimSpace(strings.TrimSuffix(s, "."))
	if len(s) < 1 || len(s) > 253 {
		return errors.New("geçersiz alan adı")
	}
	for _, p := range strings.Split(s, ".") {
		if len(p) < 1 || len(p) > 63 || p[0] == '-' || p[len(p)-1] == '-' {
			return errors.New("geçersiz alan adı")
		}
		for _, c := range p {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return errors.New("geçersiz alan adı")
			}
		}
	}
	return nil
}
func DNSTest(w http.ResponseWriter, r *http.Request) {
	var q struct {
		AlanAdi string `json:"alan_adi"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if json.NewDecoder(r.Body).Decode(&q) != nil || alanAdiDogrula(q.AlanAdi) != nil {
		httpx.WriteError(w, 400, "geçersiz alan adı")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	ips, e := net.DefaultResolver.LookupIPAddr(ctx, q.AlanAdi)
	if e != nil {
		httpx.WriteError(w, 502, "DNS çözümlemesi başarısız: "+e.Error())
		return
	}
	out := []string{}
	for _, ip := range ips {
		out = append(out, ip.IP.String())
	}
	httpx.WriteJSON(w, 200, map[string]any{"alan_adi": q.AlanAdi, "adresler": out})
}
func DNSCozumleyiciKaydet(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Sunucular []string `json:"sunucular"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&q) != nil || len(q.Sunucular) < 1 || len(q.Sunucular) > 3 {
		httpx.WriteError(w, 400, "1–3 DNS sunucusu girilmelidir")
		return
	}
	for _, s := range q.Sunucular {
		if net.ParseIP(strings.TrimSpace(s)) == nil {
			httpx.WriteError(w, 400, "geçersiz DNS sunucusu")
			return
		}
	}
	d, _ := resolvConfOku()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	bag, cihaz := aktifNMBaglanti(ctx)
	if bag == "" || cihaz == "" {
		httpx.WriteError(w, 500, "aktif NetworkManager bağlantısı bulunamadı")
		return
	}
	bIgnore, _ := exec.CommandContext(ctx, "nmcli", "-g", "ipv4.ignore-auto-dns", "connection", "show", bag).Output()
	eskiIgnore := strings.TrimSpace(string(bIgnore))
	uygula := func(ns []string) error {
		args := []string{"connection", "modify", bag, "ipv4.ignore-auto-dns", "yes", "ipv4.dns", strings.Join(ns, ",")}
		if out, e := exec.CommandContext(ctx, "nmcli", args...).CombinedOutput(); e != nil {
			return fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
		// Profildeki DNS değişikliğini bağlantıyı indirip kaldırmadan etkin cihaza
		// uygula; böylece panel/SSH oturumu kesilmez ve varsayılan rota değişmez.
		out, e := exec.CommandContext(ctx, "nmcli", "device", "reapply", cihaz).CombinedOutput()
		if e != nil {
			return fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
		return nil
	}
	if e := uygula(q.Sunucular); e != nil {
		httpx.WriteError(w, 500, "DNS uygulanamadı: "+e.Error())
		return
	}
	testCtx, testCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer testCancel()
	if _, e := net.DefaultResolver.LookupHost(testCtx, "pool.ntp.org"); e != nil {
		_ = uygula(d.Sunucular)
		if eskiIgnore != "" {
			_, _ = exec.CommandContext(context.Background(), "nmcli", "connection", "modify", bag, "ipv4.ignore-auto-dns", eskiIgnore).CombinedOutput()
			_, _ = exec.CommandContext(context.Background(), "nmcli", "device", "reapply", cihaz).CombinedOutput()
		}
		httpx.WriteError(w, 500, "yeni DNS ile çözümleme başarısız; eski ayar geri yüklendi")
		return
	}
	httpx.WriteJSON(w, 200, q)
}

type swapDurum struct {
	ToplamByte     uint64   `json:"toplam_byte"`
	KullanilanByte uint64   `json:"kullanilan_byte"`
	Swappiness     int      `json:"swappiness"`
	Kaynaklar      []string `json:"kaynaklar"`
}

func swapOku() swapDurum {
	d := swapDurum{}
	if b, e := os.ReadFile("/proc/swaps"); e == nil {
		for i, ln := range strings.Split(string(b), "\n") {
			if i == 0 {
				continue
			}
			f := strings.Fields(ln)
			if len(f) >= 5 {
				kb, _ := strconv.ParseUint(f[2], 10, 64)
				used, _ := strconv.ParseUint(f[3], 10, 64)
				d.ToplamByte += kb * 1024
				d.KullanilanByte += used * 1024
				d.Kaynaklar = append(d.Kaynaklar, f[0])
			}
		}
	}
	b, _ := os.ReadFile("/proc/sys/vm/swappiness")
	d.Swappiness, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	return d
}
func SwapDurumHandler(w http.ResponseWriter, r *http.Request) { httpx.WriteJSON(w, 200, swapOku()) }
func SwapAyarKaydet(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Swappiness int `json:"swappiness"`
		OlusturMB  int `json:"olustur_mb"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&q) != nil || q.Swappiness < 0 || q.Swappiness > 100 || q.OlusturMB < 0 || q.OlusturMB > 32768 || (q.OlusturMB > 0 && q.OlusturMB < 256) {
		httpx.WriteError(w, 400, "geçersiz swap ayarı")
		return
	}
	if err := atomikYaz("/etc/sysctl.d/90-sanalcp-swap.conf", fmt.Sprintf("vm.swappiness = %d\n", q.Swappiness), 0644); err != nil {
		httpx.WriteError(w, 500, "swappiness kaydedilemedi")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if b, e := exec.CommandContext(ctx, "sysctl", "-p", "/etc/sysctl.d/90-sanalcp-swap.conf").CombinedOutput(); e != nil {
		httpx.WriteError(w, 500, strings.TrimSpace(string(b)))
		return
	}
	if q.OlusturMB > 0 {
		if swapOku().ToplamByte > 0 {
			httpx.WriteError(w, 409, "sunucuda zaten swap alanı var")
			return
		}
		var st syscall.Statfs_t
		if syscall.Statfs("/", &st) != nil || uint64(q.OlusturMB+512)*(1<<20) > st.Bavail*uint64(st.Bsize) {
			httpx.WriteError(w, 400, "swap için yeterli boş disk alanı yok")
			return
		}
		if _, e := os.Stat("/swapfile"); !os.IsNotExist(e) {
			httpx.WriteError(w, 409, "/swapfile zaten mevcut")
			return
		}
		cmds := [][]string{{"fallocate", "-l", strconv.Itoa(q.OlusturMB) + "M", "/swapfile"}, {"chmod", "600", "/swapfile"}, {"mkswap", "/swapfile"}, {"swapon", "/swapfile"}}
		for _, c := range cmds {
			if b, e := exec.CommandContext(ctx, c[0], c[1:]...).CombinedOutput(); e != nil {
				_ = os.Remove("/swapfile")
				httpx.WriteError(w, 500, "swap oluşturulamadı: "+strings.TrimSpace(string(b)))
				return
			}
		}
		fstab, _ := os.ReadFile("/etc/fstab")
		if !strings.Contains(string(fstab), "/swapfile ") {
			f, e := os.OpenFile("/etc/fstab", os.O_APPEND|os.O_WRONLY, 0644)
			if e != nil {
				httpx.WriteError(w, 500, "swap etkin ancak fstab güncellenemedi")
				return
			}
			_, e = f.WriteString("/swapfile none swap sw 0 0\n")
			_ = f.Close()
			if e != nil {
				httpx.WriteError(w, 500, "swap etkin ancak fstab güncellenemedi")
				return
			}
		}
	}
	httpx.WriteJSON(w, 200, swapOku())
}
