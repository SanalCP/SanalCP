// Package kota: plan limitlerini check eder (yeni domain/DB/FTP ekleme oncesi)
package kota

import (
	"context"
	"database/sql"
	"fmt"
)

type LimitHatasi struct {
	Mesaj string
}

func (e *LimitHatasi) Error() string { return e.Mesaj }

// ---------- Bayi limitleri (WHM "reseller limits" karşılığı) ----------
//
// reseller_limits tablosunda satırı olmayan bayi SINIRSIZDIR; 0 değeri de
// sınırsız demektir (service_plans'taki mevcut kota sözleşmesiyle aynı).
// Böylece limit tanımlamak opsiyonel kalır ve mevcut davranış değişmez.

// CheckBayiMusteriEklenebilir: bayi yeni müşteri açabilir mi?
func CheckBayiMusteriEklenebilir(ctx context.Context, db *sql.DB, bayiUserID int64) error {
	maks, err := bayiLimiti(ctx, db, bayiUserID, "max_customer")
	if err != nil || maks <= 0 {
		return nil
	}
	var mevcut int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM customers WHERE owner_user_id=?`, bayiUserID).Scan(&mevcut)
	if mevcut >= maks {
		return &LimitHatasi{Mesaj: fmt.Sprintf("bayi limiti aşıldı: en fazla %d müşteri", maks)}
	}
	return nil
}

// CheckBayiDomainEklenebilir: bayinin toplam domain kotası dolmuş mu?
//
// Müşteri planının max_domain'inden ayrıdır: o tek müşteriyi, bu bayinin
// tüm müşterilerinin toplamını sınırlar. İkisi de uygulanır.
func CheckBayiDomainEklenebilir(ctx context.Context, db *sql.DB, bayiUserID int64) error {
	maks, err := bayiLimiti(ctx, db, bayiUserID, "max_domain")
	if err != nil || maks <= 0 {
		return nil
	}
	var mevcut int
	_ = db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM domains d JOIN customers c ON c.id = d.customer_id
		WHERE c.owner_user_id = ?`, bayiUserID).Scan(&mevcut)
	if mevcut >= maks {
		return &LimitHatasi{Mesaj: fmt.Sprintf("bayi limiti aşıldı: en fazla %d domain", maks)}
	}
	return nil
}

// CheckBayiDiskKotasi: bayinin tüm müşterilerinin toplam disk kullanımı
// limitini aşmış mı?
//
// domains.boyut_kb periyodik olarak güncellenir (bkz. internal/domains
// disk toplayıcı), yani bu kontrol ANLIK değil son ölçüme dayanır. Amaç sert
// bir kesme değil, yeni kaynak eklemeyi durdurmak: kota dolmuşsa bayi yeni
// domain açamaz. Var olan siteler çalışmaya devam eder — disk kesmesi
// tenant seviyesinde XFS kotasının işidir (bkz. internal/kaynaklimit).
func CheckBayiDiskKotasi(ctx context.Context, db *sql.DB, bayiUserID int64) error {
	maks, err := bayiLimiti(ctx, db, bayiUserID, "disk_kota_mb")
	if err != nil || maks <= 0 {
		return nil
	}
	var kullanilanKB int64
	_ = db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(d.boyut_kb), 0)
		FROM domains d JOIN customers c ON c.id = d.customer_id
		WHERE c.owner_user_id = ?`, bayiUserID).Scan(&kullanilanKB)
	kullanilanMB := int(kullanilanKB / 1024)
	if kullanilanMB >= maks {
		return &LimitHatasi{Mesaj: fmt.Sprintf(
			"bayi disk kotası dolu: %d MB / %d MB", kullanilanMB, maks)}
	}
	return nil
}

// CheckBayiTrafikKotasi: aylık trafik kotası. domains.trafik_kb, aylık
// toplayıcı tarafından doldurulur (bkz. internal/istatistik).
func CheckBayiTrafikKotasi(ctx context.Context, db *sql.DB, bayiUserID int64) error {
	maks, err := bayiLimiti(ctx, db, bayiUserID, "trafik_kota_mb")
	if err != nil || maks <= 0 {
		return nil
	}
	var kullanilanKB int64
	_ = db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(d.trafik_kb), 0)
		FROM domains d JOIN customers c ON c.id = d.customer_id
		WHERE c.owner_user_id = ?`, bayiUserID).Scan(&kullanilanKB)
	kullanilanMB := int(kullanilanKB / 1024)
	if kullanilanMB >= maks {
		return &LimitHatasi{Mesaj: fmt.Sprintf(
			"bayi trafik kotası dolu: %d MB / %d MB", kullanilanMB, maks)}
	}
	return nil
}

// bayiLimiti: reseller_limits'ten tek bir sayısal limiti okur.
// Satır yoksa 0 (sınırsız) döner.
func bayiLimiti(ctx context.Context, db *sql.DB, bayiUserID int64, kolon string) (int, error) {
	// kolon adı yalnız bu paketten sabit string olarak gelir (SQL enjeksiyonu
	// yüzeyi yok); yine de beklenen değerlerle sınırlanır.
	switch kolon {
	case "max_customer", "max_domain", "disk_kota_mb", "trafik_kota_mb":
	default:
		return 0, fmt.Errorf("bilinmeyen limit kolonu: %s", kolon)
	}
	var v int
	err := db.QueryRowContext(ctx,
		`SELECT `+kolon+` FROM reseller_limits WHERE user_id=?`, bayiUserID).Scan(&v)
	if err != nil {
		return 0, nil // satır yok = sınırsız
	}
	return v, nil
}

// CheckDomainEklenebilir: customer_id varsa onun plan.max_domain'ine bak
func CheckDomainEklenebilir(ctx context.Context, db *sql.DB, customerID *int64) error {
	if customerID == nil {
		return nil // admin için sınır yok
	}
	var planID *int64
	if err := db.QueryRowContext(ctx, `SELECT plan_id FROM customers WHERE id=?`, *customerID).Scan(&planID); err != nil {
		return nil
	}
	if planID == nil {
		return nil
	}
	var maks int
	if err := db.QueryRowContext(ctx, `SELECT max_domain FROM service_plans WHERE id=?`, *planID).Scan(&maks); err != nil {
		return nil
	}
	if maks <= 0 {
		return nil // sınırsız
	}
	var mevcut int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM domains WHERE customer_id=?`, *customerID).Scan(&mevcut)
	if mevcut >= maks {
		return &LimitHatasi{Mesaj: fmt.Sprintf("plan limiti aşıldı: max %d domain", maks)}
	}
	return nil
}

// CheckDBEklenebilir: domain'in customer plan.max_db
func CheckDBEklenebilir(ctx context.Context, db *sql.DB, domainID int64) error {
	var customerID *int64
	if err := db.QueryRowContext(ctx, `SELECT customer_id FROM domains WHERE id=?`, domainID).Scan(&customerID); err != nil {
		return nil
	}
	if customerID == nil {
		return nil
	}
	var planID *int64
	_ = db.QueryRowContext(ctx, `SELECT plan_id FROM customers WHERE id=?`, *customerID).Scan(&planID)
	if planID == nil {
		return nil
	}
	var maks int
	_ = db.QueryRowContext(ctx, `SELECT max_db FROM service_plans WHERE id=?`, *planID).Scan(&maks)
	if maks <= 0 {
		return nil
	}
	var mevcut int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM db_accounts a JOIN domains d ON d.id=a.domain_id WHERE d.customer_id=?`,
		*customerID).Scan(&mevcut)
	if mevcut >= maks {
		return &LimitHatasi{Mesaj: fmt.Sprintf("plan limiti aşıldı: max %d veritabanı", maks)}
	}
	return nil
}

// CheckMailboxEklenebilir: domain'in customer plan.max_email limitini kontrol eder.
func CheckMailboxEklenebilir(ctx context.Context, db *sql.DB, domainID int64) error {
	var customerID *int64
	if err := db.QueryRowContext(ctx, `SELECT customer_id FROM domains WHERE id=?`, domainID).Scan(&customerID); err != nil {
		return nil
	}
	if customerID == nil {
		return nil
	}
	var planID *int64
	_ = db.QueryRowContext(ctx, `SELECT plan_id FROM customers WHERE id=?`, *customerID).Scan(&planID)
	if planID == nil {
		return nil
	}
	var maks int
	_ = db.QueryRowContext(ctx, `SELECT max_email FROM service_plans WHERE id=?`, *planID).Scan(&maks)
	if maks <= 0 {
		return nil
	}
	var mevcut int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mailboxes m JOIN domains d ON d.id=m.domain_id WHERE d.customer_id=?`,
		*customerID).Scan(&mevcut)
	if mevcut >= maks {
		return &LimitHatasi{Mesaj: fmt.Sprintf("plan limiti aşıldı: max %d e-posta kutusu", maks)}
	}
	return nil
}
