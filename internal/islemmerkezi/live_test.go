package islemmerkezi

import (
	"database/sql"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

// SANALCP_ISLEM_LIVE_DSN verilirse birleşik UNION sorgusunu gerçek MariaDB
// parser'ı ve migration şeması üzerinde çalıştırır. Normal birim testleri dış
// servise bağımlı kalmasın diye varsayılan koşuda atlanır.
func TestListeCanliMariaDB(t *testing.T) {
	dsn := os.Getenv("SANALCP_ISLEM_LIVE_DSN")
	if dsn == "" {
		t.Skip("SANALCP_ISLEM_LIVE_DSN ayarlı değil")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(listeSQL)
	if err != nil {
		t.Fatalf("birleşik sorgu: %v", err)
	}
	_ = rows.Close()
	w := httptest.NewRecorder()
	(&Handlers{DB: db}).Liste(w, httptest.NewRequest("GET", "/api/v1/islemler", nil))
	if w.Code != 200 {
		t.Fatalf("durum=%d gövde=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "[]\n" {
		t.Fatalf("boş şemada [] bekleniyordu: %s", got)
	}
}
