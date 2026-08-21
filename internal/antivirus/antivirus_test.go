package antivirus

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestInitSinir: Init ile ayarlanan üst sınırın acquire'a yansıdığını doğrular.
// 2 slotlu kuyruk → 2 eşzamanlı geçer, 3. hata verir; release sonrası yeniden geçer.
func TestInitSinir(t *testing.T) {
	Init(2)
	if got := MaxConcurrent(); got != 2 {
		t.Fatalf("MaxConcurrent = %d, beklenen 2", got)
	}
	if !acquire() || !acquire() {
		t.Fatal("ilk iki acquire başarısız — kuyruk mantığı bozuk")
	}
	if acquire() {
		t.Fatal("3. acquire geçti — sınır 2'yi aştı")
	}
	release()
	if !acquire() {
		t.Fatal("release sonrası acquire başarısız — slot iade edilmedi")
	}
	release()
	release()
}

// TestInitGecersizDegerler: <1 değerler 1'e çekilir, böylece operator yanlışlıkla
// 0 verip "sınırsız tarama" tuzağına düşmez.
func TestInitGecersizDegerler(t *testing.T) {
	for _, v := range []int{0, -1, -100} {
		Init(v)
		if got := MaxConcurrent(); got != 1 {
			t.Errorf("Init(%d) → MaxConcurrent = %d, beklenen 1", v, got)
		}
	}
	Init(1) // sonraki testler için
}

// TestAcquireReleaseParalel: N goroutine paralel acquire eder; en fazla N'i
// aynı anda tutulur (race detector bunu görür). Init(3) + 50 goroutine +
// her biri 1 acquire 1 release → hiçbir an 3'ten fazla "tutulan" olmamalı.
func TestAcquireReleaseParalel(t *testing.T) {
	Init(3)
	const G = 50
	var tutulan int32
	var maksTutulan int32
	var wg sync.WaitGroup
	wg.Add(G)
	for i := 0; i < G; i++ {
		go func() {
			defer wg.Done()
			if !acquire() {
				t.Error("acquire başarısız — 50 talep için 3 slot yetmeli (her biri hemen release)")
				return
			}
			şimdi := atomic.AddInt32(&tutulan, 1)
			defer atomic.AddInt32(&tutulan, -1)
			defer release()
			// anlık eşzamanlılık sayacını güncelle
			for {
				m := atomic.LoadInt32(&maksTutulan)
				if şimdi <= m || atomic.CompareAndSwapInt32(&maksTutulan, m, şimdi) {
					break
				}
			}
		}()
	}
	wg.Wait()
	if maksTutulan > 3 {
		t.Errorf("maks eşzamanlı = %d, sınır 3 aşıldı", maksTutulan)
	}
	if maksTutulan < 2 {
		t.Logf("uyarı: maks eşzamanlı = %d, paralellik gerçekleşmedi (zamanlayıcı sıralı çalıştırmış olabilir)", maksTutulan)
	}
	Init(1) // sonraki testler için
}

// TestAcquireInitCagirilmadan: Init çağrılmamışsa acquire non-op (sınırsız).
// Bu kasıtlı: test harness'i Init'i atlamamalı diye zorlamak yerine, unutan
// testler sessizce "sınırsız" çalışır — gerçek OOM yalnız üretimde olur.
// Burada yalnızca release'in nil-kanalda patlamadığını doğruluyoruz.
func TestAcquireInitCagirilmadan(t *testing.T) {
	sem = nil // Init etkisini geri al
	if !acquire() {
		t.Fatal("Init yok → acquire false döndü, sınırsız olmalıydı")
	}
	release() // nil kanaldan <- yapmamalı — sem==nil koruması var
	sem = make(chan struct{}, 1) // sonraki testler için geri kur
}