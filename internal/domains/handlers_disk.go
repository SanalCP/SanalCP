package domains

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"sanalcp/internal/adlar"
	"sanalcp/internal/diskusage"
	"sanalcp/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// DiskHesapla: du -sb /home/c_<user>/public_html → boyut_kb DB'ye yaz
func (h *Handlers) DiskHesapla(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var sk string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, is_demo FROM domains WHERE id=?`, id).
		Scan(&sk, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo abonelik disk hesabı yapılamaz")
		return
	}
	if !adlar.SKGecerli(sk) {
		httpx.WriteError(w, http.StatusBadRequest, "güvenlik")
		return
	}
	path := "/home/" + sk
	byteB, err := diskusage.Measure(r.Context(), path, true)
	if err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "disk ölçümü tamamlanamadı; lütfen yeniden deneyin")
		return
	}
	kb := byteB / 1024
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET boyut_kb=? WHERE id=?`, kb, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"id":       id,
		"boyut_kb": kb,
		"path":     path,
	})
}
