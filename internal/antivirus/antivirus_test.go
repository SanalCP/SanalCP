package antivirus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPuanliMotorGucluWebshelliAciklar(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "wp-content", "uploads", "2026", "image.php")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`<?php eval(base64_decode($_POST['x']));`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, bulgular := runScan(context.Background(), root)
	if len(bulgular) != 1 {
		t.Fatalf("bulgu sayısı=%d, beklenen 1: %#v", len(bulgular), bulgular)
	}
	b := bulgular[0]
	if b.Puan != 95 || b.Risk != "kritik" {
		t.Fatalf("puan/risk=%d/%q, beklenen 95/kritik", b.Puan, b.Risk)
	}
	if len(b.Gerekceler) != 2 {
		t.Fatalf("gerekçeler=%#v, içerik+konum bekleniyordu", b.Gerekceler)
	}
}

func TestPuanliMotorTekZayifSinyaliRaporlamaz(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "app.php")
	uzun := "<?php $fixture='" + strings.Repeat("A", 600) + "';"
	if err := os.WriteFile(p, []byte(uzun), 0o644); err != nil {
		t.Fatal(err)
	}
	_, bulgular := runScan(context.Background(), root)
	if len(bulgular) != 0 {
		t.Fatalf("zayıf tek sinyal yanlış pozitif oldu: %#v", bulgular)
	}
}

func TestKonumTekBasinaBulguUretmez(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "wp-content", "uploads", "plugin.php")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`<?php function render_image() { return true; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, bulgular := runScan(context.Background(), root)
	if len(bulgular) != 0 {
		t.Fatalf("konum tek başına bulgu üretti: %#v", bulgular)
	}
}

func TestWordPressChecksumBulgulari(t *testing.T) {
	cikti := `Warning: File doesn't verify against checksum: wp-includes/version.php
Warning: File should not exist: wp-admin/css/.cache.php
Warning: File should not exist: robots.txt
Error: WordPress installation doesn't verify against checksums.`
	bulgular := wordpressChecksumBulgular("/home/c_site/public_html", cikti)
	if len(bulgular) != 2 {
		t.Fatalf("bulgu sayısı=%d, beklenen 2: %#v", len(bulgular), bulgular)
	}
	if bulgular[0].Puan != 100 || bulgular[0].Risk != "kritik" || bulgular[0].Motor != "wordpress-checksum" {
		t.Fatalf("değişmiş çekirdek bulgusu yanlış: %#v", bulgular[0])
	}
	if bulgular[1].Puan != 85 || bulgular[1].Risk != "yuksek" {
		t.Fatalf("beklenmeyen çekirdek dosyası yanlış: %#v", bulgular[1])
	}
	if strings.Contains(bulgular[0].Dosya, "robots.txt") || strings.Contains(bulgular[1].Dosya, "robots.txt") {
		t.Fatal("site kökündeki özel robots.txt yanlış pozitif oldu")
	}
}

func TestWordPressChecksumYolKacisiniReddeder(t *testing.T) {
	cikti := "Warning: File doesn't verify against checksum: ../../etc/passwd\n" +
		"Warning: File should not exist: /etc/shadow\n"
	if got := wordpressChecksumBulgular("/home/c_site/public_html", cikti); len(got) != 0 {
		t.Fatalf("kök dışı checksum yolları kabul edildi: %#v", got)
	}
}

func TestGuvenliDosyaOzetiSymlinkTakipEtmez(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "c_test")
	root := filepath.Join(home, "public_html")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	dis := filepath.Join(base, "secret.php")
	if err := os.WriteFile(dis, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "image.php")
	if err := os.Symlink(dis, link); err != nil {
		t.Fatal(err)
	}
	if _, err := guvenliDosyaOzeti(root, link); err == nil {
		t.Fatal("symlink içeriğinin özeti alındı")
	}
}

// TestInitSinir: Init ile ayarlanan üst sınırın acquire'a yansıdığını doğrular.
// 2 slotlu kuyruk → 2 eşzamanlı geçer, 3. hata verir; release sonrası yeniden geçer.
func TestInitSinir(t *testing.T) {
	Init(2)
	if got := MaxConcurrent(); got != 2 {
		t.Fatalf("MaxConcurrent = %d, beklenen 2", got)
	}
	if !acquire() || !acquire() {
		t.Fatal("ilk iki acquire başarısız — kuyruk mantığı bozuk")
	}
	if acquire() {
		t.Fatal("3. acquire geçti — sınır 2'yi aştı")
	}
	release()
	if !acquire() {
		t.Fatal("release sonrası acquire başarısız — slot iade edilmedi")
	}
	release()
	release()
}

// TestInitGecersizDegerler: <1 değerler 1'e çekilir, böylece operator yanlışlıkla
// 0 verip "sınırsız tarama" tuzağına düşmez.
func TestInitGecersizDegerler(t *testing.T) {
	for _, v := range []int{0, -1, -100} {
		Init(v)
		if got := MaxConcurrent(); got != 1 {
			t.Errorf("Init(%d) → MaxConcurrent = %d, beklenen 1", v, got)
		}
	}
	Init(1) // sonraki testler için
}

// TestAcquireReleaseParalel: N goroutine paralel acquire eder; en fazla N'i
// aynı anda tutulur (race detector bunu görür). Init(3) + 50 goroutine +
// her biri 1 acquire 1 release → hiçbir an 3'ten fazla "tutulan" olmamalı.
func TestAcquireReleaseParalel(t *testing.T) {
	Init(3)
	const G = 50
	var tutulan int32
	var maksTutulan int32
	var wg sync.WaitGroup
	wg.Add(G)
	for i := 0; i < G; i++ {
		go func() {
			defer wg.Done()
			if !acquire() {
				t.Error("acquire başarısız — 50 talep için 3 slot yetmeli (her biri hemen release)")
				return
			}
			şimdi := atomic.AddInt32(&tutulan, 1)
			defer atomic.AddInt32(&tutulan, -1)
			defer release()
			// anlık eşzamanlılık sayacını güncelle
			for {
				m := atomic.LoadInt32(&maksTutulan)
				if şimdi <= m || atomic.CompareAndSwapInt32(&maksTutulan, m, şimdi) {
					break
				}
			}
		}()
	}
	wg.Wait()
	if maksTutulan > 3 {
		t.Errorf("maks eşzamanlı = %d, sınır 3 aşıldı", maksTutulan)
	}
	if maksTutulan < 2 {
		t.Logf("uyarı: maks eşzamanlı = %d, paralellik gerçekleşmedi (zamanlayıcı sıralı çalıştırmış olabilir)", maksTutulan)
	}
	Init(1) // sonraki testler için
}

// TestAcquireInitCagirilmadan: Init çağrılmamışsa acquire non-op (sınırsız).
// Bu kasıtlı: test harness'i Init'i atlamamalı diye zorlamak yerine, unutan
// testler sessizce "sınırsız" çalışır — gerçek OOM yalnız üretimde olur.
// Burada yalnızca release'in nil-kanalda patlamadığını doğruluyoruz.
func TestAcquireInitCagirilmadan(t *testing.T) {
	sem = nil // Init etkisini geri al
	if !acquire() {
		t.Fatal("Init yok → acquire false döndü, sınırsız olmalıydı")
	}
	release()                    // nil kanaldan <- yapmamalı — sem==nil koruması var
	sem = make(chan struct{}, 1) // sonraki testler için geri kur
}
