package backups

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// ensureHostKey: SFTP hedefleri için TOFU (trust-on-first-use) host key
// pinlemesi. d.HostKey zaten doluysa (daha önce pinlenmiş) dokunmadan döner.
// Boşsa ssh-keyscan ile host key'i alır ve — d kalıcı bir kayıtsa (ID>0) —
// backup_destinations.host_key'e kalıcı olarak yazar; böylece SONRAKİ HER
// bağlantı artık "ilk görüşte güven" değil, PİNLENMİŞ anahtara karşı
// doğrulanır (StrictHostKeyChecking=yes). Ad-hoc/kaydedilmemiş test
// çağrılarında (ID==0) yalnız bu çağrı için kullanılır, kalıcı yazılmaz.
func ensureHostKey(ctx context.Context, db *sql.DB, d *Destination) (string, error) {
	if d.HostKey != "" {
		return d.HostKey, nil
	}
	out, err := exec.CommandContext(ctx, "ssh-keyscan",
		"-p", strconv.Itoa(d.Port), "-T", "10", d.Host).Output()
	if err != nil {
		return "", fmt.Errorf("host key alınamadı (ssh-keyscan): %w", err)
	}
	keys := strings.TrimSpace(string(out))
	if keys == "" {
		return "", fmt.Errorf("host key alınamadı: sunucu SSH bağlantısına yanıt vermedi")
	}
	if d.ID != 0 {
		if _, err := db.ExecContext(ctx,
			`UPDATE backup_destinations SET host_key=? WHERE id=?`, keys, d.ID); err != nil {
			return "", fmt.Errorf("host key kaydedilemedi: %w", err)
		}
	}
	d.HostKey = keys
	return keys, nil
}

// knownHostsDosyaYaz: pinlenmiş known_hosts içeriğini 0600 izinli, sadece bu
// bağlantı için geçerli bir geçici dosyaya yazar. Çağıran defer ile silmelidir.
func knownHostsDosyaYaz(keys string) (string, error) {
	f, err := os.CreateTemp("", "sanalcp-knownhosts-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(keys + "\n"); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
