// adapter.go — apps.Uygulama arayüzünün WordPress implementasyonu. wordpress.go/
// toolkit.go/bakim.go'daki HİÇBİR SATIRA dokunulmaz; bu dosya onlardaki unexported
// yardımcıları (aynı pakette olduğu için) doğrudan çağırarak apps çerçevesine
// paralel bir yol açar. /wordpress/* route'ları bu dosyadan tamamen bağımsız çalışmaya
// devam eder.
package wordpress

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sanalcp/internal/apps"
)

func init() {
	apps.Kaydet(Adapter{})
}

type Adapter struct{}

func (Adapter) Slug() string              { return "wordpress" }
func (Adapter) Ad() string                { return "WordPress" }
func (Adapter) DBOnEki() string           { return "wp" }
func (Adapter) MarkerDosya() string       { return "wp-config.php" }
func (Adapter) GuncelleDesteklenir() bool { return true }

func (Adapter) FormAlanlari() []apps.FormAlan {
	return []apps.FormAlan{
		{Anahtar: "site_basligi", Etiket: "Site Başlığı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_kullanici", Etiket: "Admin Kullanıcı", Tur: "text", Zorunlu: true},
		{Anahtar: "admin_email", Etiket: "Admin E-posta", Tur: "email", Zorunlu: true},
	}
}

func (Adapter) DBAdiOku(dizin string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dizin, "wp-config.php"))
	if err != nil {
		return "", false
	}
	m := reDBName.FindSubmatch(b)
	if m == nil {
		return "", false
	}
	dbName := string(m[1])
	if !dbAdiWPGuard(dbName) {
		return "", false
	}
	return dbName, true
}

// Kur: wordpress.go'daki Kur handler'ının WordPress'e-özel gövdesinin adaptasyonu
// (DB oluşturma ortak apps katmanında zaten yapıldığı için burada tekrar edilmez).
func (Adapter) Kur(ctx context.Context, i apps.KurulumIstek) (apps.KurulumSonuc, error) {
	if out, err := wpKomut(ctx, i.SK, "core", "download", "--path="+i.Hedef, "--locale=tr_TR"); err != nil {
		return apps.KurulumSonuc{}, wpHataOlustur("WordPress indirme", out)
	}
	if out, err := wpKomutStdin(ctx, i.SK, i.DBParola+"\n", "config", "create", "--dbname="+i.DBAdi,
		"--dbuser="+i.DBKullanici, "--prompt=dbpass", "--dbhost=localhost", "--locale=tr_TR",
		"--path="+i.Hedef, "--skip-check", "--quiet"); err != nil {
		return apps.KurulumSonuc{}, wpHataOlustur("wp-config oluşturma", out)
	}
	if err := wpConfigDBParolaDogrula(i.Hedef, i.DBParola); err != nil {
		return apps.KurulumSonuc{}, err
	}
	adminParola := randParola()
	if out, err := wpKomutStdin(ctx, i.SK, adminParola+"\n", "core", "install", "--url="+i.URL,
		"--title="+i.Alanlar["site_basligi"], "--admin_user="+i.Alanlar["admin_kullanici"],
		"--prompt=admin_password", "--admin_email="+i.Alanlar["admin_email"],
		"--skip-email", "--path="+i.Hedef, "--quiet"); err != nil {
		return apps.KurulumSonuc{}, wpHataOlustur("WordPress kurulum", out)
	}
	out, err := wpKomutStdin(ctx, i.SK, i.Alanlar["admin_kullanici"]+"\n"+adminParola+"\n",
		"eval", wpParolaDogrulaPHP, "--path="+i.Hedef, "--quiet")
	if err != nil || !bytes.Contains(out, []byte("PAROLA_OK")) {
		return apps.KurulumSonuc{}, wpHataOlustur("admin parolası doğrulama", out)
	}
	surum := ""
	if b, err := wpKomut(ctx, i.SK, "core", "version", "--path="+i.Hedef); err == nil {
		surum = strings.TrimSpace(string(b))
	}
	return apps.KurulumSonuc{
		SiteURL: i.URL, AdminURL: i.URL + "/wp-admin",
		AdminKullanici: i.Alanlar["admin_kullanici"], AdminParola: adminParola,
		Surum: surum,
	}, nil
}

func wpHataOlustur(asama string, out []byte) error {
	msg := strings.TrimSpace(string(out))
	if len(msg) > 600 {
		msg = msg[len(msg)-600:]
	}
	return &wpAdapterHata{asama: asama, mesaj: msg}
}

type wpAdapterHata struct{ asama, mesaj string }

func (e *wpAdapterHata) Error() string { return e.asama + ": " + e.mesaj }

func (Adapter) Bilgi(ctx context.Context, sk, dizin, url string) (apps.Kurulum, error) {
	k := apps.Kurulum{SiteURL: url, AdminURL: url + "/wp-admin", Durum: "bilinmiyor"}
	c1, cancel1 := context.WithTimeout(ctx, 15*time.Second)
	if b, err := wpStdout(c1, sk, "core", "version", "--path="+dizin); err == nil {
		k.Surum = strings.TrimSpace(string(b))
	}
	cancel1()
	c2, cancel2 := context.WithTimeout(ctx, 25*time.Second)
	if b, err := wpStdout(c2, sk, "core", "check-update", "--path="+dizin, "--format=json"); err == nil {
		bt := bytes.TrimSpace(b)
		if len(bt) == 0 || string(bt) == "[]" {
			k.Durum = "guncel"
		} else {
			var ups []struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(bt, &ups) == nil {
				if len(ups) > 0 {
					k.Durum = "eski"
					k.SonSurum = ups[0].Version
				} else {
					k.Durum = "guncel"
				}
			}
		}
	}
	cancel2()
	if fi, err := os.Stat(filepath.Join(dizin, "wp-config.php")); err == nil {
		k.KurulumTarihi = fi.ModTime().Format("2006-01-02")
	}
	return k, nil
}

func (Adapter) Guncelle(ctx context.Context, sk, dizin string) error {
	out1, e1 := wpKomut(ctx, sk, "core", "update", "--path="+dizin)
	_, _ = wpKomut(ctx, sk, "core", "update-db", "--path="+dizin)
	if e1 != nil {
		return wpHataOlustur("güncelleme", out1)
	}
	return nil
}
