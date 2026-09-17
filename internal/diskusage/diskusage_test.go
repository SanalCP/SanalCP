package diskusage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentSamePathAndCache(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	m := newMeter(func(context.Context, string, bool) (int64, error) {
		calls.Add(1)
		close(started)
		<-release
		return 123, nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := m.measure(context.Background(), "/same", false)
			if err != nil || n != 123 {
				t.Errorf("%d %v", n, err)
			}
		}()
	}
	<-started
	close(release)
	wg.Wait()
	n, err := m.measure(context.Background(), "/same/.", false)
	if err != nil || n != 123 || calls.Load() != 1 {
		t.Fatalf("scan not cached/coalesced: %d %v %d", n, err, calls.Load())
	}
}

func TestGlobalConcurrencyAndCancelledWaiter(t *testing.T) {
	var active, peak atomic.Int32
	started := make(chan struct{}, 16)
	release := make(chan struct{})
	m := newMeter(func(ctx context.Context, _ string, _ bool) (int64, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for p := peak.Load(); n > p; p = peak.Load() {
			if peak.CompareAndSwap(p, n) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
			return 42, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancelled := make(chan error, 1)
	go func() { _, err := m.measure(ctx, "/same", false); cancelled <- err }()
	<-started
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := m.measure(context.Background(), fmt.Sprintf("/path%d", i), false)
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	<-started
	cancel()
	if err := <-cancelled; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	// The shared measurement remains available to another waiter.
	wg.Add(1)
	go func() {
		defer wg.Done()
		n, err := m.measure(context.Background(), "/same", false)
		if n != 42 || err != nil {
			t.Errorf("other waiter: %d %v", n, err)
		}
	}()
	close(release)
	wg.Wait()
	if peak.Load() != 2 {
		t.Fatalf("global concurrency = %d", peak.Load())
	}
}

func TestFailureBackoffKeepsPreviousValue(t *testing.T) {
	var calls int
	failure := errors.New("du timeout")
	m := newMeter(func(context.Context, string, bool) (int64, error) {
		calls++
		if calls == 1 {
			return 99, nil
		}
		return 0, failure
	})
	n, err := m.measure(context.Background(), "/path", false)
	if n != 99 || err != nil {
		t.Fatal(n, err)
	}
	m.mu.Lock()
	k := key{"/path", false}
	old := m.cache[k]
	old.expires = time.Now().Add(-time.Second)
	m.cache[k] = old
	m.mu.Unlock()
	for i := 0; i < 3; i++ {
		n, err = m.measure(context.Background(), "/path", false)
		if n != 99 || !errors.Is(err, failure) {
			t.Fatal(n, err)
		}
	}
	if calls != 2 {
		t.Fatalf("failure retry storm: %d", calls)
	}
}

func TestQueueBoundAndTimeout(t *testing.T) {
	m := newMeter(func(ctx context.Context, _ string, _ bool) (int64, error) { <-ctx.Done(); return 0, ctx.Err() })
	m.timeout = 20 * time.Millisecond
	m.mu.Lock()
	for i := 0; i < 32; i++ {
		m.pending[key{fmt.Sprint(i), false}] = &flight{done: make(chan struct{})}
	}
	m.mu.Unlock()
	if _, err := m.measure(context.Background(), "/overflow", false); !errors.Is(err, ErrBusy) {
		t.Fatalf("unbounded queue: %v", err)
	}
	m.mu.Lock()
	clear(m.pending)
	m.mu.Unlock()
	if _, err := m.measure(context.Background(), "/timeout", false); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout: %v", err)
	}
}

func TestScanPreservesApparentAndAllocatedSemantics(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "sparse"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(64 << 20); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a, err := scan(ctx, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := scan(ctx, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if a < 64<<20 || b >= a {
		t.Fatalf("sparse disk accounting: apparent=%d allocated=%d", a, b)
	}
}
