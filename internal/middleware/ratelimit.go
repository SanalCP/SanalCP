package middleware

// Giriş (login) kaba-kuvvet koruması — IP başına kayan pencere + kilitleme.
//
// NEDEN: Panel girişi sunucunun ROOT parolasıdır ve :8443 internete açıktır.
// Hız sınırı olmadan çevrimiçi kaba-kuvvet ile doğrudan tam sunucu ele geçirilebilir.
// nginx tarafında zaten bir istek-hızı limiti var (bkz. assets/nginx/_panel.conf,
// sanal_login zone) ama o saniyede istek sayısını sınırlar; bu middleware ayrıca
// BAŞARISIZ deneme sayısına göre kilitleyip kademeli gecikme ekler.
//
// TASARIM:
//   - Yalnız BAŞARISIZ (401) denemeler sayılır.
//   - Başarıda sayaç SIFIRLANMAZ: 2FA akışında parola doğru olunca 200 + iki_fa_gerekli
//     dönüyor; sıfırlasaydık saldırgan "parola-only" isteğiyle sayacı sürekli sıfırlayıp
//     TOTP kodunu sınırsız deneyebilirdi. Sayaç pencere dolunca kendiliğinden düşer.
//   - Politika: 15 dk içinde 5 başarısız deneme → o IP 30 dakika banlanır.
//   - Kademeli gecikme: her başarısız denemeden sonra istek yavaşlatılır (üst sınırlı).
//   - Kayıtlar periyodik budanır (bellek şişmesi/DoS önlenir).
//
// NOT: IP anahtarı httpx.ClientIP'ten gelir; bu, TRUSTED_PROXY_CIDRS ile
// tanımlanmış güvenilir bir vekilden gelmediği sürece X-Forwarded-For/X-Real-IP
// başlıklarını hiç okumaz, doğrudan r.RemoteAddr kullanır — aksi halde sahte
// bir başlıkla bu sınır IP-rotasyonuyla atlatılabilirdi.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"sanalcp/internal/httpx"
)

const (
	girisPencere  = 15 * time.Minute // başarısız denemelerin sayıldığı pencere
	girisMaxHata  = 5                // pencere içinde izin verilen başarısız deneme
	girisKilit    = 30 * time.Minute // aşılınca kilit (ban) süresi
	girisMaxGecik = 2 * time.Second  // kademeli gecikme üst sınırı
)

type girisKayit struct {
	hatalar    []time.Time
	kilitBitis time.Time
}

var (
	girisMu  sync.Mutex
	girisMap = map[string]*girisKayit{}
)

func init() { go girisTemizleyici() }

// girisTemizleyici — eski kayıtları budar (sınırsız bellek büyümesini önler).
func girisTemizleyici() {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for range t.C {
		simdi := time.Now()
		esik := simdi.Add(-(girisPencere + girisKilit))
		girisMu.Lock()
		for ip, k := range girisMap {
			bosVeEski := len(k.hatalar) == 0 || k.hatalar[len(k.hatalar)-1].Before(esik)
			if k.kilitBitis.Before(simdi) && bosVeEski {
				delete(girisMap, ip)
			}
		}
		girisMu.Unlock()

		// Hesap haritası da budanır: aksi hâlde saldırgan her istekte FARKLI bir
		// kullanıcı adı göndererek haritayı sınırsız büyütüp belleği tüketebilirdi
		// (kimlik doğrulama ÖNCESİ erişilebilen bir yüzey).
		hesapEsik := simdi.Add(-(hesapPencere + hesapKilit))
		hesapMu.Lock()
		for ad, k := range hesapMap {
			bosVeEski := len(k.hatalar) == 0 || k.hatalar[len(k.hatalar)-1].Before(hesapEsik)
			if k.kilitBitis.Before(simdi) && bosVeEski {
				delete(hesapMap, ad)
			}
		}
		hesapMu.Unlock()
	}
}

// girisDurum — pencere dışı hataları budar; (mevcut hata sayısı, kalan kilit süresi).
func girisDurum(ip string) (int, time.Duration) {
	simdi := time.Now()
	girisMu.Lock()
	defer girisMu.Unlock()
	k := girisMap[ip]
	if k == nil {
		return 0, 0
	}
	if simdi.Before(k.kilitBitis) {
		return girisMaxHata, k.kilitBitis.Sub(simdi)
	}
	kes := simdi.Add(-girisPencere)
	yeni := k.hatalar[:0]
	for _, t := range k.hatalar {
		if t.After(kes) {
			yeni = append(yeni, t)
		}
	}
	k.hatalar = yeni
	return len(k.hatalar), 0
}

func girisHataEkle(ip string) {
	simdi := time.Now()
	girisMu.Lock()
	defer girisMu.Unlock()
	k := girisMap[ip]
	if k == nil {
		k = &girisKayit{}
		girisMap[ip] = k
	}
	k.hatalar = append(k.hatalar, simdi)
	if len(k.hatalar) >= girisMaxHata {
		k.kilitBitis = simdi.Add(girisKilit)
		k.hatalar = nil
	}
}

// sureMetni — kalan süreyi insana okunur biçime çevirir (1800 sn yerine "30 dakika").
func sureMetni(sn int) string {
	if sn < 60 {
		return fmt.Sprintf("%d saniye", sn)
	}
	dk := (sn + 59) / 60
	return fmt.Sprintf("%d dakika", dk)
}

// girisDurumYazici — handler'ın yazdığı HTTP durum kodunu yakalar.
type girisDurumYazici struct {
	http.ResponseWriter
	kod int
}

func (d *girisDurumYazici) WriteHeader(k int) {
	d.kod = k
	d.ResponseWriter.WriteHeader(k)
}

// ---- Hesap bazlı sınır (dağıtık kaba-kuvvete karşı) ----
//
// 🔴 NEDEN GEREKLİ: yukarıdaki sınır yalnız IP başınadır. Bir botnet ya da bol
// IPv6 önekine sahip bir saldırgan her denemeyi başka bir adresten yaparsa IP
// sayacı hiç dolmaz ve TEK bir hesaba (pratikte root'a) yapılan çevrimiçi
// kaba-kuvvet SINIRSIZ olur. Bu yüzden hedef HESABA göre de sayılır.
//
// KİLİTLEME-DoS DENGESİ: hesap kilidi, saldırganın yöneticiyi dışarıda bırakmak
// için kasten yanlış parola göndermesine de yarayabilir. Bu yüzden eşik IP
// sınırından belirgin biçimde YÜKSEK (kazara tetiklenmez) ve süre KISA tutuldu:
// dağıtık saldırıyı sınırsızdan ~80 deneme/saat'e indirir, ama yöneticiyi en
// fazla 15 dakika bekletir.
const (
	hesapPencere = 15 * time.Minute
	hesapMaxHata = 20
	hesapKilit   = 15 * time.Minute
)

var (
	hesapMu  sync.Mutex
	hesapMap = map[string]*girisKayit{}
)

// hesapDurum / hesapHataEkle — girisDurum/girisHataEkle ile aynı kayan-pencere
// mantığı, ayrı eşik ve ayrı harita üzerinde.
func hesapDurum(ad string) time.Duration {
	simdi := time.Now()
	hesapMu.Lock()
	defer hesapMu.Unlock()
	k := hesapMap[ad]
	if k == nil {
		return 0
	}
	if simdi.Before(k.kilitBitis) {
		return k.kilitBitis.Sub(simdi)
	}
	kes := simdi.Add(-hesapPencere)
	yeni := k.hatalar[:0]
	for _, t := range k.hatalar {
		if t.After(kes) {
			yeni = append(yeni, t)
		}
	}
	k.hatalar = yeni
	return 0
}

func hesapHataEkle(ad string) {
	simdi := time.Now()
	hesapMu.Lock()
	defer hesapMu.Unlock()
	k := hesapMap[ad]
	if k == nil {
		k = &girisKayit{}
		hesapMap[ad] = k
	}
	k.hatalar = append(k.hatalar, simdi)
	if len(k.hatalar) >= hesapMaxHata {
		k.kilitBitis = simdi.Add(hesapKilit)
		k.hatalar = nil
	}
}

// girisMaxGovde — bir giriş isteği gövdesinin üst sınırı. Gerçek gövde birkaç
// yüz bayttır (kullanıcı/parola/kod); sınır, sayaç anahtarını çıkarmak için
// istek başına keyfi bellek ayırmayı engeller (kimlik doğrulama ÖNCESİ bir DoS
// yüzeyi).
const girisMaxGovde = 8 << 10

// hesapAdiOku — istek gövdesinden hedef kullanıcı adını okur ve gövdeyi
// handler'ın tekrar okuyabilmesi için GERİ KOYAR.
//
// asiri=true ise gövde sınırı aşmıştır; çağıran isteği reddeder. Gövdeyi kırpıp
// devam etmek YANLIŞ olurdu: kırpılmış JSON handler'da 400'e düşerdi ve istek
// hiçbir sayaca girmeden geçip giderdi.
func hesapAdiOku(r *http.Request) (ad string, asiri bool) {
	if r.Body == nil {
		return "", false
	}
	govde, err := io.ReadAll(io.LimitReader(r.Body, girisMaxGovde+1))
	_ = r.Body.Close()
	if len(govde) > girisMaxGovde {
		return "", true
	}
	r.Body = io.NopCloser(bytes.NewReader(govde))
	if err != nil {
		return "", false
	}
	var g struct {
		Kullanici string `json:"kullanici"`
	}
	if json.Unmarshal(govde, &g) != nil {
		return "", false // ad çözülemedi → yalnız IP sınırı işler
	}
	return strings.ToLower(strings.TrimSpace(g.Kullanici)), false
}

// GirisLimiti — kimlik doğrulama uçlarına kaba-kuvvet koruması (401 sayar).
// İki bağımsız sayaç uygular: kaynak IP (yoğun tek-kaynak saldırısı) ve hedef
// hesap (IP rotasyonlu dağıtık saldırı).
func GirisLimiti(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := httpx.ClientIP(r)
		adet, kalan := girisDurum(ip)
		if kalan > 0 {
			sn := int(kalan.Seconds()) + 1
			w.Header().Set("Retry-After", strconv.Itoa(sn))
			httpx.WriteError(w, http.StatusTooManyRequests,
				fmt.Sprintf("çok fazla başarısız giriş denemesi — %s sonra tekrar deneyin", sureMetni(sn)))
			return
		}
		hesap, asiri := hesapAdiOku(r)
		if asiri {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "giriş isteği çok büyük")
			return
		}
		if hesap != "" {
			if hk := hesapDurum(hesap); hk > 0 {
				sn := int(hk.Seconds()) + 1
				w.Header().Set("Retry-After", strconv.Itoa(sn))
				httpx.WriteError(w, http.StatusTooManyRequests,
					fmt.Sprintf("bu hesaba çok fazla başarısız giriş denendi — %s sonra tekrar deneyin", sureMetni(sn)))
				return
			}
		}
		if adet > 0 { // kademeli yavaşlatma
			g := time.Duration(adet) * 250 * time.Millisecond
			if g > girisMaxGecik {
				g = girisMaxGecik
			}
			time.Sleep(g)
		}
		dw := &girisDurumYazici{ResponseWriter: w, kod: http.StatusOK}
		next.ServeHTTP(dw, r)
		if dw.kod == http.StatusUnauthorized {
			girisHataEkle(ip)
			if hesap != "" {
				hesapHataEkle(hesap)
			}
		}
		// Sayaç BAŞARIDA da sıfırlanmaz — IP tarafındaki gerekçenin aynısı, hesap
		// ölçeğinde: 2FA akışında parola doğru olunca 200 + iki_fa_gerekli dönüyor.
		// Sıfırlasaydık, parolayı ele geçirmiş bir saldırgan her denemeden önce
		// "parola-only" istek atıp sayacı düşürerek TOTP kodunu IP rotasyonuyla
		// sınırsız deneyebilirdi. Pencere dolunca sayaç kendiliğinden düşer.
	})
}
