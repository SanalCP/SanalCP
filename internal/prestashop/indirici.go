// Package prestashop: apps.Uygulama arayüzünün PrestaShop implementasyonu.
package prestashop

import (
	"archive/zip"
	"context"
	"crypto/md5" // #nosec G501 -- resmî API yalnız MD5 yayımlıyor; TLS ve alan adı allowlist'i de zorunlu.
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	psReleaseAPI      = "https://api.prestashop-project.org/prestashop/stable"
	psMaksPaketBoyutu = int64(256 << 20)
	psMaksAcilanBoyut = int64(1024 << 20)
	psMaksDosyaSayisi = 100000
)

type psReleaseBilgi struct{ Surum, URL, MD5, PHPMin, PHPMax string }

var psReleaseCache struct {
	sync.Mutex
	bilgi psReleaseBilgi
	zaman time.Time
}

func psRelease(ctx context.Context) (psReleaseBilgi, error) {
	psReleaseCache.Lock()
	defer psReleaseCache.Unlock()
	if psReleaseCache.bilgi.Surum != "" && time.Since(psReleaseCache.zaman) < 10*time.Minute {
		return psReleaseCache.bilgi, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, psReleaseAPI, nil)
	if err != nil {
		return psReleaseBilgi{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return psReleaseBilgi{}, fmt.Errorf("PrestaShop sürüm bilgisi alınamadı: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return psReleaseBilgi{}, fmt.Errorf("PrestaShop sürüm bilgisi alınamadı: HTTP %d", resp.StatusCode)
	}
	var b struct {
		Version string `json:"version"`
		URL     string `json:"zip_download_url"`
		MD5     string `json:"zip_md5"`
		PHPMin  string `json:"php_min_version"`
		PHPMax  string `json:"php_max_version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&b); err != nil {
		return psReleaseBilgi{}, fmt.Errorf("PrestaShop sürüm bilgisi ayrıştırılamadı: %w", err)
	}
	if !psResmiURL(b.URL) || b.Version == "" || len(b.MD5) != 32 {
		return psReleaseBilgi{}, fmt.Errorf("PrestaShop dağıtım metadata'sı geçersiz")
	}
	if _, err := hex.DecodeString(b.MD5); err != nil {
		return psReleaseBilgi{}, fmt.Errorf("PrestaShop paket özeti geçersiz")
	}
	rel := psReleaseBilgi{b.Version, b.URL, strings.ToLower(b.MD5), b.PHPMin, b.PHPMax}
	psReleaseCache.bilgi, psReleaseCache.zaman = rel, time.Now()
	return rel, nil
}

func psResmiURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() == "api.prestashop-project.org" &&
		strings.HasPrefix(u.EscapedPath(), "/assets/prestashop-classic/")
}

func psSonSurum(ctx context.Context) (string, error) {
	r, err := psRelease(ctx)
	return r.Surum, err
}

// psIndirVeAc resmî classic paketi indirir, bütünlüğünü doğrular ve dış
// dağıtım sarmalındaki prestashop.zip içeriğini hedefe güvenle açar.
func psIndirVeAc(ctx context.Context, surum, hedef string) error {
	rel, err := psRelease(ctx)
	if err != nil {
		return err
	}
	if rel.Surum != surum {
		return fmt.Errorf("PrestaShop sürümü indirme sırasında değişti: %s → %s", surum, rel.Surum)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.URL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("PrestaShop paketi indirilemedi: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PrestaShop paketi indirilemedi: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > psMaksPaketBoyutu {
		return fmt.Errorf("PrestaShop paketi boyut sınırını aşıyor")
	}
	tmp, err := os.CreateTemp("", "prestashop-*.zip")
	if err != nil {
		return fmt.Errorf("geçici dosya oluşturulamadı: %w", err)
	}
	ad := tmp.Name()
	defer os.Remove(ad)
	h := md5.New() // #nosec G401 -- resmî API uyumluluğu; URL ayrıca allowlist'li.
	n, kopyaErr := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, psMaksPaketBoyutu+1))
	kapatErr := tmp.Close()
	if kopyaErr != nil {
		return fmt.Errorf("PrestaShop paketi yazılamadı: %w", kopyaErr)
	}
	if kapatErr != nil {
		return kapatErr
	}
	if n > psMaksPaketBoyutu {
		return fmt.Errorf("PrestaShop paketi boyut sınırını aşıyor")
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != rel.MD5 {
		return fmt.Errorf("PrestaShop paket bütünlük doğrulaması başarısız")
	}
	return psDagitimZipAc(ad, hedef)
}

func psDagitimZipAc(disZip, hedef string) error {
	zr, err := zip.OpenReader(disZip)
	if err != nil {
		return fmt.Errorf("PrestaShop dağıtım zip'i açılamadı: %w", err)
	}
	defer zr.Close()
	var ic *zip.File
	for _, f := range zr.File {
		if f.Name == "prestashop.zip" && !f.FileInfo().IsDir() {
			ic = f
			break
		}
	}
	if ic == nil || ic.UncompressedSize64 > uint64(psMaksPaketBoyutu) {
		return fmt.Errorf("PrestaShop dağıtımında geçerli prestashop.zip bulunamadı")
	}
	r, err := ic.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	tmp, err := os.CreateTemp("", "prestashop-inner-*.zip")
	if err != nil {
		return err
	}
	ad := tmp.Name()
	defer os.Remove(ad)
	n, kopyaErr := io.Copy(tmp, io.LimitReader(r, psMaksPaketBoyutu+1))
	kapatErr := tmp.Close()
	if kopyaErr != nil {
		return kopyaErr
	}
	if kapatErr != nil {
		return kapatErr
	}
	if n > psMaksPaketBoyutu || uint64(n) != ic.UncompressedSize64 {
		return fmt.Errorf("iç PrestaShop paketi boyutu geçersiz")
	}
	return psZipAc(ad, hedef)
}

func psZipAc(zipYolu, hedef string) error {
	zr, err := zip.OpenReader(zipYolu)
	if err != nil {
		return fmt.Errorf("zip açılamadı: %w", err)
	}
	defer zr.Close()
	if len(zr.File) > psMaksDosyaSayisi {
		return fmt.Errorf("PrestaShop zip dosya sayısı sınırını aşıyor")
	}
	onEk := ortakUstDizin(zr.File)
	kok := filepath.Clean(hedef)
	var toplam int64
	for _, f := range zr.File {
		if f.UncompressedSize64 > uint64(psMaksAcilanBoyut) {
			return fmt.Errorf("PrestaShop açılmış boyut sınırını aşıyor")
		}
		toplam += int64(f.UncompressedSize64)
		if toplam > psMaksAcilanBoyut {
			return fmt.Errorf("PrestaShop açılmış boyut sınırını aşıyor")
		}
		rel := filepath.Clean(strings.TrimPrefix(f.Name, onEk))
		if rel == "." {
			continue
		}
		if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("güvensiz zip girdisi: %s", f.Name)
		}
		yol := filepath.Join(kok, rel)
		if yol != kok && !strings.HasPrefix(yol, kok+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz zip girdisi: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(yol, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(yol), 0o755); err != nil {
			return err
		}
		if err := psDosyaCikar(f, yol); err != nil {
			return err
		}
	}
	return nil
}

func psDosyaCikar(f *zip.File, hedefYol string) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	out, err := os.OpenFile(hedefYol, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, kopyaErr := io.Copy(out, io.LimitReader(r, int64(f.UncompressedSize64)))
	kapatErr := out.Close()
	if kopyaErr != nil {
		return kopyaErr
	}
	return kapatErr
}

func ortakUstDizin(files []*zip.File) string {
	if len(files) == 0 {
		return ""
	}
	ilk := strings.SplitN(files[0].Name, "/", 2)
	if len(ilk) != 2 {
		return ""
	}
	aday := ilk[0] + "/"
	for _, f := range files {
		if !strings.HasPrefix(f.Name, aday) {
			return ""
		}
	}
	return aday
}

func psKomut(ctx context.Context, sk string, args ...string) ([]byte, error) {
	full := append([]string{"-u", sk, "--", "env", "HOME=/home/" + sk, "TMPDIR=/home/" + sk,
		"/usr/bin/php"}, args...)
	return exec.CommandContext(ctx, "runuser", full...).CombinedOutput()
}
