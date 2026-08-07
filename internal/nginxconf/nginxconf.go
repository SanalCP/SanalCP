// Package nginxconf, panel nginx vhost'unun KANONİK kaynağıdır.
//
// NEDEN BURADA: _panel.conf eskiden yalnız assets/nginx/ altında duruyordu ve
// sunucuya SADECE kurulum sırasında (scripts/sanalcp-install.sh) kopyalanıyordu.
// sanalcp-update nginx conf'larına hiç dokunmadığı için, conf'ta yapılan her
// güvenlik değişikliği (gövde sınırları, CSP sertleştirmesi) mevcut kurulumlara
// ASLA inmiyordu — yalnız yeni kurulumlar alıyordu. Dosya artık binary'ye gömülü
// olduğu için panel kendi kendine güncelleyebiliyor (bkz. provisioner.
// HealPanelVhostOnStartup) ve düzeltme, binary'nin indiği anda yayılıyor.
//
// assets/nginx/_panel.conf artık BURADAN ÜRETİLİR (scripts/package-release.sh);
// elle düzenlenmemeli. Tek kaynak bu dosyadır.
package nginxconf

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

//go:embed _panel.conf
var PanelConf string

// PanelConfYol: kurulu panel vhost'unun sunucudaki yeri.
const PanelConfYol = "/etc/nginx/conf.d/_panel.conf"

// BilinenHashler: bugüne kadar YAYINLANMIŞ _panel.conf sürümlerinin sha256'ları.
//
// Kurulu dosyanın hash'i bu kümedeyse dosya "el değmemiş" demektir ve panel onu
// güvenle kanonik sürümle değiştirebilir. Kümede yoksa admin dosyayı elle
// düzenlemiştir; o durumda ÜZERİNE YAZILMAZ (bkz. HealPanelVhostOnStartup) —
// yönetici emeğini sessizce silmek, güncel olmayan bir CSP'den daha kötüdür.
//
// 🔴 Her _panel.conf değişikliğinde, BİR ÖNCEKİ sürümün hash'i buraya eklenmeli;
// aksi hâlde o sürümü çalıştıran kurulumlar "özelleştirilmiş" sayılır ve
// güncellemeyi alamaz. Yeni hash'i şu komut verir:
//
//	git show <onceki-commit>:internal/nginxconf/_panel.conf | sha256sum
//
// (0.5.9 ve öncesinde dosya assets/nginx/_panel.conf yolundaydı.)
var BilinenHashler = map[string]string{
	// 0.5.5 – 0.5.9 arası değişmeden kalan sürüm (assets/nginx/_panel.conf).
	"8455894d0e5f0fc47875bb9cdb53911220de1dfa1499c61519e109975bbd09f5": "0.5.5-0.5.9",
	// 0.5.9 – 0.5.24 arası değişmeden kalan sürüm — standalone "http2 on;" nginx
	// 1.20 (AlmaLinux 9 AppStream) tarafından tanınmıyordu ("unknown directive"),
	// yerine listen satırlarına http2 parametresi eklendi (bkz. 0.5.25 duyurusu).
	"5c92ea24317f08fdd48014ca46d09a3c05a20db12aec4785262d8363cc12056c": "0.5.9-0.5.24",
}

// Hash, verilen içeriğin sha256'sını onaltılık olarak döner.
func Hash(icerik []byte) string {
	toplam := sha256.Sum256(icerik)
	return hex.EncodeToString(toplam[:])
}

// KanonikHash, gömülü (hedef) sürümün hash'idir.
func KanonikHash() string {
	return Hash([]byte(PanelConf))
}

// ElDegmemis, kurulu içeriğin bilinen bir yayın sürümüyle birebir aynı olup
// olmadığını söyler. Kanonik sürümün kendisi de "el değmemiş" sayılır (zaten
// güncel), böylece çağıran tek kontrolle "dokunulabilir mi" sorusunu yanıtlar.
func ElDegmemis(icerik []byte) bool {
	h := Hash(icerik)
	if h == KanonikHash() {
		return true
	}
	_, ok := BilinenHashler[h]
	return ok
}
