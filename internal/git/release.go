package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"sanalcp/internal/appruntime"
	"sanalcp/internal/jailpath"
)

var releaseNameRE = regexp.MustCompile(`^release-[a-f0-9]{12}-[a-f0-9]{16}$`)
var releaseSHA = regexp.MustCompile(`^[a-fA-F0-9]{40,64}$`)

func validReleaseRel(id int64, rel string) bool {
	base := ".sanalcp/releases/" + strconv.FormatInt(id, 10)
	return filepath.Dir(rel) == base && releaseNameRE.MatchString(filepath.Base(rel))
}

func stageGitRelease(ctx context.Context, id int64, sk, targetDir, branch, repoURL, token string) (appruntime.StagedRelease, error) {
	var stage appruntime.StagedRelease
	if !gecerliRepoURL(repoURL) || !gecerliTargetDir(targetDir) || !gecerliBranch(branch) {
		return stage, errors.New("geçersiz Git kaynağı")
	}
	home, err := jailpath.TenantHome(sk)
	if err != nil {
		return stage, err
	}
	if err := jailpath.DizinDogrula(home, targetDir); err != nil {
		return stage, fmt.Errorf("Git hedefi güvenli değil: %w", err)
	}
	dst := filepath.Join(home, targetDir)
	if _, err := os.Stat(filepath.Join(dst, ".git")); err != nil {
		return stage, errors.New("Git deposu bulunamadı; önce klonlayın")
	}
	if err := scrubRepoConfig(home, targetDir); err != nil {
		return stage, err
	}
	if out, err := runAsUserArgs(ctx, sk, dst, "git", "-C", dst, "remote", "set-url", "origin", repoURL); err != nil {
		return stage, fmt.Errorf("origin güncellenemedi: %w: %s", err, out)
	}
	stage.Log, err = runAsUserCredentials(ctx, sk, dst, repoURL, token, "git", "-C", dst, "fetch", "--", repoURL, branch)
	if err != nil {
		return stage, fmt.Errorf("Git fetch başarısız: %w", err)
	}
	shaOut, err := runAsUserArgs(ctx, sk, dst, "git", "-C", dst, "rev-parse", "FETCH_HEAD")
	if err != nil {
		return stage, err
	}
	stage.Commit = strings.ToLower(strings.TrimSpace(shaOut))
	if !releaseSHA.MatchString(stage.Commit) {
		return stage, errors.New("Git commit geçersiz")
	}
	parent := ".sanalcp/releases/" + strconv.FormatInt(id, 10)
	if err := jailpath.DizinOlustur(home, parent, sk); err != nil {
		return stage, fmt.Errorf("sürüm dizini oluşturulamadı: %w", err)
	}
	stage.ReleaseDir = filepath.Join(parent, "release-"+stage.Commit[:12]+"-"+randomHex(8))
	path := filepath.Join(home, stage.ReleaseDir)
	out, err := runAsUserArgs(ctx, sk, dst, "git", "-C", dst, "worktree", "add", "--detach", path, stage.Commit)
	stage.Log += out
	if err != nil {
		_, _ = runAsUserArgs(context.Background(), sk, dst, "git", "-C", dst, "worktree", "remove", "--force", path)
		return stage, fmt.Errorf("Git sürümü açılamadı: %w", err)
	}
	if err := jailpath.DizinDogrula(home, stage.ReleaseDir); err != nil {
		_, _ = runAsUserArgs(context.Background(), sk, dst, "git", "-C", dst, "worktree", "remove", "--force", path)
		return stage, fmt.Errorf("Git sürüm dizini güvenli değil: %w", err)
	}
	_, _ = runAsUserArgs(ctx, sk, home, "restorecon", "-R", path)
	return stage, nil
}
