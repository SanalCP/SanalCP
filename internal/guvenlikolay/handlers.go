package guvenlikolay

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sanalcp/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Handlers struct{ DB *sql.DB }

type aday struct {
	Tur, Seviye, Baslik, Aciklama, Kaynak, IP string
	DomainID                                  sql.NullInt64
	Sayi                                      int
	Ilk, Son                                  string
}

var (
	wafTx   = regexp.MustCompile(`(?ms)--[0-9A-Za-z]+-A--\s*(.*?)--[0-9A-Za-z]+-B--\s*(.*?)--[0-9A-Za-z]+-H--\s*(.*?)--[0-9A-Za-z]+-Z--`)
	wafIP   = regexp.MustCompile(`(?:^|\s)((?:\d{1,3}\.){3}\d{1,3}|[0-9a-fA-F:]{3,})(?:\s|$)`)
	wafHost = regexp.MustCompile(`(?mi)^Host:\s*([^:\s]+)`)
	wafRule = regexp.MustCompile(`\[id "(\d+)"\]`)
	wafMsg  = regexp.MustCompile(`\[msg "([^"]+)"\]`)
)

func wafAdaylari(path string) []aday {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	const max = 2 << 20
	if st.Size() > max {
		_, _ = f.Seek(st.Size()-max, 0)
	}
	b, _ := io.ReadAll(io.LimitReader(f, max))
	now := time.Now().Format("2006-01-02 15:04:05")
	tip := map[string]*aday{}
	for _, m := range wafTx.FindAllSubmatch(b, -1) {
		ip, host, rule, msg := "", "", "", "ModSecurity kural eşleşmesi"
		if x := wafIP.FindSubmatch(m[1]); len(x) > 1 {
			ip = string(x[1])
		}
		if x := wafHost.FindSubmatch(m[2]); len(x) > 1 {
			host = strings.ToLower(string(x[1]))
		}
		if x := wafRule.FindSubmatch(m[3]); len(x) > 1 {
			rule = string(x[1])
		}
		if x := wafMsg.FindSubmatch(m[3]); len(x) > 1 {
			msg = string(x[1])
		}
		key := ip + "|" + host + "|" + rule
		if tip[key] == nil {
			tip[key] = &aday{Tur: "waf_kural", Seviye: "yuksek", Baslik: "WAF kural eşleşmeleri", Kaynak: "waf", IP: ip, Ilk: now, Son: now, Aciklama: fmt.Sprintf("%s üzerinde ModSecurity %s kuralı: %s", host, rule, msg)}
		}
		tip[key].Sayi++
	}
	out := make([]aday, 0, len(tip))
	for _, a := range tip {
		out = append(out, *a)
	}
	return out
}

func anahtar(a aday) string {
	d := int64(0)
	if a.DomainID.Valid {
		d = a.DomainID.Int64
	}
	b := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%s|%s", a.Tur, a.IP, d, a.Ilk[:min(10, len(a.Ilk))], a.Son[:min(13, len(a.Son))])))
	return fmt.Sprintf("%x", b[:])
}

func (h *Handlers) senkronize() {
	adaylar := []aday{}
	adaylar = append(adaylar, wafAdaylari("/var/log/nginx/modsec_audit.log")...)
	rows, _ := h.DB.Query(`SELECT ip,COUNT(*),DATE_FORMAT(MIN(ts),'%Y-%m-%d %H:%i:%s'),DATE_FORMAT(MAX(ts),'%Y-%m-%d %H:%i:%s') FROM audit_log WHERE ok=0 AND action IN ('auth.login','auth.2fa','musteri.login') AND ts>=NOW()-INTERVAL 15 MINUTE AND ip<>'' GROUP BY ip HAVING COUNT(*)>=5`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var a aday
			a.Tur = "yogun_giris_deneme"
			a.Seviye = "yuksek"
			a.Baslik = "Yoğun başarısız giriş denemesi"
			a.Kaynak = "giris"
			_ = rows.Scan(&a.IP, &a.Sayi, &a.Ilk, &a.Son)
			a.Aciklama = fmt.Sprintf("%s adresinden 15 dakika içinde %d başarısız giriş denemesi.", a.IP, a.Sayi)
			adaylar = append(adaylar, a)
		}
	}
	rows2, _ := h.DB.Query(`SELECT s.ip,COUNT(f.id),DATE_FORMAT(MIN(f.ts),'%Y-%m-%d %H:%i:%s'),DATE_FORMAT(MAX(s.ts),'%Y-%m-%d %H:%i:%s') FROM audit_log s JOIN audit_log f ON f.ip=s.ip AND f.ok=0 AND f.ts BETWEEN s.ts-INTERVAL 15 MINUTE AND s.ts WHERE s.ok=1 AND s.action IN ('auth.login','musteri.login') AND s.ts>=NOW()-INTERVAL 1 DAY GROUP BY s.id,s.ip HAVING COUNT(f.id)>=5`)
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var a aday
			a.Tur = "deneme_sonrasi_giris"
			a.Seviye = "kritik"
			a.Baslik = "Başarısız denemeler sonrası başarılı giriş"
			a.Kaynak = "giris"
			_ = rows2.Scan(&a.IP, &a.Sayi, &a.Ilk, &a.Son)
			a.Aciklama = fmt.Sprintf("%s adresi %d başarısız denemeden sonra giriş yaptı.", a.IP, a.Sayi)
			adaylar = append(adaylar, a)
		}
	}
	rows3, _ := h.DB.Query(`SELECT b.domain_id,COUNT(*),DATE_FORMAT(MIN(b.created_at),'%Y-%m-%d %H:%i:%s'),DATE_FORMAT(MAX(b.created_at),'%Y-%m-%d %H:%i:%s'),COALESCE(d.alan_adi,'') FROM av_bulgular b LEFT JOIN domains d ON d.id=b.domain_id WHERE b.karantina=0 AND COALESCE(b.istisna,0)=0 AND b.created_at>=NOW()-INTERVAL 1 DAY GROUP BY b.domain_id,d.alan_adi`)
	if rows3 != nil {
		defer rows3.Close()
		for rows3.Next() {
			var a aday
			var alan string
			a.Tur = "zararli_dosya"
			a.Seviye = "kritik"
			a.Baslik = "Aktif zararlı dosya bulgusu"
			a.Kaynak = "dosya"
			_ = rows3.Scan(&a.DomainID, &a.Sayi, &a.Ilk, &a.Son, &alan)
			a.Aciklama = fmt.Sprintf("%s üzerinde %d aktif bulgu karantina bekliyor.", alan, a.Sayi)
			adaylar = append(adaylar, a)
		}
	}
	rows4, _ := h.DB.Query(`SELECT ip,COUNT(*),DATE_FORMAT(MIN(created_at),'%Y-%m-%d %H:%i:%s'),DATE_FORMAT(MAX(created_at),'%Y-%m-%d %H:%i:%s') FROM panel_hiz_olaylari WHERE created_at>=NOW()-INTERVAL 15 MINUTE GROUP BY ip HAVING COUNT(*)>=20`)
	if rows4 != nil {
		defer rows4.Close()
		for rows4.Next() {
			var a aday
			a.Tur = "panel_hiz_saldirisi"
			a.Seviye = "yuksek"
			a.Baslik = "Yoğun panel hız sınırı olayı"
			a.Kaynak = "waf"
			_ = rows4.Scan(&a.IP, &a.Sayi, &a.Ilk, &a.Son)
			a.Aciklama = fmt.Sprintf("%s adresi 15 dakikada %d kez sınırlandı.", a.IP, a.Sayi)
			adaylar = append(adaylar, a)
		}
	}
	rows5, _ := h.DB.Query(`SELECT j.domain_id,COUNT(*),DATE_FORMAT(MIN(j.created_at),'%Y-%m-%d %H:%i:%s'),DATE_FORMAT(MAX(j.finished_at),'%Y-%m-%d %H:%i:%s'),COALESCE(d.alan_adi,'') FROM laravel_deploy_jobs j LEFT JOIN domains d ON d.id=j.domain_id WHERE j.status IN ('failed','rolled_back') AND j.created_at>=NOW()-INTERVAL 1 DAY GROUP BY j.domain_id,d.alan_adi`)
	if rows5 != nil {
		defer rows5.Close()
		for rows5.Next() {
			var a aday
			var alan string
			a.Tur = "basarisiz_deploy"
			a.Seviye = "orta"
			a.Baslik = "Başarısız veya geri alınmış deploy"
			a.Kaynak = "surec"
			_ = rows5.Scan(&a.DomainID, &a.Sayi, &a.Ilk, &a.Son, &alan)
			a.Aciklama = fmt.Sprintf("%s için son 24 saatte %d deploy başarısız oldu veya geri alındı.", alan, a.Sayi)
			adaylar = append(adaylar, a)
		}
	}
	rows6, _ := h.DB.Query(`SELECT COUNT(*),DATE_FORMAT(MIN(created_at),'%Y-%m-%d %H:%i:%s'),DATE_FORMAT(MAX(finished_at),'%Y-%m-%d %H:%i:%s') FROM remote_transfer_jobs WHERE status='failed' AND created_at>=NOW()-INTERVAL 1 DAY HAVING COUNT(*)>0`)
	if rows6 != nil {
		defer rows6.Close()
		for rows6.Next() {
			var a aday
			a.Tur = "basarisiz_tasima"
			a.Seviye = "orta"
			a.Baslik = "Başarısız uzak site taşıması"
			a.Kaynak = "surec"
			_ = rows6.Scan(&a.Sayi, &a.Ilk, &a.Son)
			a.Aciklama = fmt.Sprintf("Son 24 saatte %d uzak taşıma işi başarısız oldu.", a.Sayi)
			adaylar = append(adaylar, a)
		}
	}
	for _, a := range adaylar {
		_, _ = h.DB.Exec(`INSERT INTO guvenlik_bildirimleri(dedup_key,tur,seviye,baslik,aciklama,kaynak,kaynak_sayisi,ip,domain_id,ilk_at,son_at) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE seviye=VALUES(seviye),aciklama=VALUES(aciklama),kaynak_sayisi=VALUES(kaynak_sayisi),son_at=VALUES(son_at),durum=IF(durum='cozuldu' AND VALUES(son_at)>COALESCE(cozuldu_at,'1000-01-01'), 'acik',durum)`, anahtar(a), a.Tur, a.Seviye, a.Baslik, a.Aciklama, a.Kaynak, a.Sayi, a.IP, a.DomainID, a.Ilk, a.Son)
	}
}

// Senkronize güvenlik adaylarını kalıcı bildirimlere işler. Merkezi işlem
// özeti de bu çağrıyı kullanır; böylece eski bildirim rozeti kaldırıldığında
// korelasyon yalnız güvenlik sayfası açılınca çalışan bir sürece dönüşmez.
func (h *Handlers) Senkronize() { h.senkronize() }

func (h *Handlers) Liste(w http.ResponseWriter, r *http.Request) {
	h.senkronize()
	durum := r.URL.Query().Get("durum")
	if durum != "cozuldu" {
		durum = "acik"
	}
	rows, err := h.DB.QueryContext(r.Context(), `SELECT id,tur,seviye,baslik,aciklama,kaynak,kaynak_sayisi,ip,COALESCE(domain_id,0),durum,DATE_FORMAT(ilk_at,'%Y-%m-%d %H:%i:%s'),DATE_FORMAT(son_at,'%Y-%m-%d %H:%i:%s') FROM guvenlik_bildirimleri WHERE durum=? ORDER BY FIELD(seviye,'kritik','yuksek','orta','dusuk'),son_at DESC LIMIT 200`, durum)
	if err != nil {
		httpx.WriteError(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, did int64
		var tur, sev, b, a, k, ip, d, ilk, son string
		var n int
		if rows.Scan(&id, &tur, &sev, &b, &a, &k, &n, &ip, &did, &d, &ilk, &son) == nil {
			out = append(out, map[string]any{"id": id, "tur": tur, "seviye": sev, "baslik": b, "aciklama": a, "kaynak": k, "kaynak_sayisi": n, "ip": ip, "domain_id": did, "durum": d, "ilk_at": ilk, "son_at": son})
		}
	}
	httpx.WriteJSON(w, 200, out)
}

func (h *Handlers) Ozet(w http.ResponseWriter, r *http.Request) {
	h.senkronize()
	var toplam, kritik int
	_ = h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*),COALESCE(SUM(seviye='kritik'),0) FROM guvenlik_bildirimleri WHERE durum='acik'`).Scan(&toplam, &kritik)
	httpx.WriteJSON(w, 200, map[string]any{"acik": toplam, "kritik": kritik})
}

func (h *Handlers) Durum(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var q struct {
		Durum string `json:"durum"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		httpx.WriteError(w, 400, "geçersiz gövde")
		return
	}
	q.Durum = strings.TrimSpace(q.Durum)
	if q.Durum != "acik" && q.Durum != "cozuldu" {
		httpx.WriteError(w, 400, "geçersiz durum")
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `UPDATE guvenlik_bildirimleri SET durum=?,cozuldu_at=IF(?='cozuldu',NOW(),NULL) WHERE id=?`, q.Durum, q.Durum, id)
	if err != nil {
		httpx.WriteError(w, 500, "durum güncellenemedi")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.WriteError(w, 404, "bildirim bulunamadı")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}
