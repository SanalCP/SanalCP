package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScrubRepoCredentials(t *testing.T) {
	home := t.TempDir()
	files := []string{"config", "FETCH_HEAD", "logs/HEAD", "logs/refs/heads/main"}
	for _, name := range files {
		path := filepath.Join(home, "public_html/.git", name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("url = https://legacy-secret@github.com/owner/repo.git\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := scrubRepoConfig(home, "public_html"); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range files {
		path := filepath.Join(home, "public_html/.git", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "url = https://github.com/owner/repo.git\n" {
			t.Fatalf("unexpected cleaned content: %q", data)
		}
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0600 {
			t.Fatal("permissions changed")
		}
		if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
			t.Fatal("lock left behind")
		}
	}
}

func TestScrubRefusesLinkedFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			external := filepath.Join(t.TempDir(), "secret")
			const original = "https://legacy-secret@github.com/owner/repo.git"
			if err := os.WriteFile(external, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, "public_html/.git")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			link := os.Link
			if kind == "symlink" {
				link = os.Symlink
			}
			if err := link(external, filepath.Join(dir, "config")); err != nil {
				t.Fatal(err)
			}
			if err := scrubRepoConfig(home, "public_html"); err == nil {
				t.Fatal("linked file accepted")
			}
			got, err := os.ReadFile(external)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(got)) != original {
				t.Fatal("external file modified")
			}
		})
	}
}
