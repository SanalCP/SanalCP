package system

import (
	"os/exec"
	"strings"
	"testing"
)

func TestAptGuvenlikPaketleriniAyristir(t *testing.T) {
	cikti := `Inst openssl [3.0.11-1] (3.0.11-1~deb12u2 Debian-Security:12/oldstable-security [amd64])
Inst nginx [1.22.1-9] (1.22.1-9+deb12u1 Debian:12.10/oldstable [amd64])
Inst linux-image-amd64 [6.1.1] (6.1.2 Ubuntu:24.04/noble-security [amd64])
Inst openssl [3.0.11-1] (3.0.11-1~deb12u2 Debian-Security:12/oldstable-security [amd64])`

	got := aptGuvenlikPaketleriniAyristir(cikti)
	if len(got) != 2 {
		t.Fatalf("%d paket bulundu, 2 bekleniyordu: %#v", len(got), got)
	}
	if got[0].Id != "linux-image-amd64" || got[1].Id != "openssl" {
		t.Fatalf("paketler tekil/sıralı değil: %#v", got)
	}
}

func TestAptEskiSecuritySurumunuYanlisSaymaz(t *testing.T) {
	cikti := `Inst curl [7.88.1-10+deb12u5-security] (7.88.1-10+deb12u6 Debian:12/oldstable [amd64])`
	if got := aptGuvenlikPaketleriniAyristir(cikti); len(got) != 0 {
		t.Fatalf("security yalnız eski sürümdeyken paket sayıldı: %#v", got)
	}
}

func TestCveWrapperAptGuvenlikGuncellemesiniIcerir(t *testing.T) {
	got := cveWrapperIcerik("tr")
	if !strings.Contains(got, "unattended-upgrade -d") || !strings.Contains(got, "apt-get install -y --only-upgrade") {
		t.Fatal("Debian/Ubuntu güvenlik güncelleme dalı eksik")
	}
	for _, dil := range []string{"tr", "en"} {
		cmd := exec.Command("bash", "-n")
		cmd.Stdin = strings.NewReader(cveWrapperIcerik(dil))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s wrapper shell sözdizimi bozuk: %v: %s", dil, err, out)
		}
	}
}
