package iceaktarim

import (
	"encoding/json"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"

	"sanalcp/internal/httpx"
	"sanalcp/internal/jailpath"
	"sanalcp/internal/sqlimport"

	"golang.org/x/sys/unix"
)

// maxConfigBayt: yapılandırma dosyası okuma sınırı. wp-config.php / .env
// birkaç KB'dır; bundan büyüğü ya bizim aradığımız dosya değildir ya da
// bozuktur — dokunmayız.
const maxConfigBayt = 1 << 20

// configAramaDerinlik: hedef dizinin altında kaç seviye alt klasöre bakılacağı.
// 1 seviye, "arşivi public_html'e açtım ama site public_html/wp/ altında"
// durumunu yakalamaya yeter; daha derini yanlış eşleşme üretir.
const configAramaDerinlik = 1

type configReq struct {
	DBAdi string `json:"db_name"`
	Dizin string `json:"dizin"` // ev dizinine göreli; boş → public_html
}

type configGuncelleme struct {
	Yol       string   `json:"yol"` // ev dizinine göreli
	Tur       string   `json:"tur"` // wordpress | laravel
	Alanlar   []string `json:"alanlar"`
	Uygulandi bool     `json:"uygulandi"`
	Not       string   `json:"not,omitempty"`
}

type configCevap struct {
	OK            bool               `json:"ok"`
	DBAdi         string             `json:"db_adi"`
	Guncellemeler []configGuncelleme `json:"guncellemeler"`
}

// StagingConfigGuncelle rewrites supported application configs after an
// internal clone. It intentionally shares the same symlink-safe writers as
// the interactive import flow.
func StagingConfigGuncelle(home, sk string, kimlik sqlimport.Hedef) []configGuncelleme {
	out := []configGuncelleme{}
	for _, dizin := range aramaDizinleri(home, "public_html") {
		if g, ok := wpGuncelle(home, sk, dizin, kimlik); ok {
			out = append(out, g)
		}
		if g, ok := envGuncelle(home, sk, dizin, kimlik); ok {
			out = append(out, g)
		}
		if g, ok := prestaShopGuncelle(home, sk, dizin, kimlik); ok {
			out = append(out, g)
		}
	}
	return out
}

// PrestaShop 1.7+ stores credentials in app/config/parameters.php; older
// releases use config/settings.inc.php. Both are plain PHP scalar values.
func prestaShopGuncelle(home, sk, dizin string, kimlik sqlimport.Hedef) (configGuncelleme, bool) {
	files := []string{path.Join(dizin, "app/config/parameters.php"), path.Join(dizin, "config/settings.inc.php")}
	for _, rel := range files {
		icerik, ok := dosyaOku(home, rel)
		if !ok {
			continue
		}
		metin := string(icerik)
		alanlar := []string{}
		values := map[string]string{"database_name": kimlik.DBAdi, "database_user": kimlik.Kullanici, "database_password": kimlik.Parola, "database_host": "localhost", "_DB_NAME_": kimlik.DBAdi, "_DB_USER_": kimlik.Kullanici, "_DB_PASSWD_": kimlik.Parola, "_DB_SERVER_": "localhost"}
		for key, value := range values {
			arrayRe := regexp.MustCompile(`(?m)(['"]` + regexp.QuoteMeta(key) + `['"]\s*=>\s*)['"][^'"\r\n]*['"]`)
			defineRe := regexp.MustCompile(`(?m)(define\s*\(\s*['"]` + regexp.QuoteMeta(key) + `['"]\s*,\s*)['"][^'"\r\n]*['"]`)
			yeni := "${1}'" + phpKacTemplate(value) + "'"
			if arrayRe.MatchString(metin) {
				metin = arrayRe.ReplaceAllString(metin, yeni)
				alanlar = append(alanlar, key)
			}
			if defineRe.MatchString(metin) {
				metin = defineRe.ReplaceAllString(metin, yeni)
				alanlar = append(alanlar, key)
			}
		}
		g := configGuncelleme{Yol: rel, Tur: "prestashop", Alanlar: alanlar}
		if len(alanlar) == 0 {
			g.Not = "DB alanları bulunamadı — elle güncelleyin"
			return g, true
		}
		if err := jailpath.DosyaYaz(home, rel, sk, []byte(metin), 0o640); err != nil {
			g.Not = "yazılamadı: " + err.Error()
			return g, true
		}
		g.Uygulandi = true
		return g, true
	}
	return configGuncelleme{}, false
}

// ConfigGuncelle — POST /domains/{id}/ice-aktarim/config
//
// Aktarılan uygulamanın veritabanı bağlantı ayarlarını hedefteki YENİ değerlerle
// günceller. Aktarımın en sık takıldığı yer burasıdır: dosyalar ve veriler
// gelir ama config eski sunucunun DB adı/kullanıcısını gösterdiği için site
// "Error establishing a database connection" verir.
//
// Desteklenen: WordPress (wp-config.php), Laravel (.env).
func (h *Handlers) ConfigGuncelle(w http.ResponseWriter, r *http.Request) {
	domainID, home, sk, err := h.domain(r)
	if err != nil {
		httpx.WriteError(w, durumKodu(err), err.Error())
		return
	}
	var req configReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	hedefDizin, err := hedefDogrula(req.Dizin)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	kimlik, err := h.dbHedefi(r, domainID, strings.TrimSpace(req.DBAdi))
	if err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	cevap := configCevap{OK: true, DBAdi: kimlik.DBAdi, Guncellemeler: []configGuncelleme{}}
	for _, dizin := range aramaDizinleri(home, hedefDizin) {
		if g, ok := wpGuncelle(home, sk, dizin, kimlik); ok {
			cevap.Guncellemeler = append(cevap.Guncellemeler, g)
		}
		if g, ok := envGuncelle(home, sk, dizin, kimlik); ok {
			cevap.Guncellemeler = append(cevap.Guncellemeler, g)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, cevap)
}

// aramaDizinleri: hedef dizin + bir seviye alt dizinleri (symlink-güvenli
// listelenir; symlink girdileri jailpath tarafından zaten açılamaz).
func aramaDizinleri(home, kok string) []string {
	out := []string{kok}
	if configAramaDerinlik < 1 {
		return out
	}
	adlar, err := jailpath.Adlari(home, kok)
	if err != nil {
		return out
	}
	for _, ad := range adlar {
		if strings.HasPrefix(ad, ".") {
			continue
		}
		alt := path.Join(kok, ad)
		if err := jailpath.DizinDogrula(home, alt); err == nil {
			out = append(out, alt)
		}
	}
	return out
}

// dosyaOku: home altındaki dosyayı symlink-güvenli okur.
func dosyaOku(home, rel string) ([]byte, bool) {
	f, err := jailpath.Ac(home, rel, unix.O_RDONLY, 0)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxConfigBayt {
		return nil, false
	}
	b, err := io.ReadAll(io.LimitReader(f, maxConfigBayt))
	if err != nil {
		return nil, false
	}
	return b, true
}

// ---- WordPress ----

// phpKac: PHP tek-tırnaklı dizge kaçışı (yalnız \ ve ' anlamlıdır).
func phpKac(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `'`, `\'`)
}

// wpDefineRe: define('SABIT', '<değer>') kalıbı. RE2 geri-referans desteklemediği
// için tırnak eşleşmesi alternatifle yazılır (tek/çift tırnak ayrı ayrı).
func wpDefineRe(sabit string) *regexp.Regexp {
	q := regexp.QuoteMeta(sabit)
	return regexp.MustCompile(
		`(?i)(define\s*\(\s*['"]` + q + `['"]\s*,\s*)` +
			`(?:'(?:[^'\\]|\\.)*'|"(?:[^"\\]|\\.)*")`)
}

var wpSabitleri = []struct {
	Ad string
	Al func(sqlimport.Hedef) string
}{
	{"DB_NAME", func(h sqlimport.Hedef) string { return h.DBAdi }},
	{"DB_USER", func(h sqlimport.Hedef) string { return h.Kullanici }},
	{"DB_PASSWORD", func(h sqlimport.Hedef) string { return h.Parola }},
	{"DB_HOST", func(sqlimport.Hedef) string { return "localhost" }},
}

func wpGuncelle(home, sk, dizin string, kimlik sqlimport.Hedef) (configGuncelleme, bool) {
	rel := path.Join(dizin, "wp-config.php")
	icerik, ok := dosyaOku(home, rel)
	if !ok {
		return configGuncelleme{}, false
	}
	g := configGuncelleme{Yol: rel, Tur: "wordpress", Alanlar: []string{}}
	metin := string(icerik)
	for _, s := range wpSabitleri {
		re := wpDefineRe(s.Ad)
		if !re.MatchString(metin) {
			continue
		}
		metin = re.ReplaceAllString(metin, "${1}'"+phpKacTemplate(s.Al(kimlik))+"'")
		g.Alanlar = append(g.Alanlar, s.Ad)
	}
	if len(g.Alanlar) == 0 {
		g.Not = "DB sabitleri düz metin olarak bulunamadı (getenv/değişken kullanılıyor olabilir) — elle güncelleyin."
		return g, true
	}
	if err := jailpath.DosyaYaz(home, rel, sk, []byte(metin), 0o640); err != nil {
		g.Not = "yazılamadı: " + err.Error()
		return g, true
	}
	g.Uygulandi = true
	return g, true
}

// phpKacTemplate: PHP kaçışına EK olarak Go regexp şablon kaçışı ($ → $$).
// Aksi hâlde parolada geçen "$1" replacement'ta grup referansı sanılırdı.
func phpKacTemplate(s string) string {
	return strings.ReplaceAll(phpKac(s), "$", "$$")
}

// ---- Laravel (.env) ----

var envAnahtarlari = []struct {
	Ad string
	Al func(sqlimport.Hedef) string
}{
	{"DB_CONNECTION", func(sqlimport.Hedef) string { return "mysql" }},
	{"DB_HOST", func(sqlimport.Hedef) string { return "localhost" }},
	{"DB_DATABASE", func(h sqlimport.Hedef) string { return h.DBAdi }},
	{"DB_USERNAME", func(h sqlimport.Hedef) string { return h.Kullanici }},
	{"DB_PASSWORD", func(h sqlimport.Hedef) string { return h.Parola }},
}

// envDegerKac: dotenv değeri. Boşluk/özel karakter içerenler çift tırnaklanır.
func envDegerKac(v string) string {
	if v == "" {
		return `""`
	}
	if strings.IndexFunc(v, func(r rune) bool {
		return r <= ' ' || strings.ContainsRune(`"'$\#`, r)
	}) < 0 {
		return v
	}
	esc := strings.ReplaceAll(v, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `"`, `\"`)
	esc = strings.ReplaceAll(esc, `$`, `\$`)
	return `"` + esc + `"`
}

func envGuncelle(home, sk, dizin string, kimlik sqlimport.Hedef) (configGuncelleme, bool) {
	rel := path.Join(dizin, ".env")
	icerik, ok := dosyaOku(home, rel)
	if !ok {
		return configGuncelleme{}, false
	}
	// Laravel doğrulaması: .env tek başına yeterli değil (birçok proje kullanır).
	// artisan varsa Laravel'dir; yoksa DB_* anahtarları var mı diye bakılır.
	laravel := false
	if _, varMi := dosyaOku(home, path.Join(dizin, "artisan")); varMi {
		laravel = true
	}
	if !laravel && !strings.Contains(string(icerik), "DB_DATABASE") {
		return configGuncelleme{}, false
	}

	g := configGuncelleme{Yol: rel, Tur: "laravel", Alanlar: []string{}}
	satirlar := strings.Split(string(icerik), "\n")
	for _, a := range envAnahtarlari {
		yeni := a.Ad + "=" + envDegerKac(a.Al(kimlik))
		bulundu := false
		for i, satir := range satirlar {
			kirp := strings.TrimSpace(satir)
			if strings.HasPrefix(kirp, "#") {
				continue
			}
			ad, _, ayrac := strings.Cut(kirp, "=")
			if !ayrac || strings.TrimSpace(ad) != a.Ad {
				continue
			}
			satirlar[i] = yeni
			bulundu = true
			break
		}
		if !bulundu {
			satirlar = append(satirlar, yeni)
		}
		g.Alanlar = append(g.Alanlar, a.Ad)
	}
	if err := jailpath.DosyaYaz(home, rel, sk, []byte(strings.Join(satirlar, "\n")), 0o640); err != nil {
		g.Not = "yazılamadı: " + err.Error()
		return g, true
	}
	g.Uygulandi = true
	return g, true
}
