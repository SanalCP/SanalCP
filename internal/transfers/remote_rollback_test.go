package transfers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"sanalcp/internal/domains"
)

func TestRollbackDomainDeleteHatasiniDondurur(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT alan_adi, sistem_kullanici, is_demo, ana_domain_id FROM domains WHERE id=\?`).
		WithArgs(int64(42)).
		WillReturnError(sql.ErrNoRows)

	h := &Handlers{Domains: &domains.Handlers{DB: db}}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/42", nil)
	if err := h.rollbackDomain(req, 42); err == nil {
		t.Fatal("başarısız domain silme rollback hatası yutuldu")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
