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
	"strings"
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
