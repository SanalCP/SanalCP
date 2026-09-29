package appruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"sanalcp/internal/jailpath"
)

type GitUpdate func(context.Context) (string, string, error)

var commitRE = regexp.MustCompile(`^[a-fA-F0-9]{40,64}$`)

type DeployResult struct {
	Commit     string
	Log        string
	Managed    bool
	RolledBack bool
}

// prepareManagedAt installs and builds only inside the selected release.
// Package hooks always run under the tenant account.
func prepareManagedAt(ctx context.Context, s site, c Config, rel string) (string, error) {
	interpreter, _, err := validate(s, c)
	if err != nil {
		return "", err
	}
	home, err := jailpath.TenantHome(s.sk)
	if err != nil {
		return "", err
	}
	if _, err := releasePath(s, rel); err != nil {
		return "", err
	}
	cwd := filepath.Join(home, rel)
	entryRel := filepath.Join(rel, strings.TrimPrefix(c.Entrypoint, "public_html/"))
	if !regularManifest(home, entryRel) {
		return "", errors.New("yeni sürümün giriş dosyası yok veya güvenli değil")
	}
	var log strings.Builder
	run := func(argv ...string) error {
		out, err := tenantCommand(ctx, s, cwd, 5*time.Minute, argv...)
		log.WriteString(out)
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(argv[0]), err)
		}
		return nil
	}
	switch c.Runtime {
	case "node":
		if !regularManifest(home, filepath.Join(rel, "package.json")) || !regularManifest(home, filepath.Join(rel, "package-lock.json")) {
			return log.String(), errors.New("package.json ve package-lock.json gerekli")
		}
		npm := filepath.Join(filepath.Dir(interpreter), "npm")
		if st, err := os.Stat(npm); err != nil || !st.Mode().IsRegular() || st.Mode()&0111 == 0 {
			return log.String(), errors.New("seçili Node.js için npm bulunamadı")
		}
		if err := run(npm, "ci", "--no-audit", "--no-fund"); err != nil {
			return log.String(), err
		}
		if err := run(npm, "run", "build", "--if-present"); err != nil {
			return log.String(), err
		}
		if err := run(npm, "prune", "--omit=dev", "--no-audit", "--no-fund"); err != nil {
			return log.String(), err
		}
	case "python":
		if !regularManifest(home, filepath.Join(rel, "requirements.txt")) {
			return log.String(), errors.New("requirements.txt gerekli")
		}
		venv := filepath.Join(cwd, ".venv")
		if err := ensureVenvAt(ctx, s, interpreter, venv); err != nil {
			return log.String(), err
		}
		if err := run(filepath.Join(venv, "bin", "python"), "-m", "pip", "install", "--requirement", "requirements.txt"); err != nil {
			return log.String(), err
		}
	default:
		return log.String(), errors.New("geçersiz çalışma türü")
	}
	return log.String(), nil
}
