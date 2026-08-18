package guvenlikduvari

// Otomatik saldırı engelleme (fail2ban muadili).
//
// TASARIM
//
// Sistem servislerinin kimlik doğrulama hataları journald üzerinden izlenir.
// Bir IP, `pencere` içinde `esik` kadar başarısız denemeye ulaşırsa nftables
// tarafında `sure` boyunca engellenir.
//
// Ban kayıtları ELLE eklenen kurallarla aynı tabloda (firewall_kurallari,
// kaynak='otomatik') tutulur — ayrı bir kural kaynağı YOKTUR. Bunun üç sonucu
// vardır: banlar panel güvenlik duvarı ekranında görünür ve elle silinebilir;
// tek bir nft ruleset üreticisi vardır (rebuildDB); mevcut whitelist kuralları
// üretilen ruleset'te zaten banlardan ÖNCE geldiği için otomatik banı da ezer.
//
// GÜVENLİK / KİLİTLENME KORUMASI
//
// Yanlış bir ban yöneticiyi kendi sunucusundan atabilir. Bu yüzden:
//
//   - Özellik VARSAYILAN OLARAK KAPALIDIR (panel_ayarlari.otoban_aktif=0).
//   - Whitelist'teki IP/CIDR'ler asla banlanmaz (banlamadan ÖNCE kontrol edilir;
//     ruleset sırası zaten ikinci bir savunma katmanıdır).
//   - Sunucunun kendi adresleri, loopback ve link-local asla banlanmaz.
//   - Banlar SÜRELİDİR; süresi dolan ruleset'e hiç alınmaz (rebuildDB) ve
//     temizlik döngüsü satırı siler. Panel çökse bile ban sonsuza kadar kalmaz.
//   - nft ruleset'inde "ct state established,related accept" en üstte olduğu
//     için ban YALNIZ yeni bağlantıları etkiler; yöneticinin açık SSH oturumu
//     kendi IP'si banlansa bile kopmaz.
//
// Sayaçlar bellekte tutulur: panel yeniden başlayınca sıfırlanır. Bu bilinçli
// bir tercihtir — güvenli yöndedir (fazladan ban yerine eksik ban) ve her
// başarısız denemede DB'ye yazma baskısı oluşturmaz.

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// izlenenBirimler: journald'dan takip edilen systemd unit'leri. Kurulu olmayan
// bir unit journalctl için hata değildir (yalnızca satır üretmez), bu yüzden
// hepsi koşulsuz istenir.
var izlenenBirimler = []string{
	"sshd.service",
	"dovecot.service",
	"postfix.service",
	"pure-ftpd.service",
}

// desen: bir günlük satırını servise ve saldırgan IP'sine eşleyen kural.
// Regex'lerin İLK yakalama grubu IP olmalıdır.
type desen struct {
	servis string
	re     *regexp.Regexp
}

// desenler: yaygın kimlik doğrulama hatası satırları.
//
// IP grubu bilerek geniş tutulur ([0-9a-fA-F:.]+) — IPv6 de yakalanır; gerçek
// doğrulama net.ParseIP ile yapılır, uymayan satır sessizce düşer.
var desenler = []desen{
	// --- sshd ---
	// "Failed password for root from 1.2.3.4 port 22 ssh2"
	// "Failed password for invalid user admin from 1.2.3.4 port 22 ssh2"
	{"ssh", regexp.MustCompile(`Failed (?:password|publickey) for (?:invalid user )?\S+ from ([0-9a-fA-F:.]+) port \d+`)},
	// "Invalid user admin from 1.2.3.4 port 54321"
	{"ssh", regexp.MustCompile(`Invalid user \S* ?from ([0-9a-fA-F:.]+) port \d+`)},
	// "error: maximum authentication attempts exceeded for root from 1.2.3.4 port 22 ssh2"
	{"ssh", regexp.MustCompile(`maximum authentication attempts exceeded for \S+ from ([0-9a-fA-F:.]+) port \d+`)},
	// "Connection closed by authenticating user root 1.2.3.4 port 22 [preauth]"
	{"ssh", regexp.MustCompile(`Connection (?:closed|reset) by (?:authenticating|invalid) user \S* ([0-9a-fA-F:.]+) port \d+`)},

	// --- dovecot (IMAP/POP3) ---
	// "imap-login: Aborted login (auth failed, 3 attempts ...): user=<x>, ..., rip=1.2.3.4, lip=..."
	// "auth: pam(x,1.2.3.4): pam_authenticate() failed"
	{"mail", regexp.MustCompile(`(?:auth failed|Aborted login|authentication failure).*?rip=([0-9a-fA-F:.]+)`)},

	// --- postfix (SMTP AUTH / SASL) ---
	// "warning: unknown[1.2.3.4]: SASL LOGIN authentication failed: authentication failure"
	{"mail", regexp.MustCompile(`\[([0-9a-fA-F:.]+)\]: SASL (?:LOGIN|PLAIN|CRAM-MD5) authentication failed`)},

	// --- pure-ftpd ---
	// "pure-ftpd: (?@1.2.3.4) [WARNING] Authentication failed for user [x]"
	{"ftp", regexp.MustCompile(`\(\?@([0-9a-fA-F:.]+)\).*Authentication failed`)},
}

// otoBanAyar: panel_ayarlari'ndan okunan çalışma ayarları.
type otoBanAyar struct {
	Aktif     bool
	Esik      int
	PencereDk int
	SureDk    int
}

// ayarCache: ayarlar her eşleşen günlük satırında DB'den okunursa, saldırı
// anında (saniyede yüzlerce satır) sorgu fırtınası oluşur. Kısa ömürlü önbellek
// hem bunu önler hem de "panel yeniden başlatmadan açılıp kapanabilme"
// davranışını korur — değişiklik en geç ayarTazelemeSuresi içinde yansır.
const ayarTazelemeSuresi = 15 * time.Second

var ayarCache struct {
	mu    sync.Mutex
	deger otoBanAyar
	zaman time.Time
}

func ayarOku(ctx context.Context, db *sql.DB) otoBanAyar {
	ayarCache.mu.Lock()
	defer ayarCache.mu.Unlock()
	if time.Since(ayarCache.zaman) < ayarTazelemeSuresi {
		return ayarCache.deger
	}
	a := ayarSorgula(ctx, db)
	ayarCache.deger = a
	ayarCache.zaman = time.Now()
	return a
}

// ayarCacheBosalt: ayar kaydedildiğinde çağrılır — değişiklik önbellek süresini
// beklemeden geçerli olur.
func ayarCacheBosalt() {
	ayarCache.mu.Lock()
	ayarCache.zaman = time.Time{}
	ayarCache.mu.Unlock()
}

func ayarSorgula(ctx context.Context, db *sql.DB) otoBanAyar {
	var a otoBanAyar
	var aktif int
	err := db.QueryRowContext(ctx,
		`SELECT otoban_aktif, otoban_esik, otoban_pencere_dk, otoban_sure_dk
		 FROM panel_ayarlari WHERE id=1`).
		Scan(&aktif, &a.Esik, &a.PencereDk, &a.SureDk)
	if err != nil {
		return otoBanAyar{} // okunamadı → kapalı say (fail-safe: banlamama yönünde)
	}
	a.Aktif = aktif == 1
	// Bozuk/sıfır değerlere karşı savunma — sıfır pencere sonsuz sayaç demek olurdu.
	if a.Esik <= 0 {
		a.Esik = 5
	}
	if a.PencereDk <= 0 {
		a.PencereDk = 10
	}
	if a.SureDk <= 0 {
		a.SureDk = 60
	}
	return a
}

// sayac: IP başına kayan pencere içindeki başarısız deneme zamanları.
type sayac struct {
	mu sync.Mutex
	m  map[string][]time.Time
}

func yeniSayac() *sayac { return &sayac{m: make(map[string][]time.Time)} }

// ekle: IP'ye bir hata kaydeder ve pencere içindeki toplam hata sayısını döner.
func (s *sayac) ekle(ip string, pencere time.Duration) int {
	simdi := time.Now()
	esik := simdi.Add(-pencere)
	s.mu.Lock()
	defer s.mu.Unlock()
	kayit := append(s.m[ip], simdi)
	// pencere dışına düşenleri at
	taze := kayit[:0]
	for _, t := range kayit {
		if t.After(esik) {
			taze = append(taze, t)
		}
	}
	s.m[ip] = taze
	return len(taze)
}

// unut: IP banlandıktan sonra sayacını sıfırlar (ban süresince tekrar tekrar
// ban yazılmasını önler).
func (s *sayac) unut(ip string) {
	s.mu.Lock()
	delete(s.m, ip)
	s.mu.Unlock()
}

// budaAnahtarlar: uzun süredir dokunulmamış IP kayıtlarını siler (bellek sızıntısı
// koruması — sürekli saldırı altında map büyümesin).
func (s *sayac) budaAnahtarlar(pencere time.Duration) {
	esik := time.Now().Add(-pencere)
	s.mu.Lock()
	defer s.mu.Unlock()
	for ip, kayit := range s.m {
		son := time.Time{}
		for _, t := range kayit {
			if t.After(son) {
				son = t
			}
		}
		if son.Before(esik) {
			delete(s.m, ip)
		}
	}
}

// OtoBanBaslat: izleyiciyi ve temizlik döngüsünü başlatır. Panel açılışında bir
// kez çağrılır (bkz. cmd/server/main.go) ve ctx iptal edilene kadar çalışır.
//
// Ayar KAPALI olsa bile başlatılır: ayarlar döngü içinde periyodik okunur, böylece
// yönetici paneli açıp kapattığında panel yeniden başlatmaya gerek kalmaz.
func OtoBanBaslat(ctx context.Context, db *sql.DB) {
	if db == nil {
		return
	}
	s := yeniSayac()
	go temizlikDongusu(ctx, db, s)
	go izleDongusu(ctx, db, s)
}

// temizlikDongusu: süresi dolmuş otomatik banları siler ve ruleset'i tazeler.
func temizlikDongusu(ctx context.Context, db *sql.DB, s *sayac) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		res, err := db.ExecContext(ctx,
			`DELETE FROM firewall_kurallari
			 WHERE kaynak='otomatik' AND bitis_at IS NOT NULL AND bitis_at <= NOW()`)
		if err != nil {
			continue
		}
		if n, _ := res.RowsAffected(); n > 0 {
			if err := rebuildDB(db); err != nil {
				log.Printf("otoban: süresi dolan %d ban silindi ama ruleset güncellenemedi: %v", n, err)
			} else {
				log.Printf("otoban: süresi dolan %d ban kaldırıldı", n)
			}
		}
		s.budaAnahtarlar(2 * time.Hour)
	}
}

// izleDongusu: journalctl'i takip eder; süreç ölürse geri çekilmeli olarak yeniden başlatır.
func izleDongusu(ctx context.Context, db *sql.DB, s *sayac) {
	bekle := 5 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		basladi := time.Now()
		if err := izle(ctx, db, s); err != nil && ctx.Err() == nil {
			log.Printf("otoban: günlük izleyici durdu (%v) — yeniden bağlanılacak", err)
		}
		if ctx.Err() != nil {
			return
		}
		// Uzun süre sorunsuz çalıştıysa bekleme süresini sıfırla; sürekli hemen
		// ölüyorsa 5sn → 2dk arası kademeli geri çekil (journalctl yoksa CPU yakmasın).
		if time.Since(basladi) > time.Minute {
			bekle = 5 * time.Second
		} else if bekle < 2*time.Minute {
			bekle *= 2
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(bekle):
		}
	}
}

// izle: journalctl'i JSON kipinde takip eder, eşleşen satırlarda değerlendir çağırır.
func izle(ctx context.Context, db *sql.DB, s *sayac) error {
	arg := []string{"-f", "-o", "json", "--since", "now", "-n", "0"}
	for _, u := range izlenenBirimler {
		arg = append(arg, "-u", u)
	}
	cmd := exec.CommandContext(ctx, "journalctl", arg...)
	cikti, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() { _ = cmd.Wait() }()

	sc := bufio.NewScanner(cikti)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // uzun günlük satırları
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var kayit struct {
			Message any `json:"MESSAGE"`
		}
		if json.Unmarshal(sc.Bytes(), &kayit) != nil {
			continue
		}
		mesaj, ok := kayit.Message.(string)
		if !ok || mesaj == "" {
			continue // MESSAGE dizi (ikili günlük) olabilir — atla
		}
		degerlendir(ctx, db, s, mesaj)
	}
	return sc.Err()
}

// degerlendir: tek bir günlük satırını desenlerle eşleştirir; eşik aşılırsa banlar.
func degerlendir(ctx context.Context, db *sql.DB, s *sayac, mesaj string) {
	for _, d := range desenler {
		m := d.re.FindStringSubmatch(mesaj)
		if m == nil || len(m) < 2 {
			continue
		}
		ip := strings.TrimSpace(m[1])
		if net.ParseIP(ip) == nil {
			return // desen eşleşti ama IP geçersiz → yoksay
		}
		ayar := ayarOku(ctx, db)
		if !ayar.Aktif {
			return
		}
		if banlanmaz(ip) {
			return
		}
		n := s.ekle(ip, time.Duration(ayar.PencereDk)*time.Minute)
		if n < ayar.Esik {
			return
		}
		if whitelistKapsiyorMu(ctx, db, ip) {
			s.unut(ip) // whitelist'te — bir daha sayma
			return
		}
		if err := banla(ctx, db, ip, d.servis, ayar); err != nil {
			log.Printf("otoban: %s banlanamadı (%s): %v", ip, d.servis, err)
			return
		}
		s.unut(ip)
		log.Printf("otoban: %s engellendi — %s servisinde %d dk içinde %d başarısız deneme (%d dk süreyle)",
			ip, d.servis, ayar.PencereDk, n, ayar.SureDk)
		return
	}
}

// banla: süreli ban satırını yazar ve ruleset'i tazeler. Aynı IP için zaten aktif
// bir ban varsa yalnızca bitiş zamanını uzatır (mükerrer satır üretmez).
func banla(ctx context.Context, db *sql.DB, ip, servis string, ayar otoBanAyar) error {
	var mevcutID int64
	err := db.QueryRowContext(ctx,
		`SELECT id FROM firewall_kurallari
		 WHERE tip='ban' AND ip=? AND aktif=1
		   AND (bitis_at IS NULL OR bitis_at > NOW()) LIMIT 1`, ip).Scan(&mevcutID)
	switch {
	case err == nil:
		// Süresiz (elle) ban varsa dokunma; süreli ise uzat.
		_, e := db.ExecContext(ctx,
			`UPDATE firewall_kurallari SET bitis_at=DATE_ADD(NOW(), INTERVAL ? MINUTE)
			 WHERE id=? AND bitis_at IS NOT NULL`, ayar.SureDk, mevcutID)
		return e // ruleset zaten bu IP'yi engelliyor, rebuild gereksiz
	case err != sql.ErrNoRows:
		return err
	}

	aciklama := "Otomatik: " + servis + " kimlik doğrulama saldırısı"
	if _, err := db.ExecContext(ctx,
		`INSERT INTO firewall_kurallari (tip, ip, port, protokol, aciklama, aktif, kaynak, servis, bitis_at)
		 VALUES ('ban', ?, 0, 'tcp', ?, 1, 'otomatik', ?, DATE_ADD(NOW(), INTERVAL ? MINUTE))`,
		ip, aciklama, servis, ayar.SureDk); err != nil {
		return err
	}
	return rebuildDB(db)
}

// banlanmaz: hiçbir koşulda banlanmaması gereken adresler.
//
// Sunucunun kendi adreslerinin banlanması, servislerin birbirine yaptığı
// bağlantıları (ör. webmail → dovecot) koparabilir; loopback banı ise paneli
// kendi kendine kilitler.
func banlanmaz(ip string) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return true
	}
	if p.IsLoopback() || p.IsUnspecified() || p.IsLinkLocalUnicast() || p.IsLinkLocalMulticast() {
		return true
	}
	return yerelAdresMi(p)
}

// yerelAdresCache: arayüz adresleri her günlük satırında yeniden okunmasın diye
// kısa süreli önbellek (IP değişimi olursa en geç 1 dakikada yansır).
var yerelAdresCache struct {
	mu       sync.Mutex
	adresler []net.IP
	zaman    time.Time
}

func yerelAdresMi(p net.IP) bool {
	yerelAdresCache.mu.Lock()
	if time.Since(yerelAdresCache.zaman) > time.Minute {
		if addrs, err := net.InterfaceAddrs(); err == nil {
			liste := make([]net.IP, 0, len(addrs))
			for _, a := range addrs {
				if n, ok := a.(*net.IPNet); ok {
					liste = append(liste, n.IP)
				}
			}
			yerelAdresCache.adresler = liste
			yerelAdresCache.zaman = time.Now()
		}
	}
	liste := yerelAdresCache.adresler
	yerelAdresCache.mu.Unlock()
	for _, a := range liste {
		if a.Equal(p) {
			return true
		}
	}
	return false
}

// whitelistKapsiyorMu: IP, aktif whitelist kurallarından birinin (tek IP veya
// CIDR) kapsamına giriyor mu? Giriyorsa asla banlanmaz.
func whitelistKapsiyorMu(ctx context.Context, db *sql.DB, ip string) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return true // ayrıştırılamayan adres → banlama yönünde davran
	}
	rows, err := db.QueryContext(ctx,
		`SELECT ip FROM firewall_kurallari WHERE tip='whitelist' AND aktif=1 AND ip<>''`)
	if err != nil {
		return true // whitelist okunamıyorsa banlama (fail-safe)
	}
	defer rows.Close()
	for rows.Next() {
		var wl string
		if rows.Scan(&wl) != nil {
			continue
		}
		wl = strings.TrimSpace(wl)
		if strings.ContainsRune(wl, '/') {
			if _, ag, err := net.ParseCIDR(wl); err == nil && ag.Contains(p) {
				return true
			}
			continue
		}
		if w := net.ParseIP(wl); w != nil && w.Equal(p) {
			return true
		}
	}
	return false
}
