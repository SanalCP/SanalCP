package nextcloud

import (
	"archive/tar"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func tarBZ2Olustur(t *testing.T, girdiler map[string]string) string {
	t.Helper()
	d := t.TempDir()
	ham := filepath.Join(d, "paket.tar")
	f, err := os.Create(ham)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	for ad, icerik := range girdiler {
		b := []byte(icerik)
		if err := tw.WriteHeader(&tar.Header{Name: ad, Mode: 0o644, Size: int64(len(b)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("bzip2"); err != nil {
		t.Skip("bzip2 test aracı yok")
	}
	if out, err := exec.Command("bzip2", ham).CombinedOutput(); err != nil {
		t.Fatalf("bzip2: %v: %s", err, out)
	}
	return ham + ".bz2"
}

func TestTarBZ2AcKokDiziniSoyer(t *testing.T) {
	arsiv := tarBZ2Olustur(t, map[string]string{"nextcloud/occ": "cli", "nextcloud/index.php": "site"})
	hedef := t.TempDir()
	if err := tarBZ2Ac(arsiv, hedef); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(hedef, "index.php")); err != nil || string(b) != "site" {
		t.Fatalf("çıktı=%q hata=%v", b, err)
	}
}

func TestTarBZ2AcDizinAsiminiReddeder(t *testing.T) {
	arsiv := tarBZ2Olustur(t, map[string]string{"nextcloud/occ": "cli", "nextcloud/../../kacak": "x"})
	if err := tarBZ2Ac(arsiv, t.TempDir()); err == nil {
		t.Fatal("dizin aşımı kabul edildi")
	}
}

func TestTarBZ2AcKokDisiHardlinkiReddeder(t *testing.T) {
	d := t.TempDir()
	ham := filepath.Join(d, "hardlink.tar")
	f, err := os.Create(ham)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	if err := tw.WriteHeader(&tar.Header{Name: "nextcloud/occ", Mode: 0o644, Size: 3, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("cli")); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "nextcloud/kacak", Linkname: "../../etc/passwd", Typeflag: tar.TypeLink}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("bzip2", ham).CombinedOutput(); err != nil {
		t.Fatalf("bzip2: %v: %s", err, out)
	}
	if err := tarBZ2Ac(ham+".bz2", t.TempDir()); err == nil {
		t.Fatal("kök dışı hardlink kabul edildi")
	}
}

func TestTarBZ2AcGuvenliSymlinkiDosyayaDonusturur(t *testing.T) {
	d := t.TempDir()
	ham := filepath.Join(d, "symlink.tar")
	f, err := os.Create(ham)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	for _, h := range []*tar.Header{
		{Name: "nextcloud/occ", Mode: 0o644, Size: 3, Typeflag: tar.TypeReg},
		{Name: "nextcloud/dist/lisans", Mode: 0o644, Size: 3, Typeflag: tar.TypeReg},
		{Name: "nextcloud/dist/lisans-kopya", Linkname: "lisans", Typeflag: tar.TypeSymlink},
	} {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := tw.Write([]byte("abc")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("bzip2", ham).CombinedOutput(); err != nil {
		t.Fatalf("bzip2: %v: %s", err, out)
	}
	hedef := t.TempDir()
	if err := tarBZ2Ac(ham+".bz2", hedef); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(filepath.Join(hedef, "dist", "lisans-kopya"))
	if err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("bağlantı normal dosyaya dönüştürülmedi: fi=%v err=%v", fi, err)
	}
}

func TestIndirVeDogrulaResmiPaket(t *testing.T) {
	if os.Getenv("NEXTCLOUD_CANLI_TEST") == "" {
		t.Skip("NEXTCLOUD_CANLI_TEST ayarlanmadı")
	}
	hedef := t.TempDir()
	surum, err := indirVeDogrula(context.Background(), hedef)
	if err != nil {
		t.Fatal(err)
	}
	if got := yerelSurumOku(hedef); !strings.HasPrefix(got, surum) {
		t.Fatalf("indirilen=%q bildirilen=%q", got, surum)
	}
}
