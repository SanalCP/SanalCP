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
	"net/url"
	"os"
	"strings"
	"time"
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
