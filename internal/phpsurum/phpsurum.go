// Package phpsurum: dinamik PHP surum kesfi + kur/kaldir
package phpsurum

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"sanalcp/internal/httpx"
	"sanalcp/internal/osfam"
)

// DesteklenenSurumler: panelin sunduğu PHP sürümleri. 🔴 5.6/7.0-7.3 EOL ve AlmaLinux 10
// Remi'de SAĞLANMAZ → listeden ÇIKARILDI (aksi halde "dnf No match for argument: php73-php-fpm").
// AlmaLinux 10 Remi'nin gerçekten sağladığı: 7.4, 8.0-8.6 (8.6 alpha) + AppStream native 8.3.
// Gerçek kurulabilirlik ayrıca RUNTIME'da dnf ile doğrulanır (Kurulabilir alanı, cache'li) →
// bir sürüm OS'tan kalkarsa panel zarif biçimde "kurulamaz" gösterir, ham dnf hatası patlamaz.
var DesteklenenSurumler = desteklenenSurumlerSec()

func desteklenenSurumlerSec() []SurumMeta {
	if osfam.Mevcut().DebianMi() {
		// deb.sury.org: her sürüm aynı kalıp, "sistem PHP'si" ayrıcalığı yok.
		// Kod alanı Debian'da sürümün kendisidir ("8.3"), Remi'deki gibi
		// sıkıştırılmış biçim ("83") DEĞİL — paket adları php8.3-fpm şeklindedir.
		return []SurumMeta{
			{"7.4", "7.4", "sury"},
			{"8.0", "8.0", "sury"},
			{"8.1", "8.1", "sury"},
			{"8.2", "8.2", "sury"},
			{"8.3", "8.3", "sury"},
			{"8.4", "8.4", "sury"},
			{"8.5", "8.5", "sury"},
			{"8.6", "8.6", "sury"},
		}
	}
	return []SurumMeta{
		{"7.4", "74", "remi"},
		{"8.0", "80", "remi"},
		{"8.1", "81", "remi"},
		{"8.2", "82", "remi"},
		{"8.3", "", "appstream"}, // AppStream native
		{"8.3", "83", "remi"},
		{"8.4", "84", "remi"},
		{"8.5", "85", "remi"},
		{"8.6", "86", "remi"},
	}
}

type SurumMeta struct {
	Surum string `json:"surum"`
	// Kod: paket adı eki. Remi'de sıkıştırılmış ("74", "82"), sury'de
	// sürümün kendisi ("7.4", "8.2") — paket adları buna göre kurulur.
	Kod string `json:"kod"`
	// Kaynak: "remi" | "appstream" (RHEL) · "sury" (Debian/Ubuntu)
	Kaynak string `json:"kaynak"`
}

type Surum struct {
	SurumMeta
	Yuklu       bool   `json:"yuklu"`
	Kurulabilir bool   `json:"kurulabilir"` // dnf'te (Remi/AppStream) mevcut mu — cache'li
	PoolDir     string `json:"pool_dir,omitempty"`
	SockDir     string `json:"sock_dir,omitempty"`
	Service     string `json:"service,omitempty"`
	PHPBin      string `json:"php_bin,omitempty"`
	GercekSurum string `json:"gercek_surum,omitempty"` // örn "8.3.31"
	ModulSayi   int    `json:"modul_sayi,omitempty"`
	Aciklama    string `json:"aciklama,omitempty"`
}

// ---- Kurulabilirlik cache'i ----
// 🔴 PERF: dnf shell-out'u pahalı (paket başına ~0.85s) ve dnf kilitli/yavaşken (ör. panel
// update dnf çalıştırırken) SANİYELERCE asılabilir. Eskiden paketMevcut() bunu İSTEK
// PATH'inde (senkron, 20s timeout) yapıyordu → TumSurumler() çağıran her endpoint (özellikle
// Domains sayfasının /php/versions'ı) takılıyordu. Artık dnf SADECE arka-plan sweeper'da
// çağrılır; istek path'i yalnızca cache OKUR, ASLA bloklamaz.
var (
	availMu     sync.Mutex
	availCache  = map[string]bool{} // pkg -> kurulabilir mi (arka-plan sweep doldurur)
	sweeperOnce sync.Once
	dnfProbe    = dnfPaketVar // test için enjekte edilebilir (varsayılan gerçek dnf)
)

const (
	availTTL   = 10 * time.Minute // arka-plan sweep periyodu
	dnfTimeout = 3 * time.Second  // her dnf sorgusu için üst sınır (yalnızca sweeper)
)

// StartAvailabilitySweeper: arka-plan dnf sweep döngüsünü (bir kez) başlatır. Sunucu
// açılışında main'den çağrılır; idempotent. İlk sweep ile periyodik yenilemeyi goroutine'de yapar.
func StartAvailabilitySweeper() {
	sweeperOnce.Do(func() { go sweepLoop() })
}

// sweepLoop: açılışta bir kez + her availTTL'de bir tüm Remi paketlerinin kurulabilirliğini
// dnf ile tarar ve availCache'i günceller. İstek path'inden BAĞIMSIZ çalışır.
func sweepLoop() {
	sweepOnce()
	t := time.NewTicker(availTTL)
	defer t.Stop()
	for range t.C {
		sweepOnce()
	}
}

// sweepOnce: tek bir dnf tarama turu. Sonucu availCache'e atomik yazar (kısmi güncelleme yok).
func sweepOnce() {
	yeni := map[string]bool{}
	for _, m := range DesteklenenSurumler {
		if m.Kaynak == "appstream" {
			continue // sistem PHP'si daima mevcut; paket yöneticisine sormaya gerek yok
		}
		pkg := fpmPaketAdi(m)
		if _, done := yeni[pkg]; done {
			continue
		}
		yeni[pkg] = dnfProbe(pkg)
	}
	availMu.Lock()
	availCache = yeni
	availMu.Unlock()
}

// dnfPaketVar: TEK paket için dnf sorgusu (yalnızca arka-plan sweeper'dan çağrılır).
// installed VEYA available → bulunursa dnf exit 0. Her sorgu dnfTimeout ile sınırlı; dnf
// kilitli/yavaşsa 3sn'de vazgeçer (istek path'ini etkilemez, sadece sweep'i sınırlar).
func dnfPaketVar(pkg string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), dnfTimeout)
	defer cancel()
	if osfam.PaketKurulu(ctx, pkg) {
		return true
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), dnfTimeout)
	defer cancel2()
	return osfam.PaketMevcut(ctx2, pkg)
}

// paketMevcut: phpXX-php-fpm paketi bu OS'ta (Remi) kurulabilir/kurulu mu?
// 🔴 İSTEK PATH'i — ASLA dnf çağırmaz, yalnızca cache okur. AppStream daima var.
// Cache boşsa (ilk boot, sweep henüz bitmemiş): makul varsayılan (false = "henüz bilinmiyor")
// döner ve sweeper'ı garanti eder; istek ASLA saniyelerce beklemez. Sweep bitince gerçek
// değer cache'e yazılır, sonraki istekler doğru sonucu anında alır.
func paketMevcut(m SurumMeta) bool {
	if m.Kaynak == "appstream" {
		return true // sistem default her zaman mevcut
	}
	StartAvailabilitySweeper() // idempotent; boot'ta main zaten başlatır, burada güvence
	pkg := fpmPaketAdi(m)
	availMu.Lock()
	v, ok := availCache[pkg]
	availMu.Unlock()
	if ok {
		return v
	}
	// Cache henüz dolmadı → istek bloklanmaz; varsayılan false. Sweep tamamlanınca düzelir.
	return false
}

// Yollar(meta): yuklenmis olsa olsa nerede olur
func yollar(m SurumMeta) (poolDir, sockDir, service, phpBin string) {
	switch m.Kaynak {
	case "sury":
		// deb.sury.org düzeni. Soketler tüm sürümlerde /run/php altındadır;
		// soket adı kiracıya göre verildiği için çakışma olmaz
		// (bkz. provisioner.phpMapDebian).
		return "/etc/php/" + m.Kod + "/fpm/pool.d",
			"/run/php",
			"php" + m.Kod + "-fpm",
			"/usr/bin/php" + m.Kod
	case "appstream":
		return "/etc/php-fpm.d", "/run/php-fpm", "php-fpm", "/usr/bin/php"
	}
	pre := "/opt/remi/php" + m.Kod + "/root"
	return "/etc/opt/remi/php" + m.Kod + "/php-fpm.d",
		"/var/opt/remi/php" + m.Kod + "/run/php-fpm",
		"php" + m.Kod + "-php-fpm",
		pre + "/usr/bin/php"
}

// kaldirDeseni: bir PHP sürümünün TÜM paketlerini eşleyen kaldırma deseni.
//
// 🔴 Aile farkı önemlidir. dnf glob'u shell tarzı yorumlar ("php74-*" beklendiği
// gibi eşleşir). apt ise özel karakter içeren argümanı REGEX sayar; "php8.3-*"
// regex olarak "php8" + herhangi bir karakter + "3-" (sıfır veya daha fazla '-')
// demektir ve istenmeyen paketleri eşleyebilir. Bu yüzden Debian'da nokta
// kaçırılmış, başa çapa konmuş açık bir regex üretilir.
func kaldirDeseni(m SurumMeta) string {
	if m.Kaynak == "sury" {
		return "^php" + strings.ReplaceAll(m.Kod, ".", `\.`) + "-"
	}
	return "php" + m.Kod + "-*"
}

// fpmPaketAdi: bir sürümün PHP-FPM paket adı (kurulum/kaldırma ve varlık sınama).
func fpmPaketAdi(m SurumMeta) string {
	if m.Kaynak == "sury" {
		return "php" + m.Kod + "-fpm"
	}
	return "php" + m.Kod + "-php-fpm"
}

// Discover: tek bir sürümün dolu metadata'sini doldur
func Discover(m SurumMeta) Surum {
	s := Surum{SurumMeta: m}
	s.PoolDir, s.SockDir, s.Service, s.PHPBin = yollar(m)
	// PHP binary varsa yüklü kabul
	if _, err := os.Stat(s.PHPBin); err == nil {
		s.Yuklu = true
		// Modül sayısı + gerçek sürüm
		if out, err := exec.Command(s.PHPBin, "-v").Output(); err == nil {
			line := strings.SplitN(string(out), "\n", 2)[0]
			// "PHP 8.3.31 (cli) ..."
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				s.GercekSurum = parts[1]
			}
		}
		if out, err := exec.Command(s.PHPBin, "-m").Output(); err == nil {
			lines := strings.Split(string(out), "\n")
			n := 0
			for _, ln := range lines {
				ln = strings.TrimSpace(ln)
				if ln != "" && !strings.HasPrefix(ln, "[") {
					n++
				}
			}
			s.ModulSayi = n
		}
	}
	if m.Kaynak == "appstream" {
		s.Aciklama = "Sistem default (AlmaLinux AppStream)"
	} else {
		s.Aciklama = "Remi modular — geliştirme/test/legacy"
	}
	// Kurulabilirlik: yüklüyse zaten kurulabilir; değilse dnf'e sor (cache'li).
	s.Kurulabilir = s.Yuklu || paketMevcut(m)
	// 🔴 AppStream girdisi "8.3" etiketini TAŞIR ama binary her zaman /usr/bin/php'dir —
	// AlmaLinux 10'da bu native olarak 8.3'tür, AlmaLinux 9'da ise kurulum betiğinin
	// modül akışını remi-8.3'e sabitlemesine bağlıdır (bkz. sanalcp-install.sh adım 3).
	// O sabitleme sessizce başarısız olursa (ör. Remi'ye ağ erişimi yoktu) sistem php'si
	// gerçekte 8.1/8.2 gibi başka bir sürüm olabilir — yüklü olması "8.3 kurulabilir"
	// anlamına GELMEZ, gerçek sürümü doğrula. Aksi hâlde müşteri "8.3" seçip sessizce
	// başka bir sürüm çalıştırır.
	if m.Kaynak == "appstream" && s.Yuklu && s.GercekSurum != "" && !strings.HasPrefix(s.GercekSurum, "8.3") {
		s.Kurulabilir = false
		s.Aciklama = "Sistem PHP'si 8.3 değil, " + s.GercekSurum + " (kurulum betiği modül akışını sabitleyemedi) — elle düzeltin: dnf module reset php -y && dnf module enable php:remi-8.3 -y && dnf install -y php php-fpm"
	}
	return s
}

// TumSurumler: desteklenen tüm sürümleri tara
func TumSurumler() []Surum {
	out := make([]Surum, 0, len(DesteklenenSurumler))
	for _, m := range DesteklenenSurumler {
		out = append(out, Discover(m))
	}
	// Yüklüleri öne, sonra sürüm sıralı (büyükten küçüğe)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Yuklu != out[j].Yuklu {
			return out[i].Yuklu
		}
		return surumKarsi(out[i].Surum, out[j].Surum) > 0
	})
	return out
}

func surumKarsi(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		ia, ib := 0, 0
		fmt.Sscanf(pa[i], "%d", &ia)
		fmt.Sscanf(pb[i], "%d", &ib)
		if ia != ib {
			return ia - ib
		}
	}
	return 0
}

// Default extension bundle (modern PHP icin)
var DefaultBundle = []string{
	"php-fpm",
	"php-cli",
	"php-mysqlnd",
	"php-mbstring",
	"php-bcmath",
	"php-intl",
	"php-gd",
	"php-soap",
	"php-opcache",
	"php-pdo",
	"php-xml",
	"php-zip",
	"php-pgsql",
	"php-ldap",
}

// suryBundle: Debian/Ubuntu (deb.sury.org) eklenti paketleri.
//
// 🔴 DefaultBundle'ın birebir çevirisi DEĞİLDİR; bazı eklentiler Debian'da
// başka adla paketlenir ya da hiç ayrı paket değildir:
//
//	php-mysqlnd -> mysql      (sürücü adı farklı)
//	php-pdo     -> (yok)      PDO çekirdekte, ayrı paket değil
//	php-opcache -> opcache    (php-cli/fpm ile birlikte gelir ama ayrı da vardır)
//	php-json    -> (yok)      PHP 8+'da çekirdekte
//
// Sürüm eki başa eklenerek "php<sürüm>-<eklenti>" üretilir.
var suryBundle = []string{
	"fpm",
	"cli",
	"mysql",
	"mbstring",
	"bcmath",
	"intl",
	"gd",
	"soap",
	"opcache",
	"xml",
	"zip",
	"pgsql",
	"ldap",
	"curl",
}

// PaketAdlari: bir sürüm için tüm paket isimlerini hazırla
func PaketAdlari(m SurumMeta) []string {
	if m.Kaynak == "sury" {
		out := make([]string, 0, len(suryBundle))
		for _, ek := range suryBundle {
			out = append(out, "php"+m.Kod+"-"+ek)
		}
		return out
	}
	pre := "php"
	if m.Kaynak == "remi" {
		pre = "php" + m.Kod + "-php"
	}
	out := make([]string, 0, len(DefaultBundle))
	for _, p := range DefaultBundle {
		out = append(out, strings.Replace(p, "php", pre, 1))
	}
	return out
}

// dnfHataOzet: dnf çıktısından anlamlı son satır(lar)ı süzer (tüm ham dökümü değil).
// "No match for argument" / "Error:" satırlarını öne çıkarır; hiçbiri yoksa son satır.
func dnfHataOzet(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var son string
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		son = ln
		low := strings.ToLower(ln)
		switch {
		// dnf
		case strings.Contains(low, "no match"),
			strings.HasPrefix(low, "error"),
			strings.Contains(low, "nothing provides"),
			// apt: "E: Unable to locate package php8.9-fpm",
			//      "E: Package 'x' has no installation candidate"
			strings.HasPrefix(ln, "E: "),
			strings.Contains(low, "unable to locate package"),
			strings.Contains(low, "no installation candidate"):
			return ln
		}
	}
	if son == "" {
		return "bilinmeyen paket yöneticisi hatası"
	}
	return son
}

// ----- HTTP -----

type Handlers struct {
	DB *sql.DB
}

func (h *Handlers) Liste(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"surumler": TumSurumler(),
	})
}

type opReq struct {
	Surum  string `json:"surum"`
	Kaynak string `json:"kaynak"`
}

func (h *Handlers) Kur(w http.ResponseWriter, r *http.Request) {
	var req opReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "gecersiz govde")
		return
	}
	var m SurumMeta
	for _, d := range DesteklenenSurumler {
		if d.Surum == req.Surum && d.Kaynak == req.Kaynak {
			m = d
			break
		}
	}
	if m.Surum == "" {
		httpx.WriteError(w, http.StatusBadRequest, "desteklenmeyen surum")
		return
	}

	// Zarif ön-kontrol: sürüm bu OS'ta (Remi) gerçekten sağlanıyor mu? EOL/kalkmış sürümlerde
	// ham "dnf No match for argument" dökümü yerine anlaşılır mesaj döneriz.
	if !paketMevcut(m) {
		httpx.WriteError(w, http.StatusConflict,
			fmt.Sprintf("PHP %s bu işletim sisteminde sağlanmıyor (Remi deposunda yok — büyük olasılıkla EOL). Kurulabilir bir sürüm seçin.", req.Surum))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	out, err := osfam.PaketKur(ctx, PaketAdlari(m)...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError,
			"PHP "+req.Surum+" kurulamadı: "+dnfHataOzet(string(out)))
		return
	}

	// Pool dizini yoksa olustur + default www.conf
	pd, _, svc, _ := yollar(m)
	_ = os.MkdirAll(pd, 0755)
	// Remi'de www.conf.disabled varsa aktive et
	if m.Kaynak == "remi" {
		dis := filepath.Join(pd, "www.conf.disabled")
		main := filepath.Join(pd, "www.conf")
		if _, err := os.Stat(dis); err == nil {
			_, _ = os.Stat(main) // varsa atla
			if _, err := os.Stat(main); err != nil {
				_ = os.Rename(dis, main)
			}
		}
	}
	// SanalCP default: buyuk form/import (phpMyAdmin, WordPress) icin max_input_vars
	phpdDir := "/etc/php.d"
	if m.Kaynak == "remi" {
		phpdDir = "/etc/opt/remi/php" + m.Kod + "/php.d"
	}
	if err := os.MkdirAll(phpdDir, 0755); err == nil {
		_ = os.WriteFile(filepath.Join(phpdDir, "99-sanalcp-input.ini"),
			[]byte("; SanalCP: buyuk form/import (phpMyAdmin, WordPress) - takilma onler\nmax_input_vars = 10000\n"), 0644)
	}

	// FPM servis enable + start
	_, _ = exec.Command("systemctl", "enable", "--now", svc).CombinedOutput()

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok":     true,
		"surum":  req.Surum,
		"kaynak": req.Kaynak,
		"output": string(out),
	})
}

func (h *Handlers) Kaldir(w http.ResponseWriter, r *http.Request) {
	var req opReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "gecersiz govde")
		return
	}
	if req.Kaynak == "appstream" {
		httpx.WriteError(w, http.StatusForbidden,
			"AppStream PHP sistemin default'u, kaldirilamaz")
		return
	}
	var m SurumMeta
	for _, d := range DesteklenenSurumler {
		if d.Surum == req.Surum && d.Kaynak == req.Kaynak {
			m = d
			break
		}
	}
	if m.Surum == "" || m.Kaynak != "remi" {
		httpx.WriteError(w, http.StatusBadRequest, "desteklenmeyen surum")
		return
	}

	// Bu sürümü kullanan domain var mı?
	var count int
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM domains WHERE php_surum=?`, req.Surum).Scan(&count)
	if count > 0 {
		httpx.WriteError(w, http.StatusConflict,
			fmt.Sprintf("Bu surumu kullanan %d domain var, once baska bir surume gec.", count))
		return
	}

	// FPM durdur
	_, svc, _, _ := yollar(m)
	_, _ = exec.Command("systemctl", "disable", "--now", svc).CombinedOutput()
	_ = svc

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	out, err := osfam.PaketSil(ctx, kaldirDeseni(m))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError,
			"PHP "+req.Surum+" kaldırılamadı: "+dnfHataOzet(string(out)))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"output": string(out),
	})
}
