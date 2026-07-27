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
		testEntry{name: "backup-demo/cron", body: "* * * * * true"},
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
