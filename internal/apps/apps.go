// Package apps: 1-tık uygulama kurulum çerçevesi (WordPress, PrestaShop, ...).
// Her uygulama türü Uygulama arayüzünü implement edip init() içinde Kaydet ile
// kendini kaydeder; ortak HTTP handler'lar (handlers.go) bu registry üzerinden çalışır.
package apps

import (
	"context"
	"sync"
)

// FormAlan: bir uygulama türünün kurulum formunda istediği tek bir alan
// (frontend'e /domains/{id}/apps/turler'dan dinamik form şeması olarak gider).
type FormAlan struct {
	Anahtar   string `json:"anahtar"`
	Etiket    string `json:"etiket"`
	Tur       string `json:"tur"` // "text" | "email" | "password"
	Zorunlu   bool   `json:"zorunlu"`
	YerTutucu string `json:"yer_tutucu,omitempty"`
}

// KurulumIstek: ortak katman (handlers.go) hedef dizini oluşturup DB'yi
// hazırladıktan SONRA Uygulama.Kur'a geçirdiği girdi. Hedef zaten var + chown'lanmış,
// DBAdi/DBKullanici/DBParola zaten oluşturulmuş bir MySQL veritabanına ait.
type KurulumIstek struct {
	DomainID    int64
	SK          string
	AlanAdi     string
	SSL         bool
	Hedef       string
	URL         string
	DBAdi       string
	DBKullanici string
	DBParola    string
	Alanlar     map[string]string
}

// KurulumSonuc: Uygulama.Kur'un başarılı dönüşü.
type KurulumSonuc struct {
	SiteURL        string
	AdminURL       string
	AdminKullanici string
	AdminParola    string
	Surum          string
	Ekstra         map[string]string
}

// Kurulum: tek bir kurulumun anlık durumu (liste taramalarında döner).
// Dizin alanı ORTAK KATMAN tarafından doldurulur — Uygulama.Bilgi implementasyonları
// bu alanı boş bırakmalı, dönen değer yok sayılır.
type Kurulum struct {
	Dizin         string `json:"dizin"`
	Surum         string `json:"surum"`
	SonSurum      string `json:"son_surum"`
	Durum         string `json:"durum"` // "guncel" | "eski" | "bilinmiyor"
	SiteURL       string `json:"site_url"`
	AdminURL      string `json:"admin_url"`
	KurulumTarihi string `json:"kurulum_tarihi"`
}

// Uygulama: her 1-tık uygulama türünün implement ettiği arayüz.
type Uygulama interface {
	Slug() string      // "wordpress", "prestashop" — route parametresi
	Ad() string         // görünen ad ("WordPress", "PrestaShop")
	DBOnEki() string     // db_accounts'ta kullanılacak DB adı/kullanıcı öneki (ör. "wp" → wp_xxxx / wpu_xxxx)
	MarkerDosya() string // kurulu tespiti için hedef dizindeki göreli yol (ör. "wp-config.php")
	FormAlanlari() []FormAlan
	GuncelleDesteklenir() bool

	Kur(ctx context.Context, i KurulumIstek) (KurulumSonuc, error)

	// Bilgi: url ortak katmanca hesaplanmış site kök adresidir (scheme+alanadi+altdizin);
	// driver kendi admin yolunu buna ekler (ör. url+"/wp-admin").
	Bilgi(ctx context.Context, sk, dizin, url string) (Kurulum, error)

	Guncelle(ctx context.Context, sk, dizin string) error // GuncelleDesteklenir()==false ise hiç çağrılmaz

	// DBAdiOku: dizin'deki config dosyasından DB adını okur VE bu ismin driver'ın
	// kendi ad-deseniyle (DBOnEki()+"_"+...) uyuştuğunu doğrular; ikisi de
	// sağlanmazsa bulundu=false döner (silmede yanlış DB'nin DROP edilmesine karşı
	// ilk katman — ikincisi apps.Handlers.Sil'deki db_accounts sahiplik kontrolü).
	DBAdiOku(dizin string) (dbAdi string, bulundu bool)
}

var (
	kayitliMu sync.RWMutex
	kayitli   = map[string]Uygulama{}
	sira      []string // eklenme sırası — Hepsi() deterministik dönsün diye
)

func Kaydet(u Uygulama) {
	kayitliMu.Lock()
	defer kayitliMu.Unlock()
	slug := u.Slug()
	if _, varMi := kayitli[slug]; !varMi {
		sira = append(sira, slug)
	}
	kayitli[slug] = u
}

func Bul(slug string) (Uygulama, bool) {
	kayitliMu.RLock()
	defer kayitliMu.RUnlock()
	u, ok := kayitli[slug]
	return u, ok
}

func Hepsi() []Uygulama {
	kayitliMu.RLock()
	defer kayitliMu.RUnlock()
	out := make([]Uygulama, 0, len(sira))
	for _, s := range sira {
		out = append(out, kayitli[s])
	}
	return out
}
