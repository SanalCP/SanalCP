// Toplu yedekleme: "Tüm Domainleri Şimdi Yedekle" düğmesinin arkasındaki iş.
//
// NEDEN AYRI BİR UÇ: düğme eskiden /admin/backups/tick'e gidiyordu, o da
// scheduler'ın normal turunu çalıştırıyordu — yani YALNIZCA backup_hour'u o
// anki saate eşit, freq'i 'none' olmayan ve son yedeği 23 saatten eski
// domainleri işliyordu. Pratikte 03:00 dışında basıldığında hiçbir şey
// yedeklenmiyor, buna rağmen arayüz "tetiklendi" diyordu. Burası o filtreleri
// uygulamaz: kapsamdaki HER domaini şimdi yedekler.
package backups

import (
	"database/sql"
	"log"
	"net/http"
	"sync"
	"time"

	"sanalcp/internal/httpx"
	"sanalcp/internal/middleware"
)

// TopluDurum: çalışan/biten toplu yedekleme işinin anlık durumu.
//
// Yedekleme dakikalar sürebildiği için istek senkron bekletilemez; arayüz bu
// durumu yoklayarak ilerlemeyi gösterir. Aksi halde kullanıcı "tetiklendi"
// mesajından sonra işin bitip bitmediğini asla öğrenemez.
type TopluDurum struct {
	Calisiyor  bool     `json:"calisiyor"`
	Toplam     int      `json:"toplam"`
	Tamamlanan int      `json:"tamamlanan"`
	Basarisiz  int      `json:"basarisiz"`
	Suanki     string   `json:"suanki"`
	Hatalar    []string `json:"hatalar"`
	Baslangic  string   `json:"baslangic"`
	Bitis      string   `json:"bitis"`
	// Bende: durumu soran kullanıcı bu işi kendisi mi başlattı. false ise
	// domain adları (Suanki/Hatalar) gizlenir — bir bayi başka bir bayinin
	// domain adlarını görmemeli.
	Bende bool `json:"bende"`
}

var (
	topluMu  sync.Mutex
	topluDur TopluDurum
	topluSah int64 // işi başlatan kullanıcı
)

// HepsiniYedekle: POST /admin/backups/hepsini-yedekle
//
// Kapsamdaki tüm domainleri saat/frekans/son-yedek filtresi OLMADAN yedekler.
// İş arkaplana alınır, uç hemen döner; ilerleme TopluYedekDurum'dan izlenir.
func (h *Handlers) HepsiniYedekle(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "kimlik yok")
		return
	}

	topluMu.Lock()
	if topluDur.Calisiyor {
		topluMu.Unlock()
		httpx.WriteError(w, http.StatusConflict, "toplu yedekleme zaten çalışıyor")
		return
	}
	topluMu.Unlock()

	// Kapsam Ozet ile aynı: bayi yalnız kendi müşterilerinin domainlerini
	// yedekler. is_demo=0 — demo abonelikte yedek alınmaz (bkz. Create).
	kosul, arg := middleware.KapsamSQL(r, "d")
	ek := " AND d.is_demo = 0"
	if kosul == "" {
		ek = " WHERE d.is_demo = 0"
	}
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT d.id, d.alan_adi, d.sistem_kullanici,
		        COALESCE(d.backup_freq,'none'), COALESCE(d.backup_retention,7),
		        COALESCE(d.backup_manuel_retention,0)
		 FROM domains d`+kosul+ek+` ORDER BY d.alan_adi`, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "listelenemedi")
		return
	}
	var hedefler []dueDomain
	for rows.Next() {
		var d dueDomain
		if err := rows.Scan(&d.ID, &d.AlanAdi, &d.SK, &d.Freq, &d.Retention,
			&d.ManuelRetention); err != nil {
			continue
		}
		hedefler = append(hedefler, d)
	}
	rows.Close()

	if len(hedefler) == 0 {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "toplam": 0})
		return
	}

	// Durumu isteğin içinde kur: goroutine başlamadan önce Calisiyor=true
	// olmalı, yoksa arka arkaya iki tıklama iki iş başlatabilir.
	topluMu.Lock()
	if topluDur.Calisiyor { // yarış: araya başka bir istek girmiş olabilir
		topluMu.Unlock()
		httpx.WriteError(w, http.StatusConflict, "toplu yedekleme zaten çalışıyor")
		return
	}
	topluDur = TopluDurum{
		Calisiyor: true,
		Toplam:    len(hedefler),
		Baslangic: time.Now().Format("2006-01-02 15:04:05"),
	}
	topluSah = c.UserID
	topluMu.Unlock()

	go topluCalistir(h.DB, hedefler)

	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"ok": true, "toplam": len(hedefler)})
}

// topluCalistir: domainleri SIRAYLA yedekler.
//
// Paralel değil: her yedek bir tar + mysqldump, yani disk ve CPU'yu doyuran bir
// iş. Onlarca domaini aynı anda yedeklemek paneli barındıran sunucuyu ve
// üzerindeki siteleri yavaşlatır — sıralı çalışmak daha uzun sürer ama sunucuyu
// ayakta tutar.
func topluCalistir(db *sql.DB, hedefler []dueDomain) {
	defer func() {
		topluMu.Lock()
		topluDur.Calisiyor = false
		topluDur.Suanki = ""
		topluDur.Bitis = time.Now().Format("2006-01-02 15:04:05")
		topluMu.Unlock()
	}()

	for _, d := range hedefler {
		topluMu.Lock()
		topluDur.Suanki = d.AlanAdi
		topluMu.Unlock()

		err := runOneBackup(db, d, "Panelden toplu yedek")

		topluMu.Lock()
		if err != nil {
			topluDur.Basarisiz++
			// Hata listesi sınırlı tutulur: 200 domainli bir sunucuda hepsi
			// birden başarısız olursa yanıt gövdesi şişmesin.
			if len(topluDur.Hatalar) < 20 {
				topluDur.Hatalar = append(topluDur.Hatalar, d.AlanAdi+": "+err.Error())
			}
		} else {
			topluDur.Tamamlanan++
		}
		topluMu.Unlock()

		if err != nil {
			log.Printf("toplu yedek %s: %v", d.AlanAdi, err)
		}
		// Retention her hâlükârda uygulanır — bu düğme tip='oto' yedek ürettiği
		// için budama olmasa her tıklama diskte kalıcı bir dosya bırakırdı.
		if err := pruneOld(db, d.ID, d.SK, d.Retention); err != nil {
			log.Printf("toplu yedek retention %s: %v", d.AlanAdi, err)
		}
		if err := pruneManuel(db, d.ID, d.SK, d.ManuelRetention); err != nil {
			log.Printf("toplu yedek manuel retention %s: %v", d.AlanAdi, err)
		}
	}
	topluMu.Lock()
	log.Printf("toplu yedekleme bitti: %d başarılı, %d başarısız",
		topluDur.Tamamlanan, topluDur.Basarisiz)
	topluMu.Unlock()
}

// TopluYedekDurum: GET /admin/backups/toplu-durum
func (h *Handlers) TopluYedekDurum(w http.ResponseWriter, r *http.Request) {
	c := middleware.ClaimsFrom(r)
	topluMu.Lock()
	d := topluDur
	sahip := topluSah
	topluMu.Unlock()

	d.Bende = c != nil && c.UserID == sahip
	if !d.Bende {
		// Sayaçlar kalsın (başka bir işin çalıştığı görülsün), domain adları
		// gitsin: kapsam dışı bir kullanıcıya isim sızdırmaz.
		d.Suanki = ""
		d.Hatalar = nil
	}
	// Hatalar nil ise JSON'da null yerine [] dönsün — arayüz .length okuyor.
	if d.Hatalar == nil {
		d.Hatalar = []string{}
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}
