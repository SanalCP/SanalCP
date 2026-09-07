package genelbakis

import (
	"context"
	"sync"
	"time"

	"sanalcp/internal/hesaplar"
)

const (
	dbBoyutFreshTTL = 60 * time.Second
	dbBoyutMaxStale = 10 * time.Minute
	dbBoyutBackoff  = 20 * time.Second
)

type dbBoyutLoader func(context.Context) (map[string]int64, error)

type dbBoyutCache struct {
	mu          sync.Mutex
	load        dbBoyutLoader
	now         func() time.Time
	data        map[string]int64
	loadedAt    time.Time
	lastFailure time.Time
	lastErr     error
	loading     bool
	wait        chan struct{}
}

var dbBoyutlariCache = newDBBoyutCache(hesaplar.VeritabaniBoyutlari, time.Now)

func newDBBoyutCache(load dbBoyutLoader, now func() time.Time) *dbBoyutCache {
	return &dbBoyutCache{load: load, now: now}
}

func (c *dbBoyutCache) get(ctx context.Context) (map[string]int64, error) {
	for {
		c.mu.Lock()
		now := c.now()
		age := now.Sub(c.loadedAt)
		hasData := c.data != nil
		if hasData && age < dbBoyutFreshTTL {
			out := cloneDBBoyutlari(c.data)
			c.mu.Unlock()
			return out, nil
		}

		if c.loading {
			if hasData && age <= dbBoyutMaxStale {
				out := cloneDBBoyutlari(c.data)
				c.mu.Unlock()
				return out, nil
			}
			wait := c.wait
			c.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		if c.lastErr != nil && now.Sub(c.lastFailure) < dbBoyutBackoff {
			if hasData && age <= dbBoyutMaxStale {
				out := cloneDBBoyutlari(c.data)
				c.mu.Unlock()
				return out, nil
			}
			err := c.lastErr
			c.mu.Unlock()
			return nil, err
		}

		c.loading = true
		c.wait = make(chan struct{})
		wait := c.wait
		c.mu.Unlock()

		data, err := c.load(ctx)

		c.mu.Lock()
		if err == nil {
			c.data = cloneDBBoyutlari(data)
			c.loadedAt = c.now()
			c.lastErr = nil
		} else {
			c.lastErr = err
			c.lastFailure = c.now()
		}
		c.loading = false
		close(wait)
		now = c.now()
		hasData = c.data != nil && now.Sub(c.loadedAt) <= dbBoyutMaxStale
		if hasData {
			data = cloneDBBoyutlari(c.data)
		}
		c.mu.Unlock()

		if hasData {
			return data, nil
		}
		return nil, err
	}
}

func cloneDBBoyutlari(src map[string]int64) map[string]int64 {
	dst := make(map[string]int64, len(src))
	for ad, boyut := range src {
		dst[ad] = boyut
	}
	return dst
}
