// Package diskusage bounds and coalesces full filesystem scans across API endpoints.
package diskusage

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrBusy = errors.New("disk ölçüm kuyruğu dolu")

type key struct {
	path     string
	apparent bool
}

type result struct {
	bytes   int64
	err     error
	expires time.Time
}

type flight struct {
	done   chan struct{}
	result result
}

type meter struct {
	mu         sync.Mutex
	cache      map[key]result
	pending    map[key]*flight
	slots      chan struct{}
	run        func(context.Context, string, bool) (int64, error)
	ttl        time.Duration
	failureTTL time.Duration
	timeout    time.Duration
}

func newMeter(run func(context.Context, string, bool) (int64, error)) *meter {
	return &meter{
		cache: make(map[key]result), pending: make(map[key]*flight),
		slots: make(chan struct{}, 2), run: run,
		ttl: time.Minute, failureTTL: 5 * time.Second, timeout: time.Minute,
	}
}

var shared = newMeter(scan)

// Measure returns bytes; apparent preserves du -sb semantics, false reports
// allocated bytes. An error may accompany a previous successful measurement.
// Cancellation stops this caller waiting, without cancelling other waiters.
func Measure(ctx context.Context, path string, apparent bool) (int64, error) {
	return shared.measure(ctx, path, apparent)
}

func (m *meter) measure(ctx context.Context, path string, apparent bool) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	k := key{filepath.Clean(path), apparent}
	m.mu.Lock()
	old := m.cache[k]
	if time.Now().Before(old.expires) {
		m.mu.Unlock()
		return old.bytes, old.err
	}
	f, ok := m.pending[k]
	if !ok {
		// Bound queued goroutines too, not only active du subprocesses.
		if len(m.pending) >= 32 {
			m.mu.Unlock()
			return old.bytes, ErrBusy
		}
		f = &flight{done: make(chan struct{})}
		m.pending[k] = f
		go m.measureOne(k, f, old)
	}
	m.mu.Unlock()
	select {
	case <-ctx.Done():
		return old.bytes, ctx.Err()
	case <-f.done:
		return f.result.bytes, f.result.err
	}
}

func (m *meter) measureOne(k key, f *flight, old result) {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()
	r := result{bytes: old.bytes}
	select {
	case m.slots <- struct{}{}:
		n, err := m.run(ctx, k.path, k.apparent)
		<-m.slots
		r.err = err
		if err == nil {
			r.bytes = n
		}
	case <-ctx.Done():
		r.err = ctx.Err()
	}
	ttl := m.ttl
	if r.err != nil {
		ttl = m.failureTTL
	}
	r.expires = time.Now().Add(ttl)
	m.mu.Lock()
	// Fixed memory bound, even if all cached entries are fresh.
	if len(m.cache) >= 512 {
		var oldest key
		var expiry time.Time
		for k, v := range m.cache {
			if expiry.IsZero() || v.expires.Before(expiry) {
				oldest, expiry = k, v.expires
			}
		}
		delete(m.cache, oldest)
	}
	m.cache[k] = r
	delete(m.pending, k)
	f.result = r
	close(f.done)
	m.mu.Unlock()
}

func scan(ctx context.Context, path string, apparent bool) (int64, error) {
	args := []string{"-s", "--block-size=1"}
	if apparent {
		args = append(args, "--apparent-size")
	}
	args = append(args, "--", path)
	out, err := exec.CommandContext(ctx, "du", args...).Output()
	if err != nil {
		return 0, fmt.Errorf("disk ölçülemedi: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0, errors.New("boş du çıktısı")
	}
	n, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || n < 0 {
		return 0, errors.New("geçersiz du çıktısı")
	}
	return n, nil
}
