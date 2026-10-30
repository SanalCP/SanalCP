package uyari

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"sanalcp/internal/monitor"
)

const (
	turAraligi   = 5 * time.Minute
	esZamanliWeb = 8
)

// Uretim: canlı kanal adresleri.
var Uretim = Gonderici{TelegramURL: "https://api.telegram.org", SMTPAddr: "127.0.0.1:25", HTTP: &http.Client{Timeout: 15 * time.Second}}

// Run: ilk tur panel açılışından 1 dk sonra, ardından 5 dakikada bir.
func Run(ctx context.Context, db *sql.DB) {
	t := time.NewTimer(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if err := Tur(ctx, db, Uretim, monitor.Probe, time.Now()); err != nil && ctx.Err() == nil {
			log.Printf("uyari: %v", err)
		}
		t.Reset(turAraligi)
	}
}

type anahtar struct {
	domainID int64
	kontrol  string
}

type kayit struct {
	durum   Durum
	ardisik int
}

type hedef struct {
	id                  int64
	alanAdi             string
	sslAktif            bool
	sslKaynak, sslBitis string
	yedek               string
}

// Tur: tüm aktif domain'leri bir kez kontrol eder, durumları kaydeder ve
// değişenleri tek mesajda bildirir. Gönderim hatası turu düşürmez (aksi halde
// aynı geçiş her turda tekrar bildirilirdi).
func Tur(ctx context.Context, db *sql.DB, g Gonderici, probe func(string) monitor.DomainHealth, simdi time.Time) error {
	ayar, err := AyarOku(ctx, db)
	if err != nil {
		return err
	}
	hedefler, err := hedefleriOku(ctx, db)
	if err != nil {
		return err
	}
	onceki, err := durumlariOku(ctx, db)
	if err != nil {
		return err
	}

	// Web probları ağ beklediği için paralel; sonuçlar domain sırasıyla işlenir.
	web := make([]monitor.DomainHealth, len(hedefler))
	var wg sync.WaitGroup
	sem := make(chan struct{}, esZamanliWeb)
	for i, h := range hedefler {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			web[i] = probe(h.alanAdi)
		}()
	}
	wg.Wait()

	var satirlar []string
	for i, h := range hedefler {
		k := anahtar{h.id, "web"}
		d, ardisik := WebDurum(web[i], onceki[k].ardisik)
		satir, err := isle(ctx, db, ayar.Dil, h.alanAdi, k, d, ardisik, true, onceki, simdi, web[i].DurumKodu)
		if err != nil {
			return err
		}
		satirlar = append(satirlar, satir...)

		var bitis *time.Time
		if t, err := time.Parse("2006-01-02", h.sslBitis); err == nil {
			bitis = &t
		}
		d, ok := SSLDurum(h.sslAktif, h.sslKaynak, bitis, simdi)
		gun := 0
		if bitis != nil {
			gun = kalanGun(*bitis, simdi)
		}
		if satir, err = isle(ctx, db, ayar.Dil, h.alanAdi, anahtar{h.id, "ssl"}, d, 0, ok, onceki, simdi, gun); err != nil {
			return err
		}
		satirlar = append(satirlar, satir...)

		d, ok = YedekDurum(h.yedek)
		if satir, err = isle(ctx, db, ayar.Dil, h.alanAdi, anahtar{h.id, "yedek"}, d, 0, ok, onceki, simdi, 0); err != nil {
			return err
		}
		satirlar = append(satirlar, satir...)
	}

	if len(satirlar) > 0 && ayar.Acik && ayar.kanalVar() {
		if err := g.Gonder(ctx, ayar, "SanalCP\n"+strings.Join(satirlar, "\n")); err != nil {
			log.Printf("uyari: bildirim gönderilemedi: %v", err)
		}
	}
	return nil
}

// isle: tek kontrolün durumunu kaydeder; bildirilecek geçiş varsa mesaj satırını döner.
func isle(ctx context.Context, db *sql.DB, dil, alanAdi string, k anahtar, d Durum, ardisik int, uygulanir bool,
	onceki map[anahtar]kayit, simdi time.Time, detay int) ([]string, error) {
	eski, var_ := onceki[k]
	if !uygulanir {
		if var_ {
			_, err := db.ExecContext(ctx, `DELETE FROM uyari_durumlari WHERE domain_id=? AND kontrol=?`, k.domainID, k.kontrol)
			return nil, err
		}
		return nil, nil
	}
	if var_ && eski.durum == d && eski.ardisik == ardisik {
		return nil, nil
	}
	// degisti_at önce atanır: ODKU atamaları soldan sağa değerlendirilir.
	if _, err := db.ExecContext(ctx, `INSERT INTO uyari_durumlari (domain_id,kontrol,durum,ardisik_hata,degisti_at) VALUES (?,?,?,?,?)
		ON DUPLICATE KEY UPDATE degisti_at=IF(durum=VALUES(durum),degisti_at,VALUES(degisti_at)), durum=VALUES(durum), ardisik_hata=VALUES(ardisik_hata)`,
		k.domainID, k.kontrol, string(d), ardisik, simdi); err != nil {
		return nil, err
	}
	// İlk gözlem sessizdir: kurulumda mevcut sorunlar için mesaj seli olmasın.
	if !var_ || eski.durum == d {
		return nil, nil
	}
	return []string{mesaj(dil, alanAdi, k.kontrol, d, detay)}, nil
}

func hedefleriOku(ctx context.Context, db *sql.DB) ([]hedef, error) {
	rows, err := db.QueryContext(ctx, `SELECT d.id, d.alan_adi, d.ssl_aktif, d.ssl_kaynak, COALESCE(DATE_FORMAT(d.ssl_bitis,'%Y-%m-%d'),''),
		COALESCE((SELECT b.dogrulama_durum FROM backups b WHERE b.domain_id=d.id ORDER BY b.id DESC LIMIT 1),'')
		FROM domains d WHERE d.askida=0 AND d.is_demo=0 ORDER BY d.id`)
	if err != nil {
		return nil, fmt.Errorf("domain listesi: %w", err)
	}
	defer rows.Close()
	var out []hedef
	for rows.Next() {
		var h hedef
		if err := rows.Scan(&h.id, &h.alanAdi, &h.sslAktif, &h.sslKaynak, &h.sslBitis, &h.yedek); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func durumlariOku(ctx context.Context, db *sql.DB) (map[anahtar]kayit, error) {
	rows, err := db.QueryContext(ctx, `SELECT domain_id, kontrol, durum, ardisik_hata FROM uyari_durumlari`)
	if err != nil {
		return nil, fmt.Errorf("uyarı durumları: %w", err)
	}
	defer rows.Close()
	out := map[anahtar]kayit{}
	for rows.Next() {
		var k anahtar
		var d string
		var a int
		if err := rows.Scan(&k.domainID, &k.kontrol, &d, &a); err != nil {
			return nil, err
		}
		out[k] = kayit{Durum(d), a}
	}
	return out, rows.Err()
}

var emoji = map[Durum]string{Ok: "🟢", Uyari: "🟡", Hata: "🔴"}

var metinler = map[string]map[string]string{
	"tr": {
		"web/hata": "site yanıt vermiyor", "web/ok": "site yeniden erişilebilir",
		"ssl/uyari": "SSL sertifikası %d gün içinde bitiyor", "ssl/hata": "SSL sertifikası bitti ya da %d gün içinde bitiyor", "ssl/ok": "SSL sertifikası yenilendi",
		"yedek/hata": "son yedeğin doğrulaması başarısız", "yedek/ok": "yedek doğrulaması düzeldi",
	},
	"en": {
		"web/hata": "site is not responding", "web/ok": "site is reachable again",
		"ssl/uyari": "SSL certificate expires in %d days", "ssl/hata": "SSL certificate expired or expires in %d days", "ssl/ok": "SSL certificate renewed",
		"yedek/hata": "latest backup failed verification", "yedek/ok": "backup verification recovered",
	},
}

func mesaj(dil, alanAdi, kontrol string, d Durum, detay int) string {
	m, ok := metinler[dil]
	if !ok {
		m = metinler["tr"]
	}
	sablon := m[kontrol+"/"+string(d)]
	metin := sablon
	switch {
	case strings.Contains(sablon, "%d"):
		metin = fmt.Sprintf(sablon, detay)
	case kontrol == "web" && d == Hata && detay > 0:
		metin = fmt.Sprintf("%s (HTTP %d)", sablon, detay)
	}
	return emoji[d] + " " + alanAdi + " — " + metin
}
