package files

// safeio_okuma_test.go — symlink-güvenli OKUMA yollarının testi.
//
// Bu testler, mutasyon tarafındaki (safeio_test.go) korumanın OKUMA tarafındaki
// eşleniğini savunur. Okuma yolları uzun süre jailJoinStrict + os.Open/os.ReadFile/
// os.ReadDir üzerindeydi; kontrol ile işlem ayrı adımlar olduğu için tenant, ara
// bileşeni symlink'e takas ederek root'a jail DIŞINDAKİ dosyayı okutabiliyordu.
// Aşağıdaki testler o sızıntının kapalı kaldığını doğrular.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileBeneath_AraDizinSymlinkReddedilir(t *testing.T) {
	home, _ := setupJail(t)
	// home/link -> outside; "link/secret" ara-bileşen symlink'i ile jail dışına iner.
	data, _, err := readFileBeneath(home, "link/secret", 0)
	if err == nil {
		t.Fatalf("jail-dışı okuma BAŞARILI oldu (sızıntı): %q", string(data))
	}
	if strings.Contains(string(data), "SECRET") {
		t.Fatalf("jail-dışı içerik döndü: %q", string(data))
	}
}

func TestReadFileBeneath_LeafSymlinkReddedilir(t *testing.T) {
	home, _ := setupJail(t)
	// home/lnk -> outside/secret; son bileşen symlink.
	data, _, err := readFileBeneath(home, "lnk", 0)
	if err == nil {
		t.Fatalf("leaf symlink üzerinden okuma BAŞARILI oldu (sızıntı): %q", string(data))
	}
}

func TestReadFileBeneath_MesruOkumaCalisir(t *testing.T) {
	home, _ := setupJail(t)
	data, boyut, err := readFileBeneath(home, "real/f", 0)
	if err != nil {
		t.Fatalf("meşru okuma başarısız: %v", err)
	}
	if string(data) != "legit" {
		t.Fatalf("içerik yanlış: %q", string(data))
	}
	if boyut != int64(len("legit")) {
		t.Fatalf("boyut yanlış: %d", boyut)
	}
}

func TestReadFileBeneath_BoyutSiniri(t *testing.T) {
	home, _ := setupJail(t)
	if err := os.WriteFile(filepath.Join(home, "buyuk"), make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	_, boyut, err := readFileBeneath(home, "buyuk", 10)
	if !errors.Is(err, errTooBig) {
		t.Fatalf("boyut sınırı uygulanmadı: %v", err)
	}
	// Sınır aşılsa da boyut bilgisi dönmeli (UI mesajı için).
	if boyut != 100 {
		t.Fatalf("boyut raporlanmadı: %d", boyut)
	}
}

func TestReadDirBeneath_AraDizinSymlinkReddedilir(t *testing.T) {
	home, _ := setupJail(t)
	if _, err := readDirBeneath(home, "link"); err == nil {
		t.Fatal("jail-dışı dizin listelendi (sızıntı)")
	}
}

// readDirBeneath jail İÇİNDEKİ bir symlink girdisini listelemeyi reddetmez —
// listeler, ama HEDEFİNİ değil BAĞIN KENDİSİNİ raporlar. Aksi hâlde jail dışı
// hedefin boyutu/izinleri/tipi sızardı.
func TestReadDirBeneath_SymlinkGirdisiDereferenceEdilmez(t *testing.T) {
	home, _ := setupJail(t)
	girdiler, err := readDirBeneath(home, ".")
	if err != nil {
		t.Fatalf("listeleme başarısız: %v", err)
	}
	var bulundu bool
	for _, g := range girdiler {
		if g.Ad != "lnk" { // home/lnk -> outside/secret
			continue
		}
		bulundu = true
		if g.Mode&os.ModeSymlink == 0 {
			t.Fatalf("symlink girdisi dereference edildi: mode=%v", g.Mode)
		}
		// Hedef 6 baytlık "SECRET"; bağın kendi boyutu hedef YOLUNUN uzunluğudur.
		if g.Boyut == 6 {
			t.Fatal("symlink hedefinin boyutu sızdı")
		}
	}
	if !bulundu {
		t.Fatal("lnk girdisi listede yok")
	}
}

func TestReadDirBeneath_MesruListelemeCalisir(t *testing.T) {
	home, _ := setupJail(t)
	girdiler, err := readDirBeneath(home, "real")
	if err != nil {
		t.Fatalf("meşru listeleme başarısız: %v", err)
	}
	if len(girdiler) != 1 || girdiler[0].Ad != "f" {
		t.Fatalf("beklenmeyen liste: %+v", girdiler)
	}
	if girdiler[0].Boyut != int64(len("legit")) {
		t.Fatalf("boyut yanlış: %d", girdiler[0].Boyut)
	}
}

func TestStatBeneath_SymlinkIleJailDisiStatReddedilir(t *testing.T) {
	home, _ := setupJail(t)
	if _, err := statBeneath(home, "link/secret"); err == nil {
		t.Fatal("jail-dışı stat başarılı oldu")
	}
	if _, err := statBeneath(home, "lnk"); err == nil {
		t.Fatal("leaf symlink üzerinden stat başarılı oldu")
	}
}

func TestDogrulanmisYol_JailDisiReddedilir(t *testing.T) {
	home, _ := setupJail(t)
	for _, rel := range []string{"link/secret", "lnk", "../etc/shadow"} {
		if yol, err := dogrulanmisYol(home, rel); err == nil {
			t.Fatalf("%q için jail-dışı yol doğrulandı: %q", rel, yol)
		}
	}
	// Meşru yol geçmeli ve home altında kalmalı.
	yol, err := dogrulanmisYol(home, "real/f")
	if err != nil {
		t.Fatalf("meşru yol reddedildi: %v", err)
	}
	if yol != filepath.Join(home, "real", "f") {
		t.Fatalf("beklenmeyen yol: %q", yol)
	}
}

// ciktiHazirla, dış araca YAZDIRILACAK hedefin symlink olmasını reddetmeli —
// aksi hâlde zip/tar çıktısı jail dışındaki bir dosyanın üzerine yazardı.
func TestCiktiHazirla_SymlinkHedefReddedilir(t *testing.T) {
	home, outside := setupJail(t)
	if _, err := ciktiHazirla(home, "lnk", "c_test"); err == nil {
		t.Fatal("symlink çıktı hedefi kabul edildi")
	}
	// Jail dışı dosya bozulmamış olmalı.
	if b, _ := os.ReadFile(filepath.Join(outside, "secret")); string(b) != "SECRET" {
		t.Fatalf("jail-dışı dosya değişti: %q", string(b))
	}
}

func TestCiktiHazirla_MevcutDosyaTemizlenir(t *testing.T) {
	home, _ := setupJail(t)
	if err := os.WriteFile(filepath.Join(home, "real", "eski.zip"), []byte("bozuk"), 0o644); err != nil {
		t.Fatal(err)
	}
	yol, err := ciktiHazirla(home, "real/eski.zip", "c_test")
	if err != nil {
		t.Fatalf("çıktı hazırlanamadı: %v", err)
	}
	if _, err := os.Stat(yol); !os.IsNotExist(err) {
		t.Fatal("duran dosya temizlenmedi (araç bozuk arşiv sanabilir)")
	}
}

// tenantKomut yalnız panelin tenant deseni (c_*) için komut üretmeli; aksi hâlde
// bir çağrı yeri yanlışlıkla "root" verip aracı tam yetkiyle çalıştırabilirdi.
func TestTenantKomut_GecersizKullaniciReddedilir(t *testing.T) {
	for _, sk := range []string{"root", "", "nginx", "../root"} {
		if _, err := tenantKomut(context.Background(), sk, "true"); err == nil {
			t.Fatalf("geçersiz tenant kabul edildi: %q", sk)
		}
	}
	if _, err := tenantKomut(context.Background(), "c_test", "true"); err != nil {
		t.Fatalf("geçerli tenant reddedildi: %v", err)
	}
}
