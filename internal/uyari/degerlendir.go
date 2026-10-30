// Package uyari: domain sorunlarını arka planda izler ve durum değişince
// admin'e Telegram/e-posta ile bildirir.
package uyari

import (
	"math"
	"time"

	"sanalcp/internal/monitor"
)

// Durum: bir kontrolün sonucu — "ok" | "uyari" | "hata".
type Durum string

const (
	Ok    Durum = "ok"
	Uyari Durum = "uyari"
	Hata  Durum = "hata"
)

// SSLDurum: bool=false → kontrol bu domain'e uygulanmaz (SSL yok, self-signed ya da bitiş bilinmiyor).
func SSLDurum(aktif bool, kaynak string, bitis *time.Time, simdi time.Time) (Durum, bool) {
	if !aktif || kaynak == "self-signed" || bitis == nil {
		return "", false
	}
	kalan := kalanGun(*bitis, simdi)
	switch {
	case kalan <= 3:
		return Hata, true
	case kalan <= 14:
		return Uyari, true
	}
	return Ok, true
}

// WebDurum: tek seferlik ağ hıçkırığı alarm üretmesin diye hata ancak art arda 2. başarısızlıkta.
// 4xx bilerek olabilir (bakım sayfası, 403) — başarısız sayılmaz.
func WebDurum(h monitor.DomainHealth, oncekiArdisik int) (Durum, int) {
	if h.Erisilebilir && h.DurumKodu < 500 {
		return Ok, 0
	}
	ardisik := oncekiArdisik + 1
	if ardisik >= 2 {
		return Hata, ardisik
	}
	return Ok, ardisik
}

// YedekDurum: en son yedeğin dogrulama_durum değeri; sonuçlanmamış doğrulama uygulanmaz.
func YedekDurum(sonDogrulama string) (Durum, bool) {
	switch sonDogrulama {
	case "basarisiz":
		return Hata, true
	case "dogrulandi":
		return Ok, true
	}
	return "", false
}

// kalanGun: DomainHealthCard ile aynı yuvarlama (yukarı).
func kalanGun(bitis, simdi time.Time) int {
	return int(math.Ceil(bitis.Sub(simdi).Hours() / 24))
}
