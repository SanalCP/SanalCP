package maildelivery

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestPostfixRecipientOutcomes(t *testing.T) {
	at := time.Date(2026, 10, 29, 1, 2, 3, 456789000, time.UTC)
	cases := []struct{ name, identifier, text, status, recipient, original string }{
		{"queue acceptance is not delivery", "postfix/qmgr", "ABC123: from=<sender@example.test>, size=123, nrcpt=2 (queue active)", "queued", "", ""},
		{"SMTP deferred", "postfix/smtp", "ABC123: to=<one@remote.test>, relay=mx.remote.test[192.0.2.10]:25, delay=1.23, delays=0.1/0.1/0.1/0.93, dsn=4.2.0, status=deferred (host mx.remote.test said: 451 try later)", "deferred", "one@remote.test", ""},
		{"SMTP handoff", "postfix/smtp", "ABC123: to=<one@remote.test>, relay=mx.remote.test[192.0.2.10]:25, delay=2.5, delays=0/0/0/2.5, dsn=2.0.0, status=sent (250 2.0.0 queued)", "delivered", "one@remote.test", ""},
		{"different recipient bounced", "postfix/smtp", "ABC123: to=<two@remote.test>, relay=mx.remote.test[192.0.2.10]:25, delay=2, dsn=5.1.1, status=bounced (550 unknown recipient)", "bounced", "two@remote.test", ""},
		{"local forward is not final delivery", "postfix/local", "ABC123: to=<alias@local.test>, orig_to=<owner@local.test>, relay=local, delay=0.2, dsn=2.0.0, status=sent (forwarded as DEF456)", "forwarded", "alias@local.test", "owner@local.test"},
		{"LMTP local delivery", "postfix/lmtp", "ABC123: to=<box@local.test>, relay=local.test[private/dovecot-lmtp], delay=0.1, dsn=2.0.0, status=sent (250 2.0.0 Saved)", "delivered", "box@local.test", ""},
		{"local mailbox", "postfix/virtual", "ABC123: to=<box@local.test>, relay=virtual, delay=0.1, dsn=2.0.0, status=sent (delivered to maildir)", "delivered", "box@local.test", ""},
		{"pipe delivery", "postfix/pipe", "ABC123: to=<box@local.test>, relay=filter, delay=0.1, dsn=2.0.0, status=sent (delivered via filter service)", "delivered", "box@local.test", ""},
		{"error transport", "postfix/error", "ABC123: to=<box@local.test>, relay=none, delay=0.1, dsn=5.0.0, status=bounced (transport unavailable)", "bounced", "box@local.test", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, ok := Parse(Record{At: at, Identifier: tc.identifier, Message: tc.text})
			if !ok || e.Status != tc.status || e.Recipient != tc.recipient || e.OriginalRecipient != tc.original || !e.At.Equal(at) {
				t.Fatalf("unexpected parsed event: %#v, accepted=%v", e, ok)
			}
			if tc.recipient != "" && (e.Response == "" || e.DelaySeconds == nil) {
				t.Fatalf("attempt details lost: %#v", e)
			}
		})
	}
}

func TestPostfixIdentityMetadata(t *testing.T) {
	at := time.Now().UTC()
	auth, ok := Parse(Record{At: at, Identifier: "postfix/submission/smtpd", Message: "ABC123: client=host[192.0.2.1], sasl_method=PLAIN, sasl_username=Box@EXAMPLE.test"})
	if !ok || auth.AuthenticatedSender != "box@example.test" || auth.Recipient != "" || auth.Status != "accepted" {
		t.Fatalf("SMTP metadata: %#v", auth)
	}
	pickup, ok := Parse(Record{At: at, Identifier: "postfix/pickup", Message: "ABC123: uid=0 from=<script@example.test>"})
	if !ok || pickup.PickupUID == nil || *pickup.PickupUID != 0 || pickup.Sender != "script@example.test" {
		t.Fatalf("pickup metadata: %#v", pickup)
	}
	cleanup, ok := Parse(Record{At: at, Identifier: "postfix/cleanup", Message: "ABC123: message-id=<opaque@example.test>"})
	if !ok || cleanup.MessageID != "<opaque@example.test>" || cleanup.Status == "delivered" || cleanup.Recipient != "" {
		t.Fatalf("cleanup metadata: %#v", cleanup)
	}
	removed, ok := Parse(Record{At: at, Identifier: "postfix/qmgr", Message: "ABC123: removed"})
	if !ok || removed.Status != "removed" {
		t.Fatalf("removed metadata: %#v", removed)
	}
}

func TestPostfixRejectsUnrelatedAndUnsafeRecords(t *testing.T) {
	at := time.Now().UTC()
	for _, r := range []Record{
		{At: at, Identifier: "application", Message: "ABC123: to=<box@example.test>, status=sent (250 ok)"},
		{At: at, Identifier: "postfix/smtpd", Message: "NOQUEUE: reject: RCPT from host: 550 denied"},
		{At: at, Identifier: "postfix/smtpd", Message: "connect from host[192.0.2.1]"},
		{At: at, Identifier: "postfix/smtp", Message: "ABC123: to=<box@example.test>, status=unknown (arbitrary text)"},
		{At: at, Identifier: "postfix/smtp", Message: "ABC123: to=<bad\n@example.test>, status=sent (250 ok)"},
		{At: at, Identifier: "postfix/bounce", Message: "ABC123: sender non-delivery notification: DEF456"},
		{At: at, Identifier: "postfix/smtp", Message: "ABC123: " + strings.Repeat("x", maxJournalLine)},
		{Identifier: "postfix/qmgr", Message: "ABC123: removed"},
	} {
		if event, ok := Parse(r); ok {
			t.Fatalf("accepted irrelevant record: %#v", event)
		}
	}
}

func TestPostfixBoundsAttemptFields(t *testing.T) {
	text := "ABC123: to=<box@example.test>, relay=" + strings.Repeat("r", 900) + ", delay=NaN, dsn=2.0.0, status=sent (" + strings.Repeat("ş", 3000) + "\nsecret)"
	event, ok := Parse(Record{At: time.Now(), Identifier: "postfix/smtp", Message: text})
	if !ok || len(event.Response) > 4096 || len(event.Relay) > 512 || !utf8.ValidString(event.Response) || strings.ContainsAny(event.Response, "\r\n") || event.DelaySeconds != nil {
		t.Fatalf("unbounded or invalid attempt: %#v", event)
	}
}

func TestDeliveryTransitionsDoNotRegress(t *testing.T) {
	now := time.Now().UTC()
	for _, target := range []string{"delivered", "bounced", "forwarded"} {
		if !nextStatus("deferred", target, now, now.Add(time.Second)) {
			t.Fatalf("deferred -> %s was rejected", target)
		}
		if nextStatus(target, "deferred", now, now.Add(time.Second)) {
			t.Fatalf("terminal %s regressed", target)
		}
		if nextStatus("deferred", target, now, now.Add(-time.Second)) {
			t.Fatalf("old %s attempt replaced newer state", target)
		}
	}
}

func TestNativeJournalDecoding(t *testing.T) {
	record, err := decodeJournalRecord([]byte(`{"__CURSOR":"s=fixture;i=1","__REALTIME_TIMESTAMP":"1793235723456789","SYSLOG_IDENTIFIER":"postfix/qmgr","MESSAGE":"ABC123: removed"}`))
	if err != nil || record.Cursor != "s=fixture;i=1" || record.At.UnixMicro() != 1793235723456789 {
		t.Fatalf("native record: %#v, %v", record, err)
	}
	for _, line := range []string{
		`{"__CURSOR":"c","__REALTIME_TIMESTAMP":"bad","SYSLOG_IDENTIFIER":"postfix/qmgr","MESSAGE":"ABC123: removed"}`,
		`{"__CURSOR":"","__REALTIME_TIMESTAMP":"1793235723456789","SYSLOG_IDENTIFIER":"postfix/qmgr","MESSAGE":"ABC123: removed"}`,
		`{"__CURSOR":"c","__REALTIME_TIMESTAMP":"1793235723456789","SYSLOG_IDENTIFIER":"postfix/qmgr","MESSAGE":{}}`,
		`{"__CURSOR":"c","__REALTIME_TIMESTAMP":"1793235723456789","SYSLOG_IDENTIFIER":"postfix/qmgr"}`,
		`{"__CURSOR":"c","__REALTIME_TIMESTAMP":"1793235723456789","SYSLOG_IDENTIFIER":["postfix/qmgr"],"MESSAGE":"ABC123: removed"}`,
	} {
		if _, err := decodeJournalRecord([]byte(line)); err == nil {
			t.Fatalf("malformed journal entry accepted: %s", line)
		}
	}
}

func TestRecoveryOverlapIsBounded(t *testing.T) {
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	last := now.Add(-time.Hour)
	if got := recoveryStart(now, last); !got.Equal(last.Add(-2 * time.Minute)) {
		t.Fatalf("missing recovery overlap: %v", got)
	}
	for _, last := range []time.Time{{}, now.Add(-90 * 24 * time.Hour)} {
		if got := recoveryStart(now, last); !got.Equal(now.Add(-30 * 24 * time.Hour)) {
			t.Fatalf("recovery exceeded retention bound: %v", got)
		}
	}
}
