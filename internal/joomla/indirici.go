package joomla

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const releaseAPI = "https://api.github.com/repos/joomla/joomla-cms/releases/latest"

type releaseBilgi struct {
	Surum, URL, SHA256 string
	Boyut              int64
}

var releaseCache struct {
	sync.Mutex
	bilgi releaseBilgi
	zaman time.Time
}

func sonSurum(ctx context.Context) (releaseBilgi, error) {
	releaseCache.Lock()
	defer releaseCache.Unlock()
	if releaseCache.bilgi.Surum != "" && time.Since(releaseCache.zaman) < 10*time.Minute {
		return releaseCache.bilgi, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseAPI, nil)
	if err != nil {
		return releaseBilgi{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return releaseBilgi{}, fmt.Errorf("Joomla sürüm bilgisi alınamadı: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return releaseBilgi{}, fmt.Errorf("Joomla sürüm bilgisi alınamadı: GitHub API HTTP %d", resp.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name, URL, Digest string
			Size              int64
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&body); err != nil {
		return releaseBilgi{}, fmt.Errorf("Joomla sürüm bilgisi ayrıştırılamadı: %w", err)
	}
	for _, a := range body.Assets {
		if strings.HasSuffix(a.Name, "Stable-Full_Package.tar.gz") &&
			strings.HasPrefix(a.URL, "https://api.github.com/repos/joomla/joomla-cms/releases/assets/") &&
			strings.HasPrefix(a.Digest, "sha256:") && a.Size > 0 && a.Size <= 128<<20 {
			b := releaseBilgi{Surum: strings.TrimPrefix(body.TagName, "v"), URL: a.URL,
				SHA256: strings.TrimPrefix(a.Digest, "sha256:"), Boyut: a.Size}
			if len(b.SHA256) != 64 || b.Surum == "" {
				break
			}
			releaseCache.bilgi, releaseCache.zaman = b, time.Now()
			return b, nil
		}
	}
	return releaseBilgi{}, fmt.Errorf("Joomla sürümünde doğrulanabilir tam tar.gz paketi bulunamadı")
}

func indirVeDogrula(ctx context.Context, hedef string) (string, error) {
	rel, err := sonSurum(ctx)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Joomla paketi indirilemedi: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Joomla paketi indirilemedi: HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "joomla-*.tar.gz")
	if err != nil {
		return "", err
	}
	ad := tmp.Name()
	defer os.Remove(ad)
	h := sha256.New()
	n, kopyaErr := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, rel.Boyut+1))
	kapatErr := tmp.Close()
	if kopyaErr != nil {
		return "", fmt.Errorf("Joomla paketi yazılamadı: %w", kopyaErr)
	}
	if kapatErr != nil {
		return "", kapatErr
	}
	if n != rel.Boyut {
		return "", fmt.Errorf("Joomla paket boyutu uyuşmuyor: beklenen %d, gelen %d", rel.Boyut, n)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != rel.SHA256 {
		return "", fmt.Errorf("Joomla paket SHA-256 doğrulaması başarısız")
	}
	if err := tarGZAc(ad, hedef); err != nil {
		return "", err
	}
	return rel.Surum, nil
}

func tarGZAc(arsiv, hedef string) error {
	f, err := os.Open(arsiv)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("Joomla gzip açılamadı: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	kok := filepath.Clean(hedef)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("Joomla tar okunamadı: %w", err)
		}
		rel := filepath.Clean(strings.TrimPrefix(h.Name, "./"))
		if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz Joomla tar girdisi: %s", h.Name)
		}
		yol := filepath.Join(kok, rel)
		if yol != kok && !strings.HasPrefix(yol, kok+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz Joomla tar girdisi: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(yol, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(yol), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(yol, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			_, kopyaErr := io.Copy(out, io.LimitReader(tr, h.Size))
			kapatErr := out.Close()
			if kopyaErr != nil {
				return kopyaErr
			}
			if kapatErr != nil {
				return kapatErr
			}
		default:
			return fmt.Errorf("desteklenmeyen Joomla tar girdisi: %s", h.Name)
		}
	}
}
