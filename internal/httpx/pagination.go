package httpx

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

const (
	VarsayilanSayfaLimiti = 50
	AzamiSayfaLimiti      = 200
)

var ErrGecersizSayfalama = errors.New("geçersiz sayfalama parametresi")

type Sayfalama struct {
	Sayfa int
	Limit int
	Arama string
}

// SayfalamaAyristir, yalnız tanınan bir query anahtarı gerçekten mevcutsa
// sayfalama kipini açar. Böylece ilgisiz query parametreleri eski API şeklini
// değiştirmez.
func SayfalamaAyristir(r *http.Request) (Sayfalama, bool, error) {
	q := r.URL.Query()
	_, sayfaVar := q["sayfa"]
	_, limitVar := q["limit"]
	_, aramaVar := q["arama"]
	if !sayfaVar && !limitVar && !aramaVar {
		return Sayfalama{}, false, nil
	}

	p := Sayfalama{Sayfa: 1, Limit: VarsayilanSayfaLimiti, Arama: strings.TrimSpace(q.Get("arama"))}
	var err error
	if sayfaVar {
		p.Sayfa, err = strconv.Atoi(q.Get("sayfa"))
		if err != nil || p.Sayfa <= 0 {
			return Sayfalama{}, true, ErrGecersizSayfalama
		}
	}
	if limitVar {
		p.Limit, err = strconv.Atoi(q.Get("limit"))
		if err != nil || p.Limit <= 0 || p.Limit > AzamiSayfaLimiti {
			return Sayfalama{}, true, ErrGecersizSayfalama
		}
	}
	if p.Sayfa-1 > int(^uint(0)>>1)/p.Limit {
		return Sayfalama{}, true, ErrGecersizSayfalama
	}
	return p, true, nil
}

func (p Sayfalama) Offset() int { return (p.Sayfa - 1) * p.Limit }

type SayfaliYanit[T any] struct {
	Icerik      []T   `json:"icerik"`
	Sayfa       int   `json:"sayfa"`
	Limit       int   `json:"limit"`
	Toplam      int64 `json:"toplam"`
	ToplamSayfa int64 `json:"toplam_sayfa"`
}

func YeniSayfaliYanit[T any](icerik []T, p Sayfalama, toplam int64) SayfaliYanit[T] {
	toplamSayfa := int64(0)
	if toplam > 0 {
		toplamSayfa = (toplam-1)/int64(p.Limit) + 1
	}
	return SayfaliYanit[T]{
		Icerik: icerik, Sayfa: p.Sayfa, Limit: p.Limit,
		Toplam: toplam, ToplamSayfa: toplamSayfa,
	}
}

// LikeDeseni, LIKE jokerlerini kullanıcı metninde literal tutar. Sorgular
// bunu "ESCAPE '!'" ile kullanmalıdır.
func LikeDeseni(s string) string {
	s = strings.ReplaceAll(s, "!", "!!")
	s = strings.ReplaceAll(s, "%", "!%")
	s = strings.ReplaceAll(s, "_", "!_")
	return "%" + s + "%"
}
