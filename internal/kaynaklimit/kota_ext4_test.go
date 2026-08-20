package kaynaklimit

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

// ext4CSVBaslik: quota-tools 4.09'un `repquota -u -O csv` başlığı. Biçim ikilinin
// içindeki format string'inden birebir doğrulandı:
//
//	"%s,%sStatus,FileStatus,%sUsed,%sSoftLimit,%sHardLimit,%sGrace,FileUsed,FileSoftLimit,FileHardLimit,FileGrace"
const ext4CSVBaslik = "User,BlockStatus,FileStatus,BlockUsed,BlockSoftLimit,BlockHardLimit,BlockGrace,FileUsed,FileSoftLimit,FileHardLimit,FileGrace"

// TestExt4LimitArgs: setquota arg-slice'ı (soft=hard*0.95, blok "M" sonekli, inode çıplak,
// 0=sınırsız, -u user quota, kök mount). Shell yok — arg-slice bütünlüğü kritiktir.
func TestExt4LimitArgs(t *testing.T) {
	cases := []struct {
		sk     string
		diskMB int
		inode  int
		want   []string
	}{
		// tam limit: soft = %95 (XFS tablosuyla aynı değerler)
		{"c_ornek", 5120, 500000, []string{"-u", "c_ornek", "4864M", "5120M", "475000", "500000", "/"}},
		// disk limit + inode sınırsız (0)
		{"c_foo", 1024, 0, []string{"-u", "c_foo", "972M", "1024M", "0", "0", "/"}},
		// her ikisi sınırsız
		{"c_bar", 0, 0, []string{"-u", "c_bar", "0M", "0M", "0", "0", "/"}},
		// yalnız inode limiti
		{"c_baz", 0, 100000, []string{"-u", "c_baz", "0M", "0M", "95000", "100000", "/"}},
		// negatif → 0'a sıkıştır
		{"c_neg", -5, -9, []string{"-u", "c_neg", "0M", "0M", "0", "0", "/"}},
	}
	for _, c := range cases {
		if got := ext4LimitArgs(c.sk, c.diskMB, c.inode); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ext4LimitArgs(%q,%d,%d)\n  got  = %#v\n  want = %#v", c.sk, c.diskMB, c.inode, got, c.want)
		}
	}
}

// TestExt4LimitArgsXFSParitesi: iki backend AYNI soft/hard sayılarını üretmeli —
// aksi halde dağıtım değiştirince tenant'ın efektif limiti sessizce kayardı.
func TestExt4LimitArgsXFSParitesi(t *testing.T) {
	for _, c := range []struct{ disk, inode int }{{5120, 500000}, {1024, 0}, {0, 100000}, {0, 0}} {
		xfs := kotaLimitArgs("c_ornek", c.disk, c.inode)
		ext := ext4LimitArgs("c_ornek", c.disk, c.inode)
		// xfs: ["-x","-c","limit -u bsoft=Am bhard=Bm isoft=C ihard=D c_ornek","/"]
		wantXFS := "limit -u bsoft=" + trimM(ext[2]) + "m bhard=" + trimM(ext[3]) + "m isoft=" + ext[4] + " ihard=" + ext[5] + " c_ornek"
		if xfs[2] != wantXFS {
			t.Errorf("disk=%d inode=%d parite bozuk:\n  xfs  = %q\n  ext4 = %#v", c.disk, c.inode, xfs[2], ext)
		}
	}
}

func trimM(s string) string { return s[:len(s)-1] }

// TestExt4QuotaonAktif: enforcement tespiti. quotaon KAPALIYKEN rc=0 döndüğü ve tanı
// mesajını STDERR'e yazdığı CANLI doğrulandı → yalnız stdout satırı esastır.
func TestExt4QuotaonAktif(t *testing.T) {
	cases := []struct {
		ad     string
		stdout string
		want   bool
	}{
		{"açık", "user quota on / (/dev/vda1) is on\n", true},
		{"kapalı (is off)", "user quota on / (/dev/vda1) is off\n", false},
		{"stdout boş (kota yok — mesaj stderr'e gitti)", "", false},
		{"group satırı yanıltmamalı", "group quota on / (/dev/vda1) is on\n", false},
		{"baştaki boşluk tolere edilir", "  user quota on / (/dev/sda2) is on  \n", true},
		{"çok satır, user ikinci", "group quota on / (/dev/vda1) is off\nuser quota on / (/dev/vda1) is on\n", true},
	}
	for _, c := range cases {
		if got := ext4QuotaonAktif(c.stdout); got != c.want {
			t.Errorf("%s: ext4QuotaonAktif(%q)=%v want=%v", c.ad, c.stdout, got, c.want)
		}
	}
}

// TestExt4MountKotali: accounting açık/enforce kapalı ayrımının ikinci kaynağı.
func TestExt4MountKotali(t *testing.T) {
	cases := []struct {
		ad     string
		mounts string
		want   bool
	}{
		{"kota seçeneği yok", "/dev/vda1 / ext4 rw,relatime 0 0\n", false},
		{"usrquota", "/dev/vda1 / ext4 rw,relatime,usrquota 0 0\n", true},
		{"quota", "/dev/vda1 / ext4 rw,quota,grpquota 0 0\n", true},
		{"usrjquota=değerli", "/dev/vda1 / ext4 rw,usrjquota=aquota.user,jqfmt=vfsv1 0 0\n", true},
		{"grpquota tek başına sayılmaz", "/dev/vda1 / ext4 rw,grpquota 0 0\n", false},
		{"başka mount'un usrquota'sı sayılmaz", "/dev/vdb1 /home ext4 rw,usrquota 0 0\n" + "/dev/vda1 / ext4 rw 0 0\n", false},
		{"kök birden çok satır: biri kotalı", "/dev/vda1 / ext4 rw 0 0\n/dev/vda1 / ext4 rw,usrquota 0 0\n", true},
		{"boş girdi", "", false},
	}
	for _, c := range cases {
		if got := ext4MountKotali(c.mounts); got != c.want {
			t.Errorf("%s: ext4MountKotali()=%v want=%v", c.ad, got, c.want)
		}
	}
}

// TestExt4CSVSatir: repquota CSV ayrıştırıcısı. Sütunlar BAŞLIK ADIYLA bulunur; sıra
// değişse veya "Block*" yerine "Space*" gelse de doğru okunmalı.
func TestExt4CSVSatir(t *testing.T) {
	cikti := ext4CSVBaslik + "\n" +
		"root,-,-,4194304,0,0,0,120000,0,0,0\n" +
		"c_ornek,-,-,1048576,4980736,5242880,0,1234,475000,500000,0\n" +
		"c_bos,-,-,0,0,0,0,3,0,0,0\n"

	bU, bH, iU, iH, ok := ext4CSVSatir(cikti, "c_ornek")
	if !ok || bU != 1048576 || bH != 5242880 || iU != 1234 || iH != 500000 {
		t.Errorf("c_ornek: got (%d,%d,%d,%d,%v) want (1048576,5242880,1234,500000,true)", bU, bH, iU, iH, ok)
	}
	// 1048576 KiB = 1024 MB, 5242880 KiB = 5120 MB → Durum()'un böldüğü değerler.
	if bU/1024 != 1024 || bH/1024 != 5120 {
		t.Errorf("MB çevrimi hatalı: %dMB / %dMB", bU/1024, bH/1024)
	}

	if _, _, _, _, ok := ext4CSVSatir(cikti, "c_yok"); ok {
		t.Error("olmayan tenant için ok=true döndü")
	}
	if _, _, _, _, ok := ext4CSVSatir("", "c_ornek"); ok {
		t.Error("boş çıktı için ok=true döndü")
	}
	// Başlık yoksa (biçim tanınmıyor) ASLA gövde satırı okunmamalı — sabit indeks varsayımı yok.
	if _, _, _, _, ok := ext4CSVSatir("c_ornek,-,-,1,2,3,4,5,6,7,8\n", "c_ornek"); ok {
		t.Error("başlıksız çıktı ayrıştırıldı — sütun indeksleri uydurulmuş olur")
	}

	// "Space*" adlandırması ve farklı sütun sırası.
	alt := "BlockGrace,SpaceUsed,User,FileHardLimit,FileUsed,SpaceHardLimit,FileStatus,BlockStatus,SpaceSoftLimit,FileSoftLimit,FileGrace\n" +
		"0,2097152,c_alt,250000,77,3145728,-,-,0,0,0\n"
	bU, bH, iU, iH, ok = ext4CSVSatir(alt, "c_alt")
	if !ok || bU != 2097152 || bH != 3145728 || iU != 77 || iH != 250000 {
		t.Errorf("Space*/karışık sıra: got (%d,%d,%d,%d,%v)", bU, bH, iU, iH, ok)
	}
}

// sahteKota: backend guard'larını (KotaUygula/KotaDurum) gerçek komut çalıştırmadan test eder.
type sahteKota struct {
	acc, enf bool
	uygulama int
}

func (s *sahteKota) Ad() string            { return "sahte" }
func (s *sahteKota) KernelBayragi() string { return "rootflags=test" }
func (s *sahteKota) Aktif() (bool, bool)   { return s.acc, s.enf }
func (s *sahteKota) Uygula(context.Context, string, int, int) error {
	s.uygulama++
	return nil
}
func (s *sahteKota) Durum(string) (int, int, int, int) { return 1, 2, 3, 4 }

// backendIle: testin süresince backend'i sabitler, sonra tespiti yeniden mümkün kılar.
func backendIle(t *testing.T, b kotaBackend) {
	t.Helper()
	kotaBackendZorla(b)
	t.Cleanup(func() { backend, backendBir = nil, sync.Once{} })
}

// TestKotaBackendYokIseSessiz: desteklenmeyen fs (backend nil) → KotaUygula hata DÖNMEZ,
// KotaDurum sıfır döner, KotaFSUyumlu false. Tenant create bu yüzden ASLA patlamamalı.
func TestKotaBackendYokIseSessiz(t *testing.T) {
	backendIle(t, nil)
	if KotaFSUyumlu() {
		t.Error("backend yokken KotaFSUyumlu()=true")
	}
	if err := KotaUygula(context.Background(), "c_ornek", 1024, 50000); err != nil {
		t.Errorf("backend yokken KotaUygula hata döndü: %v", err)
	}
	if a, b, c, d := KotaDurum("c_ornek"); a|b|c|d != 0 {
		t.Errorf("backend yokken KotaDurum=(%d,%d,%d,%d) want hepsi 0", a, b, c, d)
	}
}

// TestKotaEnforcementKapaliAtlanir: accounting açık / enforcement kapalı (XFS uqnoenforce
// veya ext4 mount-kotalı-ama-quotaon-kapalı) → limit YAZILMAZ, hata da dönmez.
func TestKotaEnforcementKapaliAtlanir(t *testing.T) {
	sk := &sahteKota{acc: true, enf: false}
	backendIle(t, sk)
	if err := KotaUygula(context.Background(), "c_ornek", 1024, 50000); err != nil {
		t.Errorf("enforcement kapalıyken KotaUygula hata döndü: %v", err)
	}
	if sk.uygulama != 0 {
		t.Errorf("enforcement kapalıyken backend.Uygula %d kez çağrıldı, 0 bekleniyordu", sk.uygulama)
	}
	if a, b, c, d := KotaDurum("c_ornek"); a|b|c|d != 0 {
		t.Errorf("enforcement kapalıyken KotaDurum=(%d,%d,%d,%d) want hepsi 0", a, b, c, d)
	}
}

// TestKotaGecersizSKReddedilir: allowlist dışı sk backend'e HİÇ ulaşmamalı.
func TestKotaGecersizSKReddedilir(t *testing.T) {
	sk := &sahteKota{acc: true, enf: true}
	backendIle(t, sk)
	if err := KotaUygula(context.Background(), "root", 1024, 50000); err == nil {
		t.Error("geçersiz sk için KotaUygula hata dönmedi")
	}
	if sk.uygulama != 0 {
		t.Errorf("geçersiz sk backend'e ulaştı (%d çağrı)", sk.uygulama)
	}
}

// 🔴 Faz 5a regresyonu: quotaon'un ÇIKIŞ KODU her iki yönde de güvenilmez.
// Debian 12 / quota-tools 4.06'da kota AÇIKKEN rc=1 dönüyor; kod hatayı görüp
// erken dönünce panel çalışan kotayı "kapalı, reboot gerekli" diye raporladı.
// Karar YALNIZ stdout'a bakmalı.
func TestExt4AktifCozCikisKodunaBakmaz(t *testing.T) {
	const acikCikti = "user quota on / (/dev/vda1) is on\n"

	// quotaon "on" diyor (rc ne olursa olsun) → hem muhasebe hem uygulama açık.
	if a, e := ext4AktifCoz(acikCikti, false); !a || !e {
		t.Errorf("quotaon 'is on' derken accounting=%v enforcement=%v — ikisi de true olmalı", a, e)
	}
	// quotaon susuyor ama mount kotalı → muhasebe var, uygulama yok.
	if a, e := ext4AktifCoz("", true); !a || e {
		t.Errorf("mount kotalı/quotaon kapalı: accounting=%v enforcement=%v — (true,false) olmalı", a, e)
	}
	// İkisi de yok → kota kapalı.
	if a, e := ext4AktifCoz("", false); a || e {
		t.Errorf("kota kapalıyken accounting=%v enforcement=%v — ikisi de false olmalı", a, e)
	}
}

// Aktif(), quotaon hata DÖNSE BİLE stdout'u dikkate almalı.
func TestExt4AktifQuotaonHataliCikisKoduylaDaCalisir(t *testing.T) {
	eski := quotaonSorgula
	t.Cleanup(func() { quotaonSorgula = eski })
	quotaonSorgula = func() (string, error) {
		return "user quota on / (/dev/sda1) is on\n", errors.New("exit status 1")
	}
	// Aktif() ayrıca quotaon'un PATH'te olmasını arar. Bunu da sahtelemezsek
	// test, makinede `quota` paketi kurulu olup olmamasına göre sonuç değiştirir:
	// geliştirme sunucusunda geçer, CI runner'ında (paket yok) düşer.
	eskiVar := quotaonVar
	t.Cleanup(func() { quotaonVar = eskiVar })
	quotaonVar = func() bool { return true }
	a, e := ext4Kota{}.Aktif()
	if !a || !e {
		t.Fatalf("rc=1 + 'is on' çıktısı: accounting=%v enforcement=%v — ikisi de true olmalı", a, e)
	}
}
