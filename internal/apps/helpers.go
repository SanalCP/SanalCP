package apps

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reAltDizin = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]{0,30}[a-z0-9])?$`)
	reEmail    = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

// zatenKuruluMu: hedef dizinde zaten bir kurulum/içerik var mı? Varsa (mesaj, true)
// döner ve kurulum DURDURULUR (mevcut içerik asla ezilmez). WordPress'teki
// kurulumZatenVar ile birebir aynı mantık, marker dosya adı parametreleştirilmiş.
func zatenKuruluMu(hedef, marker string) (string, bool) {
	if _, err := os.Stat(filepath.Join(hedef, marker)); err == nil {
		return "bu dizinde zaten bir kurulum var (mevcut kurulum korunuyor)", true
	}
	entries, err := os.ReadDir(hedef)
	if err != nil {
		return "", false // dizin yok = temiz
	}
	for _, e := range entries {
		switch strings.ToLower(e.Name()) {
		case "index.html", "index.htm", "favicon.ico", "robots.txt",
			"error_log", ".user.ini", ".well-known", "cgi-bin",
			".ftpquota", ".htaccess", ".git", ".gitkeep":
			continue
		}
		return "hedef dizin boş değil — mevcut içerik/kurulum korunuyor (üzerine yazılmaz). Boş bir alt dizin seçin", true
	}
	return "", false
}

// cozDizin: {dizin} kullanıcı girdisini domain'in public_html'i İÇİNDE güvenli
// bir mutlak yola çözer + marker dosyasının orada var olduğunu doğrular.
func cozDizin(sk, dizinStr, marker string) (string, error) {
	root := "/home/" + sk + "/public_html"
	d := strings.TrimPrefix(strings.TrimSpace(dizinStr), "/ (kök)")
	rel := strings.Trim(strings.TrimSpace(d), "/")
	dir := root
	if rel != "" && rel != "(kök)" {
		dir = filepath.Join(root, rel)
	}
	clean := filepath.Clean(dir)
	if clean != root && !strings.HasPrefix(clean, root+"/") {
		return "", fmt.Errorf("yol domain dizini dışında")
	}
	if _, err := os.Stat(filepath.Join(clean, marker)); err != nil {
		return "", fmt.Errorf("bu dizinde kurulum bulunamadı")
	}
	return clean, nil
}

// randSlug: DB adı/kullanıcı adı için 8 hex karakterlik rastgele parça
// (WordPress'teki randSlug ile birebir aynı).
func randSlug() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand okunamadı, slug üretilemiyor: " + err.Error())
	}
	return hex.EncodeToString(b)
}
