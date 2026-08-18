package users

// Kişisel API token'larının yönetimi.
//
// Her kullanıcı KENDİ token'larını yönetir; başkasının token'ını göremez ve
// iptal edemez. Token, sahibinin rolüyle çalıştığı için "başkası adına token
// üretme" yetki yükseltmesi anlamına gelirdi — bu yüzden hiçbir rol, başkası
// için token üretemez (yönetici dahil).

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sanalcp/internal/auth"
	"sanalcp/internal/httpx"
	"sanalcp/internal/middleware"

	"github.com/go-chi/chi/v5"
)

type apiTokenSatir struct {
	ID            int64  `json:"id"`
	Ad            string `json:"ad"`
	Onek          string `json:"onek"`
	Aktif         bool   `json:"aktif"`
	BitisAt       string `json:"bitis_at"`
	SonKullanimAt string `json:"son_kullanim_at"`
	SonKullanimIP string `json:"son_kullanim_ip"`
	CreatedAt     string `json:"created_at"`
}

// APITokenListe — GET /api/v1/me/api-tokenlari
func (h *Handlers) APITokenListe(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "oturum yok")
		return
	}
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT id, ad, onek, aktif,
		       COALESCE(DATE_FORMAT(bitis_at,'%Y-%m-%d %H:%i'),''),
		       COALESCE(DATE_FORMAT(son_kullanim_at,'%Y-%m-%d %H:%i'),''),
		       son_kullanim_ip,
		       COALESCE(DATE_FORMAT(created_at,'%Y-%m-%d %H:%i'),'')
		  FROM api_tokenlari WHERE user_id=? ORDER BY id DESC`, c.UserID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "listelenemedi")
		return
	}
	defer rows.Close()
	out := []apiTokenSatir{}
	for rows.Next() {
		var s apiTokenSatir
		var ak int
		if rows.Scan(&s.ID, &s.Ad, &s.Onek, &ak, &s.BitisAt, &s.SonKullanimAt,
			&s.SonKullanimIP, &s.CreatedAt) == nil {
			s.Aktif = ak == 1
			out = append(out, s)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tokenlar": out})
}

type apiTokenOlusturReq struct {
	Ad string `json:"ad"`
	// GunSonra: kaç gün sonra sona ersin. 0 = süresiz.
	GunSonra int `json:"gun_sonra"`
}

// APITokenOlustur — POST /api/v1/me/api-tokenlari
//
// Ham token YALNIZ bu yanıtta döner ve bir daha elde edilemez (yalnız SHA-256
// özeti saklanır).
func (h *Handlers) APITokenOlustur(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "oturum yok")
		return
	}
	var b apiTokenOlusturReq
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	b.Ad = strings.TrimSpace(b.Ad)
	if b.Ad == "" || len(b.Ad) > 64 {
		httpx.WriteError(w, http.StatusBadRequest, "token adı 1-64 karakter olmalı")
		return
	}
	if b.GunSonra < 0 || b.GunSonra > 3650 {
		httpx.WriteError(w, http.StatusBadRequest, "gün 0-3650 arası olmalı (0=süresiz)")
		return
	}
	// Hesap başına makul bir üst sınır — sızıntı durumunda temizlenmesi gereken
	// yüzey sınırlı kalsın.
	var adet int
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM api_tokenlari WHERE user_id=?`, c.UserID).Scan(&adet)
	if adet >= 20 {
		httpx.WriteError(w, http.StatusBadRequest, "en fazla 20 API token oluşturabilirsiniz")
		return
	}

	var bitis any
	if b.GunSonra > 0 {
		bitis = time.Now().AddDate(0, 0, b.GunSonra).Format("2006-01-02 15:04:05")
	}
	ham, id, err := auth.APITokenUret(h.DB, c.UserID, b.Ad, bitis)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "token üretilemedi: "+err.Error())
		return
	}
	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "apitoken.olustur", b.Ad, true)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok": true, "id": id, "ad": b.Ad,
		// Tek gösterim — istemci bunu kaydetmezse bir daha alamaz.
		"token": ham,
	})
}

// APITokenSil — DELETE /api/v1/me/api-tokenlari/{id}
// Yalnız kendi token'ını siler; başkasının id'si verilse bile eşleşmez.
func (h *Handlers) APITokenSil(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "oturum yok")
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	res, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM api_tokenlari WHERE id=? AND user_id=?`, id, c.UserID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silinemedi")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.WriteError(w, http.StatusNotFound, "token bulunamadı")
		return
	}
	auth.WriteAudit(h.DB, c.UserID, c.Username, httpx.ClientIP(r), "apitoken.sil",
		strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
