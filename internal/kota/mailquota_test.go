package kota

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

var planKotaSorgusu = `FROM domains d LEFT JOIN service_plans p`

// Ek alan adı: satırındaki plan kopyası bayat olabilir; kota, ana hesabın
// GÜNCEL planından okunmalı.
func TestPlanMailboxQuotaMBEkAlanAdiAnaHesabinPlaniniKullanir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cols := []string{"ana_domain_id", "plan_id", "id", "mailbox_quota_mb"}
	// ek domain (kendi plan kopyası 1, kota 50) → ana domain 7 → plan 2, kota 2048
	mock.ExpectQuery(planKotaSorgusu).WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(7, 1, 1, 50))
	mock.ExpectQuery(planKotaSorgusu).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(nil, 2, 2, 2048))

	mb, err := PlanMailboxQuotaMB(context.Background(), db, 9)
	if err != nil || mb != 2048 {
		t.Fatalf("mb=%d err=%v, beklenen 2048 (ana hesap planı)", mb, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestPlanMailboxQuotaMBPlansizVeEksikPlan(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cols := []string{"ana_domain_id", "plan_id", "id", "mailbox_quota_mb"}

	// Plansız domain: sınırsız (0), hata yok.
	mock.ExpectQuery(planKotaSorgusu).WillReturnRows(sqlmock.NewRows(cols).AddRow(nil, nil, nil, nil))
	if mb, err := PlanMailboxQuotaMB(context.Background(), db, 1); err != nil || mb != 0 {
		t.Fatalf("plansız: mb=%d err=%v", mb, err)
	}
	// plan_id dolu ama plan satırı yok: sessizce "sınırsız" sayılmamalı (fail-closed).
	mock.ExpectQuery(planKotaSorgusu).WillReturnRows(sqlmock.NewRows(cols).AddRow(nil, 5, nil, nil))
	if _, err := PlanMailboxQuotaMB(context.Background(), db, 1); err == nil {
		t.Fatal("eksik plan hata döndürmeliydi")
	}
}

// Döngüsel ana_domain_id zinciri sonsuz döngüye girmeden hata vermeli.
func TestPlanMailboxQuotaMBDongusel(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cols := []string{"ana_domain_id", "plan_id", "id", "mailbox_quota_mb"}
	mock.ExpectQuery(planKotaSorgusu).WillReturnRows(sqlmock.NewRows(cols).AddRow(2, nil, nil, nil))
	mock.ExpectQuery(planKotaSorgusu).WillReturnRows(sqlmock.NewRows(cols).AddRow(1, nil, nil, nil))
	if _, err := PlanMailboxQuotaMB(context.Background(), db, 1); err == nil {
		t.Fatal("döngü hata döndürmeliydi")
	}
}
