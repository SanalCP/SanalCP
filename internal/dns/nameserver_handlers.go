package dns

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"sanalcp/internal/httpx"
	"sanalcp/internal/middleware"

	"github.com/go-chi/chi/v5"
)

// NSAyar: nameserver çifti ayarı (hem panel geneli hem bayi için aynı gövde).
type NSAyar struct {
	NS1 string `json:"ns1"`
	NS2 string `json:"ns2"`
	// Kaynak: değerlerin nereden geldiği — "bayi" | "panel" | "varsayilan".
	// Panel bunu kullanıcıya "şu an X kullanılıyor" diye göstermek için kullanır.
	Kaynak string `json:"kaynak,omitempty"`
	// Uyari: gerçek bir NS çifti tanımlı değilse doldurulur.
	Uyari string `json:"uyari,omitempty"`
	// Oneri1/Oneri2: ayarlanmamışsa panelin alan adından TAHMİN edilen değerler.
	// Yalnız öneridir; admin onaylamadan hiçbir zone'a yazılmaz.
	Oneri1 string `json:"oneri1,omitempty"`
	Oneri2 string `json:"oneri2,omitempty"`
}

func nsGovdeOku(r *http.Request) (string, string, error) {
	var a NSAyar
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&a); err != nil {
		return "", "", errGecersizGovde
	}
	ns1 := strings.ToLower(strings.TrimSpace(a.NS1))
	ns2 := strings.ToLower(strings.TrimSpace(a.NS2))
	if !GecerliNSHost(ns1) || !GecerliNSHost(ns2) {
		return "", "", errGecersizNS
	}
	if ns1 == ns2 {
		return "", "", errAyniNS
	}
	return ns1, ns2, nil
}

var (
	errGecersizGovde = &nsHata{"geçersiz istek gövdesi"}
	errGecersizNS    = &nsHata{"nameserver adları tam nitelikli alan adı olmalı (ör. ns1.ornek.com)"}
	errAyniNS        = &nsHata{"ns1 ve ns2 aynı olamaz"}
)

type nsHata struct{ m string }

func (e *nsHata) Error() string { return e.m }

// GetNameserver — GET /nameserver (AdminOnly)
// Panel geneli nameserver çifti + hangi kaynaktan geldiği.
func (h *Handlers) GetNameserver(w http.ResponseWriter, r *http.Request) {
	var out NSAyar
	if a, b, ok := panelNS(r.Context(), h.DB); ok {
		out.NS1, out.NS2, out.Kaynak = a, b, "panel"
	} else {
		out.Kaynak = "yok"
		out.Uyari = "Nameserver tanımlı değil — zone'lar geçici olarak ns1.<domain> " +
			"(vanity) kullanıyor ve müşteriler domainlerini panele yönlendiremez."
		if o1, o2, ok := OneriliNS(r.Context(), h.DB); ok {
			out.Oneri1, out.Oneri2 = o1, o2
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// PutNameserver — PUT /nameserver (AdminOnly)
func (h *Handlers) PutNameserver(w http.ResponseWriter, r *http.Request) {
	ns1, ns2, err := nsGovdeOku(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE panel_ayarlari SET ns1_hostname=?, ns2_hostname=? WHERE id=1`, ns1, ns2); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kaydedilemedi: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, NSAyar{NS1: ns1, NS2: ns2, Kaynak: "panel"})
}

// GetBayiNameserver — GET /bayi/nameserver (BayiVeUstu)
// Bayinin kendi white-label NS'leri; tanımlı değilse panel genelini gösterir.
func (h *Handlers) GetBayiNameserver(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "yetkisiz")
		return
	}
	var out NSAyar
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT ns1, ns2 FROM bayi_nameserver WHERE user_id=?`, c.UserID).Scan(&out.NS1, &out.NS2)
	if err == nil && GecerliNSHost(out.NS1) && GecerliNSHost(out.NS2) {
		out.Kaynak = "bayi"
		httpx.WriteJSON(w, http.StatusOK, out)
		return
	}
	if a, b, ok := panelNS(r.Context(), h.DB); ok {
		httpx.WriteJSON(w, http.StatusOK, NSAyar{NS1: a, NS2: b, Kaynak: "panel"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, NSAyar{Kaynak: "yok",
		Uyari: "Sunucu genelinde nameserver tanımlı değil; yöneticinize başvurun."})
}

// PutBayiNameserver — PUT /bayi/nameserver (BayiVeUstu)
//
// Bayi kendi alan adındaki nameserver'ları tanımlar. A kayıtlarını bayi KENDİ
// DNS'inde oluşturmalıdır — panel bunu doğrulayamaz (bayinin alan adı burada
// barınmıyor olabilir), bu yüzden yanıtta hatırlatma döner.
func (h *Handlers) PutBayiNameserver(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "yetkisiz")
		return
	}
	ns1, ns2, err := nsGovdeOku(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO bayi_nameserver(user_id, ns1, ns2) VALUES(?,?,?)
		 ON DUPLICATE KEY UPDATE ns1=VALUES(ns1), ns2=VALUES(ns2)`,
		c.UserID, ns1, ns2); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kaydedilemedi: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, NSAyar{NS1: ns1, NS2: ns2, Kaynak: "bayi",
		Uyari: ns1 + " ve " + ns2 + " için A kayıtlarını kendi DNS sağlayıcınızda " +
			"sunucu IP'sine yönlendirmeyi unutmayın; aksi halde müşteri domainleri çözülmez."})
}

// NSTasi — POST /dns/nameserver-tasi (AdminOnly)
//
// Mevcut domainlerin NS kayıtlarını ve SOA primary NS'ini güncel ortak
// nameserver çiftine taşır, zone'ları yeniden yazar.
//
// Neden ayrı bir uç: nameserver ayarı değiştiğinde (veya vanity modelden
// geçişte) DB'deki dns_records/dns_soa satırları ESKİ değerleri tutmaya devam
// eder; şablon yalnız YENİ domainlerde çalışır. Bu uç olmadan geçiş elle
// domain domain yapılırdı.
func (h *Handlers) NSTasi(w http.ResponseWriter, r *http.Request) {
	sonuc, err := NameserverTasi(r.Context(), h.DB)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sonuc)
}

// TasimaSonucu: NSTasi raporu.
type TasimaSonucu struct {
	Toplam    int      `json:"toplam"`
	Guncellen int      `json:"guncellenen"`
	Hatalar   []string `json:"hatalar"`
}

// NameserverTasi: tüm domainlerin NS/SOA kayıtlarını çözümlenen çifte eşitler.
// Idempotenttir — zaten doğru olan domainlere dokunmaz.
func NameserverTasi(ctx context.Context, db *sql.DB) (TasimaSonucu, error) {
	var sonuc TasimaSonucu
	rows, err := db.QueryContext(ctx, `SELECT id, alan_adi FROM domains ORDER BY id`)
	if err != nil {
		return sonuc, err
	}
	type kayit struct {
		id      int64
		alanAdi string
	}
	var liste []kayit
	for rows.Next() {
		var k kayit
		if err := rows.Scan(&k.id, &k.alanAdi); err == nil {
			liste = append(liste, k)
		}
	}
	rows.Close()

	for _, k := range liste {
		sonuc.Toplam++
		ns1, ns2 := NameserverCifti(ctx, db, k.id, k.alanAdi)
		degisti, err := nsKayitlariEsitle(ctx, db, k.id, k.alanAdi, ns1, ns2)
		if err != nil {
			sonuc.Hatalar = append(sonuc.Hatalar, k.alanAdi+": "+err.Error())
			continue
		}
		if !degisti {
			continue
		}
		sonuc.Guncellen++
		if zerr := WriteZone(ctx, db, k.id); zerr != nil {
			sonuc.Hatalar = append(sonuc.Hatalar, k.alanAdi+" zone: "+zerr.Error())
			log.Printf("dns nameserver-tasi WriteZone domain=%d: %v", k.id, zerr)
		}
	}
	return sonuc, nil
}

// nsKayitlariEsitle: bir domainin apex NS kayıtlarını tam olarak {ns1, ns2}
// yapar ve SOA primary NS'ini ns1'e çeker. Değişiklik olduysa true döner.
//
// Eski vanity model artıkları da temizlenir: müşteri domaininin altındaki
// ns1/ns2 A kayıtları yeni modelde anlamsızdır (nameserver sağlayıcının kendi
// alan adında yaşar) ve bırakılırsa DNS denetleyicilerinde kafa karıştırır.
func nsKayitlariEsitle(ctx context.Context, db *sql.DB, domainID int64, alanAdi, ns1, ns2 string) (bool, error) {
	var mevcut []string
	rows, err := db.QueryContext(ctx,
		`SELECT deger FROM dns_records WHERE domain_id=? AND tip='NS' AND ad='@'`, domainID)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err == nil {
			mevcut = append(mevcut, strings.ToLower(strings.TrimSuffix(d, ".")))
		}
	}
	rows.Close()

	istenen := map[string]bool{ns1: true, ns2: true}
	dogru := len(mevcut) == 2
	for _, m := range mevcut {
		if !istenen[m] {
			dogru = false
		}
	}
	// SOA primary
	var soaNS string
	_ = db.QueryRowContext(ctx,
		`SELECT primary_ns FROM dns_soa WHERE domain_id=?`, domainID).Scan(&soaNS)
	soaDogru := strings.EqualFold(strings.TrimSuffix(soaNS, "."), ns1)

	// Glue (in-zone A) kayıtları: nameserver bu zone'un içindeyse zorunlu,
	// dışındaysa artık gereksiz. Kararı GlueEsitle domain başına verir.
	var ipv4 string
	_ = db.QueryRowContext(ctx, `SELECT ipv4 FROM domains WHERE id=?`, domainID).Scan(&ipv4)
	glueDegisti, err := GlueEsitle(ctx, db, domainID, alanAdi, ipv4, ns1, ns2)
	if err != nil {
		return false, err
	}

	if dogru && soaDogru && !glueDegisti {
		return false, nil
	}

	if !dogru {
		if _, err := db.ExecContext(ctx,
			`DELETE FROM dns_records WHERE domain_id=? AND tip='NS' AND ad='@'`, domainID); err != nil {
			return false, err
		}
		for _, h := range []string{ns1, ns2} {
			if _, err := db.ExecContext(ctx,
				`INSERT INTO dns_records(domain_id, ad, tip, deger, ttl, oncelik, aktif)
				 VALUES(?, '@', 'NS', ?, 86400, 0, 1)`, domainID, h); err != nil {
				return false, err
			}
		}
	}
	if !soaDogru {
		if _, err := db.ExecContext(ctx,
			`UPDATE dns_soa SET primary_ns=? WHERE domain_id=?`, ns1, domainID); err != nil {
			return false, err
		}
	}
	return true, nil
}

// GetDomainNameserver — GET /domains/{id}/nameserver (MusteriScope)
//
// Müşteriye "domainini nereye yönlendireceğini" söyleyen uç. Bağlantı Bilgisi
// ekranında IPv4'ün yanında gösterilir: hosting sağlayıcılarının "domaininizi
// ns1.../ns2... adreslerine yönlendirin" yönergesinin panel karşılığı.
func (h *Handlers) GetDomainNameserver(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var alanAdi string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT alan_adi FROM domains WHERE id=?`, id).Scan(&alanAdi); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	ns1, ns2 := NameserverCifti(r.Context(), h.DB, id, alanAdi)
	out := NSAyar{NS1: ns1, NS2: ns2, Kaynak: "panel"}
	if _, _, ok := bayiNS(r.Context(), h.DB, id); ok {
		out.Kaynak = "bayi"
	} else if !NSAyarli(r.Context(), h.DB) {
		// Vanity geri düşüşü: bu değerler müşteriye VERİLEMEZ, çünkü her domain
		// için kayıt şirketinde glue record gerektirir.
		out.Kaynak = "yok"
		out.Uyari = "Sunucuda ortak nameserver tanımlı olmadığı için bu adresler kullanılamaz; " +
			"yöneticiniz Ayarlar > DNS bölümünden nameserver tanımlamalıdır."
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
