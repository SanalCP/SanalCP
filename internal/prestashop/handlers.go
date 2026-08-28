package prestashop

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-sql-driver/mysql"
	"golang.org/x/sys/unix"
	"sanalcp/internal/backups"
	"sanalcp/internal/hesaplar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/jailpath"
	"sanalcp/internal/osfam"
)

type Handlers struct{ DB *sql.DB }
type psDomain struct {
	ID                     int64
	AlanAdi, SK, Home, PHP string
	SSL                    bool
}
type psInstall struct{ Rel, Root, DBName, DBUser, DBPass, Prefix string }

var relRE = regexp.MustCompile(`^public_html(?:/[A-Za-z0-9._-]+)*$`)

func (h *Handlers) domain(r *http.Request) (psDomain, error) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var d psDomain
	var cert string
	var demo int
	err := h.DB.QueryRowContext(r.Context(), `SELECT id,alan_adi,sistem_kullanici,php_surum,COALESCE(cert_path,''),COALESCE(is_demo,0) FROM domains WHERE id=?`, id).Scan(&d.ID, &d.AlanAdi, &d.SK, &d.PHP, &cert, &demo)
	if err != nil {
		return d, os.ErrNotExist
	}
	if demo == 1 {
		return d, errors.New("demo aboneliğinde kullanılamaz")
	}
	d.SSL = cert != ""
	d.Home, err = jailpath.TenantHome(d.SK)
	return d, err
}
func temizRel(v string) (string, error) {
	v = strings.Trim(strings.TrimSpace(strings.ReplaceAll(v, "\\", "/")), "/")
	if v == "" {
		v = "public_html"
	}
	v = path.Clean(v)
	if !relRE.MatchString(v) {
		return "", errors.New("geçersiz PrestaShop dizini")
	}
	return v, nil
}

var phpArrayRE = regexp.MustCompile(`(?m)['"](database_name|database_user|database_password|database_prefix)['"]\s*=>\s*['"]([^'"\r\n]*)['"]`)
var legacyRE = regexp.MustCompile(`(?m)define\s*\(\s*['"](_DB_NAME_|_DB_USER_|_DB_PASSWD_|_DB_PREFIX_)['"]\s*,\s*['"]([^'"\r\n]*)['"]`)

func (h *Handlers) install(r *http.Request) (psDomain, psInstall, error) {
	d, e := h.domain(r)
	if e != nil {
		return d, psInstall{}, e
	}
	rel, e := temizRel(r.URL.Query().Get("dizin"))
	if e != nil {
		return d, psInstall{}, e
	}
	if e = jailpath.DizinDogrula(d.Home, rel); e != nil {
		return d, psInstall{}, errors.New("PrestaShop dizini bulunamadı veya güvenli değil")
	}
	p := psInstall{Rel: rel, Root: filepath.Join(d.Home, filepath.FromSlash(rel)), Prefix: "ps_"}
	var raw []byte
	for _, f := range []string{path.Join(rel, "app/config/parameters.php"), path.Join(rel, "config/settings.inc.php")} {
		fd, x := jailpath.Ac(d.Home, f, unix.O_RDONLY, 0)
		if x != nil {
			continue
		}
		raw, x = io.ReadAll(io.LimitReader(fd, 1<<20))
		fd.Close()
		if x == nil {
			break
		}
	}
	if len(raw) == 0 {
		return d, p, errors.New("PrestaShop yapılandırması bulunamadı")
	}
	vals := parseDBConfig(raw)
	p.DBName, p.DBUser, p.Prefix = vals["database_name"], vals["database_user"], vals["database_prefix"]
	if p.Prefix == "" {
		p.Prefix = "ps_"
	}
	if !hesaplar.GecerliDBKimlik(p.DBName) || !hesaplar.GecerliDBKimlik(p.DBUser) || !hesaplar.GecerliDBKimlik(p.Prefix+"configuration") {
		return d, p, errors.New("PrestaShop DB kimlikleri geçersiz")
	}
	var enc string
	e = h.DB.QueryRowContext(r.Context(), `SELECT db_pass_plain FROM db_accounts WHERE domain_id=? AND db_name=? AND db_user=? LIMIT 1`, d.ID, p.DBName, p.DBUser).Scan(&enc)
	if e != nil {
		return d, p, errors.New("PrestaShop veritabanı bu domaine kayıtlı değil")
	}
	p.DBPass, e = hesaplar.DecryptDBPassword(enc)
	return d, p, e
}

func parseDBConfig(raw []byte) map[string]string {
	vals := map[string]string{}
	for _, m := range phpArrayRE.FindAllSubmatch(raw, -1) {
		vals[string(m[1])] = string(m[2])
	}
	for _, m := range legacyRE.FindAllSubmatch(raw, -1) {
		k := map[string]string{"_DB_NAME_": "database_name", "_DB_USER_": "database_user", "_DB_PASSWD_": "database_password", "_DB_PREFIX_": "database_prefix"}[string(m[1])]
		vals[k] = string(m[2])
	}
	return vals
}
func openPS(ctx context.Context, p psInstall) (*sql.DB, error) {
	cfg := mysql.NewConfig()
	cfg.User = p.DBUser
	cfg.Passwd = p.DBPass
	cfg.Net = "unix"
	cfg.Addr = osfam.MariaDBSoket()
	cfg.DBName = p.DBName
	db, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		return nil, e
	}
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, e
	}
	return db, nil
}

type Durum struct {
	Kurulu      bool      `json:"kurulu"`
	Dizin       string    `json:"dizin"`
	Surum       string    `json:"surum"`
	PHP         string    `json:"php"`
	SSL         bool      `json:"ssl"`
	Bakim       bool      `json:"bakim"`
	DB          string    `json:"db"`
	Prefix      string    `json:"prefix"`
	ModulToplam int       `json:"modul_toplam"`
	ModulAktif  int       `json:"modul_aktif"`
	CacheMB     int64     `json:"cache_mb"`
	LogSayisi   int       `json:"log_sayisi"`
	Kontroller  []Kontrol `json:"kontroller"`
}
type Kontrol struct {
	Ad    string `json:"ad"`
	OK    bool   `json:"ok"`
	Detay string `json:"detay"`
}

func (h *Handlers) Durum(w http.ResponseWriter, r *http.Request) {
	d, p, e := h.install(r)
	if e != nil {
		httpx.WriteError(w, 400, e.Error())
		return
	}
	out := Durum{Kurulu: true, Dizin: p.Rel, Surum: psSurumDosyadanOku(p.Root), PHP: d.PHP, SSL: d.SSL, DB: p.DBName, Prefix: p.Prefix, CacheMB: dirSize(filepath.Join(p.Root, "var", "cache")) / (1 << 20), LogSayisi: logCount(p.Root)}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	db, e := openPS(ctx, p)
	if e == nil {
		defer db.Close()
		var enabled int
		e = db.QueryRowContext(ctx, "SELECT CAST(value AS UNSIGNED) FROM `"+p.Prefix+"configuration` WHERE name='PS_SHOP_ENABLE' ORDER BY id_configuration DESC LIMIT 1").Scan(&enabled)
		out.Bakim = enabled == 0
		_ = db.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(active),0) FROM `"+p.Prefix+"module`").Scan(&out.ModulToplam, &out.ModulAktif)
	}
	out.Kontroller = []Kontrol{{"config", true, "Yapılandırma okunabiliyor"}, {"database", e == nil, func() string {
		if e != nil {
			return e.Error()
		}
		return "Bağlantı başarılı"
	}()}, {"install_dir", !dirExists(filepath.Join(p.Root, "install")), "install/ dizini kaldırılmış olmalı"}, {"ssl", d.SSL, "HTTPS sertifikası"}, {"logs", true, fmt.Sprintf("%d log dosyası", out.LogSayisi)}}
	httpx.WriteJSON(w, 200, out)
}

type bakimReq struct {
	Dizin string `json:"dizin"`
	Aktif bool   `json:"aktif"`
}

func (h *Handlers) Bakim(w http.ResponseWriter, r *http.Request) {
	var req bakimReq
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req) != nil {
		httpx.WriteError(w, 400, "geçersiz istek")
		return
	}
	q := r.URL.Query()
	q.Set("dizin", req.Dizin)
	r.URL.RawQuery = q.Encode()
	d, p, e := h.install(r)
	if e != nil {
		httpx.WriteError(w, 400, e.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	name, _, e := backups.CreateRecoveryArchive(ctx, h.DB, d.ID, d.AlanAdi, d.SK, "PrestaShop bakım modu değişikliği öncesi kurtarma noktası")
	if e != nil {
		httpx.WriteError(w, 500, "kurtarma noktası alınamadı: "+e.Error())
		return
	}
	db, e := openPS(ctx, p)
	if e != nil {
		httpx.WriteError(w, 500, e.Error())
		return
	}
	defer db.Close()
	value := 1
	if req.Aktif {
		value = 0
	}
	res, e := db.ExecContext(ctx, "UPDATE `"+p.Prefix+"configuration` SET value=? WHERE name='PS_SHOP_ENABLE'", value)
	if e != nil {
		httpx.WriteError(w, 500, e.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.WriteError(w, 500, "PS_SHOP_ENABLE ayarı bulunamadı")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "bakim": req.Aktif, "kurtarma_yedegi": name})
}

type dizinReq struct {
	Dizin string `json:"dizin"`
}

func (h *Handlers) CacheTemizle(w http.ResponseWriter, r *http.Request) {
	var req dizinReq
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req) != nil {
		httpx.WriteError(w, 400, "geçersiz istek")
		return
	}
	q := r.URL.Query()
	q.Set("dizin", req.Dizin)
	r.URL.RawQuery = q.Encode()
	d, p, e := h.install(r)
	if e != nil {
		httpx.WriteError(w, 400, e.Error())
		return
	}
	cleared := []string{}
	for _, rel := range []string{"var/cache/prod", "var/cache/dev", "cache/smarty/cache", "cache/smarty/compile"} {
		full := path.Join(p.Rel, rel)
		if jailpath.DizinDogrula(d.Home, full) == nil {
			if e = jailpath.IceriginiSil(d.Home, full); e != nil {
				httpx.WriteError(w, 500, "cache temizlenemedi: "+e.Error())
				return
			}
			cleared = append(cleared, rel)
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "temizlenen": cleared})
}

func (h *Handlers) KurtarmaNoktasi(w http.ResponseWriter, r *http.Request) {
	var req dizinReq
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req)
	q := r.URL.Query()
	q.Set("dizin", req.Dizin)
	r.URL.RawQuery = q.Encode()
	d, _, e := h.install(r)
	if e != nil {
		httpx.WriteError(w, 400, e.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	name, size, e := backups.CreateRecoveryArchive(ctx, h.DB, d.ID, d.AlanAdi, d.SK, "Manuel PrestaShop kurtarma noktası")
	if e != nil {
		httpx.WriteError(w, 500, e.Error())
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"ok": true, "dosya": name, "boyut_b": size})
}

type LogResp struct {
	Dosya    string   `json:"dosya"`
	Satirlar []string `json:"satirlar"`
	Kesildi  bool     `json:"kesildi"`
}

func (h *Handlers) Loglar(w http.ResponseWriter, r *http.Request) {
	d, p, e := h.install(r)
	if e != nil {
		httpx.WriteError(w, 400, e.Error())
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 500 {
		limit = 200
	}
	files := logFiles(p.Root)
	if len(files) == 0 {
		httpx.WriteJSON(w, 200, LogResp{Satirlar: []string{}})
		return
	}
	latest := files[0]
	rel, er := filepath.Rel(d.Home, latest)
	if er != nil {
		httpx.WriteError(w, 500, "log yolu geçersiz")
		return
	}
	f, er := jailpath.Ac(d.Home, filepath.ToSlash(rel), unix.O_RDONLY, 0)
	if er != nil {
		httpx.WriteError(w, 404, "log okunamadı")
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	start := int64(0)
	cut := false
	if st != nil && st.Size() > 512<<10 {
		start = st.Size() - (512 << 10)
		_, _ = f.Seek(start, io.SeekStart)
		cut = true
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	lines := []string{}
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > limit {
			lines = lines[1:]
		}
	}
	httpx.WriteJSON(w, 200, LogResp{Dosya: filepath.Base(latest), Satirlar: lines, Kesildi: cut})
}

func dirExists(p string) bool { st, e := os.Stat(p); return e == nil && st.IsDir() }
func dirSize(root string) int64 {
	var n int64
	_ = filepath.Walk(root, func(_ string, i os.FileInfo, e error) error {
		if e == nil && i != nil && !i.IsDir() {
			n += i.Size()
		}
		return nil
	})
	return n
}
func logFiles(root string) []string {
	var out []string
	for _, pat := range []string{filepath.Join(root, "var/logs/*.log"), filepath.Join(root, "log/*.log")} {
		m, _ := filepath.Glob(pat)
		out = append(out, m...)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := os.Stat(out[i])
		b, _ := os.Stat(out[j])
		return a != nil && b != nil && a.ModTime().After(b.ModTime())
	})
	return out
}
func logCount(root string) int { return len(logFiles(root)) }
