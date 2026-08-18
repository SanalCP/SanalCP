package kaynaklimit

// ── Disk kotası: dosya sisteminden bağımsız ortak katman ──────────────────────
//
// Tenant home'u (/home/c_<sk>) AYRI mount OLMASA da user quota, tenant kullanıcısına
// (c_<sk>) kök mount üzerinden uygulanır. Dosyalar zaten c_<sk>:c_<sk> sahipli → user
// quota tam eşleşir + tenant chown yapamadığı için kaçış-korumalı. Eski PROJECT-quota
// yaklaşımı (/home ayrı mount + pquota) bu altyapıda çalışmaz → user-quota ile değiştirildi.
//
// İKİ BACKEND. Kök dosya sistemi:
//   - XFS      → xfs_quota(8)                    (AlmaLinux/RHEL varsayılanı)
//   - ext2/3/4 → setquota(8) + repquota(8)       (Debian/Ubuntu bulut imajı varsayılanı)
//   - diğeri   → backend YOK; kota bu sunucuda desteklenmiyor denir (btrfs, zfs, overlay).
//     Panelin geri kalanı çalışmaya devam eder.
//
// 🔴 HER İKİ AİLEDE DE kök fs kotası ancak MOUNT anında açılır; canlı remount ile
// açılamaz → GRUB kernel bayrağı (backend'e göre `rootflags=uquota` veya
// `rootflags=usrquota`) + tek seferlik reboot ŞART (installer/update script yazar).
// Kota fs'te AKTİF DEĞİLKEN TÜM kota işlemleri SESSİZCE atlanır — asla hard-fail
// (aksi halde tenant create patlardı).

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// kotaMount: user quota'nın uygulandığı mount noktası. /home ayrı mount DEĞİL → kök.
const kotaMount = "/"

// Plan atanmamış tenant için makul üst sınır (CloudLinux paritesi — sınırsız bırakma).
const (
	varsayilanDiskMB = 5120   // 5 GB
	varsayilanInode  = 500000 // 500k dosya+dizin
)

// reKotaSK: sistem kullanıcı allowlist'i. provisioner.SlugFromDomain "c_" + [a-z0-9_] üretir.
// Kota komutlarının arg-slice'ına YALNIZ buradan geçen sk gider → shell/arg injection kapalı.
// Ayrıca "c_" ön eki, setquota'nın "salt rakamsa UID say" davranışını da devre dışı bırakır.
var reKotaSK = regexp.MustCompile(`^c_[a-z0-9_]{1,60}$`)

// kotaBackend: dosya sistemine özgü kota işlemleri.
//
// Paket-İÇİ arayüz: dışarıdan uygulayan yok, export edilmez. Amaç XFS ve ext4
// yollarını tek bir çağrı yüzeyinde toplamak — üstteki ortak katman (sentinel,
// allowlist, plan çözümlemesi, heal döngüsü) hangi fs'te olduğunu bilmez.
type kotaBackend interface {
	// Ad: log/teşhis için backend adı ("xfs" | "ext4").
	Ad() string
	// KernelBayragi: kotayı kalıcı açan GRUB kernel parametresi (sentinel metninde geçer).
	KernelBayragi() string
	// Aktif: kök fs'te user quota accounting/enforcement açık mı.
	// accounting=true + enforcement=false → kullanım sayılıyor ama limitler UYGULANMIYOR.
	Aktif() (accounting, enforcement bool)
	// Uygula: tenant'a disk(MB) + inode limitini yazar. 0 = o metrik SINIRSIZ.
	// Çağrıldığında sk allowlist'ten geçmiş ve enforcement açık olduğu doğrulanmıştır.
	Uygula(ctx context.Context, sk string, diskMB, inode int) error
	// Durum: anlık kullanım + limit. Okunamayan değerler 0 döner (hata dönmez —
	// UI'da kota bilgisi eksik göstermek, ekranı patlatmaktan iyidir).
	Durum(sk string) (kullanilanMB, limitMB, kullanilanInode, limitInode int)
}

var (
	backendBir sync.Once
	backend    kotaBackend
)

// kotaBackendSec: kök fs tipine göre backend seçer — bir kez, statfs(2) ile.
//
// statfs BAŞARISIZSA temkinli davranılır ve XFS varsayılır: "desteklenmiyor" gibi
// kalıcı/net bir hüküm vermek yerine mevcut reboot-gerekli akışına düşülür (çağıran
// taraf zaten enforcement kapalı görüp sessizce atlar).
func kotaBackendSec() kotaBackend {
	backendBir.Do(func() {
		var st unix.Statfs_t
		if err := unix.Statfs(kotaMount, &st); err != nil {
			log.Printf("kota: statfs(%s) başarısız (%v) — XFS varsayılıyor", kotaMount, err)
			backend = xfsKota{}
			return
		}
		switch int64(st.Type) {
		case unix.XFS_SUPER_MAGIC:
			backend = xfsKota{}
		case unix.EXT4_SUPER_MAGIC: // ext2/ext3/ext4 aynı magic'i paylaşır (0xef53)
			backend = ext4Kota{}
		default:
			backend = nil // btrfs/zfs/overlay/... → kota desteklenmiyor
		}
	})
	return backend
}

// kotaBackendZorla: YALNIZCA TESTLER İÇİN — seçilen backend'i elle geçersiz kılar.
func kotaBackendZorla(b kotaBackend) {
	backendBir.Do(func() {}) // sync.Once'ı tüket ki sonraki çağrı statfs yapmasın
	backend = b
}

// KotaFSUyumlu: kök dosya sisteminde disk kotası MÜMKÜN mü (XFS veya extN).
// false ise kota bu sunucuda KALICI olarak desteklenmiyor — reboot bunu ÇÖZMEZ;
// tek çözüm sunucunun desteklenen bir kök dosya sistemiyle yeniden kurulmasıdır.
func KotaFSUyumlu() bool { return kotaBackendSec() != nil }

// mountKotaAktif: kök fs'te user quota accounting/enforcement açık mı (backend'e sorar).
// Backend yoksa (desteklenmeyen fs) her ikisi de false.
func mountKotaAktif() (accounting, enforcement bool) {
	b := kotaBackendSec()
	if b == nil {
		return false, false
	}
	return b.Aktif()
}

// ── Kota görünürlük sentinel'i ─────────────────────────────────────────────────
// fs'te user-quota enforcement AKTİF DEĞİLKEN (kota kapalı → tek seferlik reboot
// bekliyor; veya accounting açık/enforce kapalı) TÜM kota işlemleri sessizce no-op olur.
// Operatör "kota aktif" sanmasın diye HealKotaOnStartup açılışta bu sentinel'i YAZAR;
// enforcement aktifken SİLER. Status endpoint bunu okuyup UI'a reboot-gerekli bayrağı düşürür.
const kotaSentinelDir = "/etc/sanalcp"
const kotaRebootSentinel = kotaSentinelDir + "/reboot-required-quota"

// kotaSentinelYaz: reboot-gerekli sentinel'ini idempotent yazar. Sabit yol; os.WriteFile =
// O_WRONLY|O_CREATE|O_TRUNC, 0644, root. İçerik = açıklama + RFC3339 zaman damgası.
// Kernel bayrağı backend'den alınır (XFS ve ext4'te FARKLI).
func kotaSentinelYaz() {
	if err := os.MkdirAll(kotaSentinelDir, 0755); err != nil {
		log.Printf("kota sentinel: dizin oluşturulamadı (%s): %v", kotaSentinelDir, err)
		return
	}
	bayrak := "rootflags=uquota"
	if b := kotaBackendSec(); b != nil {
		bayrak = b.KernelBayragi()
	}
	body := "disk kotası aktif değil — " + bayrak + " + reboot gerekli\n" +
		time.Now().Format(time.RFC3339) + "\n"
	if err := os.WriteFile(kotaRebootSentinel, []byte(body), 0644); err != nil {
		log.Printf("kota sentinel yazılamadı (%s): %v", kotaRebootSentinel, err)
	}
}

// kotaSentinelSil: enforcement aktifken (reboot sonrası) bayat reboot uyarısını kaldırır.
// Dosya yoksa no-op (idempotent).
func kotaSentinelSil() {
	if err := os.Remove(kotaRebootSentinel); err != nil && !os.IsNotExist(err) {
		log.Printf("kota sentinel silinemedi (%s): %v", kotaRebootSentinel, err)
	}
}

// KotaRebootGerekli: disk kotası enforcement AKTİF DEĞİL mi (tek seferlik reboot bekliyor).
// Sentinel dosyası VARSA (HealKotaOnStartup açılışta yazmış) VEYA canlı enforcement
// KAPALIYSA true. os.Stat önce denenir → sentinel varken exec'siz. Status endpoint UI bayrağı
// (kota_reboot_gerekli) buradan beslenir.
func KotaRebootGerekli() bool {
	if _, err := os.Stat(kotaRebootSentinel); err == nil {
		return true
	}
	_, enf := mountKotaAktif()
	return !enf
}

// kotaSoft: soft limit = hard * 0.95. Negatif girdi 0'a sıkıştırılır (0 = sınırsız).
// İki backend de aynı %95 kuralını kullanır → tek yerde.
func kotaSoft(hard int) (soft, duzeltilmisHard int) {
	if hard < 0 {
		hard = 0
	}
	return hard * 95 / 100, hard
}

// KotaUygula: tenant (c_<sk>) için user disk+inode kotasını uygular.
// fs'te kota AKTİF DEĞİLSE (reboot bekliyor / desteklenmiyor) → log + return nil (ASLA hata).
// diskMB/inode 0 = o limiti sınırsız bırak. Komut arg-slice ile çağrılır (shell yok);
// sk allowlist'ten (reKotaSK) geçer → injection yok.
func KotaUygula(ctx context.Context, sk string, diskMB, inode int) error {
	if !reKotaSK.MatchString(sk) {
		return fmt.Errorf("kota: geçersiz sistem kullanıcı biçimi: %q", sk)
	}
	b := kotaBackendSec()
	if b == nil {
		return nil // desteklenmeyen fs → sessiz atla (KotaFSUyumlu UI'da açıklıyor)
	}
	if acc, enf := b.Aktif(); !enf {
		// enforcement kapalı → limit YAZMA (enforce edilmeyecek).
		if acc {
			log.Printf("kota[%s]: accounting açık ama enforcement KAPALI — limitler enforce EDİLMİYOR, %s atlandı", b.Ad(), sk)
		} else {
			log.Printf("kota[%s]: fs'te aktif değil — tek seferlik reboot gerekli (%s), %s atlandı", b.Ad(), b.KernelBayragi(), sk)
		}
		return nil
	}
	home := "/home/" + sk
	if _, err := os.Stat(home); os.IsNotExist(err) {
		return nil // henüz provision edilmemiş → sessiz atla
	}
	// Kullanıcı gerçekten var mı + uid>0 (root'a/sisteme ASLA kota koyma).
	uidOut, err := exec.Command("id", "-u", sk).Output()
	if err != nil {
		return nil
	}
	if uid := strings.TrimSpace(string(uidOut)); uid == "" || uid == "0" {
		return fmt.Errorf("kota: %s geçersiz uid (%q)", sk, uid)
	}
	if err := b.Uygula(ctx, sk, diskMB, inode); err != nil {
		return err
	}
	log.Printf("kota[%s] uygulandı: %s disk=%dMB inode=%d", b.Ad(), sk, diskMB, inode)
	return nil
}

// efektifKota: domain override (>0) > plan değeri > (plan yoksa) varsayılan.
// Plan ATANMIŞSA plan değeri kullanılır (0 = plan tarafından açıkça sınırsız); plan YOKSA
// varsayılan makul sınır uygulanır (CloudLinux paritesi). Domain override her ikisini de ezer.
func efektifKota(diskOverride, inodeOverride int, planVar bool, planDisk, planInode int) (int, int) {
	disk, inode := varsayilanDiskMB, varsayilanInode
	if planVar {
		disk, inode = planDisk, planInode
	}
	if diskOverride > 0 {
		disk = diskOverride
	}
	if inodeOverride > 0 {
		inode = inodeOverride
	}
	return disk, inode
}

// DomainKotaUygula: domain için efektif kotayı (override>plan>varsayılan) çözer ve KotaUygula
// çağırır. Create + plan-değişim hook'ları (UygulaHepsi/LimitleriReAssert) ve HealKotaOnStartup
// buradan geçer → tek çözümleme kaynağı.
func DomainKotaUygula(ctx context.Context, db *sql.DB, domainID int64) error {
	var sk string
	var dDisk, dInode int
	var planID sql.NullInt64
	var pDisk, pInode int
	err := db.QueryRowContext(ctx, `
		SELECT d.sistem_kullanici,
		       COALESCE(d.disk_kota_mb,0), COALESCE(d.inode_kota,0),
		       d.plan_id,
		       COALESCE(p.disk_kota_mb,0), COALESCE(p.inode_kota,0)
		FROM domains d LEFT JOIN service_plans p ON p.id=d.plan_id
		WHERE d.id=?`, domainID).
		Scan(&sk, &dDisk, &dInode, &planID, &pDisk, &pInode)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(sk, "c_") {
		return nil // admin/geçersiz sistem kullanıcı → dokunma
	}
	disk, inode := efektifKota(dDisk, dInode, planID.Valid, pDisk, pInode)
	return KotaUygula(ctx, sk, disk, inode)
}

// KotaDurum: tenant'ın anlık disk(MB)/inode kullanım + limitlerini backend'den okur (UI için).
// Kota aktif değilse veya sk geçersizse hepsi 0 döner.
func KotaDurum(sk string) (kullanilanMB, limitMB, kullanilanInode, limitInode int) {
	if !reKotaSK.MatchString(sk) {
		return 0, 0, 0, 0
	}
	b := kotaBackendSec()
	if b == nil {
		return 0, 0, 0, 0
	}
	if acc, enf := b.Aktif(); !enf {
		// enforcement kapalı → limitler enforce edilmiyor; kullanım/limit gösterme (0 dön).
		if acc {
			log.Printf("kota durum[%s]: accounting açık ama enforcement KAPALI — limitler enforce EDİLMİYOR", b.Ad())
		}
		return 0, 0, 0, 0
	}
	return b.Durum(sk)
}

// HealKotaOnStartup: açılışta TÜM tenant'lar (c_<sk>) için efektif user kotasını
// (override>plan>varsayılan) idempotent RE-ASSERT eder. fs'te kota AKTİF DEĞİLSE
// (tek seferlik reboot bekliyor / fs desteklemiyor) HİÇBİR ŞEY uygulanmaz; hepsi
// "atlandı" sayılır (ASLA hata). Panel boot'unu bloklamaz (bg goroutine olarak çağrılır).
// Kod/plan drift'i her restart'ta mevcut tenant'lara iner.
func HealKotaOnStartup(ctx context.Context, db *sql.DB) {
	if db == nil {
		return
	}
	// fs kota enforcement kapalıysa: reboot-gerekli sentinel'ini YAZ (UI görünürlüğü) + tek log + çık.
	if acc, enf := mountKotaAktif(); !enf {
		kotaSentinelYaz()
		var toplam int
		_ = db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM domains WHERE sistem_kullanici LIKE 'c\_%'`).Scan(&toplam)
		ad := "yok"
		if b := kotaBackendSec(); b != nil {
			ad = b.Ad()
		}
		if acc {
			log.Printf("kota heal[%s]: 0 tenant / %d atlandı (accounting açık ama enforcement KAPALI — limitler enforce EDİLMİYOR; sentinel yazıldı)", ad, toplam)
		} else {
			log.Printf("kota heal[%s]: 0 tenant / %d atlandı (fs'te kota kapalı — tek seferlik reboot gerekli; sentinel yazıldı)", ad, toplam)
		}
		return
	}
	// enforcement aktif → reboot sonrası bayat reboot uyarısını kaldır (idempotent).
	kotaSentinelSil()
	rows, err := db.QueryContext(ctx,
		`SELECT id FROM domains WHERE sistem_kullanici LIKE 'c\_%' ORDER BY id`)
	if err != nil {
		log.Printf("kota heal: domain listesi okunamadı: %v", err)
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()

	var uygulandi, atlandi int
	for _, id := range ids {
		select {
		case <-ctx.Done():
			log.Printf("kota heal: iptal (ctx) — %d tenant / %d atlandı", uygulandi, atlandi)
			return
		default:
		}
		if e := DomainKotaUygula(ctx, db, id); e != nil {
			log.Printf("kota heal: domain %d hata: %v", id, e)
			atlandi++
			continue
		}
		uygulandi++
	}
	log.Printf("kota heal: %d tenant / %d atlandı", uygulandi, atlandi)
}
