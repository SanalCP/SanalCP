// Backup off-site destinations: FTP/SFTP ve S3 uyumlu uzak depolama.
package backups

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"sanalcp/internal/secretcrypt"
)

type Destination struct {
	ID         int64  `json:"id"`
	DomainID   int64  `json:"domain_id"`
	Tip        string `json:"tip"` // "ftp" | "sftp"
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Kullanici  string `json:"kullanici"`
	Parola     string `json:"parola,omitempty"` // write-only: GET'te boş döner
	UzakDizin  string `json:"uzak_dizin"`
	Bucket     string `json:"bucket,omitempty"`
	Region     string `json:"region,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	PathStyle  bool   `json:"path_style"`
	Aktif      bool   `json:"aktif"`
	SonYukleme string `json:"son_yukleme,omitempty"`
	SonDurum   string `json:"son_durum,omitempty"`
	SonHata    string `json:"son_hata,omitempty"`
	HostKey    string `json:"-"` // SFTP TOFU: pinlenmiş ssh-keyscan çıktısı, API'ye asla dönmez
}

func gecerliTip(t string) bool {
	return t == "ftp" || t == "sftp" || t == "s3" || t == "b2"
}

func objectStorageTip(t string) bool { return t == "s3" || t == "b2" }

// hostRe: FTP/SFTP hedefi için izin verilen hostname/IPv4 karakter kümesi.
// lftp/ssh/curl komut satırlarına bu değer tırnaksız veya kısmen tırnaklı
// gömüldüğünden (bkz. lftpURL, testConnection), ';', '!', '"', '$', '`' gibi
// script/shell meta karakterlerini baştan reddetmek command injection'ı önler.
var hostRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,252}[A-Za-z0-9])?$`)

// gecerliHost: hostname veya IPv4/IPv6 adresini kabul eder; script/shell
// meta karakteri (;, !, ", $, `, |, boşluk, vb.) içeren her şeyi reddeder.
func gecerliHost(h string) bool {
	if h == "" || len(h) > 253 {
		return false
	}
	// IPv6, köşeli parantez olmadan da girilebilir (örn. "::1") — ':' hostRe'de
	// yok, o yüzden ayrıca izin veriyoruz; net.ParseIP zaten sıkı doğrular.
	if strings.Contains(h, ":") {
		return net.ParseIP(h) != nil
	}
	return hostRe.MatchString(h)
}

// ipYasakli: SSRF koruması — loopback (127.0.0.0/8, ::1), RFC1918/RFC4193
// özel ağlar, link-local (169.254.0.0/16 — cloud metadata IP'si dahil, fe80::/10),
// multicast ve unspecified (0.0.0.0) adresleri reddeder.
func ipYasakli(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// hostGuvenliMi: host bir IP literaliyse doğrudan, hostname ise DNS çözümü
// SONRASI TÜM adreslerin genel (public) olduğunu doğrular. Herhangi bir hosting
// müşterisi backup_destinations kaydı oluşturabildiği için (MusteriScope route)
// bu kontrol olmadan panelin kendi sunucusuna/iç ağına (127.0.0.1, RFC1918,
// cloud metadata 169.254.169.254) SSRF yapılabilirdi.
//
// NOT: DNS rebinding'e karşı TAM koruma değildir (kontrol ile gerçek bağlantı
// arasında TOCTOU penceresi var) — S3 yolu için asıl koruma s3.go'daki
// Dialer.Control ile bağlantı ANINDA yapılıyor. Burası hızlı/erken geri bildirim
// ve lftp/ssh/curl gibi Go dışı süreçlerin (Control ile korunamayan) tek
// savunma katmanıdır.
func hostGuvenliMi(ctx context.Context, host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if ipYasakli(ip) {
			return fmt.Errorf("host: yerel/özel ağ adresine izin verilmiyor")
		}
		return nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("host çözümlenemedi: %w", err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("host çözümlenemedi")
	}
	for _, a := range addrs {
		if ipYasakli(a.IP) {
			return fmt.Errorf("host: yerel/özel ağ adresine çözümleniyor, izin verilmiyor")
		}
	}
	return nil
}

// readDestination: bir domain'in destinasyon kaydını döner (yoksa nil, nil).
func readDestination(ctx context.Context, db *sql.DB, domainID int64) (*Destination, error) {
	d := &Destination{DomainID: domainID}
	var aktif int
	var sonYuk sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT id, tip, host, port, kullanici, parola, uzak_dizin,
		        bucket, region, endpoint, path_style, aktif,
		        DATE_FORMAT(son_yukleme,'%Y-%m-%d %H:%i'), son_durum, son_hata, host_key
		 FROM backup_destinations WHERE domain_id=?`, domainID).
		Scan(&d.ID, &d.Tip, &d.Host, &d.Port, &d.Kullanici, &d.Parola, &d.UzakDizin,
			&d.Bucket, &d.Region, &d.Endpoint, &d.PathStyle, &aktif, &sonYuk,
			&d.SonDurum, &d.SonHata, &d.HostKey)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.Aktif = aktif == 1
	if sonYuk.Valid {
		d.SonYukleme = sonYuk.String
	}
	// Göç öncesi (HealLegacyPlaintextDestinationPasswords çalışmadan önce) satırlar
	// düz metin olabilir — DecryptDBPassword ile aynı desen: eski satır olduğu gibi
	// döner ki bu pencerede kırılmasın.
	if secretcrypt.IsEncrypted(d.Parola) {
		pt, err := box.Decrypt(d.Parola)
		if err != nil {
			return nil, fmt.Errorf("parola çözülemedi: %w", err)
		}
		d.Parola = pt
	}
	return d, nil
}

// lftpURL: tip + host + port'tan lftp URL'i kurar.
func lftpURL(d *Destination) string {
	if d.Tip == "sftp" {
		return fmt.Sprintf("sftp://%s:%d", d.Host, d.Port)
	}
	return fmt.Sprintf("ftp://%s:%d", d.Host, d.Port)
}

// sftpConnectAyarlari: sftp tipi hedefler için lftp'nin varsayılan
// "sftp:auto-confirm yes" (host key'i HER ZAMAN sorgusuz kabul et — MITM'e
// açık) davranışını TOFU pinlemesiyle değiştirir: ilk bağlantıda ssh-keyscan
// ile alınan host key kalıcı olarak pinlenir (ensureHostKey), sonraki HER
// bağlantı sadece o anahtara karşı doğrulanır (StrictHostKeyChecking=yes) —
// anahtar değişirse (MITM veya sunucu gerçekten yeniden kurulmuşsa) bağlantı
// "Host key verification failed" ile reddedilir (aşağıdaki bad-phrase kontrolü
// zaten bunu yakalıyordu). FTP'de host key kavramı yok, no-op döner.
func sftpConnectAyarlari(ctx context.Context, db *sql.DB, d *Destination) (lftpSet string, cleanup func(), err error) {
	if d.Tip != "sftp" {
		return "", func() {}, nil
	}
	keys, err := ensureHostKey(ctx, db, d)
	if err != nil {
		return "", nil, err
	}
	path, err := knownHostsDosyaYaz(keys)
	if err != nil {
		return "", nil, err
	}
	set := fmt.Sprintf(
		`set sftp:auto-confirm no; `+
			`set sftp:connect-program "ssh -a -x -o StrictHostKeyChecking=yes -o UserKnownHostsFile=%s"; `,
		path)
	return set, func() { _ = os.Remove(path) }, nil
}

// lftpCalistir: script'i "-c" argümanı (ps çıktısında görünür, parola dahil)
// yerine STDIN üzerinden verir — parola process argümanlarında ASLA görünmez.
func lftpCalistir(ctx context.Context, script string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "lftp")
	cmd.Stdin = strings.NewReader(script)
	return cmd.CombinedOutput()
}

// uploadToRemote: lokal tar.gz'yi uzak hedefe yükler.
// lftp ile: connect → cd → put. SFTP için pinlenmiş host key doğrulanır.
func uploadToRemote(ctx context.Context, db *sql.DB, d *Destination, localPath, dosyaAdi string) error {
	if !d.Aktif {
		return nil // disable: sessizce skip
	}
	if objectStorageTip(d.Tip) {
		return uploadS3Object(ctx, d, localPath, dosyaAdi)
	}
	if err := hostGuvenliMi(ctx, d.Host); err != nil {
		return err
	}
	sftpSet, cleanup, err := sftpConnectAyarlari(ctx, db, d)
	if err != nil {
		return fmt.Errorf("host key: %w", err)
	}
	defer cleanup()
	url := lftpURL(d)
	// cmd:fail-exit ile herhangi bir komut başarısız olursa lftp non-zero exit eder
	script := fmt.Sprintf(
		`set cmd:fail-exit yes; `+
			`%s`+
			`set ssl:verify-certificate no; `+
			`set ftp:ssl-allow no; `+
			`set net:max-retries 1; `+
			`set net:timeout 15; `+
			`set net:reconnect-interval-base 2; `+
			`open -u "%s","%s" "%s"; `+
			`mkdir -p -f "%s"; `+
			`cd "%s"; `+
			`put -O . "%s"; `+
			`bye`,
		sftpSet, lftpEscape(d.Kullanici), lftpEscape(d.Parola), url,
		lftpEscape(d.UzakDizin), lftpEscape(d.UzakDizin), localPath)

	out, err := lftpCalistir(ctx, script)
	if err != nil {
		return fmt.Errorf("lftp: %s: %w", strings.TrimSpace(string(out)), err)
	}
	// Output'ta dahi hata izi varsa fail say (defense in depth)
	bad := []string{"Login failed", "Access failed", "Connection refused", "Permission denied",
		"Could not resolve", "Host key verification failed", "No route to host"}
	for _, p := range bad {
		if strings.Contains(string(out), p) {
			return fmt.Errorf("lftp: %s", strings.TrimSpace(string(out)))
		}
	}
	_ = dosyaAdi
	return nil
}

func downloadFromRemote(ctx context.Context, db *sql.DB, d *Destination, dosyaAdi, localPath string) error {
	if objectStorageTip(d.Tip) {
		return downloadS3Object(ctx, d, dosyaAdi, localPath)
	}
	if err := hostGuvenliMi(ctx, d.Host); err != nil {
		return err
	}
	sftpSet, cleanup, err := sftpConnectAyarlari(ctx, db, d)
	if err != nil {
		return fmt.Errorf("host key: %w", err)
	}
	defer cleanup()
	script := fmt.Sprintf(
		`set cmd:fail-exit yes; %s`+
			`set ssl:verify-certificate yes; set ftp:ssl-allow no; `+
			`set net:max-retries 1; set net:timeout 15; `+
			`open -u "%s","%s" "%s"; cd "%s"; get "%s" -o "%s"; bye`,
		sftpSet, lftpEscape(d.Kullanici), lftpEscape(d.Parola), lftpURL(d),
		lftpEscape(d.UzakDizin), lftpEscape(dosyaAdi), lftpEscape(localPath))
	out, err := lftpCalistir(ctx, script)
	if err != nil {
		_ = os.Remove(localPath)
		return fmt.Errorf("uzak indirme: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func deleteFromRemote(ctx context.Context, db *sql.DB, d *Destination, dosyaAdi string) error {
	if objectStorageTip(d.Tip) {
		return deleteS3Object(ctx, d, dosyaAdi)
	}
	if err := hostGuvenliMi(ctx, d.Host); err != nil {
		return err
	}
	sftpSet, cleanup, err := sftpConnectAyarlari(ctx, db, d)
	if err != nil {
		return fmt.Errorf("host key: %w", err)
	}
	defer cleanup()
	script := fmt.Sprintf(
		`set cmd:fail-exit yes; %s`+
			`set ssl:verify-certificate yes; set ftp:ssl-allow no; `+
			`set net:max-retries 1; set net:timeout 15; `+
			`open -u "%s","%s" "%s"; cd "%s"; rm "%s"; bye`,
		sftpSet, lftpEscape(d.Kullanici), lftpEscape(d.Parola), lftpURL(d),
		lftpEscape(d.UzakDizin), lftpEscape(dosyaAdi))
	out, err := lftpCalistir(ctx, script)
	if err != nil {
		return fmt.Errorf("uzak silme: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// lftpEscape: lftp komut satırı içinde çift tırnak içine konacak değerleri escape eder.
func lftpEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// testConnection: kimlik bilgilerini test eder.
// SFTP için sshpass+ssh, FTP için curl — her ikisi de auth-specific exit kodu döner.
// Parola HİÇBİR ZAMAN process argümanına (ps çıktısında görünür) konmaz:
// sshpass -e (env var) ve curl --netrc-file (0600 geçici dosya) kullanılır.
func testConnection(ctx context.Context, db *sql.DB, d *Destination) error {
	if objectStorageTip(d.Tip) {
		return testS3Connection(ctx, d)
	}
	if err := hostGuvenliMi(ctx, d.Host); err != nil {
		return err
	}
	if d.Tip == "sftp" {
		keys, err := ensureHostKey(ctx, db, d)
		if err != nil {
			return fmt.Errorf("host key: %w", err)
		}
		khPath, err := knownHostsDosyaYaz(keys)
		if err != nil {
			return fmt.Errorf("host key: %w", err)
		}
		defer os.Remove(khPath)
		// ssh BatchMode=no + PreferredAuthentications=password: publickey
		// by-pass — kullanıcı parolasının gerçekten geçerli olduğunu garanti eder.
		// Pinlenmiş known_hosts + StrictHostKeyChecking=yes: MITM'e karşı korur.
		host := fmt.Sprintf("%s@%s", d.Kullanici, d.Host)
		args := []string{
			"-e", "ssh",
			"-p", fmt.Sprintf("%d", d.Port),
			"-o", "ConnectTimeout=10",
			"-o", "StrictHostKeyChecking=yes",
			"-o", "UserKnownHostsFile=" + khPath,
			"-o", "PreferredAuthentications=password",
			"-o", "PubkeyAuthentication=no",
			"-o", "BatchMode=no",
			host, "true",
		}
		cmd := exec.CommandContext(ctx, "sshpass", args...)
		cmd.Env = append(os.Environ(), "SSHPASS="+d.Parola)
		out, err := cmd.CombinedOutput()
		if err != nil {
			short := strings.TrimSpace(string(out))
			if short == "" {
				short = err.Error()
			}
			return fmt.Errorf("%s", short)
		}
		return nil
	}
	// FTP — curl --netrc-file ftp://host:port/  (NLST root)
	netrcPath, err := ftpNetrcYaz(d.Host, d.Kullanici, d.Parola)
	if err != nil {
		return fmt.Errorf("netrc: %w", err)
	}
	defer os.Remove(netrcPath)
	url := fmt.Sprintf("ftp://%s:%d/", d.Host, d.Port)
	args := []string{
		"-sS",
		"--connect-timeout", "10",
		"--max-time", "15",
		"--netrc-file", netrcPath,
		"--ftp-skip-pasv-ip",
		url,
	}
	cmd := exec.CommandContext(ctx, "curl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		short := strings.TrimSpace(string(out))
		if short == "" {
			short = err.Error()
		}
		return fmt.Errorf("%s", short)
	}
	return nil
}

// ftpNetrcYaz: FTP kimlik bilgilerini 0600 izinli geçici bir .netrc dosyasına
// yazar (curl --netrc-file için) — parola process argümanına ASLA konmaz.
// login/password değerleri çift tırnak içine alınır ki boşluk/özel karakter
// içerebilsinler; netrc biçimi kendi içinde \ ve " karakterlerini escape ile
// destekler (curl >=7.84).
func ftpNetrcYaz(host, kullanici, parola string) (string, error) {
	f, err := os.CreateTemp("", "sanalcp-netrc-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	esc := func(s string) string {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		return s
	}
	content := fmt.Sprintf("machine %s\nlogin \"%s\"\npassword \"%s\"\n",
		host, esc(kullanici), esc(parola))
	if _, err := f.WriteString(content); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// pushToDestinationAsync: yedek başarıyla oluştuktan sonra arkaplanda upload tetikler.
// Hata olsa bile API cevabını bloke etmez; son_durum/son_hata DB'ye yazılır.
func pushToDestinationAsync(db *sql.DB, domainID, backupID int64, localPath, dosyaAdi string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		d, err := readDestination(ctx, db, domainID)
		if err != nil || d == nil || !d.Aktif {
			return
		}
		_, _ = db.Exec(`UPDATE backups SET uzak_durum='yukleniyor', uzak_hata='' WHERE id=?`, backupID)
		if err := uploadToRemote(ctx, db, d, localPath, dosyaAdi); err != nil {
			short := err.Error()
			if len(short) > 500 {
				short = short[:500]
			}
			_, _ = db.Exec(`UPDATE backup_destinations
				SET son_durum='hata', son_hata=?, son_yukleme=NOW() WHERE domain_id=?`,
				short, domainID)
			_, _ = db.Exec(`UPDATE backups SET uzak_durum='hata', uzak_hata=? WHERE id=?`,
				short, backupID)
			log.Printf("backup destination upload domain=%d: %v", domainID, err)
			return
		}
		_, _ = db.Exec(`UPDATE backup_destinations
			SET son_durum='basarili', son_hata='', son_yukleme=NOW() WHERE domain_id=?`,
			domainID)
		remoteKey := strings.Trim(strings.TrimSpace(d.UzakDizin), "/")
		if remoteKey != "" {
			remoteKey += "/"
		}
		remoteKey += dosyaAdi
		_, _ = db.Exec(`UPDATE backups
			SET uzak_durum='basarili', uzak_anahtar=?, uzak_hata='' WHERE id=?`,
			remoteKey, backupID)
		log.Printf("backup destination upload domain=%d başarılı: %s", domainID, dosyaAdi)
	}()
}
