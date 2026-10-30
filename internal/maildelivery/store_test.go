package maildelivery

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func deliveryStoreMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *Collector) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	return db, mock, NewCollector(db, Config{})
}

func messageRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "sender", "message_id", "authenticated_sender", "pickup_uid", "outbound_domain_id", "first_seen_at", "last_seen_at", "closed_at"})
}

func TestReplayDedupPrecedesReusedQueueResolution(t *testing.T) {
	_, mock, collector := deliveryStoreMock(t)
	now := time.Date(2026, 10, 29, 1, 0, 0, 0, time.UTC)
	record := Record{Cursor: "old-entry", At: now.Add(-time.Hour), Identifier: "postfix/qmgr", UID: "0", Exe: "/usr/lib/postfix/sbin/qmgr", Message: "ABC123: from=<old@example.test>, size=100, nrcpt=1 (queue active)"}
	key := eventKey("journal:local", record.Cursor)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .cursor.,last_event_at,warning FROM mail_delivery_sources").WillReturnRows(sqlmock.NewRows([]string{"cursor", "last_event_at", "warning"}).AddRow("vacuumed-cursor", now, "history_gap"))
	mock.ExpectQuery("SELECT EXISTS.*mail_delivery_events").WithArgs(key[:]).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("UPDATE mail_delivery_sources").WithArgs("old-entry", now, now, "history_gap", "journal:local").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := collector.commit(context.Background(), readPosition{cursor: "vacuumed-cursor"}, []Record{record}, now, true); err != nil {
		t.Fatal(err)
	}
	// No message/recipient mutation is expected: a reused current ABC123 is
	// untouched even though the recovery source overlaps this older entry.
}

func TestIngestionFailureDoesNotAdvanceCursor(t *testing.T) {
	_, mock, collector := deliveryStoreMock(t)
	now := time.Now().UTC()
	record := Record{Cursor: "uncommitted", At: now, Identifier: "postfix/qmgr", UID: "0", Exe: "/usr/lib/postfix/sbin/qmgr", Message: "ABC123: removed"}
	key := eventKey("journal:local", record.Cursor)
	failure := errors.New("database unavailable during ingestion")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .cursor.,last_event_at,warning FROM mail_delivery_sources").WillReturnRows(sqlmock.NewRows([]string{"cursor", "last_event_at", "warning"}).AddRow("previous", now, ""))
	mock.ExpectQuery("SELECT EXISTS.*mail_delivery_events").WithArgs(key[:]).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("SELECT .* FROM mail_delivery_messages").WillReturnError(failure)
	mock.ExpectRollback()
	if err := collector.commit(context.Background(), readPosition{cursor: "previous"}, []Record{record}, now, false); !errors.Is(err, failure) {
		t.Fatalf("expected ingestion failure, got %v", err)
	}
}

func TestConcurrentCollectorCannotOverwriteNewerCursor(t *testing.T) {
	_, mock, collector := deliveryStoreMock(t)
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .cursor.,last_event_at,warning FROM mail_delivery_sources").WillReturnRows(sqlmock.NewRows([]string{"cursor", "last_event_at", "warning"}).AddRow("another-process-advanced", now, ""))
	mock.ExpectRollback()
	if err := collector.commit(context.Background(), readPosition{cursor: "stale"}, nil, now, false); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryGapRemainsVisibleAfterEmptySuccessfulTick(t *testing.T) {
	_, mock, collector := deliveryStoreMock(t)
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .cursor.,last_event_at,warning FROM mail_delivery_sources").WillReturnRows(sqlmock.NewRows([]string{"cursor", "last_event_at", "warning"}).AddRow("current", now, "history_gap"))
	mock.ExpectExec("UPDATE mail_delivery_sources").WithArgs("current", now, now, "history_gap", "journal:local").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := collector.commit(context.Background(), readPosition{cursor: "current"}, nil, now, false); err != nil {
		t.Fatal(err)
	}
}

func TestRemovedQueueIDCreatesSeparateInstance(t *testing.T) {
	db, mock, collector := deliveryStoreMock(t)
	first := time.Date(2026, 10, 29, 1, 0, 0, 0, time.UTC)
	closed := first.Add(time.Minute)
	next := closed.Add(time.Minute)
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM mail_delivery_messages").WillReturnRows(messageRows().AddRow(10, "old@example.test", "<old>", "old@example.test", nil, 1, first, closed, closed))
	mock.ExpectQuery("SELECT MIN.*mail_delivery_messages").WillReturnRows(sqlmock.NewRows([]string{"first_seen_at"}).AddRow(nil))
	mock.ExpectExec("INSERT INTO mail_delivery_messages").WithArgs("journal:local", "ABC123", next, next, nil).WillReturnResult(sqlmock.NewResult(11, 1))
	mock.ExpectRollback()
	message, err := collector.message(context.Background(), tx, Event{QueueID: "ABC123", Kind: "qmgr", At: next})
	if err != nil {
		t.Fatal(err)
	}
	if message.id != 11 || message.auth != "" || message.sender != "" || message.outbound.Valid {
		t.Fatalf("queue instances merged: %#v", message)
	}
	_ = tx.Rollback()
}

func TestDeferredQmgrReactivationKeepsInstance(t *testing.T) {
	db, mock, collector := deliveryStoreMock(t)
	first := time.Now().UTC()
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM mail_delivery_messages").WillReturnRows(messageRows().AddRow(10, "old@example.test", "<same>", "old@example.test", nil, 1, first, first, nil))
	mock.ExpectRollback()
	message, err := collector.message(context.Background(), tx, Event{QueueID: "ABC123", Kind: "qmgr", At: first.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if message.id != 10 {
		t.Fatalf("qmgr retry opened a second message: %#v", message)
	}
	_ = tx.Rollback()
}

func TestFreshSubmissionSeparatesUnremovedQueueID(t *testing.T) {
	db, mock, collector := deliveryStoreMock(t)
	first := time.Now().UTC()
	next := first.Add(time.Hour)
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM mail_delivery_messages").WillReturnRows(messageRows().AddRow(10, "old@example.test", "<old>", "old@example.test", nil, 1, first, first, nil))
	mock.ExpectQuery("SELECT EXISTS.*mail_delivery_events").WithArgs(uint64(10), "accepted").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("UPDATE mail_delivery_messages").WithArgs(next, uint64(10)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT MIN.*mail_delivery_messages").WillReturnRows(sqlmock.NewRows([]string{"first_seen_at"}).AddRow(nil))
	mock.ExpectExec("INSERT INTO mail_delivery_messages").WillReturnResult(sqlmock.NewResult(11, 1))
	mock.ExpectRollback()
	message, err := collector.message(context.Background(), tx, Event{QueueID: "ABC123", Kind: "smtpd", At: next})
	if err != nil {
		t.Fatal(err)
	}
	if message.id != 11 {
		t.Fatalf("fresh submission merged into prior instance: %#v", message)
	}
	_ = tx.Rollback()
}

func TestSpoofedEnvelopeSenderDoesNotGrantVisibility(t *testing.T) {
	db, mock, collector := deliveryStoreMock(t)
	first := time.Now().UTC()
	at := first.Add(time.Second)
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM mail_delivery_messages").WillReturnRows(messageRows().AddRow(10, "", "<same>", "", nil, nil, first, first, nil))
	mock.ExpectExec("UPDATE mail_delivery_messages").WithArgs("victim@other-tenant.test", "<same>", "", nil, nil, at, nil, uint64(10)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO mail_delivery_events").WillReturnResult(sqlmock.NewResult(20, 1))
	mock.ExpectRollback()
	if err := collector.apply(context.Background(), tx, Event{QueueID: "ABC123", Kind: "qmgr", Sender: "victim@other-tenant.test", Status: "queued", At: at}, eventKey("journal:local", "spoofed")); err != nil {
		t.Fatal(err)
	}
	// No mailbox/domain lookup or visibility insert may be attempted from From.
	_ = tx.Rollback()
}

func TestLateAuthenticationBindsExistingRecipientsToVerifiedDomain(t *testing.T) {
	db, mock, collector := deliveryStoreMock(t)
	first := time.Now().UTC()
	at := first.Add(time.Second)
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM mail_delivery_messages").WillReturnRows(messageRows().AddRow(10, "victim@other.test", "<same>", "", nil, nil, first, first, nil))
	mock.ExpectQuery("SELECT EXISTS.*mail_delivery_events").WithArgs(uint64(10), "accepted").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("SELECT mb.domain_id FROM mailboxes").WithArgs("actual@owner.test", first, first).WillReturnRows(sqlmock.NewRows([]string{"domain_id"}).AddRow(42))
	mock.ExpectExec("UPDATE mail_delivery_messages").WithArgs("victim@other.test", "<same>", "actual@owner.test", nil, int64(42), at, nil, uint64(10)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT IGNORE INTO mail_delivery_visibility").WithArgs(int64(42), uint64(10)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO mail_delivery_events").WillReturnResult(sqlmock.NewResult(20, 1))
	mock.ExpectRollback()
	if err := collector.apply(context.Background(), tx, Event{QueueID: "ABC123", Kind: "smtpd", AuthenticatedSender: "actual@owner.test", Status: "accepted", At: at}, eventKey("journal:local", "late-auth")); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
}

func TestDeletedOutboundDomainIsNotReboundOnRetry(t *testing.T) {
	db, mock, collector := deliveryStoreMock(t)
	first := time.Now().UTC()
	at := first.Add(time.Hour)
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM mail_delivery_messages").WillReturnRows(messageRows().AddRow(10, "sender@owner.test", "<same>", "sender@owner.test", nil, nil, first, first, nil))
	mock.ExpectExec("UPDATE mail_delivery_messages").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO mail_delivery_events").WillReturnResult(sqlmock.NewResult(20, 1))
	mock.ExpectRollback()
	if err := collector.apply(context.Background(), tx, Event{QueueID: "ABC123", Kind: "qmgr", Sender: "sender@owner.test", Status: "queued", At: at}, eventKey("journal:local", "retry-after-domain-delete")); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
}

func TestRecipientRetryUpdatesOnlyItsOwnOutcome(t *testing.T) {
	db, mock, _ := deliveryStoreMock(t)
	first := time.Now().UTC()
	at := first.Add(time.Hour)
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM mail_deliveries").WithArgs(uint64(10), "one@remote.test").WillReturnRows(sqlmock.NewRows([]string{"id", "status", "original_recipient", "last_seen_at"}).AddRow(101, "deferred", "", first))
	mock.ExpectExec("UPDATE mail_deliveries").WithArgs("", "delivered", at, at, "mx.remote.test", "2.0.0", "250 queued", uint64(101)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()
	id, err := applyRecipient(context.Background(), tx, trackedMessage{id: 10, first: first}, Event{Recipient: "one@remote.test", Status: "delivered", At: at, Relay: "mx.remote.test", DSN: "2.0.0", Response: "250 queued"})
	if err != nil || id != 101 {
		t.Fatalf("retry lost recipient identity: %d, %v", id, err)
	}
	_ = tx.Rollback()
}

func TestCrossDomainRecipientsGetSeparateVisibilityBindings(t *testing.T) {
	db, mock, _ := deliveryStoreMock(t)
	first := time.Now().UTC()
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i, recipient := range []string{"one@tenant-a.test", "two@tenant-b.test"} {
		id := uint64(101 + i)
		domain := []string{"tenant-a.test", "tenant-b.test"}[i]
		mock.ExpectQuery("SELECT .* FROM mail_deliveries").WithArgs(uint64(10), recipient).WillReturnRows(sqlmock.NewRows([]string{"id", "status", "original_recipient", "last_seen_at"}))
		mock.ExpectExec("INSERT INTO mail_deliveries").WithArgs(uint64(10), recipient, "", "delivered", first, first, "", "", "250 queued").WillReturnResult(sqlmock.NewResult(int64(id), 1))
		mock.ExpectExec("INSERT IGNORE INTO mail_delivery_visibility").WithArgs(id, domain, first).WillReturnResult(sqlmock.NewResult(0, 1))
		if got, err := applyRecipient(context.Background(), tx, trackedMessage{id: 10, first: first}, Event{Recipient: recipient, Status: "delivered", At: first, Response: "250 queued"}); err != nil || got != id {
			t.Fatalf("recipient binding: %d, %v", got, err)
		}
	}
	mock.ExpectRollback()
	_ = tx.Rollback()
}

func TestLateOriginalRecipientAddsOnlyAliasOwnersVisibility(t *testing.T) {
	db, mock, _ := deliveryStoreMock(t)
	first := time.Now().UTC()
	at := first.Add(time.Second)
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM mail_deliveries").WithArgs(uint64(10), "external@remote.test").WillReturnRows(sqlmock.NewRows([]string{"id", "status", "original_recipient", "last_seen_at"}).AddRow(101, "delivered", "", at))
	mock.ExpectExec("UPDATE mail_deliveries").WithArgs("alias@tenant-a.test", first, first, uint64(101)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT IGNORE INTO mail_delivery_visibility").WithArgs(uint64(101), "tenant-a.test", first).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()
	if _, err := applyRecipient(context.Background(), tx, trackedMessage{id: 10, first: first}, Event{Recipient: "external@remote.test", OriginalRecipient: "alias@tenant-a.test", Status: "deferred", At: first}); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
}
