package mail

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sanalpanel/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type SendLimits struct {
	MailboxID       int64  `json:"mailbox_id"`
	Email           string `json:"email"`
	HourLimit       int    `json:"hour_limit"`
	DayLimit        int    `json:"day_limit"`
	SentHour        int    `json:"sent_hour"`
	SentDay         int    `json:"sent_day"`
	SpamSuspendedAt string `json:"spam_suspended_at,omitempty"`
}

func StartPolicyServer(db *sql.DB, address string) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Printf("mail policy dinlenemedi (%s): %v", address, err)
		return
	}
	log.Printf("mail gönderim policy servisi %s üzerinde", address)
	go func() {
		defer listener.Close()
		// Accept hatasında koşulsuz `continue` etmek, listener kalıcı olarak
		// bozulduğunda (kapanmış fd) sonsuz bir CPU döngüsü yaratır. Kapanma
		// kalıcıdır → çık; geçici hatalarda (fd/bellek baskısı) kısa bir geri
		// çekilmeyle devam et, art arda hatada süreyi ikiye katla.
		bekleme := 5 * time.Millisecond
		const enFazlaBekleme = time.Second
		for {
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					log.Printf("mail policy dinleyici kapandı: %v", err)
					return
				}
				log.Printf("mail policy accept: %v", err)
				time.Sleep(bekleme)
				if bekleme < enFazlaBekleme {
					bekleme *= 2
				}
				continue
			}
			bekleme = 5 * time.Millisecond
			go handlePolicyConnection(db, conn)
		}
	}()
	go gonderimGunluguTemizle(db)
}

// gonderimGunluguTemizle — mail_send_log her giden posta için bir satır yazar ve
// hiçbir şey silmezse aylar içinde milyonlarca satıra çıkar; policy her mailde bu
// tablo üzerinde iki SUM koşturduğu için maliyet doğrudan gönderim gecikmesine
// yansır. Limit pencereleri 1 saat ve 1 gün olduğundan 2 gün fazlasıyla yeterli
// (git_webhook_deliveries'teki 30 günlük temizlikle aynı desen).
func gonderimGunluguTemizle(db *sql.DB) {
	const parti = 50000
	for {
		// Tek DELETE ile milyonlarca satır silmek InnoDB'de uzun kilit demektir;
		// partiler halinde sil ve biriken geçmişi ilk turda tamamen tüket.
		for tur := 0; tur < 200; tur++ {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			res, err := db.ExecContext(ctx,
				`DELETE FROM mail_send_log WHERE ts < NOW()-INTERVAL 2 DAY LIMIT ?`, parti)
			cancel()
			if err != nil {
				log.Printf("mail_send_log temizliği: %v", err)
				break
			}
			if n, err := res.RowsAffected(); err != nil || n < parti {
				break
			}
		}
		time.Sleep(time.Hour)
	}
}

func handlePolicyConnection(db *sql.DB, conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	scanner := bufio.NewScanner(conn)
	attrs := map[string]string{}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			action := evaluateSendPolicy(db, attrs)
			_, _ = fmt.Fprintf(conn, "action=%s\n\n", action)
			attrs = map[string]string{}
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			attrs[key] = value
		}
	}
}

func evaluateSendPolicy(db *sql.DB, attrs map[string]string) string {
	email := strings.ToLower(strings.TrimSpace(attrs["sasl_username"]))
	if email == "" {
		return "DUNNO"
	}
	recipients, _ := strconv.Atoi(attrs["recipient_count"])
	if recipients < 1 {
		recipients = 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "DUNNO"
	}
	defer tx.Rollback()
	var mailboxID, domainID int64
	var status string
	var hourLimit, dayLimit int
	err = tx.QueryRowContext(ctx, `SELECT id, domain_id, status, send_limit_hour, send_limit_day
		FROM mailboxes WHERE email=? FOR UPDATE`, email).
		Scan(&mailboxID, &domainID, &status, &hourLimit, &dayLimit)
	if err != nil {
		return "DUNNO"
	}
	if status != "active" {
		return "REJECT 5.7.1 Posta hesabı aktif değil"
	}
	var sentHour, sentDay int
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(recipient_count),0) FROM mail_send_log
		WHERE mailbox_id=? AND ok=1 AND ts >= NOW()-INTERVAL 1 HOUR`, mailboxID).Scan(&sentHour)
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(recipient_count),0) FROM mail_send_log
		WHERE mailbox_id=? AND ok=1 AND ts >= NOW()-INTERVAL 1 DAY`, mailboxID).Scan(&sentDay)
	exceeded := (hourLimit > 0 && sentHour+recipients > hourLimit) ||
		(dayLimit > 0 && sentDay+recipients > dayLimit)
	if exceeded {
		_, _ = tx.ExecContext(ctx, `UPDATE mailboxes
			SET status='suspended', spam_suspended_at=NOW() WHERE id=?`, mailboxID)
		_, _ = tx.ExecContext(ctx, `INSERT INTO mail_send_log(mailbox_id,domain_id,ok,recipient_count)
			VALUES(?,?,0,?)`, mailboxID, domainID, recipients)
		_ = tx.Commit()
		log.Printf("mail spam koruması: %s otomatik askıya alındı (saat=%d/%d gün=%d/%d)",
			email, sentHour, hourLimit, sentDay, dayLimit)
		return "REJECT 5.7.1 Gönderim limiti aşıldı; hesap güvenlik amacıyla askıya alındı"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mail_send_log(mailbox_id,domain_id,ok,recipient_count)
		VALUES(?,?,1,?)`, mailboxID, domainID, recipients); err != nil {
		return "DUNNO"
	}
	if err := tx.Commit(); err != nil {
		return "DUNNO"
	}
	return "DUNNO"
}

func (h *Handlers) SendLimitsGet(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	mid, _ := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	var s SendLimits
	var suspended sql.NullString
	err := h.DB.QueryRowContext(r.Context(), `
		SELECT m.id,m.email,m.send_limit_hour,m.send_limit_day,
		  (SELECT COALESCE(SUM(recipient_count),0) FROM mail_send_log l WHERE l.mailbox_id=m.id AND l.ok=1 AND l.ts>=NOW()-INTERVAL 1 HOUR),
		  (SELECT COALESCE(SUM(recipient_count),0) FROM mail_send_log l WHERE l.mailbox_id=m.id AND l.ok=1 AND l.ts>=NOW()-INTERVAL 1 DAY),
		  DATE_FORMAT(m.spam_suspended_at,'%Y-%m-%d %H:%i')
		FROM mailboxes m WHERE m.id=? AND m.domain_id=?`, mid, id).
		Scan(&s.MailboxID, &s.Email, &s.HourLimit, &s.DayLimit, &s.SentHour, &s.SentDay, &suspended)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "posta kutusu bulunamadı")
		return
	}
	if suspended.Valid {
		s.SpamSuspendedAt = suspended.String
	}
	httpx.WriteJSON(w, http.StatusOK, s)
}

func (h *Handlers) SendLimitsPut(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok || demo {
		httpx.WriteError(w, http.StatusForbidden, "işlem yapılamaz")
		return
	}
	mid, _ := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	var req SendLimits
	if json.NewDecoder(r.Body).Decode(&req) != nil ||
		req.HourLimit < 0 || req.HourLimit > 100000 ||
		req.DayLimit < 0 || req.DayLimit > 100000 ||
		(req.HourLimit > 0 && req.DayLimit > 0 && req.HourLimit > req.DayLimit) {
		httpx.WriteError(w, http.StatusBadRequest, "limitler 0–100000 arasında; saat limiti gün limitini aşmamalı")
		return
	}
	if !h.mailboxBelongs(r.Context(), id, mid) {
		httpx.WriteError(w, http.StatusNotFound, "posta kutusu bulunamadı")
		return
	}
	_, err := h.DB.ExecContext(r.Context(), `UPDATE mailboxes
		SET send_limit_hour=?,send_limit_day=? WHERE id=? AND domain_id=?`,
		req.HourLimit, req.DayLimit, mid, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "limit kaydedilemedi")
		return
	}
	h.audit(r, "mail.send_limits.update", strconv.FormatInt(mid, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
