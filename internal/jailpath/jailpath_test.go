package jailpath

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestTenantHome(t *testing.T) {
	if _, err := TenantHome("c_ornek"); err != nil {
		t.Errorf("geçerli tenant reddedildi: %v", err)
	}
	for _, kotu := range []string{"", "root", "c_a/../../etc", "c_a/b", "../etc", "c_a\\b"} {
		if _, err := TenantHome(kotu); err == nil {
			t.Errorf("TenantHome(%q) kabul edildi, reddedilmeliydi", kotu)
		}
	}
}

// kur: sahte bir tenant home'u + jail DIŞINDA bir hedef dizin kurar.
func kur(t *testing.T) (home, disari string) {
	t.Helper()
	base := t.TempDir()
	home = filepath.Join(base, "home_c_test")
	disari = filepath.Join(base, "DISARIDA")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(disari, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(disari, "kritik.conf"), []byte("dokunulmamalı"), 0o644); err != nil {
		t.Fatal(err)
	}
	return home, disari
}

// Asıl güvenlik iddiası: hedef bir symlink olduğunda hiçbir işlem jail
// dışına ULAŞAMAZ — sessizce başarısız olmak yerine açık hata döner.
func TestSymlinkHedefReddedilir(t *testing.T) {
	home, disari := kur(t)
	if err := os.Symlink(disari, filepath.Join(home, "public_html")); err != nil {
		t.Fatal(err)
	}

	if _, err := AcDizin(home, "public_html"); !errors.Is(err, unix.ELOOP) {
		t.Errorf("AcDizin symlink'te err=%v, ELOOP bekleniyordu", err)
	}
	if err := DizinDogrula(home, "public_html"); err == nil {
		t.Error("DizinDogrula symlink'i kabul etti")
	}
	if err := IceriginiSil(home, "public_html"); err == nil {
		t.Error("IceriginiSil symlink'i kabul etti")
	}
	if err := DizinOlustur(home, "public_html/alt", "c_test"); err == nil {
		t.Error("DizinOlustur symlink bileşeni üzerinden dizin oluşturdu")
	}
	if err := DosyaYaz(home, "public_html/x.txt", "c_test", []byte("x"), 0o644); err == nil {
		t.Error("DosyaYaz symlink üzerinden yazdı")
	}

	// Jail dışındaki dosya her hâlükârda dokunulmamış olmalı.
	if _, err := os.Stat(filepath.Join(disari, "kritik.conf")); err != nil {
		t.Fatalf("jail DIŞINDAKİ dosya etkilendi: %v", err)
	}
	if e, _ := os.ReadDir(disari); len(e) != 1 {
		t.Fatalf("jail dışı dizin içeriği değişti: %d girdi", len(e))
	}
}

func TestHomeDisinaCikilamaz(t *testing.T) {
	home, _ := kur(t)
	for _, rel := range []string{"../DISARIDA", "../../etc", "a/../../DISARIDA"} {
		if _, err := AcDizin(home, rel); err == nil {
			t.Errorf("AcDizin(%q) home dışına çıktı", rel)
		}
	}
}

func TestIceriginiSilYalnizIcerigiSiler(t *testing.T) {
	home, disari := kur(t)
	hedef := filepath.Join(home, "public_html")
	if err := os.MkdirAll(filepath.Join(hedef, "alt", "derin"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a.txt", "alt/b.txt", "alt/derin/c.txt"} {
		if err := os.WriteFile(filepath.Join(hedef, p), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// İçeride jail dışını gösteren bir symlink: SİLİNMELİ ama hedefi DURMALI.
	if err := os.Symlink(disari, filepath.Join(hedef, "kacis")); err != nil {
		t.Fatal(err)
	}

	if err := IceriginiSil(home, "public_html"); err != nil {
		t.Fatalf("IceriginiSil: %v", err)
	}
	girdiler, err := os.ReadDir(hedef)
	if err != nil {
		t.Fatal(err)
	}
	if len(girdiler) != 0 {
		t.Errorf("içerik silinmedi: %d girdi kaldı", len(girdiler))
	}
	if _, err := os.Stat(hedef); err != nil {
		t.Errorf("dizinin KENDİSİ silinmiş olmamalıydı: %v", err)
	}
	// symlink'in hedefi asla silinmemeli
	if _, err := os.Stat(filepath.Join(disari, "kritik.conf")); err != nil {
		t.Errorf("symlink hedefi silindi: %v", err)
	}
}

func TestDizinOlusturVeDosyaYaz(t *testing.T) {
	home, _ := kur(t)
	if err := DizinOlustur(home, "subdomains/a.example.com", "c_yok_boyle_kullanici"); err != nil {
		t.Fatalf("DizinOlustur: %v", err)
	}
	fi, err := os.Stat(filepath.Join(home, "subdomains", "a.example.com"))
	if err != nil || !fi.IsDir() {
		t.Fatalf("dizin oluşmadı: %v", err)
	}
	// idempotent olmalı
	if err := DizinOlustur(home, "subdomains/a.example.com", "c_yok_boyle_kullanici"); err != nil {
		t.Errorf("ikinci çağrı hata verdi: %v", err)
	}

	if err := DosyaYaz(home, "subdomains/a.example.com/index.html", "c_yok", []byte("merhaba"), 0o644); err != nil {
		t.Fatalf("DosyaYaz: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(home, "subdomains", "a.example.com", "index.html"))
	if err != nil || string(b) != "merhaba" {
		t.Errorf("dosya içeriği: %q, %v", b, err)
	}
}
