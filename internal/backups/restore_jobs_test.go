package backups

import (
	"context"
	"testing"
	"time"
)

func TestCancelRestoreJobsCancelsRegisteredWork(t *testing.T) {
	h := &Handlers{restoreCancels: make(map[int64]context.CancelFunc)}
	ctx, cancel := context.WithCancel(context.Background())
	h.restoreCancels[41] = cancel

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := h.CancelRestoreJobs(waitCtx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("çalışan geri yükleme context'i iptal edilmedi")
	}
}

func TestTruncateRestoreMessage(t *testing.T) {
	long := make([]byte, 2500)
	for i := range long {
		long[i] = 'x'
	}
	if got := truncateRestoreMessage("  tamam  "); got != "tamam" {
		t.Fatalf("boşluklar temizlenmedi: %q", got)
	}
	if got := truncateRestoreMessage(string(long)); len(got) != 2000 {
		t.Fatalf("mesaj sınırı uygulanmadı: %d", len(got))
	}
}

func TestRestoreQueueIsBounded(t *testing.T) {
	for i := 0; i < cap(restoreQueue); i++ {
		if !reserveRestoreQueue() {
			t.Fatalf("kuyruk erken doldu: %d", i)
		}
	}
	if reserveRestoreQueue() {
		t.Fatal("dolu geri yükleme kuyruğu yeni işi kabul etti")
	}
	for i := 0; i < cap(restoreQueue); i++ {
		releaseRestoreQueue()
	}
	if !reserveRestoreQueue() {
		t.Fatal("boşalan kuyruk yeni işi kabul etmedi")
	}
	releaseRestoreQueue()
}
