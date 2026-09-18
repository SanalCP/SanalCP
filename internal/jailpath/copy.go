package jailpath

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// tenantTreeCommand pins the working directory and drops all root credentials
// before executing the tool. A renamed public_html cannot redirect the command.
func tenantTreeCommand(ctx context.Context, sk string, dir *os.File, args ...string) (*exec.Cmd, error) {
	if _, err := TenantHome(sk); err != nil {
		return nil, err
	}
	uid, gid, ok := TenantIDs(sk)
	if !ok || uid == 0 || gid == 0 {
		return nil, fmt.Errorf("tenant kimliği bulunamadı: %s", sk)
	}
	cmd := exec.CommandContext(ctx, "tar", append([]string{"-C", "/proc/self/fd/3"}, args...)...)
	cmd.ExtraFiles = []*os.File{dir}
	cmd.Dir = "/"
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent", "LANG=C"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: []uint32{uint32(gid)}}}
	return cmd, nil
}

// AgacKopyala copies between different tenant users without a root rsync.
// The source is read as its owner, into a root-only snapshot. Only after a
// successful snapshot is the pinned destination cleared and extracted as its
// owner. Symlinks in the tree are preserved, never dereferenced by the sender.
func AgacKopyala(ctx context.Context, sourceSK, sourceRel, targetSK, targetRel string) error {
	sh, err := TenantHome(sourceSK)
	if err != nil {
		return err
	}
	th, err := TenantHome(targetSK)
	if err != nil {
		return err
	}
	source, err := AcDizin(sh, sourceRel)
	if err != nil {
		return fmt.Errorf("kaynak dizin güvenli değil: %w", err)
	}
	defer source.Close()
	target, err := AcDizin(th, targetRel)
	if err != nil {
		return fmt.Errorf("hedef dizin güvenli değil: %w", err)
	}
	defer target.Close()
	return copyPinnedTrees(ctx, sourceSK, source, targetSK, target)
}

func copyPinnedTrees(ctx context.Context, sourceSK string, source *os.File, targetSK string, target *os.File) error {
	sender, err := tenantTreeCommand(ctx, sourceSK, source, "--one-file-system", "-cf", "-", ".")
	if err != nil {
		return err
	}
	receiver, err := tenantTreeCommand(ctx, targetSK, target, "--no-same-owner", "--no-same-permissions", "-xf", "-")
	if err != nil {
		return err
	}
	snapshot, err := os.CreateTemp("", "sanalcp-tree-*.tar")
	if err != nil {
		return err
	}
	defer os.Remove(snapshot.Name())
	defer snapshot.Close()
	var stderr bytes.Buffer
	sender.Stdout, sender.Stderr = snapshot, &stderr
	if err := sender.Run(); err != nil {
		return fmt.Errorf("kaynak kopyalanamadı: %w: %s", err, stderr.String())
	}
	return extractSnapshot(ctx, snapshot, receiver, target, true)
}

func extractSnapshot(ctx context.Context, snapshot *os.File, receiver *exec.Cmd, target *os.File, deleteMissing bool) error {
	if _, err := snapshot.Seek(0, 0); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if deleteMissing {
		if err := clearDirectory(target); err != nil {
			return err
		}
	}
	var stderr bytes.Buffer
	receiver.Stdin, receiver.Stderr = snapshot, &stderr
	if err := receiver.Run(); err != nil {
		return fmt.Errorf("hedef kopyalanamadı: %w: %s", err, stderr.String())
	}
	return nil
}

// PaketYukle lets existing archive parsers extract only into a private root
// directory. Publication into a mutable tenant tree always runs as that tenant.
func PaketYukle(ctx context.Context, sk, targetPath string, extract func(string) error) error {
	home, err := TenantHome(sk)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(home, targetPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return fmt.Errorf("paket hedefi tenant dışında")
	}
	target, err := AcDizin(home, rel)
	if err != nil {
		return err
	}
	defer target.Close()
	receiver, err := tenantTreeCommand(ctx, sk, target, "--no-same-owner", "--no-same-permissions", "-xf", "-")
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp("", "sanalcp-package-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := extract(stage); err != nil {
		return err
	}
	snapshot, err := os.CreateTemp("", "sanalcp-package-*.tar")
	if err != nil {
		return err
	}
	defer os.Remove(snapshot.Name())
	defer snapshot.Close()
	// stage is root-owned 0700 and contains only the verified package.
	sender := exec.CommandContext(ctx, "tar", "--one-file-system", "--mode=u+rwX,go+rX", "-cf", "-", "-C", stage, ".")
	sender.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent", "LANG=C"}
	sender.Stdout = snapshot
	if err := sender.Run(); err != nil {
		return err
	}
	return extractSnapshot(ctx, snapshot, receiver, target, false)
}
