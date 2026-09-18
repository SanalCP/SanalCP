package jailpath

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Opt-in because this test creates two disposable system identities. It never
// changes an existing account or uses a real tenant directory.
func TestEntegrasyonTenantCopyPinned(t *testing.T) {
	if os.Getenv("SANALCP_TENANT_IT") != "1" || os.Geteuid() != 0 {
		t.Skip("SANALCP_TENANT_IT=1 and root required")
	}
	users := []string{fmt.Sprintf("c_it%x_a", time.Now().UnixNano()), fmt.Sprintf("c_it%x_b", time.Now().UnixNano())}
	for _, sk := range users {
		if out, err := exec.Command("useradd", "-M", "-U", "-d", "/nonexistent", "-s", "/usr/sbin/nologin", sk).CombinedOutput(); err != nil {
			t.Fatalf("useradd: %v: %s", err, out)
		}
		t.Cleanup(func() {
			if out, err := exec.Command("userdel", sk).CombinedOutput(); err != nil {
				t.Errorf("userdel: %v: %s", err, out)
			}
		})
	}
	for _, swap := range []bool{false, true} {
		t.Run(fmt.Sprintf("swap=%v", swap), func(t *testing.T) {
			base := t.TempDir()
			sourcePath, targetPath := filepath.Join(base, "source"), filepath.Join(base, "target")
			outside := filepath.Join(base, "outside")
			for _, p := range []string{sourcePath, targetPath, outside} {
				if err := os.Mkdir(p, 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(outside, "private"), []byte("untouched"), 0600); err != nil {
				t.Fatal(err)
			}
			for i, p := range []string{sourcePath, targetPath} {
				uid, gid, _ := TenantIDs(users[i])
				if err := os.Chown(p, uid, gid); err != nil {
					t.Fatal(err)
				}
			}
			uid, gid, _ := TenantIDs(users[0])
			if err := os.WriteFile(filepath.Join(sourcePath, "config.php"), []byte("private source config"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chown(filepath.Join(sourcePath, "config.php"), uid, gid); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(sourcePath, "link")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(targetPath, "obsolete"), []byte("old"), 0644); err != nil {
				t.Fatal(err)
			}
			source, err := AcDizin(base, "source")
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			target, err := AcDizin(base, "target")
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			actualTarget := targetPath
			if swap {
				for _, p := range []string{sourcePath, targetPath} {
					if err := os.Rename(p, p+"-pinned"); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(outside, p); err != nil {
						t.Fatal(err)
					}
				}
				actualTarget += "-pinned"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := copyPinnedTrees(ctx, users[0], source, users[1], target); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(actualTarget, "config.php"))
			if err != nil || string(b) != "private source config" {
				t.Fatalf("copy: %q %v", b, err)
			}
			var st unix.Stat_t
			if err := unix.Stat(filepath.Join(actualTarget, "config.php"), &st); err != nil {
				t.Fatal(err)
			}
			targetUID, _, _ := TenantIDs(users[1])
			if st.Uid != uint32(targetUID) {
				t.Fatalf("owner %d, want %d", st.Uid, targetUID)
			}
			if _, err := os.Lstat(filepath.Join(actualTarget, "obsolete")); !os.IsNotExist(err) {
				t.Fatalf("obsolete file remained: %v", err)
			}
			if _, err := os.Readlink(filepath.Join(actualTarget, "link")); err != nil {
				t.Fatal("symlink was not preserved:", err)
			}
			b, err = os.ReadFile(filepath.Join(outside, "private"))
			if err != nil || string(b) != "untouched" {
				t.Fatalf("outside changed: %q %v", b, err)
			}
			entries, _ := os.ReadDir(outside)
			if len(entries) != 1 {
				t.Fatal("copy escaped pinned destination")
			}
		})
	}
	t.Run("package publication pins destination", func(t *testing.T) {
		sk := users[1]
		home, _ := TenantHome(sk)
		if err := os.Mkdir(home, 0755); err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(home)
		uid, gid, _ := TenantIDs(sk)
		if err := os.Chown(home, uid, gid); err != nil {
			t.Fatal(err)
		}
		if err := DizinOlustur(home, "public_html", sk); err != nil {
			t.Fatal(err)
		}
		outside := t.TempDir()
		if err := os.WriteFile(filepath.Join(outside, "index.php"), []byte("outside"), 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := PaketYukle(ctx, sk, filepath.Join(home, "public_html"), func(stage string) error {
			st, err := os.Stat(stage)
			if err != nil {
				return err
			}
			if st.Mode().Perm() != 0700 {
				t.Fatal("package stage is not private")
			}
			if err := os.WriteFile(filepath.Join(stage, "index.php"), []byte("package"), 0644); err != nil {
				return err
			}
			if err := os.Rename(filepath.Join(home, "public_html"), filepath.Join(home, "pinned")); err != nil {
				return err
			}
			return os.Symlink(outside, filepath.Join(home, "public_html"))
		})
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(home, "pinned", "index.php"))
		if err != nil || string(b) != "package" {
			t.Fatalf("package: %q %v", b, err)
		}
		b, err = os.ReadFile(filepath.Join(outside, "index.php"))
		if err != nil || string(b) != "outside" {
			t.Fatalf("escaped: %q %v", b, err)
		}
		st, err := os.Stat(filepath.Join(home, "pinned"))
		if err != nil || st.Mode().Perm() != 0755 {
			t.Fatalf("web root permissions: %v %v", st, err)
		}
		called := false
		if err := PaketYukle(ctx, sk, filepath.Join(home, "public_html"), func(string) error { called = true; return nil }); err == nil || called {
			t.Fatal("initial symlink was accepted")
		}
		if err := YeniDizin(home, "data", sk, 0750); err != nil {
			t.Fatal(err)
		}
		if err := YeniDizin(home, "data", sk, 0750); !os.IsExist(err) {
			t.Fatalf("existing data directory accepted: %v", err)
		}
	})
}

func TestTenantCommandRejectsRootAndMissingIdentity(t *testing.T) {
	f, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, sk := range []string{"root", "../root", "c_nonexistent_copy_test"} {
		if _, err := tenantTreeCommand(context.Background(), sk, f, "-cf", "-", "."); err == nil {
			t.Fatalf("accepted %q", sk)
		}
	}
}
