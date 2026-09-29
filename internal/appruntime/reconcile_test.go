package appruntime

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestUnitDirectiveFindsExactKey(t *testing.T) {
	body := []byte("WorkingDirectory=/home/c_example/public_html\nExecStart=/usr/bin/node /home/c_example/public_html/app.js\n")
	if got := unitDirective(body, "WorkingDirectory"); got != "/home/c_example/public_html" {
		t.Fatalf("working directory = %q", got)
	}
	if got := unitDirective(body, "ExecStart"); got != "/usr/bin/node /home/c_example/public_html/app.js" {
		t.Fatalf("exec start = %q", got)
	}
	if got := unitDirective(body, "Environment"); got != "" {
		t.Fatalf("unexpected directive = %q", got)
	}
}

func TestPendingMarkerForcesRecoveryEvenWhenUnitMatches(t *testing.T) {
	body := []byte("WorkingDirectory=/home/c_example/public_html\nExecStart=/usr/bin/node /home/c_example/public_html/app.js\n")
	if unitNeedsRecovery(body, body, "/home/c_example/public_html", false) {
		t.Fatal("matching unit needs recovery")
	}
	if !unitNeedsRecovery(body, body, "/home/c_example/public_html", true) {
		t.Fatal("pending switch was ignored")
	}
	if !unitNeedsRecovery(body, body, "/home/c_example/other", false) {
		t.Fatal("wrong working directory was ignored")
	}
}

func TestReconcileWithoutManagedApps(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT domain_id FROM app_runtimes").WillReturnRows(sqlmock.NewRows([]string{"domain_id"}))
	if err := Reconcile(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
