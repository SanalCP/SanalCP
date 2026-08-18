package osfam

// Paket yöneticisi soyutlaması.
//
// TASARIM: Komutlar SAF fonksiyonlarla (arg-slice üreterek) kurulur, çalıştırma
// ayrı. Böylece "Debian'da doğru komut üretiliyor mu?" sorusu bir Debian makinesi
// olmadan birim testle yanıtlanır — port sırasında en çok ihtiyaç duyulan şey bu.
//
// apt çağrılarında DEBIAN_FRONTEND=noninteractive ZORUNLUDUR: aksi halde bazı
// paketler (postfix başta olmak üzere) kurulum sırasında etkileşimli soru sorar
// ve panel arkasında çalışan komut sonsuza kadar bekler.

import (
	"context"
	"os"
	"os/exec"
)

// AptOrtam: apt komutlarına eklenen ortam değişkenleri. Etkileşimli soruları
// kapatır ve Debian'ın "yeni yapılandırma dosyası ne yapılsın?" sorusunu
// mevcut dosyayı koruyacak şekilde otomatik yanıtlar (panelin yazdığı
// yapılandırmaların paket güncellemesinde ezilmemesi için).
var AptOrtam = []string{
	"DEBIAN_FRONTEND=noninteractive",
	"APT_LISTCHANGES_FRONTEND=none",
}

// KurArgs: paket kurma komutu.
func (b Bilgi) KurArgs(paketler ...string) []string {
	if b.DebianMi() {
		return append([]string{"apt-get", "install", "-y",
			"-o", "Dpkg::Options::=--force-confold", // mevcut conf dosyasını koru
			"-o", "Dpkg::Options::=--force-confdef"}, paketler...)
	}
	return append([]string{"dnf", "install", "-y"}, paketler...)
}

// SilArgs: paket kaldırma komutu.
func (b Bilgi) SilArgs(paketler ...string) []string {
	if b.DebianMi() {
		return append([]string{"apt-get", "remove", "-y"}, paketler...)
	}
	return append([]string{"dnf", "remove", "-y"}, paketler...)
}

// KuruluMuArgs: paketin KURULU olup olmadığını sınayan komut.
// Komut başarılıysa (exit 0) paket kuruludur.
func (b Bilgi) KuruluMuArgs(paket string) []string {
	if b.DebianMi() {
		// dpkg-query -W -f='${Status}' başarılı olsa da paket "deinstall"
		// durumunda olabilir; -s ile birlikte grep gerektirmeyen en güvenilir
		// sinyal budur: dpkg-query çıkışı 1 döner paket hiç yoksa.
		return []string{"dpkg-query", "-W", "-f=${db:Status-Status}", paket}
	}
	return []string{"dnf", "-q", "list", "--installed", paket}
}

// MevcutMuArgs: paketin DEPODA bulunup bulunmadığını sınayan komut.
func (b Bilgi) MevcutMuArgs(paket string) []string {
	if b.DebianMi() {
		return []string{"apt-cache", "show", paket}
	}
	return []string{"dnf", "-q", "list", "--available", paket}
}

// AraArgs: paket arama komutu (panel Paket Yöneticisi ekranı).
func (b Bilgi) AraArgs(sorgu string) []string {
	if b.DebianMi() {
		return []string{"apt-cache", "search", sorgu}
	}
	return []string{"dnf", "search", "--quiet", sorgu}
}

// BilgiArgs: paket ayrıntısı komutu.
func (b Bilgi) BilgiArgs(paket string) []string {
	if b.DebianMi() {
		return []string{"apt-cache", "show", paket}
	}
	return []string{"dnf", "info", "--quiet", paket}
}

// DepoTazeleArgs: paket dizinini tazeleme komutu. RHEL'de dnf kendi önbelleğini
// yönettiği için genelde gerekmez; Debian'da `apt-get update` OLMADAN kurulum
// "paket bulunamadı" ile başarısız olur.
func (b Bilgi) DepoTazeleArgs() []string {
	if b.DebianMi() {
		return []string{"apt-get", "update"}
	}
	return []string{"dnf", "-q", "makecache"}
}

// Komut: verilen arg-slice'tan çalıştırılabilir *exec.Cmd üretir; Debian ise
// apt'nin etkileşimsiz ortam değişkenlerini ekler.
//
// Arg-slice üreticileri (KurArgs vb.) ile çalıştırma bilinçli olarak ayrıdır:
// üreticiler saf ve test edilebilir kalır, yan etki tek noktada toplanır.
func (b Bilgi) Komut(ctx context.Context, args []string) *exec.Cmd {
	if len(args) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if b.DebianMi() {
		cmd.Env = append(os.Environ(), AptOrtam...)
	}
	return cmd
}

// PaketKur: paketleri kurar, komut çıktısını döner.
func PaketKur(ctx context.Context, paketler ...string) ([]byte, error) {
	b := Mevcut()
	return b.Komut(ctx, b.KurArgs(paketler...)).CombinedOutput()
}

// PaketSil: paketleri kaldırır.
func PaketSil(ctx context.Context, paketler ...string) ([]byte, error) {
	b := Mevcut()
	return b.Komut(ctx, b.SilArgs(paketler...)).CombinedOutput()
}

// PaketKurulu: paket kurulu mu?
//
// Debian tarafında dpkg-query paket hiç tanınmıyorsa sıfırdan farklı döner;
// tanınıyor ama kaldırılmışsa "config-files"/"not-installed" yazar. Yalnız
// "installed" kabul edilir — aksi halde kaldırılmış bir paket kurulu sanılırdı.
func PaketKurulu(ctx context.Context, paket string) bool {
	b := Mevcut()
	out, err := b.Komut(ctx, b.KuruluMuArgs(paket)).Output()
	if err != nil {
		return false
	}
	if b.DebianMi() {
		return string(out) == "installed"
	}
	return true
}

// PaketMevcut: paket depoda var mı?
func PaketMevcut(ctx context.Context, paket string) bool {
	b := Mevcut()
	return b.Komut(ctx, b.MevcutMuArgs(paket)).Run() == nil
}

// DepoTazele: paket dizinini tazeler (Debian'da kurulum öncesi gereklidir).
func DepoTazele(ctx context.Context) ([]byte, error) {
	b := Mevcut()
	return b.Komut(ctx, b.DepoTazeleArgs()).CombinedOutput()
}
