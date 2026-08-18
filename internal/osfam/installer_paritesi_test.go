package osfam

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Installer ↔ osfam paritesi.
//
// NEDEN VAR: sanalcp-install.sh paket ve servis adlarını KENDİ tablosundan
// çözer (kurulum anında, Go çalışmadan önce). Panel ise aynı adları buradan
// çözer (çalışma anında). İki tablo ayrışırsa installer bir şey kurar, panel
// başka bir adı arar — ve bu yalnız gerçek bir sunucuda, kurulumdan sonra
// ortaya çıkar. Bu test iki tabloyu sahte bir /etc/os-release ile yan yana
// koyup karşılaştırır.
//
// Betik `SANALCP_TANIM_TESTI=1` ile source edildiğinde yalnız tanımları yapıp
// döner; sisteme dokunmaz.

type installerCikti map[string]string

func installerCoz(t *testing.T, osRelease string) installerCikti {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash yok")
	}
	betik, err := filepath.Abs("../../sanalcp-install.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(betik); err != nil {
		t.Skipf("installer bulunamadı: %v", err)
	}
	// 🔴 Ortak kütüphaneyi de STAT'la. Go'nun test önbelleği, test ikilisinin
	// açtığı dosyaları izler; betiği `bash` okuduğu için ortak.sh dokunulmadan
	// kalırsa değişiklik önbelleği geçersiz KILMAZ ve test eski sonucu döner
	// (yani sessizce körelir).
	ortak, _ := filepath.Abs("../../assets/ops/sanalcp-ortak.sh")
	if _, err := os.Stat(ortak); err != nil {
		t.Skipf("ortak kütüphane bulunamadı: %v", err)
	}
	sahte := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(sahte, []byte(osRelease), 0644); err != nil {
		t.Fatal(err)
	}

	// Betikten okunacak değerler: mantıksal ad -> yazdırılacak ifade.
	sorgular := []struct{ anahtar, ifade string }{
		{"paket:" + PaketWeb, `paket_ad web`},
		{"paket:" + PaketDB, `paket_ad db`},
		{"paket:" + PaketDNS, `paket_ad dns`},
		{"paket:" + PaketDNSArac, `paket_ad dns-arac`},
		{"paket:" + PaketAntivirus, `paket_ad antivirus`},
		{"paket:" + PaketAVGuncel, `paket_ad av-guncel`},
		{"paket:" + PaketCron, `paket_ad cron`},
		{"paket:" + PaketApache, `paket_ad apache`},
		{"paket:" + PaketApacheAra, `paket_ad apache-ara`},
		{"paket:" + PaketBsdtar, `paket_ad bsdtar`},
		{"paket:" + PaketCache, `paket_ad cache`},
		{"paket:" + PaketFTP, `paket_ad ftp`},
		{"paket:" + PaketSSH, `paket_ad ssh`},
		{"paket:" + PaketKotaXFS, `paket_ad kota-xfs`},
		{"paket:" + PaketKotaExt, `paket_ad kota-ext`},
		{"servis:" + PaketWeb, `servis_ad web`},
		{"servis:" + PaketDB, `servis_ad db`},
		{"servis:" + PaketDNS, `servis_ad dns`},
		{"servis:" + PaketCron, `servis_ad cron`},
		{"servis:" + PaketApache, `servis_ad apache`},
		{"servis:" + PaketFTP, `servis_ad ftp`},
		{"servis:" + PaketCache, `servis_ad cache`},
		{"servis:" + PaketAntivirus, `servis_ad antivirus`},
		{"servis:" + PaketSSH, `servis_ad ssh`},
		{"webkullanici", `printf '%s' "$WEB_USER"`},
	}
	var sb strings.Builder
	sb.WriteString(". " + betik + "\n")
	for _, q := range sorgular {
		sb.WriteString("printf '%s=%s\\n' '" + q.anahtar + "' \"$(" + q.ifade + ")\"\n")
	}

	cmd := exec.Command("bash", "-c", sb.String())
	cmd.Env = append(os.Environ(), "SANALCP_TANIM_TESTI=1", "SANALCP_OS_RELEASE="+sahte)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("installer source edilemedi: %v\n%s", err, out)
	}
	sonuc := installerCikti{}
	for _, satir := range strings.Split(string(out), "\n") {
		if i := strings.IndexByte(satir, '='); i > 0 && !strings.HasPrefix(strings.TrimSpace(satir), "✓") {
			sonuc[strings.TrimSpace(satir[:i])] = strings.TrimSpace(satir[i+1:])
		}
	}
	if len(sonuc) == 0 {
		t.Fatalf("installer'dan hiçbir ad okunamadı:\n%s", out)
	}
	return sonuc
}

func TestInstallerAdlariOsfamIleAyni(t *testing.T) {
	durumlar := []struct {
		ad        string
		osRelease string
	}{
		{"almalinux10", "ID=almalinux\nVERSION_ID=\"10.0\"\nPLATFORM_ID=\"platform:el10\"\nID_LIKE=\"rhel centos fedora\"\n"},
		{"almalinux9", "ID=almalinux\nVERSION_ID=\"9.6\"\nPLATFORM_ID=\"platform:el9\"\nID_LIKE=\"rhel centos fedora\"\n"},
		{"debian13", "ID=debian\nVERSION_ID=\"13\"\nVERSION_CODENAME=trixie\n"},
		{"debian12", "ID=debian\nVERSION_ID=\"12\"\nVERSION_CODENAME=bookworm\n"},
		{"ubuntu2604", "ID=ubuntu\nVERSION_ID=\"26.04\"\nVERSION_CODENAME=resolute\nID_LIKE=debian\n"},
		{"ubuntu2404", "ID=ubuntu\nVERSION_ID=\"24.04\"\nVERSION_CODENAME=noble\nID_LIKE=debian\n"},
	}
	for _, d := range durumlar {
		t.Run(d.ad, func(t *testing.T) {
			betikten := installerCoz(t, d.osRelease)
			b := Ayristir(d.osRelease)

			for anahtar, betikDeger := range betikten {
				tur, mantiksal, _ := strings.Cut(anahtar, ":")
				var goDeger string
				switch tur {
				case "paket":
					goDeger = b.Paket(mantiksal)
				case "servis":
					goDeger = b.Servis(mantiksal)
				case "webkullanici":
					goDeger = b.WebKullanici()
				default:
					continue
				}
				if betikDeger != goDeger {
					t.Errorf("%s: installer %q diyor, osfam %q diyor — kurulan şey ile panelin aradığı şey farklı",
						anahtar, betikDeger, goDeger)
				}
			}
		})
	}
}

// Aile tespitinin kendisi de aynı olmalı: betik yanlış aileye karar verirse
// yanlış paket yöneticisini çağırır.
func TestInstallerAileTespitiOsfamIleAyni(t *testing.T) {
	durumlar := map[string]struct {
		osRelease string
		beklenen  Aile
	}{
		"rocky":  {"ID=\"rocky\"\nVERSION_ID=\"9.4\"\nID_LIKE=\"rhel centos fedora\"\n", RHEL},
		"debian": {"ID=debian\nVERSION_ID=\"13\"\nVERSION_CODENAME=trixie\n", Debian},
		"ubuntu": {"ID=ubuntu\nVERSION_ID=\"24.04\"\nID_LIKE=debian\n", Debian},
	}
	for ad, d := range durumlar {
		t.Run(ad, func(t *testing.T) {
			if got := Ayristir(d.osRelease).Aile; got != d.beklenen {
				t.Fatalf("osfam aile: %v (beklenen %v)", got, d.beklenen)
			}
			// Betik tarafı: web kullanıcısı aileyi doğrudan yansıtır.
			betikten := installerCoz(t, d.osRelease)
			beklenenKullanici := "nginx"
			if d.beklenen == Debian {
				beklenenKullanici = "www-data"
			}
			if betikten["webkullanici"] != beklenenKullanici {
				t.Fatalf("installer web kullanıcısı %q, beklenen %q — aile tespiti ayrışmış",
					betikten["webkullanici"], beklenenKullanici)
			}
		})
	}
}

// Ops betikleri de aynı tabloyu kullanmalı.
//
// NEDEN: her betiğin kendi "dnf install" satırını taşıması, Debian portunun
// gözden kaçtığı yerlerin tam olarak nerede biriktiğiydi. Paket yöneticisine
// dokunan bir ops betiği ortak kütüphaneyi source ETMİYORSA, sessizce yalnız
// RHEL'de çalışır.
func TestOpsBetikleriOrtakKutuphaneyiSourceEder(t *testing.T) {
	dizin, err := filepath.Abs("../../assets/ops")
	if err != nil {
		t.Fatal(err)
	}
	girdiler, err := os.ReadDir(dizin)
	if err != nil {
		t.Skipf("assets/ops okunamadı: %v", err)
	}
	var bakilan int
	for _, g := range girdiler {
		if g.IsDir() || g.Name() == "sanalcp-ortak.sh" {
			continue
		}
		ham, err := os.ReadFile(filepath.Join(dizin, g.Name()))
		if err != nil {
			t.Fatal(err)
		}
		icerik := string(ham)
		paketYoneticisiKullaniyor := strings.Contains(icerik, "dnf install") ||
			strings.Contains(icerik, "apt-get install") ||
			strings.Contains(icerik, "pkg_kur")
		if !paketYoneticisiKullaniyor {
			continue
		}
		bakilan++
		if !strings.Contains(icerik, "sanalcp-ortak") {
			t.Errorf("%s: paket yöneticisi çağırıyor ama sanalcp-ortak source etmiyor — Debian'da sessizce çalışmaz",
				g.Name())
		}
	}
	if bakilan == 0 {
		t.Fatal("hiçbir ops betiği paket yöneticisi kullanmıyor görünüyor — test yanlış dizine bakıyor olabilir")
	}
}
