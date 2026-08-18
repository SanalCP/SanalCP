package guvenlikduvari

// Otomatik ban ayarlarının HTTP uçları. Ayarlar panel_ayarlari(id=1) satırında
// tutulur; izleyici bunları periyodik okuduğu için değişiklik panel yeniden
// başlatılmadan geçerli olur (bkz. otoban.go).

import (
	"encoding/json"
	"net/http"

	"sanalcp/internal/httpx"
)

type otoBanAyarResp struct {
	Aktif     bool `json:"aktif"`
	Esik      int  `json:"esik"`
	PencereDk int  `json:"pencere_dk"`
	SureDk    int  `json:"sure_dk"`
	// AktifBanSayisi: o an yürürlükte olan otomatik ban sayısı (UI göstergesi).
	AktifBanSayisi int `json:"aktif_ban_sayisi"`
}

// OtoBanAyar — GET /api/v1/firewall/otoban (AdminOnly).
// Önbelleği atlayarak doğrudan sorgular: yönetici kaydettikten hemen sonra
// gerçek değeri görmelidir.
func (h *Handlers) OtoBanAyar(w http.ResponseWriter, r *http.Request) {
	a := ayarSorgula(r.Context(), h.DB)
	resp := otoBanAyarResp{Aktif: a.Aktif, Esik: a.Esik, PencereDk: a.PencereDk, SureDk: a.SureDk}
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM firewall_kurallari
		 WHERE kaynak='otomatik' AND aktif=1 AND (bitis_at IS NULL OR bitis_at > NOW())`).
		Scan(&resp.AktifBanSayisi)
	httpx.WriteJSON(w, http.StatusOK, resp)
}

type otoBanKaydetReq struct {
	Aktif     bool `json:"aktif"`
	Esik      int  `json:"esik"`
	PencereDk int  `json:"pencere_dk"`
	SureDk    int  `json:"sure_dk"`
}

// OtoBanKaydet — PUT /api/v1/firewall/otoban (AdminOnly).
//
// Sınırlar bilinçlidir: eşiğin 1 olması tek bir yanlış parolada ban demektir
// (yöneticinin kendini kilitlemesi için en kısa yol), bu yüzden en az 3 istenir.
// Süre üst sınırı 30 gün — pratikte "kalıcı" ban elle eklenmelidir ki kayıtta
// bilinçli bir yönetici kararı olarak görünsün.
func (h *Handlers) OtoBanKaydet(w http.ResponseWriter, r *http.Request) {
	var req otoBanKaydetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if req.Esik < 3 || req.Esik > 100 {
		httpx.WriteError(w, http.StatusBadRequest, "eşik 3-100 arası olmalı")
		return
	}
	if req.PencereDk < 1 || req.PencereDk > 1440 {
		httpx.WriteError(w, http.StatusBadRequest, "pencere 1-1440 dakika arası olmalı")
		return
	}
	if req.SureDk < 1 || req.SureDk > 43200 {
		httpx.WriteError(w, http.StatusBadRequest, "ban süresi 1-43200 dakika (30 gün) arası olmalı")
		return
	}
	aktif := 0
	if req.Aktif {
		aktif = 1
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE panel_ayarlari SET otoban_aktif=?, otoban_esik=?, otoban_pencere_dk=?, otoban_sure_dk=?
		 WHERE id=1`, aktif, req.Esik, req.PencereDk, req.SureDk); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "ayar kaydedilemedi: "+err.Error())
		return
	}
	ayarCacheBosalt() // izleyici yeni ayarı beklemeden görsün
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// OtoBanTemizle — POST /api/v1/firewall/otoban/temizle (AdminOnly).
// Yürürlükteki TÜM otomatik banları kaldırır. Yanlış yapılandırma sonucu toplu
// ban oluştuğunda yöneticinin tek hamlede geri alabilmesi için vardır; elle
// eklenen kurallara dokunmaz.
func (h *Handlers) OtoBanTemizle(w http.ResponseWriter, r *http.Request) {
	res, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM firewall_kurallari WHERE kaynak='otomatik'`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "temizlenemedi: "+err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if err := h.rebuild(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "firewall güncellenemedi: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "silinen": n})
}
