package git

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTemizleDizinIcerigiJailDisinaCikamaz: gerçek sömürü senaryosunun
// regresyon testi.
//
// Saldırı (0.4.0 öncesi): tenant kendi home'unda `ln -s /etc ~/pwn` yapıp
// target_dir="pwn" ile POST /domains/{id}/git/klonla çağırırdı. Eski kod
// os.ReadDir + os.RemoveAll ile YOL üzerinden çalıştığı ve panel root olduğu
// için symlink izlenip /etc'nin tüm içeriği ROOT OLARAK siliniyordu.
//
// Beklenen davranış: işlem hata ile reddedilir ve symlink hedefindeki hiçbir
// dosyaya DOKUNULMAZ.
func TestTemizleDizinIcerigiJailDisinaCikamaz(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home_c_kurban")
	disari := filepath.Join(base, "etc_taklidi")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(disari, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, ad := range []string{"passwd", "shadow", "nginx.conf"} {
		if err := os.WriteFile(filepath.Join(disari, ad), []byte("kritik"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Tenant'ın kurduğu kaçış symlink'i — target_dir="pwn" gecerliTargetDir'den GEÇER.
	if err := os.Symlink(disari, filepath.Join(home, "pwn")); err != nil {
		t.Fatal(err)
	}

	if err := temizleDizinIcerigi(home, "pwn"); err == nil {
		t.Error("symlink hedefi temizlenmeye çalışıldı, hata bekleniyordu")
	}

	girdiler, err := os.ReadDir(disari)
	if err != nil {
		t.Fatalf("jail dışı dizin okunamadı: %v", err)
	}
	if len(girdiler) != 3 {
		t.Fatalf("jail DIŞINDAKİ dosyalar silindi: %d/3 kaldı", len(girdiler))
	}
}

// Normal (symlink'siz) hedefte temizleme yine çalışmalı — güvenlik düzeltmesi
// meşru kullanımı bozmamalı.
func TestTemizleDizinIcerigiNormalHedefiTemizler(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home_c_test")
	hedef := filepath.Join(home, "public_html")
	if err := os.MkdirAll(filepath.Join(hedef, "alt"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"index.php", ".htaccess", "alt/x.txt"} {
		if err := os.WriteFile(filepath.Join(hedef, p), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := temizleDizinIcerigi(home, "public_html"); err != nil {
		t.Fatalf("normal hedef temizlenemedi: %v", err)
	}
	girdiler, err := os.ReadDir(hedef)
	if err != nil {
		t.Fatal(err)
	}
	if len(girdiler) != 0 {
		t.Errorf("dotfile dahil içerik silinmeliydi, %d girdi kaldı", len(girdiler))
	}
}
