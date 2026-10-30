package uyari

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"sanalcp/internal/httpx"
	"sanalcp/internal/middleware"
)

type Handlers struct {
	DB        *sql.DB
	Gonderici Gonderici
}

var (
	chatIDRe = regexp.MustCompile(`^(-?\d{1,20}|@[A-Za-z0-9_]{5,32})$`)
	tokenRe  = regexp.MustCompile(`^\d+:[A-Za-z0-9_-]{30,}$`)
)

// GET /system/uyari-bildirim — token asla dönmez, yalnız ayarlı olup olmadığı.
func (h *Handlers) Ayar(w http.ResponseWriter, r *http.Request) {
	a, err := AyarOku(r.Context(), h.DB)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "ayar okunamadı")
		return
	}
	if middleware.DemoPaneliMi(r) {
		a.TelegramChatID, a.Eposta = "", "" // demo ziyaretçisi admin'in iletişim bilgisini görmesin
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"acik": a.Acik, "telegram_token_ayarli": a.TelegramToken != "",
		"telegram_chat_id": a.TelegramChatID, "eposta": a.Eposta,
	})
}

// PUT /system/uyari-bildirim — telegram_token: "" mevcut token'ı korur, "-" siler.
func (h *Handlers) AyarKaydet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Acik           bool   `json:"acik"`
		TelegramToken  string `json:"telegram_token"`
		TelegramChatID string `json:"telegram_chat_id"`
		Eposta         string `json:"eposta"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	req.TelegramToken, req.TelegramChatID, req.Eposta = strings.TrimSpace(req.TelegramToken), strings.TrimSpace(req.TelegramChatID), strings.TrimSpace(req.Eposta)
	if req.TelegramChatID != "" && !chatIDRe.MatchString(req.TelegramChatID) {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz Telegram chat ID")
		return
	}
	if req.Eposta != "" {
		// Yalın adres şartı: "Ad <a@b>" biçimi e-posta başlığına olduğu gibi yazılırdı.
		if a, err := mail.ParseAddress(req.Eposta); err != nil || a.Address != req.Eposta || len(req.Eposta) > 320 {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz e-posta adresi")
			return
		}
	}
	sorgu := `UPDATE panel_ayarlari SET bildirim_acik=?, telegram_chat_id=?, bildirim_eposta=?`
	args := []any{req.Acik, req.TelegramChatID, req.Eposta}
	switch req.TelegramToken {
	case "":
	case "-":
		sorgu += `, telegram_token_enc=NULL`
	default:
		if !tokenRe.MatchString(req.TelegramToken) {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz Telegram bot token")
			return
		}
		if box == nil {
			httpx.WriteError(w, http.StatusInternalServerError, "şifreleme kutusu hazır değil")
			return
		}
		enc, err := box.Encrypt(req.TelegramToken)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "token şifrelenemedi")
			return
		}
		sorgu += `, telegram_token_enc=?`
		args = append(args, enc)
	}
	if _, err := h.DB.ExecContext(r.Context(), sorgu+` WHERE id=1`, args...); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "ayar kaydedilemedi")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /system/uyari-bildirim/test — kayıtlı kanallara (açık/kapalı fark etmeksizin) deneme mesajı.
func (h *Handlers) Test(w http.ResponseWriter, r *http.Request) {
	a, err := AyarOku(r.Context(), h.DB)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "ayar okunamadı")
		return
	}
	if !a.kanalVar() {
		httpx.WriteError(w, http.StatusBadRequest, "bildirim kanalı ayarlı değil")
		return
	}
	if err := h.Gonderici.Gonder(r.Context(), a, "SanalCP test mesajı"); err != nil {
		httpx.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
