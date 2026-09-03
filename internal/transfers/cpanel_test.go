package transfers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"strings"
	"testing"
)

type testEntry struct {
	name string
	body string
	typ  byte
}

func archive(t *testing.T, entries ...testEntry) *bytes.Reader {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0600, Size: int64(len(e.body)), Typeflag: typ}); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			_, _ = tw.Write([]byte(e.body))
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return bytes.NewReader(b.Bytes())
}

func TestAnalyzeCPanelInventory(t *testing.T) {
	r := archive(t,
		testEntry{name: "backup-demo/cp/userdata/demo/main", body: "main_domain: example.com\n"},
		testEntry{name: "backup-demo/homedir/public_html/index.php", body: "<?php echo 1;"},
		testEntry{name: "backup-demo/mysql/demo_wp.sql", body: "CREATE TABLE x(id int);"},
		testEntry{name: "backup-demo/dnszones/example.com.db", body: "$TTL 3600"},
		testEntry{name: "backup-demo/homedir/mail/example.com/a", body: "mail"},
		testEntry{name: "backup-demo/homedir/etc/example.com/shadow", body: "info:$6$hash:1::::\ndestek:$6$hash:1::::\n"},
		testEntry{name: "backup-demo/va/example.com", body: "sales: external@example.net\n"},
		testEntry{name: "backup-demo/cron", body: "# WordPress görevi\n*/5 * * * * /usr/bin/php /home/demo/public_html/wp-cron.php\nMAILTO=demo@example.com\n@reboot /bin/true\n"},
		testEntry{name: "backup-demo/sslcerts/example.com.crt", body: "certificate"},
	)
	got, err := AnalyzeCPanel(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "cpanel" || got.Username != "demo" || got.PrimaryDomain != "example.com" {
		t.Fatalf("unexpected identity: %+v", got)
	}
	if got.WebFiles != 1 || len(got.Databases) != 1 || got.MailFiles != 1 || !got.CronPresent {
		t.Fatalf("unexpected inventory: %+v", got)
	}
	if len(got.Mailboxes) != 2 || got.AliasCount != 1 {
		t.Fatalf("unexpected mail inventory: %+v", got)
	}
	if got.SSLCerts != 1 {
		t.Fatalf("unexpected SSL inventory: %+v", got)
	}
	if len(got.CronJobs) != 1 || got.CronJobs[0].Minute != "*/5" ||
		got.CronJobs[0].Command != "/usr/bin/php /home/demo/public_html/wp-cron.php" {
		t.Fatalf("unexpected cron inventory: %+v", got)
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "2 cron satırı") {
		t.Fatalf("unsupported cron warning missing: %+v", got.Warnings)
	}
}

func TestAnalyzeRejectsTraversal(t *testing.T) {
	r := archive(t, testEntry{name: "backup-demo/../../etc/shadow", body: "x"})
	_, err := AnalyzeCPanel(r)
	if !errors.Is(err, ErrUnsafeArchive) {
		t.Fatalf("want ErrUnsafeArchive, got %v", err)
	}
}

func TestAnalyzeRejectsSymlink(t *testing.T) {
	r := archive(t, testEntry{name: "backup-demo/homedir/public_html/link", typ: tar.TypeSymlink})
	_, err := AnalyzeCPanel(r)
	if !errors.Is(err, ErrUnsafeArchive) {
		t.Fatalf("want ErrUnsafeArchive, got %v", err)
	}
}

func TestDatabaseMappingsAreNamespacedAndUnique(t *testing.T) {
	got := databaseMappings(
		[]string{"old_wp", "old-shop", "old shop"},
		"c_example_com", "c_example_com_main", "c_example_com_db",
	)
	if len(got) != 3 {
		t.Fatalf("unexpected mappings: %+v", got)
	}
	if got[0].Target != "c_example_com_main" {
		t.Fatalf("first DB must use default: %+v", got[0])
	}
	if got[1].Target != "c_example_com_old_shop" || got[2].Target != "c_example_com_old_shop_2" {
		t.Fatalf("targets must be sanitized and unique: %+v", got)
	}
	for _, m := range got {
		if !strings.HasPrefix(m.Target, "c_example_com_") || len(m.Target) > 64 {
			t.Fatalf("unsafe target: %+v", m)
		}
	}
}

func TestRewriteApplicationDatabaseConfigs(t *testing.T) {
	maps := []DBMap{{Source: "old_db", Target: "c_site_main"}}
	tests := []struct {
		name  string
		in    string
		fn    dbConfigRewriter
		wants []string
	}{
		{"laravel", "DB_HOST=mysql\nDB_DATABASE=old_db\nDB_USERNAME=old\nDB_PASSWORD=oldpass\n", rewriteDotEnvDB, []string{`DB_HOST=localhost`, `DB_DATABASE="c_site_main"`, `DB_USERNAME="c_site_db"`, `DB_PASSWORD="p\$a\\ss"`}},
		{"symfony", `DATABASE_URL="mysql://old:pass@localhost:3306/old_db?serverVersion=8.0"`, rewriteDotEnvDB, []string{`mysql://c_site_db:p$a%5Css@localhost:3306/c_site_main?serverVersion=8.0`}},
		{"custom-php-const", `const DB_HOST = 'localhost'; const DB_NAME = 'old_db'; const DB_USER = 'old'; const DB_PASS = 'pass';`, rewritePHPConstDB, []string{`DB_HOST = 'localhost'`, `DB_NAME = 'c_site_main'`, `DB_USER = 'c_site_db'`, `DB_PASS = 'p$a\\ss'`}},
		{"prestashop-modern", `return ['parameters'=>['database_host'=>'localhost','database_name'=>'old_db','database_user'=>'old','database_password'=>'pass']];`, rewritePrestaParametersDB, []string{`'database_name'=>'c_site_main'`, `'database_user'=>'c_site_db'`, `'database_password'=>'p$`}},
		{"prestashop-legacy", `define('_DB_SERVER_', 'localhost'); define('_DB_NAME_', 'old_db'); define('_DB_USER_', 'old'); define('_DB_PASSWD_', 'pass');`, rewritePrestaLegacyDB, []string{`define('_DB_NAME_', 'c_site_main')`, `define('_DB_USER_', 'c_site_db')`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed, err := tt.fn([]byte(tt.in), maps, "c_site_db", `p$a\ss`)
			if err != nil || !changed {
				t.Fatalf("changed=%v err=%v", changed, err)
			}
			for _, want := range tt.wants {
				if !strings.Contains(string(got), want) {
					t.Errorf("%q yok; çıktı: %s", want, got)
				}
			}
		})
	}
}

// Regresyon: replacePHPValue eşleşmenin tamamını değiştirdiği için desenler
// sonlandırıcı ';' kapsarsa noktalı virgüller silinir; config.php parse error
// verir ve aktarılan site HTTP 500'e düşer.
func TestRewritePHPConstDBNoktaliVirgulleriKorur(t *testing.T) {
	in := "<?php\nconst DB_HOST = 'localhost';\nconst DB_NAME = 'old_db';\nconst DB_USER = 'old';\nconst DB_PASS = 'pass';\nconst DB_CHARSET = 'utf8mb4';\n"
	got, changed, err := rewritePHPConstDB([]byte(in), []DBMap{{Source: "old_db", Target: "c_site_main"}}, "c_site_db", "gizli")
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	for _, want := range []string{
		"const DB_HOST = 'localhost';",
		"const DB_NAME = 'c_site_main';",
		"const DB_USER = 'c_site_db';",
		"const DB_PASS = 'gizli';",
		"const DB_CHARSET = 'utf8mb4';",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("%q yok; çıktı:\n%s", want, got)
		}
	}
	if a, b := strings.Count(in, ";"), strings.Count(string(got), ";"); a != b {
		t.Errorf("noktalı virgül sayısı değişti: %d -> %d; çıktı:\n%s", a, b, got)
	}
}

func TestParseCronJobsCapsAndPreservesFiveFieldTasks(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 101; i++ {
		body.WriteString("0 2 * * 1 /bin/echo ok\n")
	}
	got, skipped := parseCronJobs(body.String())
	if len(got) != 100 || skipped != 1 {
		t.Fatalf("unexpected cron cap: jobs=%d skipped=%d", len(got), skipped)
	}
}

// tar'ın ölümcül nedeni çıktının SONUNDA olur; uyarı seli baştan kırpılınca
// gerçek hata (ör. kota) mesajdan tamamen düşüyordu.
func TestTarHataOzetiUyariSeliniAtipGercekNedeniKorur(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString("tar: var/cache/prod/x: time stamp 2027-08-31 20:31:55 is 31323162.2 s in the future\n")
	}
	b.WriteString("tar: public_html/img/big.bin: Cannot write: Disk quota exceeded\n")
	b.WriteString("tar: Exiting with failure status due to previous errors\n")

	got := tarHataOzeti([]byte(b.String()), errors.New("exit status 2"))
	if !strings.Contains(got, "Disk quota exceeded") {
		t.Fatalf("gerçek neden kayboldu: %q", got)
	}
	if strings.Contains(got, "in the future") {
		t.Fatalf("uyarı seli sızdı: %q", got)
	}
}

func TestTarHataOzetiSadeceGurultuVarsaHamCiktiyaDuser(t *testing.T) {
	got := tarHataOzeti([]byte("tar: a: time stamp is 5 s in the future\n"), errors.New("exit status 2"))
	if got == "" {
		t.Fatal("boş özet")
	}
}

func TestHomeAtlananlarOzetiGuvensizAdlariEler(t *testing.T) {
	ekler := arsivEkler{nativeHomeAtla: "m", uyeler: map[string][]byte{
		"m": []byte("src\nvendor\n\n../etc\nkotu; rm -rf /\nsql\n"),
	}}
	got := homeAtlananlarOzeti(ekler)
	for _, want := range []string{"src/", "vendor/", "sql/"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q yok: %s", want, got)
		}
	}
	for _, kotu := range []string{"..", "rm -rf"} {
		if strings.Contains(got, kotu) {
			t.Errorf("güvensiz ad sızdı (%s): %s", kotu, got)
		}
	}
	if bos := homeAtlananlarOzeti(arsivEkler{uyeler: map[string][]byte{}}); bos != "" {
		t.Fatalf("üye yokken özet üretildi: %q", bos)
	}
}

// Panel MySQL hesabını YALNIZ '<kullanıcı>@localhost' açar; yapılandırmaya
// 127.0.0.1 yazmak TCP'ye zorlar ve uygulama 1698 "Access denied" alır.
func TestDBHostAsla127OlmazHepsiLocalhost(t *testing.T) {
	maps := []DBMap{{Source: "old_db", Target: "c_site_main"}}
	girdiler := []struct {
		ad string
		in string
		fn dbConfigRewriter
	}{
		{"const", "<?php\nconst DB_HOST = '10.0.0.9';\nconst DB_NAME = 'old_db';\n", rewritePHPConstDB},
		{"define", "<?php\ndefine('DB_HOST', '10.0.0.9');\ndefine('DB_NAME', 'old_db');\n", rewritePHPDefineDB},
		{"dizi", "<?php\nreturn ['db_host' => '10.0.0.9', 'db_name' => 'old_db'];\n", rewritePHPArrayDB},
		{"dotenv", "DB_HOST=10.0.0.9\nDB_DATABASE=old_db\n", rewriteDotEnvDB},
		{"presta-legacy", "<?php define('_DB_SERVER_', '10.0.0.9'); define('_DB_NAME_', 'old_db');", rewritePrestaLegacyDB},
	}
	for _, g := range girdiler {
		t.Run(g.ad, func(t *testing.T) {
			got, changed, err := g.fn([]byte(g.in), maps, "c_site_db", "gizli")
			if err != nil || !changed {
				t.Fatalf("changed=%v err=%v", changed, err)
			}
			if strings.Contains(string(got), "127.0.0.1") {
				t.Errorf("127.0.0.1 yazıldı: %s", got)
			}
			if !strings.Contains(string(got), "localhost") {
				t.Errorf("localhost yok: %s", got)
			}
		})
	}
}

// panel.boomptstudio.com: define() biçimi tanınmadığı için kaynak parolasıyla
// kalıyordu. pole.secureserver.tr: dizi biçimi aynı şekilde atlanıyordu.
func TestDefineVeDiziBicimleriDBBilgileriniUyarlar(t *testing.T) {
	maps := []DBMap{{Source: "eski_db", Target: "c_site_main"}}

	def := "<?php\ndefine('DB_HOST', 'localhost');\ndefine('DB_NAME', 'eski_db');\ndefine('DB_USER', 'eski_kul');\ndefine('DB_PASS', 'eskiparola');\ndefine('DB_CHARSET', 'utf8mb4');\n"
	got, changed, err := rewritePHPDefineDB([]byte(def), maps, "c_site_db", "yeniparola")
	if err != nil || !changed {
		t.Fatalf("define: changed=%v err=%v", changed, err)
	}
	for _, want := range []string{"define('DB_NAME', 'c_site_main')", "define('DB_USER', 'c_site_db')", "define('DB_PASS', 'yeniparola')", "define('DB_CHARSET', 'utf8mb4')"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("define: %q yok; çıktı: %s", want, got)
		}
	}
	if strings.Contains(string(got), "eskiparola") || strings.Contains(string(got), "eski_kul") {
		t.Errorf("define: kaynak kimliği kaldı: %s", got)
	}

	dizi := "<?php\nreturn [\n    'db_host'    => 'localhost',\n    'db_name'    => 'eski_db',\n    'db_user'    => 'eski_kul',\n    'db_pass'    => 'eskiparola',\n    'db_charset' => 'utf8mb4',\n];\n"
	got, changed, err = rewritePHPArrayDB([]byte(dizi), maps, "c_site_db", "yeniparola")
	if err != nil || !changed {
		t.Fatalf("dizi: changed=%v err=%v", changed, err)
	}
	for _, want := range []string{"'db_name'    => 'c_site_main'", "'db_user'    => 'c_site_db'", "'db_pass'    => 'yeniparola'", "'db_charset' => 'utf8mb4'"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("dizi: %q yok; çıktı: %s", want, got)
		}
	}
	if strings.Contains(string(got), "eskiparola") {
		t.Errorf("dizi: kaynak parolası kaldı: %s", got)
	}
}

// DB_PASS deseni DB_PASSWORD'ü yutmamalı (aksi hâlde biri bozulurdu).
func TestDefineDBPassDesenisDBPasswordUYutmaz(t *testing.T) {
	in := "<?php define('DB_NAME','eski_db'); define('DB_PASSWORD','eskiparola');"
	got, changed, err := rewritePHPDefineDB([]byte(in), []DBMap{{Source: "eski_db", Target: "c_site_main"}}, "u", "yeni")
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if !strings.Contains(string(got), "define('DB_PASSWORD','yeni')") {
		t.Fatalf("DB_PASSWORD uyarlanmadı: %s", got)
	}
}
