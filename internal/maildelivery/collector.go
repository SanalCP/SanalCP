package maildelivery

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"sync"
	"time"
)

// Config controls a trusted local collector. JournalFiles is only for native
// journal fixtures/offline recovery; HTTP handlers never accept these paths.
type Config struct {
	SourceID     string
	JournalFiles []string
	BatchSize    int
	Now          func() time.Time
}

type Collector struct {
	db        *sql.DB
	config    Config
	journal   journalReader
	mu        sync.Mutex
	lastBatch int
}

func NewCollector(db *sql.DB, config Config) *Collector {
	if config.SourceID == "" {
		config.SourceID = "journal:local"
	}
	if len(config.SourceID) > 64 {
		panic("maildelivery source ID exceeds 64 bytes")
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 500
	}
	if config.BatchSize > 2000 {
		config.BatchSize = 2000
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	files := append([]string(nil), config.JournalFiles...)
	return &Collector{db: db, config: config, journal: journalReader{files: files}}
}

// Collect reads and atomically commits one bounded batch with its source cursor.
// It does not start the application, create users, or change Postfix settings.
func (c *Collector) Collect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastBatch = 0
	now := c.config.Now().UTC()
	if _, err := c.db.ExecContext(ctx, `INSERT IGNORE INTO mail_delivery_sources(id) VALUES(?)`, c.config.SourceID); err != nil {
		return err
	}
	var cursor sql.NullString
	var lastAt sql.NullTime
	if err := c.db.QueryRowContext(ctx, "SELECT `cursor`,last_event_at FROM mail_delivery_sources WHERE id=?", c.config.SourceID).Scan(&cursor, &lastAt); err != nil {
		return err
	}
	position := readPosition{cursor: cursor.String, lastAt: lastAt.Time}
	since := now.Add(-7 * 24 * time.Hour)
	records, err := c.journal.read(ctx, position, since, c.config.BatchSize)
	gap := false
	if errors.Is(err, errJournalCursor) {
		gap = true
		since = recoveryStart(now, position.lastAt)
		records, err = c.journal.read(ctx, readPosition{}, since, c.config.BatchSize)
	}
	if err == nil {
		err = c.commit(ctx, position, records, now, gap)
	}
	if err != nil {
		if ctx.Err() == nil {
			_, stateErr := c.db.ExecContext(ctx, `UPDATE mail_delivery_sources SET state='error',warning=CASE WHEN warning='history_gap' THEN warning ELSE 'source_unavailable' END WHERE id=?`, c.config.SourceID)
			if stateErr != nil {
				return errors.Join(err, stateErr)
			}
		}
		return err
	}
	c.lastBatch = len(records)
	// Active queue instances are retained even when their oldest attempt is
	// over 30 days old. Otherwise a retry could silently become a new message.
	if _, err := c.db.ExecContext(ctx, `DELETE FROM mail_delivery_messages WHERE closed_at IS NOT NULL AND last_seen_at<? ORDER BY last_seen_at,id LIMIT 200`, now.Add(-30*24*time.Hour)); err != nil {
		return err
	}
	return nil
}

func recoveryStart(now, lastAt time.Time) time.Time {
	start := lastAt.Add(-2 * time.Minute)
	floor := now.Add(-30 * 24 * time.Hour)
	if lastAt.IsZero() || start.Before(floor) {
		return floor
	}
	if start.After(now) {
		return now.Add(-2 * time.Minute)
	}
	return start
}

// Run is the production systemd-journal collector. Cancellation terminates any
// in-flight journalctl child and rolls back uncommitted records.
func Run(ctx context.Context, db *sql.DB) {
	collector := NewCollector(db, Config{})
	for {
		if ctx.Err() != nil {
			return
		}
		err := collector.Collect(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("mail delivery collector: %v", err)
		}
		delay := 5 * time.Second
		if err == nil && collector.lastBatch == collector.config.BatchSize {
			delay = 10 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
