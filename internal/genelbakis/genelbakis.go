// Package genelbakis — sunucu genelinde tek ekranda özet listeler.
//
// Panelin mevcut DNS/SSL/e-posta/veritabanı uçlarının hepsi domain kapsamlı
// (/domains/{id}/dns gibi). Bu paket bunların sunucu genelindeki karşılığını
// verir: "hangi sertifika ne zaman bitiyor", "hangi domainin MX kaydı eksik",
// "toplam kaç posta kutusu var" sorularını domainleri tek tek gezmeden
// yanıtlamak için. Yalnız okuma yapar — değişiklik hâlâ domain kapsamlı
// uçlardan geçer, böylece yetki ve doğrulama mantığı tek yerde kalır.
//
// Yetki: admin + bayi (BayiVeUstu). Listeler middleware.KapsamSQL ile
// daraltılır — admin tüm domainleri, bayi yalnız kendi müşterilerininkini,
// müşteri yalnız kendi domainini görür. Daraltma sorgunun içindedir; satır
// satır filtrelemek liste uçlarında sızıntıyı önlemez.
package genelbakis

import (
	"context"
	"database/sql"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"sanalpanel/internal/httpx"
	"sanalpanel/internal/middleware"
)

type Handlers struct{ DB *sql.DB }

// ---------- DNS ----------

type DNSSatir struct {
	DomainID    int64  `json:"domain_id"`
	AlanAdi     string `json:"alan_adi"`
	Durum       string `json:"durum"`
	KayitSayisi int    `json:"kayit_sayisi"`
	ASayisi     int    `json:"a_sayisi"`
	MXSayisi    int    `json:"mx_sayisi"`
	TXTSayisi   int    `json:"txt_sayisi"`
	PasifSayisi int    `json:"pasif_sayisi"`
	DNSSECAktif bool   `json:"dnssec_aktif"`
}

func (h *Handlers) DNS(w http.ResponseWriter, r *http.Request) {
	q := `
SELECT d.id, d.alan_adi, d.durum, d.dnssec_aktif,
       COUNT(r.id),
       COALESCE(SUM(r.tip='A'), 0),
       COALESCE(SUM(r.tip='MX'), 0),
       COALESCE(SUM(r.tip='TXT'), 0),
       COALESCE(SUM(r.aktif=0), 0)
FROM domains d
LEFT JOIN dns_records r ON r.domain_id = d.id`

	kosul, arg := middleware.KapsamSQL(r, "d")
	q += kosul + `
GROUP BY d.id, d.alan_adi, d.durum, d.dnssec_aktif
ORDER BY d.alan_adi`

	rows, err := h.DB.QueryContext(r.Context(), q, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := make([]DNSSatir, 0)
	for rows.Next() {
		var s DNSSatir
		var dnssec int
		if err := rows.Scan(&s.DomainID, &s.AlanAdi, &s.Durum, &dnssec,
			&s.KayitSayisi, &s.ASayisi, &s.MXSayisi, &s.TXTSayisi, &s.PasifSayisi); err != nil {
			continue
		}
		s.DNSSECAktif = dnssec == 1
		out = append(out, s)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// ---------- SSL ----------

type SSLSatir struct {
	DomainID int64  `json:"domain_id"`
	AlanAdi  string `json:"alan_adi"`
	Durum    string `json:"durum"`
	Aktif    bool   `json:"ssl_aktif"`
	Bitis    string `json:"ssl_bitis"` // YYYY-AA-GG, bilinmiyorsa ""
	KalanGun *int   `json:"kalan_gun"` // bitiş yoksa null
}

func (h *Handlers) SSL(w http.ResponseWriter, r *http.Request) {
	// Sıralama, ekranın asıl işine göre: önce süresi dolmuş/dolmak üzere olan
	// sertifikalar, sonra ileri tarihliler, en sonda SSL'i hiç olmayanlar.
	q := `
SELECT d.id, d.alan_adi, d.durum, d.ssl_aktif,
       COALESCE(DATE_FORMAT(d.ssl_bitis, '%Y-%m-%d'), ''),
       CASE WHEN d.ssl_bitis IS NULL THEN NULL ELSE DATEDIFF(d.ssl_bitis, CURDATE()) END
FROM domains d`

	kosul, arg := middleware.KapsamSQL(r, "d")
	q += kosul + `
ORDER BY (d.ssl_bitis IS NULL), d.ssl_bitis ASC, d.alan_adi`

	rows, err := h.DB.QueryContext(r.Context(), q, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := make([]SSLSatir, 0)
	for rows.Next() {
		var s SSLSatir
		var aktif int
		var kalan sql.NullInt64
		if err := rows.Scan(&s.DomainID, &s.AlanAdi, &s.Durum, &aktif, &s.Bitis, &kalan); err != nil {
			continue
		}
		s.Aktif = aktif == 1
		if kalan.Valid {
			g := int(kalan.Int64)
			s.KalanGun = &g
		}
		out = append(out, s)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// ---------- E-posta ----------

type MailSatir struct {
	DomainID    int64  `json:"domain_id"`
	AlanAdi     string `json:"alan_adi"`
	MailAktif   bool   `json:"mail_aktif"`
	MailDurum   string `json:"mail_durum"` // active | suspended | "" (hiç açılmamış)
	KutuSayisi  int    `json:"kutu_sayisi"`
	AliasSayisi int    `json:"alias_sayisi"`
	PasifKutu   int    `json:"pasif_kutu"`
}

func (h *Handlers) Mail(w http.ResponseWriter, r *http.Request) {
	// Alt sorgu kullanılıyor: mailboxes ve mail_aliases'ı aynı anda JOIN etmek
	// kartezyen çarpım üretir ve sayımları şişirir.
	q := `
SELECT d.id, d.alan_adi,
       COALESCE(md.durum, ''),
       (SELECT COUNT(*) FROM mailboxes mb WHERE mb.domain_id = d.id),
       (SELECT COUNT(*) FROM mail_aliases a WHERE a.domain_id = d.id),
       (SELECT COUNT(*) FROM mailboxes mb2 WHERE mb2.domain_id = d.id AND mb2.status = 'suspended')
FROM domains d
LEFT JOIN mail_domains md ON md.domain_id = d.id`

	kosul, arg := middleware.KapsamSQL(r, "d")
	q += kosul + `
ORDER BY d.alan_adi`

	rows, err := h.DB.QueryContext(r.Context(), q, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := make([]MailSatir, 0)
	for rows.Next() {
		var s MailSatir
		if err := rows.Scan(&s.DomainID, &s.AlanAdi, &s.MailDurum,
			&s.KutuSayisi, &s.AliasSayisi, &s.PasifKutu); err != nil {
			continue
		}
		s.MailAktif = s.MailDurum != ""
		out = append(out, s)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// ---------- Veritabanları ----------

type DBSatir struct {
	ID        int64  `json:"id"`
	DomainID  int64  `json:"domain_id"`
	AlanAdi   string `json:"alan_adi"`
	DBAdi     string `json:"db_adi"`
	DBUser    string `json:"db_user"`
	DBHost    string `json:"db_host"`
	BoyutKB   int64  `json:"boyut_kb"`
	Olusturma string `json:"olusturma"`
}

// dbBoyutlari: şema adı -> KB.
//
// Panelin kendi DSN'i (`panel` kullanıcısı) yalnız `panel` şemasına yetkilidir;
// MySQL information_schema.TABLES'ı yetkiye göre filtrelediği için o bağlantı
// üzerinden müşteri veritabanlarının boyutu HİÇ görünmez (sessizce 0 döner).
// Bu yüzden panelin veritabanı oluştururken de kullandığı yol izleniyor:
// root'un unix-socket kimliğiyle çalışan `mysql` istemcisi
// (bkz. internal/hesaplar, internal/kaynaklimit).
//
// Hata durumunda boş harita döner — boyut sütunu "—" gösterir, liste yine gelir.
func dbBoyutlari() map[string]int64 {
	ctx, iptal := context.WithTimeout(context.Background(), 10*time.Second)
	defer iptal()

	out, err := exec.CommandContext(ctx, "mysql", "-N", "-B", "-e",
		`SELECT table_schema, COALESCE(SUM(data_length + index_length), 0) DIV 1024
		 FROM information_schema.TABLES GROUP BY table_schema`).Output()
	if err != nil {
		return map[string]int64{}
	}

	boyut := make(map[string]int64)
	for _, satir := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		alan := strings.Split(satir, "\t")
		if len(alan) != 2 {
			continue
		}
		kb, err := strconv.ParseInt(strings.TrimSpace(alan[1]), 10, 64)
		if err != nil {
			continue
		}
		boyut[strings.TrimSpace(alan[0])] = kb
	}
	return boyut
}

func (h *Handlers) Veritabanlari(w http.ResponseWriter, r *http.Request) {
	q := `
SELECT a.id, a.domain_id, d.alan_adi, a.db_name, a.db_user, a.db_host,
       COALESCE(DATE_FORMAT(a.created_at, '%Y-%m-%d'), '')
FROM db_accounts a
JOIN domains d ON d.id = a.domain_id`

	kosul, arg := middleware.KapsamSQL(r, "d")
	q += kosul + `
ORDER BY d.alan_adi, a.db_name`

	rows, err := h.DB.QueryContext(r.Context(), q, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := make([]DBSatir, 0)
	for rows.Next() {
		var s DBSatir
		if err := rows.Scan(&s.ID, &s.DomainID, &s.AlanAdi, &s.DBAdi, &s.DBUser, &s.DBHost, &s.Olusturma); err != nil {
			continue
		}
		out = append(out, s)
	}

	boyutlar := dbBoyutlari()

	for i := range out {
		out[i].BoyutKB = boyutlar[out[i].DBAdi]
	}

	httpx.WriteJSON(w, http.StatusOK, out)
}
