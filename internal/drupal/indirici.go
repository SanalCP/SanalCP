package drupal

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const releaseAPI = "https://updates.drupal.org/release-history/drupal/current"

type releaseBilgi struct {
	Surum, URL, MD5 string
	Boyut           int64
}

var releaseCache struct {
	sync.Mutex
	bilgi releaseBilgi
	zaman time.Time
}

type releaseXML struct {
	Releases []struct {
		Version  string `xml:"version"`
		Status   string `xml:"status"`
		Security struct {
			Covered string `xml:"covered,attr"`
		} `xml:"security"`
		Files []struct {
			URL   string `xml:"url"`
			Tur   string `xml:"archive_type"`
			MD5   string `xml:"md5"`
			Boyut int64  `xml:"size"`
		} `xml:"files>file"`
	} `xml:"releases>release"`
}

var kararliSurum = regexp.MustCompile(`^11\.[0-9]+\.[0-9]+$`)

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
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return releaseBilgi{}, fmt.Errorf("Drupal sürüm bilgisi alınamadı: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return releaseBilgi{}, fmt.Errorf("Drupal sürüm bilgisi alınamadı: HTTP %d", resp.StatusCode)
	}
	var body releaseXML
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return releaseBilgi{}, fmt.Errorf("Drupal sürüm bilgisi ayrıştırılamadı: %w", err)
	}
	for _, r := range body.Releases {
		if r.Status != "published" || r.Security.Covered != "1" || !kararliSurum.MatchString(r.Version) {
			continue
		}
		for _, f := range r.Files {
			b := releaseBilgi{Surum: r.Version, URL: f.URL, MD5: strings.ToLower(f.MD5), Boyut: f.Boyut}
			if f.Tur == "tar.gz" && resmiPaketURL(b.URL) && len(b.MD5) == 32 && b.Boyut > 0 && b.Boyut <= 128<<20 {
				releaseCache.bilgi, releaseCache.zaman = b, time.Now()
				return b, nil
			}
		}
	}
	return releaseBilgi{}, fmt.Errorf("güvenlik kapsamındaki kararlı Drupal 11 paketi bulunamadı")
}

func resmiPaketURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() == "ftp.drupal.org" &&
		strings.HasPrefix(u.EscapedPath(), "/files/projects/drupal-") && strings.HasSuffix(u.EscapedPath(), ".tar.gz")
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
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Drupal paketi indirilemedi: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Drupal paketi indirilemedi: HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "drupal-*.tar.gz")
	if err != nil {
		return "", err
	}
	ad := tmp.Name()
	defer os.Remove(ad)
	// Drupal servisi yalnız MD5 yayımlar; TLS, alan adı/yol allowlist'i ve
	// bildirilen boyut da birlikte doğrulanır.
	h := md5.New()
	n, kopyaErr := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, rel.Boyut+1))
	kapatErr := tmp.Close()
	if kopyaErr != nil {
		return "", fmt.Errorf("Drupal paketi yazılamadı: %w", kopyaErr)
	}
	if kapatErr != nil {
		return "", kapatErr
	}
	if n != rel.Boyut {
		return "", fmt.Errorf("Drupal paket boyutu uyuşmuyor: beklenen %d, gelen %d", rel.Boyut, n)
	}
	if hex.EncodeToString(h.Sum(nil)) != rel.MD5 {
		return "", fmt.Errorf("Drupal paket özeti doğrulanamadı")
	}
	if err := tarGZAc(ad, hedef, "drupal-"+rel.Surum); err != nil {
		return "", err
	}
	return rel.Surum, nil
}

func tarGZAc(arsiv, hedef, kokDizin string) error {
	f, err := os.Open(arsiv)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("Drupal gzip açılamadı: %w", err)
	}
	defer gz.Close()
	tr, kok := tar.NewReader(gz), filepath.Clean(hedef)
	var toplam int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("Drupal tar okunamadı: %w", err)
		}
		temiz := filepath.Clean(h.Name)
		if temiz == kokDizin && h.Typeflag == tar.TypeDir {
			continue
		}
		rel, ok := strings.CutPrefix(temiz, kokDizin+string(os.PathSeparator))
		if !ok || rel == "." || rel == "" || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz Drupal tar girdisi: %s", h.Name)
		}
		yol := filepath.Join(kok, rel)
		if !strings.HasPrefix(yol, kok+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz Drupal tar girdisi: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(yol, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			toplam += h.Size
			if h.Size < 0 || toplam > 512<<20 {
				return fmt.Errorf("Drupal paketi açılmış boyut sınırını aşıyor")
			}
			if err := os.MkdirAll(filepath.Dir(yol), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(yol, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			n, cpErr := io.CopyN(out, tr, h.Size)
			closeErr := out.Close()
			if cpErr != nil || n != h.Size {
				return fmt.Errorf("Drupal tar girdisi eksik: %s", h.Name)
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("desteklenmeyen Drupal tar girdisi: %s", h.Name)
		}
	}
}
