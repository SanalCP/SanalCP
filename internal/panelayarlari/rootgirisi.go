package panelayarlari

// Panelin root/shadow giriş yolu anahtarı (bkz. migrations/0069).
//
// Bu bayrak YALNIZ :8443 panel girişini etkiler. Sunucunun SSH root erişimi,
// root parolası ve sshd yapılandırması bundan HİÇ etkilenmez.

import (
	"encoding/json"
	"net/http"

	"sanalcp/internal/httpx"
	"sanalcp/internal/panelbayrak"
)

// RootGirisi — GET /api/v1/system/root-girisi (AdminOnly).
func (h *Handlers) RootGirisi(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"acik": panelbayrak.RootGirisiAcik(r.Context(), h.DB),
	})
}

type rootGirisiKaydetReq struct {
	Acik bool `json:"acik"`
}

// RootGirisiKaydet — PUT /api/v1/system/root-girisi (AdminOnly).
//
// KİLİTLENME KORUMASI: kapatma yönünde, sistemde aktif ve root OLMAYAN bir
// admin hesabı yoksa istek reddedilir. Aksi halde tek istekle kendini dışarı
// kilitlemek mümkün olurdu — root kapanır, girebilecek başka hesap yoktur ve
// geri açacak oturum da açılamaz. Açma yönünde böyle bir risk yok, sayım
// yapılmaz.
//
// Bu kontrol internal/users.sonAdminMi'den BAĞIMSIZDIR: o, hesap silmeyi/
// pasifleştirmeyi korur; bu, bayrağın kendisini kapatmayı korur. İkisi de
// gerekli — biri olmadan diğeri kilitlenmeyi tek adımda mümkün bırakır.
func (h *Handlers) RootGirisiKaydet(w http.ResponseWriter, r *http.Request) {
	var req rootGirisiKaydetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}

	if !req.Acik {
		var n int
		if err := h.DB.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM users WHERE role='admin' AND status='active' AND id<>1`).
			Scan(&n); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "admin sayısı okunamadı")
			return
		}
		if n == 0 {
			httpx.WriteError(w, http.StatusBadRequest,
				"root girişi kapatılamaz: önce root dışında aktif bir yönetici hesabı oluşturun")
			return
		}
	}

	deger := 0
	if req.Acik {
		deger = 1
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE panel_ayarlari SET root_girisi_acik=? WHERE id=1`, deger); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "acik": req.Acik})
}
