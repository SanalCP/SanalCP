package system

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Eşzamanlı istekler tek ölçümü paylaşmalı; süre dolunca yeniden ölçülmeli.
func TestUsageOnbellekPaylasilir(t *testing.T) {
	usageMu.Lock()
	usageZaman = time.Time{}
	usageMu.Unlock()

	var wg sync.WaitGroup
	var zamanlar sync.Map
	var n atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			usageOku()
			usageMu.Lock()
			zamanlar.Store(usageZaman, true)
			usageMu.Unlock()
			n.Add(1)
		}()
	}
	wg.Wait()
	farkli := 0
	zamanlar.Range(func(_, _ any) bool { farkli++; return true })
	if farkli != 1 {
		t.Fatalf("8 eşzamanlı istek %d ayrı ölçüm yaptı, beklenen 1", farkli)
	}

	usageMu.Lock()
	usageZaman = time.Now().Add(-usageOnbellekSuresi - time.Second)
	eski := usageZaman
	usageMu.Unlock()
	usageOku()
	usageMu.Lock()
	defer usageMu.Unlock()
	if !usageZaman.After(eski) {
		t.Fatal("süresi dolmuş önbellek yenilenmedi")
	}
}

// Yakın tarihli bir önceki örnek varsa ReadCPU uyumamalı.
func TestReadCPUOncekiOrnekleUyumaz(t *testing.T) {
	if _, err := readCPUStat(); err != nil {
		t.Skip("/proc/stat yok")
	}
	_, _ = ReadCPU() // önceki örneği oluştur
	time.Sleep(cpuEnAzAralik + 20*time.Millisecond)
	bas := time.Now()
	c, err := ReadCPU()
	if err != nil {
		t.Fatal(err)
	}
	if sure := time.Since(bas); sure >= cpuOrnekBekleme {
		t.Fatalf("ReadCPU %v sürdü; önceki örnek varken beklememeliydi", sure)
	}
	if c.Yuzde < 0 || c.Yuzde > 100 {
		t.Fatalf("CPU yüzdesi aralık dışında: %v", c.Yuzde)
	}
}
