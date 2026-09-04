// Package altalangoc: alt alan adı (subdomain) kayıtlarını bağımsız domain
// hesaplarına taşır ve bu taşımanın çekirdeğini — "bir alan adı için sıfırdan
// tenant sağla" — ortak bir yerde tutar.
//
// NEDEN AYRI PAKET: aynı mantığa İKİ çağıran var:
//
//  1. canlıdaki mevcut alt alan adlarının göçü (bu paketteki Goc),
//  2. eski sürüm bir SanalCP kaynağından gelen aktarım arşivi — arşiv hâlâ
//     subdomains.jsonl taşıyor ve içindekiler hedefte bağımsız domain olarak
//     açılmalı (internal/transfers).
//
// İkisini de internal/domains içine koymak transfers -> domains -> transfers
// bağımlılığı yaratırdı; mantığı iki yere kopyalamak ise sağlama sırasının
// (FPM soketi, FTP, DB, CLI token, DNS) zamanla ayrışması demekti.
//
// 🔴 KOTA KONTROLÜ YOK: TenantOlustur kasıtlı olarak kota.CheckDomainEklenebilir
// çağırmaz. Bu bir GÖÇ yolu, yeni satış değil — plan sınırındaki bir müşterinin
// var olan alt alan adı, kota yüzünden göç edemeyip yayından kalkamaz.
// Bu yüzden paket yalnız admin uçlarından ve aktarım akışından çağrılmalıdır.
package altalangoc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"sanalcp/internal/adlar"
	"sanalcp/internal/cliapi"
	"sanalcp/internal/dns"
	"sanalcp/internal/hesaplar"
	"sanalcp/internal/provisioner"
	"sanalcp/internal/tenanthesap"
)

// Kayit: göç edilecek tek bir alt alan adının envanter satırı.
type Kayit struct {
	ID          int64  `json:"id"` // subdomanlar.id
	TamAd       string `json:"tam_ad"`
	AltAd       string `json:"alt_ad"`
	PHPSurum    string `json:"php_surum"`
	AnaDomainID int64  `json:"ana_domain_id"`
	AnaAlanAdi  string `json:"ana_alan_adi"`
	EskiSK      string `json:"eski_sk"`
	HedefSK     string `json:"hedef_sk"`
	KaynakDizin string `json:"kaynak_dizin"`
	BoyutKB     int64  `json:"boyut_kb"`
	DosyaSayisi int    `json:"dosya_sayisi"`
	SertifikaVar bool  `json:"sertifika_var"`
	// SymlinkVar: kaynak ağaçta symlink bulundu. Alt alan adı ana hesabın
	// vendor/ gibi dizinlerini paylaşıyor olabilir; göçten sonra bu bağ KOPAR.
	// Ön kontrol bunu engel saymaz ama işaretler — operatör elle bakmalı.
	SymlinkVar bool `json:"symlink_var"`
	// Sorun: doluysa bu kayıt göç EDİLEMEZ (ad çakışması, geçersiz ad, hedef
	// kullanıcı zaten var vb.).
	Sorun string `json:"sorun,omitempty"`
}

// Sonuc: tek bir kaydın göç sonucu.
type Sonuc struct {
	ID       int64  `json:"id"`
	TamAd    string `json:"tam_ad"`
	Basarili bool   `json:"basarili"`
	DomainID int64  `json:"domain_id,omitempty"`
	HedefSK  string `json:"hedef_sk,omitempty"`
	Hata     string `json:"hata,omitempty"`
	// Adimlar: dry-run'da "yapılacaklar", gerçek çalıştırmada "yapıldı" listesi.
	Adimlar []string `json:"adimlar,omitempty"`
}

func docrootOf(sk, tamAd string) string { return "/home/" + sk + "/subdomains/" + tamAd }
func subConfPath(sk, altAd string) string {
	return "/etc/nginx/conf.d/sub_" + sk + "_" + altAd + ".conf"
}
func sslDir(sk string) string { return "/home/" + sk + "/ssl" }

// Envanter: göç edilecek tüm alt alan adlarını, disk durumları ve ön kontrol
// sonuçlarıyla birlikte döner. Hiçbir şey yazmaz.
func Envanter(ctx context.Context, db *sql.DB) ([]Kayit, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.alt_ad, s.tam_ad, COALESCE(s.php_surum,'8.3'),
		       d.id, d.alan_adi, d.sistem_kullanici
		FROM subdomanlar s
		JOIN domains d ON d.id = s.domain_id
		ORDER BY d.alan_adi, s.alt_ad`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Kayit{}
	for rows.Next() {
		var k Kayit
		if err := rows.Scan(&k.ID, &k.AltAd, &k.TamAd, &k.PHPSurum,
			&k.AnaDomainID, &k.AnaAlanAdi, &k.EskiSK); err != nil {
			return nil, err
		}
		k.HedefSK = provisioner.SlugFromDomain(k.TamAd)
		k.KaynakDizin = docrootOf(k.EskiSK, k.TamAd)
		k.BoyutKB, k.DosyaSayisi, k.SymlinkVar = agacOzeti(k.KaynakDizin)
		crt, _ := certYolu(k.EskiSK, k.TamAd)
		k.SertifikaVar = dosyaVar(crt)
		k.Sorun = onKontrol(ctx, db, &k)
		out = append(out, k)
	}
	return out, rows.Err()
}

// onKontrol: kaydın göç edilebilir olup olmadığını belirler. Boş dönerse
// edilebilir. Hiçbir şey yazmaz — dry-run da gerçek çalıştırma da bunu kullanır.
func onKontrol(ctx context.Context, db *sql.DB, k *Kayit) string {
	if !adlar.SKGecerli(k.EskiSK) {
		return "ana domainin sistem kullanıcısı geçersiz: " + k.EskiSK
	}
	if err := provisioner.ValidateDomain(k.TamAd); err != nil {
		return "alan adı geçersiz: " + err.Error()
	}
	if !adlar.SKGecerli(k.HedefSK) {
		return "hedef sistem kullanıcısı üretilemedi: " + k.HedefSK
	}
	// Aynı slug'a düşen BAŞKA bir domain varsa göç, o hesabın dosyalarının
	// üstüne yazardı. SlugFromDomain 26 karakterde kestiği için uzun adlarda
	// çakışma gerçek bir olasılık.
	var cakisan string
	err := db.QueryRowContext(ctx,
		`SELECT alan_adi FROM domains WHERE sistem_kullanici=? LIMIT 1`, k.HedefSK).Scan(&cakisan)
	if err == nil {
		return "hedef sistem kullanıcısı (" + k.HedefSK + ") zaten " + cakisan + " tarafından kullanılıyor"
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "çakışma kontrolü başarısız: " + err.Error()
	}
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM domains WHERE alan_adi=?`, k.TamAd).Scan(&n); err == nil && n > 0 {
		return "bu ad zaten bağımsız bir domain olarak kayıtlı"
	}
	if !dizinVar(k.KaynakDizin) {
		return "kaynak dizin yok: " + k.KaynakDizin
	}
	return ""
}

// Goc: verilen alt alan adlarını bağımsız domain hesaplarına taşır.
//
// dryRun=true iken HİÇBİR yazma yapılmaz; her kayıt için yapılacak adımlar
// listelenir. Kayıtlar birbirinden bağımsızdır: biri hata verirse diğerleri
// denenmeye devam eder, sonuç listesinde hangisinin neden düştüğü durur.
//
// nginx yalnız EN SONDA bir kez doğrulanıp yeniden yüklenir. Kayıt başına
// reload etmek, N alt alan adında N kesinti penceresi demekti.
func Goc(ctx context.Context, db *sql.DB, ipv4 string, ids []int64, dryRun bool) ([]Sonuc, error) {
	tumu, err := Envanter(ctx, db)
	if err != nil {
		return nil, err
	}
	istenen := map[int64]bool{}
	for _, id := range ids {
		istenen[id] = true
	}

	sonuclar := []Sonuc{}
	degisti := false
	for i := range tumu {
		k := tumu[i]
		if len(istenen) > 0 && !istenen[k.ID] {
			continue
		}
		s := Sonuc{ID: k.ID, TamAd: k.TamAd, HedefSK: k.HedefSK}
		if k.Sorun != "" {
			s.Hata = k.Sorun
			sonuclar = append(sonuclar, s)
			continue
		}
		if dryRun {
			s.Basarili = true
			s.Adimlar = planAdimlari(k)
			sonuclar = append(sonuclar, s)
			continue
		}
		domainID, adimlar, err := gocEt(ctx, db, ipv4, k)
		s.Adimlar = adimlar
		if err != nil {
			s.Hata = err.Error()
		} else {
			s.Basarili = true
			s.DomainID = domainID
			degisti = true
		}
		sonuclar = append(sonuclar, s)
	}

	if degisti {
		if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
			// Reload YAPMA: çalışan nginx bozuk conf'tan etkilenmez, ama reload
			// edilirse TÜM sunucudaki siteler düşer.
			return sonuclar, fmt.Errorf("nginx doğrulanamadı, reload yapılmadı: %s",
				strings.TrimSpace(string(out)))
		}
		_ = exec.Command("systemctl", "reload", "nginx").Run()
	}
	return sonuclar, nil
}

func planAdimlari(k Kayit) []string {
	a := []string{
		fmt.Sprintf("Linux kullanıcısı %s + PHP-FPM havuzu + vhost sağla (PHP %s)", k.HedefSK, k.PHPSurum),
		fmt.Sprintf("domains satırı yaz, ana domainin (%s) müşterisine bağla", k.AnaAlanAdi),
		fmt.Sprintf("%d dosya (%d KB) → /home/%s/public_html", k.DosyaSayisi, k.BoyutKB, k.HedefSK),
	}
	if k.SertifikaVar {
		a = append(a, "sertifikayı kopyala ve vhost'u SSL'li yaz")
	} else {
		a = append(a, "sertifika yok — SSL kapalı başlar, SSL sayfasından alınmalı")
	}
	if k.SymlinkVar {
		a = append(a, "⚠ kaynak ağaçta symlink var — ana hesapla paylaşılan dosya bağı kopabilir")
	}
	a = append(a,
		fmt.Sprintf("eski vhost'u sil: %s", subConfPath(k.EskiSK, k.AltAd)),
		fmt.Sprintf("ana zone'dan %s A kaydını sil", k.AltAd),
		"subdomanlar satırını sil (kaynak dizin diskte BIRAKILIR)")
	return a
}

func gocEt(ctx context.Context, db *sql.DB, ipv4 string, k Kayit) (int64, []string, error) {
	adimlar := []string{}
	ekle := func(f string, v ...any) { adimlar = append(adimlar, fmt.Sprintf(f, v...)) }

	// Sahiplik ana domainden devralınır: göç, domainin kimin olduğunu değiştirmemeli.
	customerID, sahipBayi := Sahiplik(ctx, db, k.AnaDomainID)

	domainID, sk, err := TenantOlustur(ctx, db, k.TamAd, k.PHPSurum, ipv4, customerID, sahipBayi)
	if err != nil {
		return 0, adimlar, err
	}
	ekle("tenant sağlandı: %s (domain id %d)", sk, domainID)

	// Dosyalar. Hata burada geri alınabilir olmalı — tenant sağlandı ama içerik
	// taşınamadıysa yarım bir site bırakmaktansa domaini geri al.
	if err := agacTasi(k.KaynakDizin, sk); err != nil {
		_ = provisioner.Deprovision(k.TamAd, sk)
		_, _ = db.ExecContext(ctx, `DELETE FROM domains WHERE id=?`, domainID)
		return 0, adimlar, fmt.Errorf("dosya taşıma: %w", err)
	}
	ekle("%d dosya /home/%s/public_html altına açıldı", k.DosyaSayisi, sk)

	// Sertifika: varsa kopyala. ACME göç anında ÇAĞRILMAZ — hepsi-ya-hiçbiri
	// davranışı yüzünden tek bir çözülmeyen ad tüm partiyi düşürürdü.
	if err := sertifikaTasi(ctx, db, k, sk, domainID); err != nil {
		log.Printf("altalangoc: sertifika taşınamadı (%s): %v", k.TamAd, err)
		ekle("sertifika taşınamadı, SSL kapalı: %v", err)
	} else if k.SertifikaVar {
		ekle("sertifika kopyalandı, vhost SSL'li yazıldı")
	}

	// Eski artıklar. Kaynak DİZİN silinmez: göç yanlış giderse geri dönülecek
	// tek yer orasıdır. Operatör doğruladıktan sonra elle siler.
	if err := os.Remove(subConfPath(k.EskiSK, k.AltAd)); err != nil && !os.IsNotExist(err) {
		log.Printf("altalangoc: eski vhost silinemedi (%s): %v", k.TamAd, err)
	} else {
		ekle("eski vhost silindi")
	}
	if _, err := db.ExecContext(ctx,
		`DELETE FROM dns_records WHERE domain_id=? AND ad=? AND tip='A'`, k.AnaDomainID, k.AltAd); err == nil {
		if err := dns.WriteZone(ctx, db, k.AnaDomainID); err != nil {
			log.Printf("altalangoc: ana zone yazılamadı (%s): %v", k.AnaAlanAdi, err)
		}
		ekle("ana zone'dan A kaydı silindi")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM subdomanlar WHERE id=?`, k.ID); err != nil {
		return domainID, adimlar, fmt.Errorf("subdomanlar satırı silinemedi: %w", err)
	}
	ekle("subdomanlar satırı silindi (kaynak dizin %s diskte bırakıldı)", k.KaynakDizin)
	return domainID, adimlar, nil
}

// Sahiplik: bir domainin müşterisini ve o müşterinin sahip bayisini döner.
// Alt alan adından türetilen yeni domain aynı müşteride kalmalı — aksi hâlde
// göç/aktarım, siteyi sahibinin panelinden görünmez yapardı.
func Sahiplik(ctx context.Context, db *sql.DB, domainID int64) (customerID, sahipBayi *int64) {
	var cid, oid sql.NullInt64
	_ = db.QueryRowContext(ctx,
		`SELECT d.customer_id, c.owner_user_id FROM domains d
		 LEFT JOIN customers c ON c.id = d.customer_id WHERE d.id=?`, domainID).Scan(&cid, &oid)
	if cid.Valid {
		v := cid.Int64
		customerID = &v
	}
	if oid.Valid {
		v := oid.Int64
		sahipBayi = &v
	}
	return customerID, sahipBayi
}

// TenantOlustur: bir alan adı için sıfırdan bağımsız domain hesabı sağlar —
// Linux kullanıcısı, PHP-FPM havuzu, nginx vhost, domains satırı, panel
// müşterisi, FTP hesabı, MySQL veritabanı, CLI token ve DNS zone.
//
// internal/domains Create akışıyla AYNI sırayı izler (bkz. handlers.go "1) …5)"
// numaralı adımlar); tek fark kota kontrollerinin olmamasıdır — paket başlığındaki
// nota bakın. Sağlama yarıda kalırsa açılmış olan her şey geri alınır.
//
// customerID nil ise tenanthesap.Hazirla yeni bir panel hesabı/müşteri üretir.
func TenantOlustur(ctx context.Context, db *sql.DB, alanAdi, phpSurum, ipv4 string,
	customerID *int64, sahipBayi *int64) (int64, string, error) {

	pr, err := provisioner.Provision(alanAdi, phpSurum)
	if err != nil {
		return 0, "", fmt.Errorf("sağlama: %w", err)
	}
	sk := pr.SistemKullanici
	dbUser := sk + "_db"
	dbName := sk + "_main"

	res, err := db.ExecContext(ctx,
		`INSERT INTO domains(alan_adi, sistem_kullanici, php_surum, ssl_aktif, durum, ipv4,
		   ftp_host, ftp_user, db_host, db_user, db_adi, web_root, is_demo, site_tipi)
		 VALUES(?,?,?,0,'aktif',?,?,?, 'localhost',?,?,?, 0, 'php')`,
		alanAdi, sk, pr.PHPSurum, ipv4, ipv4, sk, dbUser, dbName, pr.WebRoot)
	if err != nil {
		_ = provisioner.Deprovision(alanAdi, sk)
		return 0, "", fmt.Errorf("domains kaydı: %w", err)
	}
	domainID, _ := res.LastInsertId()

	if customerID != nil {
		_, _ = db.ExecContext(ctx, `UPDATE domains SET customer_id=? WHERE id=?`, *customerID, domainID)
	} else if _, err := tenanthesap.Hazirla(ctx, db, sk, alanAdi, sahipBayi); err != nil {
		// Domain sağlandı ve kaydedildi; hesap zinciri kurulamadıysa göç
		// başarısız SAYILMAZ — açılıştaki doldurma bunu yine yakalar.
		log.Printf("altalangoc: tenant hesabı hazırlanamadı (domain=%d, tenant=%s): %v", domainID, sk, err)
	}

	uidN, gidN := uidGidOf(sk)
	if err := hesaplar.FTPCreate(db, domainID, sk, hesaplar.RandomParola(20), uidN, gidN); err != nil {
		log.Printf("altalangoc: FTP create %q: %v", sk, err)
	}
	if err := hesaplar.MySQLCreateDB(db, domainID, dbName, dbUser, hesaplar.RandomParola(24)); err != nil {
		log.Printf("altalangoc: MySQL create %q: %v", dbName, err)
	}
	if tok, err := cliapi.GenerateToken(db, domainID); err != nil {
		log.Printf("altalangoc: CLI token %q: %v", sk, err)
	} else if err := cliapi.WriteTokenFile(sk, tok, uidN, gidN); err != nil {
		log.Printf("altalangoc: CLI token dosyası %q: %v", sk, err)
	}
	if _, err := dns.SeedDefaults(ctx, db, domainID, alanAdi, ipv4); err != nil {
		log.Printf("altalangoc: DNS SeedDefaults %q: %v", alanAdi, err)
	}
	if err := dns.WriteZone(ctx, db, domainID); err != nil {
		log.Printf("altalangoc: DNS WriteZone %q: %v", alanAdi, err)
	}
	return domainID, sk, nil
}

// agacTasi: kaynak ağacı hedef tenant'ın public_html'ine KOPYALAR (kaynak
// silinmez).
//
// 🔴 tar KULLANILIR, cp/zip DEĞİL: tar symlink'i symlink olarak saklar; zip
// varsayılanda takip eder ve root olarak çalışan bir kopyalama, tenant'ın
// yazabildiği bir ağaçta symlink'i izleyerek jail dışına yazardı. Açma tarafı
// runuser ile hedef kullanıcı olarak çalışır — root asla hedefe yazmaz.
// (internal/transfers restoreNativeChildTrees ile aynı desen.)
func agacTasi(kaynak, hedefSK string) error {
	if !adlar.SKGecerli(hedefSK) {
		return errors.New("güvensiz hedef kullanıcı")
	}
	if !dizinVar(kaynak) {
		return errors.New("kaynak dizin yok: " + kaynak)
	}
	hedef := "/home/" + hedefSK + "/public_html"

	// Provision'ın yazdığı karşılama sayfası, taşınan sitenin index'ini
	// gölgeleyebilir (nginx index sırası). Kaynak boş değilse kaldır.
	if bos, _ := dizinBos(kaynak); !bos {
		_ = os.Remove(filepath.Join(hedef, "index.html"))
	}

	okuma := exec.Command("tar", "-cf", "-", "-C", kaynak, ".")
	yazma := exec.Command("runuser", "-u", hedefSK, "--", "tar", "-xf", "-", "-C", hedef)
	boru, err := okuma.StdoutPipe()
	if err != nil {
		return err
	}
	var okumaHata, yazmaHata strings.Builder
	okuma.Stderr = &okumaHata
	yazma.Stderr = &yazmaHata
	yazma.Stdin = boru
	if err := okuma.Start(); err != nil {
		return err
	}
	if err := yazma.Start(); err != nil {
		_ = okuma.Process.Kill()
		_ = okuma.Wait()
		return err
	}
	// Okuma ÖNCE beklenir: yazma tarafı boruyu tüketirken okuma biter, sonra
	// boru kapanır ve yazma EOF görür. Ters sırada büyük ağaçlarda kilitlenir.
	errOku := okuma.Wait()
	errYaz := yazma.Wait()
	if errOku != nil {
		return fmt.Errorf("tar okuma: %s: %w", strings.TrimSpace(okumaHata.String()), errOku)
	}
	if errYaz != nil {
		return fmt.Errorf("tar yazma: %s: %w", strings.TrimSpace(yazmaHata.String()), errYaz)
	}
	_, _ = exec.Command("restorecon", "-RF", hedef).CombinedOutput()
	return nil
}

func certYolu(sk, tamAd string) (string, string) {
	d := sslDir(sk)
	return filepath.Join(d, tamAd+".crt"), filepath.Join(d, tamAd+".key")
}

// sertifikaTasi: alt alan adının sertifikası varsa yeni tenant'a kopyalar ve
// vhost'u SSL'li varyantla yeniden yazdırır. Sertifika yoksa sessizce geçer —
// yeni domain HTTP'de açılır, kullanıcı SSL sayfasından Let's Encrypt alır.
func sertifikaTasi(ctx context.Context, db *sql.DB, k Kayit, yeniSK string, domainID int64) error {
	if !k.SertifikaVar {
		return nil
	}
	eskiCrt, eskiKey := certYolu(k.EskiSK, k.TamAd)
	yeniCrt, yeniKey := certYolu(yeniSK, k.TamAd)
	if err := os.MkdirAll(sslDir(yeniSK), 0o750); err != nil {
		return err
	}
	for _, c := range [][2]string{{eskiCrt, yeniCrt}, {eskiKey, yeniKey}} {
		ham, err := os.ReadFile(c[0])
		if err != nil {
			return err
		}
		mod := fs.FileMode(0o600)
		if strings.HasSuffix(c[1], ".crt") {
			mod = 0o644
		}
		if err := os.WriteFile(c[1], ham, mod); err != nil {
			return err
		}
	}
	uidN, gidN := uidGidOf(yeniSK)
	_ = os.Chown(yeniCrt, uidN, gidN)
	_ = os.Chown(yeniKey, uidN, gidN)

	if _, err := db.ExecContext(ctx, `UPDATE domains SET ssl_aktif=1 WHERE id=?`, domainID); err != nil {
		return err
	}
	return provisioner.RerenderVhost(db, domainID)
}

func uidGidOf(u string) (int, int) {
	uu, err := user.Lookup(u)
	if err != nil {
		return 0, 0
	}
	uid, _ := strconv.Atoi(uu.Uid)
	gid, _ := strconv.Atoi(uu.Gid)
	return uid, gid
}

func dosyaVar(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.Mode().IsRegular()
}

func dizinVar(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func dizinBos(p string) (bool, error) {
	f, err := os.Open(p)
	if err != nil {
		return false, err
	}
	defer f.Close()
	girdiler, err := f.Readdirnames(1)
	return len(girdiler) == 0, err
}

// agacOzeti: envanter için kaba boyut/dosya sayısı ve symlink varlığı. Hata
// durumunda sıfır döner — envanter tamamlayıcı bilgidir, tek bir okunamayan
// dizin yüzünden listeyi düşürmek doğru olmaz.
func agacOzeti(kok string) (boyutKB int64, dosya int, symlink bool) {
	var toplam int64
	_ = filepath.WalkDir(kok, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			symlink = true
			return nil
		}
		if d.IsDir() {
			return nil
		}
		dosya++
		if fi, e := d.Info(); e == nil {
			toplam += fi.Size()
		}
		return nil
	})
	return toplam / 1024, dosya, symlink
}
