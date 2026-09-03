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
		{"laravel", "DB_HOST=mysql\nDB_DATABASE=old_db\nDB_USERNAME=old\nDB_PASSWORD=oldpass\n", rewriteDotEnvDB, []string{`DB_HOST=127.0.0.1`, `DB_DATABASE="c_site_main"`, `DB_USERNAME="c_site_db"`, `DB_PASSWORD="p\$a\\ss"`}},
		{"symfony", `DATABASE_URL="mysql://old:pass@localhost:3306/old_db?serverVersion=8.0"`, rewriteDotEnvDB, []string{`mysql://c_site_db:p$a%5Css@127.0.0.1:3306/c_site_main?serverVersion=8.0`}},
		{"custom-php-const", `const DB_HOST = 'localhost'; const DB_NAME = 'old_db'; const DB_USER = 'old'; const DB_PASS = 'pass';`, rewritePHPConstDB, []string{`DB_HOST = '127.0.0.1'`, `DB_NAME = 'c_site_main'`, `DB_USER = 'c_site_db'`, `DB_PASS = 'p$a\\ss'`}},
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
