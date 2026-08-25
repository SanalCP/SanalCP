package domains

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"

	"sanalcp/internal/hesaplar"
	"sanalcp/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// dbKullanicilariGetir: bir domain+db_name icin GRANT'li tum kullanicilari
// dondurur (Task 4-7'de paylasilan yardimci).
func dbKullanicilariGetir(ctx context.Context, db *sql.DB, domainID int64, dbAdi string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT db_user FROM db_accounts WHERE domain_id=? AND db_name=?`, domainID, dbAdi)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

type dbKullaniciSatiri struct {
	ID          int64  `json:"id"`
	DBKullanici string `json:"db_kullanici"`
	DBParola    string `json:"db_parola"`
	Olusturulma string `json:"olusturulma"`
}

type dbGrupDetay struct {
	DBAdi        string              `json:"db_adi"`
	DBHost       string              `json:"db_host"`
	Charset      string              `json:"charset"`
	Collation    string              `json:"collation"`
	BoyutMB      float64             `json:"boyut_mb"`
	Kullanicilar []dbKullaniciSatiri `json:"kullanicilar"`
}

// DatabaseGrupDetay: GET /domains/{id}/databases/{dbAdi}
func (h *Handlers) DatabaseGrupDetay(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	dbAdi := chi.URLParam(r, "dbAdi")

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, db_user, db_host, db_pass_plain, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i')
		 FROM db_accounts WHERE domain_id=? AND db_name=? ORDER BY id`, id, dbAdi)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB sorgu: "+err.Error())
		return
	}
	defer rows.Close()

	var out dbGrupDetay
	out.DBAdi = dbAdi
	for rows.Next() {
		var k dbKullaniciSatiri
		var host string
		if err := rows.Scan(&k.ID, &k.DBKullanici, &host, &k.DBParola, &k.Olusturulma); err != nil {
			continue
		}
		out.DBHost = host
		if dec, err := hesaplar.DecryptDBPassword(k.DBParola); err == nil {
			k.DBParola = dec
		}
		out.Kullanicilar = append(out.Kullanicilar, k)
	}
	if len(out.Kullanicilar) == 0 {
		httpx.WriteError(w, http.StatusNotFound, "veritabanı bulunamadı")
		return
	}

	var boyutMB sql.NullFloat64
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT SUM(data_length+index_length)/1024/1024 FROM information_schema.tables WHERE table_schema=?`,
		dbAdi).Scan(&boyutMB)
	out.BoyutMB = boyutMB.Float64

	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT default_character_set_name, default_collation_name FROM information_schema.schemata WHERE schema_name=?`,
		dbAdi).Scan(&out.Charset, &out.Collation)

	httpx.WriteJSON(w, http.StatusOK, out)
}
