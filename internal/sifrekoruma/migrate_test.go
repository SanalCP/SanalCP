package sifrekoruma

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMigrationDatabaseFailurePreservesLegacyPasswords(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "d1_a_b")
	passwords := "alice:hashA\nbob:hashB\n"
	if err := os.WriteFile(legacy, []byte(passwords), 0644); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT k.id").WillReturnRows(sqlmock.NewRows([]string{"id", "domain_id", "sk", "php", "yol", "kullanici", "file"}).
		AddRow(1, 1, "c_test", "8.3", "/a-b", "alice", legacy).
		AddRow(2, 1, "c_test", "8.3", "/a/b", "bob", legacy))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE korumali_dizinler").WillReturnError(errors.New("DB unavailable"))
	mock.ExpectRollback()
	if _, err := migrateFiles(context.Background(), db, dir); err == nil {
		t.Fatal("DB failure ignored")
	}
	b, err := os.ReadFile(legacy)
	if err != nil || string(b) != passwords {
		t.Fatalf("retry cannot recover passwords: %q %v", b, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordFileDoesNotAliasPaths(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range []string{"/", "/root", "/a-b", "/a/b", "/a_b", "/a.b", "/a//b"} {
		f := passwordFile("/etc/nginx/htpasswd", 1, p)
		if seen[f] {
			t.Fatalf("collision: %s", p)
		}
		seen[f] = true
		if f == passwordFile("/etc/nginx/htpasswd", 2, p) {
			t.Fatal("domain collision")
		}
	}
}

func TestSplitLegacyFiltersUsersAndLocksAmbiguousPasswords(t *testing.T) {
	entries := []protectedEntry{
		{domainID: 1, path: "/a-b", user: "alice"},
		{domainID: 1, path: "/a/b", user: "bob"},
		{domainID: 1, path: "/a-b", user: "shared"},
		{domainID: 1, path: "/a/b", user: "shared"},
	}
	files, locked := splitLegacyPasswords("/tmp", entries, []byte("alice:hashA\nbob:hashB\nshared:overwritten\nremoved:hashC\n"))
	if locked != 2 {
		t.Fatalf("ambiguous passwords not locked: %d", locked)
	}
	a := string(files[passwordFile("/tmp", 1, "/a-b")])
	b := string(files[passwordFile("/tmp", 1, "/a/b")])
	if a != "alice:hashA\nshared:!\n" || b != "bob:hashB\nshared:!\n" {
		t.Fatalf("incorrect isolation: %q %q", a, b)
	}
}

func TestMigrationSeparatesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "d1_a_b")
	if err := os.WriteFile(legacy, []byte("alice:hashA\nbob:hashB\n"), 0644); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows := sqlmock.NewRows([]string{"id", "domain_id", "sk", "php", "yol", "kullanici", "file"}).
		AddRow(1, 1, "c_test", "8.3", "/a-b", "alice", legacy).
		AddRow(2, 1, "c_test", "8.3", "/a/b", "bob", legacy)
	mock.ExpectQuery("SELECT k.id").WillReturnRows(rows)
	mock.ExpectBegin()
	for i, p := range []string{"/a-b", "/a/b"} {
		mock.ExpectExec(regexp.QuoteMeta("UPDATE korumali_dizinler SET htpasswd_dosya=? WHERE id=?")).WithArgs(passwordFile(dir, 1, p), i+1).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
	if _, err := migrateFiles(context.Background(), db, dir); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{"/a-b": "alice:hashA\n", "/a/b": "bob:hashB\n"} {
		b, err := os.ReadFile(passwordFile(dir, 1, p))
		if err != nil || string(b) != want {
			t.Fatalf("%s: %q %v", p, b, err)
		}
	}
	b, err := os.ReadFile(legacy)
	if err != nil || len(b) != 0 {
		t.Fatal("old worker still accepts shared passwords")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	// On a restart migrated rows need a render, but hashes are not rewritten.
	mock.ExpectQuery("SELECT k.id").WillReturnRows(sqlmock.NewRows([]string{"id", "domain_id", "sk", "php", "yol", "kullanici", "file"}).AddRow(1, 1, "c_test", "8.3", "/a-b", "alice", passwordFile(dir, 1, "/a-b")))
	entries, err := migrateFiles(context.Background(), db, dir)
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(passwordFile(dir, 1, "/a-b"))
	if !strings.Contains(string(b), "hashA") {
		t.Fatal("retry lost password")
	}
}
