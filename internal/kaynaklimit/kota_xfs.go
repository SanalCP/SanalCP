package kaynaklimit

// XFS kota backend — xfs_quota(8). RHEL ailesinin (AlmaLinux/Rocky) bulut imajlarında
// kök dosya sistemi varsayılan olarak XFS'tir.
//
// 🔴 Kök fs XFS kotası ancak MOUNT anında açılır; canlı remount ile açılamaz →
// GRUB `rootflags=uquota` + tek seferlik reboot ŞART.

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type xfsKota struct{}

func (xfsKota) Ad() string            { return "xfs" }
func (xfsKota) KernelBayragi() string { return "rootflags=uquota" }

// Aktif: `xfs_quota -x -c 'state -u' /` çıktısını parse eder.
// noquota'da çıktı boş → (false,false). accounting açık + enforcement kapalı =
// `uqnoenforce` durumu (kullanım sayılır, limit uygulanmaz).
func (xfsKota) Aktif() (accounting, enforcement bool) {
	out, err := exec.Command("xfs_quota", "-x", "-c", "state -u", kotaMount).CombinedOutput()
	if err != nil {
		return false, false
	}
	for _, ln := range strings.Split(string(out), "\n") {
		t := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(t, "Accounting:"):
			accounting = strings.Contains(t, "ON")
		case strings.HasPrefix(t, "Enforcement:"):
			enforcement = strings.Contains(t, "ON")
		}
	}
	return accounting, enforcement
}

// kotaLimitArgs: xfs_quota'ya verilecek arg-slice (saf → birim-test edilebilir).
// soft = hard*0.95. diskMB veya inode 0 ise o metrik "0" = SINIRSIZ bırakılır
// (xfs_quota'da bhard/ihard=0 → limit yok). sk çağırandan önce reKotaSK'dan geçmiş olmalı.
func kotaLimitArgs(sk string, diskMB, inode int) []string {
	diskSoft, diskHard := kotaSoft(diskMB)
	inodeSoft, inodeHard := kotaSoft(inode)
	limit := fmt.Sprintf("limit -u bsoft=%dm bhard=%dm isoft=%d ihard=%d %s",
		diskSoft, diskHard, inodeSoft, inodeHard, sk)
	return []string{"-x", "-c", limit, kotaMount}
}

func (xfsKota) Uygula(ctx context.Context, sk string, diskMB, inode int) error {
	out, err := exec.CommandContext(ctx, "xfs_quota", kotaLimitArgs(sk, diskMB, inode)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("xfs_quota limit %s: %s: %w", sk, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// kotaReportSatir: `xfs_quota report -u -N <metric> /` çıktısında sk satırının Used + Hard
// alanlarını döner (blok metriği KB; inode metriği adet). Satır: User Used Soft Hard [grace...].
func kotaReportSatir(metric, sk string) (used, hard int) {
	out, err := exec.Command("xfs_quota", "-x", "-c", "report -u -N "+metric, kotaMount).CombinedOutput()
	if err != nil {
		return 0, 0
	}
	for _, ln := range strings.Split(string(out), "\n") {
		f := strings.Fields(ln)
		if len(f) < 4 || f[0] != sk {
			continue
		}
		used, _ = strconv.Atoi(f[1])
		hard, _ = strconv.Atoi(f[3])
		return used, hard
	}
	return 0, 0
}

func (xfsKota) Durum(sk string) (kullanilanMB, limitMB, kullanilanInode, limitInode int) {
	bUsedKB, bHardKB := kotaReportSatir("-b", sk) // KB
	iUsed, iHard := kotaReportSatir("-i", sk)     // adet
	return bUsedKB / 1024, bHardKB / 1024, iUsed, iHard
}
