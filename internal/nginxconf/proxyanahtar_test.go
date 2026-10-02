package nginxconf

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProxyAnahtariHazirlaUretirVeKorur(t *testing.T) {
	dizin := t.TempDir()
	confDizin := filepath.Join(dizin, "conf.d")
	if err := os.Mkdir(confDizin, 0o755); err != nil {
		t.Fatal(err)
	}
	anahtarYol := filepath.Join(dizin, "sanalcp", "proxy-anahtar")
	confYol := filepath.Join(confDizin, "_sanalcp_proxy_anahtar.conf")

	a1, degisti, err := proxyAnahtariHazirla(anahtarYol, confYol)
	if err != nil || !degisti || !proxyAnahtarRE.MatchString(a1) {
		t.Fatalf("ilk hazırlama: anahtar=%q degisti=%v err=%v", a1, degisti, err)
	}
	for _, yol := range []string{anahtarYol, confYol} {
		st, err := os.Stat(yol)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Errorf("%s izni %o, beklenen 600", yol, st.Mode().Perm())
		}
	}
	a2, degisti, err := proxyAnahtariHazirla(anahtarYol, confYol)
	if err != nil || degisti || a2 != a1 {
		t.Fatalf("ikinci hazırlama anahtarı değiştirmemeli: %q→%q degisti=%v err=%v", a1, a2, degisti, err)
	}
	icerik, _ := os.ReadFile(confYol)
	if string(icerik) != ProxyAnahtarConf(a1) {
		t.Fatal("map dosyası beklenen içerikte değil")
	}
}

func TestProxyAnahtariNginxYoksaSessiz(t *testing.T) {
	dizin := t.TempDir()
	a, degisti, err := proxyAnahtariHazirla(filepath.Join(dizin, "k"), filepath.Join(dizin, "yok", "x.conf"))
	if a != "" || degisti || err != nil {
		t.Fatalf("nginx dizini yokken no-op beklenirdi: %q %v %v", a, degisti, err)
	}
}

func TestPanelConfProxyAnahtariKullanir(t *testing.T) {
	if strings.Contains(PanelConf, "X-Forwarded-For $remote_addr") ||
		strings.Contains(PanelConf, "X-Real-IP $remote_addr") {
		t.Fatal("_panel.conf hâlâ $remote_addr'ı istemci IP'si olarak gönderiyor")
	}
	// Backend'e giden her proxy_pass bloğu anahtarı da göndermeli.
	if n, m := strings.Count(PanelConf, "proxy_pass http://127.0.0.1:8080;"),
		strings.Count(PanelConf, "proxy_set_header X-SanalCP-Proxy $sanalcp_proxy_anahtari;"); m < n-1 {
		t.Fatalf("%d backend proxy bloğu var ama yalnız %d tanesi anahtar gönderiyor", n, m)
	}
	if !strings.Contains(PanelConf, "location ^~ /api/v1/internal/ {") {
		t.Fatal("/api/v1/internal/ uçları nginx'te dışarı kapatılmamış")
	}
}

// Kurulum betiği map dosyasını bash ile üretir; panel açılışta Go ile yeniden
// üretir. İkisi aynı değilse her açılışta gereksiz reload olur — ve biçim
// ayrışırsa sessizce bozulabilir.
func TestKurulumBetigiMapBicimiAyni(t *testing.T) {
	betik, err := os.ReadFile("../../sanalcp-install.sh")
	if err != nil {
		t.Skip("kurulum betiği bulunamadı")
	}
	s := string(betik)
	bas := strings.Index(s, "<<PROXYMAP\n")
	son := strings.Index(s, "\nPROXYMAP\n")
	if bas < 0 || son < 0 {
		t.Fatal("kurulum betiğinde PROXYMAP heredoc'u yok")
	}
	govde := s[bas+len("<<PROXYMAP\n") : son+1]
	const anahtar = "0123456789abcdef0123456789abcdef"
	cmd := exec.Command("bash", "-c", "PROXY_ANAHTAR="+anahtar+"; cat <<PROXYMAP\n"+govde+"PROXYMAP\n")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("bash çalıştırılamadı: %v", err)
	}
	if string(out) != ProxyAnahtarConf(anahtar) {
		t.Fatalf("kurulum betiği ile Go biçimi farklı:\n--- betik\n%s--- go\n%s", out, ProxyAnahtarConf(anahtar))
	}
}
