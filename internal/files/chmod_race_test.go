package files

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestChmodTreeConcurrentSymlinkSwap(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if err := unix.Renameat2(fd, "a", fd, "b", unix.RENAME_EXCHANGE); err != nil {
		t.Fatal(err)
	}
	stop, done := make(chan struct{}), make(chan struct{})
	defer func() { close(stop); <-done }()
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = unix.Renameat2(fd, "a", fd, "b", unix.RENAME_EXCHANGE)
		}
	}()
	for i := 0; i < 20000; i++ {
		if err := chmodAt(fd, "a", 0750, 0644); err != nil {
			t.Fatal(err)
		}
		st, err := os.Stat(outside)
		if err != nil || st.Mode().Perm() != 0600 {
			t.Fatalf("outside permissions changed: %v %v", st, err)
		}
	}
}

func TestChmodTreeRegularDirectoriesAndSpecialFiles(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "public_html", "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(home, "public_html", "sub", "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(home, "public_html", "fifo"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := chmodTreeBeneath(home, "public_html", 0750, 0644); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]os.FileMode{"public_html": 0750, "public_html/sub": 0750, "public_html/sub/file": 0644, "public_html/fifo": 0600} {
		st, err := os.Stat(filepath.Join(home, name))
		if err != nil || st.Mode().Perm() != want {
			t.Fatalf("%s: %v %v", name, st, err)
		}
	}
}
