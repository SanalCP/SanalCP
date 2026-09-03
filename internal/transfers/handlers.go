package transfers

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"sanalcp/internal/adlar"
	"sanalcp/internal/archivex"
	"sanalcp/internal/cron"
	"sanalcp/internal/domainek"
	"sanalcp/internal/domains"
	"sanalcp/internal/hesaplar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/mail"
	"sanalcp/internal/provisioner"
	"sanalcp/internal/subdomain"

	"github.com/go-chi/chi/v5"
)

const MaxUploadBytes = int64(20 << 30)

type Handlers struct {
	DB        *sql.DB
	Domains   *domains.Handlers
	Mail      *mail.Handlers
	Cron      *cron.Handlers
	Addon     *domainek.Handlers
	Subdomain *subdomain.Handlers
}

// Analyze accepts a cPanel full backup and returns an inventory. It never
// extracts or persists archive contents.
func (h *Handlers) Analyze(w http.ResponseWriter, r *http.Request) {
	// Büyük cPanel arşiv yüklemeleri sunucunun kısa varsayılan zaman aşımını
	// (bkz. cmd/server/main.go) aşabilir — bu uç için istisna açılır.
	httpx.ExtendDeadline(w, 30*time.Minute)
	// ExtendBodyLimit (r.Body = ... değil): global httpx.LimitBody gövdeyi zaten
	// 2 MiB'e sarmaladı, üstüne sarmak iç içe iki sınırın KÜÇÜĞÜNÜ geçerli kılardı.
	httpx.ExtendBodyLimit(w, r, MaxUploadBytes)
	// Envanter arşivi baştan sona sıralı okur, dolayısıyla yüklemeyi diske almaya
	// gerek yok: ParseMultipartForm 20 GiB'e kadar dosyayı geçici dizine kopyalardı.
	// MultipartReader gövdeyi doğrudan akış olarak verir.
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "çok parçalı (multipart) gövde gerekli")
		return
	}
	var f *multipart.Part
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "arşiv yüklenemedi veya boyut sınırı aşıldı")
			return
		}
		if part.FormName() == "archive" {
			f = part
			break
		}
		_ = part.Close()
	}
	if f == nil {
		httpx.WriteError(w, http.StatusBadRequest, "archive alanında cPanel .tar.gz yedeği gerekli")
		return
	}
	defer f.Close()
	low := strings.ToLower(f.FileName())
	if !strings.HasSuffix(low, ".tar.gz") && !strings.HasSuffix(low, ".tgz") {
		httpx.WriteError(w, http.StatusBadRequest, "ilk sürüm yalnız cPanel .tar.gz/.tgz tam yedeklerini destekliyor")
		return
	}
	inv, err := AnalyzeCPanel(f)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrArchiveTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		httpx.WriteError(w, status, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, inv)
}

type importResponse struct {
	OK           bool             `json:"ok"`
	DomainID     int64            `json:"domain_id"`
	Domain       string           `json:"domain"`
	SystemUser   string           `json:"system_user"`
	WebFiles     int              `json:"web_files"`
	Databases    []DBMap          `json:"databases"`
	Credentials  any              `json:"credentials"`
	Mailboxes    []MailCredential `json:"mailboxes"`
	Aliases      int              `json:"aliases"`
	CronJobs     int              `json:"cron_jobs"`
	SSLImported  bool             `json:"ssl_imported"`
	SSLExpires   string           `json:"ssl_expires,omitempty"`
	Skipped      []string         `json:"skipped"`
	Subdomains   int              `json:"subdomains"`
	AddonDomains int              `json:"addon_domains"`
	Source       Inventory        `json:"source"`
}

type MailCredential struct {
	Email             string `json:"email"`
	Password          string `json:"password"`
	PasswordPreserved bool   `json:"password_preserved,omitempty"`
}

type DBMap struct {
	Source string `json:"source"`
	Target string `json:"target"`
	User   string `json:"user"`
}

// Import creates a new SanalCP domain and restores the web root plus a
// cPanel databases. Additional databases share the domain's default DB user,
// matching SanalCP's supported one-user-to-many-databases model.
func (h *Handlers) Import(w http.ResponseWriter, r *http.Request) {
	if h.Domains == nil {
		httpx.WriteError(w, http.StatusInternalServerError, "domain sağlayıcısı hazır değil")
		return
	}
	// Büyük cPanel arşiv yüklemeleri sunucunun kısa varsayılan zaman aşımını
	// (bkz. cmd/server/main.go) aşabilir — bu uç için istisna açılır.
	httpx.ExtendDeadline(w, 30*time.Minute)
	// ExtendBodyLimit (r.Body = ... değil): global httpx.LimitBody gövdeyi zaten
	// 2 MiB'e sarmaladı, üstüne sarmak iç içe iki sınırın KÜÇÜĞÜNÜ geçerli kılardı.
	httpx.ExtendBodyLimit(w, r, MaxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "arşiv yüklenemedi veya boyut sınırı aşıldı")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	f, _, err := r.FormFile("archive")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "archive alanı zorunlu")
		return
	}
	defer f.Close()

	tmp, err := os.CreateTemp("", "sanalcp-cpanel-*.tar.gz")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "geçici arşiv oluşturulamadı")
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	_, copyErr := io.Copy(tmp, f)
	closeErr := tmp.Close() // hata yolunda da kapat: yoksa fd sızar
	if copyErr != nil || closeErr != nil {
		httpx.WriteError(w, http.StatusBadRequest, "arşiv kaydedilemedi")
		return
	}
	src, err := os.Open(tmpPath)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "arşiv açılamadı")
		return
	}
	inv, err := AnalyzeCPanel(src)
	src.Close()
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	domain := strings.ToLower(strings.TrimSpace(r.FormValue("domain")))
	if domain == "" {
		domain = inv.PrimaryDomain
	}
	if domain == "" {
		httpx.WriteError(w, http.StatusBadRequest, "ana domain belirlenemedi")
		return
	}
	customerID, err := requiredInt64(r.FormValue("customer_id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "customer_id zorunlu")
		return
	}
	phpVersion := strings.TrimSpace(r.FormValue("php_version"))
	if phpVersion == "" {
		phpVersion = "8.3"
	}
	var planID *int64
	if s := strings.TrimSpace(r.FormValue("plan_id")); s != "" {
		v, e := requiredInt64(s)
		if e != nil {
			httpx.WriteError(w, http.StatusBadRequest, "plan_id geçersiz")
			return
		}
		planID = &v
	}

	createBody, _ := json.Marshal(map[string]any{
		"alan_adi": domain, "php_surum": phpVersion,
		"customer_id": customerID, "plan_id": planID,
	})
	cr := httptest.NewRequest(http.MethodPost, "/api/v1/domains", bytes.NewReader(createBody)).
		WithContext(r.Context())
	cr.Header.Set("Content-Type", "application/json")
	cw := httptest.NewRecorder()
	h.Domains.Create(cw, cr)
	if cw.Code != http.StatusCreated {
		copyRecorded(w, cw)
		return
	}
	var created struct {
		ID              int64  `json:"id"`
		AlanAdi         string `json:"alan_adi"`
		SistemKullanici string `json:"sistem_kullanici"`
		DBAdi           string `json:"db_adi"`
		DBUser          string `json:"db_user"`
		Parolalar       struct {
			FTP string `json:"ftp"`
			DB  string `json:"db"`
		} `json:"olusturulan_parolalar"`
	}
	if err := json.Unmarshal(cw.Body.Bytes(), &created); err != nil || created.ID <= 0 {
		httpx.WriteError(w, http.StatusInternalServerError, "oluşturulan domain yanıtı okunamadı")
		return
	}
	committed := false
	defer func() {
		if !committed {
			h.rollbackDomain(r, created.ID)
		}
	}()

	// Arşivin küçük yardımcı üyeleri (SSL çifti + alias tablosu) tek geçişte
	// okunur; aşağıdaki adımların hiçbiri arşivi yeniden taramaz.
	ekler, err := okuArsivEkleri(tmpPath, inv)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "arşiv yardımcı dosyaları okunamadı: "+err.Error())
		return
	}

	if err := restoreWeb(tmpPath, inv.ArchiveRoot, created.SistemKullanici); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "web dosyaları aktarılamadı: "+err.Error())
		return
	}
	if _, native := ekler.uyeler[ekler.nativeDomain]; native {
		if err := restoreNativeHome(tmpPath, inv.ArchiveRoot, created.SistemKullanici); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı ana dizini dosyaları aktarılamadı: "+err.Error())
			return
		}
	}
	dbSources := nativeDatabaseSources(inv.Databases, ekler.uyeler[ekler.nativeDomain])
	dbMaps := databaseMappings(dbSources, created.SistemKullanici, created.DBAdi, created.DBUser)
	for i, m := range dbMaps {
		if i > 0 {
			if err := hesaplar.MySQLCreateDBForUser(h.DB, created.ID, m.Target, created.DBUser); err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "ek veritabanı oluşturulamadı: "+err.Error())
				return
			}
		}
	}
	if err := restoreDatabases(tmpPath, inv.ArchiveRoot, dbMaps); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "veritabanı aktarılamadı: "+err.Error())
		return
	}
	if err := rewriteApplicationDBConfigs(created.SistemKullanici, dbMaps, created.DBUser, created.Parolalar.DB); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "uygulama veritabanı ayarı güncellenemedi: "+err.Error())
		return
	}
	mailCreds, aliasCount, err := h.importMail(r, tmpPath, ekler, inv, created.ID, created.AlanAdi, created.SistemKullanici)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "e-posta aktarılamadı: "+err.Error())
		return
	}
	preserved, err := h.importNativeMetadata(r.Context(), ekler, created.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "SanalCP metadata aktarılamadı: "+err.Error())
		return
	}
	subCount, addonCount, err := h.importNativeChildDomains(r, tmpPath, ekler, inv, created.ID, created.SistemKullanici)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "alt/ek alan adları aktarılamadı: "+err.Error())
		return
	}
	for i := range mailCreds {
		if preserved[mailCreds[i].Email] {
			mailCreds[i].Password = ""
			mailCreds[i].PasswordPreserved = true
		}
	}
	cronCount, err := h.importCron(r, inv, created.ID, created.SistemKullanici)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "cron görevleri aktarılamadı: "+err.Error())
		return
	}
	sslImported, sslExpires, sslWarning, err := h.importSSL(
		r, ekler, inv, created.ID, created.AlanAdi,
	)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "SSL sertifikası aktarılamadı: "+err.Error())
		return
	}
	skipped := []string{}
	if sslWarning != "" {
		skipped = append(skipped, sslWarning)
	}
	if not := homeAtlananlarOzeti(ekler); not != "" {
		skipped = append(skipped, not)
	}
	committed = true
	httpx.WriteJSON(w, http.StatusCreated, importResponse{
		OK: true, DomainID: created.ID, Domain: created.AlanAdi,
		SystemUser: created.SistemKullanici, WebFiles: inv.WebFiles,
		Databases: dbMaps, Credentials: created.Parolalar, Mailboxes: mailCreds,
		Aliases: aliasCount, CronJobs: cronCount, SSLImported: sslImported,
		SSLExpires: sslExpires, Source: inv, Skipped: skipped,
		Subdomains: subCount, AddonDomains: addonCount,
	})
}

var wpDBNameRE = regexp.MustCompile(`(?m)define\s*\(\s*['"]DB_NAME['"]\s*,\s*['"]([^'"]+)['"]\s*\)\s*;`)
var wpDBUserRE = regexp.MustCompile(`(?m)define\s*\(\s*['"]DB_USER['"]\s*,\s*['"][^'"]*['"]\s*\)\s*;`)
var wpDBPassRE = regexp.MustCompile(`(?m)define\s*\(\s*['"]DB_PASSWORD['"]\s*,\s*['"][^'"]*['"]\s*\)\s*;`)

func rewriteWordPressDBConfig(sk string, maps []DBMap, dbUser, dbPass string) error {
	p := "/home/" + sk + "/public_html/wp-config.php"
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(b) > 2<<20 {
		return errors.New("wp-config.php çok büyük")
	}
	m := wpDBNameRE.FindSubmatch(b)
	if len(m) != 2 {
		return nil
	}
	targetDB := ""
	for _, dm := range maps {
		if dm.Source == string(m[1]) {
			targetDB = dm.Target
			break
		}
	}
	if targetDB == "" {
		return fmt.Errorf("wp-config.php veritabanı dump listesinde yok: %s", m[1])
	}
	escape := func(s string) string {
		s = strings.ReplaceAll(s, `\`, `\\`)
		return strings.ReplaceAll(s, `'`, `\'`)
	}
	b = wpDBNameRE.ReplaceAll(b, []byte("define('DB_NAME', '"+escape(targetDB)+"');"))
	b = wpDBUserRE.ReplaceAll(b, []byte("define('DB_USER', '"+escape(dbUser)+"');"))
	b = wpDBPassRE.ReplaceAll(b, []byte("define('DB_PASSWORD', '"+escape(dbPass)+"');"))
	return os.WriteFile(p, b, 0o600)
}

// homeAtlananlarOzeti — kaynak ana dizininde AKTARILMAYAN dizinlerin listesi.
// Panel modeli uygulamanın public_html içinde durmasını varsayar; web kökünün
// üstünde kod tutan siteler (ör. src/ + vendor/ + bootstrap.php) hedefte
// sessizce kırılıyordu: aktarım "başarılı" görünüyor, site fatal veriyordu.
// Kapsamı büyütmek yerine ne bırakıldığını AÇIKÇA bildiriyoruz.
func homeAtlananlarOzeti(ekler arsivEkler) string {
	raw, ok := ekler.uyeler[ekler.nativeHomeAtla]
	if !ok {
		return ""
	}
	adlar := []string{}
	for _, satir := range strings.Split(string(raw), "\n") {
		satir = strings.TrimSpace(satir)
		// İçerik KAYNAK sunucudan gelir; mesaja yalnız güvenli adlar girsin.
		if !homeDizinAdiRE.MatchString(satir) {
			continue
		}
		adlar = append(adlar, satir+"/")
		if len(adlar) == 12 {
			adlar = append(adlar, "…")
			break
		}
	}
	if len(adlar) == 0 {
		return ""
	}
	return "kaynak ana dizinindeki şu dizinler aktarılmadı (yalnız public_html taşınır; gerekiyorsa elle kopyalayın): " + strings.Join(adlar, ", ")
}

var homeDizinAdiRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// hedefDBHost — panel MySQL hesaplarını YALNIZ '<kullanıcı>@localhost' olarak
// açar. Yapılandırmaya 127.0.0.1 yazmak TCP bağlantısı demektir; o host için
// yetki kaydı olmadığından uygulama "Access denied (1698)" alır. Panelin ve
// yöneticinin `mysql -u … -p…` denemeleri sokete gittiği için bu SESSİZ kalıyor,
// yalnız site kırılıyordu (bkz. ajanda.uygulamasi.tr, 2026-09-03).
const hedefDBHost = "localhost"

func rewriteApplicationDBConfigs(sk string, maps []DBMap, dbUser, dbPass string) error {
	if err := rewriteWordPressDBConfig(sk, maps, dbUser, dbPass); err != nil {
		return err
	}
	web := "/home/" + sk + "/public_html/"
	// PHP yapılandırmaları aynı bilgiyi const / define / dizi biçimlerinden
	// herhangi biriyle tutabiliyor; hepsi denenir, eşleşmeyen dosyayı
	// değiştirmeden döner.
	phpBicimleri := []dbConfigRewriter{rewritePHPConstDB, rewritePHPDefineDB, rewritePHPArrayDB}
	hedefler := []struct {
		yol  string
		adim []dbConfigRewriter
	}{
		{web + ".env", []dbConfigRewriter{rewriteDotEnvDB}},
		{web + "config.php", phpBicimleri},
		{web + "config.local.php", phpBicimleri},
		{web + "app/config/parameters.php", []dbConfigRewriter{rewritePrestaParametersDB}},
		{web + "config/settings.inc.php", []dbConfigRewriter{rewritePrestaLegacyDB}},
		// Web kökünün ÜSTÜ: exporter bu düz dosyaları native_home ile taşıyor.
		// Taşıdığımız hâlde DB bilgilerini uyarlamazsak dosya hedefe KAYNAK
		// sunucunun parolasıyla gider ve uygulama bağlanamaz.
		{"/home/" + sk + "/.env", []dbConfigRewriter{rewriteDotEnvDB}},
		{"/home/" + sk + "/config.php", phpBicimleri},
		{"/home/" + sk + "/config.local.php", phpBicimleri},
	}
	for _, h := range hedefler {
		for _, fn := range h.adim {
			if err := rewriteDBConfigFile(h.yol, maps, dbUser, dbPass, fn); err != nil {
				return fmt.Errorf("%s: %w", strings.TrimPrefix(h.yol, "/home/"+sk+"/"), err)
			}
		}
	}
	// Eski framework cache'leri mutlak yol, eski DB bilgisi ve eski sunucunun
	// zaman damgalarını taşır. Uygulama ilk istekte kendi kullanıcısıyla yeniden üretir.
	_ = os.RemoveAll(web + "var/cache")
	for _, p := range []string{"bootstrap/cache/config.php", "bootstrap/cache/packages.php", "bootstrap/cache/services.php"} {
		_ = os.Remove(web + p)
	}
	return nil
}

type dbConfigRewriter func([]byte, []DBMap, string, string) ([]byte, bool, error)

func rewriteDBConfigFile(p string, maps []DBMap, user, pass string, fn dbConfigRewriter) error {
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(b) > 2<<20 {
		return errors.New("yapılandırma dosyası çok büyük")
	}
	out, changed, err := fn(b, maps, user, pass)
	if err != nil || !changed {
		return err
	}
	return os.WriteFile(p, out, 0o600)
}

func mappedDB(source string, maps []DBMap) (string, error) {
	for _, m := range maps {
		if m.Source == source {
			return m.Target, nil
		}
	}
	return "", fmt.Errorf("veritabanı dump listesinde yok: %s", source)
}

var envDBNameRE = regexp.MustCompile(`(?m)^DB_DATABASE\s*=\s*["']?([^\s"']+)["']?\s*$`)
var envDBUserRE = regexp.MustCompile(`(?m)^DB_USERNAME\s*=.*$`)
var envDBPassRE = regexp.MustCompile(`(?m)^DB_PASSWORD\s*=.*$`)
var envDBHostRE = regexp.MustCompile(`(?m)^DB_HOST\s*=.*$`)
var symfonyDBURLRE = regexp.MustCompile(`(?m)^DATABASE_URL\s*=\s*["']?[^\s"']+/([^?\s"']+)(\?[^\s"']*)?["']?\s*$`)

// DİKKAT: replacePHPValue eşleşmenin TAMAMINI grup 1 + yeni değerle değiştirir.
// Bu yüzden desenler sonlandırıcı ';' KAPSAMAMALIDIR; kapsarsa noktalı virgül
// silinir ve config.php parse error verip site HTTP 500'e düşer.
var phpConstDBNameRE = regexp.MustCompile(`(?m)(\bconst\s+DB_NAME\s*=\s*)['"]([^'"]+)['"]`)
var phpConstDBUserRE = regexp.MustCompile(`(?m)(\bconst\s+DB_USER\s*=\s*)['"][^'"]*['"]`)
var phpConstDBPassRE = regexp.MustCompile(`(?m)(\bconst\s+DB_PASS(?:WORD)?\s*=\s*)['"][^'"]*['"]`)
var phpConstDBHostRE = regexp.MustCompile(`(?m)(\bconst\s+DB_HOST\s*=\s*)['"][^'"]*['"]`)

func rewriteDotEnvDB(b []byte, maps []DBMap, user, pass string) ([]byte, bool, error) {
	m := envDBNameRE.FindSubmatch(b)
	if len(m) != 2 {
		sm := symfonyDBURLRE.FindSubmatch(b)
		if len(sm) != 3 {
			return b, false, nil
		}
		target, err := mappedDB(string(sm[1]), maps)
		if err != nil {
			return nil, false, err
		}
		u := &url.URL{Scheme: "mysql", User: url.UserPassword(user, pass), Host: hedefDBHost + ":3306", Path: "/" + target, RawQuery: strings.TrimPrefix(string(sm[2]), "?")}
		return replaceWholeMatch(b, symfonyDBURLRE, "DATABASE_URL="+u.String()), true, nil
	}
	target, err := mappedDB(string(m[1]), maps)
	if err != nil {
		return nil, false, err
	}
	quote := func(s string) string {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "\n", `\n`).Replace(s) + `"`
	}
	b = replaceWholeMatch(b, envDBNameRE, "DB_DATABASE="+quote(target))
	b = replaceWholeMatch(b, envDBUserRE, "DB_USERNAME="+quote(user))
	b = replaceWholeMatch(b, envDBPassRE, "DB_PASSWORD="+quote(pass))
	b = replaceWholeMatch(b, envDBHostRE, "DB_HOST="+hedefDBHost)
	return b, true, nil
}

func replaceWholeMatch(b []byte, re *regexp.Regexp, value string) []byte {
	return re.ReplaceAllFunc(b, func([]byte) []byte { return []byte(value) })
}

func rewritePHPConstDB(b []byte, maps []DBMap, user, pass string) ([]byte, bool, error) {
	m := phpConstDBNameRE.FindSubmatch(b)
	if len(m) != 3 {
		return b, false, nil
	}
	target, err := mappedDB(string(m[2]), maps)
	if err != nil {
		return nil, false, err
	}
	b = replacePHPValue(b, phpConstDBNameRE, target)
	b = replacePHPValue(b, phpConstDBUserRE, user)
	b = replacePHPValue(b, phpConstDBPassRE, pass)
	b = replacePHPValue(b, phpConstDBHostRE, hedefDBHost)
	return b, true, nil
}

func phpArrayValueRE(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)(['"]` + key + `['"]\s*=>\s*)['"][^'"]*['"]`)
}
func phpDefineValueRE(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)(define\s*\(\s*['"]` + key + `['"]\s*,\s*)['"][^'"]*['"]`)
}
func phpQuoted(s string) string {
	return `'` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `'`, `\'`) + `'`
}
func replacePHPValue(b []byte, re *regexp.Regexp, value string) []byte {
	return re.ReplaceAllFunc(b, func(match []byte) []byte {
		parts := re.FindSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		return append(append([]byte{}, parts[1]...), []byte(phpQuoted(value))...)
	})
}

// rewritePHPDefineDB — define('DB_NAME', '…') biçimi. wp-config.php dışındaki
// elle yazılmış config.php dosyalarında yaygın; const biçimini tanıyıp bunu
// atlamak siteyi kaynak sunucunun parolasıyla bırakıyordu.
func rewritePHPDefineDB(b []byte, maps []DBMap, user, pass string) ([]byte, bool, error) {
	re := phpDefineValueRE("DB_NAME")
	target, ok, err := phpIlkDegerdenHedef(b, re, maps)
	if err != nil || !ok {
		return b, false, err
	}
	b = replacePHPValue(b, re, target)
	b = replacePHPValue(b, phpDefineValueRE("DB_USER"), user)
	// DB_PASS deseni DB_PASSWORD'ü YAKALAMAZ: ad hemen tırnakla biter.
	b = replacePHPValue(b, phpDefineValueRE("DB_PASS"), pass)
	b = replacePHPValue(b, phpDefineValueRE("DB_PASSWORD"), pass)
	b = replacePHPValue(b, phpDefineValueRE("DB_HOST"), hedefDBHost)
	return b, true, nil
}

// rewritePHPArrayDB — `return ['db_name' => '…', 'db_pass' => '…']` biçimi
// (ör. web kökü üstündeki config.local.php).
func rewritePHPArrayDB(b []byte, maps []DBMap, user, pass string) ([]byte, bool, error) {
	re := phpArrayValueRE("db_name")
	target, ok, err := phpIlkDegerdenHedef(b, re, maps)
	if err != nil || !ok {
		return b, false, err
	}
	b = replacePHPValue(b, re, target)
	b = replacePHPValue(b, phpArrayValueRE("db_user"), user)
	b = replacePHPValue(b, phpArrayValueRE("db_pass"), pass)
	b = replacePHPValue(b, phpArrayValueRE("db_password"), pass)
	b = replacePHPValue(b, phpArrayValueRE("db_host"), hedefDBHost)
	return b, true, nil
}

// phpIlkDegerdenHedef — desenin ilk eşleşmesindeki tırnaklı değeri kaynak DB
// adı sayıp dump listesinden hedef adı çözer.
func phpIlkDegerdenHedef(b []byte, re *regexp.Regexp, maps []DBMap) (string, bool, error) {
	m := re.FindSubmatch(b)
	if len(m) == 0 {
		return "", false, nil
	}
	vm := phpSonDegerRE.FindSubmatch(m[0])
	if len(vm) != 2 {
		return "", false, nil
	}
	target, err := mappedDB(string(vm[1]), maps)
	if err != nil {
		return "", false, err
	}
	return target, true, nil
}

var phpSonDegerRE = regexp.MustCompile(`['"]([^'"]*)['"]\s*$`)

func rewritePrestaParametersDB(b []byte, maps []DBMap, user, pass string) ([]byte, bool, error) {
	re := phpArrayValueRE("database_name")
	m := re.FindSubmatch(b)
	if len(m) == 0 {
		return b, false, nil
	}
	valueRE := regexp.MustCompile(`['"]([^'"]*)['"]\s*$`)
	vm := valueRE.FindSubmatch(m[0])
	if len(vm) != 2 {
		return b, false, nil
	}
	target, err := mappedDB(string(vm[1]), maps)
	if err != nil {
		return nil, false, err
	}
	b = replacePHPValue(b, re, target)
	b = replacePHPValue(b, phpArrayValueRE("database_user"), user)
	b = replacePHPValue(b, phpArrayValueRE("database_password"), pass)
	b = replacePHPValue(b, phpArrayValueRE("database_host"), hedefDBHost)
	return b, true, nil
}

func rewritePrestaLegacyDB(b []byte, maps []DBMap, user, pass string) ([]byte, bool, error) {
	re := phpDefineValueRE("_DB_NAME_")
	m := re.FindSubmatch(b)
	if len(m) == 0 {
		return b, false, nil
	}
	valueRE := regexp.MustCompile(`['"]([^'"]*)['"]\s*$`)
	vm := valueRE.FindSubmatch(m[0])
	if len(vm) != 2 {
		return b, false, nil
	}
	target, err := mappedDB(string(vm[1]), maps)
	if err != nil {
		return nil, false, err
	}
	b = replacePHPValue(b, re, target)
	b = replacePHPValue(b, phpDefineValueRE("_DB_USER_"), user)
	b = replacePHPValue(b, phpDefineValueRE("_DB_PASSWD_"), pass)
	b = replacePHPValue(b, phpDefineValueRE("_DB_SERVER_"), hedefDBHost)
	return b, true, nil
}

func (h *Handlers) importSSL(r *http.Request, ekler arsivEkler, inv Inventory, domainID int64, targetDomain string) (bool, string, string, error) {
	if inv.SSLCerts == 0 {
		return false, "", "", nil
	}
	certPEM, keyPEM, err := ekler.sslCifti()
	if errors.Is(err, errMemberNotFound) {
		return false, "", "Kaynak SSL sertifikası için eşleşen özel anahtar bulunamadı; SSL aktarılmadı.", nil
	}
	if err != nil {
		return false, "", "", err
	}
	certPath, keyPath, expires, err := provisioner.InstallImportedSSL(targetDomain, certPEM, keyPEM)
	if errors.Is(err, provisioner.ErrImportedSSLInvalid) {
		return false, "", err.Error() + "; SSL aktarılmadı.", nil
	}
	if err != nil {
		return false, "", "", err
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET ssl_aktif=1, ssl_kaynak='imported', cert_path=?, key_path=?, ssl_bitis=? WHERE id=?`,
		certPath, keyPath, expires, domainID); err != nil {
		return false, "", "", err
	}
	if err := provisioner.RerenderVhost(h.DB, domainID); err != nil {
		return false, "", "", err
	}
	return true, expires.UTC().Format("2006-01-02"), "", nil
}

// arsivEkler — arşivin küçük yardımcı üyeleri (SSL çifti, alias tablosu). Hepsi
// tek geçişte okunur; bkz. readSmallTarMembers.
type arsivEkler struct {
	certAdaylari   []string
	keyAdaylari    []string
	bundleAdaylari []string
	aliasUyesi     string
	nativeDomain   string
	nativeDNS      string
	nativeMailbox  string
	nativeSecurity string
	nativeRedirect string
	nativeIPRules  string
	nativeNginx    string
	nativeRate     string
	nativeSpam     string
	nativeAuto     string
	nativeFilters  string
	nativeSubs     string
	nativeAddons   string
	nativeAddonDNS string
	nativeHomeAtla string
	uyeler         map[string][]byte
}

func okuArsivEkleri(archivePath string, inv Inventory) (arsivEkler, error) {
	e := arsivEkler{uyeler: map[string][]byte{}}
	root, domain := inv.ArchiveRoot, inv.PrimaryDomain
	if domain == "" {
		return e, nil
	}
	e.certAdaylari = []string{
		root + "/sslcerts/" + domain + ".crt",
		root + "/homedir/ssl/certs/" + domain + ".crt",
		root + "/homedir/ssl/" + domain + ".crt",
	}
	e.keyAdaylari = []string{
		root + "/sslkeys/" + domain + ".key",
		root + "/homedir/ssl/private/" + domain + ".key",
		root + "/homedir/ssl/" + domain + ".key",
	}
	e.bundleAdaylari = []string{
		root + "/sslcerts/" + domain + ".cabundle",
		root + "/homedir/ssl/certs/" + domain + ".cabundle",
		root + "/homedir/ssl/" + domain + ".cabundle",
	}
	e.aliasUyesi = root + "/va/" + domain
	e.nativeDomain = root + "/sanalcp/domain.json"
	e.nativeDNS = root + "/sanalcp/dns.jsonl"
	e.nativeMailbox = root + "/sanalcp/mailboxes.jsonl"
	e.nativeSecurity = root + "/sanalcp/security.json"
	e.nativeRedirect = root + "/sanalcp/redirect.json"
	e.nativeIPRules = root + "/sanalcp/ip_rules.jsonl"
	e.nativeNginx = root + "/sanalcp/nginx.json"
	e.nativeRate = root + "/sanalcp/rate_limit.json"
	e.nativeSpam = root + "/sanalcp/mail_spam.json"
	e.nativeAuto = root + "/sanalcp/autoresponders.jsonl"
	e.nativeFilters = root + "/sanalcp/mail_filters.jsonl"
	e.nativeSubs = root + "/sanalcp/subdomains.jsonl"
	e.nativeAddons = root + "/sanalcp/addon_domains.jsonl"
	e.nativeAddonDNS = root + "/sanalcp/addon_dns.jsonl"
	e.nativeHomeAtla = root + "/sanalcp/home_skipped.txt"

	istekler := append([]string{}, e.certAdaylari...)
	istekler = append(istekler, e.keyAdaylari...)
	istekler = append(istekler, e.bundleAdaylari...)
	istekler = append(istekler, e.aliasUyesi)
	istekler = append(istekler, e.nativeDomain, e.nativeDNS, e.nativeMailbox)
	istekler = append(istekler, e.nativeSecurity, e.nativeRedirect, e.nativeIPRules,
		e.nativeNginx, e.nativeRate, e.nativeSpam, e.nativeAuto, e.nativeFilters)
	istekler = append(istekler, e.nativeSubs, e.nativeAddons, e.nativeAddonDNS, e.nativeHomeAtla)
	uyeler, err := readSmallTarMembersMatching(archivePath, istekler, []string{root + "/sanalcp/addons/"})
	if err != nil {
		return e, err
	}
	e.uyeler = uyeler
	return e, nil
}

func (e arsivEkler) ilk(adaylar []string) ([]byte, bool) {
	for _, aday := range adaylar {
		if body, ok := e.uyeler[aday]; ok && len(body) > 0 {
			return body, true
		}
	}
	return nil, false
}

func (e arsivEkler) sslCifti() ([]byte, []byte, error) {
	certPEM, ok := e.ilk(e.certAdaylari)
	if !ok {
		return nil, nil, errMemberNotFound
	}
	keyPEM, ok := e.ilk(e.keyAdaylari)
	if !ok {
		return nil, nil, errMemberNotFound
	}
	if bundle, ok := e.ilk(e.bundleAdaylari); ok {
		certPEM = append(append(append([]byte{}, certPEM...), '\n'), bundle...)
	}
	return certPEM, keyPEM, nil
}

func (h *Handlers) importCron(r *http.Request, inv Inventory, domainID int64, targetUser string) (int, error) {
	if len(inv.CronJobs) == 0 {
		return 0, nil
	}
	if h.Cron == nil {
		return 0, errors.New("cron sağlayıcısı hazır değil")
	}
	created := 0
	for _, job := range inv.CronJobs {
		command := job.Command
		if inv.Username != "" {
			command = strings.ReplaceAll(command, "/home/"+inv.Username+"/", "/home/"+targetUser+"/")
		}
		body, _ := json.Marshal(map[string]string{
			"dakika": job.Minute, "saat": job.Hour, "gun": job.Day,
			"ay": job.Month, "hafta": job.Weekday,
			"komut": command, "yorum": job.Comment,
		})
		req := domainRequest(r, http.MethodPost, "/cron", domainID, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Cron.Create(rr, req)
		if rr.Code != http.StatusCreated {
			return 0, fmt.Errorf("%d. görev: %s", created+1, strings.TrimSpace(rr.Body.String()))
		}
		created++
	}
	return created, nil
}

func (h *Handlers) importMail(r *http.Request, archivePath string, ekler arsivEkler, inv Inventory, domainID int64, targetDomain, sk string) ([]MailCredential, int, error) {
	if len(inv.Mailboxes) == 0 && inv.AliasCount == 0 && inv.MailFiles == 0 {
		return []MailCredential{}, 0, nil
	}
	if h.Mail == nil {
		return nil, 0, errors.New("mail sağlayıcısı hazır değil")
	}
	if err := mail.MailUygula(r.Context(), h.DB, domainID); err != nil {
		return nil, 0, err
	}
	creds := make([]MailCredential, 0, len(inv.Mailboxes))
	yerelAdlar := make([]string, 0, len(inv.Mailboxes))
	for _, local := range inv.Mailboxes {
		body, _ := json.Marshal(map[string]string{"local_part": local})
		req := domainRequest(r, http.MethodPost, "/mail", domainID, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Mail.Ekle(rr, req)
		if rr.Code != http.StatusCreated {
			return nil, 0, fmt.Errorf("%s kutusu: %s", local, strings.TrimSpace(rr.Body.String()))
		}
		var result struct {
			Email  string `json:"email"`
			Parola string `json:"parola"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			return nil, 0, err
		}
		creds = append(creds, MailCredential{Email: result.Email, Password: result.Parola})
		yerelAdlar = append(yerelAdlar, local)
	}
	if inv.PrimaryDomain != "" && len(yerelAdlar) > 0 {
		if err := restoreMailboxes(archivePath, inv.ArchiveRoot, inv.PrimaryDomain, yerelAdlar, sk); err != nil {
			return nil, 0, fmt.Errorf("posta mesajları: %w", err)
		}
	}

	aliases := readAliases(ekler, inv.PrimaryDomain, targetDomain)
	created := 0
	for _, a := range aliases {
		body, _ := json.Marshal(map[string]string{"local_part": a.Local, "destination": a.Destination})
		req := domainRequest(r, http.MethodPost, "/mail/aliases", domainID, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Mail.AliasEkle(rr, req)
		if rr.Code == http.StatusCreated {
			created++
			continue
		}
		return nil, 0, fmt.Errorf("%s aliası: %s", a.Local, strings.TrimSpace(rr.Body.String()))
	}
	return creds, created, nil
}

func domainRequest(parent *http.Request, method, url string, domainID int64, body io.Reader) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", strconv.FormatInt(domainID, 10))
	ctx := context.WithValue(parent.Context(), chi.RouteCtxKey, rc)
	req := httptest.NewRequest(method, url, body).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// restoreMailboxes — bütün Maildir'leri TEK tar çağrısıyla çıkarır.
//
// Kutu başına ayrı çağrı, 20 GiB'lık arşivi kutu sayısı kadar baştan açıyordu.
// Üye yolu root/homedir/mail/<kaynak-domain>/<local> olduğundan 4 bileşen atılıp
// hedef /home/<sk>/mail seçilirse her kutu tam da kendi dizinine düşer.
func restoreMailboxes(archivePath, root, sourceDomain string, locals []string, sk string) error {
	if !adlar.SKGecerli(sk) || root == "" {
		return errors.New("güvensiz hedef")
	}
	// Katman 2: sistem tar'ına geçmeden önce arşivi Go stdlib ile ön-tara —
	// mutlak yol / ".." / symlink-hardlink-aygıt üyesi varsa çıkarma reddedilir.
	// (archivex.GuvenliCikar'ın kullandığı aynı ortak taramadır.)
	if err := archivex.Tara(archivePath, archivex.TurTarGz); err != nil {
		return err
	}
	target := "/home/" + sk + "/mail"
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	args := []string{"-u", sk, "--", "tar", "-xz", "-f", "-", "-C", target, "--strip-components=4"}
	for _, local := range locals {
		args = append(args, root+"/homedir/mail/"+sourceDomain+"/"+local)
	}
	cmd := exec.Command("runuser", args...)
	cmd.Stdin = f
	if out, err := cmd.CombinedOutput(); err != nil {
		// Metadata'da görünen boş bir kutunun arşivde Maildir'i olmayabilir; tar
		// bunu ölümcül saymaz ama çıkış kodunu bozar. Diğer kutular yine açılmıştır.
		if strings.Contains(string(out), "Not found in archive") || strings.Contains(string(out), "Not found") {
			_, _ = exec.Command("restorecon", "-RF", target).CombinedOutput()
			return nil
		}
		return fmt.Errorf("tar: %s", strings.TrimSpace(string(out)))
	}
	_, _ = exec.Command("restorecon", "-RF", target).CombinedOutput()
	return nil
}

type aliasImport struct {
	Local       string
	Destination string
}

func readAliases(ekler arsivEkler, sourceDomain, targetDomain string) []aliasImport {
	out := []aliasImport{}
	if sourceDomain == "" {
		return out
	}
	body, ok := ekler.uyeler[ekler.aliasUyesi]
	if !ok {
		return out
	}
	for _, line := range strings.Split(string(body), "\n") {
		p := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(p) != 2 {
			continue
		}
		source := strings.TrimSpace(p[0])
		destRaw := strings.TrimSpace(p[1])
		if source == "" || destRaw == "" || strings.HasPrefix(destRaw, ":") || strings.HasPrefix(destRaw, "|") {
			continue
		}
		local := strings.TrimSuffix(strings.ToLower(source), "@"+strings.ToLower(sourceDomain))
		if local == "*" {
			local = ""
		}
		if local != "" && !localPartRE.MatchString(local) {
			continue
		}
		var dests []string
		for _, d := range strings.Split(destRaw, ",") {
			d = strings.ToLower(strings.TrimSpace(d))
			if d == "" {
				continue
			}
			if !strings.Contains(d, "@") && localPartRE.MatchString(d) {
				d += "@" + targetDomain
			}
			d = strings.ReplaceAll(d, "@"+strings.ToLower(sourceDomain), "@"+targetDomain)
			if strings.Contains(d, "@") {
				dests = append(dests, d)
			}
		}
		if len(dests) > 0 {
			out = append(out, aliasImport{Local: local, Destination: strings.Join(dests, ",")})
		}
	}
	return out
}

var errMemberNotFound = errors.New("arşiv üyesi bulunamadı")

// readSmallTarMembers — istenen küçük üyeleri TEK arşiv geçişinde toplar.
//
// 🔴 NEDEN TOPLU: arşiv gzip'tir, yani rastgele erişim yoktur — her arama tüm
// dosyayı baştan açar. Üye başına ayrı çağrı yapmak 20 GiB'lık bir cPanel
// yedeğinde aktarımı saatlere çıkarıyordu (yalnız SSL için 9 aday × tam
// decompress). Bulunamayan üyeler sonuç haritasında hiç yer almaz.
func readSmallTarMembers(archivePath string, wants []string) (map[string][]byte, error) {
	return readSmallTarMembersMatching(archivePath, wants, nil)
}

func readSmallTarMembersMatching(archivePath string, wants, prefixes []string) (map[string][]byte, error) {
	aranan := make(map[string]string, len(wants)) // temizlenmiş ad -> orijinal istek
	for _, w := range wants {
		if w != "" {
			aranan[path.Clean(w)] = w
		}
	}
	found := make(map[string][]byte, len(aranan))
	cleanPrefixes := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		if prefix != "" {
			cleanPrefixes = append(cleanPrefixes, strings.TrimSuffix(path.Clean(prefix), "/")+"/")
		}
	}
	if len(aranan) == 0 && len(cleanPrefixes) == 0 {
		return found, nil
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var prefixBytes int64
	for len(cleanPrefixes) > 0 || len(found) < len(aranan) {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		cleanName := path.Clean(h.Name)
		istek, exact := aranan[cleanName]
		prefixMatch := false
		for _, prefix := range cleanPrefixes {
			if strings.HasPrefix(cleanName, prefix) {
				prefixMatch = true
				istek = cleanName
				break
			}
		}
		if (!exact && !prefixMatch) || h.Typeflag != tar.TypeReg {
			continue
		}
		if h.Size > maxMetadataBytes {
			return nil, ErrArchiveTooLarge
		}
		if prefixMatch {
			prefixBytes += h.Size
			if prefixBytes > 32<<20 || len(found) >= 2048 {
				return nil, ErrArchiveTooLarge
			}
		}
		body, err := io.ReadAll(io.LimitReader(tr, maxMetadataBytes))
		if err != nil {
			return nil, err
		}
		found[istek] = body
	}
	return found, nil
}

func databaseMappings(sources []string, sk, defaultDB, dbUser string) []DBMap {
	out := make([]DBMap, 0, len(sources))
	used := map[string]bool{defaultDB: true}
	for i, source := range sources {
		target := defaultDB
		if i > 0 {
			suffix := dbSuffix(source)
			maxSuffix := 64 - len(sk) - 1
			if maxSuffix < 1 {
				maxSuffix = 1
			}
			if len(suffix) > maxSuffix {
				suffix = suffix[:maxSuffix]
			}
			target = sk + "_" + suffix
			base := target
			for n := 2; used[target]; n++ {
				tail := "_" + strconv.Itoa(n)
				limit := 64 - len(tail)
				if len(base) > limit {
					base = base[:limit]
				}
				target = base + tail
			}
		}
		used[target] = true
		out = append(out, DBMap{Source: source, Target: target, User: dbUser})
	}
	return out
}

func dbSuffix(source string) string {
	s := strings.ToLower(source)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
		} else if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	s = strings.Trim(b.String(), "_")
	if s == "" {
		return "db"
	}
	if len(s) > 32 {
		s = s[:32]
	}
	return strings.TrimRight(s, "_")
}

func requiredInt64(s string) (int64, error) {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || v <= 0 {
		return 0, errors.New("geçersiz sayı")
	}
	return v, nil
}

func copyRecorded(w http.ResponseWriter, rr *httptest.ResponseRecorder) {
	for k, values := range rr.Header() {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(rr.Code)
	_, _ = w.Write(rr.Body.Bytes())
}

func (h *Handlers) rollbackDomain(r *http.Request, id int64) error {
	if h.Domains == nil {
		return errors.New("domain silme handler'ı yok")
	}
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", strconv.FormatInt(id, 10))
	ctx := contextWithRoute(r, rc)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/"+strconv.FormatInt(id, 10), nil).
		WithContext(ctx)
	rr := httptest.NewRecorder()
	h.Domains.Delete(rr, req)
	if rr.Code < http.StatusOK || rr.Code >= http.StatusMultipleChoices {
		return fmt.Errorf("domain silme HTTP %d: %s", rr.Code, strings.TrimSpace(rr.Body.String()))
	}
	return nil
}

func contextWithRoute(r *http.Request, rc *chi.Context) context.Context {
	return context.WithValue(r.Context(), chi.RouteCtxKey, rc)
}

func restoreWeb(archivePath, root, sk string) error {
	if !adlar.SKGecerli(sk) || root == "" {
		return errors.New("güvensiz hedef")
	}
	// Katman 2: sistem tar'ına geçmeden önce arşivi Go stdlib ile ön-tara —
	// mutlak yol / ".." / symlink-hardlink-aygıt üyesi varsa çıkarma reddedilir.
	if err := archivex.Tara(archivePath, archivex.TurTarGz); err != nil {
		return err
	}
	home := "/home/" + sk
	target := home + "/public_html"
	if out, err := exec.Command("runuser", "-u", sk, "--", "find", target, "-mindepth", "1", "-delete").CombinedOutput(); err != nil {
		return fmt.Errorf("hedef temizleme: %s", strings.TrimSpace(string(out)))
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	member := root + "/homedir/public_html"
	// --warning=no-timestamp: kaynak sunucuda ileri tarihli mtime taşıyan dosyalar
	// (ör. PrestaShop var/cache) on binlerce zararsız uyarı üretip gerçek hatayı
	// çıktının içinde boğuyordu. Uyarı zaten çıkış kodunu etkilemiyor.
	cmd := exec.Command("runuser", "-u", sk, "--", "tar", "-xz", "-f", "-", "-C", target,
		"--strip-components=3", "--warning=no-timestamp", member)
	cmd.Stdin = f
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar: %s", tarHataOzeti(out, err))
	}
	_, _ = exec.Command("restorecon", "-RF", target).CombinedOutput()
	return nil
}

// tarHataOzeti — tar'ın ölümcül nedeni ÇIKTININ SONUNDA olur. Çıktıyı baştan
// kırpmak (uyarı seli + 1000 karakterlik kısaltma) tam da o satırı yok ediyordu:
// hata mesajı binlerce "time stamp ... in the future" uyarısından ibaret kalıyor,
// asıl neden ("Cannot write: Disk quota exceeded" gibi) hiç görünmüyordu.
func tarHataOzeti(out []byte, err error) string {
	tutulan := []string{}
	for _, satir := range strings.Split(string(out), "\n") {
		satir = strings.TrimSpace(satir)
		if satir == "" || tarGurultusu(satir) {
			continue
		}
		tutulan = append(tutulan, satir)
	}
	if len(tutulan) == 0 {
		ham := strings.Join(strings.Fields(string(out)), " ")
		if ham == "" {
			return err.Error()
		}
		return sonKarakterler(ham, 400)
	}
	if len(tutulan) > 6 {
		tutulan = tutulan[len(tutulan)-6:]
	}
	return sonKarakterler(strings.Join(tutulan, "; "), 500)
}

// tarGurultusu — çıkış kodunu etkilemeyen, teşhis değeri olmayan tar uyarıları.
func tarGurultusu(satir string) bool {
	for _, iz := range []string{"s in the future", "Removing leading", "socket ignored"} {
		if strings.Contains(satir, iz) {
			return true
		}
	}
	return false
}

func sonKarakterler(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// restoreNativeHome, yalnız SanalCP dışa aktarıcısının web kökü dışında
// paketlediği güvenli üst-seviye dosyaları geri yükler. Alt dizin, symlink ve
// kabuk/SSH kimliği kabul edilmez.
func restoreNativeHome(archivePath, root, sk string) error {
	if !adlar.SKGecerli(sk) || root == "" {
		return errors.New("güvensiz hedef")
	}
	if err := archivex.Tara(archivePath, archivex.TurTarGz); err != nil {
		return err
	}
	prefix := path.Clean(root+"/homedir/native_home") + "/"
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	members := make([]string, 0, 8)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := path.Clean(h.Name)
		if !strings.HasPrefix(name, prefix) || h.Typeflag != tar.TypeReg {
			continue
		}
		base := strings.TrimPrefix(name, prefix)
		if base == "" || strings.Contains(base, "/") || nativeHomeFileDenied(base) {
			return fmt.Errorf("güvensiz kullanıcı ana dizini üyesi: %s", name)
		}
		members = append(members, name)
		if len(members) > 256 {
			return errors.New("çok fazla kullanıcı ana dizini dosyası")
		}
	}
	if len(members) == 0 {
		return nil
	}
	f, err = os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	args := []string{"-u", sk, "--", "tar", "-xz", "-f", "-", "-C", "/home/" + sk,
		"--strip-components=3", "--no-same-owner", "--no-same-permissions"}
	args = append(args, members...)
	cmd := exec.Command("runuser", args...)
	cmd.Stdin = f
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func nativeHomeFileDenied(base string) bool {
	switch base {
	case ".bash_logout", ".bash_profile", ".bashrc", ".bash_history", ".profile",
		".lesshst", ".viminfo", ".mysql_history", ".wget-hsts", "authorized_keys":
		return true
	}
	return false
}

// restoreDatabases — bütün SQL dump'larını TEK arşiv geçişinde içe aktarır.
//
// Dump başına ayrı geçiş, veritabanı sayısı kadar tam gzip decompress demekti.
// Arşiv sıralı okunur; hangi üyeye denk gelinirse ilgili hedefe aktarılır.
func restoreDatabases(archivePath, root string, maps []DBMap) error {
	hedef := make(map[string]string, len(maps)) // arşiv üyesi -> hedef DB
	for _, m := range maps {
		hedef[path.Clean(root+"/mysql/"+m.Source+".sql")] = m.Target
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	kalan := len(hedef)
	for kalan > 0 {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		targetDB, ok := hedef[path.Clean(h.Name)]
		if !ok || h.Typeflag != tar.TypeReg {
			continue
		}
		delete(hedef, path.Clean(h.Name))
		kalan--
		if err := dumpAktar(tr, targetDB); err != nil {
			return err
		}
	}
	if kalan > 0 {
		eksik := make([]string, 0, kalan)
		for _, targetDB := range hedef {
			eksik = append(eksik, targetDB)
		}
		sort.Strings(eksik)
		return fmt.Errorf("SQL dump arşivde bulunamadı: %s", strings.Join(eksik, ", "))
	}
	return nil
}

// dumpAktar — tek bir dump'ı hedef veritabanına akıtır. Kaynak dump'taki
// CREATE DATABASE / USE satırları atılır; aksi halde içe aktarma hedef yerine
// cPanel'deki özgün veritabanı adına yazardı.
func dumpAktar(src io.Reader, targetDB string) error {
	cmd := exec.Command("mysql", targetDB)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	bw := bufio.NewWriter(stdin)
	br := bufio.NewReader(src)
	for {
		line, readErr := br.ReadString('\n')
		upper := strings.ToUpper(strings.TrimSpace(line))
		if !strings.HasPrefix(upper, "CREATE DATABASE ") && !strings.HasPrefix(upper, "USE ") {
			if _, err := bw.WriteString(line); err != nil {
				_ = stdin.Close()
				_ = cmd.Wait()
				return err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = stdin.Close()
			_ = cmd.Wait()
			return readErr
		}
	}
	if err := bw.Flush(); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return err
	}
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("mysql %s: %s", targetDB, strings.TrimSpace(stderr.String()))
	}
	return nil
}
