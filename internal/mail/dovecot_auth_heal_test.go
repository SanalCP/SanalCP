package mail

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stokAuthConf: Dovecot'un stok 10-auth.conf'undaki ilgili bölümün kopyası.
const stokAuthConf = `##
## Password databases
##
#!include auth-master.conf.ext
!include auth-system.conf.ext
#!include auth-sql.conf.ext
#!include auth-ldap.conf.ext
`

func dovecotOrtami(t *testing.T, authIcerik string, sanalcpVar bool) string {
	t.Helper()
	dizin := t.TempDir()
	auth := filepath.Join(dizin, "10-auth.conf")
	if err := os.WriteFile(auth, []byte(authIcerik), 0o644); err != nil {
		t.Fatal(err)
	}
	sanalcp := filepath.Join(dizin, "10-sanalcp-mail.conf")
	if sanalcpVar {
		if err := os.WriteFile(sanalcp, []byte("protocols = imap lmtp\npassdb {\n  driver = sql\n}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	eskiA, eskiS := dovecotAuthConf, dovecotSanalCPConf
	dovecotAuthConf, dovecotSanalCPConf = auth, sanalcp
	t.Cleanup(func() { dovecotAuthConf, dovecotSanalCPConf = eskiA, eskiS })
	return dizin
}

func TestPamKapatStokConfigi(t *testing.T) {
	dizin := dovecotOrtami(t, stokAuthConf, true)

	if !pamKapat() {
		t.Fatal("aktif PAM include'u kapatılmadı")
	}
	b, _ := os.ReadFile(filepath.Join(dizin, "10-auth.conf"))
	s := string(b)
	if strings.Contains(s, "\n!include auth-system.conf.ext") {
		t.Errorf("PAM include hâlâ aktif:\n%s", s)
	}
	if !strings.Contains(s, "#!include auth-system.conf.ext") {
		t.Errorf("satır yorumlanmadı:\n%s", s)
	}
	// Diğer include satırlarına DOKUNULMAMALI.
	for _, korunmali := range []string{"#!include auth-master.conf.ext", "#!include auth-sql.conf.ext"} {
		if !strings.Contains(s, korunmali) {
			t.Errorf("%q kayboldu", korunmali)
		}
	}
}

// Idempotent: zaten yorumlanmışsa değişiklik bildirmemeli.
func TestPamKapatIdempotent(t *testing.T) {
	dovecotOrtami(t, stokAuthConf, true)
	if !pamKapat() {
		t.Fatal("ilk çağrı değişiklik yapmalıydı")
	}
	if pamKapat() {
		t.Error("ikinci çağrı yeniden değişiklik bildirdi")
	}
}

func TestAuthCacheEkle(t *testing.T) {
	dizin := dovecotOrtami(t, stokAuthConf, true)

	if !authCacheEkle() {
		t.Fatal("auth cache eklenmedi")
	}
	b, _ := os.ReadFile(filepath.Join(dizin, "10-sanalcp-mail.conf"))
	if !strings.Contains(string(b), "auth_cache_size = 10M") {
		t.Errorf("önbellek ayarı yazılmadı:\n%s", b)
	}
	if !strings.Contains(string(b), "driver = sql") {
		t.Error("mevcut içerik korunmadı")
	}
	if authCacheEkle() {
		t.Error("ikinci çağrı yeniden ekledi (idempotent değil)")
	}
}

// 🔴 En önemli koruma: SanalCP drop-in'i YOKSA başka amaçla kurulmuş bir
// Dovecot'a hiç dokunulmamalı (sistem kullanıcılarına IMAP veriliyor olabilir).
func TestHealDovecotAuthYabanciKurulumaDokunmaz(t *testing.T) {
	dizin := dovecotOrtami(t, stokAuthConf, false) // drop-in YOK

	HealDovecotAuth(context.Background())

	b, _ := os.ReadFile(filepath.Join(dizin, "10-auth.conf"))
	if string(b) != stokAuthConf {
		t.Errorf("SanalCP kurulumu olmayan Dovecot değiştirildi:\n%s", b)
	}
}
