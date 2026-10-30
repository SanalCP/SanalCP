package maildelivery

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type trackedMessage struct {
	id        uint64
	sender    string
	messageID string
	auth      string
	uid       sql.NullInt64
	outbound  sql.NullInt64
	first     time.Time
	last      time.Time
	closed    sql.NullTime
}

func eventKey(source, cursor string) [32]byte {
	h := sha256.New()
	_, _ = h.Write([]byte(source))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(cursor))
	var key [32]byte
	h.Sum(key[:0])
	return key
}

func (c *Collector) commit(ctx context.Context, expected readPosition, records []Record, now time.Time, gap bool) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var cursor sql.NullString
	var lastAt sql.NullTime
	var warning string
	if err := tx.QueryRowContext(ctx, "SELECT `cursor`,last_event_at,warning FROM mail_delivery_sources WHERE id=? FOR UPDATE", c.config.SourceID).Scan(&cursor, &lastAt, &warning); err != nil {
		return err
	}
	// A second process may have advanced this source while journalctl ran.
	// Its cursor wins; never move the persisted position backwards.
	if cursor.String != expected.cursor {
		return nil
	}
	for _, record := range records {
		event, ok := Parse(record)
		if ok && !c.trusted(record) {
			ok = false
		}
		if ok {
			key := eventKey(c.config.SourceID, record.Cursor)
			var exists bool
			// Dedup MUST precede queue-instance resolution, including after a
			// removed ID has already been reused by another message.
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM mail_delivery_events WHERE event_key=?)`, key[:]).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				if err := c.apply(ctx, tx, event, key); err != nil {
					return err
				}
			}
		}
		cursor = sql.NullString{String: record.Cursor, Valid: true}
		if !lastAt.Valid || record.At.After(lastAt.Time) {
			lastAt = sql.NullTime{Time: record.At, Valid: true}
		}
	}
	if gap || warning == "history_gap" {
		warning = "history_gap"
	} else {
		warning = ""
	}
	if _, err := tx.ExecContext(ctx, "UPDATE mail_delivery_sources SET `cursor`=?,last_event_at=?,collected_at=?,state='ok',warning=? WHERE id=?", cursor, lastAt, now, warning, c.config.SourceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (c *Collector) apply(ctx context.Context, tx *sql.Tx, e Event, key [32]byte) error {
	m, err := c.message(ctx, tx, e)
	if err != nil {
		return err
	}
	oldSender, oldAuth, oldUID := m.sender, m.auth, m.uid
	if m.sender == "" && e.Sender != "" {
		m.sender = e.Sender
	}
	if m.messageID == "" && e.MessageID != "" {
		m.messageID = e.MessageID
	}
	if m.auth == "" && e.AuthenticatedSender != "" {
		m.auth = e.AuthenticatedSender
	}
	if !m.uid.Valid && e.PickupUID != nil {
		m.uid = sql.NullInt64{Int64: int64(*e.PickupUID), Valid: true}
	}
	if e.At.After(m.last) {
		m.last = e.At
	}
	if e.Kind == "removed" {
		m.closed = sql.NullTime{Time: m.last, Valid: true}
	}

	// Attribution is resolved once when trusted identity arrives, not on each
	// retry. A deleted domain's SET NULL must never rebind historical mail by name.
	resolveAuth := oldAuth == "" && m.auth != ""
	resolveUID := false
	if m.uid.Valid && !m.outbound.Valid && m.auth == "" {
		if !oldUID.Valid && m.sender != "" {
			resolveUID = true
		}
		if e.Kind == "qmgr" || !oldUID.Valid {
			queued, err := hasMessageStatus(ctx, tx, m.id, "queued")
			if err != nil {
				return err
			}
			resolveUID = resolveUID || (!oldUID.Valid && queued) || (e.Kind == "qmgr" && !queued)
		}
		// A pickup with a known envelope sender may precede qmgr. If its
		// metadata arrived late, the first sender record can still attribute it.
		if oldSender == "" && m.sender != "" && e.Kind == "qmgr" {
			queued, err := hasMessageStatus(ctx, tx, m.id, "queued")
			if err != nil {
				return err
			}
			resolveUID = resolveUID || !queued
		}
	}
	attributed := false
	if !m.outbound.Valid && (resolveAuth || resolveUID) {
		id, err := outboundDomain(ctx, tx, m, resolveAuth)
		if err != nil {
			return err
		}
		if id.Valid {
			m.outbound = id
			attributed = true
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE mail_delivery_messages SET sender=?,message_id=?,authenticated_sender=?,pickup_uid=?,outbound_domain_id=?,last_seen_at=?,closed_at=? WHERE id=?`, m.sender, m.messageID, m.auth, m.uid, m.outbound, m.last, m.closed, m.id); err != nil {
		return err
	}
	if attributed {
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO mail_delivery_visibility(delivery_id,domain_id) SELECT id,? FROM mail_deliveries WHERE message_fk=?`, m.outbound.Int64, m.id); err != nil {
			return err
		}
	}

	var deliveryID any
	if e.Recipient != "" {
		id, err := applyRecipient(ctx, tx, m, e)
		if err != nil {
			return err
		}
		deliveryID = id
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO mail_delivery_events(message_fk,delivery_id,event_key,at,status,relay,dsn,response,delay_seconds) VALUES(?,?,?,?,?,?,?,?,?)`, m.id, deliveryID, key[:], e.At, e.Status, e.Relay, e.DSN, e.Response, e.DelaySeconds)
	return err
}

func hasMessageStatus(ctx context.Context, tx *sql.Tx, id uint64, status string) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM mail_delivery_events WHERE message_fk=? AND status=?)`, id, status).Scan(&exists)
	return exists, err
}

func (c *Collector) message(ctx context.Context, tx *sql.Tx, e Event) (trackedMessage, error) {
	var m trackedMessage
	err := tx.QueryRowContext(ctx, `SELECT id,sender,message_id,authenticated_sender,pickup_uid,outbound_domain_id,first_seen_at,last_seen_at,closed_at FROM mail_delivery_messages WHERE source_id=? AND queue_id=? AND first_seen_at<=? ORDER BY first_seen_at DESC,id DESC LIMIT 1 FOR UPDATE`, c.config.SourceID, e.QueueID, e.At).Scan(&m.id, &m.sender, &m.messageID, &m.auth, &m.uid, &m.outbound, &m.first, &m.last, &m.closed)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return m, err
	}
	if errors.Is(err, sql.ErrNoRows) && e.Kind != "smtpd" && e.Kind != "pickup" {
		// A backward realtime-clock step can timestamp later records before the
		// stored acceptance. An open instance seen moments later is the same
		// message; an explicit new submission or an older closed one is not.
		fallback := tx.QueryRowContext(ctx, `SELECT id,sender,message_id,authenticated_sender,pickup_uid,outbound_domain_id,first_seen_at,last_seen_at,closed_at FROM mail_delivery_messages WHERE source_id=? AND queue_id=? AND closed_at IS NULL AND first_seen_at>? AND first_seen_at<=? ORDER BY first_seen_at,id LIMIT 1 FOR UPDATE`, c.config.SourceID, e.QueueID, e.At, e.At.Add(10*time.Minute))
		if scanErr := fallback.Scan(&m.id, &m.sender, &m.messageID, &m.auth, &m.uid, &m.outbound, &m.first, &m.last, &m.closed); scanErr == nil {
			if _, execErr := tx.ExecContext(ctx, `UPDATE mail_delivery_messages SET first_seen_at=? WHERE id=?`, e.At, m.id); execErr != nil {
				return m, execErr
			}
			m.first = e.At
			return m, nil
		} else if !errors.Is(scanErr, sql.ErrNoRows) {
			return m, scanErr
		}
	}
	fresh := errors.Is(err, sql.ErrNoRows) || (m.closed.Valid && e.At.After(m.closed.Time))
	if !fresh && (e.Kind == "smtpd" || e.Kind == "pickup") {
		// An actual new smtpd/pickup submission is a new instance. qmgr
		// reactivation is deliberately not a start marker.
		exists, err := hasMessageStatus(ctx, tx, m.id, "accepted")
		if err != nil {
			return m, err
		}
		fresh = exists
	}
	if !fresh && e.MessageID != "" && m.messageID != "" && e.MessageID != m.messageID {
		fresh = true
	}
	if !fresh {
		return m, nil
	}
	if m.id != 0 && !m.closed.Valid {
		if _, err := tx.ExecContext(ctx, `UPDATE mail_delivery_messages SET closed_at=GREATEST(last_seen_at,?) WHERE id=?`, e.At, m.id); err != nil {
			return m, err
		}
	}
	m = trackedMessage{first: e.At, last: e.At}
	// An unseen old record must not attach to a newer reused queue ID. Give
	// its historical instance an upper boundary at the next known instance.
	var next sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT MIN(first_seen_at) FROM mail_delivery_messages WHERE source_id=? AND queue_id=? AND first_seen_at>?`, c.config.SourceID, e.QueueID, e.At).Scan(&next); err != nil {
		return m, err
	}
	if next.Valid {
		m.closed = sql.NullTime{Time: next.Time.Add(-time.Microsecond), Valid: true}
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO mail_delivery_messages(source_id,queue_id,first_seen_at,last_seen_at,closed_at) VALUES(?,?,?,?,?)`, c.config.SourceID, e.QueueID, e.At, e.At, m.closed)
	if err != nil {
		return m, err
	}
	id, err := result.LastInsertId()
	m.id = uint64(id)
	return m, err
}

func outboundDomain(ctx context.Context, tx *sql.Tx, m trackedMessage, authenticated bool) (sql.NullInt64, error) {
	var id uint64
	var err error
	if authenticated {
		err = tx.QueryRowContext(ctx, `SELECT mb.domain_id FROM mailboxes mb JOIN mail_domains md ON md.id=mb.mail_domain_id AND md.domain_id=mb.domain_id WHERE mb.email=? AND mb.created_at<=? AND md.created_at<=? LIMIT 1`, m.auth, m.first, m.first).Scan(&id)
	} else {
		_, senderDomain, _ := strings.Cut(m.sender, "@")
		err = tx.QueryRowContext(ctx, `SELECT d.id FROM mail_domains md JOIN domains d ON d.sistem_kullanici=md.sistem_kullanici WHERE md.uid_n=? AND md.created_at<=? AND d.olusturulma<=? ORDER BY (d.alan_adi=?) DESC,(d.ana_domain_id IS NULL) DESC,d.id LIMIT 1`, m.uid.Int64, m.first, m.first, senderDomain).Scan(&id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullInt64{}, nil
	}
	return sql.NullInt64{Int64: int64(id), Valid: err == nil}, err
}

func applyRecipient(ctx context.Context, tx *sql.Tx, m trackedMessage, e Event) (uint64, error) {
	var id uint64
	var status, original string
	var last time.Time
	err := tx.QueryRowContext(ctx, `SELECT id,status,original_recipient,last_seen_at FROM mail_deliveries WHERE message_fk=? AND recipient=? FOR UPDATE`, m.id, e.Recipient).Scan(&id, &status, &original, &last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	created := errors.Is(err, sql.ErrNoRows)
	originalAdded := original == "" && e.OriginalRecipient != ""
	if originalAdded {
		original = e.OriginalRecipient
	}
	if created {
		result, err := tx.ExecContext(ctx, `INSERT INTO mail_deliveries(message_fk,recipient,original_recipient,status,first_seen_at,last_seen_at,relay,dsn,response) VALUES(?,?,?,?,?,?,?,?,?)`, m.id, e.Recipient, original, e.Status, e.At, e.At, e.Relay, e.DSN, e.Response)
		if err != nil {
			return 0, err
		}
		n, err := result.LastInsertId()
		if err != nil {
			return 0, err
		}
		id = uint64(n)
	} else if nextStatus(status, e.Status, last, e.At) {
		if _, err := tx.ExecContext(ctx, `UPDATE mail_deliveries SET original_recipient=?,status=?,first_seen_at=LEAST(first_seen_at,?),last_seen_at=?,relay=?,dsn=?,response=? WHERE id=?`, original, e.Status, e.At, e.At, e.Relay, e.DSN, e.Response, id); err != nil {
			return 0, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE mail_deliveries SET original_recipient=?,first_seen_at=LEAST(first_seen_at,?),last_seen_at=GREATEST(last_seen_at,?) WHERE id=?`, original, e.At, e.At, id); err != nil {
			return 0, err
		}
	}
	if created {
		if err := bindRecipientDomain(ctx, tx, id, e.Recipient, m.first); err != nil {
			return 0, err
		}
		if m.outbound.Valid {
			if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO mail_delivery_visibility(delivery_id,domain_id) VALUES(?,?)`, id, m.outbound.Int64); err != nil {
				return 0, err
			}
		}
	}
	if originalAdded {
		if err := bindRecipientDomain(ctx, tx, id, original, m.first); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func bindRecipientDomain(ctx context.Context, tx *sql.Tx, deliveryID uint64, recipient string, first time.Time) error {
	_, domain, _ := strings.Cut(recipient, "@")
	// Bind on first recipient/original-recipient discovery only. The creation
	// cutoff also prevents a recreated domain from acquiring old active mail.
	_, err := tx.ExecContext(ctx, `INSERT IGNORE INTO mail_delivery_visibility(delivery_id,domain_id) SELECT ?,domain_id FROM mail_domains WHERE alan_adi=? AND created_at<=?`, deliveryID, domain, first)
	return err
}
