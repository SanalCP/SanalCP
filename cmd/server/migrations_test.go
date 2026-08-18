package main

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	sanaldb "sanalcp/internal/db"
)

// Migration smoke testi.
//
// NEDEN VAR: 0.7.0 ile yayınlanan 0068_api_tokenlari.sql, users.id BIGINT UNSIGNED
// iken FK kolonunu BIGINT (signed) tanımlıyordu. InnoDB foreign key'de imza
// uyuşmazlığını reddeder (errno 150) → migration başarısız → runMigrations hata
// döner → main log.Fatalf ile ölür → PANEL HİÇ AÇILMAZ. Hiçbir teste takılmadı
// çünkü migration'lar test sırasında hiç çalıştırılmıyordu; hata ancak gerçek bir
// sunucu güncellenince ortaya çıktı.
//
// Bu test migration'ları SIFIRDAN, gerçek MariaDB üzerinde uygular ve
// runMigrations'ın TA KENDİSİNİ çağırır — çalıştırıcının ayrıştırma mantığı
// (yorum atma, ';' ile bölme, checksum defteri) testte KOPYALANMAZ. Kopyalansaydı
// test yeşil kalırken üretim patlayabilirdi; nitekim ilk taslakta öyle olmuştu:
// kopya ayrıştırıcı her ifade için yeni bağlantı açtığı için 0001'deki
// `USE panel` etkisini hiç görmüyordu.
//
// 🔴 MİGRATION'LAR SABİT OLARAK `panel` VERİTABANINI HEDEFLER (0001_init.sql
// içinde CREATE DATABASE + USE panel). Bu yüzden test rastgele adlı bir DB'de
// çalışamaz. Makinede ZATEN bir `panel` veritabanı varsa test ATLANIR — canlı
// veriye asla dokunmaz. Temiz bir MariaDB'de (CI konteyneri, tek kullanımlık
// instance) `panel` oluşturulur, migration'lar koşar ve DB silinir.
//
// ÇALIŞTIRMA: DSN otomatik bulunur (unix socket) ya da MIGRASYON_TEST_DSN ile
// verilir; DSN veritabanı adı İÇERMEMELİDİR. MariaDB yoksa test SKIP olur.

const testDBAdi = "panel"

// testDSN: sunucu-seviyesi DSN (veritabanı adı YOK).
func testDSN() string {
	if v := strings.TrimSpace(os.Getenv("MIGRASYON_TEST_DSN")); v != "" {
		return v
	}
	for _, sock := range []string{
		"/run/mysqld/mysqld.sock",     // Debian/Ubuntu
		"/var/lib/mysql/mysql.sock",   // RHEL ailesi
		"/var/run/mysqld/mysqld.sock", // eski Debian düzeni
	} {
		if _, err := os.Stat(sock); err == nil {
			return "root@unix(" + sock + ")/"
		}
	}
	return ""
}

// migrationTestDB: boş bir `panel` veritabanı hazırlar ve temizliğini kaydeder.
// Mevcut bir `panel` varsa testi ATLAR (canlı veri korunur).
func migrationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := testDSN()
	if dsn == "" {
		t.Skip("MariaDB bulunamadı — MIGRASYON_TEST_DSN ile sunucu DSN'i verin")
	}
	sunucu, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("sql.Open: %v", err)
	}
	if err := sunucu.Ping(); err != nil {
		sunucu.Close()
		t.Skipf("MariaDB'ye bağlanılamadı (%s): %v", dsn, err)
	}
	defer func() { _ = sunucu.Close() }()

	var varMi string
	err = sunucu.QueryRow(
		"SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME=?", testDBAdi).Scan(&varMi)
	if err == nil {
		t.Skipf("bu sunucuda zaten bir %q veritabanı var — canlı veriye dokunmamak için atlanıyor "+
			"(temiz bir MariaDB'ye MIGRASYON_TEST_DSN ile yönlendirin)", testDBAdi)
	}
	if err != sql.ErrNoRows {
		t.Skipf("%q veritabanı sorgulanamadı: %v", testDBAdi, err)
	}

	if _, err := sunucu.Exec("CREATE DATABASE " + testDBAdi); err != nil {
		t.Fatalf("test için %q oluşturulamadı: %v", testDBAdi, err)
	}
	temizle := func() {
		temizSunucu, err := sql.Open("mysql", dsn)
		if err != nil {
			t.Logf("temizlik bağlantısı açılamadı: %v", err)
			return
		}
		defer temizSunucu.Close()
		if _, err := temizSunucu.Exec("DROP DATABASE IF EXISTS " + testDBAdi); err != nil {
			t.Logf("test veritabanı silinemedi (%s): %v", testDBAdi, err)
		}
	}

	// Üretimdeki bağlantı kurulumunun aynısı (havuz boyutu, ping) kullanılır.
	d, err := sanaldb.Open(dsn + testDBAdi)
	if err != nil {
		temizle()
		t.Fatalf("test veritabanına bağlanılamadı: %v", err)
	}
	t.Cleanup(func() { d.Close(); temizle() })
	return d
}

// depoMigrationDizini: testin süresince migrationsDir'i depodaki migrations/ dizinine
// çevirir. Üretim sabiti (/opt/sanalcp/src/migrations) test bitince geri gelir.
func depoMigrationDizini(t *testing.T) {
	t.Helper()
	eski := migrationsDir
	migrationsDir = "../../migrations"
	t.Cleanup(func() { migrationsDir = eski })
}

// sqlDosyaSayisi: migrations dizinindeki .sql dosya adedi.
func sqlDosyaSayisi(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("migration dizini okunamadı: %v", err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			n++
		}
	}
	return n
}

// TestMigrationlarSifirDBdeUygulanir: depodaki TÜM migration'lar boş bir veritabanına
// baştan sona hatasız uygulanmalı. Bu test kırmızıysa o sürüm, kurulan/güncellenen
// HER sunucuda paneli açılmaz hale getirir.
func TestMigrationlarSifirDBdeUygulanir(t *testing.T) {
	d := migrationTestDB(t)
	depoMigrationDizini(t)

	if err := runMigrations(d); err != nil {
		t.Fatalf("migration'lar sıfır DB'de uygulanamadı: %v", err)
	}

	dosya := sqlDosyaSayisi(t)
	var kayit int
	if err := d.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&kayit); err != nil {
		t.Fatalf("schema_migrations okunamadı: %v", err)
	}
	if kayit != dosya {
		t.Errorf("schema_migrations %d kayıt içeriyor, dizinde %d .sql var", kayit, dosya)
	}
	t.Logf("%d migration uygulandı", kayit)
}

// TestMigrationlarIdempotent: ikinci çalıştırma hiçbir şey yapmadan temiz dönmeli.
// Panel her restart'ta runMigrations çağırır; buradaki bir hata restart'ın paneli
// öldürmesi demektir. Checksum defterinin kararlılığını da doğrular — 0.7.0
// kesintisi tam olarak checksum uyuşmazlığından tetiklenmişti (dosya yayın sonrası
// değişmişti).
func TestMigrationlarIdempotent(t *testing.T) {
	d := migrationTestDB(t)
	depoMigrationDizini(t)

	if err := runMigrations(d); err != nil {
		t.Fatalf("birinci koşu: %v", err)
	}
	if err := runMigrations(d); err != nil {
		t.Fatalf("ikinci koşu (idempotent olmalı): %v", err)
	}
}
