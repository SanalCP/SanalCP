package cloudflare

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"sanalcp/internal/httpx"
	"sanalcp/internal/secretcrypt"

	"github.com/go-chi/chi/v5"
)

var box *secretcrypt.Box

func Init(b *secretcrypt.Box) { box = b }

type Handlers struct {
	DB     *sql.DB
	Client *Client
}

func (h *Handlers) client() *Client {
	if h.Client != nil {
		return h.Client
	}
	return NewClient()
}
func (h *Handlers) token(ctx context.Context) (string, error) {
	var enc string
	if err := h.DB.QueryRowContext(ctx, `SELECT api_token FROM cloudflare_settings WHERE id=1`).Scan(&enc); err != nil {
		return "", err
	}
	if box == nil {
		return "", errors.New("şifreleme altyapısı hazır değil")
	}
	return box.Decrypt(enc)
}
func (h *Handlers) domain(ctx context.Context, id int64) (string, bool, error) {
	var name string
	var demo int
	err := h.DB.QueryRowContext(ctx, `SELECT alan_adi,is_demo FROM domains WHERE id=?`, id).Scan(&name, &demo)
	return strings.ToLower(strings.TrimSuffix(name, ".")), demo == 1, err
}
func (h *Handlers) zone(ctx context.Context, id int64) (Zone, error) {
	var z Zone
	err := h.DB.QueryRowContext(ctx, `SELECT zone_id,zone_name,status FROM cloudflare_zones WHERE domain_id=?`, id).Scan(&z.ID, &z.Name, &z.Status)
	return z, err
}

func (h *Handlers) Status(w http.ResponseWriter, r *http.Request) {
	_, err := h.token(r.Context())
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"configured": err == nil})
}
func (h *Handlers) SaveToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		httpx.WriteError(w, 400, "geçersiz gövde")
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if len(req.Token) < 20 || len(req.Token) > 512 {
		httpx.WriteError(w, 400, "geçersiz API token")
		return
	}
	if err := h.client().Verify(r.Context(), req.Token); err != nil {
		httpx.WriteError(w, 400, err.Error())
		return
	}
	if box == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "şifreleme altyapısı hazır değil")
		return
	}
	enc, err := box.Encrypt(req.Token)
	if err != nil {
		httpx.WriteError(w, 500, "token şifrelenemedi")
		return
	}
	_, err = h.DB.ExecContext(r.Context(), `INSERT INTO cloudflare_settings(id,api_token) VALUES(1,?) ON DUPLICATE KEY UPDATE api_token=VALUES(api_token)`, enc)
	if err != nil {
		httpx.WriteError(w, 500, "token kaydedilemedi")
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}
func (h *Handlers) DeleteToken(w http.ResponseWriter, r *http.Request) {
	_, err := h.DB.ExecContext(r.Context(), `DELETE FROM cloudflare_settings WHERE id=1`)
	if err != nil {
		httpx.WriteError(w, 500, "token silinemedi")
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handlers) DomainStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	name, _, err := h.domain(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, 404, "domain bulunamadı")
		return
	}
	_, tokErr := h.token(r.Context())
	z, zerr := h.zone(r.Context(), id)
	httpx.WriteJSON(w, 200, map[string]any{"configured": tokErr == nil, "connected": zerr == nil, "domain": name, "zone": z})
}
func (h *Handlers) Connect(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	name, demo, err := h.domain(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, 404, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, 403, "demo domain bağlanamaz")
		return
	}
	tok, err := h.token(r.Context())
	if err != nil {
		httpx.WriteError(w, 409, "önce Cloudflare API token tanımlayın")
		return
	}
	z, err := h.client().FindZone(r.Context(), tok, name)
	if err != nil {
		httpx.WriteError(w, 400, err.Error())
		return
	}
	_, err = h.DB.ExecContext(r.Context(), `INSERT INTO cloudflare_zones(domain_id,zone_id,zone_name,status) VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE zone_id=VALUES(zone_id),zone_name=VALUES(zone_name),status=VALUES(status)`, id, z.ID, z.Name, z.Status)
	if err != nil {
		httpx.WriteError(w, 500, "zone bağlantısı kaydedilemedi")
		return
	}
	httpx.WriteJSON(w, 200, z)
}
func (h *Handlers) Disconnect(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	h.DB.ExecContext(r.Context(), `DELETE FROM cloudflare_zones WHERE domain_id=?`, id)
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}
func (h *Handlers) Records(w http.ResponseWriter, r *http.Request) {
	tok, z, ok := h.authZone(w, r)
	if !ok {
		return
	}
	v, err := h.client().Records(r.Context(), tok, z.ID)
	if err != nil {
		httpx.WriteError(w, 502, err.Error())
		return
	}
	httpx.WriteJSON(w, 200, v)
}

var cfID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

func recordInput(r *http.Request) (RecordInput, error) {
	var in RecordInput
	err := json.NewDecoder(r.Body).Decode(&in)
	in.Type = strings.ToUpper(strings.TrimSpace(in.Type))
	in.Name = strings.TrimSpace(in.Name)
	in.Content = strings.TrimSpace(in.Content)
	if err != nil || in.Name == "" || in.Content == "" || strings.ContainsAny(in.Name+in.Content, "\r\n") {
		return in, errors.New("geçersiz DNS kaydı")
	}
	if in.TTL < 0 {
		in.TTL = 1
	}
	return in, nil
}
func (h *Handlers) CreateRecord(w http.ResponseWriter, r *http.Request) {
	tok, z, ok := h.authZone(w, r)
	if !ok {
		return
	}
	in, err := recordInput(r)
	if err != nil {
		httpx.WriteError(w, 400, err.Error())
		return
	}
	v, err := h.client().CreateRecord(r.Context(), tok, z.ID, in)
	if err != nil {
		httpx.WriteError(w, 502, err.Error())
		return
	}
	httpx.WriteJSON(w, 201, v)
}
func (h *Handlers) UpdateRecord(w http.ResponseWriter, r *http.Request) {
	rid := chi.URLParam(r, "rid")
	if !cfID.MatchString(rid) {
		httpx.WriteError(w, 400, "geçersiz kayıt kimliği")
		return
	}
	tok, z, ok := h.authZone(w, r)
	if !ok {
		return
	}
	in, err := recordInput(r)
	if err != nil {
		httpx.WriteError(w, 400, err.Error())
		return
	}
	v, err := h.client().UpdateRecord(r.Context(), tok, z.ID, rid, in)
	if err != nil {
		httpx.WriteError(w, 502, err.Error())
		return
	}
	httpx.WriteJSON(w, 200, v)
}
func (h *Handlers) DeleteRecord(w http.ResponseWriter, r *http.Request) {
	rid := chi.URLParam(r, "rid")
	if !cfID.MatchString(rid) {
		httpx.WriteError(w, 400, "geçersiz kayıt kimliği")
		return
	}
	tok, z, ok := h.authZone(w, r)
	if !ok {
		return
	}
	if err := h.client().DeleteRecord(r.Context(), tok, z.ID, rid); err != nil {
		httpx.WriteError(w, 502, err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}
func (h *Handlers) Purge(w http.ResponseWriter, r *http.Request) {
	tok, z, ok := h.authZone(w, r)
	if !ok {
		return
	}
	if err := h.client().PurgeCache(r.Context(), tok, z.ID); err != nil {
		httpx.WriteError(w, 502, err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}
func (h *Handlers) authZone(w http.ResponseWriter, r *http.Request) (string, Zone, bool) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	tok, err := h.token(r.Context())
	if err != nil {
		httpx.WriteError(w, 409, "Cloudflare token tanımlı değil")
		return "", Zone{}, false
	}
	z, err := h.zone(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, 409, "domain Cloudflare'a bağlı değil")
		return "", Zone{}, false
	}
	return tok, z, true
}
