package domains

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"sanalcp/internal/httpx"
	"sanalcp/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

type sslIssueReq struct {
	Tip string `json:"tip"` // "self-signed" | "letsencrypt"
}

type sslDurumResp struct {
	Aktif    bool   `json:"aktif"`
	Kaynak   string `json:"kaynak"`
	BitisISO string `json:"bitis_iso,omitempty"`
	CertYol  string `json:"cert_yol,omitempty"`
	KeyYol   string `json:"key_yol,omitempty"`
}

func (h *Handlers) SSLDurum(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var aktif int
	var kaynak, certYol, keyYol, bitis string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT ssl_aktif, ssl_kaynak, cert_path, key_path,
		   COALESCE(DATE_FORMAT(ssl_bitis,'%Y-%m-%dT%H:%i:%sZ'),'')
		 FROM domains WHERE id=?`, id).
		Scan(&aktif, &kaynak, &certYol, &keyYol, &bitis)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okuma: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sslDurumResp{
		Aktif:    aktif == 1,
		Kaynak:   kaynak,
		BitisISO: bitis,
		CertYol:  certYol,
		KeyYol:   keyYol,
	})
}

func (h *Handlers) SSLIssue(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req sslIssueReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if req.Tip == "" {
		req.Tip = "self-signed"
	}
	if req.Tip != "self-signed" && req.Tip != "letsencrypt" {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz tip (self-signed|letsencrypt)")
		return
	}
	var alanAdi, sk, phpSurum, backend string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT alan_adi, sistem_kullanici, php_surum, is_demo, COALESCE(web_backend,'php-fpm') FROM domains WHERE id=?`, id).
		Scan(&alanAdi, &sk, &phpSurum, &isDemo, &backend)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğe SSL kurulamaz")
		return
	}

	var certYol, keyYol string
	// gercekTip: FIILEN kurulan sertifika tipi. req.Tip "letsencrypt" istese bile,
	// LE cekimi basarisiz olup sslFailSafe self-signed'a duserse gercekTip
	// "self-signed" kalir — panel kendi DB'sine ASLA yalan soylemez (canlida
	// gozlemlenen "kuruldu gosteriyor ama kurmuyor" hatasinin kok nedeni buydu).
	gercekTip := req.Tip
	// sslNot: LE akışının kullanıcıya iletilecek açıklaması (ör. "www DNS'te yok,
	// sertifikaya eklenmedi" veya apex hiç çözülmüyorsa sebebi).
	var sslNot string
	switch req.Tip {
	case "self-signed":
		certYol, keyYol, err = provisioner.EnableSelfSigned(alanAdi, sk, phpSurum, backend)
	case "letsencrypt":
		var gercek bool
		certYol, keyYol, gercek, sslNot, err = provisioner.EnableLetsEncrypt(r.Context(), alanAdi, sk, phpSurum, backend)
		if !gercek {
			gercekTip = "self-signed"
		}
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "SSL kurulum: "+err.Error())
		return
	}

	bitis := time.Now().Add(365 * 24 * time.Hour)
	if gercekTip == "letsencrypt" {
		bitis = time.Now().Add(90 * 24 * time.Hour)
	}

	// Sertifika kurulumu (EnableLetsEncrypt/EnableSelfSigned) burada zaten
	// TAMAMLANDI — sunucuda gerçekten kuruldu. İstemci bu noktaya kadar
	// beklerken axios timeout'una (30s) takılıp bağlantıyı keserse r.Context()
	// iptal edilir ve aşağıdaki UPDATE hiç yazılmaz; halbuki iş fiilen bitmiştir.
	// Bu yüzden bu son DB yazımını istek bağlamından bağımsız tutuyoruz.
	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.DB.ExecContext(dbCtx,
		`UPDATE domains SET ssl_aktif=1, ssl_kaynak=?, cert_path=?, key_path=?, ssl_bitis=?
		 WHERE id=?`, gercekTip, certYol, keyYol, bitis, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}

	resp := map[string]any{
		"ok":    true,
		"id":    id,
		"tip":   gercekTip,
		"cert":  certYol,
		"key":   keyYol,
		"bitis": bitis.Format("2006-01-02"),
	}
	// Kullanıcıya NE OLDUĞUNU söyle. Eskiden yalnız genel bir "DNS işaret
	// etmiyor olabilir" metni vardı; gerçek sebep (hangi host, neden) panelde
	// hiç görünmediği için kullanıcı "SSL kuruldu ama tarayıcı uyarı veriyor"
	// durumunda kalıyordu.
	if req.Tip == "letsencrypt" && gercekTip != "letsencrypt" {
		uyari := "Let's Encrypt sertifikası alınamadı; site geçici olarak self-signed sertifika ile korunuyor (tarayıcı uyarı gösterir). "
		if sslNot != "" {
			uyari += sslNot
		} else {
			uyari += "Domain DNS'i bu sunucuya işaret etmiyor olabilir; DNS düzelince tekrar deneyin."
		}
		resp["uyari"] = uyari
	} else if sslNot != "" {
		// LE BAŞARILI ama sertifikaya bir host eklenemedi (ör. www kaydı yok).
		resp["bilgi"] = sslNot
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handlers) SSLDisable(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var alanAdi, sk, phpSurum, backend string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT alan_adi, sistem_kullanici, php_surum, is_demo, COALESCE(web_backend,'php-fpm') FROM domains WHERE id=?`, id).
		Scan(&alanAdi, &sk, &phpSurum, &isDemo, &backend)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo abonelik dokunulamaz")
		return
	}
	if err := provisioner.DisableSSL(alanAdi, sk, phpSurum, backend); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "SSL kapat: "+err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET ssl_aktif=0, ssl_kaynak='', cert_path='', key_path='', ssl_bitis=NULL
		 WHERE id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GET /domains/{id}/www-yonlendir — apex (ör. sanalcp.com) ziyaret edilince
// www alt alan adına (www.sanalcp.com) 301 yönlendirilsin mi?
func (h *Handlers) WWWYonlendirDurum(w http.ResponseWriter, r *http.Request) {
	id, _, _, _, ok := h.domainInfo(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	var aktif int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT www_yonlendir FROM domains WHERE id=?`, id).Scan(&aktif); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okunamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"aktif": aktif == 1})
}

// PUT /domains/{id}/www-yonlendir  {aktif: bool}
func (h *Handlers) WWWYonlendirAyarla(w http.ResponseWriter, r *http.Request) {
	id, sk, phpSurum, demo, ok := h.domainInfo(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	var req struct {
		Aktif bool `json:"aktif"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	deger := 0
	if req.Aktif {
		deger = 1
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET www_yonlendir=? WHERE id=?`, deger, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kaydedilemedi: "+err.Error())
		return
	}
	if err := h.applyVhost(r, id, sk, phpSurum); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "vhost render: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "aktif": req.Aktif})
}
