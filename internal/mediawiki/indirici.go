package mediawiki

import (
	"archive/tar"
	"compress/gzip"
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

var indirClient = &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}, Timeout: 10 * time.Minute}

const (
	sabitSurum        = "1.46.0"
	paketURL          = "https://releases.wikimedia.org/mediawiki/1.46/mediawiki-1.46.0.tar.gz"
	paketSHA256       = "ac395e4ffd3b63b86a242efd679257503e463445ba9f989b514d9d3b342c456a"
	paketBoyut  int64 = 98615738
)

func indirVeDogrula(ctx context.Context, hedef string) error {
	tmp, err := os.CreateTemp("", "mediawiki-*.tar.gz")
	if err != nil {
		return err
	}
	ad := tmp.Name()
	defer os.Remove(ad)
	var sonHata error
	for deneme := 0; deneme < 5; deneme++ {
		offset, _ := tmp.Seek(0, io.SeekEnd)
		if offset >= paketBoyut {
			break
		}
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, paketURL, nil)
		if reqErr != nil {
			return reqErr
		}
		req.Header.Set("User-Agent", "curl/8.5.0")
		if offset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		}
		resp, getErr := indirClient.Do(req)
		if getErr != nil {
			sonHata = getErr
			continue
		}
		if offset > 0 && resp.StatusCode == http.StatusOK {
			if err := tmp.Truncate(0); err != nil {
				resp.Body.Close()
				return err
			}
			if _, err := tmp.Seek(0, io.SeekStart); err != nil {
				resp.Body.Close()
				return err
			}
			offset = 0
		}
		if (offset == 0 && resp.StatusCode != http.StatusOK) || (offset > 0 && resp.StatusCode != http.StatusPartialContent) {
			resp.Body.Close()
			return fmt.Errorf("MediaWiki paketi indirilemedi: HTTP %d", resp.StatusCode)
		}
		_, sonHata = io.Copy(tmp, io.LimitReader(resp.Body, paketBoyut-offset+1))
		resp.Body.Close()
		if sonHata == nil {
			break
		}
	}
	n, _ := tmp.Seek(0, io.SeekEnd)
	if err := tmp.Close(); err != nil {
		return err
	}
	if n != paketBoyut {
		return fmt.Errorf("MediaWiki paket boyutu uyuşmuyor: beklenen %d, gelen %d (son hata: %v)", paketBoyut, n, sonHata)
	}
	dosya, err := os.Open(ad)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, hashErr := io.Copy(h, dosya)
	closeErr := dosya.Close()
	if hashErr != nil {
		return hashErr
	}
	if closeErr != nil {
		return closeErr
	}
	if hex.EncodeToString(h.Sum(nil)) != paketSHA256 {
		return fmt.Errorf("MediaWiki paket SHA-256 doğrulaması başarısız")
	}
	return tarGZAc(ad, hedef)
}

func tarGZAc(arsiv, hedef string) error {
	f, err := os.Open(arsiv)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("MediaWiki gzip açılamadı: %w", err)
	}
	defer gz.Close()
	tr, kok := tar.NewReader(gz), filepath.Clean(hedef)
	var toplam int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("MediaWiki tar okunamadı: %w", err)
		}
		temiz := filepath.Clean(h.Name)
		if temiz == "mediawiki-"+sabitSurum && h.Typeflag == tar.TypeDir {
			continue
		}
		rel, ok := strings.CutPrefix(temiz, "mediawiki-"+sabitSurum+string(os.PathSeparator))
		if !ok || rel == "" || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz MediaWiki tar girdisi: %s", h.Name)
		}
		yol := filepath.Join(kok, rel)
		if !strings.HasPrefix(yol, kok+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz MediaWiki tar girdisi: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(yol, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			toplam += h.Size
			if h.Size < 0 || toplam > 1<<30 {
				return fmt.Errorf("MediaWiki paketi açma sınırını aşıyor")
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
				return fmt.Errorf("MediaWiki tar girdisi eksik: %s", h.Name)
			}
			if outErr != nil {
				return outErr
			}
		default:
			return fmt.Errorf("desteklenmeyen MediaWiki tar girdisi: %s", h.Name)
		}
	}
	if _, err := os.Stat(filepath.Join(kok, "maintenance", "run.php")); err != nil {
		return fmt.Errorf("MediaWiki paketi maintenance CLI içermiyor")
	}
	return nil
}
