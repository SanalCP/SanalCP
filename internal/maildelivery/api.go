package maildelivery

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"sanalcp/internal/httpx"
	"sanalcp/internal/middleware"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	DB *sql.DB
}

type Delivery struct {
	ID                uint64 `json:"id"`
	QueueID           string `json:"queue_id"`
	MessageID         string `json:"message_id"`
	Sender            string `json:"sender"`
	Recipient         string `json:"recipient"`
	OriginalRecipient string `json:"original_recipient"`
	Status            string `json:"status"`
	FirstSeenAt       string `json:"first_seen_at"`
	LastSeenAt        string `json:"last_seen_at"`
	Relay             string `json:"relay"`
	DSN               string `json:"dsn"`
	Response          string `json:"response"`
}

type DeliveryEvent struct {
	ID           uint64   `json:"id"`
	At           string   `json:"at"`
	Status       string   `json:"status"`
	Relay        string   `json:"relay"`
	DSN          string   `json:"dsn"`
	Response     string   `json:"response"`
	DelaySeconds *float64 `json:"delay_seconds,omitempty"`
}

type CollectorStatus struct {
	State           string  `json:"state"`
	Warning         string  `json:"warning"`
	LastCollectedAt *string `json:"last_collected_at"`
}

type deliveryListResponse struct {
	Items     []Delivery      `json:"items"`
	Total     uint64          `json:"total"`
	Page      int             `json:"page"`
	PageSize  int             `json:"page_size"`
	Collector CollectorStatus `json:"collector"`
}

type deliveryDetailResponse struct {
	Delivery        Delivery        `json:"delivery"`
	Events          []DeliveryEvent `json:"events"`
	EventsTruncated bool            `json:"events_truncated"`
	Collector       CollectorStatus `json:"collector"`
}

const deliveryEventLimit = 1000

// SQL formatting avoids depending on the driver's parseTime/location settings.
const deliveryColumns = `d.id, m.queue_id, m.message_id, m.sender, d.recipient,
	d.original_recipient, d.status,
	DATE_FORMAT(d.first_seen_at,'%Y-%m-%dT%H:%i:%s.%fZ'),
	DATE_FORMAT(d.last_seen_at,'%Y-%m-%dT%H:%i:%s.%fZ'),
	d.relay, d.dsn, d.response`

const deliveryTables = ` FROM mail_deliveries d JOIN mail_delivery_messages m ON m.id=d.message_fk`

type deliveryFilters struct {
	where    string
	args     []any
	page     int
	pageSize int
}

var deliveryLikeEscaper = strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")

func literalDeliveryPattern(value string) string {
	return "%" + deliveryLikeEscaper.Replace(value) + "%"
}

func deliveryDate(value string, endOfDay bool) (time.Time, error) {
	if len(value) == len("2006-01-02") {
		t, err := time.Parse("2006-01-02", value)
		if err != nil || t.Year() < 1000 {
			return time.Time{}, errors.New("geçersiz tarih")
		}
		if endOfDay {
			t = t.Add(24*time.Hour - time.Microsecond)
		}
		return t, nil
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, errors.New("geçersiz tarih")
	}
	_, offset := t.Zone()
	if offset != 0 || t.Year() < 1000 || t.Year() > 9999 {
		return time.Time{}, errors.New("tarih UTC olmalıdır")
	}
	return t.UTC(), nil
}

func parseDeliveryFilters(q url.Values) (deliveryFilters, error) {
	f := deliveryFilters{page: 1, pageSize: 25}
	values := make(map[string]string, 8)
	for _, field := range []struct {
		name string
		max  int
	}{{"sender", 320}, {"recipient", 320}, {"queue_id", 64}, {"status", 16}, {"from", 40}, {"to", 40}, {"page", 7}, {"page_size", 3}} {
		v := q[field.name]
		if len(v) > 1 || (len(v) == 1 && (len(v[0]) > field.max || !utf8.ValidString(v[0]) || strings.ContainsRune(v[0], '\x00'))) {
			return f, errors.New("geçersiz filtre")
		}
		values[field.name] = strings.TrimSpace(q.Get(field.name))
	}
	for _, field := range []struct {
		name string
		max  int
		dest *int
	}{{"page", 1000000, &f.page}, {"page_size", 100, &f.pageSize}} {
		if values[field.name] == "" {
			if _, supplied := q[field.name]; supplied {
				return f, errors.New("geçersiz sayfalama")
			}
			continue
		}
		for _, c := range values[field.name] {
			if c < '0' || c > '9' {
				return f, errors.New("geçersiz sayfalama")
			}
		}
		n, err := strconv.Atoi(values[field.name])
		if err != nil || n < 1 || n > field.max {
			return f, errors.New("geçersiz sayfalama")
		}
		*field.dest = n
	}
	for _, field := range []struct{ name, column string }{{"sender", "m.sender"}, {"recipient", "d.recipient"}} {
		if v := values[field.name]; v != "" {
			f.where += " AND LOWER(" + field.column + ") LIKE LOWER(?) ESCAPE '!'"
			f.args = append(f.args, literalDeliveryPattern(v))
		}
	}
	if v := values["queue_id"]; v != "" {
		f.where += " AND BINARY m.queue_id=BINARY ?"
		f.args = append(f.args, v)
	}
	if v := values["status"]; v != "" {
		switch v {
		case "pending", "deferred", "delivered", "bounced", "forwarded":
		default:
			return f, errors.New("geçersiz teslimat durumu")
		}
		f.where += " AND d.status=?"
		f.args = append(f.args, v)
	}
	var from, to time.Time
	for _, field := range []struct {
		name, operator string
		dest           *time.Time
	}{{"from", ">=", &from}, {"to", "<=", &to}} {
		if values[field.name] == "" {
			continue
		}
		t, err := deliveryDate(values[field.name], field.name == "to")
		if err != nil {
			return f, err
		}
		*field.dest = t
		// DATETIME(6) cannot represent nanoseconds. Round a lower bound up,
		// not down, so an earlier attempt never slips into the requested range.
		if field.name == "from" && t.Nanosecond()%1000 != 0 {
			t = t.Truncate(time.Microsecond).Add(time.Microsecond)
			if t.Year() > 9999 {
				return f, errors.New("geçersiz tarih")
			}
		}
		f.where += " AND d.last_seen_at" + field.operator + "?"
		f.args = append(f.args, t.Format("2006-01-02 15:04:05.000000"))
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return f, errors.New("tarih aralığı ters olamaz")
	}
	return f, nil
}

// Visibility is immutable attribution, while ownership is resolved NOW. In
// particular, the untrusted envelope sender never grants access to a delivery.
func deliveryScope(r *http.Request, domainScoped bool) (string, []any, int) {
	claims := middleware.ClaimsFrom(r)
	if claims == nil {
		return "", nil, http.StatusUnauthorized
	}
	if claims.Role != middleware.RolAdmin && claims.Role != middleware.RolBayi && !(domainScoped && claims.Role == middleware.RolMusteri) {
		return "", nil, http.StatusForbidden
	}
	if claims.Role != middleware.RolAdmin && claims.UserID <= 0 {
		return "", nil, http.StatusForbidden
	}
	if !domainScoped && claims.Role == middleware.RolAdmin {
		return "1=1", nil, 0
	}
	where := `EXISTS (SELECT 1 FROM mail_delivery_visibility v JOIN domains vd ON vd.id=v.domain_id`
	if claims.Role != middleware.RolAdmin {
		where += ` JOIN customers vc ON vc.id=vd.customer_id`
	}
	where += ` WHERE v.delivery_id=d.id`
	var args []any
	if domainScoped {
		id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
		if err != nil || id == 0 {
			return "", nil, http.StatusBadRequest
		}
		where += ` AND vd.id=?`
		args = append(args, id)
	}
	switch claims.Role {
	case middleware.RolBayi:
		where += ` AND vc.owner_user_id=?`
		args = append(args, claims.UserID)
	case middleware.RolMusteri:
		where += ` AND vc.user_id=?`
		args = append(args, claims.UserID)
	}
	return where + ")", args, 0
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, false)
}

func (h *Handlers) DomainList(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, true)
}

func (h *Handlers) Detail(w http.ResponseWriter, r *http.Request) {
	h.detail(w, r, false)
}

func (h *Handlers) DomainDetail(w http.ResponseWriter, r *http.Request) {
	h.detail(w, r, true)
}

func deliveryReadError(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusServiceUnavailable, "teslimat geçmişi okunamadı")
}

func (h *Handlers) readTransaction(ctx context.Context) (*sql.Tx, error) {
	if h.DB == nil {
		return nil, errors.New("database unavailable")
	}
	// Count, rows, ownership and collector state come from one snapshot.
	return h.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request, domainScoped bool) {
	scope, args, status := deliveryScope(r, domainScoped)
	if status != 0 {
		httpx.WriteError(w, status, "teslimat geçmişine erişim yok")
		return
	}
	filters, err := parseDeliveryFilters(r.URL.Query())
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	where := " WHERE " + scope + filters.where
	args = append(args, filters.args...)
	tx, err := h.readTransaction(r.Context())
	if err != nil {
		deliveryReadError(w)
		return
	}
	defer tx.Rollback()
	result := deliveryListResponse{Items: []Delivery{}, Page: filters.page, PageSize: filters.pageSize}
	if err := tx.QueryRowContext(r.Context(), "SELECT COUNT(*)"+deliveryTables+where, args...).Scan(&result.Total); err != nil {
		deliveryReadError(w)
		return
	}
	listArgs := append(args, filters.pageSize, (filters.page-1)*filters.pageSize)
	rows, err := tx.QueryContext(r.Context(), "SELECT "+deliveryColumns+deliveryTables+where+" ORDER BY d.last_seen_at DESC,d.id DESC LIMIT ? OFFSET ?", listArgs...)
	if err != nil {
		deliveryReadError(w)
		return
	}
	for rows.Next() {
		var item Delivery
		if err := scanDelivery(rows, &item); err != nil {
			rows.Close()
			deliveryReadError(w)
			return
		}
		result.Items = append(result.Items, item)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil || closeErr != nil {
		deliveryReadError(w)
		return
	}
	result.Collector, err = readCollectorStatus(r.Context(), tx)
	if err != nil {
		deliveryReadError(w)
		return
	}
	if err := tx.Commit(); err != nil {
		deliveryReadError(w)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

type deliveryScanner interface {
	Scan(...any) error
}

func scanDelivery(row deliveryScanner, d *Delivery, extra ...any) error {
	fields := []any{&d.ID, &d.QueueID, &d.MessageID, &d.Sender, &d.Recipient,
		&d.OriginalRecipient, &d.Status, &d.FirstSeenAt, &d.LastSeenAt, &d.Relay, &d.DSN, &d.Response}
	return row.Scan(append(fields, extra...)...)
}

func readCollectorStatus(ctx context.Context, tx *sql.Tx) (CollectorStatus, error) {
	result := CollectorStatus{State: "starting"}
	var collected sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT state, warning, DATE_FORMAT(collected_at,'%Y-%m-%dT%H:%i:%s.%fZ')
		FROM mail_delivery_sources WHERE id='journal:local'`).Scan(&result.State, &result.Warning, &collected)
	if errors.Is(err, sql.ErrNoRows) {
		return CollectorStatus{State: "starting"}, nil
	}
	if err != nil {
		return CollectorStatus{}, err
	}
	if collected.Valid {
		result.LastCollectedAt = &collected.String
	}
	switch result.State {
	case "starting", "ok", "error":
	default:
		result.State = "error"
	}
	switch result.Warning {
	case "", "history_gap", "source_unavailable":
	default:
		result.Warning = "source_unavailable"
	}
	return result, nil
}

func (h *Handlers) detail(w http.ResponseWriter, r *http.Request, domainScoped bool) {
	scope, args, status := deliveryScope(r, domainScoped)
	if status != 0 {
		httpx.WriteError(w, status, "teslimat geçmişine erişim yok")
		return
	}
	id, err := strconv.ParseUint(chi.URLParam(r, "deliveryID"), 10, 64)
	if err != nil || id == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz teslimat kimliği")
		return
	}
	tx, err := h.readTransaction(r.Context())
	if err != nil {
		deliveryReadError(w)
		return
	}
	defer tx.Rollback()
	result := deliveryDetailResponse{Events: []DeliveryEvent{}}
	var messageID uint64
	args = append(args, id)
	err = scanDelivery(tx.QueryRowContext(r.Context(), "SELECT "+deliveryColumns+", d.message_fk"+deliveryTables+" WHERE "+scope+" AND d.id=?", args...), &result.Delivery, &messageID)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "teslimat bulunamadı")
		return
	}
	if err != nil {
		deliveryReadError(w)
		return
	}
	// Only this recipient's attempts and safe message-level milestones. Metadata
	// response text can mention another recipient, so never expose that text.
	rows, err := tx.QueryContext(r.Context(), `SELECT id, DATE_FORMAT(at,'%Y-%m-%dT%H:%i:%s.%fZ'), status,
		CASE WHEN delivery_id IS NULL THEN '' ELSE relay END,
		CASE WHEN delivery_id IS NULL THEN '' ELSE dsn END,
		CASE WHEN delivery_id IS NULL THEN '' ELSE response END,
		CASE WHEN delivery_id IS NULL THEN NULL ELSE delay_seconds END
		FROM mail_delivery_events WHERE message_fk=? AND
		(delivery_id=? OR (delivery_id IS NULL AND status IN ('accepted','queued','removed')))
		ORDER BY at DESC,id DESC LIMIT ?`, messageID, id, deliveryEventLimit+1)
	if err != nil {
		deliveryReadError(w)
		return
	}
	for rows.Next() {
		var event DeliveryEvent
		var delay sql.NullFloat64
		if err := rows.Scan(&event.ID, &event.At, &event.Status, &event.Relay, &event.DSN, &event.Response, &delay); err != nil {
			rows.Close()
			deliveryReadError(w)
			return
		}
		if len(result.Events) == deliveryEventLimit {
			result.EventsTruncated = true
			continue
		}
		if delay.Valid {
			event.DelaySeconds = &delay.Float64
		}
		result.Events = append(result.Events, event)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil || closeErr != nil {
		deliveryReadError(w)
		return
	}
	for i, j := 0, len(result.Events)-1; i < j; i, j = i+1, j-1 {
		result.Events[i], result.Events[j] = result.Events[j], result.Events[i]
	}
	result.Collector, err = readCollectorStatus(r.Context(), tx)
	if err != nil {
		deliveryReadError(w)
		return
	}
	if err := tx.Commit(); err != nil {
		deliveryReadError(w)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}
