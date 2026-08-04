package sqlimport

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Entegrasyon testi: GERÇEK bir MariaDB sunucusuna karşı çalışır.
//
// Neden gerekli: bu paketin tüm güvenlik iddiası ("dump ne içerirse içersin
// hedef şemanın dışına çıkamaz") MariaDB'nin yetki denetimine dayanır ve
// yalnız gerçek sunucuda doğrulanabilir. Ayrıca mysqldump'ın DEFINER'lı
// çıktısının süzgeçten geçtikten sonra düşük yetkili kullanıcıyla içe
// aktarılabildiğini de elle yazılmış SQL değil, ancak gerçek dump gösterir.
//
// Çalıştırmak için (root + MariaDB gereklidir):
//
//	SANALCP_SQLIMPORT_IT=1 go test ./internal/sqlimport/ -run Entegrasyon -v
//
// Test kendi şemasını/kullanıcısını oluşturur ve sonunda TAMAMEN temizler.
const (
	itKaynakDB = "zz_sqlimport_it_src"
	itHedefDB  = "zz_sqlimport_it_dst"
	itKullanan = "zz_sqlimport_it_u"
	itParola   = `It1_p"a\s$ w#d`
	itArkakapi = "zz_sqlimport_it_pwn"
)

func rootSQL(t *testing.T, stmts string) {
	t.Helper()
	cmd := exec.Command("mysql", "-uroot")
	cmd.Stdin = strings.NewReader(stmts)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("root SQL başarısız: %v: %s", err, stderr.String())
	}
}

func rootSorgu(t *testing.T, sorgu string) string {
	t.Helper()
	out, err := exec.Command("mysql", "-uroot", "-N", "-B", "-e", sorgu).Output()
	if err != nil {
		t.Fatalf("root sorgu başarısız: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func itTemizle(t *testing.T) {
	t.Helper()
	rootSQL(t, "DROP DATABASE IF EXISTS `"+itKaynakDB+"`;\n"+
		"DROP DATABASE IF EXISTS `"+itHedefDB+"`;\n"+
		"DROP USER IF EXISTS '"+itKullanan+"'@'localhost';\n"+
		"DROP USER IF EXISTS '"+itArkakapi+"'@'%';\n"+
		"FLUSH PRIVILEGES;\n")
}

func itHazirla(t *testing.T) Hedef {
	t.Helper()
	if os.Getenv("SANALCP_SQLIMPORT_IT") != "1" {
		t.Skip("entegrasyon testi: SANALCP_SQLIMPORT_IT=1 ile çalıştırın")
	}
	if _, err := exec.LookPath("mysql"); err != nil {
		t.Skip("mysql istemcisi yok")
	}
	if os.Geteuid() != 0 {
		t.Skip("root gerekli (mysql -uroot soket erişimi)")
	}
	itTemizle(t)
	t.Cleanup(func() { itTemizle(t) })

	rootSQL(t, "CREATE DATABASE `"+itHedefDB+"`;\n"+
		"CREATE USER '"+itKullanan+"'@'localhost' IDENTIFIED BY '"+
		strings.ReplaceAll(strings.ReplaceAll(itParola, `\`, `\\`), `'`, `\'`)+"';\n"+
		"GRANT ALL PRIVILEGES ON `"+itHedefDB+"`.* TO '"+itKullanan+"'@'localhost';\n"+
		"FLUSH PRIVILEGES;\n")
	return Hedef{DBAdi: itHedefDB, Kullanici: itKullanan, Parola: itParola, Host: "localhost"}
}

// Gerçek mysqldump çıktısı (tablo + view + trigger + procedure, hepsi
// DEFINER=`root`@`localhost` damgalı) düşük yetkili kullanıcıyla içe
// aktarılabilmeli.
func TestEntegrasyonGercekMysqldumpIceAktarilir(t *testing.T) {
	hedef := itHazirla(t)

	rootSQL(t, "CREATE DATABASE `"+itKaynakDB+"`;\n"+
		"USE `"+itKaynakDB+"`;\n"+
		"CREATE TABLE urun (id INT PRIMARY KEY, ad VARCHAR(40), fiyat INT);\n"+
		"INSERT INTO urun VALUES (1,'kalem',10),(2,'defter',20);\n"+
		"CREATE VIEW ucuz AS SELECT id, ad FROM urun WHERE fiyat < 15;\n"+
		"CREATE TRIGGER urun_bi BEFORE INSERT ON urun FOR EACH ROW SET NEW.fiyat = IFNULL(NEW.fiyat,0);\n")
	// NOT: gövdede BEGIN..END kullanılmıyor — mysql istemcisi ";" üzerinden
	// böldüğü için çok ifadeli gövde DELIMITER gerektirirdi.
	rootSQL(t, "USE `"+itKaynakDB+"`;\n"+
		"CREATE PROCEDURE toplam() SELECT SUM(fiyat) FROM urun;\n")

	dump, err := exec.Command("mysqldump", "-uroot",
		"--single-transaction", "--routines", "--triggers", itKaynakDB).Output()
	if err != nil {
		t.Fatalf("mysqldump: %v", err)
	}
	if !bytes.Contains(dump, []byte("DEFINER=")) {
		t.Fatal("test kurulumu hatalı: dump DEFINER içermiyor")
	}

	ctx, iptal := context.WithTimeout(context.Background(), 60*time.Second)
	defer iptal()
	if err := Uygula(ctx, hedef, bytes.NewReader(dump)); err != nil {
		t.Fatalf("gerçek dump içe aktarılamadı: %v", err)
	}

	kontrol := map[string]string{
		"tablo": "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + itHedefDB + "' AND TABLE_TYPE='BASE TABLE'",
		"view":  "SELECT COUNT(*) FROM information_schema.VIEWS WHERE TABLE_SCHEMA='" + itHedefDB + "'",
		"trig":  "SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA='" + itHedefDB + "'",
		"proc":  "SELECT COUNT(*) FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA='" + itHedefDB + "'",
		"satir": "SELECT COUNT(*) FROM `" + itHedefDB + "`.urun",
	}
	beklenen := map[string]string{"tablo": "1", "view": "1", "trig": "1", "proc": "1", "satir": "2"}
	for ad, sorgu := range kontrol {
		if got := rootSorgu(t, sorgu); got != beklenen[ad] {
			t.Errorf("%s: %s bekleniyordu, %s geldi", ad, beklenen[ad], got)
		}
	}

	// Nesneler artık kaynak sunucunun root'una değil, içe aktaran kullanıcıya ait.
	if def := rootSorgu(t, "SELECT DEFINER FROM information_schema.VIEWS WHERE TABLE_SCHEMA='"+itHedefDB+"'"); !strings.HasPrefix(def, itKullanan+"@") {
		t.Errorf("view DEFINER'ı içe aktaran kullanıcı olmalıydı: %q", def)
	}
}

// Asıl güvenlik iddiası: dump hedef şemanın DIŞINA çıkamaz.
func TestEntegrasyonKotuDumpReddedilir(t *testing.T) {
	hedef := itHazirla(t)
	ctx := context.Background()

	kotular := map[string]string{
		"duz USE mysql": "USE mysql;\nCREATE USER '" + itArkakapi + "'@'%' IDENTIFIED BY 'pwn';\n" +
			"GRANT ALL PRIVILEGES ON *.* TO '" + itArkakapi + "'@'%';\n",
		"yorumda gizli USE": "/*!50000 USE mysql */;\nSELECT COUNT(*) FROM user;\n",
		"nitelenmis yazma":  "INSERT INTO mysql.user (User) VALUES ('" + itArkakapi + "');\n",
		"dogrudan GRANT":    "GRANT ALL PRIVILEGES ON *.* TO '" + itKullanan + "'@'localhost' WITH GRANT OPTION;\n",
		"baska sema":        "CREATE TABLE mysql.zz_sizinti (a INT);\n",
	}
	for ad, sql := range kotular {
		if err := Uygula(ctx, hedef, strings.NewReader(sql)); err == nil {
			t.Errorf("%s: KABUL EDİLDİ — güvenlik açığı", ad)
		}
	}
	if n := rootSorgu(t, "SELECT COUNT(*) FROM mysql.user WHERE User='"+itArkakapi+"'"); n != "0" {
		t.Fatalf("arka kapı kullanıcısı oluştu (%s) — güvenlik açığı", n)
	}
	if n := rootSorgu(t, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='mysql' AND TABLE_NAME='zz_sizinti'"); n != "0" {
		t.Fatal("mysql şemasına tablo yazıldı — güvenlik açığı")
	}
	// Kullanıcının GLOBAL yetkisi genişlememiş olmalı: USER_PRIVILEGES yalnız
	// global (*.*) yetkileri listeler; tek satır "USAGE" beklenir.
	genel := rootSorgu(t, "SELECT GROUP_CONCAT(PRIVILEGE_TYPE) FROM information_schema.USER_PRIVILEGES "+
		"WHERE GRANTEE=\"'"+itKullanan+"'@'localhost'\"")
	if genel != "USAGE" {
		t.Fatalf("kullanıcı global yetki kazandı: %q", genel)
	}
}

func TestEntegrasyonTablolariSil(t *testing.T) {
	hedef := itHazirla(t)
	ctx := context.Background()

	kurulum := "CREATE TABLE a (id INT PRIMARY KEY);\n" +
		"CREATE TABLE b (id INT PRIMARY KEY, a_id INT, FOREIGN KEY (a_id) REFERENCES a(id));\n" +
		"CREATE VIEW v AS SELECT id FROM a;\n"
	if err := Uygula(ctx, hedef, strings.NewReader(kurulum)); err != nil {
		t.Fatalf("kurulum: %v", err)
	}
	if err := TablolariSil(ctx, hedef); err != nil {
		t.Fatalf("TablolariSil: %v", err)
	}
	if n := rootSorgu(t, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+itHedefDB+"'"); n != "0" {
		t.Errorf("tablo/view kalmamalıydı, %s kaldı", n)
	}
	// Şemanın kendisi DURMALI (yalnız içeriği silinir).
	if n := rootSorgu(t, "SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME='"+itHedefDB+"'"); n != "1" {
		t.Error("hedef şema silinmemeliydi")
	}
}
