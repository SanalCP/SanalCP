package kaynaklimit

// ext2/3/4 kota backend — quota-tools (setquota(8) / repquota(8) / quotaon(8)).
// Debian ve Ubuntu bulut imajlarında kök dosya sistemi neredeyse her zaman ext4'tür.
//
// 🔴 Kök fs ext4 kotası da MOUNT anında açılır → GRUB `rootflags=usrquota` +
// tek seferlik reboot. (XFS'te bayrak `uquota`, burada `usrquota` — FARKLI.)
//
// ÇIKTI BİÇİMLERİ quota-tools 4.09 (Debian 13 trixie) üzerinde CANLI doğrulandı;
// her fonksiyonun başında hangi davranışın gözlendiği yazılıdır. Ayrıştırıcılar
// SAF fonksiyonlardır → birim testli (bkz. kota_ext4_test.go).

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type ext4Kota struct{}

func (ext4Kota) Ad() string            { return "ext4" }
func (ext4Kota) KernelBayragi() string { return "rootflags=usrquota" }

// quotaAracUyari: "quota paketi kurulu değil" uyarısını her çağrıda değil, bir kez logla.
var quotaAracUyari sync.Once

// ext4QuotaonAktif: `quotaon -p -u /` STDOUT'unu ayrıştırır (enforcement açık mı).
//
// DOĞRULANDI (quota-tools 4.09): kota AÇIKKEN stdout'a `%s quota on %s (%s) is %s`
// biçiminde tek satır yazılır → "user quota on / (/dev/vda1) is on".
// Kota KAPALIYKEN bu satır HİÇ yazılmaz; mesaj STDERR'e gider ("Mountpoint (or device)
// / not found or has no quota enabled.") ve komut yine **rc=0** döner.
// → Çıkış koduna GÜVENİLEMEZ; satırın varlığı + " is on" son eki esastır.
func ext4QuotaonAktif(stdout string) bool {
	for _, ln := range strings.Split(stdout, "\n") {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(t, "user quota on ") {
			continue
		}
		return strings.HasSuffix(t, " is on")
	}
	return false
}

// ext4MountKotali: /proc/self/mounts içeriğinde kök mount kota seçeneğiyle mi bağlı.
//
// quotaon KAPALI ama bu AÇIKSA: kota dosyaları var, kullanım sayılıyor, ama limitler
// enforce EDİLMİYOR — XFS'teki `uqnoenforce` durumunun ext4 karşılığı. Seçenek
// değer taşıyabildiği için ("usrjquota=aquota.user") ad kısmı ayrılarak bakılır.
func ext4MountKotali(mounts string) bool {
	for _, ln := range strings.Split(mounts, "\n") {
		f := strings.Fields(ln)
		if len(f) < 4 || f[1] != kotaMount {
			continue
		}
		for _, o := range f[3:4] {
			for _, sec := range strings.Split(o, ",") {
				if i := strings.IndexByte(sec, '='); i >= 0 {
					sec = sec[:i]
				}
				switch sec {
				case "usrquota", "quota", "usrjquota":
					return true
				}
			}
		}
	}
	return false
}

// quotaonVar: `quotaon` ikilisi PATH'te mi. Değişken olmasının sebebi test
// edilebilirlik: Aktif() bu kontrolü doğrudan exec.LookPath ile yapsaydı,
// quotaonSorgula'yı sahteleyen testler makinede `quota` paketi kurulu olup
// olmamasına göre farklı sonuç verirdi (CI'da kurulu değildir, geliştirme
// sunucusunda kuruludur).
var quotaonVar = func() bool {
	_, err := exec.LookPath("quotaon")
	return err == nil
}

// Aktif: enforcement quotaon'dan, accounting quotaon+mount seçeneğinden okunur.
func (ext4Kota) Aktif() (accounting, enforcement bool) {
	if !quotaonVar() {
		quotaAracUyari.Do(func() {
			log.Printf("kota[ext4]: quotaon bulunamadı — `quota` paketi kurulu değil, disk kotası kullanılamıyor")
		})
		return false, false
	}
	cikti, _ := quotaonSorgula()
	return ext4AktifCoz(cikti, mountKotaliOku())
}

// quotaonSorgula: `quotaon -p -u /` STDOUT'u. Hata bilerek yok sayılır.
//
// 🔴 ÇIKIŞ KODU HER İKİ YÖNDE DE GÜVENİLMEZ — canlı olarak iki farklı sistemde
// doğrulandı:
//   - kota KAPALI  : tanı mesajı stderr'e gider, rc=0 (yani "başarı" gibi görünür)
//   - kota AÇIK    : stdout "user quota on / (/dev/sda1) is on", rc=1 (Debian 12,
//     quota-tools 4.06) — hata sanılıp erken dönülürse kota AÇIKKEN kapalı
//     raporlanır. Faz 5a'da tam olarak bu oldu: panel "reboot gerekli" dedi,
//     oysa kota çalışıyordu.
//
// Tek güvenilir sinyal STDOUT'tur.
var quotaonSorgula = func() (string, error) {
	if _, err := exec.LookPath("quotaon"); err != nil {
		quotaAracUyari.Do(func() {
			log.Printf("kota[ext4]: quotaon bulunamadı — `quota` paketi kurulu değil, disk kotası kullanılamıyor")
		})
		return "", err
	}
	var stdout bytes.Buffer
	cmd := exec.Command("quotaon", "-p", "-u", kotaMount)
	cmd.Stdout = &stdout // stderr BİLEREK yutulur: kapalıyken tanı mesajı oraya gider
	err := cmd.Run()
	return stdout.String(), err
}

func mountKotaliOku() bool {
	mounts, _ := os.ReadFile("/proc/self/mounts")
	return ext4MountKotali(string(mounts))
}

// ext4AktifCoz: saf karar — quotaon çıktısı + mount seçeneği.
// quotaon "on" diyorsa hem muhasebe hem uygulama açıktır; demiyorsa ama mount
// kotalıysa muhasebe var, uygulama yok (XFS'teki uqnoenforce'un karşılığı).
func ext4AktifCoz(quotaonCikti string, mountKotali bool) (accounting, enforcement bool) {
	if ext4QuotaonAktif(quotaonCikti) {
		return true, true
	}
	return mountKotali, false
}

// ext4LimitArgs: setquota'ya verilecek arg-slice (saf → birim-test edilebilir).
//
// Biçim: `setquota -u <ad> <bsoft> <bhard> <isoft> <ihard> <fs>`.
// Blok limitleri VARSAYILAN OLARAK KiB bloklarıdır; "M" son eki mebibayt anlamına
// gelir (man setquota: K/M/G/T kabul edilir) → panelin MB birimiyle birebir eşleşsin
// diye "M" ile verilir. inode limitleri harfi harfine sayıdır (k/m/g/t son ekleri
// 10^3 katları demek olduğu için ASLA kullanılmaz).
// 0 = o metrik SINIRSIZ — XFS ile aynı semantik (man: "To disable a quota, set the
// corresponding parameter to 0").
// sk çağırandan önce adlar.SKGecerli'den geçmiş olmalı; "c_" ön eki setquota'nın
// "ad salt rakamsa UID say" davranışını da devre dışı bırakır.
func ext4LimitArgs(sk string, diskMB, inode int) []string {
	diskSoft, diskHard := kotaSoft(diskMB)
	inodeSoft, inodeHard := kotaSoft(inode)
	return []string{
		"-u", sk,
		strconv.Itoa(diskSoft) + "M", strconv.Itoa(diskHard) + "M",
		strconv.Itoa(inodeSoft), strconv.Itoa(inodeHard),
		kotaMount,
	}
}

func (ext4Kota) Uygula(ctx context.Context, sk string, diskMB, inode int) error {
	out, err := exec.CommandContext(ctx, "setquota", ext4LimitArgs(sk, diskMB, inode)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("setquota %s: %s: %w", sk, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ext4CSVSatir: `repquota -u -O csv /` çıktısından sk satırının kullanım + hard limit
// değerlerini çıkarır. Blok değerleri KiB, inode değerleri adettir.
//
// DOĞRULANDI (quota-tools 4.09) — başlık satırı:
//
//	User,BlockStatus,FileStatus,BlockUsed,BlockSoftLimit,BlockHardLimit,BlockGrace,
//	FileUsed,FileSoftLimit,FileHardLimit,FileGrace
//
// SÜTUNLAR SABİT İNDEKSLE DEĞİL, BAŞLIK ADIYLA bulunur: repquota sürümleri arasında
// sütun sırası değişebilir ve blok sütunları sürüme/bayrağa göre "Block*" yerine
// "Space*" adlanabilir (ikisi de ikilinin içinde mevcut). Değerler ham sayı kalsın
// diye `-s`/human-readable ASLA verilmez (aksi halde "5120M" gibi okunur).
func ext4CSVSatir(cikti, sk string) (blokKullanilanKB, blokHardKB, inodeKullanilan, inodeHard int, ok bool) {
	iAd, iBlokUsed, iBlokHard, iDosyaUsed, iDosyaHard := -1, -1, -1, -1, -1
	var alanSayisi int

	for _, ln := range strings.Split(cikti, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		f := strings.Split(ln, ",")

		if iAd < 0 {
			// Henüz başlık görülmedi: bu satır başlık mı?
			for i, h := range f {
				switch strings.TrimSpace(h) {
				case "User":
					iAd = i
				case "BlockUsed", "SpaceUsed":
					iBlokUsed = i
				case "BlockHardLimit", "SpaceHardLimit":
					iBlokHard = i
				case "FileUsed":
					iDosyaUsed = i
				case "FileHardLimit":
					iDosyaHard = i
				}
			}
			if iAd < 0 || iBlokUsed < 0 || iBlokHard < 0 || iDosyaUsed < 0 || iDosyaHard < 0 {
				// Başlık değil (veya tanınmayan biçim) → indeksleri sıfırla, devam et.
				iAd, iBlokUsed, iBlokHard, iDosyaUsed, iDosyaHard = -1, -1, -1, -1, -1
				continue
			}
			alanSayisi = len(f)
			continue
		}

		if len(f) != alanSayisi || strings.TrimSpace(f[iAd]) != sk {
			continue
		}
		blokKullanilanKB, _ = strconv.Atoi(strings.TrimSpace(f[iBlokUsed]))
		blokHardKB, _ = strconv.Atoi(strings.TrimSpace(f[iBlokHard]))
		inodeKullanilan, _ = strconv.Atoi(strings.TrimSpace(f[iDosyaUsed]))
		inodeHard, _ = strconv.Atoi(strings.TrimSpace(f[iDosyaHard]))
		return blokKullanilanKB, blokHardKB, inodeKullanilan, inodeHard, true
	}
	return 0, 0, 0, 0, false
}

func (ext4Kota) Durum(sk string) (kullanilanMB, limitMB, kullanilanInode, limitInode int) {
	// DOĞRULANDI: kota kapalıyken repquota **rc=1** döner (quotaon'un aksine) → hata
	// yolu güvenilir. Output() kullanılır ki stderr'deki tanı mesajı CSV'ye karışmasın.
	out, err := exec.Command("repquota", "-u", "-O", "csv", kotaMount).Output()
	if err != nil {
		return 0, 0, 0, 0
	}
	bUsedKB, bHardKB, iUsed, iHard, ok := ext4CSVSatir(string(out), sk)
	if !ok {
		return 0, 0, 0, 0 // tenant henüz kota tablosunda yok
	}
	return bUsedKB / 1024, bHardKB / 1024, iUsed, iHard
}
