package middleware

// ratelimit_hesap_test.go — hesap bazlı kaba-kuvvet sınırının testi.
//
// Savunulan davranış: IP başına sınır, her denemesini başka bir adresten yapan
// dağıtık bir saldırganı DURDURMAZ. Hesap sayacı olmadan tek bir hesaba (pratikte
// root'a) yapılan çevrimiçi kaba-kuvvet sınırsızdır.

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// sayaclariSifirla — paket-global haritalar testler arasında sızmasın.
func sayaclariSifirla(t *testing.T) {
	t.Helper()
	girisMu.Lock()
	girisMap = map[string]*girisKayit{}
	girisMu.Unlock()
	hesapMu.Lock()
	hesapMap = map[string]*girisKayit{}
	hesapMu.Unlock()
}

// hepsiBasarisiz — her zaman 401 dönen bir giriş handler'ı.
var hepsiBasarisiz = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusUnauthorized)
})

// girisDene — verilen IP'den verilen kullanıcı adıyla bir giriş denemesi yapar.
func girisDene(h http.Handler, ip, kullanici string) int {
	govde := `{"kullanici":"` + kullanici + `","parola":"yanlis"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(govde))
	r.RemoteAddr = ip + ":12345"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

// Farklı IP'lerden aynı hesaba yapılan denemeler eşiği aşınca kilitlenmeli.
func TestGirisLimiti_DagitikSaldiriHesapBazindaKilitlenir(t *testing.T) {
	sayaclariSifirla(t)
	h := GirisLimiti(hepsiBasarisiz)

	// Her deneme BAŞKA bir IP'den → IP sayacı hiç dolmaz.
	for i := 0; i < hesapMaxHata; i++ {
		ip := "10.0." + strconv.Itoa(i/250) + "." + strconv.Itoa(i%250+1)
		if kod := girisDene(h, ip, "root"); kod != http.StatusUnauthorized {
			t.Fatalf("%d. deneme beklenmedik kod: %d", i+1, kod)
		}
	}
	// Eşik doldu: yepyeni bir IP'den gelse bile hesap kilitli olmalı.
	if kod := girisDene(h, "203.0.113.9", "root"); kod != http.StatusTooManyRequests {
		t.Fatalf("hesap kilitlenmedi, kod=%d (dağıtık kaba-kuvvet sınırsız)", kod)
	}
}

// Bir hesabın kilitlenmesi BAŞKA hesapları etkilememeli.
func TestGirisLimiti_KilitYalnizHedefHesabaAit(t *testing.T) {
	sayaclariSifirla(t)
	h := GirisLimiti(hepsiBasarisiz)

	for i := 0; i < hesapMaxHata; i++ {
		girisDene(h, "10.1."+strconv.Itoa(i/250)+"."+strconv.Itoa(i%250+1), "root")
	}
	// Farklı hesap, hiç kullanılmamış IP → normal 401 almalı, 429 değil.
	if kod := girisDene(h, "203.0.113.10", "bayi1"); kod != http.StatusUnauthorized {
		t.Fatalf("başka hesap da kilitlendi, kod=%d", kod)
	}
}

// IP sınırı hesap sınırından bağımsız çalışmaya devam etmeli (regresyon).
func TestGirisLimiti_IPSiniriHalaIsler(t *testing.T) {
	sayaclariSifirla(t)
	h := GirisLimiti(hepsiBasarisiz)

	// Aynı IP, her seferinde FARKLI hesap → hesap sayacı dolmaz, IP sayacı dolar.
	for i := 0; i < girisMaxHata; i++ {
		if kod := girisDene(h, "198.51.100.7", "k"+strconv.Itoa(i)); kod != http.StatusUnauthorized {
			t.Fatalf("%d. deneme beklenmedik kod: %d", i+1, kod)
		}
	}
	if kod := girisDene(h, "198.51.100.7", "baska"); kod != http.StatusTooManyRequests {
		t.Fatalf("IP sınırı çalışmadı, kod=%d", kod)
	}
}

// Handler gövdeyi middleware'den SONRA okuyabilmeli — hesap adını okumak için
// gövde tüketildiği hâlde geri konur.
func TestGirisLimiti_GovdeHandlerIcinKorunur(t *testing.T) {
	sayaclariSifirla(t)
	var gorulen string
	h := GirisLimiti(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 256)
		n, _ := r.Body.Read(b)
		gorulen = string(b[:n])
		w.WriteHeader(http.StatusOK)
	}))
	girisDene(h, "198.51.100.8", "root")
	if !strings.Contains(gorulen, `"kullanici":"root"`) {
		t.Fatalf("handler gövdeyi okuyamadı: %q", gorulen)
	}
}

// Aşırı büyük gövde, sayaçları atlatarak geçmek yerine reddedilmeli.
func TestGirisLimiti_AsiriGovdeReddedilir(t *testing.T) {
	sayaclariSifirla(t)
	h := GirisLimiti(hepsiBasarisiz)
	govde := `{"kullanici":"root","dolgu":"` + strings.Repeat("A", girisMaxGovde) + `"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(govde))
	r.RemoteAddr = "198.51.100.9:12345"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("aşırı gövde reddedilmedi, kod=%d", w.Code)
	}
}
