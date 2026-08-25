// Package hesaplar: FTP hesabi + MySQL DB hesabi olusturma yardimcilari
package hesaplar

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"sanalcp/internal/adlar"
	"sanalcp/internal/secretcrypt"

	"github.com/go-sql-driver/mysql"

	"sanalcp/internal/osfam"
)

// box: db_pass_plain gibi panel metadata'sındaki parolaları şifreler.
// main.go'da Init() ile bir kez ayarlanır (bkz. middleware.Init/provisioner.Init
// ile aynı paket-seviyesi singleton deseni).
var box *secretcrypt.Box

// rootDB: MariaDB'ye ROOT olarak — CREATE/ALTER/DROP USER, GRANT gibi panelin
// kendi düşük-yetkili 'panel' bağlantısıyla YAPILAMAYACAK işlemler için.
// Unix socket + boş parola: sanalcp-install.sh'nin "mysql -u root <<SQL" ile
// birebir aynı mekanizma — taze bir MariaDB kurulumunda root@localhost parola
// İSTEMEZ, ama YALNIZ unix socket üzerinden ('root'@'127.0.0.1' hiç yok, TCP
// erişimi reddedilir — gerçek sunucuda doğrulandı). Önceden bu işlemler her
// çağrıda `mysql` CLI process'i başlatılarak yapılıyordu; artık kalıcı bir
// bağlantı havuzu üzerinden native driver ile yapılıyor.
var rootDB *sql.DB

// rootSocket: MariaDB unix soketi — dağıtım paketlemesine göre çözülür
// (RHEL /var/lib/mysql/mysql.sock · Debian /run/mysqld/mysqld.sock).
// Sabit bırakıldığında panel Debian'da hiç açılmıyordu.
var rootSocket = osfam.MariaDBSoket()

// Init: panelin sır şifreleme kutusunu ayarlar VE root MySQL bağlantısını açar.
// main.go'da config.Load() sonrası, herhangi bir hesap oluşturma isteğinden
// ÖNCE çağrılmalıdır. Panelin ana 'panel' bağlantısı bu noktaya gelmeden önce
// zaten kurulmuş (aynı MariaDB'ye) olduğu için ayrı bir bekle-tekrar-dene
// döngüsüne gerek yok.
func Init(b *secretcrypt.Box) error {
	box = b
	cfg := mysql.NewConfig()
	cfg.User = "root"
	cfg.Net = "unix"
	cfg.Addr = rootSocket
	d, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return fmt.Errorf("root mysql open: %w", err)
	}
	d.SetMaxOpenConns(4)
	d.SetMaxIdleConns(2)
	d.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.PingContext(ctx); err != nil {
		d.Close()
		return fmt.Errorf("root mysql ping: %w", err)
	}
	rootDB = d
	return nil
}

// rootExecAll: sırayla root olarak çalıştırır; ilk hata anında durur.
func rootExecAll(stmts ...string) error {
	for _, s := range stmts {
		if _, err := rootDB.Exec(s); err != nil {
			return fmt.Errorf("mysql: %w", err)
		}
	}
	return nil
}

// DecryptDBPassword: db_accounts.db_pass_plain sütunundaki şifreli değeri
// gösterme/phpMyAdmin-SSO için çözer. Değer henüz göç edilmemiş eski bir
// düz-metin kayıtsa (HealLegacyPlaintextSecrets boot'ta göçürür, ama ara bir
// istek yetişirse) OLDUĞU GİBİ döner — görüntüleme o pencerede kırılmasın diye.
func DecryptDBPassword(enc string) (string, error) {
	if !secretcrypt.IsEncrypted(enc) {
		return enc, nil
	}
	return box.Decrypt(enc)
}

// RandomParola: URL-safe alphanumeric parola (default 20 karakter).
//
// İki incelik — ikisi de "sessizce zayıf parola üretme" riskine karşı:
//
//  1. rand.Read hatası YUTULMAZ. Go 1.23'te crypto/rand.Read hâlâ hata
//     dönebilir (hiç dönmeme garantisi 1.24 ile geldi); yutulsaydı tampon
//     sıfır kalır ve parola sabit "AAAA..." olurdu. Zayıf parola üretmektense
//     panikliyoruz: chimw.Recoverer isteği 500'e çevirir, panel ayakta kalır
//     (Go 1.24'ün crypto/rand'ı da aynı şeyi yapar).
//  2. Doğrudan modulo yerine reddetme örneklemesi. 256 % 56 = 32 olduğundan
//     `b[i] % 56` alfabenin ilk 32 karakterini diğerlerinden %25 daha olası
//     kılardı; eşiğin (224) üstündeki baytları atıp yeniden çekiyoruz.
func RandomParola(n int) string {
	if n <= 0 {
		n = 20
	}
	const harf = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
	const esik = 256 - (256 % len(harf)) // 224
	out := make([]byte, 0, n)
	buf := make([]byte, n)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			panic("crypto/rand okunamadı, parola üretilemiyor: " + err.Error())
		}
		for _, c := range buf {
			if int(c) >= esik {
				continue // biaslı kuyruk — at, yeniden çek
			}
			out = append(out, harf[int(c)%len(harf)])
			if len(out) == n {
				break
			}
		}
	}
	return string(out)
}

var reDBKimlik = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// reDBSonek: musteri-verdigi DB/kullanici SONEKI (panel `<sk>_` onekini kendisi ekler).
// Yalniz kucuk harf/rakam/alt-cizgi, 1-32 karakter. Onek eklendikten sonra toplam <=64
// olmasi ayrica GecerliDBKimlik ile dogrulanir.
var reDBSonek = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// GecerliDBKimlik: MySQL identifier (db/kullanici adi) guvenli mi? backtick/tirnak/bosluk yok => SQLi kapali
func GecerliDBKimlik(s string) bool {
	return reDBKimlik.MatchString(s)
}

// GecerliDBSonek: musteri sonek girdisi guvenli mi? (onek eklemeden ONCE dogrulanir)
func GecerliDBSonek(s string) bool {
	return reDBSonek.MatchString(s)
}

// ParolaGucluMu: musteri DB parolasi yeterince guclu mu? >=12 karakter + karisik
// (en az bir harf ve bir rakam) + tek satir. UI'de gosterilmek uzere Turkce neden dondurur.
func ParolaGucluMu(pw string) (bool, string) {
	if !ParolaGecerli(pw) {
		return false, "parola geçersiz karakter (satır sonu/kontrol) içeriyor"
	}
	if len([]rune(pw)) < 12 {
		return false, "parola en az 12 karakter olmalı"
	}
	var harf, rakam bool
	for _, r := range pw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			harf = true
		case r >= '0' && r <= '9':
			rakam = true
		}
	}
	if !harf || !rakam {
		return false, "parola harf ve rakam içermeli (karışık)"
	}
	return true, ""
}

// MusteriDBKimlikGecerli: musteri-verdigi ad guvenli VE domain kullanicisiyla namespaced mi?
func MusteriDBKimlikGecerli(sk, s string) bool {
	if !GecerliDBKimlik(s) {
		return false
	}
	return s == sk || strings.HasPrefix(s, sk+"_")
}

// sqlKac: MySQL string-literal ('...') icin kacis (ters-bolu + tek-tirnak)
func sqlKac(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

// ParolaGecerli: parola tek-satir mi? chpasswd/mysql satir-enjeksiyonunu engeller.
func ParolaGecerli(pw string) bool {
	return !strings.ContainsAny(pw, "\r\n\x00")
}

// FTPCreate: ftp_accounts tablosuna kayit ekler. Parola DB'ye yescrypt ($y$)
// hash'i olarak yazılır — düz metin hiçbir zaman diske inmez (bkz. yescrypt.go).
// Pure-FTPd `MYSQLCrypt crypt` modu ve SyncSSHPassword'un `chpasswd -e`
// çağrısı bu hash'i doğrudan kullanır.
func FTPCreate(db *sql.DB, domainID int64, sistemKullanici, parola string, uidN, gidN int) error {
	hash, err := yescryptHashFTP(parola)
	if err != nil {
		return fmt.Errorf("ftp parola hash: %w", err)
	}
	home := "/home/" + sistemKullanici
	_, err = db.Exec(
		`INSERT INTO ftp_accounts(domain_id, username, password_md5, home_dir, uid_n, gid_n, status)
		 VALUES(?,?,?,?,?,?, 'active')
		 ON DUPLICATE KEY UPDATE password_md5=VALUES(password_md5), home_dir=VALUES(home_dir), uid_n=VALUES(uid_n), gid_n=VALUES(gid_n), status='active'`,
		domainID, sistemKullanici, hash, home, uidN, gidN)
	return err
}

// FTPUpdatePassword: mevcut FTP hesabinin parolasini guncelle (yescrypt hash olarak).
func FTPUpdatePassword(db *sql.DB, sistemKullanici, parola string) error {
	hash, err := yescryptHashFTP(parola)
	if err != nil {
		return fmt.Errorf("ftp parola hash: %w", err)
	}
	_, err = db.Exec(
		`UPDATE ftp_accounts SET password_md5=? WHERE username=?`,
		hash, sistemKullanici)
	return err
}

// FTPDelete: hesabi ve domain ile birlikte CASCADE silinir, ama yine de explicit
func FTPDelete(db *sql.DB, sistemKullanici string) error {
	_, err := db.Exec(`DELETE FROM ftp_accounts WHERE username=?`, sistemKullanici)
	return err
}

// MySQLCreateDB: MariaDB'de yeni veritabani + kullanici olustur + GRANT, sonra db_accounts'a kaydet
func MySQLCreateDB(db *sql.DB, domainID int64, dbName, dbUser, dbPass string) error {
	if !GecerliDBKimlik(dbName) || !GecerliDBKimlik(dbUser) {
		return fmt.Errorf("güvenlik: geçersiz veritabanı adı veya kullanıcısı")
	}
	// 1) MariaDB'de DB + user create (root, native driver — bkz. Init/rootDB)
	if err := rootExecAll(
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", dbName),
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s'", dbUser, sqlKac(dbPass)),
		fmt.Sprintf("ALTER USER '%s'@'localhost' IDENTIFIED BY '%s'", dbUser, sqlKac(dbPass)),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'", dbName, dbUser),
		"FLUSH PRIVILEGES",
	); err != nil {
		return err
	}

	// 2) panel DB metadata — db_pass_plain AES-GCM ile şifreli tutulur (bkz.
	// internal/secretcrypt); gerçek MySQL auth mysql.user'ın kendi hash'iyle
	// yapılır, bu sütun yalnız panelin gösterme/PMA-SSO için tuttuğu bir kopyadır.
	encPass, encErr := box.Encrypt(dbPass)
	if encErr != nil {
		return fmt.Errorf("db parola şifreleme: %w", encErr)
	}
	_, err := db.Exec(
		`INSERT INTO db_accounts(domain_id, db_name, db_user, db_pass_plain, db_host)
		 VALUES(?,?,?,?, 'localhost')`,
		domainID, dbName, dbUser, encPass)
	return err
}

// MySQLCreateDBForUser: yeni DB olustur + MEVCUT bir DB-kullanicisina GRANT ver
// (kullanicinin parolasina DOKUNMAZ — baska DB'leri bozmaz). db_accounts'a bu domain+db
// icin mevcut kullanicinin parolasiyla (db_pass_plain) yeni satir ekler.
// Cagiran, dbUser'in bu domaine ait oldugunu ONCEDEN dogrulamalidir (sahiplik + onek).
func MySQLCreateDBForUser(db *sql.DB, domainID int64, dbName, dbUser string) error {
	if !GecerliDBKimlik(dbName) || !GecerliDBKimlik(dbUser) {
		return fmt.Errorf("güvenlik: geçersiz veritabanı adı veya kullanıcısı")
	}
	// Mevcut kullanicinin parolasi (yeni db_accounts satiri icin — phpMyAdmin SSO).
	var pass string
	if err := db.QueryRow(
		`SELECT db_pass_plain FROM db_accounts WHERE db_user=? LIMIT 1`, dbUser).Scan(&pass); err != nil {
		return fmt.Errorf("mevcut kullanıcı parolası bulunamadı: %w", err)
	}
	// DB olustur + mevcut kullaniciya GRANT (CREATE/ALTER USER YOK → parola korunur).
	if err := rootExecAll(
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", dbName),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'", dbName, dbUser),
		"FLUSH PRIVILEGES",
	); err != nil {
		return err
	}
	_, err := db.Exec(
		`INSERT INTO db_accounts(domain_id, db_name, db_user, db_pass_plain, db_host)
		 VALUES(?,?,?,?, 'localhost')`,
		domainID, dbName, dbUser, pass)
	return err
}

// MySQLDropDB: DB ve user kaldir + metadata sil
func MySQLDropDB(db *sql.DB, dbName, dbUser string) error {
	if !GecerliDBKimlik(dbName) || !GecerliDBKimlik(dbUser) {
		return fmt.Errorf("güvenlik: geçersiz veritabanı adı veya kullanıcısı")
	}
	if err := rootExecAll(
		fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName),
		fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost'", dbUser),
		"FLUSH PRIVILEGES",
	); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM db_accounts WHERE db_name=?`, dbName)
	return err
}

// MySQLDropDBKeepUser: yalniz DB'yi kaldir + metadata satirini sil; kullaniciya DOKUNMA.
// Kullanici baska DB'lerde de kullaniliyorsa (mevcut-kullanici modu) onlari bozmamak icin
// tek-DB silmede kullanilir.
func MySQLDropDBKeepUser(db *sql.DB, dbName string) error {
	if !GecerliDBKimlik(dbName) {
		return fmt.Errorf("güvenlik: geçersiz veritabanı adı")
	}
	if err := rootExecAll(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName)); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM db_accounts WHERE db_name=?`, dbName)
	return err
}

// MySQLGrantNewUser: var olan bir DB'ye YENİ bir kullanıcı olustur + GRANT ver
// (CREATE DATABASE YAPMAZ — DB zaten var olmalı). db_accounts'a yeni satır ekler.
func MySQLGrantNewUser(db *sql.DB, domainID int64, dbName, dbUser, dbPass string) error {
	if !GecerliDBKimlik(dbName) || !GecerliDBKimlik(dbUser) {
		return fmt.Errorf("güvenlik: geçersiz veritabanı adı veya kullanıcısı")
	}
	if err := rootExecAll(
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s'", dbUser, sqlKac(dbPass)),
		fmt.Sprintf("ALTER USER '%s'@'localhost' IDENTIFIED BY '%s'", dbUser, sqlKac(dbPass)),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'", dbName, dbUser),
		"FLUSH PRIVILEGES",
	); err != nil {
		return err
	}
	encPass, encErr := box.Encrypt(dbPass)
	if encErr != nil {
		return fmt.Errorf("db parola şifreleme: %w", encErr)
	}
	_, err := db.Exec(
		`INSERT INTO db_accounts(domain_id, db_name, db_user, db_pass_plain, db_host)
		 VALUES(?,?,?,?, 'localhost')`,
		domainID, dbName, dbUser, encPass)
	return err
}

// MySQLGrantExistingUser: var olan bir DB'ye, ZATEN var olan bir kullanıcıya GRANT ver
// (CREATE/ALTER USER YOK → parola korunur). Çağıran, dbUser'ın bu domaine ait olduğunu
// ÖNCEDEN doğrulamalıdır (sahiplik + önek). Çağıran ayrıca db_accounts'tan mevcut
// parolayı okuyup yanıtta gösterebilir (bu fonksiyon parolayı döndürmez).
func MySQLGrantExistingUser(db *sql.DB, domainID int64, dbName, dbUser string) error {
	if !GecerliDBKimlik(dbName) || !GecerliDBKimlik(dbUser) {
		return fmt.Errorf("güvenlik: geçersiz veritabanı adı veya kullanıcısı")
	}
	var pass string
	if err := db.QueryRow(
		`SELECT db_pass_plain FROM db_accounts WHERE db_user=? LIMIT 1`, dbUser).Scan(&pass); err != nil {
		return fmt.Errorf("mevcut kullanıcı parolası bulunamadı: %w", err)
	}
	if err := rootExecAll(
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'", dbName, dbUser),
		"FLUSH PRIVILEGES",
	); err != nil {
		return err
	}
	_, err := db.Exec(
		`INSERT INTO db_accounts(domain_id, db_name, db_user, db_pass_plain, db_host)
		 VALUES(?,?,?,?, 'localhost')`,
		domainID, dbName, dbUser, pass)
	return err
}

// MySQLRevokeUser: kullanicinin bir DB uzerindeki erisimini kaldirir (DB'nin
// kendisine DOKUNMAZ). dropUser=true ise kullanici MariaDB'den tamamen silinir
// (baska hicbir DB'de kullanilmiyorsa cagiran bunu ONCEDEN kontrol etmelidir);
// false ise yalniz bu DB uzerindeki GRANT geri alinir, kullanici yasamaya devam eder.
func MySQLRevokeUser(db *sql.DB, dbName, dbUser string, dropUser bool) error {
	if !GecerliDBKimlik(dbName) || !GecerliDBKimlik(dbUser) {
		return fmt.Errorf("güvenlik: geçersiz veritabanı adı veya kullanıcısı")
	}
	stmts := []string{fmt.Sprintf("REVOKE ALL PRIVILEGES ON `%s`.* FROM '%s'@'localhost'", dbName, dbUser)}
	if dropUser {
		stmts = append(stmts, fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost'", dbUser))
	}
	stmts = append(stmts, "FLUSH PRIVILEGES")
	if err := rootExecAll(stmts...); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM db_accounts WHERE db_name=? AND db_user=?`, dbName, dbUser)
	return err
}

// MySQLRenameDB: MySQL'de native RENAME DATABASE yok — CREATE + mysqldump|mysql
// taşıma (view/trigger/routine/event de taşınır, RENAME TABLE döngüsünün aksine)
// + grant taşıma + DROP ile taklit edilir. Herhangi bir adım basarisiz olursa
// yeni DB best-effort silinir, eski DB adim 4'e kadar DOKUNULMAMIŞ kalır.
func MySQLRenameDB(ctx context.Context, db *sql.DB, domainID int64, eskiAd, yeniAd string, kullanicilar []string) error {
	if !GecerliDBKimlik(eskiAd) || !GecerliDBKimlik(yeniAd) {
		return fmt.Errorf("güvenlik: geçersiz veritabanı adı")
	}
	for _, u := range kullanicilar {
		if !GecerliDBKimlik(u) {
			return fmt.Errorf("güvenlik: geçersiz kullanıcı adı")
		}
	}

	temizle := func() { _ = rootExecAll(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", yeniAd)) }

	if err := rootExecAll(
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", yeniAd),
	); err != nil {
		return err
	}

	dump := exec.CommandContext(ctx, "mysqldump", "--single-transaction", "--routines", "--triggers", "--events", eskiAd)
	restore := exec.CommandContext(ctx, "mysql", yeniAd)
	pipe, err := dump.StdoutPipe()
	if err != nil {
		temizle()
		return fmt.Errorf("pipe: %w", err)
	}
	restore.Stdin = pipe
	var dumpErr, restoreErr strings.Builder
	dump.Stderr = &dumpErr
	restore.Stderr = &restoreErr

	if err := restore.Start(); err != nil {
		temizle()
		return fmt.Errorf("mysql start: %w", err)
	}
	if err := dump.Run(); err != nil {
		_ = restore.Wait()
		temizle()
		return fmt.Errorf("mysqldump %s: %s: %w", eskiAd, strings.TrimSpace(dumpErr.String()), err)
	}
	if err := restore.Wait(); err != nil {
		temizle()
		return fmt.Errorf("mysql %s: %s: %w", yeniAd, strings.TrimSpace(restoreErr.String()), err)
	}

	grants := make([]string, 0, len(kullanicilar)*2+1)
	for _, u := range kullanicilar {
		grants = append(grants,
			fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'", yeniAd, u),
			fmt.Sprintf("REVOKE ALL PRIVILEGES ON `%s`.* FROM '%s'@'localhost'", eskiAd, u))
	}
	grants = append(grants, "FLUSH PRIVILEGES")
	if err := rootExecAll(grants...); err != nil {
		temizle()
		return fmt.Errorf("grant/revoke: %w", err)
	}

	if err := rootExecAll(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", eskiAd)); err != nil {
		return fmt.Errorf("yeni veritabanı aktif ama eski silinemedi (elle temizleyin): %w", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE db_accounts SET db_name=? WHERE db_name=? AND domain_id=?`, yeniAd, eskiAd, domainID); err != nil {
		return fmt.Errorf("metadata güncelleme: %w", err)
	}
	return nil
}

// MySQLDropAllForDomain: domain silinince ona ait tum DB'leri kaldir
func MySQLDropAllForDomain(db *sql.DB, domainID int64) error {
	rows, err := db.Query(`SELECT db_name, db_user FROM db_accounts WHERE domain_id=?`, domainID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var dbName, dbUser string
		if err := rows.Scan(&dbName, &dbUser); err != nil {
			continue
		}
		_ = MySQLDropDB(db, dbName, dbUser)
	}
	return nil
}

// SyncSSHPassword: sistem (SSH) hesabının parolasını FTP parolasıyla eşitler.
// ftp_accounts.password_md5 artık DÜZ METİN değil, yescrypt ($y$) hash'i
// tutuyor (bkz. FTPCreate/FTPUpdatePassword) — bu hash `chpasswd -e` ile
// ÇÖZÜLMEDEN doğrudan /etc/shadow'a yazılır, panel süreci parolayı hiçbir
// zaman görmez.
func SyncSSHPassword(db *sql.DB, sistemKullanici string) error {
	if !adlar.SKGecerli(sistemKullanici) {
		return fmt.Errorf("güvenlik: c_ prefiksli olmayan kullanıcı")
	}
	var pw string
	if err := db.QueryRow(
		`SELECT password_md5 FROM ftp_accounts WHERE username=? AND status='active'`,
		sistemKullanici).Scan(&pw); err != nil {
		return fmt.Errorf("ftp parola oku: %w", err)
	}
	if strings.TrimSpace(pw) == "" {
		return fmt.Errorf("ftp parolası boş")
	}
	if !ParolaGecerli(pw) {
		return fmt.Errorf("güvenlik: parola geçersiz karakter içeriyor")
	}
	if !isYescryptHash(pw) {
		return fmt.Errorf("güvenlik: ftp parolası henüz hash'e göç etmemiş (bkz. HealLegacyPlaintextSecrets)")
	}
	cmd := exec.Command("chpasswd", "-e")
	cmd.Stdin = strings.NewReader(sistemKullanici + ":" + pw)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chpasswd: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// LockSSHPassword: SSH kapatıldığında sistem parolasını kilitler (passwd -l).
func LockSSHPassword(sistemKullanici string) error {
	if !adlar.SKGecerli(sistemKullanici) {
		return fmt.Errorf("güvenlik: c_ prefiksli olmayan kullanıcı")
	}
	out, err := exec.Command("passwd", "-l", sistemKullanici).CombinedOutput()
	if err != nil {
		return fmt.Errorf("passwd -l: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
