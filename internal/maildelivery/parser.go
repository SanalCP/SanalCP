package maildelivery

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const maxJournalLine = 256 * 1024

// Record is a native journal entry. Raw journal messages are never persisted.
type Record struct {
	Cursor     string
	At         time.Time
	Identifier string
	Message    string
	// UID and Exe are journald-trusted (_-prefixed) producer fields; the
	// SYSLOG_IDENTIFIER tag is writable by any local process.
	UID string
	Exe string
}

// Event contains only the metadata needed for recipient tracking.
type Event struct {
	QueueID             string
	Kind                string
	Status              string
	Sender              string
	MessageID           string
	AuthenticatedSender string
	PickupUID           *uint64
	Recipient           string
	OriginalRecipient   string
	Relay               string
	DSN                 string
	Response            string
	DelaySeconds        *float64
	At                  time.Time
}

func postfixIdentifier(s string) string {
	switch s {
	case "postfix/smtpd", "postfix/submission/smtpd", "postfix/submissions/smtpd":
		return "smtpd"
	case "postfix/pickup":
		return "pickup"
	case "postfix/cleanup":
		return "cleanup"
	case "postfix/qmgr":
		return "qmgr"
	case "postfix/smtp", "postfix/lmtp", "postfix/local", "postfix/virtual", "postfix/pipe", "postfix/error":
		return "delivery"
	case "postfix/bounce", "postfix/postsuper":
		return "other"
	}
	return ""
}

// Parse rejects unrelated or malformed records before any database work.
func Parse(r Record) (Event, bool) {
	kind := postfixIdentifier(r.Identifier)
	if kind == "" || len(r.Message) > maxJournalLine || r.At.IsZero() {
		return Event{}, false
	}
	qid, text, ok := strings.Cut(r.Message, ": ")
	if !ok || !queueID(qid) {
		return Event{}, false
	}
	e := Event{QueueID: qid, Kind: kind, At: r.At.UTC(), Status: "accepted"}
	switch kind {
	case "smtpd":
		if !strings.HasPrefix(text, "client=") {
			return Event{}, false
		}
		e.AuthenticatedSender = address(field(text, "sasl_username"))
	case "pickup":
		v, err := strconv.ParseUint(field(text, "uid"), 10, 63)
		if err != nil {
			return Event{}, false
		}
		e.PickupUID = &v
		e.Sender = address(field(text, "from"))
	case "cleanup":
		if text == "removed (canceled)" {
			// Client aborted mid-DATA: the queue file is discarded without any
			// qmgr record. Without closing it the instance would never expire.
			e.Kind, e.Status = "removed", "removed"
			break
		}
		if !strings.HasPrefix(text, "message-id=") {
			return Event{}, false
		}
		e.MessageID = bounded(strings.TrimPrefix(text, "message-id="), 998)
		e.Status = "pending"
	case "qmgr":
		if text == "removed" {
			e.Kind, e.Status = "removed", "removed"
		} else if strings.HasPrefix(text, "from=<") && strings.Contains(text, "(queue active)") {
			e.Sender, e.Status = address(field(text, "from")), "queued"
		} else {
			return Event{}, false
		}
	case "delivery":
		e.Recipient = address(field(text, "to"))
		if e.Recipient == "" {
			return Event{}, false
		}
		e.OriginalRecipient = address(field(text, "orig_to"))
		e.Relay = bounded(field(text, "relay"), 512)
		e.DSN = bounded(field(text, "dsn"), 32)
		if v, err := strconv.ParseFloat(field(text, "delay"), 64); err == nil && v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) {
			e.DelaySeconds = &v
		}
		statusText := fieldRest(text, "status")
		status, response, _ := strings.Cut(statusText, " ")
		switch status {
		case "sent":
			e.Status = "delivered"
			if strings.HasPrefix(strings.TrimPrefix(response, "("), "forwarded as ") {
				e.Status = "forwarded"
			}
		case "deferred", "bounced":
			e.Status = status
		default:
			return Event{}, false
		}
		e.Response = bounded(strings.TrimSuffix(strings.TrimPrefix(response, "("), ")"), 4096)
	case "other":
		// Bounce notifications refer to a different message and are not a
		// recipient outcome. An administrative postsuper removal only closes
		// the queue instance; it invents no delivery result.
		if r.Identifier == "postfix/postsuper" && text == "removed" {
			e.Kind, e.Status = "removed", "removed"
			break
		}
		return Event{}, false
	default:
		return Event{}, false
	}
	return e, true
}

func queueID(s string) bool {
	if len(s) < 5 || len(s) > 64 || s == "NOQUEUE" {
		return false
	}
	for i := range s {
		if c := s[i]; !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func fieldRest(s, key string) string {
	prefix := key + "="
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	if i := strings.Index(s, ", "+prefix); i >= 0 {
		return s[i+len(prefix)+2:]
	}
	// pickup uses a space rather than a comma between uid and from.
	if key == "from" {
		if i := strings.Index(s, " "+prefix); i >= 0 {
			return s[i+len(prefix)+1:]
		}
	}
	return ""
}

func field(s, key string) string {
	v := fieldRest(s, key)
	if strings.HasPrefix(v, "<") {
		if end := strings.IndexByte(v, '>'); end >= 0 {
			return v[1:end]
		}
	}
	if i := strings.IndexAny(v, ", "); i >= 0 {
		v = v[:i]
	}
	return v
}

func address(s string) string {
	s = strings.TrimSpace(strings.Trim(s, "<>"))
	if len(s) == 0 || len(s) > 320 || strings.Count(s, "@") != 1 || strings.ContainsAny(s, "\r\n\t <>\x00") || !utf8.ValidString(s) {
		return ""
	}
	local, domain, _ := strings.Cut(s, "@")
	if local == "" || domain == "" {
		return ""
	}
	return strings.ToLower(s)
}

func bounded(s string, n int) string {
	if len(s) > n {
		s = s[:n]
		for !utf8.ValidString(s) && len(s) > 0 {
			s = s[:len(s)-1]
		}
	}
	if strings.IndexFunc(s, func(r rune) bool { return r < 32 || r == 127 || r == utf8.RuneError }) >= 0 {
		s = strings.Map(func(r rune) rune {
			if r < 32 || r == 127 || r == utf8.RuneError {
				return ' '
			}
			return r
		}, s)
	}
	return s
}

func nextStatus(current, incoming string, currentAt, incomingAt time.Time) bool {
	if incomingAt.Before(currentAt) {
		return false
	}
	// A retry cannot undo a completed delivery even when logs arrive late.
	if (current == "delivered" || current == "bounced" || current == "forwarded") && incoming != current {
		return false
	}
	return true
}
