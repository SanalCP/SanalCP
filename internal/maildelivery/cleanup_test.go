package maildelivery

import (
	"testing"
	"time"
)

// Observed in production Postfix 3.10 logs: an aborted DATA phase is logged only
// by cleanup. It must close the tracked instance so retention can remove it.
func TestCleanupCanceledMessageClosesInstance(t *testing.T) {
	e, ok := Parse(Record{At: time.Now(), Identifier: "postfix/cleanup", Message: "ABC123: removed (canceled)"})
	if !ok || e.Kind != "removed" || e.Status != "removed" || e.Recipient != "" {
		t.Fatalf("canceled message not treated as removal: %#v accepted=%v", e, ok)
	}
	if _, ok := Parse(Record{At: time.Now(), Identifier: "postfix/cleanup", Message: "milter-reject: END-OF-MESSAGE from host[192.0.2.1]: 5.7.1 rejected"}); ok {
		t.Fatal("SMTP-time milter rejection has no queue instance and must not be tracked")
	}
}
