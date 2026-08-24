# Panel Lisans Numarası + Kurulum Telemetrisi Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Her SanalCP kurulumunun `/araclar-ayarlar` sayfasında görünen bir lisans numarası olmasını ve panel sahibinin (Firebase/Firestore üzerinden) tüm kurulumların IP/sürüm/zaman envanterini merkezi görebilmesini sağlamak.

**Architecture:** Mevcut `internal/system/surumkontrol.go` içindeki periyodik sürüm-kontrol goroutine'ine (aynı bayrak, aynı periyot) yeni bir `telemetriGonder()` çağrısı eklenir. Bu fonksiyon kurulumun kendi IP'sini harici bir uca sorar, mevcut sürüm/OS ailesi/dil bilgisini toplar ve doğrudan Firestore REST API'sine (Cloud Function ara katmanı yok) `PATCH` ile yazar. Firestore Security Rules yazmayı whitelist'li alanlarla ve kendi doküman ID'sine sınırlar, okumayı tamamen kapatır. Lisans numarası, panelde zaten var olan `KurulumKimligi()` değeridir — ayrı bir üretim mekanizması yok.

**Tech Stack:** Go (backend, `internal/system`), React/TypeScript (frontend, `frontend/src/components`), Firestore REST API + Security Rules.

**Spec:** `docs/superpowers/specs/2026-08-24-panel-lisans-telemetri-design.md`

## Global Constraints

- Açma/kapama: mevcut `PANEL_SURUM_KONTROL` bayrağı hem sürüm kontrolünü hem telemetriyi kapatır — ayrı bir bayrak eklenmez.
- Lisans numarası = mevcut `KurulumKimligi()` (16 byte hex, `/etc/sanalcp/kurulum-kimlik`) — yeni üretim mekanizması yok.
- Firestore'da okuma tamamen kapalı (`allow read: if false`); yazma yalnız `create`/`update`, whitelist alanlarla, kendi doküman ID'sine.
- `ilk_kurulum_zamani` doküman ilk yazıldıktan sonra değişmez (Rules'da `update` isteklerinde eşitlik kontrolü).
- Ağ hatası (IP ucu veya Firestore ulaşılamaz) = tamamen sessiz; panel hiçbir yerde hata göstermez, hiçbir akışı etkilemez.
- `kaynak_ip` kendi beyanıdır — panel harici bir "IP nedir" ucuna sorup sonucu gönderir (bağlantıdan otomatik tespit değil).
- Firestore proje adı ve API anahtarı gömülü varsayılanla (env var ile override edilebilir) — bkz. Task 2 sonundaki "Devreye Alma Notu".

---

### Task 1: `internal/system/telemetri.go` — çekirdek mantık

**Files:**
- Create: `internal/system/telemetri.go`
- Create: `internal/system/telemetri_test.go`

**Interfaces:**
- Consumes: `KurulumKimligi()` (`internal/system/surumkontrol.go`, mevcut), `surumMevcutOku()` (`internal/system/surumkontrol.go`, mevcut, aynı paket), `panelDili()` (`internal/system/dil.go`, mevcut, aynı paket), `osfam.Mevcut().ID` (`internal/osfam/osfam.go`, mevcut, `string`).
- Produces: `func telemetriGonder()` — argümansız, yan etkili (ağ çağrısı yapar); Task 2 bunu `SurumBaslat`'ın goroutine döngüsünden çağıracak.

- [ ] **Step 1: `ipBicimGecerliMi` için başarısız test yaz**

`internal/system/telemetri_test.go` dosyasını oluştur:

```go
package system

import "testing"

func TestIpBicimGecerliMi(t *testing.T) {
	vakalar := []struct {
		girdi   string
		gecerli bool
	}{
		{"1.2.3.4", true},
		{"2001:db8::1", true},
		{"", false},
		{"<html>hata</html>", false},
		{"1.2.3.4\n5.6.7.8", false},
	}
	for _, v := range vakalar {
		if got := ipBicimGecerliMi(v.girdi); got != v.gecerli {
			t.Errorf("ipBicimGecerliMi(%q) = %v, beklenen %v", v.girdi, got, v.gecerli)
		}
	}
}
```

- [ ] **Step 2: Testin başarısız olduğunu doğrula**

Çalıştır: `go test ./internal/system/... -run TestIpBicimGecerliMi -v`
Beklenen: derleme hatası — `undefined: ipBicimGecerliMi`

- [ ] **Step 3: `telemetri.go` iskeletini ve `ipBicimGecerliMi`'yi yaz**

`internal/system/telemetri.go`:

```go
package system

// Kurulum telemetrisi — panel sahibinin tüm kurulumların envanterini
// (hangi IP'ye ne zaman hangi sürüm kurulmuş, şu an hangi sürümde)
// görebilmesi için. bkz. docs/superpowers/specs/2026-08-24-panel-lisans-telemetri-design.md
//
// AYNI GORUTIN: surumkontrol.go'daki periyodik döngüye (SurumBaslat) binilir,
// ayrı bir zamanlayıcı YOK. PANEL_SURUM_KONTROL=0 telemetriyi de kapatır.
//
// AĞ HATASI = SESSİZ: IP ucu veya Firestore ulaşılamazsa panel etkilenmez,
// hiçbir yerde hata gösterilmez (surumkontrol.go'daki felsefeyle aynı).

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sanalcp/internal/osfam"
)

const (
	ipUCVarsayilan       = "https://api.ipify.org?format=text"
	firestoreTabanVarsayilan = "https://firestore.googleapis.com/v1"
	// Gerçek Firebase projesi/anahtarı henüz sağlanmadı — bkz. Task 2 sonundaki
	// "Devreye Alma Notu". Boş bırakıldıkça telemetriGonder() sessizce no-op'tur.
	firebaseProjeVarsayilan   = ""
	firebaseAnahtarVarsayilan = ""

	ilkZamanYol          = "/etc/sanalcp/kurulum-ilk-zaman"
	telemetriGovdeSiniri = 8 << 10
)

// telemetriAlanlar — Firestore doküman alanları, updateMask için de kullanılır.
var telemetriAlanlar = []string{
	"kurulum_kimlik", "mevcut_surum", "kaynak_ip",
	"osfam", "panel_dili", "ilk_kurulum_zamani", "son_gorulme_zamani",
}

// ipBicimGecerliMi: saf karar fonksiyonu — IP ucu beklenmedik bir hata sayfası
// ya da boş gövde dönerse Firestore'a çöp yazılmasın diye kaba biçim kontrolü.
// Sıkı bir IP parser DEĞİL (IPv6 dahil çok çeşit geçerli biçim var); yalnız
// "makul uzunlukta, boşluksuz/etiketsiz tek satır" ister.
func ipBicimGecerliMi(s string) bool {
	if s == "" || len(s) > 45 { // 45 = en uzun IPv6 metin gösterimi
		return false
	}
	return !strings.ContainsAny(s, " \t\n\"<>")
}
```

- [ ] **Step 4: Testin geçtiğini doğrula**

Çalıştır: `go test ./internal/system/... -run TestIpBicimGecerliMi -v`
Beklenen: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/system/telemetri.go internal/system/telemetri_test.go
git commit -m "feat(system): telemetri iskeleti + IP biçim kontrolü"
```

- [ ] **Step 6: `telemetriHazirMi` için başarısız test yaz**

`internal/system/telemetri_test.go`'a ekle:

```go
func TestTelemetriHazirMi(t *testing.T) {
	if telemetriHazirMi("", "") {
		t.Error("proje ve anahtar boşken hazır olmamalı")
	}
	if telemetriHazirMi("proje", "") {
		t.Error("anahtar boşken hazır olmamalı")
	}
	if telemetriHazirMi("", "anahtar") {
		t.Error("proje boşken hazır olmamalı")
	}
	if !telemetriHazirMi("proje", "anahtar") {
		t.Error("ikisi de doluyken hazır olmalı")
	}
}
```

- [ ] **Step 7: Testin başarısız olduğunu doğrula**

Çalıştır: `go test ./internal/system/... -run TestTelemetriHazirMi -v`
Beklenen: derleme hatası — `undefined: telemetriHazirMi`

- [ ] **Step 8: `telemetriHazirMi` ve ortam değişkeni okuyucularını yaz**

`telemetri.go`'ya ekle:

```go
func ipUC() string {
	if v := strings.TrimSpace(os.Getenv("PANEL_IP_UC")); v != "" {
		return v
	}
	return ipUCVarsayilan
}

func firestoreTaban() string {
	if v := strings.TrimSpace(os.Getenv("PANEL_FIREBASE_UC")); v != "" {
		return v
	}
	return firestoreTabanVarsayilan
}

func firebaseProje() string {
	if v := strings.TrimSpace(os.Getenv("PANEL_FIREBASE_PROJE")); v != "" {
		return v
	}
	return firebaseProjeVarsayilan
}

func firebaseAnahtar() string {
	if v := strings.TrimSpace(os.Getenv("PANEL_FIREBASE_API_ANAHTARI")); v != "" {
		return v
	}
	return firebaseAnahtarVarsayilan
}

// telemetriHazirMi: saf karar fonksiyonu — proje veya anahtar boşsa (henüz
// devreye alınmamış) telemetriGonder() sessizce no-op olur.
func telemetriHazirMi(proje, anahtar string) bool {
	return proje != "" && anahtar != ""
}
```

- [ ] **Step 9: Testin geçtiğini doğrula**

Çalıştır: `go test ./internal/system/... -run TestTelemetriHazirMi -v`
Beklenen: PASS

- [ ] **Step 10: Commit**

```bash
git add internal/system/telemetri.go internal/system/telemetri_test.go
git commit -m "feat(system): telemetri devreye alma kontrolü + ortam değişkenleri"
```

- [ ] **Step 11: `firestoreDegerleriOlustur` ve `firestoreURL` için başarısız test yaz**

`internal/system/telemetri_test.go`'a ekle:

```go
func TestFirestoreDegerleriOlustur(t *testing.T) {
	d := firestoreDegerleriOlustur("kimlik123", "0.9.7", "1.2.3.4", "almalinux", "tr", "2026-01-01T00:00:00Z")
	alanlar, ok := d["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields alanı map[string]any değil: %#v", d)
	}
	kimlik, ok := alanlar["kurulum_kimlik"].(map[string]any)
	if !ok || kimlik["stringValue"] != "kimlik123" {
		t.Errorf("kurulum_kimlik = %#v, beklenen stringValue=kimlik123", alanlar["kurulum_kimlik"])
	}
	ilkZaman, ok := alanlar["ilk_kurulum_zamani"].(map[string]any)
	if !ok || ilkZaman["timestampValue"] != "2026-01-01T00:00:00Z" {
		t.Errorf("ilk_kurulum_zamani = %#v, beklenen timestampValue=2026-01-01T00:00:00Z", alanlar["ilk_kurulum_zamani"])
	}
	if _, ok := alanlar["son_gorulme_zamani"].(map[string]any); !ok {
		t.Error("son_gorulme_zamani alanı yok")
	}
}

func TestFirestoreURL(t *testing.T) {
	old := os.Getenv("PANEL_FIREBASE_PROJE")
	os.Setenv("PANEL_FIREBASE_PROJE", "test-proje")
	defer os.Setenv("PANEL_FIREBASE_PROJE", old)

	u := firestoreURL("kimlik123")
	if !strings.Contains(u, "/projects/test-proje/databases/(default)/documents/kurulumlar/kimlik123") {
		t.Errorf("URL yol beklenmedik: %s", u)
	}
	for _, alan := range telemetriAlanlar {
		if !strings.Contains(u, "updateMask.fieldPaths="+alan) {
			t.Errorf("URL, %q için updateMask içermiyor: %s", alan, u)
		}
	}
}
```

`telemetri_test.go`'nun başına `"os"` ve `"strings"` importlarını ekle.

- [ ] **Step 12: Testlerin başarısız olduğunu doğrula**

Çalıştır: `go test ./internal/system/... -run 'TestFirestoreDegerleriOlustur|TestFirestoreURL' -v`
Beklenen: derleme hatası — `undefined: firestoreDegerleriOlustur`, `undefined: firestoreURL`

- [ ] **Step 13: `firestoreDegerleriOlustur` ve `firestoreURL`'i yaz**

`telemetri.go`'ya ekle:

```go
// firestoreDegerleriOlustur — Firestore REST doküman alan haritasını kurar.
func firestoreDegerleriOlustur(kimlik, surum, ip, osAile, dil, ilkZaman string) map[string]any {
	deger := func(s string) map[string]any { return map[string]any{"stringValue": s} }
	return map[string]any{
		"fields": map[string]any{
			"kurulum_kimlik":     deger(kimlik),
			"mevcut_surum":       deger(surum),
			"kaynak_ip":          deger(ip),
			"osfam":              deger(osAile),
			"panel_dili":         deger(dil),
			"ilk_kurulum_zamani": map[string]any{"timestampValue": ilkZaman},
			"son_gorulme_zamani": map[string]any{"timestampValue": time.Now().UTC().Format(time.RFC3339)},
		},
	}
}

// firestoreURL — PATCH hedefi. Firestore'un PATCH ucu, doküman yoksa
// oluşturur (create), varsa günceller (update) — Rules bu ikisini otomatik
// ayırt eder, Go tarafında ayrı dallanma gerekmez.
func firestoreURL(kimlik string) string {
	base := firestoreTaban() + "/projects/" + firebaseProje() +
		"/databases/(default)/documents/kurulumlar/" + url.PathEscape(kimlik)
	q := url.Values{}
	for _, alan := range telemetriAlanlar {
		q.Add("updateMask.fieldPaths", alan)
	}
	if k := firebaseAnahtar(); k != "" {
		q.Set("key", k)
	}
	return base + "?" + q.Encode()
}
```

- [ ] **Step 14: Testlerin geçtiğini doğrula**

Çalıştır: `go test ./internal/system/... -run 'TestFirestoreDegerleriOlustur|TestFirestoreURL' -v`
Beklenen: PASS

- [ ] **Step 15: Commit**

```bash
git add internal/system/telemetri.go internal/system/telemetri_test.go
git commit -m "feat(system): Firestore doküman + URL oluşturma"
```

- [ ] **Step 16: `ipTespitEt` için mock sunuculu başarısız test yaz**

`internal/system/telemetri_test.go`'a ekle:

```go
func TestIpTespitEt(t *testing.T) {
	sunucu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("203.0.113.7"))
	}))
	defer sunucu.Close()

	old := os.Getenv("PANEL_IP_UC")
	os.Setenv("PANEL_IP_UC", sunucu.URL)
	defer os.Setenv("PANEL_IP_UC", old)

	if got := ipTespitEt(); got != "203.0.113.7" {
		t.Errorf("ipTespitEt() = %q, beklenen 203.0.113.7", got)
	}
}

func TestIpTespitEtHataDurumu(t *testing.T) {
	sunucu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer sunucu.Close()

	old := os.Getenv("PANEL_IP_UC")
	os.Setenv("PANEL_IP_UC", sunucu.URL)
	defer os.Setenv("PANEL_IP_UC", old)

	if got := ipTespitEt(); got != "" {
		t.Errorf("ipTespitEt() hata durumunda %q döndü, beklenen boş", got)
	}
}
```

`telemetri_test.go`'nun başına `"net/http"` ve `"net/http/httptest"` importlarını ekle.

- [ ] **Step 17: Testlerin başarısız olduğunu doğrula**

Çalıştır: `go test ./internal/system/... -run TestIpTespitEt -v`
Beklenen: derleme hatası — `undefined: ipTespitEt`

- [ ] **Step 18: `ipTespitEt`'i yaz**

`telemetri.go`'ya ekle:

```go
// ipTespitEt — kendi genel IP'sini öğrenir (self-report, bağlantıdan otomatik
// tespit DEĞİL — bkz. spec'teki "IP tespiti" kararı). Hata = boş döner.
func ipTespitEt() string {
	cli := &http.Client{Timeout: 10 * time.Second}
	resp, err := cli.Get(ipUC())
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return ""
	}
	ip := strings.TrimSpace(string(b))
	if !ipBicimGecerliMi(ip) {
		return ""
	}
	return ip
}
```

- [ ] **Step 19: Testlerin geçtiğini doğrula**

Çalıştır: `go test ./internal/system/... -run TestIpTespitEt -v`
Beklenen: PASS (her iki test de)

- [ ] **Step 20: Commit**

```bash
git add internal/system/telemetri.go internal/system/telemetri_test.go
git commit -m "feat(system): IP kendi-beyan tespiti"
```

- [ ] **Step 21: `telemetriGonderVeri` için mock sunuculu başarısız test yaz**

`internal/system/telemetri_test.go`'a ekle:

```go
func TestTelemetriGonderVeri(t *testing.T) {
	var alinanYol, alinanMetod string
	var alinanGovde map[string]any
	sunucu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alinanYol = r.URL.Path + "?" + r.URL.RawQuery
		alinanMetod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&alinanGovde)
		w.WriteHeader(http.StatusOK)
	}))
	defer sunucu.Close()

	for k, v := range map[string]string{
		"PANEL_FIREBASE_UC":          sunucu.URL,
		"PANEL_FIREBASE_PROJE":       "test-proje",
		"PANEL_FIREBASE_API_ANAHTARI": "test-anahtar",
	} {
		old := os.Getenv(k)
		os.Setenv(k, v)
		defer os.Setenv(k, old)
	}

	err := telemetriGonderVeri("kimlik123", "0.9.7", "1.2.3.4", "almalinux", "tr", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("telemetriGonderVeri hata döndü: %v", err)
	}
	if alinanMetod != http.MethodPatch {
		t.Errorf("metod = %s, beklenen PATCH", alinanMetod)
	}
	if !strings.Contains(alinanYol, "/kurulumlar/kimlik123") {
		t.Errorf("yol beklenmedik: %s", alinanYol)
	}
	alanlar, _ := alinanGovde["fields"].(map[string]any)
	kimlik, _ := alanlar["kurulum_kimlik"].(map[string]any)
	if kimlik["stringValue"] != "kimlik123" {
		t.Errorf("gövdedeki kurulum_kimlik = %#v", alanlar["kurulum_kimlik"])
	}
}

func TestTelemetriGonderVeriAgHatasi(t *testing.T) {
	for k, v := range map[string]string{
		"PANEL_FIREBASE_UC":          "http://127.0.0.1:1", // hiçbir şey dinlemiyor
		"PANEL_FIREBASE_PROJE":       "test-proje",
		"PANEL_FIREBASE_API_ANAHTARI": "test-anahtar",
	} {
		old := os.Getenv(k)
		os.Setenv(k, v)
		defer os.Setenv(k, old)
	}
	if err := telemetriGonderVeri("kimlik123", "0.9.7", "", "", "", "2026-01-01T00:00:00Z"); err == nil {
		t.Error("ulaşılamayan sunucuda hata dönmeli (panel bunu YUTAR, ama iç fonksiyon hatayı görmeli)")
	}
}
```

`telemetri_test.go`'nun başına `"encoding/json"` importunu ekle.

- [ ] **Step 22: Testlerin başarısız olduğunu doğrula**

Çalıştır: `go test ./internal/system/... -run TestTelemetriGonderVeri -v`
Beklenen: derleme hatası — `undefined: telemetriGonderVeri`

- [ ] **Step 23: `telemetriGonderVeri`'yi yaz**

`telemetri.go`'ya ekle:

```go
// telemetriGonderVeri — asıl ağ çağrısı. Girdileri parametre olarak alır ki
// testler gerçek IP tespiti / dosya sistemi olmadan mock sunucularla
// çalışabilsin. Hata döner (telemetriGonder bu hatayı yutar).
func telemetriGonderVeri(kimlik, surum, ip, osAile, dil, ilkZaman string) error {
	govde, err := json.Marshal(firestoreDegerleriOlustur(kimlik, surum, ip, osAile, dil, ilkZaman))
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPatch, firestoreURL(kimlik), bytes.NewReader(govde))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	cli := &http.Client{Timeout: 20 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, telemetriGovdeSiniri))
	return nil
}
```

- [ ] **Step 24: Testlerin geçtiğini doğrula**

Çalıştır: `go test ./internal/system/... -run TestTelemetriGonderVeri -v`
Beklenen: PASS (her iki test de)

- [ ] **Step 25: Commit**

```bash
git add internal/system/telemetri.go internal/system/telemetri_test.go
git commit -m "feat(system): Firestore PATCH ağ çağrısı"
```

- [ ] **Step 26: `kurulumIlkZamani` ve dış yüz `telemetriGonder`'ı yaz (test yok — dosya sistemi yan etkisi, `KurulumKimligi()` ile aynı idiom, o da test edilmiyor)**

`telemetri.go`'ya ekle:

```go
// kurulumIlkZamani — bu kurulumun ilk telemetri gönderiminin zamanı. Yoksa
// üretir ve KALICI olarak diske yazar (KurulumKimligi() ile birebir aynı
// desen) — her PATCH'te AYNI değer gönderilir, böylece Firestore Rules'daki
// "ilk_kurulum_zamani immutable" kısıtı Go tarafından da doğal olarak sağlanır.
func kurulumIlkZamani() string {
	if b, err := os.ReadFile(ilkZamanYol); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	zaman := time.Now().UTC().Format(time.RFC3339)
	_ = os.MkdirAll(filepath.Dir(ilkZamanYol), 0o755)
	_ = os.WriteFile(ilkZamanYol, []byte(zaman+"\n"), 0o600)
	return zaman
}

// telemetriGonder — surumkontrol.go'daki goroutine döngüsünde surumGetir()
// ile aynı turda çağrılır (bkz. Task 2). Kapalıysa (PANEL_SURUM_KONTROL=0)
// çağrı yeri zaten hiç çağırmaz. Firebase henüz devreye alınmadıysa
// (telemetriHazirMi==false) sessizce no-op'tur.
func telemetriGonder() {
	if !telemetriHazirMi(firebaseProje(), firebaseAnahtar()) {
		return
	}
	kimlik := KurulumKimligi()
	if kimlik == "" {
		return // rastgele üretim başarısız oldu (bkz. KurulumKimligi)
	}
	_ = telemetriGonderVeri(
		kimlik,
		surumMevcutOku(),
		ipTespitEt(),
		osfam.Mevcut().ID,
		panelDili(),
		kurulumIlkZamani(),
	)
}
```

- [ ] **Step 27: Derlemeyi doğrula**

Çalıştır: `go build ./... && go vet ./internal/system/...`
Beklenen: hatasız

- [ ] **Step 28: Tüm paket testlerini çalıştır**

Çalıştır: `go test ./internal/system/... -v`
Beklenen: PASS (yeni testler dahil, mevcut `surumkontrol_test.go` testleri de bozulmamış)

- [ ] **Step 29: Commit**

```bash
git add internal/system/telemetri.go
git commit -m "feat(system): telemetriGonder dış yüzü + ilk kurulum zamanı kalıcılığı"
```

---

### Task 2: `surumkontrol.go`'a bağlama + lisans numarasının dışa açılması

**Files:**
- Modify: `internal/system/surumkontrol.go` (`SurumBaslat` goroutine döngüsü, `SurumBilgi` handler)
- Test: `internal/system/surumkontrol_test.go`

**Interfaces:**
- Consumes: `telemetriGonder()` (Task 1, aynı paket).
- Produces: `GET /system/surum` yanıtına yeni alan `kurulum_kimlik` eklenir (Task 4 frontend bunu okuyacak).

- [ ] **Step 1: `SurumBilgi`'nin `kurulum_kimlik` döndürdüğünü doğrulayan başarısız test yaz**

`internal/system/surumkontrol_test.go`'a ekle (dosyanın başına `"net/http"`, `"net/http/httptest"`, `"encoding/json"` importlarını ekle):

```go
func TestSurumBilgiKurulumKimligiIcerir(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/system/surum", nil)
	w := httptest.NewRecorder()
	SurumBilgi(w, r)

	var body map[string]any
	if err := json.NewDecoder(w.Result().Body).Decode(&body); err != nil {
		t.Fatalf("yanıt parse edilemedi: %v", err)
	}
	// KurulumKimligi() dosya yazımı başarısız olsa bile (test ortamında
	// /etc/sanalcp'e yazma izni olmayabilir) rastgele üretilmiş değeri
	// döndürür — bkz. surumkontrol.go, bu yüzden test izole dizin gerektirmez.
	kimlik, _ := body["kurulum_kimlik"].(string)
	if len(kimlik) < 16 {
		t.Errorf("kurulum_kimlik beklenmedik: %q", kimlik)
	}
}
```

- [ ] **Step 2: Testin başarısız olduğunu doğrula**

Çalıştır: `go test ./internal/system/... -run TestSurumBilgiKurulumKimligiIcerir -v`
Beklenen: FAIL — yanıt gövdesinde `kurulum_kimlik` alanı yok (`kimlik` boş string, `len(kimlik) < 16`)

- [ ] **Step 3: `SurumBilgi`'ye `kurulum_kimlik` ekle**

```go
func SurumBilgi(w http.ResponseWriter, r *http.Request) {
	surumMu.RLock()
	mevcut, buildTarihi := surumMevcut, surumBuildTarihi
	surumMu.RUnlock()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"mevcut":         mevcut,
		"build_tarihi":   buildTarihi,
		"kurulum_kimlik": KurulumKimligi(),
	})
}
```

- [ ] **Step 4: Testin geçtiğini doğrula**

Çalıştır: `go test ./internal/system/... -run TestSurumBilgiKurulumKimligiIcerir -v`
Beklenen: PASS

- [ ] **Step 5: Tüm paket testlerinin hâlâ geçtiğini doğrula (regresyon)**

Çalıştır: `go test ./internal/system/... -v`
Beklenen: PASS (tümü)

- [ ] **Step 6: Commit**

```bash
git add internal/system/surumkontrol.go internal/system/surumkontrol_test.go
git commit -m "feat(system): lisans numarasını /system/surum ucuna ekle"
```

- [ ] **Step 7: `telemetriGonder()`'ı goroutine döngüsüne bağla**

`internal/system/surumkontrol.go` içindeki `SurumBaslat`'ın goroutine'inde (mevcut kod):

```go
	go func() {
		time.Sleep(surumRastgele(10*time.Second, 60*time.Second))
		for {
			surumGetir()
			time.Sleep(surumPeriyot + surumRastgele(-2*time.Hour, 2*time.Hour))
		}
	}()
```

şu şekilde değiştir (yalnız `surumGetir()`'in altına `telemetriGonder()` eklenir, döngü yapısı aynı kalır):

```go
	go func() {
		time.Sleep(surumRastgele(10*time.Second, 60*time.Second))
		for {
			surumGetir()
			telemetriGonder()
			time.Sleep(surumPeriyot + surumRastgele(-2*time.Hour, 2*time.Hour))
		}
	}()
```

`SurumKontrolTetikle()` içindeki `go surumGetir()` satırını da giriş-tetikli tazelemenin telemetriyi de kapsaması için güncelle:

```go
	go func() {
		surumGetir()
		telemetriGonder()
	}()
```

- [ ] **Step 8: Derlemeyi doğrula**

Çalıştır: `go build ./... && go vet ./internal/system/...`
Beklenen: hatasız

- [ ] **Step 9: Tüm paket testlerini çalıştır (regresyon)**

Çalıştır: `go test ./internal/system/... -v`
Beklenen: PASS (tümü — `telemetriGonder()` bu testlerde `telemetriHazirMi()==false` olduğu için gerçek ağa çıkmaz, no-op kalır)

- [ ] **Step 10: Commit**

```bash
git add internal/system/surumkontrol.go
git commit -m "feat(system): telemetriGonder'ı sürüm kontrol döngüsüne bağla"
```

**Devreye Alma Notu (kod dışı, manuel adım):** `firebaseProjeVarsayilan` / `firebaseAnahtarVarsayilan` şu an boş — yani bu görev tamamlandığında telemetri kodu doğru ve test edilmiş durumda ama gerçek bir Firebase projesi kurulana kadar hiçbir ağ isteği atmaz (`telemetriHazirMi` güvenlik kapısı sayesinde). Gerçek dağıtımdan önce: (1) Firebase Console'da yeni proje + Firestore veritabanı aç, (2) Task 3'teki `firestore.rules`'u yayınla, (3) projenin Web API key'ini al, (4) `telemetri.go`'daki iki sabiti gerçek değerlerle doldur (veya üretim ortamında `PANEL_FIREBASE_PROJE`/`PANEL_FIREBASE_API_ANAHTARI` env var'larını set et) ve yeni bir sürüm yayınla.

---

### Task 3: `firestore.rules`

**Files:**
- Create: `firestore.rules`

**Interfaces:**
- Consumes: yok (statik dokümantasyon dosyası).
- Produces: yok (Go kodu tarafından okunmaz/derlenmez; proje sahibi bunu Firebase Console'a elle yayınlar — bkz. Task 2'nin "Devreye Alma Notu").

- [ ] **Step 1: Kuralları yaz**

`firestore.rules`:

```
rules_version = '2';

// SanalCP kurulum telemetrisi — bkz.
// docs/superpowers/specs/2026-08-24-panel-lisans-telemetri-design.md
//
// Okuma tamamen kapalı: yalnız proje sahibi Firebase Console / Admin SDK
// üzerinden görür. Yazma yalnız kendi doküman ID'sine (kurulum_kimlik),
// whitelist'li alanlarla. ilk_kurulum_zamani ilk yazımdan sonra değişmez.
service cloud.firestore {
  match /databases/{database}/documents {
    match /kurulumlar/{kurulumId} {
      allow read: if false;
      allow delete: if false;

      allow create: if request.resource.data.keys().hasOnly([
                        'kurulum_kimlik', 'mevcut_surum', 'kaynak_ip',
                        'osfam', 'panel_dili',
                        'ilk_kurulum_zamani', 'son_gorulme_zamani'
                      ])
                      && request.resource.data.kurulum_kimlik == kurulumId
                      && request.resource.data.kurulum_kimlik is string
                      && request.resource.data.kurulum_kimlik.size() >= 16;

      allow update: if request.resource.data.keys().hasOnly([
                        'kurulum_kimlik', 'mevcut_surum', 'kaynak_ip',
                        'osfam', 'panel_dili',
                        'ilk_kurulum_zamani', 'son_gorulme_zamani'
                      ])
                      && request.resource.data.kurulum_kimlik == resource.data.kurulum_kimlik
                      && request.resource.data.ilk_kurulum_zamani == resource.data.ilk_kurulum_zamani;
    }
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add firestore.rules
git commit -m "docs(firestore): kurulum telemetrisi güvenlik kuralları"
```

---

### Task 4: Frontend — lisans numarası kartı

**Files:**
- Create: `frontend/src/components/LisansBilgisi.tsx`
- Create: `frontend/src/i18n/locales/tr/LisansBilgisi.json`
- Create: `frontend/src/i18n/locales/en/LisansBilgisi.json`
- Modify: `frontend/src/pages/AraclarAyarlarPage.tsx`

**Interfaces:**
- Consumes: `GET /system/surum` → `{ mevcut: string; build_tarihi?: string; kurulum_kimlik: string }` (Task 2).
- Produces: `<LisansBilgisi />` React bileşeni, `AraclarAyarlarPage`'in "Sunucu bakımı" grid'ine eklenir.

- [ ] **Step 1: `LisansBilgisi.tsx`'i yaz**

`frontend/src/components/LisansBilgisi.tsx`:

```tsx
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'

type SurumBilgi = { mevcut: string; build_tarihi?: string; kurulum_kimlik: string }

export default function LisansBilgisi() {
  const { t } = useTranslation(['LisansBilgisi'])
  const [bilgi, setBilgi] = useState<SurumBilgi | null>(null)
  const [hata, setHata] = useState('')
  const [kopyalandi, setKopyalandi] = useState(false)

  const yukle = useCallback(async () => {
    try {
      const { data } = await api.get<SurumBilgi>('/system/surum')
      setBilgi(data)
    } catch (e) {
      setHata(apiHata(e, t('LisansBilgisi:load_failed')))
    }
  }, [t])

  useEffect(() => { void yukle() }, [yukle])

  async function kopyala() {
    if (!bilgi?.kurulum_kimlik) return
    await navigator.clipboard.writeText(bilgi.kurulum_kimlik)
    setKopyalandi(true)
    setTimeout(() => setKopyalandi(false), 2000)
  }

  return (
    <div className="h-full rounded-2xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900/60">
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-300">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} className="h-5 w-5" aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 5.25a3 3 0 0 1 3 3m3 0a6 6 0 0 1-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1 1 21.75 8.25Z" />
          </svg>
        </div>
        <div className="min-w-0 flex-1">
          <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('LisansBilgisi:label')}</span>
          <p className="mt-1 text-xs leading-relaxed text-slate-500 dark:text-slate-400">{t('LisansBilgisi:desc')}</p>

          {hata && <div className="mt-2 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">{hata}</div>}

          {bilgi && (
            <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center">
              <code className="flex-1 truncate rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-700 dark:border-slate-800 dark:bg-slate-950/60 dark:text-slate-300">
                {bilgi.kurulum_kimlik}
              </code>
              <button
                type="button"
                onClick={kopyala}
                className="rounded-lg border border-slate-200 px-3 py-2 text-xs font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-800 dark:text-slate-300 dark:hover:bg-slate-800"
              >
                {kopyalandi ? t('LisansBilgisi:copied') : t('LisansBilgisi:copy')}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 2: i18n dosyalarını yaz**

`frontend/src/i18n/locales/tr/LisansBilgisi.json`:

```json
{
  "label": "Lisans Numarası",
  "desc": "Bu kurulumu tanımlayan benzersiz numara. Destek talebi açarken paylaşabilirsiniz.",
  "copy": "Kopyala",
  "copied": "Kopyalandı",
  "load_failed": "Lisans numarası okunamadı"
}
```

`frontend/src/i18n/locales/en/LisansBilgisi.json`:

```json
{
  "label": "License Number",
  "desc": "The unique identifier for this installation. You can share it when opening a support request.",
  "copy": "Copy",
  "copied": "Copied",
  "load_failed": "Could not read the license number"
}
```

- [ ] **Step 3: `AraclarAyarlarPage.tsx`'e bağla**

`frontend/src/pages/AraclarAyarlarPage.tsx`'in import bloğuna ekle (dosyanın en üstündeki diğer bileşen import'larının yanına):

```tsx
import LisansBilgisi from '@/components/LisansBilgisi'
```

"Sunucu bakımı" grid'ine (`<PanelGuncelleme />`, `<HostnameAyari />` vb.'nin bulunduğu `<div className="grid ...">` bloğu) ekle:

```tsx
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          <PanelGuncelleme />
          <HostnameAyari />
          <PanelDomain />
          <NameserverAyari />
          <SunucuOptimize />
          <OturumBostaAyari />
          <LisansBilgisi />
        </div>
```

- [ ] **Step 4: Build ve lint'i doğrula**

Çalıştır: `cd frontend && npm run build && npm run lint`
Beklenen: hatasız (tip hatası yok, ESLint uyarısı yok)

- [ ] **Step 5: Panelde görsel doğrulama**

`cd frontend && npm run dev` ile geliştirme sunucusunu başlat, tarayıcıda `/araclar-ayarlar` sayfasına git, "Sunucu bakımı" bölümünde "Lisans Numarası" kartının göründüğünü, kimliğin yüklendiğini ve "Kopyala" butonunun çalıştığını doğrula.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/LisansBilgisi.tsx frontend/src/i18n/locales/tr/LisansBilgisi.json frontend/src/i18n/locales/en/LisansBilgisi.json frontend/src/pages/AraclarAyarlarPage.tsx
git commit -m "feat(frontend): araçlar/ayarlar sayfasına lisans numarası kartı"
```
