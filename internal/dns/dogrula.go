// dogrula.go — Bir domainin DNS kayıtlarının GERÇEKTEN yayında olup olmadığını
// denetler (delegasyon, A, MX, SPF, DKIM, DMARC).
//
// 🔴 SORGULAR PUBLIC DNS'E YAPILIR, panelin kendi BIND'ine değil.
//
// Panel zone'u yerelde üretse bile domain başka bir sağlayıcıya (Cloudflare,
// kayıt şirketi vb.) delege edilmiş olabilir; o durumda yereldeki zone hiç
// kullanılmaz. "127.0.0.1'e sorup OK demek" tam da kullanıcının gördüğü
// kafa karışıklığını üretir: panel "kayıt var" der, dünya göremez.
// Bu yüzden doğrulama, dışarıdan bakan bir çözümleyici üzerinden yapılır.
package dns

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sanalcp/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// dogrulamaZamanAsimi: tüm kontrollerin toplam üst sınırı. Ölü bir resolver
// panel isteğini asılı bırakmasın.
const dogrulamaZamanAsimi = 12 * time.Second

// Durum değerleri (frontend renklendirmesi bunlara bakar).
const (
	DurumOK    = "ok"
	DurumUyari = "uyari"
	DurumHata  = "hata"
)

// Kontrol: tek bir DNS denetiminin sonucu.
type Kontrol struct {
	Anahtar  string `json:"anahtar"` // ns | a | mail_a | mx | spf | dkim | dmarc
	Baslik   string `json:"baslik"`
	Durum    string `json:"durum"`
	Beklenen string `json:"beklenen,omitempty"`
	Bulunan  string `json:"bulunan,omitempty"`
	Mesaj    string `json:"mesaj,omitempty"`
}

// DogrulamaSonucu: tüm kontroller + özet.
type DogrulamaSonucu struct {
	AlanAdi    string    `json:"alan_adi"`
	Kontroller []Kontrol `json:"kontroller"`
	OK         int       `json:"ok_sayisi"`
	Uyari      int       `json:"uyari_sayisi"`
	Hata       int       `json:"hata_sayisi"`
}

// Dogrula — GET /domains/{id}/dns/dogrula (MusteriScope)
func (h *Handlers) Dogrula(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var alanAdi, ipv4 string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT alan_adi, COALESCE(ipv4,'') FROM domains WHERE id=?`, id).Scan(&alanAdi, &ipv4); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	ctx, iptal := context.WithTimeout(r.Context(), dogrulamaZamanAsimi)
	defer iptal()
	httpx.WriteJSON(w, http.StatusOK, DogrulamaYap(ctx, h.DB, id, alanAdi, ipv4))
}

// DogrulamaYap: kontrolleri sırayla çalıştırır ve özetler.
func DogrulamaYap(ctx context.Context, db *sql.DB, domainID int64, alanAdi, ipv4 string) DogrulamaSonucu {
	s := DogrulamaSonucu{AlanAdi: alanAdi, Kontroller: []Kontrol{}}
	r := &net.Resolver{} // sistem çözümleyicisi (dışarıya bakan)

	ns1, ns2 := NameserverCifti(ctx, db, domainID, alanAdi)
	s.Kontroller = append(s.Kontroller,
		nsKontrol(ctx, r, alanAdi, ns1, ns2),
		aKontrol(ctx, r, alanAdi, ipv4),
		mailAKontrol(ctx, r, alanAdi, ipv4),
		mxKontrol(ctx, r, alanAdi),
		spfKontrol(ctx, r, alanAdi, ipv4),
		dkimKontrol(ctx, r, db, domainID, alanAdi),
		dmarcKontrol(ctx, r, alanAdi),
	)
	for _, k := range s.Kontroller {
		switch k.Durum {
		case DurumOK:
			s.OK++
		case DurumUyari:
			s.Uyari++
		default:
			s.Hata++
		}
	}
	return s
}

// nsKontrol: domain gerçekten bu panelin nameserver'larına delege edilmiş mi?
// Edilmemişse diğer kontroller yereldeki zone'u değil BAŞKA bir sağlayıcının
// kayıtlarını ölçüyor demektir — kullanıcının bunu bilmesi şart.
func nsKontrol(ctx context.Context, r *net.Resolver, alanAdi, ns1, ns2 string) Kontrol {
	k := Kontrol{Anahtar: "ns", Baslik: "Nameserver delegasyonu",
		Beklenen: ns1 + ", " + ns2}
	nsler, err := r.LookupNS(ctx, alanAdi)
	if err != nil || len(nsler) == 0 {
		k.Durum = DurumHata
		k.Mesaj = "Domainin NS kaydı okunamadı (domain kayıtlı/aktif mi?)."
		return k
	}
	var adlar []string
	bizde := 0
	for _, n := range nsler {
		ad := strings.ToLower(strings.TrimSuffix(n.Host, "."))
		adlar = append(adlar, ad)
		if ad == ns1 || ad == ns2 {
			bizde++
		}
	}
	k.Bulunan = strings.Join(adlar, ", ")
	switch {
	case bizde >= 2:
		k.Durum = DurumOK
	case bizde == 1:
		k.Durum = DurumUyari
		k.Mesaj = "Nameserver'lardan yalnız biri bu sunucuya ait; kayıt şirketinde ikisini de tanımlayın."
	default:
		// Bilinçli bir tercih olabilir (DNS'i Cloudflare vb. tutmak) ve kayıtlar
		// orada doğruysa her şey çalışır — bu yüzden HATA değil UYARI. Yine de
		// bilinmesi şart: panelden yapılan DNS düzenlemeleri yayına çıkmaz.
		k.Durum = DurumUyari
		k.Mesaj = "Domain bu panele delege edilmemiş; DNS başka bir sağlayıcıda. " +
			"Aşağıdaki kayıtlar oradan okundu — panelden yaptığınız DNS değişiklikleri yayına ÇIKMAZ, " +
			"kayıtları o sağlayıcıda güncellemeniz gerekir."
	}
	return k
}

func aKontrol(ctx context.Context, r *net.Resolver, alanAdi, ipv4 string) Kontrol {
	k := Kontrol{Anahtar: "a", Baslik: "A kaydı (" + alanAdi + ")", Beklenen: ipv4}
	adresler, err := r.LookupIPAddr(ctx, alanAdi)
	if err != nil || len(adresler) == 0 {
		k.Durum = DurumHata
		k.Mesaj = "Alan adı hiçbir IP'ye çözülmüyor."
		return k
	}
	k.Bulunan = ipListesi(adresler)
	if ipIcerir(adresler, ipv4) {
		k.Durum = DurumOK
	} else {
		k.Durum = DurumUyari
		k.Mesaj = "Alan adı bu sunucuya işaret etmiyor (başka bir yerde barınıyor olabilir)."
	}
	return k
}

// mailAKontrol: MX hedefi olan mail.<domain> çözülüyor mu? Bu kayıt bir WEB
// arayüzü için değil, e-postanın teslim adresi olarak gereklidir.
func mailAKontrol(ctx context.Context, r *net.Resolver, alanAdi, ipv4 string) Kontrol {
	host := "mail." + alanAdi
	k := Kontrol{Anahtar: "mail_a", Baslik: "A kaydı (" + host + ")", Beklenen: ipv4}
	adresler, err := r.LookupIPAddr(ctx, host)
	if err != nil || len(adresler) == 0 {
		k.Durum = DurumHata
		k.Mesaj = host + " çözülmüyor; MX bu adrese işaret ettiği için e-posta teslim edilemez."
		return k
	}
	k.Bulunan = ipListesi(adresler)
	if ipIcerir(adresler, ipv4) {
		k.Durum = DurumOK
	} else {
		k.Durum = DurumUyari
		k.Mesaj = host + " bu sunucuya işaret etmiyor."
	}
	return k
}

func mxKontrol(ctx context.Context, r *net.Resolver, alanAdi string) Kontrol {
	beklenen := "mail." + alanAdi
	k := Kontrol{Anahtar: "mx", Baslik: "MX kaydı", Beklenen: beklenen}
	mxler, err := r.LookupMX(ctx, alanAdi)
	if err != nil || len(mxler) == 0 {
		k.Durum = DurumHata
		k.Mesaj = "MX kaydı yok; bu domaine e-posta gönderilemez."
		return k
	}
	var adlar []string
	bulundu := false
	for _, m := range mxler {
		ad := strings.ToLower(strings.TrimSuffix(m.Host, "."))
		adlar = append(adlar, fmt.Sprintf("%d %s", m.Pref, ad))
		if ad == beklenen {
			bulundu = true
		}
	}
	k.Bulunan = strings.Join(adlar, ", ")
	if bulundu {
		k.Durum = DurumOK
	} else {
		k.Durum = DurumUyari
		k.Mesaj = "MX başka bir sunucuya işaret ediyor (harici e-posta servisi kullanıyorsanız normaldir)."
	}
	return k
}

// spfKontrol: apex TXT içinde v=spf1 var mı ve bu sunucuya yetki veriyor mu?
func spfKontrol(ctx context.Context, r *net.Resolver, alanAdi, ipv4 string) Kontrol {
	k := Kontrol{Anahtar: "spf", Baslik: "SPF", Beklenen: "v=spf1 ... ip4:" + ipv4 + " ..."}
	kayitlar, err := r.LookupTXT(ctx, alanAdi)
	if err != nil {
		k.Durum = DurumHata
		k.Mesaj = "TXT kayıtları okunamadı."
		return k
	}
	var spfler []string
	for _, t := range kayitlar {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=spf1") {
			spfler = append(spfler, t)
		}
	}
	if len(spfler) == 0 {
		k.Durum = DurumHata
		k.Mesaj = "SPF kaydı yok; gönderdiğiniz e-postalar spam'e düşebilir."
		return k
	}
	// Birden fazla SPF kaydı SPF'i GEÇERSİZ kılar (RFC 7208) — sessiz bir
	// teslim sorunudur, ayrıca uyarılmalı.
	if len(spfler) > 1 {
		k.Durum = DurumHata
		k.Bulunan = strings.Join(spfler, " | ")
		k.Mesaj = "Birden fazla SPF kaydı var; bu SPF'i tamamen geçersiz kılar. Tek kayıt bırakın."
		return k
	}
	k.Bulunan = spfler[0]
	low := strings.ToLower(spfler[0])
	// ip4:<ip> ya da "a"/"mx" mekanizmaları bu sunucuyu yetkilendirebilir.
	if (ipv4 != "" && strings.Contains(low, "ip4:"+ipv4)) ||
		alanKelimeIcerir(low, "a") || alanKelimeIcerir(low, "mx") {
		k.Durum = DurumOK
	} else {
		k.Durum = DurumUyari
		k.Mesaj = "SPF bu sunucuyu yetkilendirmiyor görünüyor (ip4/a/mx mekanizması yok)."
	}
	return k
}

// alanKelimeIcerir: SPF mekanizması boşlukla ayrılmış tam kelime olarak var mı?
// ("a" araması "ip4:..." içindeki harfe takılmamalı.)
func alanKelimeIcerir(spf, mekanizma string) bool {
	for _, parca := range strings.Fields(spf) {
		p := strings.TrimLeft(parca, "+-~?")
		if p == mekanizma || strings.HasPrefix(p, mekanizma+":") || strings.HasPrefix(p, mekanizma+"/") {
			return true
		}
	}
	return false
}

// dkimKontrol: yayındaki DKIM public key, panelin ürettiği anahtarla AYNI mı?
// "TXT var" demek yetmez — eski/yanlış bir anahtar imzaları doğrulanamaz kılar.
func dkimKontrol(ctx context.Context, r *net.Resolver, db *sql.DB, domainID int64, alanAdi string) Kontrol {
	selector := "default"
	_ = db.QueryRowContext(ctx,
		`SELECT dkim_selector FROM dns_template_meta WHERE id=1`).Scan(&selector)
	if strings.TrimSpace(selector) == "" {
		selector = "default"
	}
	host := selector + "._domainkey." + alanAdi
	k := Kontrol{Anahtar: "dkim", Baslik: "DKIM (" + selector + ")"}

	var pub string
	if err := db.QueryRowContext(ctx,
		`SELECT public_key FROM dkim_keys WHERE domain_id=? AND selector=?`,
		domainID, selector).Scan(&pub); err != nil || pub == "" {
		k.Durum = DurumUyari
		k.Mesaj = "Panelde bu domain için DKIM anahtarı üretilmemiş."
		return k
	}
	k.Beklenen = "p=" + kisaltAnahtar(pub)

	kayitlar, err := r.LookupTXT(ctx, host)
	if err != nil || len(kayitlar) == 0 {
		k.Durum = DurumHata
		k.Mesaj = host + " TXT kaydı yayında değil."
		return k
	}
	// Çözümleyici uzun TXT'leri parçalara bölebilir; hepsi birleştirilir.
	tam := strings.Join(kayitlar, "")
	k.Bulunan = "p=" + kisaltAnahtar(txtdenP(tam))
	if txtdenP(tam) == pub {
		k.Durum = DurumOK
	} else {
		k.Durum = DurumHata
		k.Mesaj = "Yayındaki DKIM anahtarı panelde üretilenle aynı değil; imzalar doğrulanamaz."
	}
	return k
}

// txtdenP: DKIM TXT içinden p= değerini çıkarır (boşluklar temizlenir —
// bazı DNS sağlayıcıları uzun anahtarı boşluklu saklar).
func txtdenP(txt string) string {
	for _, parca := range strings.Split(txt, ";") {
		p := strings.TrimSpace(parca)
		if strings.HasPrefix(p, "p=") {
			return strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "").Replace(p[2:])
		}
	}
	return ""
}

func kisaltAnahtar(s string) string {
	if len(s) <= 24 {
		return s
	}
	return s[:12] + "…" + s[len(s)-8:]
}

func dmarcKontrol(ctx context.Context, r *net.Resolver, alanAdi string) Kontrol {
	host := "_dmarc." + alanAdi
	k := Kontrol{Anahtar: "dmarc", Baslik: "DMARC", Beklenen: "v=DMARC1; p=..."}
	kayitlar, err := r.LookupTXT(ctx, host)
	if err != nil || len(kayitlar) == 0 {
		k.Durum = DurumUyari
		k.Mesaj = "DMARC kaydı yok; zorunlu değil ama teslim edilebilirliği artırır."
		return k
	}
	for _, t := range kayitlar {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=dmarc1") {
			k.Bulunan = t
			k.Durum = DurumOK
			return k
		}
	}
	k.Bulunan = strings.Join(kayitlar, " | ")
	k.Durum = DurumUyari
	k.Mesaj = "TXT kaydı var ama v=DMARC1 ile başlamıyor."
	return k
}

func ipListesi(adresler []net.IPAddr) string {
	var s []string
	for _, a := range adresler {
		s = append(s, a.IP.String())
	}
	return strings.Join(s, ", ")
}

func ipIcerir(adresler []net.IPAddr, ip string) bool {
	hedef := net.ParseIP(ip)
	if hedef == nil {
		return false
	}
	for _, a := range adresler {
		if a.IP.Equal(hedef) {
			return true
		}
	}
	return false
}
