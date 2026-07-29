// Package bayipaketleri: adminin bayilere (reseller) atayacağı hazır limit
// paketlerinin (reseller_plans) CRUD'u. Bayinin KENDİSİ bu kataloğu göremez —
// paket yalnız admin'in KullanicilarPage'deki "Bayi Limitleri" ekranında hızlı
// doldurma aracı olarak kullanılır (bkz. internal/users LimitKaydet).
package bayipaketleri

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"sanalpanel/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	DB *sql.DB
}

type Paket struct {
	ID           int64  `json:"id"`
	Ad           string `json:"ad"`
	Aciklama     string `json:"aciklama"`
	MaxMusteri   int    `json:"max_customer"`
	MaxDomain    int    `json:"max_domain"`
	DiskKotaMB   int64  `json:"disk_kota_mb"`
	TrafikKotaMB int64  `json:"trafik_kota_mb"`
	FiyatKurus   int64  `json:"fiyat_kurus"`
	// FazlaSatis: WHM "Overselling Allowed" karşılığı (bkz. migrations/0057).
	// true (varsayılan) = bayinin müşterilerine atadığı hizmet planı kotalarının
	// toplamı kendi disk/trafik limitini aşabilir, yalnız GERÇEK kullanım
	// sınırlanır. false = taahhüt toplamı da limitin üstüne çıkamaz.
	FazlaSatis  bool   `json:"fazla_satis"`
	Varsayilan  bool   `json:"varsayilan"`
	BayiSayisi  int    `json:"bayi_sayisi"`
	Olusturulma string `json:"olusturulma"`
}

const selectAll = `SELECT p.id, p.ad, p.aciklama, p.max_customer, p.max_domain, p.disk_kota_mb, p.trafik_kota_mb,
  p.fiyat_kurus, p.fazla_satis, p.varsayilan,
  (SELECT COUNT(*) FROM reseller_limits rl WHERE rl.reseller_plan_id = p.id),
  COALESCE(DATE_FORMAT(p.created_at,'%Y-%m-%d'),'')
  FROM reseller_plans p`

func scan(rs interface{ Scan(...any) error }) (Paket, error) {
	var p Paket
	var fs, vars int
	err := rs.Scan(&p.ID, &p.Ad, &p.Aciklama, &p.MaxMusteri, &p.MaxDomain, &p.DiskKotaMB, &p.TrafikKotaMB,
		&p.FiyatKurus, &fs, &vars, &p.BayiSayisi, &p.Olusturulma)
	p.FazlaSatis = fs == 1
	p.Varsayilan = vars == 1
	return p, err
}

// GET /bayi-paketleri
func (h *Handlers) Liste(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), selectAll+` ORDER BY p.varsayilan DESC, p.max_domain, p.id`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "listelenemedi: "+err.Error())
		return
	}
	defer rows.Close()
	out := []Paket{}
	for rows.Next() {
		if p, err := scan(rows); err == nil {
			out = append(out, p)
		}
	}
	_ = rows.Err()
	httpx.WriteJSON(w, http.StatusOK, out)
}

// GET /bayi-paketleri/{id}
func (h *Handlers) Getir(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	row := h.DB.QueryRowContext(r.Context(), selectAll+` WHERE p.id=?`, id)
	p, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "paket bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

type istek struct {
	Ad           string `json:"ad"`
	Aciklama     string `json:"aciklama"`
	MaxMusteri   int    `json:"max_customer"`
	MaxDomain    int    `json:"max_domain"`
	DiskKotaMB   int64  `json:"disk_kota_mb"`
	TrafikKotaMB int64  `json:"trafik_kota_mb"`
	FiyatKurus   int64  `json:"fiyat_kurus"`
	FazlaSatis   *bool  `json:"fazla_satis"` // nil = varsayılan (true, mevcut davranış)
	Varsayilan   bool   `json:"varsayilan"`
}

func (q *istek) dogrula() string {
	q.Ad = strings.TrimSpace(q.Ad)
	if q.Ad == "" || len([]rune(q.Ad)) > 100 {
		return "paket adı zorunlu (en çok 100 karakter)"
	}
	if q.MaxMusteri < 0 || q.MaxDomain < 0 || q.DiskKotaMB < 0 || q.TrafikKotaMB < 0 || q.FiyatKurus < 0 {
		return "limitler/fiyat negatif olamaz"
	}
	return ""
}

func (q *istek) fazlaSatis() int {
	if q.FazlaSatis == nil || *q.FazlaSatis {
		return 1
	}
	return 0
}

// POST /bayi-paketleri
func (h *Handlers) Olustur(w http.ResponseWriter, r *http.Request) {
	var q istek
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if msg := q.dogrula(); msg != "" {
		httpx.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	v := 0
	if q.Varsayilan {
		v = 1
		_, _ = h.DB.ExecContext(r.Context(), `UPDATE reseller_plans SET varsayilan=0`)
	}
	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO reseller_plans(ad, aciklama, max_customer, max_domain, disk_kota_mb, trafik_kota_mb, fiyat_kurus, fazla_satis, varsayilan)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		q.Ad, strings.TrimSpace(q.Aciklama), q.MaxMusteri, q.MaxDomain, q.DiskKotaMB, q.TrafikKotaMB, q.FiyatKurus, q.fazlaSatis(), v)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			httpx.WriteError(w, http.StatusConflict, "bu isimde paket zaten var")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "oluşturulamadı: "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// PUT /bayi-paketleri/{id} — mevcut bayilerin limitleri DEĞİŞMEZ (anlık-görüntü
// modeli); istenirse admin bayiyi tekrar bu pakete "uygulayarak" günceller.
func (h *Handlers) Guncelle(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var q istek
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if msg := q.dogrula(); msg != "" {
		httpx.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	v := 0
	if q.Varsayilan {
		v = 1
		_, _ = h.DB.ExecContext(r.Context(), `UPDATE reseller_plans SET varsayilan=0 WHERE id<>?`, id)
	}
	res, err := h.DB.ExecContext(r.Context(),
		`UPDATE reseller_plans SET ad=?, aciklama=?, max_customer=?, max_domain=?, disk_kota_mb=?, trafik_kota_mb=?,
		   fiyat_kurus=?, fazla_satis=?, varsayilan=? WHERE id=?`,
		q.Ad, strings.TrimSpace(q.Aciklama), q.MaxMusteri, q.MaxDomain, q.DiskKotaMB, q.TrafikKotaMB, q.FiyatKurus, q.fazlaSatis(), v, id)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			httpx.WriteError(w, http.StatusConflict, "bu isimde paket zaten var")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "güncellenemedi: "+err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.WriteError(w, http.StatusNotFound, "paket bulunamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// DELETE /bayi-paketleri/{id} — bağlı bayi varsa reddet (paket referansı olmadan
// yetim kalmasınlar; admin önce bayileri başka pakete taşımalı).
func (h *Handlers) Sil(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var n int
	_ = h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM reseller_limits WHERE reseller_plan_id=?`, id).Scan(&n)
	if n > 0 {
		httpx.WriteError(w, http.StatusConflict,
			"bu pakete bağlı "+strconv.Itoa(n)+" bayi var; önce onları başka pakete taşıyın")
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM reseller_plans WHERE id=?`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silinemedi: "+err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.WriteError(w, http.StatusNotFound, "paket bulunamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Limitleri: paket id -> (max_customer, max_domain, disk_kota_mb, trafik_kota_mb, fazla_satis).
// internal/users.LimitKaydet paket seçildiğinde limitleri buradan anlık-görüntü kopyalar.
func Limitleri(r *http.Request, db *sql.DB, paketID int64) (maxMusteri, maxDomain int, diskMB, trafikMB int64, fazlaSatis, ok bool) {
	if paketID <= 0 {
		return 0, 0, 0, 0, false, false
	}
	var fs int
	if err := db.QueryRowContext(r.Context(),
		`SELECT max_customer, max_domain, disk_kota_mb, trafik_kota_mb, fazla_satis FROM reseller_plans WHERE id=?`, paketID).
		Scan(&maxMusteri, &maxDomain, &diskMB, &trafikMB, &fs); err != nil {
		return 0, 0, 0, 0, false, false
	}
	return maxMusteri, maxDomain, diskMB, trafikMB, fs == 1, true
}
