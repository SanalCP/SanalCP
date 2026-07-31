// Package accounts: customers CRUD.
package accounts

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"sanalcp/internal/httpx"
	"sanalcp/internal/kota"
	"sanalcp/internal/middleware"

	"github.com/go-chi/chi/v5"
)

type Customer struct {
	ID      int64  `json:"id"`
	Ad      string `json:"ad"`
	Eposta  string `json:"eposta"`
	PlanID  *int64 `json:"plan_id"`
	Durum   string `json:"durum"`
	Notlar  string `json:"notlar"`
	Created string `json:"olusturma"`
}

type Handlers struct {
	DB *sql.DB
}

// ------------ Customers ------------

func (h *Handlers) ListCustomers(w http.ResponseWriter, r *http.Request) {
	// Bayi yalnız kendi müşterilerini görür (customers.owner_user_id).
	q := `SELECT id, ad, eposta, plan_id, durum, notlar, DATE_FORMAT(created_at,'%Y-%m-%d')
	      FROM customers`
	var arg []any
	if c := middleware.ClaimsFrom(r); c != nil && c.Role == middleware.RolBayi {
		q += ` WHERE owner_user_id = ?`
		arg = append(arg, c.UserID)
	}
	q += ` ORDER BY id`

	rows, err := h.DB.QueryContext(r.Context(), q, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := make([]Customer, 0)
	for rows.Next() {
		var cs Customer
		if err := rows.Scan(&cs.ID, &cs.Ad, &cs.Eposta, &cs.PlanID, &cs.Durum, &cs.Notlar, &cs.Created); err == nil {
			out = append(out, cs)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	var cs Customer
	if err := json.NewDecoder(r.Body).Decode(&cs); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if cs.Ad == "" || cs.Eposta == "" {
		httpx.WriteError(w, http.StatusBadRequest, "ad ve eposta zorunlu")
		return
	}
	if cs.Durum == "" {
		cs.Durum = "aktif"
	}

	// Sahiplik: bayinin açtığı müşteri kendisine bağlanır; admin'in açtığı
	// sahipsizdir (doğrudan admin'e ait).
	var sahip any
	if c := middleware.ClaimsFrom(r); c != nil && c.Role == middleware.RolBayi {
		if err := kota.CheckBayiMusteriEklenebilir(r.Context(), h.DB, c.UserID); err != nil {
			httpx.WriteError(w, http.StatusForbidden, err.Error())
			return
		}
		sahip = c.UserID
	}

	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO customers(ad, eposta, plan_id, durum, notlar, owner_user_id) VALUES(?,?,?,?,?,?)`,
		cs.Ad, cs.Eposta, cs.PlanID, cs.Durum, cs.Notlar, sahip)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB: "+err.Error())
		return
	}
	cs.ID, _ = res.LastInsertId()
	httpx.WriteJSON(w, http.StatusCreated, cs)
}

func (h *Handlers) UpdateCustomer(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if !h.musteriyeYetkiliMi(r, id) {
		httpx.WriteError(w, http.StatusForbidden, "bu müşteriye erişim yok")
		return
	}
	var cs Customer
	if err := json.NewDecoder(r.Body).Decode(&cs); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE customers SET ad=?, eposta=?, plan_id=?, durum=?, notlar=? WHERE id=?`,
		cs.Ad, cs.Eposta, cs.PlanID, cs.Durum, cs.Notlar, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// musteriyeYetkiliMi: admin her müşteriye, bayi yalnız kendi müşterisine.
// Bulunamayan kayıt için de false (bayi id deneyerek varlık çıkaramasın).
func (h *Handlers) musteriyeYetkiliMi(r *http.Request, customerID int64) bool {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		return false
	}
	if c.Role == middleware.RolAdmin {
		return true
	}
	if c.Role != middleware.RolBayi {
		return false
	}
	return middleware.BayiMusterisiMi(r, c.UserID, customerID)
}

func (h *Handlers) DeleteCustomer(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if !h.musteriyeYetkiliMi(r, id) {
		httpx.WriteError(w, http.StatusForbidden, "bu müşteriye erişim yok")
		return
	}
	var n int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM domains WHERE customer_id=?`, id).Scan(&n); err == nil && n > 0 {
		httpx.WriteError(w, http.StatusConflict, "önce bu müşterinin domainlerini kaldırın")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM customers WHERE id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
