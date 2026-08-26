// Package prestashop: apps.Uygulama arayüzünün PrestaShop implementasyonu.
package prestashop

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// psSonSurum: PrestaShop/PrestaShop reposunun GitHub API'sindeki en güncel
// release tag'ini çeker (ör. "9.1.5"). WP-CLI'nin "core download"unun her
// zaman en güncel sürümü indirmesiyle aynı felsefe.
func psSonSurum(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/PrestaShop/PrestaShop/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("PrestaShop sürüm bilgisi alınamadı: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PrestaShop sürüm bilgisi alınamadı: GitHub API HTTP %d", resp.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.TagName == "" {
		return "", fmt.Errorf("PrestaShop sürüm bilgisi ayrıştırılamadı")
	}
	return body.TagName, nil
}

// psIndirVeAc: PrestaShop kurulum paketini indirir ve hedef dizine açar.
//
// 🔴 URL KALIBI DOĞRULANMADI (bkz. Task 8 notu, plan dosyası) — ilk canlı
// kurulumda başarısız olursa TEK değişecek yer burasıdır.
func psIndirVeAc(ctx context.Context, surum, hedef string) error {
	url := fmt.Sprintf("https://download.prestashop.com/download/releases/prestashop_%s.zip", surum)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("PrestaShop kaynağı indirilemedi (%s): %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PrestaShop kaynağı indirilemedi: %s → HTTP %d", url, resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "prestashop-*.zip")
	if err != nil {
		return fmt.Errorf("geçici dosya oluşturulamadı: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return fmt.Errorf("indirilen dosya yazılamadı: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return psZipAc(tmp.Name(), hedef)
}

// psZipAc: zip'i hedef dizine açar. Zip içeriği ya doğrudan kökte (index.php,
// install/, ...) ya da tek bir ortak üst dizin altında paketlenmiş olabilir —
// ikinci durumda o üst dizin soyulur. zip-slip'e karşı korumalıdır.
func psZipAc(zipYolu, hedef string) error {
	zr, err := zip.OpenReader(zipYolu)
	if err != nil {
		return fmt.Errorf("zip açılamadı: %w", err)
	}
	defer zr.Close()

	onEk := ortakUstDizin(zr.File)
	hedefTemiz := filepath.Clean(hedef)

	for _, f := range zr.File {
		rel := strings.TrimPrefix(f.Name, onEk)
		if rel == "" {
			continue
		}
		hedefYol := filepath.Join(hedef, rel)
		if hedefYol != hedefTemiz && !strings.HasPrefix(hedefYol, hedefTemiz+string(os.PathSeparator)) {
			return fmt.Errorf("güvensiz zip girdisi: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(hedefYol, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(hedefYol), 0o755); err != nil {
			return err
		}
		if err := psDosyaCikar(f, hedefYol); err != nil {
			return err
		}
	}
	return nil
}

func psDosyaCikar(f *zip.File, hedefYol string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(hedefYol, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

// ortakUstDizin: tüm zip girdileri tek bir ortak üst dizin altındaysa o dizin
// adını ("ad/" formatında) döner, yoksa boş string (kökte açılır) döner.
func ortakUstDizin(files []*zip.File) string {
	if len(files) == 0 {
		return ""
	}
	ilkParca := strings.SplitN(files[0].Name, "/", 2)
	if len(ilkParca) != 2 {
		return ""
	}
	aday := ilkParca[0] + "/"
	for _, f := range files {
		if !strings.HasPrefix(f.Name, aday) {
			return ""
		}
	}
	return aday
}

// psKomut: PHP'yi domain kullanıcısı olarak, WordPress modülündeki wpKomut ile
// birebir aynı runuser+env+TMPDIR deseniyle çalıştırır (bkz. internal/wordpress/
// wordpress.go — TMPDIR=/home/sk, root'a ait /var/lib/sanalcp/tmp izin hatasından
// kaçınmak için).
func psKomut(ctx context.Context, sk string, args ...string) ([]byte, error) {
	full := append([]string{"-u", sk, "--", "env", "HOME=/home/" + sk, "TMPDIR=/home/" + sk,
		"/usr/bin/php"}, args...)
	cmd := exec.CommandContext(ctx, "runuser", full...)
	return cmd.CombinedOutput()
}
