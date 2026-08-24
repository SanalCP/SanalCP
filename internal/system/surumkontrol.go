package system

// Sürüm kontrolü + güvenlik duyuru kanalı.
//
// NE YAPAR: Panel günde bir kez yayın manifestini (surum.json) okur; yeni sürüm
// veya kritik güvenlik duyurusu varsa arayüzde gösterir. Bugüne kadar müşterilere
// acil yama duyurusu yapacak HİÇBİR kanalımız yoktu — asıl kazanç bu.
//
// 🔒 GİZLİLİK — bilerek verilmiş kararlar:
//   - İstek DÜZ bir GET'tir: sorgu dizesi YOK, gövde YOK, özel başlık YOK.
//     Domain adı, hostname, IP, müşteri verisi, lisans — HİÇBİRİ gönderilmez.
//   - Kurulum kimliği ÜRETİLİR (/etc/sanalcp/kurulum-kimlik) ama
//     GÖNDERİLMEZ. Şu anki uç statik bir dosya; kimliği sayan kimse yok, dolayısıyla
//     göndermek karşılıksız bir kimlik sızıntısı olurdu. Kimliği şimdiden üretiyoruz
//     ki ileride sayım ucuna geçilirse kurulumların kararlı kimliği hazır olsun.
//   - PANEL_SURUM_KONTROL=0 → hiç istek atılmaz (goroutine hiç başlamaz).
//
// AĞ HATASI = SESSİZ. İnternet yoksa panel etkilenmez; durum "kontrol edilemedi"
// olarak kalır, hiçbir yerde hata patlatmaz.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"sanalcp/internal/httpx"
)

const (
	surumUCVarsayilan = "https://raw.githubusercontent.com/sanalcp/sanalcp/main/surum.json"
	surumKimlikYol    = "/etc/sanalcp/kurulum-kimlik"
	surumOnbellekYol  = "/opt/sanalcp/surum-onbellek.json"
	surumPeriyot      = 24 * time.Hour
	surumGovdeSiniri  = 64 << 10 // 64KB — manifest küçüktür; fazlası kötüye kullanım
)

// SurumYayin — yayın manifestinin (surum.json) şeması.
type SurumYayin struct {
	SonSurum    string `json:"son_surum"`
	Duyuru      string `json:"duyuru"`
	Kritik      bool   `json:"kritik"`
	YayinTarihi string `json:"yayin_tarihi"`
}

type surumOnbellek struct {
	Yayin      SurumYayin `json:"yayin"`
	SonKontrol time.Time  `json:"son_kontrol"`
	// Mevcut — önbellek yazıldığı ANDAKİ derlenmiş sürüm. Yükleme sırasında
	// o anki surumMevcut ile karşılaştırılır: FARKLIYSA (yani araya bir
	// güncelleme girmiş) önbellek ATLANIR — aksi hâlde bir önceki sürümün
	// (ör. 0.3.5) çekilmiş son_surum'u, YENİ kurulan sürümle (ör. 0.3.7)
	// karşılaştırılıp "kurulu: 0.3.7 → yeni: 0.3.5" gibi TERS/anlamsız bir
	// bildirim üretebilirdi — nitekim üretti (canlıda gözlemlendi).
	Mevcut string `json:"mevcut"`
}

var (
	surumMu          sync.RWMutex
	surumYayin       SurumYayin
	surumSon         time.Time
	surumHata        string
	surumMevcut      string
	surumBuildTarihi string
	surumAcik        bool
)

// surumKontrolAcikMi — PANEL_SURUM_KONTROL=0 ise tamamen kapalı.
func surumKontrolAcikMi() bool {
	v := strings.TrimSpace(os.Getenv("PANEL_SURUM_KONTROL"))
	return v != "0" && !strings.EqualFold(v, "false") && !strings.EqualFold(v, "hayir")
}

func surumUC() string {
	if v := strings.TrimSpace(os.Getenv("PANEL_SURUM_UC")); v != "" {
		return v
	}
	return surumUCVarsayilan
}

// KurulumKimligi — kalıcı, anonim kurulum kimliği. Yoksa üretir.
// SAF RASTGELE: hostname/IP/MAC'ten TÜRETİLMEZ — türetilmiş kimlik geri
// çözülebilir, yani anonim değildir.
func KurulumKimligi() string {
	if b, err := os.ReadFile(surumKimlikYol); err == nil {
		if s := strings.TrimSpace(string(b)); len(s) >= 16 {
			return s
		}
	}
	ham := make([]byte, 16)
	if _, err := rand.Read(ham); err != nil {
		return "" // üretilemedi — sessiz geç, kritik değil
	}
	kimlik := hex.EncodeToString(ham)
	_ = os.MkdirAll(filepath.Dir(surumKimlikYol), 0o755)
	// 0600 + root: kimlik müşteri sitelerinden okunabilir olmamalı
	_ = os.WriteFile(surumKimlikYol, []byte(kimlik+"\n"), 0o600)
	return kimlik
}

// surumTetikleCooldown: giriş-tetikli manuel kontrollerin GitHub'ı gereksiz
// yormaması için minimum aralık. Arka plan döngüsü zaten ~24 saatte bir
// çalışıyor; bu yalnızca "giriş anında hemen güncel göster" içindir — art
// arda birden çok admin/bayi girişi (ör. birkaç dakika içinde) GitHub'a her
// seferinde ayrı istek atmaz, en fazla cooldown süresinde bir kez.
const surumTetikleCooldown = 5 * time.Minute

// SurumKontrolTetikle — bir admin/bayi başarıyla giriş yaptığında çağrılır
// (bkz. internal/auth Login). Arka plandaki ~24 saatlik periyodu beklemeden
// GÜNCEL bir kontrol tetikler. Fire-and-forget (goroutine) — login yanıtını
// GECİKTİRMEZ, hata durumunda da login akışını etkilemez (surumGetir zaten
// ağ hatalarını sessizce yutuyor, bkz. dosya başındaki AĞ HATASI notu).
func SurumKontrolTetikle() {
	if !surumKontrolAcikMi() {
		return
	}
	surumMu.RLock()
	son := surumSon
	surumMu.RUnlock()
	if !surumTetikleGerekliMi(son, time.Now()) {
		return
	}
	go surumGetir()
}

// surumTetikleGerekliMi: saf karar fonksiyonu (test edilebilir) — son kontrol
// hiç yapılmamışsa (sıfır zaman) ya da cooldown'dan eskiyse yeni bir kontrol
// gerekir.
func surumTetikleGerekliMi(sonKontrol, simdi time.Time) bool {
	if sonKontrol.IsZero() {
		return true
	}
	return simdi.Sub(sonKontrol) >= surumTetikleCooldown
}

// SurumBaslat — arka plan sürüm kontrolünü başlatır. Kapalıysa hiç çalışmaz.
func SurumBaslat(mevcutSurum, buildTarihi string) {
	surumMu.Lock()
	surumMevcut = mevcutSurum
	surumBuildTarihi = buildTarihi
	surumAcik = surumKontrolAcikMi()
	surumMu.Unlock()

	// Kimliği kapalıyken de üret: ileride sayıma geçilirse kurulum kimliği hazır olur.
	_ = KurulumKimligi()

	if !surumKontrolAcikMi() {
		return
	}
	surumOnbellekYukle()

	go func() {
		// Açılışta rastgele 10-60sn gecikme: kurulumlar aynı anda vurup ucu
		// boğmasın (thundering herd). Statik CDN için bu serpiştirme yeterli;
		// dakikalarca beklemek panelin durumu geç göstermesine yol açardı.
		time.Sleep(surumRastgele(10*time.Second, 60*time.Second))
		for {
			surumGetir()
			// Periyoda ±2 saat serpiştirme — aynı sebep.
			time.Sleep(surumPeriyot + surumRastgele(-2*time.Hour, 2*time.Hour))
		}
	}()
}

func surumRastgele(min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
	if err != nil {
		return min
	}
	return min + time.Duration(n.Int64())
}

// surumGetir — manifesti çeker. DÜZ GET, sorgu dizesi yok.
func surumGetir() {
	cli := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodGet, surumUC(), nil)
	if err != nil {
		surumHataYaz("istek kurulamadı")
		return
	}
	// Sürüm bilgisi yalnız User-Agent'ta; sunucu tarafı yayın dağılımını
	// görmek isterse buradan görür, ayrı bir tanımlayıcı taşımaz.
	req.Header.Set("User-Agent", "SanalCP/"+surumMevcutOku())

	resp, err := cli.Do(req)
	if err != nil {
		surumHataYaz("ağa ulaşılamadı")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		surumHataYaz("uç HTTP " + resp.Status)
		return
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, surumGovdeSiniri))
	if err != nil {
		surumHataYaz("yanıt okunamadı")
		return
	}
	var y SurumYayin
	if err := json.Unmarshal(b, &y); err != nil {
		surumHataYaz("manifest çözümlenemedi")
		return
	}
	if strings.TrimSpace(y.SonSurum) == "" {
		surumHataYaz("manifestte sürüm yok")
		return
	}

	surumMu.Lock()
	surumYayin = y
	surumSon = time.Now()
	surumHata = ""
	surumMu.Unlock()
	surumOnbellekYaz()
}

func surumMevcutOku() string {
	surumMu.RLock()
	defer surumMu.RUnlock()
	return surumMevcut
}

func surumHataYaz(m string) {
	surumMu.Lock()
	surumHata = m
	surumMu.Unlock()
}

func surumOnbellekYaz() {
	surumMu.RLock()
	o := surumOnbellek{Yayin: surumYayin, SonKontrol: surumSon, Mevcut: surumMevcut}
	surumMu.RUnlock()
	if b, err := json.Marshal(o); err == nil {
		_ = os.MkdirAll(filepath.Dir(surumOnbellekYol), 0o755)
		_ = os.WriteFile(surumOnbellekYol, b, 0o644)
	}
}

// surumOnbellekYukle — restart sonrası son bilinen durumu geri yükler ki
// panel açılır açılmaz "bilinmiyor" göstermesin. AMA önbellek FARKLI bir
// derlenmiş sürüm tarafından yazıldıysa (araya bir güncelleme girmiş) hiç
// yüklenmez — bkz. surumOnbellek.Mevcut. Bu durumda panel yalnızca ilk
// arka plan taramasına (açılıştan 10-60sn sonra) veya girişte tetiklenen
// kontrole (bkz. SurumKontrolTetikle) kadar "kontrol edilmedi" gösterir;
// bu, eski/ters bir karşılaştırma göstermekten HER ZAMAN daha iyidir.
func surumOnbellekYukle() {
	b, err := os.ReadFile(surumOnbellekYol)
	if err != nil {
		return
	}
	var o surumOnbellek
	if json.Unmarshal(b, &o) != nil {
		return
	}
	surumMu.Lock()
	defer surumMu.Unlock()
	if !surumOnbellekGuvenilirMi(o.Mevcut, surumMevcut) {
		return
	}
	surumYayin = o.Yayin
	surumSon = o.SonKontrol
}

// surumOnbellekGuvenilirMi: saf karar fonksiyonu (test edilebilir) — önbellek
// FARKLI bir derlenmiş sürüm tarafından yazıldıysa (araya bir güncelleme
// girmiş) ya da bu alanı hiç içermeyen eski biçimli bir önbellekse (boş
// string) güvenilmez. Eski biçimli önbellekler bu düzeltmeden SONRAKİ ilk
// güncellemede otomatik olarak yeni biçimle değişecek.
func surumOnbellekGuvenilirMi(onbellekMevcut, suankiMevcut string) bool {
	return onbellekMevcut == suankiMevcut
}

// SurumKontrolYenile — operatör "şimdi kontrol et" derse. Kapalıysa istek atmaz.
func SurumKontrolYenile(w http.ResponseWriter, r *http.Request) {
	if !surumKontrolAcikMi() {
		httpx.WriteError(w, http.StatusConflict, "sürüm kontrolü kapalı (PANEL_SURUM_KONTROL=0)")
		return
	}
	surumGetir()
	SurumKontrolDurum(w, r)
}

// SurumKontrolDurum — arayüz için mevcut durum.
func SurumKontrolDurum(w http.ResponseWriter, r *http.Request) {
	surumMu.RLock()
	mevcut, y, son, hata, acik := surumMevcut, surumYayin, surumSon, surumHata, surumAcik
	buildTarihi := surumBuildTarihi
	surumMu.RUnlock()

	// Kasıtlı olarak SADECE eşitlik kıyası: sürüm etiketleri "0.3.0-f2" gibi
	// serbest biçimli; semver sıralaması yanlış pozitif üretir. Farklıysa
	// "yeni sürüm var" deriz — dev makinede ileri sürüm çalıştıran operatör
	// için yanıltıcı olabilir, bilinçli kabul.
	guncellemeVar := acik && y.SonSurum != "" && y.SonSurum != mevcut

	cevap := map[string]any{
		"acik":           acik,
		"mevcut":         mevcut,
		"build_tarihi":   buildTarihi,
		"son":            y.SonSurum,
		"guncelleme_var": guncellemeVar,
		"duyuru":         y.Duyuru,
		"kritik":         y.Kritik && guncellemeVar,
		"yayin_tarihi":   y.YayinTarihi,
		"hata":           hata,
	}
	if !son.IsZero() {
		cevap["son_kontrol"] = son.UTC().Format(time.RFC3339)
	}
	httpx.WriteJSON(w, http.StatusOK, cevap)
}

// SurumBilgi — footer için TÜM oturum açmış kullanıcılara (müşteri dahil)
// açık, minimal uç. SurumKontrolDurum'un aksine dış güncelleme/duyuru
// verisi taşımaz — yalnız BU kurulumun kendi sürüm ve derleme tarihi.
// Rol kısıtı yok: BayiVeUstu arkasındaki /system/surum-kontrol'e
// erişemeyen sıradan müşteri hesapları da footer'da sürümü görebilsin diye.
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
