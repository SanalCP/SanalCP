// Package panelbayrak: panel_ayarlari tablosundaki davranış bayraklarının
// salt-okunur erişimi.
//
// NEDEN AYRI PAKET: bu bayrağı üç yer okuyor — internal/auth (giriş kapısı),
// internal/users (son-admin sayımı) ve internal/panelayarlari (HTTP uçları).
// Okuyucu panelayarlari'nda dursaydı auth ve users o paketi import etmek
// zorunda kalırdı; panelayarlari ise provisioner'ı çekiyor (Let's Encrypt,
// os/exec) — auth gibi bir kimlik doğrulama paketine taşınacak bir yük değil,
// ayrıca provisioner ileride auth'a ihtiyaç duyarsa import cycle olurdu.
// Bu paketin iç bağımlılığı SIFIR ve öyle kalmalı.
package panelbayrak

import (
	"context"
	"database/sql"
)

// RootGirisiAcik — panelin root/shadow giriş yolu açık mı?
//
// FAIL-CLOSED: bayrak okunamıyorsa false (root reddedilir). Kaybedilen bir şey
// yok — panel_ayarlari okunamıyorsa users tablosu da okunamaz, yani alternatif
// giriş yolu zaten çalışmıyordur. Aynı handler'daki 2FA durumu okumasının
// fail-closed kararıyla tutarlı (bkz. internal/auth/handlers.go Login).
func RootGirisiAcik(ctx context.Context, db *sql.DB) bool {
	var acik int
	if err := db.QueryRowContext(ctx,
		`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).Scan(&acik); err != nil {
		return false
	}
	return acik == 1
}
