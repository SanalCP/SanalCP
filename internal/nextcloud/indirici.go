package nextcloud

import (
	"archive/tar"
	"compress/bzip2"
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
	"strings"
	"sync"
	"time"
)

const releaseAPI = "https://api.github.com/repos/nextcloud-releases/server/releases/latest"

type releaseBilgi struct {
	Surum, URL, SHA256 string
	Boyut              int64
}

var releaseCache struct {
	sync.Mutex
	bilgi releaseBilgi
	zaman time.Time
}

var kararliSurum = regexp.MustCompile(`^34\.[0-9]+\.[0-9]+$`)

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
		return releaseBilgi{}, fmt.Errorf("Nextcloud sürüm bilgisi alınamadı: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return releaseBilgi{}, fmt.Errorf("Nextcloud sürüm bilgisi alınamadı: GitHub API HTTP %d", resp.StatusCode)
	}
	var body struct {
		Tag               string `json:"tag_name"`
		Draft, Prerelease bool
		Assets            []struct {
			Name, URL, Digest string
			Size              int64
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&body); err != nil {
		return releaseBilgi{}, fmt.Errorf("Nextcloud sürüm bilgisi ayrıştırılamadı: %w", err)
	}
	surum := strings.TrimPrefix(body.Tag, "v")
	if body.Draft || body.Prerelease || !kararliSurum.MatchString(surum) {
		return releaseBilgi{}, fmt.Errorf("desteklenen kararlı Nextcloud 34 sürümü bulunamadı")
	}
	beklenen := "nextcloud-" + surum + ".tar.bz2"
	for _, a := range body.Assets {
		if a.Name != beklenen || !strings.HasPrefix(a.URL, "https://api.github.com/repos/nextcloud-releases/server/releases/assets/") ||
			!strings.HasPrefix(a.Digest, "sha256:") || a.Size <= 0 || a.Size > 384<<20 {
			continue
		}
		ozet := strings.TrimPrefix(a.Digest, "sha256:")
		if len(ozet) == 64 {
			b := releaseBilgi{Surum: surum, URL: a.URL, SHA256: ozet, Boyut: a.Size}
			releaseCache.bilgi, releaseCache.zaman = b, time.Now()
			return b, nil
		}
	}
	return releaseBilgi{}, fmt.Errorf("doğrulanabilir Nextcloud paketi bulunamadı")
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
		return "", fmt.Errorf("Nextcloud paketi indirilemedi: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Nextcloud paketi indirilemedi: HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "nextcloud-*.tar.bz2")
	if err != nil {
		return "", err
	}
	ad := tmp.Name()
	defer os.Remove(ad)
	h := sha256.New()
	n, cpErr := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, rel.Boyut+1))
	closeErr := tmp.Close()
	if cpErr != nil {
		return "", fmt.Errorf("Nextcloud paketi yazılamadı: %w", cpErr)
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n != rel.Boyut {
		return "", fmt.Errorf("Nextcloud paket boyutu uyuşmuyor: beklenen %d, gelen %d", rel.Boyut, n)
	}
	if hex.EncodeToString(h.Sum(nil)) != rel.SHA256 {
		return "", fmt.Errorf("Nextcloud paket SHA-256 doğrulaması başarısız")
	}
	if err := tarBZ2Ac(ad, hedef); err != nil {
		return "", err
	}
	return rel.Surum, nil
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
	dosyaSayisi := 0
	type bekleyenBaglanti struct{ yol, hedef string }
	var bekleyen []bekleyenBaglanti
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("Nextcloud tar.bz2 okunamadı: %w", err)
		}
		temiz := filepath.Clean(h.Name)
		if temiz == "nextcloud" && h.Typeflag == tar.TypeDir {
			continue
		}
		rel, ok := strings.CutPrefix(temiz, "nextcloud"+string(os.PathSeparator))
		if !ok || rel == "" || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz Nextcloud tar girdisi: %s", h.Name)
		}
		yol := filepath.Join(kok, rel)
		if !strings.HasPrefix(yol, kok+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz Nextcloud tar girdisi: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(yol, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			dosyaSayisi++
			toplam += h.Size
			if h.Size < 0 || dosyaSayisi > 200000 || toplam > 2<<30 {
				return fmt.Errorf("Nextcloud paketi açma sınırını aşıyor")
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
			if cpErr != nil {
				return fmt.Errorf("Nextcloud tar girdisi açılamadı (%s, %d/%d bayt): %w", h.Name, n, h.Size, cpErr)
			}
			if n != h.Size {
				return fmt.Errorf("Nextcloud tar girdisi eksik (%s, %d/%d bayt)", h.Name, n, h.Size)
			}
			if outErr != nil {
				return outErr
			}
		case tar.TypeLink:
			// Resmî paket, tekrarlanan lisans dosyalarını tar hardlink'i olarak
			// saklayabilir. Hem bağlantı hem hedef yalnız nextcloud/ kökü içinde olmalı.
			hedefTemiz := filepath.Clean(h.Linkname)
			hedefRel, ok := strings.CutPrefix(hedefTemiz, "nextcloud"+string(os.PathSeparator))
			if !ok || hedefRel == "" || filepath.IsAbs(hedefRel) || hedefRel == ".." || strings.HasPrefix(hedefRel, ".."+string(os.PathSeparator)) {
				return fmt.Errorf("güvensiz Nextcloud hardlink hedefi: %s", h.Linkname)
			}
			hedefYol := filepath.Join(kok, hedefRel)
			if !strings.HasPrefix(hedefYol, kok+string(os.PathSeparator)) {
				return fmt.Errorf("güvensiz Nextcloud hardlink hedefi: %s", h.Linkname)
			}
			if err := os.MkdirAll(filepath.Dir(yol), 0o755); err != nil {
				return err
			}
			if err := os.Link(hedefYol, yol); err != nil {
				return fmt.Errorf("Nextcloud hardlink oluşturulamadı (%s): %w", h.Name, err)
			}
		case tar.TypeSymlink:
			// Nextcloud dağıtımı bazı aynı lisans dosyalarını göreli symlink ile
			// yineler. Sunucuda symlink bırakmak yerine, yalnız arşiv kökü içinde
			// çözülen ve daha önce açılmış hedefe hardlink üretiriz.
			if filepath.IsAbs(h.Linkname) {
				return fmt.Errorf("güvensiz Nextcloud symlink hedefi: %s", h.Linkname)
			}
			hedefArsiv := filepath.Clean(filepath.Join(filepath.Dir(temiz), h.Linkname))
			hedefRel, ok := strings.CutPrefix(hedefArsiv, "nextcloud"+string(os.PathSeparator))
			if !ok || hedefRel == "" || hedefRel == ".." || strings.HasPrefix(hedefRel, ".."+string(os.PathSeparator)) {
				return fmt.Errorf("güvensiz Nextcloud symlink hedefi: %s", h.Linkname)
			}
			hedefYol := filepath.Join(kok, hedefRel)
			if !strings.HasPrefix(hedefYol, kok+string(os.PathSeparator)) {
				return fmt.Errorf("güvensiz Nextcloud symlink hedefi: %s", h.Linkname)
			}
			bekleyen = append(bekleyen, bekleyenBaglanti{yol: yol, hedef: hedefYol})
		default:
			return fmt.Errorf("desteklenmeyen Nextcloud tar girdisi (tip %d): %s", h.Typeflag, h.Name)
		}
	}
	// Tar girdilerinde symlink hedefi bağlantıdan sonra gelebilir. Hedefi açılmış
	// normal dosya olana dek birkaç tur çöz; hiçbir zaman gerçek symlink üretme.
	for len(bekleyen) > 0 {
		kalan := make([]bekleyenBaglanti, 0, len(bekleyen))
		ilerledi := false
		for _, b := range bekleyen {
			fi, err := os.Lstat(b.hedef)
			if os.IsNotExist(err) {
				kalan = append(kalan, b)
				continue
			}
			if err != nil || !fi.Mode().IsRegular() {
				return fmt.Errorf("Nextcloud bağlantı hedefi normal dosya değil: %s", b.hedef)
			}
			if err := os.MkdirAll(filepath.Dir(b.yol), 0o755); err != nil {
				return err
			}
			if err := os.Link(b.hedef, b.yol); err != nil {
				return fmt.Errorf("Nextcloud güvenli bağlantısı oluşturulamadı (%s): %w", b.yol, err)
			}
			ilerledi = true
		}
		if !ilerledi {
			return fmt.Errorf("Nextcloud bağlantı hedefi pakette bulunamadı: %s", kalan[0].hedef)
		}
		bekleyen = kalan
	}
	if _, err := os.Stat(filepath.Join(kok, "occ")); err != nil {
		return fmt.Errorf("Nextcloud paketi occ dosyasını içermiyor")
	}
	return nil
}
