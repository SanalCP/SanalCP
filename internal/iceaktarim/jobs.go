package iceaktarim

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"sanalcp/internal/backups"
	"sanalcp/internal/httpx"
)

var jobCreateMu sync.Mutex

type ImportJob struct {
	ID           int64  `json:"id"`
	DomainID     int64  `json:"domain_id"`
	Tur          string `json:"tur"`
	Durum        string `json:"durum"`
	Ilerleme     int    `json:"ilerleme"`
	Adim         string `json:"adim"`
	Hedef        string `json:"hedef"`
	Mesaj        string `json:"mesaj"`
	RecoveryFile string `json:"recovery_file"`
	CreatedAt    string `json:"created_at"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
}

func (h *Handlers) newJob(ctx context.Context, domainID int64, tur, hedef, adim string) (int64, error) {
	jobCreateMu.Lock()
	defer jobCreateMu.Unlock()
	var active int
	if err := h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_jobs WHERE domain_id=? AND durum IN ('queued','running')`, domainID).Scan(&active); err != nil {
		return 0, err
	}
	if active > 0 {
		return 0, fmt.Errorf("bu domain için devam eden bir aktarım işi var")
	}
	res, err := h.DB.ExecContext(ctx, `INSERT INTO import_jobs(domain_id,tur,hedef,adim) VALUES(?,?,?,?)`, domainID, tur, hedef, adim)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
func (h *Handlers) jobUpdate(id int64, durum string, ilerleme int, adim, mesaj, recovery string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = h.DB.ExecContext(ctx, `UPDATE import_jobs SET durum=?,ilerleme=?,adim=?,mesaj=?,recovery_file=IF(?='',recovery_file,?),started_at=IF(started_at IS NULL AND ?='running',NOW(),started_at),finished_at=IF(? IN ('success','failed','rolled_back'),NOW(),finished_at) WHERE id=?`, durum, ilerleme, adim, mesaj, recovery, recovery, durum, durum, id)
}
func scanJob(row *sql.Row) (ImportJob, error) {
	var j ImportJob
	err := row.Scan(&j.ID, &j.DomainID, &j.Tur, &j.Durum, &j.Ilerleme, &j.Adim, &j.Hedef, &j.Mesaj, &j.RecoveryFile, &j.CreatedAt, &j.StartedAt, &j.FinishedAt)
	return j, err
}

const jobSelect = `SELECT id,domain_id,tur,durum,ilerleme,adim,hedef,COALESCE(mesaj,''),recovery_file,DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'),COALESCE(DATE_FORMAT(started_at,'%Y-%m-%d %H:%i:%s'),''),COALESCE(DATE_FORMAT(finished_at,'%Y-%m-%d %H:%i:%s'),'') FROM import_jobs`

func (h *Handlers) Isler(w http.ResponseWriter, r *http.Request) {
	d, e := h.sourceDomain(r)
	if e != nil {
		httpx.WriteError(w, durumKodu(e), e.Error())
		return
	}
	rows, e := h.DB.QueryContext(r.Context(), jobSelect+` WHERE domain_id=? ORDER BY id DESC LIMIT 20`, d.ID)
	if e != nil {
		httpx.WriteError(w, 500, "işler okunamadı")
		return
	}
	defer rows.Close()
	out := []ImportJob{}
	for rows.Next() {
		var j ImportJob
		if rows.Scan(&j.ID, &j.DomainID, &j.Tur, &j.Durum, &j.Ilerleme, &j.Adim, &j.Hedef, &j.Mesaj, &j.RecoveryFile, &j.CreatedAt, &j.StartedAt, &j.FinishedAt) == nil {
			out = append(out, j)
		}
	}
	httpx.WriteJSON(w, 200, out)
}

// RecoverInterruptedJobs rolls back imports interrupted by a panel restart.
func (h *Handlers) RecoverInterruptedJobs() {
	rows, err := h.DB.Query(`SELECT j.id,j.domain_id,j.tur,j.hedef,j.recovery_file,d.sistem_kullanici FROM import_jobs j JOIN domains d ON d.id=j.domain_id WHERE j.durum IN ('queued','running')`)
	if err != nil {
		return
	}
	defer rows.Close()
	type item struct {
		id, domainID              int64
		tur, target, recovery, sk string
	}
	var all []item
	for rows.Next() {
		var x item
		if rows.Scan(&x.id, &x.domainID, &x.tur, &x.target, &x.recovery, &x.sk) == nil {
			all = append(all, x)
		}
	}
	for _, x := range all {
		if x.recovery == "" {
			h.jobUpdate(x.id, "failed", 100, "Panel yeniden başlatıldı", "Kurtarma noktası oluşmadan kesildi; hedef değiştirilmedi.", "")
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		scope, dbName := rollbackScope(x.target), ""
		if x.tur == "database" {
			scope = "database"
			dbName = x.target
		}
		err := backups.RestoreRecoveryArchive(ctx, h.DB, x.domainID, x.sk, filepath.Join(backups.BackupRoot, x.sk, x.recovery), scope, dbName)
		cancel()
		if err != nil {
			h.jobUpdate(x.id, "failed", 100, "Yeniden başlatma sonrası geri alma başarısız", err.Error(), x.recovery)
		} else {
			h.jobUpdate(x.id, "rolled_back", 100, "Panel yeniden başlatması sonrası otomatik geri alındı", "", x.recovery)
		}
	}
}
func (h *Handlers) Is(w http.ResponseWriter, r *http.Request) {
	d, e := h.sourceDomain(r)
	if e != nil {
		httpx.WriteError(w, durumKodu(e), e.Error())
		return
	}
	jid, _ := strconv.ParseInt(chi.URLParam(r, "jid"), 10, 64)
	j, e := scanJob(h.DB.QueryRowContext(r.Context(), jobSelect+` WHERE id=? AND domain_id=?`, jid, d.ID))
	if e == sql.ErrNoRows {
		httpx.WriteError(w, 404, "iş bulunamadı")
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, "iş okunamadı")
		return
	}
	httpx.WriteJSON(w, 200, j)
}

type healthItem struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func (h *Handlers) Saglik(w http.ResponseWriter, r *http.Request) {
	d, e := h.sourceDomain(r)
	if e != nil {
		httpx.WriteError(w, durumKodu(e), e.Error())
		return
	}
	out := []healthItem{}
	entries, e := os.ReadDir(filepath.Join(d.Home, "public_html"))
	out = append(out, healthItem{Name: "files", OK: e == nil && len(entries) > 0, Detail: func() string {
		if e != nil {
			return e.Error()
		}
		if len(entries) == 0 {
			return "public_html boş"
		}
		return strconv.Itoa(len(entries)) + " kök girdi"
	}()})
	var dbCount int
	e = h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM db_accounts WHERE domain_id=?`, d.ID).Scan(&dbCount)
	out = append(out, healthItem{Name: "database", OK: e == nil, Detail: strconv.Itoa(dbCount) + " veritabanı hesabı"})
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://127.0.0.1/", nil)
	req.Host = d.AlanAdi
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	detail := ""
	ok := false
	if e != nil {
		detail = e.Error()
	} else {
		detail = resp.Status
		ok = resp.StatusCode < 500
		resp.Body.Close()
	}
	out = append(out, healthItem{Name: "http", OK: ok, Detail: detail})
	httpx.WriteJSON(w, 200, map[string]any{"ok": out[0].OK && out[1].OK && out[2].OK, "checks": out})
}

type sourceInfo struct {
	ID                int64
	AlanAdi, Home, SK string
}

func (h *Handlers) sourceDomain(r *http.Request) (sourceInfo, error) {
	id, home, sk, e := h.domain(r)
	if e != nil {
		return sourceInfo{}, e
	}
	var ad string
	e = h.DB.QueryRowContext(r.Context(), `SELECT alan_adi FROM domains WHERE id=?`, id).Scan(&ad)
	return sourceInfo{ID: id, AlanAdi: ad, Home: home, SK: sk}, e
}
