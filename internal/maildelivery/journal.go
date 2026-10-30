package maildelivery

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var errJournalCursor = errors.New("journal cursor is no longer available")

type journalReader struct{ files []string }

type readPosition struct {
	cursor string
	lastAt time.Time
}

// read streams the oldest matching records first, stopping the child at a hard
// batch boundary. --no-tail is essential: -n would silently skip older entries.
func (j journalReader) read(ctx context.Context, p readPosition, since time.Time, limit int) ([]Record, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	args := []string{"--no-pager", "--all", "--no-tail", "--output=json", "--output-fields=__CURSOR,__REALTIME_TIMESTAMP,SYSLOG_IDENTIFIER,MESSAGE,_UID,_EXE"}
	for _, file := range j.files {
		args = append(args, "--file="+file)
	}
	if p.cursor != "" {
		args = append(args, "--cursor="+p.cursor)
	} else {
		args = append(args, "--since=@"+strconv.FormatInt(since.Unix(), 10))
	}
	for _, id := range []string{"postfix/smtpd", "postfix/submission/smtpd", "postfix/submissions/smtpd", "postfix/pickup", "postfix/cleanup", "postfix/qmgr", "postfix/smtp", "postfix/lmtp", "postfix/local", "postfix/virtual", "postfix/pipe", "postfix/error", "postfix/bounce", "postfix/postsuper"} {
		args = append(args, "SYSLOG_IDENTIFIER="+id)
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr cappedOutput
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 8192), maxJournalLine)
	records := make([]Record, 0, limit)
	var scanErr error
	full := false
	cursorFound := p.cursor == ""
	for scanner.Scan() {
		if string(scanner.Bytes()) == "-- No entries --" {
			continue
		}
		record, err := decodeJournalRecord(scanner.Bytes())
		if err != nil {
			scanErr = err
			break
		}
		if !cursorFound {
			if record.Cursor != p.cursor {
				scanErr = errJournalCursor
				break
			}
			cursorFound = true
			continue
		}
		records = append(records, record)
		if len(records) == limit {
			full = true
			break
		}
	}
	if scanErr == nil {
		scanErr = scanner.Err()
	}
	if full || scanErr != nil {
		cancel()
	}
	_ = stdout.Close()
	waitErr := cmd.Wait()
	if scanErr != nil {
		return nil, scanErr
	}
	if ctx.Err() != nil && !full {
		return nil, ctx.Err()
	}
	if waitErr != nil && !full {
		text := stderr.String()
		if p.cursor != "" && (strings.Contains(text, "seek to cursor") || strings.Contains(text, "seek cursor") || strings.Contains(text, "cursor: Invalid argument")) {
			return nil, errJournalCursor
		}
		return nil, fmt.Errorf("journalctl failed: %w: %s", waitErr, text)
	}
	text := strings.ToLower(stderr.String())
	if strings.Contains(text, "permission") || strings.Contains(text, "no journal files") {
		return nil, errors.New("journal source is inaccessible")
	}
	if !cursorFound {
		return nil, errJournalCursor
	}
	return records, nil
}

func journalText(raw json.RawMessage) string {
	var s string
	if len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

func decodeJournalRecord(line []byte) (Record, error) {
	var row struct {
		Cursor     string          `json:"__CURSOR"`
		Timestamp  string          `json:"__REALTIME_TIMESTAMP"`
		Identifier string          `json:"SYSLOG_IDENTIFIER"`
		Message    json.RawMessage `json:"MESSAGE"`
		UID        json.RawMessage `json:"_UID"`
		Exe        json.RawMessage `json:"_EXE"`
	}
	if err := json.Unmarshal(line, &row); err != nil {
		return Record{}, fmt.Errorf("invalid journal JSON: %w", err)
	}
	micros, err := strconv.ParseInt(row.Timestamp, 10, 64)
	if err != nil || micros <= 0 || row.Cursor == "" || len(row.Cursor) > 8192 || row.Identifier == "" {
		return Record{}, errors.New("journal entry has no valid cursor, identifier or timestamp")
	}
	var message string
	if len(row.Message) == 0 {
		return Record{}, errors.New("journal entry has no MESSAGE field")
	}
	switch row.Message[0] {
	case '"':
		if err := json.Unmarshal(row.Message, &message); err != nil {
			return Record{}, err
		}
	case '[':
		// Native binary MESSAGE values cannot be standard Postfix records.
		var binary []byte
		if err := json.Unmarshal(row.Message, &binary); err != nil {
			return Record{}, err
		}
	default:
		return Record{}, errors.New("journal entry has invalid MESSAGE field")
	}
	return Record{Cursor: row.Cursor, At: time.UnixMicro(micros).UTC(), Identifier: row.Identifier, Message: message, UID: journalText(row.UID), Exe: journalText(row.Exe)}, nil
}

type cappedOutput struct{ data []byte }

func (w *cappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if len(w.data) < 4096 {
		keep := min(4096-len(w.data), len(p))
		w.data = append(w.data, p[:keep]...)
	}
	return n, nil
}
func (w *cappedOutput) String() string { return strings.TrimSpace(string(w.data)) }
