package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
)

// VarsayilanGovdeSiniri — /api/ altındaki SIRADAN uçların istek gövdesi üst
// sınırı. Panelin JSON uçlarının tamamı birkaç KiB'lik gövdelerle çalışır;
// 2 MiB, en şişkin form (ör. özel vhost/nginx yapılandırması, dashboard düzeni)
// için bile fazlasıyla yeterli bir tavan bırakır.
//
// 🔴 GÜVENLİK: Bu sınır gelmeden önce ~95 JSON ucu gövdeyi SINIRSIZ okuyordu —
// json.NewDecoder(r.Body) tüm gövdeyi belleğe alır. Kimliği doğrulanmış bir
// bayi/müşteri, tek bir dev JSON alanıyla panel sürecini OOM'a sürükleyebilir,
// birkaç eşzamanlı istekle sunucuyu düşürebilirdi. Gerçekten büyük gövde
// bekleyen uçlar (dosya/arşiv/DB yükleme) ExtendBodyLimit ile kendi sınırlarını
// açar — istisna listesi dar ve açıkça görünür kalsın diye varsayılan düşüktür.
const VarsayilanGovdeSiniri = 2 << 20 // 2 MiB

type govdeAnahtar struct{}

// LimitBody, geçen her isteğin gövdesini n bayta sınırlayan bir middleware
// döner. Sınır aşılınca okuma *http.MaxBytesError ile hata verir; handler'lar
// bunu GovdeSinirAsildi ile 413'e çevirebilir.
//
// Orijinal (sarmalanmamış) gövde context'e konur; böylece büyük yükleme uçları
// ExtendBodyLimit ile kendi sınırlarını uygulayabilir. Sarmalanmış gövdeyi
// tekrar sarmak işe yaramazdı — iç içe iki MaxBytesReader'da DAİMA küçük olan
// sınır geçerli olur, yani 2 MiB'lik varsayılan 20 GiB'lik yüklemeyi keserdi.
func LimitBody(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil || r.Body == http.NoBody {
				next.ServeHTTP(w, r)
				return
			}
			orig := r.Body
			r = r.WithContext(context.WithValue(r.Context(), govdeAnahtar{}, orig))
			r.Body = http.MaxBytesReader(w, orig, n)
			next.ServeHTTP(w, r)
		})
	}
}

// ExtendBodyLimit, SADECE gerçekten büyük gövde bekleyen uçlarda çağrılır
// (dosya yükleme, arşiv/SQL içe aktarım, cPanel transfer). LimitBody'nin
// koyduğu varsayılan sarmalayıcıyı atıp orijinal gövdeyi n bayta sınırlar.
//
// LimitBody zincirde yoksa (ör. birim testi) mevcut gövde sarmalanır; bu
// durumda da sınır uygulanır, sadece "varsayılanı geçersiz kılma" kısmı
// anlamsızdır. ExtendDeadline ile aynı desen — ikisi birlikte çağrılmalıdır.
func ExtendBodyLimit(w http.ResponseWriter, r *http.Request, n int64) {
	orig, _ := r.Context().Value(govdeAnahtar{}).(io.ReadCloser)
	if orig == nil {
		orig = r.Body
	}
	if orig == nil || orig == http.NoBody {
		return
	}
	r.Body = http.MaxBytesReader(w, orig, n)
}

// GovdeSinirAsildi, hatanın gövde üst sınırının aşılmasından kaynaklanıp
// kaynaklanmadığını söyler. multipart ayrıştırıcı MaxBytesError'ı bazen
// sarmalayarak değil düz metne çevirerek döndürdüğü için metin kontrolü de
// yapılır (bkz. internal/files/files.go).
func GovdeSinirAsildi(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// ---- eşzamanlı yükleme sınırı ----

// EszamanliYuklemeSiniri — tek bir tenant'ın (kullanıcı/domain) aynı anda
// yürütebileceği büyük yükleme sayısı.
//
// 🔴 GÜVENLİK: Gövde başına üst sınır tek bir isteğin boyutunu kesiyor ama
// eşzamanlılığı kesmiyordu — bir müşteri 20 GiB'lik yüklemeyi 50 kez paralel
// başlatarak diski ve goroutine'leri tüketebilirdi. Sınır AŞILDIĞINDA istek
// beklemez, 429 ile hemen reddedilir: kuyruğa alma slow-DoS'u yalnızca
// goroutine düzeyinde yeniden üretirdi.
const EszamanliYuklemeSiniri = 3

var (
	yuklemeMu     sync.Mutex
	yuklemeSayaci = map[string]int{}
)

// YuklemeSlotAl, anahtar (tenant kimliği) için bir yükleme slotu ayırmayı
// dener. Başarılıysa slotu bırakan fonksiyonu ve true döner; kota doluysa
// (nil, false) döner ve çağıran 429 yazmalıdır.
func YuklemeSlotAl(anahtar string) (func(), bool) {
	yuklemeMu.Lock()
	defer yuklemeMu.Unlock()
	if yuklemeSayaci[anahtar] >= EszamanliYuklemeSiniri {
		return nil, false
	}
	yuklemeSayaci[anahtar]++
	var birKez sync.Once
	return func() {
		birKez.Do(func() {
			yuklemeMu.Lock()
			defer yuklemeMu.Unlock()
			if yuklemeSayaci[anahtar] <= 1 {
				delete(yuklemeSayaci, anahtar) // harita sınırsız büyümesin
				return
			}
			yuklemeSayaci[anahtar]--
		})
	}, true
}

// YuklemeSlotVeyaHata, YuklemeSlotAl'ın handler'lardan tek satırla
// çağrılabilen sarmalayıcısı: kota doluysa 429 yazıp (nil, false) döner.
func YuklemeSlotVeyaHata(w http.ResponseWriter, anahtar string) (func(), bool) {
	birak, ok := YuklemeSlotAl(anahtar)
	if !ok {
		WriteError(w, http.StatusTooManyRequests,
			"aynı anda en fazla 3 yükleme yapılabilir; mevcut yüklemeler bitince tekrar deneyin")
		return nil, false
	}
	return birak, true
}
