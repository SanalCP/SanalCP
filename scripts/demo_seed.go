//go:build ignore

// Demo panel tohumlama — SADECE demo VPS'inde, kurulumdan sonra elle çalıştırılır:
//
//	go run scripts/demo_seed.go -dsn '...' -taban-url https://127.0.0.1:8443 \
//	  -kullanici demo -parola '...'
//
// Akış: demo_modu_acik'ı GEÇİCİ olarak kapatır (yazma engeli aradan çekilsin
// diye), panelin KENDİ HTTP API'sinden birkaç örnek domain oluşturur (gerçek
// nginx/php-fpm/MySQL kaynağı doğar — sahte satır değil), sonra bayrağı 1'e
// geri çevirir.
//
// Bu script internal/domains paketini DOĞRUDAN import ETMEZ: panel API'sinin
// dışında ikinci bir oluşturma yolu açmak şema/iş kuralı driftine yol açar;
// HTTP API zaten "panelin yaptığı her şeyin" tek gerçek kaynağı (bkz.
// docs/API.md, README.md "Tam yönetim API'si").
package main

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/go-sql-driver/mysql"
)

var ornekDomainler = []string{"vitrin-magaza.demo", "blog-ornegi.demo", "kurumsal-site.demo"}

func main() {
	dsn := flag.String("dsn", "", "MySQL DSN (panel_ayarlari bayrağını geçici kapatmak/açmak için)")
	tabanURL := flag.String("taban-url", "https://127.0.0.1:8443", "panel API taban adresi")
	kullanici := flag.String("kullanici", "demo", "panel giriş kullanıcı adı")
	parola := flag.String("parola", "", "panel giriş parolası")
	flag.Parse()
	if *dsn == "" || *parola == "" {
		log.Fatalf("dsn ve parola zorunlu")
	}

	if err := run(*dsn, *tabanURL, *kullanici, *parola); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(dsn, tabanURL, kullanici, parola string) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("db aç: %v", err)
	}
	defer db.Close()

	if err := demoBayragiAyarla(db, 0); err != nil {
		return fmt.Errorf("bayrak kapatılamadı: %v", err)
	}
	defer func() {
		if err := demoBayragiAyarla(db, 1); err != nil {
			log.Printf("UYARI: bayrak geri açılamadı, elle kontrol et: UPDATE panel_ayarlari SET demo_modu_acik=1 WHERE id=1; (%v)", err)
		}
	}()

	// SIGINT/SIGTERM (örn. operatör çok-dakikalık domain oluşturma döngüsü
	// sırasında Ctrl-C basarsa) Go'nun defer zincirini ATLAR — process anında
	// sonlanır, yukarıdaki defer hiç çalışmaz ve demo panel kalıcı olarak
	// yazma-açık kalır. Bayrağı burada elle geri çeviriyoruz.
	sinyal := make(chan os.Signal, 1)
	signal.Notify(sinyal, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sinyal
		log.Printf("sinyal alındı (%v), demo bayrağı geri açılıyor...", s)
		if err := demoBayragiAyarla(db, 1); err != nil {
			log.Printf("UYARI: bayrak geri açılamadı, elle kontrol et: UPDATE panel_ayarlari SET demo_modu_acik=1 WHERE id=1; (%v)", err)
		}
		os.Exit(1)
	}()

	jar, _ := cookiejar.New(nil)
	istemci := &http.Client{Jar: jar, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // self-signed, loopback
	}}

	if err := girisYap(istemci, tabanURL, kullanici, parola); err != nil {
		return fmt.Errorf("giriş: %v", err)
	}

	basarili := 0
	for _, ad := range ornekDomainler {
		if err := domainOlustur(istemci, tabanURL, ad); err != nil {
			log.Printf("UYARI: %s oluşturulamadı: %v", ad, err)
			continue
		}
		fmt.Printf("oluşturuldu: %s\n", ad)
		basarili++
	}

	// Hiçbir domain oluşmadıysa "tamam" demek yanıltıcı: operatör boş bir
	// dump alıp sanalcp-demo-reset'e her gece o boş durumu geri
	// yükletebilir. Sessizce 0-satırlık bir "başarı" raporlamak yerine
	// açıkça hata veriyoruz.
	if basarili == 0 {
		fmt.Fprintln(os.Stderr, "HATA: hiçbir örnek domain oluşturulamadı, dump ALMA — önce sorunu çöz")
		return fmt.Errorf("tohumlama başarısız: 0/%d domain oluşturuldu", len(ornekDomainler))
	}

	fmt.Println("tohumlama tamam. Şimdi tek seferlik dump al:")
	fmt.Println("  mysqldump --single-transaction --databases panel | gzip -c > /var/backups/sanalcp/demo/demoseed.sql.gz")
	return nil
}

func demoBayragiAyarla(db *sql.DB, deger int) error {
	_, err := db.Exec(`UPDATE panel_ayarlari SET demo_modu_acik=? WHERE id=1`, deger)
	return err
}

func girisYap(c *http.Client, tabanURL, kullanici, parola string) error {
	gov, _ := json.Marshal(map[string]string{"kullanici": kullanici, "parola": parola})
	req, err := http.NewRequest(http.MethodPost, tabanURL+"/api/v1/auth/login", bytes.NewReader(gov))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", tabanURL) // CSRFKoruma origin kontrolü için gerekli
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("giriş %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func domainOlustur(c *http.Client, tabanURL, alanAdi string) error {
	gov, _ := json.Marshal(map[string]string{"alan_adi": alanAdi})
	req, err := http.NewRequest(http.MethodPost, tabanURL+"/api/v1/domains", bytes.NewReader(gov))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", tabanURL)
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
