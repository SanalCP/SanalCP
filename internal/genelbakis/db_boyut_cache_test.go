package genelbakis

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDBBoyutCacheTTLVeMapKopyasi(t *testing.T) {
	now := time.Unix(1_000, 0)
	calls := 0
	c := newDBBoyutCache(func(context.Context) (map[string]int64, error) {
		calls++
		return map[string]int64{"site_db": 42}, nil
	}, func() time.Time { return now })

	got, err := c.get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got["site_db"] = 99
	now = now.Add(30 * time.Second)
	got, err = c.get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || got["site_db"] != 42 {
		t.Fatalf("calls=%d, map=%v", calls, got)
	}
}

func TestDBBoyutCacheRefreshHatasindaStaleVeBackoff(t *testing.T) {
	now := time.Unix(2_000, 0)
	calls := 0
	c := newDBBoyutCache(func(context.Context) (map[string]int64, error) {
		calls++
		if calls == 1 {
			return map[string]int64{"site_db": 42}, nil
		}
		return nil, errors.New("db yok")
	}, func() time.Time { return now })

	if _, err := c.get(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(dbBoyutFreshTTL)
	got, err := c.get(context.Background())
	if err != nil || got["site_db"] != 42 {
		t.Fatalf("stale dönmedi: map=%v err=%v", got, err)
	}
	now = now.Add(10 * time.Second)
	if _, err := c.get(context.Background()); err != nil {
		t.Fatalf("backoff sırasında stale dönmedi: %v", err)
	}
	if calls != 2 {
		t.Fatalf("backoff loader'ı tekrar çağırdı: %d", calls)
	}
}

func TestDBBoyutCacheMaxStaleSonrasiHata(t *testing.T) {
	now := time.Unix(3_000, 0)
	calls := 0
	wantErr := errors.New("db yok")
	c := newDBBoyutCache(func(context.Context) (map[string]int64, error) {
		calls++
		if calls == 1 {
			return map[string]int64{"site_db": 42}, nil
		}
		return nil, wantErr
	}, func() time.Time { return now })

	if _, err := c.get(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(dbBoyutMaxStale + time.Second)
	if got, err := c.get(context.Background()); !errors.Is(err, wantErr) || got != nil {
		t.Fatalf("max stale sonrası map=%v err=%v", got, err)
	}
	if got, err := c.get(context.Background()); !errors.Is(err, wantErr) || got != nil {
		t.Fatalf("hata backoff'u map=%v err=%v", got, err)
	}
	if calls != 2 {
		t.Fatalf("backoff loader'ı tekrar çağırdı: %d", calls)
	}
}

func TestDBBoyutCacheColdStartStampedeOlmaz(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	c := newDBBoyutCache(func(context.Context) (map[string]int64, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return map[string]int64{"site_db": 42}, nil
	}, time.Now)

	const n = 12
	results := make(chan map[string]int64, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	go func() {
		defer wg.Done()
		got, err := c.get(context.Background())
		results <- got
		errs <- err
	}()
	<-started
	for i := 1; i < n; i++ {
		go func() {
			defer wg.Done()
			got, err := c.get(context.Background())
			results <- got
			errs <- err
		}()
	}
	close(release)
	wg.Wait()
	close(results)
	close(errs)

	if calls.Load() != 1 {
		t.Fatalf("loader %d kez çağrıldı", calls.Load())
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for got := range results {
		if got["site_db"] != 42 {
			t.Fatalf("beklenmeyen map: %v", got)
		}
	}
}

func TestDBBoyutCacheRefreshSirasindaDigerIstekStaleAlir(t *testing.T) {
	now := time.Unix(4_000, 0)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	c := newDBBoyutCache(func(context.Context) (map[string]int64, error) {
		if calls.Add(1) == 1 {
			return map[string]int64{"site_db": 42}, nil
		}
		close(started)
		<-release
		return map[string]int64{"site_db": 84}, nil
	}, func() time.Time { return now })

	if _, err := c.get(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(dbBoyutFreshTTL)
	refreshed := make(chan map[string]int64, 1)
	go func() {
		got, _ := c.get(context.Background())
		refreshed <- got
	}()
	<-started

	stale, err := c.get(context.Background())
	if err != nil || stale["site_db"] != 42 {
		t.Fatalf("refresh sırasında stale dönmedi: map=%v err=%v", stale, err)
	}
	close(release)
	if got := <-refreshed; got["site_db"] != 84 {
		t.Fatalf("refresh sonucu beklenmeyen map: %v", got)
	}
}
