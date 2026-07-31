package mail

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"sanalcp/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Autoresponder struct {
	MailboxID    int64  `json:"mailbox_id"`
	Email        string `json:"email"`
	Enabled      bool   `json:"enabled"`
	Subject      string `json:"subject"`
	Body         string `json:"body"`
	IntervalDays int    `json:"interval_days"`
}

type MailFilter struct {
	ID          int64  `json:"id"`
	MailboxID   int64  `json:"mailbox_id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	MatchField  string `json:"match_field"`
	MatchValue  string `json:"match_value"`
	ActionType  string `json:"action_type"`
	ActionValue string `json:"action_value"`
	Priority    int    `json:"priority"`
	Enabled     bool   `json:"enabled"`
}

var sieveFolderRE = regexp.MustCompile(`^[A-Za-z0-9 _.-]{1,64}$`)

func (h *Handlers) AutoresponderGet(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	mid, _ := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	var a Autoresponder
	var enabled int
	err := h.DB.QueryRowContext(r.Context(), `
		SELECT m.id, m.email, a.enabled, a.subject_text, a.body_text, a.interval_days
		FROM mailboxes m LEFT JOIN mail_autoresponders a ON a.mailbox_id=m.id
		WHERE m.id=? AND m.domain_id=?`, mid, id).
		Scan(&a.MailboxID, &a.Email, &enabled, &a.Subject, &a.Body, &a.IntervalDays)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "posta kutusu bulunamadı")
		return
	}
	if err != nil {
		// LEFT JOIN NULL means mailbox exists but no responder.
		var email string
		if e := h.DB.QueryRowContext(r.Context(),
			`SELECT email FROM mailboxes WHERE id=? AND domain_id=?`, mid, id).Scan(&email); e == nil {
			httpx.WriteJSON(w, http.StatusOK, Autoresponder{MailboxID: mid, Email: email, IntervalDays: 7})
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "otomatik yanıtlayıcı okunamadı")
		return
	}
	a.Enabled = enabled == 1
	httpx.WriteJSON(w, http.StatusOK, a)
}

func (h *Handlers) AutoresponderPut(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	mid, _ := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	var req Autoresponder
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.Body = strings.TrimSpace(req.Body)
	if req.Subject == "" || req.Body == "" || len(req.Subject) > 255 || len(req.Body) > 10000 {
		httpx.WriteError(w, http.StatusBadRequest, "konu ve 10.000 karaktere kadar mesaj zorunlu")
		return
	}
	if req.IntervalDays < 1 || req.IntervalDays > 30 {
		httpx.WriteError(w, http.StatusBadRequest, "yanıt aralığı 1–30 gün olmalı")
		return
	}
	if !h.mailboxBelongs(r.Context(), id, mid) {
		httpx.WriteError(w, http.StatusNotFound, "posta kutusu bulunamadı")
		return
	}
	var oldEnabled int
	var oldSubject, oldBody string
	var oldDays int
	oldErr := h.DB.QueryRowContext(r.Context(), `SELECT enabled,subject_text,body_text,interval_days
		FROM mail_autoresponders WHERE mailbox_id=?`, mid).
		Scan(&oldEnabled, &oldSubject, &oldBody, &oldDays)
	_, err := h.DB.ExecContext(r.Context(), `
		INSERT INTO mail_autoresponders(mailbox_id, domain_id, enabled, subject_text, body_text, interval_days)
		VALUES(?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE enabled=VALUES(enabled), subject_text=VALUES(subject_text),
		  body_text=VALUES(body_text), interval_days=VALUES(interval_days)`,
		mid, id, req.Enabled, req.Subject, req.Body, req.IntervalDays)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "otomatik yanıtlayıcı kaydedilemedi")
		return
	}
	if err := ApplyMailboxSieve(r.Context(), h.DB, mid); err != nil {
		if oldErr == nil {
			_, _ = h.DB.Exec(`UPDATE mail_autoresponders SET enabled=?,subject_text=?,body_text=?,
				interval_days=? WHERE mailbox_id=?`, oldEnabled, oldSubject, oldBody, oldDays, mid)
		} else {
			_, _ = h.DB.Exec(`DELETE FROM mail_autoresponders WHERE mailbox_id=?`, mid)
		}
		httpx.WriteError(w, http.StatusServiceUnavailable, "Sieve uygulanamadı: "+err.Error())
		return
	}
	h.audit(r, "mail.autoresponder.update", strconv.FormatInt(mid, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handlers) AutoresponderDelete(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok || demo {
		httpx.WriteError(w, http.StatusForbidden, "işlem yapılamaz")
		return
	}
	mid, _ := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	if !h.mailboxBelongs(r.Context(), id, mid) {
		httpx.WriteError(w, http.StatusNotFound, "posta kutusu bulunamadı")
		return
	}
	_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM mail_autoresponders WHERE mailbox_id=?`, mid)
	if err := ApplyMailboxSieve(r.Context(), h.DB, mid); err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	h.audit(r, "mail.autoresponder.delete", strconv.FormatInt(mid, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handlers) FilterList(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT f.id, f.mailbox_id, m.email, f.name, f.match_field, f.match_value,
		       f.action_type, f.action_value, f.priority_n, f.enabled
		FROM mail_filters f JOIN mailboxes m ON m.id=f.mailbox_id
		WHERE f.domain_id=? ORDER BY m.email, f.priority_n, f.id`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "filtreler okunamadı")
		return
	}
	defer rows.Close()
	out := []MailFilter{}
	for rows.Next() {
		var f MailFilter
		var enabled int
		if rows.Scan(&f.ID, &f.MailboxID, &f.Email, &f.Name, &f.MatchField, &f.MatchValue,
			&f.ActionType, &f.ActionValue, &f.Priority, &enabled) == nil {
			f.Enabled = enabled == 1
			out = append(out, f)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) FilterCreate(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok || demo {
		httpx.WriteError(w, http.StatusForbidden, "işlem yapılamaz")
		return
	}
	var req MailFilter
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.MatchValue = strings.TrimSpace(req.MatchValue)
	req.ActionValue = strings.TrimSpace(req.ActionValue)
	if !h.mailboxBelongs(r.Context(), id, req.MailboxID) {
		httpx.WriteError(w, http.StatusNotFound, "posta kutusu bulunamadı")
		return
	}
	if err := validateFilter(req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Priority == 0 {
		req.Priority = 100
	}
	res, err := h.DB.ExecContext(r.Context(), `
		INSERT INTO mail_filters(mailbox_id, domain_id, name, match_field, match_value,
		  action_type, action_value, priority_n, enabled) VALUES(?,?,?,?,?,?,?,?,?)`,
		req.MailboxID, id, req.Name, req.MatchField, req.MatchValue,
		req.ActionType, req.ActionValue, req.Priority, req.Enabled)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "filtre kaydedilemedi")
		return
	}
	fid, _ := res.LastInsertId()
	if err := ApplyMailboxSieve(r.Context(), h.DB, req.MailboxID); err != nil {
		_, _ = h.DB.Exec(`DELETE FROM mail_filters WHERE id=?`, fid)
		httpx.WriteError(w, http.StatusServiceUnavailable, "Sieve uygulanamadı: "+err.Error())
		return
	}
	h.audit(r, "mail.filter.create", strconv.FormatInt(fid, 10), true)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": fid})
}

func (h *Handlers) FilterDelete(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok || demo {
		httpx.WriteError(w, http.StatusForbidden, "işlem yapılamaz")
		return
	}
	fid, _ := strconv.ParseInt(chi.URLParam(r, "fid"), 10, 64)
	var mid int64
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT mailbox_id FROM mail_filters WHERE id=? AND domain_id=?`, fid, id).Scan(&mid); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "filtre bulunamadı")
		return
	}
	_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM mail_filters WHERE id=?`, fid)
	if err := ApplyMailboxSieve(r.Context(), h.DB, mid); err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	h.audit(r, "mail.filter.delete", strconv.FormatInt(fid, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func validateFilter(f MailFilter) error {
	if f.Name == "" || len(f.Name) > 128 || f.MatchValue == "" || len(f.MatchValue) > 255 {
		return errors.New("filtre adı ve eşleşme değeri zorunlu")
	}
	if f.MatchField != "from" && f.MatchField != "to" && f.MatchField != "subject" {
		return errors.New("eşleşme alanı geçersiz")
	}
	switch f.ActionType {
	case "move":
		if !sieveFolderRE.MatchString(f.ActionValue) {
			return errors.New("hedef klasör geçersiz")
		}
	case "redirect":
		if !reDestEmail.MatchString(strings.ToLower(f.ActionValue)) {
			return errors.New("yönlendirme adresi geçersiz")
		}
	case "discard":
	default:
		return errors.New("filtre aksiyonu geçersiz")
	}
	return nil
}

func (h *Handlers) mailboxBelongs(ctx context.Context, domainID, mailboxID int64) bool {
	var n int
	_ = h.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mailboxes WHERE id=? AND domain_id=?`, mailboxID, domainID).Scan(&n)
	return n == 1
}

func ApplyMailboxSieve(ctx context.Context, db *sql.DB, mailboxID int64) error {
	if _, err := exec.LookPath("sievec"); err != nil {
		return fmt.Errorf("dovecot-pigeonhole kurulu değil")
	}
	var home, email string
	var uid, gid int
	if err := db.QueryRowContext(ctx, `
		SELECT TRIM(TRAILING '/' FROM m.maildir), m.email, md.uid_n, md.gid_n
		FROM mailboxes m JOIN mail_domains md ON md.id=m.mail_domain_id WHERE m.id=?`, mailboxID).
		Scan(&home, &email, &uid, &gid); err != nil {
		return err
	}
	var out bytes.Buffer
	out.WriteString(`require ["fileinto", "vacation", "mailbox"];

# Rspamd tarafından işaretlenen postaları Junk'a taşı.
if header :contains "X-Spam" "Yes" {
  fileinto :create "Junk";
  stop;
}
`)
	rows, err := db.QueryContext(ctx, `
		SELECT match_field, match_value, action_type, action_value
		FROM mail_filters WHERE mailbox_id=? AND enabled=1 ORDER BY priority_n,id`, mailboxID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var field, value, action, actionValue string
		if err := rows.Scan(&field, &value, &action, &actionValue); err != nil {
			rows.Close()
			return err
		}
		header := map[string]string{"from": "From", "to": "To", "subject": "Subject"}[field]
		fmt.Fprintf(&out, "\nif header :contains %s %s {\n", sieveQuote(header), sieveQuote(value))
		switch action {
		case "move":
			fmt.Fprintf(&out, "  fileinto :create %s;\n", sieveQuote(actionValue))
		case "redirect":
			fmt.Fprintf(&out, "  redirect %s;\n", sieveQuote(actionValue))
		case "discard":
			out.WriteString("  discard;\n")
		}
		out.WriteString("  stop;\n}\n")
	}
	rows.Close()

	var enabled int
	var subject, body string
	var days int
	err = db.QueryRowContext(ctx, `SELECT enabled, subject_text, body_text, interval_days
		FROM mail_autoresponders WHERE mailbox_id=?`, mailboxID).
		Scan(&enabled, &subject, &body, &days)
	if err == nil && enabled == 1 {
		fmt.Fprintf(&out, "\nvacation :days %d :subject %s text:\n%s\n.\n;\n",
			days, sieveQuote(subject), sieveMultiline(body))
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(home, ".dovecot.sieve.new")
	active := filepath.Join(home, ".dovecot.sieve")
	if err := os.WriteFile(tmp, out.Bytes(), 0o600); err != nil {
		return err
	}
	_ = os.Chown(tmp, uid, gid)
	if output, err := exec.CommandContext(ctx, "sievec", tmp).CombinedOutput(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("sievec: %s", strings.TrimSpace(string(output)))
	}
	if err := os.Rename(tmp, active); err != nil {
		return err
	}
	_ = os.Chown(active, uid, gid)
	compiledTmp := tmp + ".svbin"
	compiled := active + ".svbin"
	if _, err := os.Stat(compiledTmp); err == nil {
		_ = os.Rename(compiledTmp, compiled)
		_ = os.Chown(compiled, uid, gid)
	}
	_ = email
	return nil
}

func sieveMultiline(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, ".") {
			lines[i] = "." + line
		}
	}
	return strings.Join(lines, "\n")
}

// sieveQuote — değeri Sieve quoted-string olarak kaçışlar (RFC 5228 §2.4.2).
//
// Satır sonu boşluğa çevrilir: quoted-string içinde satır sonunu temsil eden bir
// kaçış YOKTUR — ters bölü yalnız kendisinden sonraki karakteri harfi harfine
// alır, yani `\n` "n" demektir. Eskiden üretilen `\n`, çok satırlı bir eşleşme
// değerini sessizce "…n…" hâline getiriyordu.
func sieveQuote(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
