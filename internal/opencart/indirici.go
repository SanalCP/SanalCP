package opencart

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const releaseAPI = "https://api.github.com/repos/opencart/opencart/releases?per_page=20"

type releaseBilgi struct {
	Surum, URL, SHA256 string
	Boyut              int64
}

var releaseCache struct {
	sync.Mutex
	bilgi releaseBilgi
	zaman time.Time
}

var kararliSurum = regexp.MustCompile(`^4\.[0-9]+\.[0-9]+\.[0-9]+$`)

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
		return releaseBilgi{}, fmt.Errorf("OpenCart sürüm bilgisi alınamadı: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return releaseBilgi{}, fmt.Errorf("OpenCart sürüm bilgisi alınamadı: GitHub API HTTP %d", resp.StatusCode)
	}
	var releases []struct {
		Tag               string `json:"tag_name"`
		Draft, Prerelease bool
		Assets            []struct {
			Name, URL, Digest string
			Size              int64
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&releases); err != nil {
		return releaseBilgi{}, fmt.Errorf("OpenCart sürüm bilgisi ayrıştırılamadı: %w", err)
	}
	var enIyi releaseBilgi
	for _, r := range releases {
		if r.Draft || r.Prerelease || !kararliSurum.MatchString(r.Tag) || surumKarsilastir(r.Tag, enIyi.Surum) <= 0 {
			continue
		}
		for _, a := range r.Assets {
			if a.Name == "opencart-"+r.Tag+".zip" &&
				strings.HasPrefix(a.URL, "https://api.github.com/repos/opencart/opencart/releases/assets/") &&
				strings.HasPrefix(a.Digest, "sha256:") && a.Size > 0 && a.Size <= 128<<20 {
				ozet := strings.TrimPrefix(a.Digest, "sha256:")
				if len(ozet) == 64 {
					enIyi = releaseBilgi{Surum: r.Tag, URL: a.URL, SHA256: ozet, Boyut: a.Size}
				}
			}
		}
	}
	if enIyi.Surum == "" {
		return releaseBilgi{}, fmt.Errorf("doğrulanabilir kararlı OpenCart 4 paketi bulunamadı")
	}
	releaseCache.bilgi, releaseCache.zaman = enIyi, time.Now()
	return enIyi, nil
}

func surumKarsilastir(a, b string) int {
	if b == "" {
		return 1
	}
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 4; i++ {
		av, _ := strconv.Atoi(aa[i])
		bv, _ := strconv.Atoi(bb[i])
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}
	return 0
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
		return "", fmt.Errorf("OpenCart paketi indirilemedi: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenCart paketi indirilemedi: HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "opencart-*.zip")
	if err != nil {
		return "", err
	}
	ad := tmp.Name()
	defer os.Remove(ad)
	h := sha256.New()
	n, cpErr := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, rel.Boyut+1))
	closeErr := tmp.Close()
	if cpErr != nil {
		return "", fmt.Errorf("OpenCart paketi yazılamadı: %w", cpErr)
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n != rel.Boyut {
		return "", fmt.Errorf("OpenCart paket boyutu uyuşmuyor: beklenen %d, gelen %d", rel.Boyut, n)
	}
	if hex.EncodeToString(h.Sum(nil)) != rel.SHA256 {
		return "", fmt.Errorf("OpenCart paket SHA-256 doğrulaması başarısız")
	}
	if err := zipAc(ad, hedef); err != nil {
		return "", err
	}
	return rel.Surum, nil
}

func zipAc(arsiv, hedef string) error {
	zr, err := zip.OpenReader(arsiv)
	if err != nil {
		return fmt.Errorf("OpenCart zip açılamadı: %w", err)
	}
	defer zr.Close()
	kok := filepath.Clean(hedef)
	var toplam int64
	var dosyaSayisi int
	for _, f := range zr.File {
		temiz := filepath.Clean(strings.ReplaceAll(f.Name, "\\", "/"))
		if filepath.IsAbs(temiz) || temiz == ".." || strings.HasPrefix(temiz, "../") {
			return fmt.Errorf("güvensiz OpenCart zip girdisi: %s", f.Name)
		}
		rel, ok := strings.CutPrefix(temiz, "upload/")
		if !ok || rel == "" || rel == "." {
			continue
		}
		yol := filepath.Join(kok, filepath.FromSlash(rel))
		if !strings.HasPrefix(yol, kok+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz OpenCart zip girdisi: %s", f.Name)
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("OpenCart zip sembolik bağlantı içeriyor: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(yol, 0o755); err != nil {
				return err
			}
			continue
		}
		dosyaSayisi++
		toplam += int64(f.UncompressedSize64)
		if dosyaSayisi > 100000 || toplam > 512<<20 {
			return fmt.Errorf("OpenCart paketi açma sınırını aşıyor")
		}
		if err := os.MkdirAll(filepath.Dir(yol), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(yol, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		n, cpErr := io.Copy(out, io.LimitReader(rc, int64(f.UncompressedSize64)+1))
		outErr, rcErr := out.Close(), rc.Close()
		if cpErr != nil || n != int64(f.UncompressedSize64) {
			return fmt.Errorf("OpenCart zip girdisi eksik: %s", f.Name)
		}
		if outErr != nil {
			return outErr
		}
		if rcErr != nil {
			return rcErr
		}
	}
	if _, err := os.Stat(filepath.Join(kok, "install", "cli_install.php")); err != nil {
		return fmt.Errorf("OpenCart paketi CLI kurucusunu içermiyor")
	}
	return nil
}
