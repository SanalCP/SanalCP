package middleware

// panel_ayarlari(id=1) tek satırı için kısa TTL'li süreç-içi önbellek.
//
// NEDEN: Bu tek satır, sıcak yolda (her /api/v1 isteği) üç kez okunuyordu:
//   - PanelErisimKisiti  — erisim_cidrleri + geçici erişim (panelerisim.go)
//   - PanelHizLimiti     — hiz_profili + limit/burst + istisnalar (panelhiz.go)
//   - RequireAuth        — oturum bosta limiti (auth.go içindeki JOIN)
//
// Ayar satırı nadiren değişir; TTL'li tek önbellek bu çarpanı kaldırır. TTL
// 2 sn olduğu için yöneticinin yaptığı bir ayar değişikliği insan ölçeğinde
// anında yansır; ayrı bir geçersiz kılma kanalı (yazıcı taraflarında invalide
// etme) gerektirmez ve böylece yazma paketleriyle bağımlılık oluşmaz.
//
// GÜVENLİK NOTU: Burada önbelleklenen kararların hiçbiri tek başına yetki
// vermez. Kimlik doğrulama her istekte users satırını TAZE okur (auth.go);
// erişim/hız kuralları ancak DB ayaktayken anlamlıdır — panel DB'si düşerse
// zaten bütün handler'lar isteği tamamlayamaz (auth dahil). Bu yüzden bayat
// veri sunmak bir güvenlik sınırını gevşetmez; DB erişilemezken bile son
// bilinen ayarla çalışmak, tüm istekleri 503'e boğmaktan daha tutarlıdır.
//
// YENİLEME: TTL dolunca ilk gelen istek DB'den yeniler; bu sırada gelen
// diğer isteklere kısa süreli bayat değer sunularak aynı anda
// N isteğin DB'ye yüklenmesi (stampede) önlenir. DB hatası önbelleğe
// ALINMAZ: son başarılı değer en fazla 30 sn kullanılabilir; sonra hata
// çağırana iletilir ve çağıran eski davranıştaki gibi fail-closed karar verir.

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	panelAyarTTL      = 2 * time.Second  // normal önbellek ömrü
	panelAyarMaxBayat = 30 * time.Second // geçici DB hatasında son bilinen değerin üst sınırı
)

// errPanelAyarDBYok: Init çağrılmadan (scopeDB nil) okuma denemesi.
var errPanelAyarDBYok = errors.New("panel_ayarlari önbelleği: DB bağlantısı yok")

type panelAyarOzet struct {
	ErisimHam   string // erisim_cidrleri
	GeciciCIDR  string // geçici erişim CIDR'i
	GeciciAktif bool   // geçici erişim penceresi açık mı
	HizProfil   string
	HizIstek    int
	HizBurst    int
	HizIstisna  string
	OturumBosta int // oturum_bosta_dakika
}

var (
	panelAyarMu          sync.Mutex
	panelAyarDeger       *panelAyarOzet
	panelAyarOkunan      time.Time // son başarılı DB okuması
	panelAyarYenileniyor bool      // bir istek şu an DB'den yeniliyor
	panelAyarBekle       chan struct{}
)

// panelAyarlariOku — önbelleği döndürür; TTL dolduysa tek istek yeniler,
// diğerleri o sırada bayat değeri kullanır. scopeDB yoksa hata döner.
func panelAyarlariOku(ctx context.Context) (*panelAyarOzet, error) {
	if scopeDB == nil {
		return nil, errPanelAyarDBYok
	}
	for {
		simdi := time.Now()
		panelAyarMu.Lock()
		if panelAyarDeger != nil && simdi.Sub(panelAyarOkunan) < panelAyarTTL {
			v := panelAyarDeger
			panelAyarMu.Unlock()
			return v, nil
		}
		if panelAyarYenileniyor {
			// Kullanılabilir bayat değer varsa istek beklemez. İlk dolumda veya
			// değer çok bayatsa tek yenileyicinin sonucunu bekler; böylece soğuk
			// başlangıçta sorgu yığını oluşmaz.
			if v := panelAyarDeger; v != nil && simdi.Sub(panelAyarOkunan) <= panelAyarMaxBayat {
				panelAyarMu.Unlock()
				return v, nil
			}
			bekle := panelAyarBekle
			panelAyarMu.Unlock()
			select {
			case <-bekle:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		panelAyarYenileniyor = true
		panelAyarBekle = make(chan struct{})
		break
	}
	panelAyarMu.Unlock()

	var o panelAyarOzet
	var geciciAktif int
	err := scopeDB.QueryRowContext(ctx, `
		SELECT COALESCE(erisim_cidrleri,''), COALESCE(gecici_erisim_cidr,''),
		       COALESCE(gecici_erisim_bitis > NOW(),0),
		       COALESCE(hiz_profili,''), COALESCE(hiz_istek_dakika,0), COALESCE(hiz_burst,0),
		       COALESCE(hiz_ip_istisnalari,''), COALESCE(oturum_bosta_dakika,0)
		  FROM panel_ayarlari WHERE id=1`).
		Scan(&o.ErisimHam, &o.GeciciCIDR, &geciciAktif,
			&o.HizProfil, &o.HizIstek, &o.HizBurst, &o.HizIstisna, &o.OturumBosta)
	if err != nil {
		// Hata önbelleğe alınmaz. Bayat değer varsa o sunulur (gerekçe üstte);
		// ilk dolumda hata olursa çağıran fail-closed kararını kendisi verir.
		panelAyarMu.Lock()
		panelAyarYenileniyor = false
		close(panelAyarBekle)
		eski := panelAyarDeger
		eskiOkunan := panelAyarOkunan
		panelAyarMu.Unlock()
		if eski != nil && time.Since(eskiOkunan) <= panelAyarMaxBayat {
			return eski, nil
		}
		return nil, err
	}
	o.GeciciAktif = geciciAktif == 1

	panelAyarMu.Lock()
	panelAyarDeger = &o
	panelAyarOkunan = time.Now()
	panelAyarYenileniyor = false
	close(panelAyarBekle)
	panelAyarMu.Unlock()
	return &o, nil
}
