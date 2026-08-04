// Package sqlimport: kullanıcının yüklediği SQL dump'ını hedef veritabanına
// GÜVENLİ biçimde uygular.
//
// 🔴 TEHDİT MODELİ — neden ayrı bir paket:
//
// Dump'ın içeriği TAMAMEN kullanıcı kontrolündedir. Panel root olarak çalıştığı
// için (bkz. assets/systemd/sanalcp.service) `exec.Command("mysql", dbAdi)`
// çağrısı MariaDB'ye root@localhost olarak, unix soketi üzerinden ve PAROLASIZ
// bağlanır. Böyle bir bağlantıya kullanıcı dump'ı akıtmak, DB sunucusunun
// tamamını kullanıcıya vermek demektir:
//
//	USE mysql;
//	UPDATE user SET Super_priv='Y' WHERE User='c_kurban_wp';
//	-- ya da doğrudan: CREATE USER 'arka'@'%' IDENTIFIED BY '...'; GRANT ALL ...
//
// Bu ifadeler hedef veritabanı doğru olsa bile çalışır: `mysql <db>` yalnız
// VARSAYILAN şemayı seçer, yetki sınırı KOYMAZ.
//
// Satır süzmek (CREATE DATABASE / USE satırlarını atmak) bir çözüm DEĞİLDİR:
// `/*!50000 USE mysql */`, çok satırlı ifadeler, `DELIMITER` oyunları ve
// prosedür gövdeleri süzgeci kolayca aşar. Ayrıştırıcı yazmak da MariaDB
// diyalektini yeniden uygulamak demek.
//
// ÇÖZÜM: import ASLA root ile yapılmaz. Hedef veritabanının KENDİ kullanıcısıyla
// bağlanılır; o kullanıcının yetkisi `GRANT ALL PRIVILEGES ON <db>.*` ile tek
// şemayla sınırlıdır (bkz. hesaplar.MySQLCreateDB). Dump ne içerirse içersin
// başka şemaya yazamaz, kullanıcı oluşturamaz, yetki veremez — MariaDB'nin
// kendi yetki denetimi sınırı zorlar, bizim süzgecimiz değil.
//
// Parola argv'ye YAZILMAZ (`ps` ile okunabilirdi — 0.3.9'daki yedek hedefleri
// düzeltmesiyle aynı ders); 0600 izinli geçici bir defaults dosyasıyla verilir.
package sqlimport

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// reDefiner: mysqldump çıktısındaki `DEFINER=<kullanıcı>@<host>` yan tümcesi.
//
// Neden temizleniyor: dump'lar view/trigger/procedure tanımlarını kaynak
// sunucunun kullanıcısına (çoğunlukla `root`@`localhost`) sabitler. Düşük
// yetkili hedef kullanıcı başkası adına nesne oluşturamayacağı için import
// "access denied; you need SUPER privileges" ile patlar. Yan tümce atılınca
// nesne içe aktaran kullanıcıya ait olur. Bu bir GÜVENLİK önlemi değil,
// kullanılabilirlik dönüşümüdür — güvenlik düşük yetkili bağlantıdan gelir.
// NOT: `SQL SECURITY DEFINER` yan tümcesine DOKUNULMAZ. DEFINER= kaldırıldıktan
// sonra nesnenin sahibi zaten içe aktaran (düşük yetkili) kullanıcı olur, yani
// "DEFINER güvenliği" tam olarak istediğimiz şeye denk düşer. INVOKER'a çevirmek
// hiçbir güvenlik kazancı sağlamadığı gibi, mysqldump'ın view'leri üç ayrı
// sürümlü yorumla (`/*!50001 CREATE...*/ /*!50013 DEFINER=...*/ /*!50001 VIEW...*/`)
// yazması yüzünden sözdizimini bozma riski taşır.
var reDefiner = regexp.MustCompile(
	"(?i)DEFINER\\s*=\\s*(?:`(?:[^`]|``)*`|'(?:[^']|'')*'|[^\\s@]+)@(?:`(?:[^`]|``)*`|'(?:[^']|'')*'|[^\\s*/]+)")

// filtreTampon: satır okuma tamponu. Bundan uzun satırlar (dev extended-insert
// satırları) süzgeçten GEÇİRİLMEDEN aktarılır — DEFINER yan tümcesi her zaman
// kısa bir DDL satırındadır, bu yüzden kayıp yoktur ve RAM sınırlı kalır.
const filtreTampon = 1 << 20

var (
	ErrGecersizHedef = errors.New("sqlimport: geçersiz hedef veritabanı bilgisi")
	// identRE: MariaDB kimliği (panelin ürettiği DB adı/kullanıcı deseni).
	identRE = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
)

// Hedef: dump'ın uygulanacağı veritabanı ve o veritabanının KENDİ kimliği.
type Hedef struct {
	DBAdi     string
	Kullanici string
	Parola    string
	Host      string // boş → localhost (unix soketi)
}

func (h Hedef) dogrula() error {
	if !identRE.MatchString(h.DBAdi) || !identRE.MatchString(h.Kullanici) {
		return ErrGecersizHedef
	}
	if h.Parola == "" {
		return fmt.Errorf("%w: parola boş", ErrGecersizHedef)
	}
	return nil
}

// cnfKac: MariaDB option-file değeri için kaçış. Değer çift tırnak içinde
// verilir; içindeki ters-eğik-çizgi ve çift tırnak kaçırılmalıdır.
func cnfKac(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

// defaultsDosya: parolayı argv YERİNE 0600 geçici dosyayla verir.
func defaultsDosya(h Hedef) (string, func(), error) {
	host := h.Host
	if host == "" {
		host = "localhost"
	}
	f, err := os.CreateTemp("", "sanalcp-sqlimport-*.cnf")
	if err != nil {
		return "", func() {}, err
	}
	ad := f.Name()
	temizle := func() { _ = os.Remove(ad) }
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		temizle()
		return "", func() {}, err
	}
	body := fmt.Sprintf("[client]\nuser=\"%s\"\npassword=\"%s\"\nhost=\"%s\"\n",
		cnfKac(h.Kullanici), cnfKac(h.Parola), cnfKac(host))
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		temizle()
		return "", func() {}, err
	}
	if err := f.Close(); err != nil {
		temizle()
		return "", func() {}, err
	}
	return ad, temizle, nil
}

// Uygula: src'deki SQL'i hedef veritabanına, hedefin KENDİ düşük yetkili
// kullanıcısıyla uygular. Dump'ın içeriği güvenilmezdir; sınırı MariaDB'nin
// yetki denetimi koyar (bkz. paket açıklaması).
func Uygula(ctx context.Context, h Hedef, src io.Reader) error {
	if err := h.dogrula(); err != nil {
		return err
	}
	cnf, temizle, err := defaultsDosya(h)
	if err != nil {
		return fmt.Errorf("geçici kimlik dosyası: %w", err)
	}
	defer temizle()

	cmd := exec.CommandContext(ctx, "mysql",
		"--defaults-extra-file="+cnf,
		"--database="+h.DBAdi,
	)
	// Temiz env: panelin sırları (PANEL_SECRET_KEY vb.) alt sürece sızmasın.
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	kopyaHata := definerSuz(src, stdin)
	kapatHata := stdin.Close()
	bekleHata := cmd.Wait()
	if bekleHata != nil {
		return fmt.Errorf("mysql %s: %s", h.DBAdi, kisalt(stderr.String()))
	}
	if kopyaHata != nil {
		return fmt.Errorf("dump okunamadı: %w", kopyaHata)
	}
	return kapatHata
}

// definerSuz: src'yi dst'ye akıtırken DEFINER yan tümcelerini temizler.
// Satır tabanlı çalışır; tampondan uzun satırlar dokunulmadan geçer.
func definerSuz(src io.Reader, dst io.Writer) error {
	br := bufio.NewReaderSize(src, filtreTampon)
	bw := bufio.NewWriterSize(dst, filtreTampon)
	for {
		parca, err := br.ReadSlice('\n')
		if len(parca) > 0 {
			var yazilacak []byte
			// Hızlı yol: DEFINER geçmeyen satırlarda regexp hiç çalıştırılmaz.
			if bytes.Contains(parca, []byte("DEFINER")) || bytes.Contains(parca, []byte("definer")) {
				yazilacak = reDefiner.ReplaceAll(parca, nil)
			} else {
				yazilacak = parca
			}
			if _, werr := bw.Write(yazilacak); werr != nil {
				return werr
			}
		}
		if err == bufio.ErrBufferFull {
			continue // uzun satırın devamı; süzgeçsiz akmaya devam
		}
		if err == io.EOF {
			return bw.Flush()
		}
		if err != nil {
			_ = bw.Flush()
			return err
		}
	}
}

// TablolariSil: hedef şemadaki TÜM tablo ve view'leri düşürür ("içe aktarmadan
// önce boşalt" seçeneği). Yine hedefin kendi kullanıcısıyla çalışır, dolayısıyla
// başka bir şemaya sıçraması mümkün değildir.
func TablolariSil(ctx context.Context, h Hedef) error {
	if err := h.dogrula(); err != nil {
		return err
	}
	cnf, temizle, err := defaultsDosya(h)
	if err != nil {
		return fmt.Errorf("geçici kimlik dosyası: %w", err)
	}
	defer temizle()

	tablolar, viewler, err := nesneleriListele(ctx, cnf, h.DBAdi)
	if err != nil {
		return err
	}
	if len(tablolar) == 0 && len(viewler) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("SET FOREIGN_KEY_CHECKS=0;\n")
	for _, v := range viewler {
		fmt.Fprintf(&b, "DROP VIEW IF EXISTS `%s`;\n", v)
	}
	for _, t := range tablolar {
		fmt.Fprintf(&b, "DROP TABLE IF EXISTS `%s`;\n", t)
	}
	b.WriteString("SET FOREIGN_KEY_CHECKS=1;\n")

	cmd := exec.CommandContext(ctx, "mysql", "--defaults-extra-file="+cnf, "--database="+h.DBAdi)
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	cmd.Stdin = strings.NewReader(b.String())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tablolar silinemedi: %s", kisalt(stderr.String()))
	}
	return nil
}

// nesneleriListele: hedef şemadaki taban tablolar ve view'ler.
//
// Sorgu information_schema üzerinde ve şema adı DATABASE() ile geldiği için
// hedef adı SQL'e enterpolasyon EDİLMEZ; kullanıcının kendi yetkisi zaten
// yalnız bu şemayı görür.
func nesneleriListele(ctx context.Context, cnf, dbAdi string) (tablolar, viewler []string, err error) {
	const sorgu = `SELECT TABLE_TYPE, TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE()`
	cmd := exec.CommandContext(ctx, "mysql", "--defaults-extra-file="+cnf,
		"--database="+dbAdi, "-N", "-B", "-e", sorgu)
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("tablo listesi alınamadı: %s", kisalt(stderr.String()))
	}
	for _, satir := range strings.Split(string(out), "\n") {
		alanlar := strings.SplitN(strings.TrimRight(satir, "\r"), "\t", 2)
		if len(alanlar) != 2 || alanlar[1] == "" {
			continue
		}
		// Ters-tırnak içeren bir nesne adı DROP ifadesinden kaçabilirdi; MariaDB
		// böyle bir ada izin verse de panelin ürettiği şemalarda görülmez —
		// yine de atlayıp sessizce bırakmak, kaçış üretmekten güvenlidir.
		if strings.ContainsAny(alanlar[1], "`\n\r") {
			continue
		}
		if strings.EqualFold(alanlar[0], "VIEW") {
			viewler = append(viewler, alanlar[1])
		} else {
			tablolar = append(tablolar, alanlar[1])
		}
	}
	return tablolar, viewler, nil
}

func kisalt(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 600 {
		return s[:600] + "…"
	}
	return s
}
