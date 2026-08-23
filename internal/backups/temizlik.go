// Toplu yedek temizliği: operatörün tek hamlede disk boşaltmasını sağlar.
//
// NEDEN AYRI BİR UÇ: retention yalnızca ileriye dönük çalışır — sınırı 3'e
// çekmek, geçmişte birikmiş 40 yedeği o domain bir sonraki kez yedeklenene
// kadar diskte tutar. Disk dolduğunda operatörün ihtiyacı olan şey "bundan
// sonra az tut" değil, "şimdi temizle"dir.
package backups

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sanalcp/internal/adlar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/middleware"
)

// Temizlik modları.
const (
	ModOto    = "oto"    // tüm otomatik yedekler
	ModManuel = "manuel" // tüm manuel (elle alınmış) yedekler
	ModGun    = "gun"    // son N gün DIŞINDA kalan her yedek (tip farketmez)
)

type TemizlikIstek struct {
	Mod string `json:"mod"`
	// Gun: mod=="gun" için KORUNACAK gün sayısı. "Son 3 gün hariç tümünü sil"
	// isteği Gun=3 demektir.
	Gun int `json:"gun"`
	// Onizleme: true ise hiçbir şey silinmez, yalnızca kaç dosya/kaç bayt
	// etkileneceği döner. Arayüz onay kutusunda gerçek sayıyı gösterebilsin diye
	// var — "emin misiniz?" sorusu somut bir rakam içermiyorsa onay değildir.
	Onizleme bool `json:"onizleme"`
}

type TemizlikSonuc struct {
	Sayi     int   `json:"sayi"`
	BoyutB   int64 `json:"boyut_b"`
	Domain   int   `json:"domain"`
	Onizleme bool  `json:"onizleme"`
}

// temizlikAday: silinmeye aday tek bir dosya.
type temizlikAday struct {
	BackupID  int64 // 0 => DB'de kaydı yok (yetim dosya)
	Dosya     string
	UzakDurum string
	BoyutB    int64
}

// Temizle: POST /admin/backups/temizle
//
// Kapsam Ozet ile aynı: admin tümünü, bayi yalnız kendi müşterilerinin
// domainlerini temizler. Aksi halde bayi paneldeki butonla tüm sunucunun
// yedeklerini silebilirdi.
func (h *Handlers) Temizle(w http.ResponseWriter, r *http.Request) {
	var req TemizlikIstek
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if req.Mod != ModOto && req.Mod != ModManuel && req.Mod != ModGun {
		httpx.WriteError(w, http.StatusBadRequest, "mod: oto|manuel|gun")
		return
	}
	// mod=gun için 0 kabul edilmez: 0 "hiçbir günü koruma", yani tek çağrıda
	// sunucudaki her yedeği silmek olurdu. Böyle bir işlem, kullanıcının
	// seçtiği "son N günü koru" düğmesinden yanlışlıkla türeyemeyecek kadar
	// yıkıcı — istenirse oto+manuel ayrı ayrı çalıştırılır.
	if req.Mod == ModGun && (req.Gun < 1 || req.Gun > 365) {
		httpx.WriteError(w, http.StatusBadRequest, "gun: 1-365")
		return
	}

	kosul, arg := middleware.KapsamSQL(r, "d")
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT d.id, d.sistem_kullanici FROM domains d`+kosul, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "listelenemedi")
		return
	}
	type hedef struct {
		ID int64
		SK string
	}
	var hedefler []hedef
	for rows.Next() {
		var t hedef
		if err := rows.Scan(&t.ID, &t.SK); err != nil {
			continue
		}
		// sk guard: yol her zaman BackupRoot/c_* altında kalmalı.
		if !adlar.SKGecerli(t.SK) {
			continue
		}
		hedefler = append(hedefler, t)
	}
	rows.Close()

	var esik time.Time
	if req.Mod == ModGun {
		esik = time.Now().AddDate(0, 0, -req.Gun)
	}

	domainIDs := make([]int64, len(hedefler))
	for i, t := range hedefler {
		domainIDs[i] = t.ID
	}
	bilinenTumu, err := tumBilinenKayitlar(r.Context(), h.DB, domainIDs)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "yedek kayıtları okunamadı")
		return
	}

	var silinecekIDler []int64
	sonuc := TemizlikSonuc{Onizleme: req.Onizleme}
	for _, t := range hedefler {
		adaylar, err := temizlikAdaylari(t.ID, t.SK, req.Mod, esik, bilinenTumu[t.ID])
		if err != nil {
			log.Printf("backup temizlik domain=%d: %v", t.ID, err)
			continue
		}
		if len(adaylar) == 0 {
			continue
		}
		sonuc.Domain++
		for _, a := range adaylar {
			sonuc.Sayi++
			sonuc.BoyutB += a.BoyutB
			if req.Onizleme {
				continue
			}
			_ = os.Remove(filepath.Join(BackupRoot, t.SK, a.Dosya))
			if a.BackupID != 0 {
				deleteRemoteBestEffort(h.DB, t.ID, a.Dosya, a.UzakDurum)
				silinecekIDler = append(silinecekIDler, a.BackupID)
			}
		}
	}
	if len(silinecekIDler) > 0 {
		if err := topluSil(r.Context(), h.DB, silinecekIDler); err != nil {
			log.Printf("backup temizlik: DB kayıtları silinemedi: %v", err)
		}
	}
	if !req.Onizleme {
		log.Printf("backup temizlik: mod=%s gun=%d domain=%d dosya=%d boyut=%d",
			req.Mod, req.Gun, sonuc.Domain, sonuc.Sayi, sonuc.BoyutB)
	}
	httpx.WriteJSON(w, http.StatusOK, sonuc)
}

// dbKayit: backups tablosundaki bir yedek satırının temizlik için gereken alanları.
type dbKayit struct {
	ID        int64
	Tip       string
	UzakDurum string
	BoyutB    int64
	Olusturma int64
}

// tumBilinenKayitlar: verilen domainlerin TÜM backups kayıtlarını TEK sorguda
// çeker ve domain_id'ye göre gruplar. domainID başına ayrı SELECT atmak yerine
// (N+1) burada toplu okunur; Temizle bunu bir kez çağırır, temizlikAdaylari
// artık DB'ye dokunmaz.
func tumBilinenKayitlar(ctx context.Context, db *sql.DB, domainIDs []int64) (map[int64]map[string]dbKayit, error) {
	out := map[int64]map[string]dbKayit{}
	if len(domainIDs) == 0 {
		return out, nil
	}
	placeholders := strings.Repeat("?,", len(domainIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(domainIDs))
	for i, id := range domainIDs {
		args[i] = id
	}
	rows, err := db.QueryContext(ctx,
		`SELECT domain_id, id, tip, dosya, uzak_durum, boyut_b, UNIX_TIMESTAMP(created_at)
		 FROM backups WHERE domain_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var domainID int64
		var k dbKayit
		var dosya string
		var ts sql.NullInt64
		if err := rows.Scan(&domainID, &k.ID, &k.Tip, &dosya, &k.UzakDurum, &k.BoyutB, &ts); err != nil {
			continue
		}
		k.Olusturma = ts.Int64
		if out[domainID] == nil {
			out[domainID] = map[string]dbKayit{}
		}
		out[domainID][dosya] = k
	}
	return out, rows.Err()
}

// topluSil: silinen yedeklerin DB kayıtlarını TEK DELETE...IN(...) ile temizler
// (domain başına ayrı ayrı Exec yerine).
func topluSil(ctx context.Context, db *sql.DB, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	_, err := db.ExecContext(ctx, `DELETE FROM backups WHERE id IN (`+placeholders+`)`, args...)
	return err
}

// temizlikAdaylari: bir domain için silinecek dosyaları toplar.
//
// Hem DB kayıtlarını hem de diskteki YETİM dosyaları kapsar. Yetimler önemli:
// Backup Yöneticisi'ndeki sayaç diski okur, dolayısıyla DB'de karşılığı olmayan
// bir dosya sayıda görünür. Yalnız DB'yi temizleseydik "tüm otomatik yedekleri
// sil" dedikten sonra sayaç sıfırlanmaz, kullanıcı işlemin çalışmadığını
// sanırdı.
//
// bilinen: bu domain'e ait backups kayıtları (tumBilinenKayitlar'dan, dosya
// adına göre) — domain başına ayrı sorgu atmamak için çağıran taraf hazırlar.
func temizlikAdaylari(domainID int64, sk, mod string, esik time.Time, bilinen map[string]dbKayit) ([]temizlikAday, error) {
	if bilinen == nil {
		bilinen = map[string]dbKayit{}
	}

	// Diskteki dosyalar: boyut ve (DB kaydı yoksa) yaş buradan okunur.
	diskte := map[string]os.FileInfo{}
	if entries, err := os.ReadDir(filepath.Join(BackupRoot, sk)); err == nil {
		for _, en := range entries {
			if en.IsDir() || !strings.HasSuffix(en.Name(), ".tar.gz") {
				continue
			}
			if fi, err := en.Info(); err == nil {
				diskte[en.Name()] = fi
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	// Aday kümesi = DB kayıtları ∪ diskteki dosyalar. Birleşim şart:
	// - yalnız DB'de olan: uzak hedefe yüklenip yerelden düşmüş yedek,
	// - yalnız diskte olan: DB satırı silinmiş yetim dosya (sayaç bunu sayar,
	//   temizlemezsek kullanıcı işlemin çalışmadığını sanır).
	adaylar := make(map[string]struct{}, len(bilinen)+len(diskte))
	for ad := range bilinen {
		adaylar[ad] = struct{}{}
	}
	for ad := range diskte {
		adaylar[ad] = struct{}{}
	}

	var out []temizlikAday
	for ad := range adaylar {
		k, dbVar := bilinen[ad]
		fi, diskVar := diskte[ad]

		var uygun bool
		switch mod {
		case ModOto:
			// DB kaydı varsa tip'e güven; yoksa dosya adına düş.
			uygun = otomatikDosya(sk, ad)
			if dbVar {
				uygun = k.Tip == TipOto
			}
		case ModManuel:
			uygun = !otomatikDosya(sk, ad)
			if dbVar {
				uygun = k.Tip != TipOto
			}
		case ModGun:
			// Yaş ölçüsü: DB kaydı varsa created_at, yoksa dosya mtime.
			// created_at tercih edilir çünkü uzak hedeften geri indirilen bir
			// yedeğin mtime'ı tazelenmiş olur ve dosya olduğundan genç görünür.
			var olusma time.Time
			switch {
			case dbVar && k.Olusturma > 0:
				olusma = time.Unix(k.Olusturma, 0)
			case diskVar:
				olusma = fi.ModTime()
			default:
				continue // yaşı bilinemeyen kaydı silme
			}
			uygun = olusma.Before(esik)
		}
		if !uygun {
			continue
		}

		boyut := k.BoyutB
		if diskVar {
			boyut = fi.Size() // disktekine güven: gerçek yer kazancı budur
		}
		out = append(out, temizlikAday{
			BackupID:  k.ID, // dbVar false ise 0 kalır → yetim dosya
			Dosya:     ad,
			UzakDurum: k.UzakDurum,
			BoyutB:    boyut,
		})
	}
	return out, nil
}
