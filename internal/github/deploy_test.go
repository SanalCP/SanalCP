package github

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
	"sanalcp/internal/secretcrypt"
)

func TestCloneURLRejectsHostAndPathInjection(t *testing.T) {
	for _, name := range []string{"owner/repo", "a/repo.name", "my-org/my_repo"} {
		got, err := CloneURL(name)
		if err != nil || got != "https://github.com/"+name+".git" {
			t.Fatalf("%q: %q %v", name, got, err)
		}
	}
	for _, name := range []string{"owner/../evil", "owner/..", "owner/.", "owner/repo?x=y", "owner/repo#x", "owner/repo\n", "owner/repo@evil.test", "//evil.test/repo", "owner/repo%2fother"} {
		if _, err := CloneURL(name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}

func TestDeployTokenScopeAndEncryption(t *testing.T) {
	old := box
	t.Cleanup(func() { box = old })
	box, _ = secretcrypt.New([32]byte{1})
	encrypted, _ := patSifrele("test-pat-never-log")
	if encrypted == "test-pat-never-log" {
		t.Fatal("PAT not encrypted")
	}
	for _, tc := range []struct {
		name, repo, selected, stored, want string
		query, fail                        bool
	}{
		{"selected", "https://github.com/owner/repo.git", "owner/repo", encrypted, "test-pat-never-log", true, false},
		{"different repo", "https://github.com/other/repo.git", "owner/repo", encrypted, "", true, false},
		{"other host", "https://evil.test/owner/repo.git", "owner/repo", encrypted, "", false, false},
		{"userinfo", "https://pat@github.com/owner/repo.git", "owner/repo", encrypted, "", false, false},
		{"plaintext", "https://github.com/owner/repo.git", "owner/repo", "plaintext", "", true, true},
		{"corrupt", "https://github.com/owner/repo.git", "owner/repo", "v1:broken", "", true, true},
		{"disconnected", "https://github.com/owner/repo.git", "", "", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			if tc.query {
				q := mock.ExpectQuery("SELECT pat, secili_repo").WithArgs(int64(42))
				if tc.name == "disconnected" {
					q.WillReturnError(sql.ErrNoRows)
				} else {
					q.WillReturnRows(sqlmock.NewRows([]string{"pat", "repo"}).AddRow(tc.stored, tc.selected))
				}
			}
			got, err := DeployToken(context.Background(), db, 42, tc.repo)
			if got != tc.want || (err != nil) != tc.fail {
				t.Fatalf("credential result mismatch, error=%v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
	box = nil
	if _, err := patSifrele("secret"); err == nil {
		t.Fatal("missing encryption key accepted")
	}
}

func TestUseStoresCleanURL(t *testing.T) {
	old := box
	t.Cleanup(func() { box = old })
	box, _ = secretcrypt.New([32]byte{2})
	encrypted, _ := patSifrele("test-pat-never-log")
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT sistem_kullanici, is_demo").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"sk", "demo"}).AddRow("c_test", 0))
	mock.ExpectQuery("SELECT pat FROM github_connections").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"pat"}).AddRow(encrypted))
	mock.ExpectQuery("SELECT COALESCE").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"secret"}).AddRow("existing-webhook-secret"))
	mock.ExpectExec("INSERT INTO git_repos").WithArgs(int64(42), "https://github.com/owner/repo.git", "main", "public_html", "existing-webhook-secret").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE github_connections SET secili_repo").WithArgs("owner/repo", "main", int64(42)).WillReturnResult(sqlmock.NewResult(1, 1))
	r := chi.NewRouter()
	r.Post("/{id}", (&Handlers{DB: db}).Use)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/42", strings.NewReader(`{"repo":"owner/repo"}`)))
	if w.Code != 200 || strings.Contains(w.Body.String(), "test-pat") || strings.Contains(w.Body.String(), encrypted) {
		t.Fatalf("unsafe/failed response: status=%d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
