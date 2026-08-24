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
	ipUCVarsayilan           = "https://api.ipify.org?format=text"
	firestoreTabanVarsayilan = "https://firestore.googleapis.com/v1"
	// Firebase projesi 2026-08-24'te kuruldu (bkz. docs/superpowers/plans/
	// 2026-08-24-panel-lisans-telemetri.md, "Devreye Alma Notu"). Web API key
	// Firebase'in tasarımı gereği gizli değildir — erişim firestore.rules ile
	// sağlanır (bkz. o dosya). Boş bırakılırsa telemetriGonder() sessizce
	// no-op'tur (bkz. telemetriHazirMi).
	firebaseProjeVarsayilan   = "sanalcp-telemetri"
	firebaseAnahtarVarsayilan = "AIzaSyCfQf1pE-BPGT07HGRtnFvOrTP3rB4I8jk"

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
