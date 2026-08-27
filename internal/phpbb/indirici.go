package phpbb

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var indirClient = &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}, Timeout: 5 * time.Minute}

const (
	sabitSurum  = "3.3.17"
	paketURL    = "https://download.phpbb.com/pub/release/3.3/3.3.17/phpBB-3.3.17.tar.bz2"
	paketSHA256 = "b52fd231e612a099c0af1d2dcb73a79f7d03926a482842c4ee2830d12f461b67"
)

func indirVeDogrula(ctx context.Context, hedef string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, paketURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "curl/8.5.0")
	resp, err := indirClient.Do(req)
	if err != nil {
		return fmt.Errorf("phpBB paketi indirilemedi: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("phpBB paketi indirilemedi: HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "phpbb-*.tar.bz2")
	if err != nil {
		return err
	}
	ad := tmp.Name()
	defer os.Remove(ad)
	h := sha256.New()
	n, cpErr := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, 32<<20))
	closeErr := tmp.Close()
	if cpErr != nil {
		return fmt.Errorf("phpBB paketi yazılamadı: %w", cpErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if n <= 0 || n >= 32<<20 {
		return fmt.Errorf("phpBB paket boyutu geçersiz")
	}
	if hex.EncodeToString(h.Sum(nil)) != paketSHA256 {
		return fmt.Errorf("phpBB paket SHA-256 doğrulaması başarısız")
	}
	return tarBZ2Ac(ad, hedef)
}

func tarBZ2Ac(arsiv, hedef string) error {
	f, err := os.Open(arsiv)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(bzip2.NewReader(f))
	kok := filepath.Clean(hedef)
	var toplam int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("phpBB tar.bz2 okunamadı: %w", err)
		}
		temiz := filepath.Clean(h.Name)
		if temiz == "phpBB3" && h.Typeflag == tar.TypeDir {
			continue
		}
		rel, ok := strings.CutPrefix(temiz, "phpBB3"+string(os.PathSeparator))
		if !ok || rel == "." || rel == "" || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz phpBB tar girdisi: %s", h.Name)
		}
		yol := filepath.Join(kok, rel)
		if !strings.HasPrefix(yol, kok+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz phpBB tar girdisi: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(yol, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			toplam += h.Size
			if h.Size < 0 || toplam > 256<<20 {
				return fmt.Errorf("phpBB paketi açma sınırını aşıyor")
			}
			if err := os.MkdirAll(filepath.Dir(yol), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(yol, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode)&0o755)
			if err != nil {
				return err
			}
			n, cpErr := io.CopyN(out, tr, h.Size)
			outErr := out.Close()
			if cpErr != nil || n != h.Size {
				return fmt.Errorf("phpBB tar girdisi eksik: %s", h.Name)
			}
			if outErr != nil {
				return outErr
			}
		default:
			return fmt.Errorf("desteklenmeyen phpBB tar girdisi: %s", h.Name)
		}
	}
	if _, err := os.Stat(filepath.Join(kok, "install", "phpbbcli.php")); err != nil {
		return fmt.Errorf("phpBB paketi kurulum CLI'si içermiyor")
	}
	return nil
}
