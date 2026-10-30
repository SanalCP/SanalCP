package planfeatures

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const fullFeatures = `{"dns":true,"mail":true,"databases":false,"files":true,"ftp":true,"cron":true,"backups":true,"git":true}`

func TestFeaturesRequireCompleteBooleanObject(t *testing.T) {
	for _, input := range []string{
		`null`, `{}`, `[]`, `{"dns":false}`,
		`{"dns":true,"mail":true,"databases":false,"files":true,"ftp":true,"cron":true,"backups":true,"other":true}`,
		`{"dns":true,"mail":true,"databases":null,"files":true,"ftp":true,"cron":true,"backups":true,"git":true}`,
		`{"dns":true,"mail":true,"databases":0,"files":true,"ftp":true,"cron":true,"backups":true,"git":true}`,
		`{"dns":true,"mail":true,"databases":"false","files":true,"ftp":true,"cron":true,"backups":true,"git":true}`,
		`{"dns":true,"mail":true,"databases":false,"files":true,"ftp":true,"cron":true,"backups":true,"git":true,"other":true}`,
	} {
		t.Run(input, func(t *testing.T) {
			f := All()
			if err := json.Unmarshal([]byte(input), &f); err == nil {
				t.Fatal("invalid features accepted")
			}
			if f != All() {
				t.Fatal("invalid input partially changed grants")
			}
		})
	}
	var f Features
	if err := json.Unmarshal([]byte(fullFeatures), &f); err != nil {
		t.Fatal(err)
	}
	if f.Databases || !f.DNS || !f.Git || f.Allows(Module("unknown")) {
		t.Fatalf("incorrect grants: %+v", f)
	}
}

func TestStoredOnlyLegacyNullDefaultsEnabled(t *testing.T) {
	f, err := Stored(sql.NullString{})
	if err != nil || f != All() {
		t.Fatalf("legacy features = %+v, %v", f, err)
	}
	for _, value := range []string{"", "null", "{}", "broken"} {
		if _, err := Stored(sql.NullString{String: value, Valid: true}); err == nil {
			t.Fatalf("corrupt settings %q granted access", value)
		}
	}
}

func TestResolveAddonUsesCurrentParentPlan(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	columns := []string{"ana_domain_id", "plan_id", "id", "features"}
	for _, disabled := range []bool{true, false} {
		stored := fullFeatures
		if !disabled {
			data, _ := json.Marshal(All())
			stored = string(data)
		}
		// The child's stale permissive copy is deliberately ignored.
		mock.ExpectQuery("SELECT d.ana_domain_id").WithArgs(int64(8)).WillReturnRows(sqlmock.NewRows(columns).AddRow(7, 2, 2, nil))
		mock.ExpectQuery("SELECT d.ana_domain_id").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows(columns).AddRow(nil, 3, 3, stored))
		features, err := Resolve(context.Background(), db, 8)
		if err != nil {
			t.Fatal(err)
		}
		if features.Databases == disabled {
			t.Fatalf("current parent grants ignored: %+v", features)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResolveNoPlanAndBrokenPlan(t *testing.T) {
	for _, tc := range []struct {
		name      string
		plan      any
		parent    any
		wantError bool
	}{
		{"no plan", nil, nil, false}, {"missing plan", int64(9), nil, true}, {"missing parent", nil, int64(7), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectQuery("SELECT d.ana_domain_id").WithArgs(int64(8)).WillReturnRows(sqlmock.NewRows([]string{"ana_domain_id", "plan_id", "id", "features"}).AddRow(tc.parent, tc.plan, nil, nil))
			if tc.parent != nil {
				mock.ExpectQuery("SELECT d.ana_domain_id").WithArgs(int64(7)).WillReturnError(sql.ErrNoRows)
			}
			f, err := Resolve(context.Background(), db, 8)
			if (err != nil) != tc.wantError {
				t.Fatalf("features=%+v error=%v", f, err)
			}
			if !tc.wantError && f != All() {
				t.Fatalf("no plan was restricted: %+v", f)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
