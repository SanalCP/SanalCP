package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestLimitBody_SinirAltiGecer(t *testing.T) {
	var okunan int64
	var okumaHatasi error
	h := LimitBody(1 << 20)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		okunan, okumaHatasi = io.Copy(io.Discard, r.Body)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 1000)))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if okumaHatasi != nil {
		t.Fatalf("sınır altındaki gövde hata verdi: %v", okumaHatasi)
	}
	if okunan != 1000 {
		t.Fatalf("okunan = %d, beklenen 1000", okunan)
	}
}

func TestLimitBody_SinirUstuKesilir(t *testing.T) {
	var okumaHatasi error
	h := LimitBody(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, okumaHatasi = io.Copy(io.Discard, r.Body)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 4096)))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if okumaHatasi == nil {
		t.Fatal("sınır aşıldığı halde okuma hatası dönmedi")
	}
	if !GovdeSinirAsildi(okumaHatasi) {
		t.Fatalf("hata MaxBytesError olarak tanınmadı: %v", okumaHatasi)
	}
}

// En kritik regresyon testi: ExtendBodyLimit, LimitBody'nin koyduğu KÜÇÜK
// varsayılanı gerçekten geçersiz kılmalı. Naif bir uygulama (r.Body'yi tekrar
// sarmak) iç içe iki MaxBytesReader üretir ve KÜÇÜK olan sınır geçerli olur —
// yani 2 MiB'lik varsayılan 10 GiB'lik yüklemeyi keserdi.
func TestExtendBodyLimit_VarsayilaniGercektenYukseltir(t *testing.T) {
	const varsayilan = 1024
	const yuksek = 64 << 10
	const govde = 8 << 10 // varsayılanın üstünde, yükseltilmiş sınırın altında

	var okunan int64
	var okumaHatasi error
	h := LimitBody(varsayilan)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ExtendBodyLimit(w, r, yuksek)
		okunan, okumaHatasi = io.Copy(io.Discard, r.Body)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", govde)))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if okumaHatasi != nil {
		t.Fatalf("yükseltilmiş sınırın altındaki gövde kesildi: %v", okumaHatasi)
	}
	if okunan != govde {
		t.Fatalf("okunan = %d, beklenen %d — varsayılan sınır hâlâ geçerli", okunan, govde)
	}
}

// Yükseltilmiş sınırın kendisi de bir tavandır: sonsuz değildir.
func TestExtendBodyLimit_KendiSinirindaKeser(t *testing.T) {
	var okumaHatasi error
	h := LimitBody(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ExtendBodyLimit(w, r, 4096)
		_, okumaHatasi = io.Copy(io.Discard, r.Body)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 16<<10)))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !GovdeSinirAsildi(okumaHatasi) {
		t.Fatalf("yükseltilmiş sınır aşıldığı halde MaxBytesError dönmedi: %v", okumaHatasi)
	}
}

// LimitBody zincirde yokken (birim testi / doğrudan çağrı) ExtendBodyLimit yine
// de sınır uygulamalı — sessizce "sınırsız"a düşmemeli.
func TestExtendBodyLimit_MiddlewareYokkenDeSinirlar(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 8192)))
	rec := httptest.NewRecorder()

	ExtendBodyLimit(rec, req, 1024)
	_, err := io.Copy(io.Discard, req.Body)

	if !GovdeSinirAsildi(err) {
		t.Fatalf("middleware yokken sınır uygulanmadı: %v", err)
	}
}

func TestYuklemeSlot_KotaDolunca429(t *testing.T) {
	const anahtar = "test-tenant"
	birakanlar := make([]func(), 0, EszamanliYuklemeSiniri)
	for i := 0; i < EszamanliYuklemeSiniri; i++ {
		birak, ok := YuklemeSlotAl(anahtar)
		if !ok {
			t.Fatalf("%d. slot alınamadı, sınır %d", i+1, EszamanliYuklemeSiniri)
		}
		birakanlar = append(birakanlar, birak)
	}

	rec := httptest.NewRecorder()
	if _, ok := YuklemeSlotVeyaHata(rec, anahtar); ok {
		t.Fatal("kota dolu olduğu halde slot verildi")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("durum = %d, beklenen 429", rec.Code)
	}

	// Bir slot bırakılınca yeniden alınabilmeli.
	birakanlar[0]()
	birak, ok := YuklemeSlotAl(anahtar)
	if !ok {
		t.Fatal("slot bırakıldıktan sonra yeniden alınamadı")
	}
	birak()
	for _, f := range birakanlar[1:] {
		f()
	}
}

// Slot sayacı tenant başına ayrılmalı: bir müşterinin kotayı doldurması
// diğerinin yüklemesini engellememeli.
func TestYuklemeSlot_TenantBasinaAyri(t *testing.T) {
	birakanlar := make([]func(), 0, EszamanliYuklemeSiniri)
	for i := 0; i < EszamanliYuklemeSiniri; i++ {
		birak, ok := YuklemeSlotAl("tenant-a")
		if !ok {
			t.Fatalf("tenant-a %d. slot alınamadı", i+1)
		}
		birakanlar = append(birakanlar, birak)
	}
	defer func() {
		for _, f := range birakanlar {
			f()
		}
	}()

	birak, ok := YuklemeSlotAl("tenant-b")
	if !ok {
		t.Fatal("tenant-a kotayı doldurdu diye tenant-b engellendi")
	}
	birak()
}

// Slot bırakma idempotent olmalı (defer + erken return birlikte çağırabilir);
// aksi halde sayaç eksiye kayar ve kota kalıcı olarak gevşer.
func TestYuklemeSlot_BirakmaIdempotent(t *testing.T) {
	const anahtar = "idempotent"
	birak, ok := YuklemeSlotAl(anahtar)
	if !ok {
		t.Fatal("ilk slot alınamadı")
	}
	birak()
	birak()
	birak()

	birakanlar := make([]func(), 0, EszamanliYuklemeSiniri)
	for i := 0; i < EszamanliYuklemeSiniri; i++ {
		f, ok := YuklemeSlotAl(anahtar)
		if !ok {
			t.Fatalf("%d. slot alınamadı — çoklu bırakma sayacı bozmuş", i+1)
		}
		birakanlar = append(birakanlar, f)
	}
	if _, ok := YuklemeSlotAl(anahtar); ok {
		t.Fatal("kota dolu olmasına rağmen slot verildi — sayaç eksiye kaymış")
	}
	for _, f := range birakanlar {
		f()
	}
}

func TestYuklemeSlot_EszamanliErisimGuvenli(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if birak, ok := YuklemeSlotAl("yaris"); ok {
				birak()
			}
		}()
	}
	wg.Wait()

	// Tüm slotlar bırakıldığına göre kota yeniden tam olmalı.
	for i := 0; i < EszamanliYuklemeSiniri; i++ {
		birak, ok := YuklemeSlotAl("yaris")
		if !ok {
			t.Fatalf("yarış sonrası %d. slot alınamadı — sayaç sızdırmış", i+1)
		}
		defer birak()
	}
}
