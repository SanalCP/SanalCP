package maildelivery

import (
	"testing"
	"time"
)

func TestOnlyJournalAuthenticatedPostfixProducersAreTrusted(t *testing.T) {
	cases := []struct {
		name   string
		record Record
		want   bool
	}{
		{"root Postfix daemon", Record{Identifier: "postfix/smtp", UID: "0", Exe: "/usr/lib/postfix/sbin/smtp"}, true},
		{"postfix user daemon", Record{Identifier: "postfix/smtpd", UID: "108", Exe: "/usr/lib/postfix/sbin/smtpd"}, true},
		{"RHEL libexec layout", Record{Identifier: "postfix/submission/smtpd", UID: "0", Exe: "/usr/libexec/postfix/smtpd"}, true},
		{"admin queue removal", Record{Identifier: "postfix/postsuper", UID: "0", Exe: "/usr/sbin/postsuper"}, true},
		{"tenant logger forged tag", Record{Identifier: "postfix/smtp", UID: "1001", Exe: "/usr/bin/logger"}, false},
		{"tenant runs real binary", Record{Identifier: "postfix/smtp", UID: "1001", Exe: "/usr/lib/postfix/sbin/smtp"}, false},
		{"postfix uid with wrong binary", Record{Identifier: "postfix/smtp", UID: "108", Exe: "/home/tenant/smtp"}, false},
		{"tag and executable disagree", Record{Identifier: "postfix/qmgr", UID: "0", Exe: "/usr/lib/postfix/sbin/smtp"}, false},
		{"missing journal credentials", Record{Identifier: "postfix/smtp"}, false},
		{"non-root postsuper", Record{Identifier: "postfix/postsuper", UID: "108", Exe: "/usr/sbin/postsuper"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := trustedRecord(tc.record, "108"); got != tc.want {
				t.Fatalf("trusted=%v want %v", got, tc.want)
			}
		})
	}
}

func TestAdministrativeQueueRemovalClosesInstanceWithoutDeliveryResult(t *testing.T) {
	event, ok := Parse(Record{At: time.Now(), Identifier: "postfix/postsuper", Message: "ABC123: removed"})
	if !ok || event.Kind != "removed" || event.Status != "removed" || event.Recipient != "" {
		t.Fatalf("unexpected postsuper event: %#v accepted=%v", event, ok)
	}
	if _, ok := Parse(Record{At: time.Now(), Identifier: "postfix/postsuper", Message: "ABC123: placed on hold"}); ok {
		t.Fatal("non-removal postsuper text must not create events")
	}
}
