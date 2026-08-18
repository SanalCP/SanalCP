// Package osfam: işletim sistemi ailesi tespiti ve aileye göre değişen
// isim/komut çözümlemesi.
//
// NEDEN VAR: Panel AlmaLinux (RHEL ailesi) için yazıldı; Debian/Ubuntu desteği
// eklenirken paket yöneticisi, paket adları, servis adları ve bazı dosya yolları
// değişiyor. Bu farkların kod içine dağılması bakımı imkânsız hale getirir —
// hepsi buradan geçer.
//
// TASARIM İLKESİ: Bu paket YALNIZCA "işletim sistemi hakkında gerçekler" bilir.
// Uygulama bilgisi (PHP havuz yolları, nginx şablonları) burada DEĞİLDİR; onu
// bilen paketler `osfam.Mevcut().Aile` değerine bakıp kendi tablolarını seçer.
// Aksi halde bu paket her şeyi import eden bir tanrı-paket olurdu.
//
// ÇÖZÜMLEME YALNIZ AİLEYE BAKMAZ: bazı paketler dağıtım SÜRÜMÜNE bağlıdır.
// Somut örnek — valkey-server Debian 13'te ve tüm hedef Ubuntu'larda var, ama
// Debian 12'de YOK (redis-server'a düşülür). Bu yüzden Bilgi; Aile'nin yanında
// ID, Surum ve KodAdi da taşır ve çözümleyiciler üçünü de görebilir.
package osfam

import (
	"bufio"
	"os"
	"strings"
	"sync"
)

// Aile: paket yöneticisi ve sistem sözleşmelerini belirleyen üst kategori.
type Aile string

const (
	RHEL     Aile = "rhel"   // AlmaLinux, Rocky, RHEL, CentOS
	Debian   Aile = "debian" // Debian, Ubuntu
	Bilinmez Aile = "bilinmez"
)

// Bilgi: /etc/os-release'ten çözülen işletim sistemi kimliği.
type Bilgi struct {
	Aile   Aile
	ID     string // os-release ID: "almalinux" | "debian" | "ubuntu"
	Surum  string // os-release VERSION_ID: "10" | "13" | "26.04"
	KodAdi string // os-release VERSION_CODENAME: "trixie" | "resolute" (RHEL'de boş)
}

// DebianMi / RHELMi: çağrı yerlerinde okunabilirlik için.
func (b Bilgi) DebianMi() bool { return b.Aile == Debian }
func (b Bilgi) RHELMi() bool   { return b.Aile == RHEL }

var (
	mevcutBir sync.Once
	mevcut    Bilgi
)

// Mevcut: çalışan sistemin bilgisi (bir kez okunur, sonra önbellekten).
func Mevcut() Bilgi {
	mevcutBir.Do(func() {
		mevcut = oku("/etc/os-release")
	})
	return mevcut
}

// Ayarla: YALNIZCA TESTLER İÇİN — tespit edilen bilgiyi elle geçersiz kılar.
// Üretim yolunda çağrılmaz.
func Ayarla(b Bilgi) {
	mevcutBir.Do(func() {}) // sync.Once'ı tüket ki sonraki Mevcut() dosyayı okumasın
	mevcut = b
}

func oku(yol string) Bilgi {
	ham, err := os.ReadFile(yol)
	if err != nil {
		return Bilgi{Aile: Bilinmez}
	}
	return Ayristir(string(ham))
}

// Ayristir: os-release içeriğinden Bilgi üretir.
//
// SAF FONKSİYON — dosya sistemi gerektirmez, bu yüzden gerçek dağıtımların
// os-release içerikleriyle birim test edilebilir. Tespit mantığındaki bir hata
// yanlış paket yöneticisi çağırmak demektir; testsiz bırakılamaz.
func Ayristir(icerik string) Bilgi {
	alan := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(icerik))
	for sc.Scan() {
		satir := strings.TrimSpace(sc.Text())
		if satir == "" || strings.HasPrefix(satir, "#") {
			continue
		}
		esit := strings.IndexByte(satir, '=')
		if esit <= 0 {
			continue
		}
		anahtar := strings.TrimSpace(satir[:esit])
		deger := strings.TrimSpace(satir[esit+1:])
		// os-release değerleri tırnaklı olabilir: ID="ubuntu"
		deger = strings.Trim(deger, `"'`)
		alan[anahtar] = deger
	}

	b := Bilgi{
		ID:     strings.ToLower(alan["ID"]),
		Surum:  alan["VERSION_ID"],
		KodAdi: strings.ToLower(alan["VERSION_CODENAME"]),
		Aile:   Bilinmez,
	}

	// Önce doğrudan ID, sonra ID_LIKE. ID_LIKE boşlukla ayrılmış birden çok
	// değer taşıyabilir ("ID_LIKE=ubuntu debian").
	adaylar := append([]string{b.ID}, strings.Fields(strings.ToLower(alan["ID_LIKE"]))...)
	for _, a := range adaylar {
		switch a {
		case "debian", "ubuntu":
			b.Aile = Debian
			return b
		case "rhel", "fedora", "centos", "almalinux", "rocky":
			b.Aile = RHEL
			return b
		}
	}
	return b
}
