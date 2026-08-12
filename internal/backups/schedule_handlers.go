package backups

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"sanalcp/internal/httpx"
)

// GET /domains/{id}/backup-schedule
func (h *Handlers) GetSchedule(w http.ResponseWriter, r *http.Request) {
	id, _, _, _, err := h.lookupDomain(r)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var s Schedule
	var last sql.NullString
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COALESCE(backup_freq,'none'), COALESCE(backup_hour,3), COALESCE(backup_retention,7),
		        COALESCE(backup_manuel_retention,0),
		        DATE_FORMAT(last_backup_at,'%Y-%m-%dT%H:%i:%sZ')
		 FROM domains WHERE id=?`, id).
		Scan(&s.Freq, &s.Hour, &s.Retention, &s.ManuelRetention, &last); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if last.Valid {
		s.LastBackupAt = last.String
	}
	httpx.WriteJSON(w, http.StatusOK, s)
}

// PUT /domains/{id}/backup-schedule
func (h *Handlers) SetSchedule(w http.ResponseWriter, r *http.Request) {
	id, _, sk, demo, err := h.lookupDomain(r)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğin yedek planı değiştirilemez")
		return
	}
	var s Schedule
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if !gecerliFreq(s.Freq) {
		httpx.WriteError(w, http.StatusBadRequest, "freq: none|daily|weekly|monthly")
		return
	}
	if s.Hour < 0 || s.Hour > 23 {
		httpx.WriteError(w, http.StatusBadRequest, "hour: 0-23")
		return
	}
	if s.Retention < 1 {
		s.Retention = 1
	}
	if s.Retention > 90 {
		s.Retention = 90
	}
	// Manuel retention'da 0 geçerli bir değer: "sınırsız". Otomatikten farklı
	// olarak 1'e yuvarlanMAZ — yuvarlansaydı ayarı hiç açmamış her domainde
	// tek bir manuel yedek dışındaki hepsi ilk kayıtta silinirdi.
	if s.ManuelRetention < 0 {
		s.ManuelRetention = 0
	}
	if s.ManuelRetention > 90 {
		s.ManuelRetention = 90
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET backup_freq=?, backup_hour=?, backup_retention=?, backup_manuel_retention=? WHERE id=?`,
		s.Freq, s.Hour, s.Retention, s.ManuelRetention, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	// Yeni sınır anında uygulanır: kullanıcı "en fazla 3 manuel yedek" dediğinde
	// bunun bir sonraki manuel yedeğe kadar beklemesi ayarı işlevsiz gösterir.
	if err := pruneManuel(h.DB, id, sk, s.ManuelRetention); err != nil {
		log.Printf("manuel retention domain=%d: %v", id, err)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "schedule": s})
}

// POST /admin/backups/tick — scheduler'ı manuel tetikle (admin only).
// Şu saat slot'unda due olan tüm domainler yedeklenir + retention uygulanır.
func (h *Handlers) TickNow(w http.ResponseWriter, r *http.Request) {
	go TickOnce(h.DB)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "tick": "started"})
}
