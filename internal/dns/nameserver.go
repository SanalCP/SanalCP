// nameserver.go — Ortak (paylaşımlı) nameserver çiftinin çözümlenmesi.
//
// Bir hosting sağlayıcısının müşterisine söylediği şey şudur: "domaininizi
// ns1.saglayici.com / ns2.saglayici.com adreslerine yönlendirin". Bunun
// çalışması için müşteri domaininin zone'undaki NS kayıtlarının O ORTAK
// nameserver'ları göstermesi gerekir.
//
// Panel eskiden her domain için `ns1.<müşteridomaini>` üretiyordu (vanity
// nameserver). Bu model her domain için kayıt şirketinde ayrı glue record
// zorunlu kılar ve paylaşımlı hostingde uygulanamaz — bkz. migrations/0063.
//
// ÇÖZÜMLEME SIRASI (ilk dolu olan kazanır):
//
//  1. Domainin bağlı olduğu BAYİ'nin white-label NS'leri (bayi_nameserver)
//  2. Panel geneli NS'ler (panel_ayarlari.ns1_hostname/ns2_hostname)
//  3. Panelin kendi alan adından türetilen ns1./ns2. (panel_ayarlari.ozel_domain)
//  4. Son çare: eski davranış (ns1.<domain>) — hiçbir ayar yapılmamışsa
//     mevcut kurulumlar bozulmasın diye korunur.
package dns

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
)

// reHostAdi: nameserver hostname doğrulaması. Zone dosyasına yazılacağı için
// katı olmalı — boşluk/newline/`$` bir zone satırını bozabilir veya
// $-yönerge enjeksiyonuna yol açabilir (bkz. dns.go gecerliKayitAlanlari).
var reHostAdi = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}\.)+[a-z]{2,63}$`)

// GecerliNSHost: verilen değer nameserver hostname olarak kullanılabilir mi?
func GecerliNSHost(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return len(s) <= 253 && reHostAdi.MatchString(s)
}

// NameserverCifti: bu domainin zone'unda yayınlanacak NS çiftini çözer.
//
// Hata döndürmez: DNS üretimi hiçbir koşulda NS'siz zone yazmamalı, bu yüzden
// her adımda makul bir geri düşüş vardır (bkz. paket başı sıralama).
func NameserverCifti(ctx context.Context, db *sql.DB, domainID int64, alanAdi string) (ns1, ns2 string) {
	if a, b, ok := bayiNS(ctx, db, domainID); ok {
		return a, b
	}
	if a, b, ok := panelNS(ctx, db); ok {
		return a, b
	}
	// Son çare: eski (vanity) davranış. Ayar yapılmamış kurulumlarda zone
	// üretimi NS'siz kalmasın diye korunur; panelde uyarı gösterilir.
	return "ns1." + alanAdi, "ns2." + alanAdi
}

// bayiNS: domainin sahibi bayinin white-label NS'leri.
//
// Sahiplik zinciri: domains.customer_id → customers.owner_user_id (bkz.
// migrations/0048). owner_user_id NULL ise domain doğrudan admin'e aittir ve
// bayi override'ı yoktur.
func bayiNS(ctx context.Context, db *sql.DB, domainID int64) (ns1, ns2 string, ok bool) {
	err := db.QueryRowContext(ctx, `
		SELECT bn.ns1, bn.ns2
		  FROM domains d
		  JOIN customers c        ON c.id = d.customer_id
		  JOIN bayi_nameserver bn ON bn.user_id = c.owner_user_id
		 WHERE d.id = ?`, domainID).Scan(&ns1, &ns2)
	if err != nil || !GecerliNSHost(ns1) || !GecerliNSHost(ns2) {
		return "", "", false
	}
	return strings.ToLower(ns1), strings.ToLower(ns2), true
}

// panelNS: panel geneli NS çifti. YALNIZ açıkça ayarlanmış değeri döner.
func panelNS(ctx context.Context, db *sql.DB) (ns1, ns2 string, ok bool) {
	var n1, n2 sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT ns1_hostname, ns2_hostname FROM panel_ayarlari WHERE id=1`).
		Scan(&n1, &n2); err != nil {
		return "", "", false
	}
	a, b := strings.ToLower(strings.TrimSpace(n1.String)), strings.ToLower(strings.TrimSpace(n2.String))
	if GecerliNSHost(a) && GecerliNSHost(b) {
		return a, b, true
	}
	return "", "", false
}

// OneriliNS: admin'e gösterilecek ÖNERİ (otomatik uygulanmaz).
//
// Panel alan adı genelde bir alt alan adıdır (cloud.saglayici.com); marka alan
// adı ilk etiketi atarak tahmin edilir. Kalan kısım tek etikete düşerse
// (saglayici.com → com) tahmin anlamsızdır, o durumda alan adı olduğu gibi
// kullanılır. Bu bir TAHMİNDİR — admin onaylamadan hiçbir yere yazılmaz.
func OneriliNS(ctx context.Context, db *sql.DB) (ns1, ns2 string, ok bool) {
	var ozel sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT ozel_domain FROM panel_ayarlari WHERE id=1`).Scan(&ozel); err != nil {
		return "", "", false
	}
	p := strings.ToLower(strings.TrimSpace(ozel.String))
	if !GecerliNSHost(p) {
		return "", "", false
	}
	if parcalar := strings.SplitN(p, ".", 2); len(parcalar) == 2 &&
		strings.Count(parcalar[1], ".") >= 1 {
		p = parcalar[1]
	}
	return "ns1." + p, "ns2." + p, true
}

// NSAyarli: panel geneli veya bayi düzeyinde GERÇEK bir nameserver çifti
// tanımlı mı? false ise zone'lar vanity (ns1.<domain>) geri düşüşünü kullanıyor
// demektir ve panel bunu admin'e uyarı olarak göstermelidir.
func NSAyarli(ctx context.Context, db *sql.DB) bool {
	_, _, ok := panelNS(ctx, db)
	return ok
}

// icZoneEtiketi: ns hostname'i alanAdi zone'unun İÇİNDE mi kalıyor?
// İçindeyse zone dosyasına yazılacak GÖRELİ etiketi döner ("ns1"), değilse
// ok=false. ns == zone ise "@" döner (apex; glue apex A kaydıdır, ayrı yönetilir).
func icZoneEtiketi(ns, alanAdi string) (string, bool) {
	n := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(ns), "."))
	z := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(alanAdi), "."))
	if n == "" || z == "" {
		return "", false
	}
	if n == z {
		return "@", true
	}
	if strings.HasSuffix(n, "."+z) {
		return strings.TrimSuffix(n, "."+z), true
	}
	return "", false
}

// GlueEsitle: zone'un KENDİ içinde kalan nameserver'lar için A (glue) kaydını
// oluşturur/günceller; zone dışındaki nameserver'lardan artakalan ns1/ns2 A
// kayıtlarını siler. Değişiklik olduysa true döner.
//
// 🔴 NEDEN GEREKLİ: bir zone'un NS kayıtları O ZONE'UN İÇİNDEKİ bir hosta
// işaret ediyorsa (in-bailiwick), BIND adres kaydını zone dosyasının İÇİNDE
// görmek zorundadır — yoksa zone hiç yüklenmez:
//
//	zone sanalcp.com/IN: NS 'ns1.sanalcp.com' has no address records (A or AAAA)
//	zone sanalcp.com/IN: not loaded due to errors.
//
// Sağlayıcının KENDİ alan adı (nameserver'ların yaşadığı zone) tam olarak bu
// durumdadır. Müşteri domainlerinde ise NS zone DIŞINDADIR (ns1.saglayici.com),
// orada in-zone A kaydı ne gerekir ne de anlamlıdır — bu yüzden karar
// domain başına, hostname'in zone'a göre konumuna bakılarak verilir.
func GlueEsitle(ctx context.Context, db *sql.DB, domainID int64, alanAdi, ipv4, ns1, ns2 string) (bool, error) {
	gerekli := map[string]bool{}
	for _, ns := range []string{ns1, ns2} {
		if et, ok := icZoneEtiketi(ns, alanAdi); ok && et != "@" {
			gerekli[et] = true
		}
	}

	degisti := false
	if ipv4 != "" {
		for et := range gerekli {
			var mevcut string
			err := db.QueryRowContext(ctx,
				`SELECT deger FROM dns_records WHERE domain_id=? AND ad=? AND tip='A' LIMIT 1`,
				domainID, et).Scan(&mevcut)
			switch {
			case errors.Is(err, sql.ErrNoRows):
				if _, e := db.ExecContext(ctx,
					`INSERT INTO dns_records(domain_id, ad, tip, deger, ttl, oncelik, aktif)
					 VALUES(?,?, 'A', ?, 3600, 0, 1)`, domainID, et, ipv4); e != nil {
					return degisti, e
				}
				degisti = true
			case err == nil && mevcut != ipv4:
				if _, e := db.ExecContext(ctx,
					`UPDATE dns_records SET deger=? WHERE domain_id=? AND ad=? AND tip='A'`,
					ipv4, domainID, et); e != nil {
					return degisti, e
				}
				degisti = true
			case err != nil:
				return degisti, err
			}
		}
	}

	// Artık gerekmeyen ns1/ns2 A kayıtlarını temizle (vanity model artığı).
	rows, err := db.QueryContext(ctx,
		`SELECT ad FROM dns_records WHERE domain_id=? AND tip='A' AND ad IN ('ns1','ns2')`, domainID)
	if err != nil {
		return degisti, err
	}
	var silinecek []string
	for rows.Next() {
		var ad string
		if err := rows.Scan(&ad); err == nil && !gerekli[ad] {
			silinecek = append(silinecek, ad)
		}
	}
	rows.Close()
	for _, ad := range silinecek {
		if _, e := db.ExecContext(ctx,
			`DELETE FROM dns_records WHERE domain_id=? AND tip='A' AND ad=?`, domainID, ad); e != nil {
			return degisti, e
		}
		degisti = true
	}
	return degisti, nil
}
