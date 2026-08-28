// Package sitekopya manages isolated, addressable staging environments.
package sitekopya

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"sanalcp/internal/adlar"
	"sanalcp/internal/backups"
	"sanalcp/internal/hesaplar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/iceaktarim"
	"sanalcp/internal/provisioner"
	"sanalcp/internal/sqlimport"
)

type Handlers struct {
	DB   *sql.DB
	IPv4 string
}
type ortam struct {
	ID        int64  `json:"id"`
	SourceID  int64  `json:"source_domain_id"`
	StagingID int64  `json:"staging_domain_id"`
	AlanAdi   string `json:"alan_adi"`
	Durum     string `json:"durum"`
	Olusturma string `json:"olusturma"`
	SonPush   string `json:"son_push"`
}
type domain struct {
	ID                              int64
	AlanAdi, SK, PHP, DBAdi, DBUser string
	CustomerID, PlanID              sql.NullInt64
}

func (h *Handlers) source(r *http.Request) (domain, error) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var d domain
	err := h.DB.QueryRowContext(r.Context(), `SELECT id,alan_adi,sistem_kullanici,php_surum,COALESCE(db_adi,''),COALESCE(db_user,''),customer_id,plan_id FROM domains WHERE id=?`, id).Scan(&d.ID, &d.AlanAdi, &d.SK, &d.PHP, &d.DBAdi, &d.DBUser, &d.CustomerID, &d.PlanID)
	return d, err
}

func (h *Handlers) Liste(w http.ResponseWriter, r *http.Request) {
	d, e := h.source(r)
	if e != nil {
		httpx.WriteError(w, 404, "domain bulunamadı")
		return
	}
	var o ortam
	e = h.DB.QueryRowContext(r.Context(), `SELECT s.id,s.source_domain_id,s.staging_domain_id,d.alan_adi,s.durum,DATE_FORMAT(s.created_at,'%Y-%m-%d %H:%i'),COALESCE(DATE_FORMAT(s.son_push_at,'%Y-%m-%d %H:%i'),'') FROM staging_environments s JOIN domains d ON d.id=s.staging_domain_id WHERE s.source_domain_id=?`, d.ID).Scan(&o.ID, &o.SourceID, &o.StagingID, &o.AlanAdi, &o.Durum, &o.Olusturma, &o.SonPush)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.WriteJSON(w, 200, []ortam{})
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, "staging okunamadı")
		return
	}
	httpx.WriteJSON(w, 200, []ortam{o})
}

type createReq struct {
	AlanAdi string `json:"alan_adi"`
}

func (h *Handlers) Olustur(w http.ResponseWriter, r *http.Request) {
	src, e := h.source(r)
	if e != nil {
		httpx.WriteError(w, 404, "domain bulunamadı")
		return
	}
	var req createReq
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req) != nil {
		httpx.WriteError(w, 400, "geçersiz istek")
		return
	}
	req.AlanAdi = strings.ToLower(strings.TrimSpace(req.AlanAdi))
	if e = provisioner.ValidateDomain(req.AlanAdi); e != nil {
		httpx.WriteError(w, 400, e.Error())
		return
	}
	var n int
	_ = h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM staging_environments WHERE source_domain_id=?`, src.ID).Scan(&n)
	if n > 0 {
		httpx.WriteError(w, 409, "bu site için staging ortamı zaten var")
		return
	}
	_ = h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM domains WHERE alan_adi=?`, req.AlanAdi).Scan(&n)
	if n > 0 {
		httpx.WriteError(w, 409, "bu alan adı zaten kullanılıyor")
		return
	}
	pr, e := provisioner.Provision(req.AlanAdi, src.PHP)
	if e != nil {
		httpx.WriteError(w, 500, "staging sağlanamadı: "+e.Error())
		return
	}
	did := int64(0)
	cleanup := func() {
		if did > 0 {
			_ = hesaplar.MySQLDropAllForDomain(h.DB, did)
			_, _ = h.DB.Exec(`DELETE FROM domains WHERE id=?`, did)
		}
		_ = provisioner.Deprovision(req.AlanAdi, pr.SistemKullanici)
	}
	dbn, dbu, dbp := pr.SistemKullanici+"_main", pr.SistemKullanici+"_db", hesaplar.RandomParola(24)
	res, e := h.DB.ExecContext(r.Context(), `INSERT INTO domains(alan_adi,sistem_kullanici,php_surum,ssl_aktif,durum,ipv4,ftp_host,ftp_user,db_host,db_user,db_adi,web_root,is_demo,site_tipi,customer_id,plan_id,notlar) VALUES(?,?,?,0,'aktif',?,?,?,'localhost',?,?,?,0,'php',?,?,?)`, req.AlanAdi, pr.SistemKullanici, src.PHP, h.IPv4, h.IPv4, pr.SistemKullanici, dbu, dbn, pr.WebRoot, nullable(src.CustomerID), nullable(src.PlanID), "Staging: "+src.AlanAdi)
	if e != nil {
		cleanup()
		httpx.WriteError(w, 500, "staging kaydı oluşturulamadı")
		return
	}
	did, _ = res.LastInsertId()
	if e = hesaplar.MySQLCreateDB(h.DB, did, dbn, dbu, dbp); e != nil {
		cleanup()
		httpx.WriteError(w, 500, "staging veritabanı oluşturulamadı: "+e.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	if out, x := exec.CommandContext(ctx, "rsync", "-a", "--delete", filepath.Join("/home", src.SK, "public_html")+"/", pr.WebRoot+"/").CombinedOutput(); x != nil {
		cleanup()
		httpx.WriteExecError(w, 500, "dosyalar kopyalanamadı", out)
		return
	}
	_ = exec.Command("chown", "-R", pr.SistemKullanici+":"+pr.SistemKullanici, pr.WebRoot).Run()
	if src.DBAdi != "" {
		if e = copyDB(ctx, src.DBAdi, sqlimport.Hedef{DBAdi: dbn, Kullanici: dbu, Parola: dbp}); e != nil {
			cleanup()
			httpx.WriteError(w, 500, "veritabanı kopyalanamadı: "+e.Error())
			return
		}
	}
	iceaktarim.StagingConfigGuncelle(filepath.Join("/home", pr.SistemKullanici), pr.SistemKullanici, sqlimport.Hedef{DBAdi: dbn, Kullanici: dbu, Parola: dbp})
	adjustAppURLs(ctx, sqlimport.Hedef{DBAdi: dbn, Kullanici: dbu, Parola: dbp}, src.AlanAdi, req.AlanAdi)
	_, _ = h.DB.ExecContext(ctx, `INSERT INTO nginx_settings(domain_id,ek_direktifler) VALUES(?,?) ON DUPLICATE KEY UPDATE ek_direktifler=VALUES(ek_direktifler)`, did, "add_header X-Robots-Tag \"noindex, nofollow, noarchive\" always;")
	_ = provisioner.RerenderVhost(h.DB, did)
	if _, e = h.DB.ExecContext(ctx, `INSERT INTO staging_environments(source_domain_id,staging_domain_id) VALUES(?,?)`, src.ID, did); e != nil {
		cleanup()
		httpx.WriteError(w, 500, "staging bağlantısı kaydedilemedi")
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"ok": true, "alan_adi": req.AlanAdi, "staging_domain_id": did})
}

type pushReq struct {
	Dosyalar   bool `json:"dosyalar"`
	Veritabani bool `json:"veritabani"`
}

func (h *Handlers) CanliyaGonder(w http.ResponseWriter, r *http.Request) {
	src, e := h.source(r)
	if e != nil {
		httpx.WriteError(w, 404, "domain bulunamadı")
		return
	}
	var req pushReq
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req) != nil || (!req.Dosyalar && !req.Veritabani) {
		httpx.WriteError(w, 400, "dosya veya veritabanı seçin")
		return
	}
	var sid int64
	var ssk, sdb string
	e = h.DB.QueryRowContext(r.Context(), `SELECT d.id,d.sistem_kullanici,d.db_adi FROM staging_environments s JOIN domains d ON d.id=s.staging_domain_id WHERE s.source_domain_id=?`, src.ID).Scan(&sid, &ssk, &sdb)
	if e != nil {
		httpx.WriteError(w, 404, "staging ortamı bulunamadı")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Minute)
	defer cancel()
	name, _, e := backups.CreateRecoveryArchive(ctx, h.DB, src.ID, src.AlanAdi, src.SK, "Staging canlıya gönderim öncesi otomatik kurtarma noktası")
	if e != nil {
		httpx.WriteError(w, 500, "kurtarma yedeği alınamadı; işlem durduruldu: "+e.Error())
		return
	}
	if req.Dosyalar {
		if out, x := exec.CommandContext(ctx, "rsync", "-a", "--delete", filepath.Join("/home", ssk, "public_html")+"/", filepath.Join("/home", src.SK, "public_html")+"/").CombinedOutput(); x != nil {
			httpx.WriteExecError(w, 500, "dosyalar gönderilemedi", out)
			return
		}
		_ = exec.Command("chown", "-R", src.SK+":"+src.SK, filepath.Join("/home", src.SK, "public_html")).Run()
	}
	if req.Veritabani && src.DBAdi != "" {
		live, e := h.dbHedef(ctx, src.ID, src.DBAdi, src.DBUser)
		if e != nil {
			httpx.WriteError(w, 500, e.Error())
			return
		}
		if e = sqlimport.TablolariSil(ctx, live); e != nil {
			httpx.WriteError(w, 500, "canlı DB hazırlanamadı: "+e.Error())
			return
		}
		if e = copyDB(ctx, sdb, live); e != nil {
			httpx.WriteError(w, 500, "veritabanı gönderilemedi: "+e.Error())
			return
		}
	}
	if live, e := h.dbHedef(ctx, src.ID, src.DBAdi, src.DBUser); e == nil {
		iceaktarim.StagingConfigGuncelle(filepath.Join("/home", src.SK), src.SK, live)
	}
	_, _ = h.DB.ExecContext(ctx, `UPDATE staging_environments SET son_push_at=NOW(),durum='hazir' WHERE source_domain_id=?`, src.ID)
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "kurtarma_yedegi": name})
}

func (h *Handlers) Sil(w http.ResponseWriter, r *http.Request) {
	src, e := h.source(r)
	if e != nil {
		httpx.WriteError(w, 404, "domain bulunamadı")
		return
	}
	var sid int64
	var ad, sk string
	if e = h.DB.QueryRowContext(r.Context(), `SELECT d.id,d.alan_adi,d.sistem_kullanici FROM staging_environments s JOIN domains d ON d.id=s.staging_domain_id WHERE s.source_domain_id=?`, src.ID).Scan(&sid, &ad, &sk); e != nil {
		httpx.WriteError(w, 404, "staging bulunamadı")
		return
	}
	if !adlar.SKGecerli(sk) {
		httpx.WriteError(w, 400, "geçersiz staging kullanıcısı")
		return
	}
	_ = hesaplar.MySQLDropAllForDomain(h.DB, sid)
	_ = provisioner.Deprovision(ad, sk)
	_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM staging_environments WHERE source_domain_id=?`, src.ID)
	_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM domains WHERE id=?`, sid)
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}

func (h *Handlers) dbHedef(ctx context.Context, id int64, name, user string) (sqlimport.Hedef, error) {
	var enc string
	if name == "" || user == "" {
		return sqlimport.Hedef{}, fmt.Errorf("veritabanı hesabı bulunamadı")
	}
	if e := h.DB.QueryRowContext(ctx, `SELECT db_pass_plain FROM db_accounts WHERE domain_id=? AND db_name=? AND db_user=? LIMIT 1`, id, name, user).Scan(&enc); e != nil {
		return sqlimport.Hedef{}, fmt.Errorf("veritabanı hesabı bulunamadı")
	}
	p, e := hesaplar.DecryptDBPassword(enc)
	return sqlimport.Hedef{DBAdi: name, Kullanici: user, Parola: p}, e
}
func copyDB(ctx context.Context, source string, target sqlimport.Hedef) error {
	if !hesaplar.GecerliDBKimlik(source) {
		return fmt.Errorf("geçersiz kaynak DB")
	}
	cmd := exec.CommandContext(ctx, "mysqldump", "--single-transaction", "--routines", "--triggers", "--events", source)
	pipe, e := cmd.StdoutPipe()
	if e != nil {
		return e
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if e = cmd.Start(); e != nil {
		return e
	}
	imp := sqlimport.Uygula(ctx, target, pipe)
	dumpErr := cmd.Wait()
	if dumpErr != nil {
		return fmt.Errorf("mysqldump: %s: %w", strings.TrimSpace(stderr.String()), dumpErr)
	}
	return imp
}

// adjustAppURLs prevents cloned WordPress/PrestaShop installations from
// redirecting visitors back to production. Failure is deliberately best-effort:
// unknown applications remain cloneable and can be configured manually.
func adjustAppURLs(ctx context.Context, target sqlimport.Hedef, oldHost, newHost string) {
	cmd := exec.CommandContext(ctx, "mysql", "--protocol=socket", "-u", target.Kullanici, target.DBAdi, "-Nse", `SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND (table_name LIKE '%\\_options' OR table_name LIKE '%\\_shop_url')`)
	cmd.Env = append(exec.Command("env").Environ(), "MYSQL_PWD="+target.Parola)
	out, err := cmd.Output()
	if err != nil {
		return
	}
	for _, table := range strings.Fields(string(out)) {
		if !hesaplar.GecerliDBKimlik(table) {
			continue
		}
		var q string
		if strings.HasSuffix(table, "_options") {
			q = fmt.Sprintf("UPDATE `%s` SET option_value='http://%s' WHERE option_name IN ('home','siteurl');", table, sqlString(newHost))
		} else if strings.HasSuffix(table, "_shop_url") {
			q = fmt.Sprintf("UPDATE `%s` SET domain='%s',domain_ssl='%s' WHERE domain IN ('%s','www.%s') OR domain_ssl IN ('%s','www.%s');", table, sqlString(newHost), sqlString(newHost), sqlString(oldHost), sqlString(oldHost), sqlString(oldHost), sqlString(oldHost))
		}
		if q != "" {
			_ = sqlimport.Uygula(ctx, target, strings.NewReader(q))
		}
	}
}

func sqlString(v string) string {
	return strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), `'`, `\'`)
}
func nullable(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}
