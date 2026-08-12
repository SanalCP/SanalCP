package wordpress

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gercekWPConfig — wp-cli 2.12'nin `wp config create` ile ürettiği gerçek
// biçim (boşluklar dahil birebir kopya). Regex bu biçme karşı doğrulanır.
const gercekWPConfig = `<?php
/** Database name */
define( 'DB_NAME', 'wp_e2e' );

/** Database username */
define( 'DB_USER', 'wpu_e2e' );

/** Database password */
define( 'DB_PASSWORD', 'Db7RtQm2Xk9PwZa4Nc6V' );

/** Database hostname */
define( 'DB_HOST', 'localhost' );

$table_prefix = 'wp_';
`

func yazWPConfig(t *testing.T, icerik string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wp-config.php"), []byte(icerik), 0o600); err != nil {
		t.Fatalf("wp-config.php yazılamadı: %v", err)
	}
	return dir
}

func TestWPConfigDBParolaDogrula_DogruParola(t *testing.T) {
	dir := yazWPConfig(t, gercekWPConfig)
	if err := wpConfigDBParolaDogrula(dir, "Db7RtQm2Xk9PwZa4Nc6V"); err != nil {
		t.Fatalf("doğru parola reddedildi: %v", err)
	}
}

// TestWPConfigDBParolaDogrula_BosParola — asıl regresyon: wp-cli tanımadığı bir
// --prompt=<ad> argümanını SESSİZCE yok sayar (exit 0, stderr boş) ve parolayı
// boş yazar. Bu fonksiyon o durumu yakalamak için var.
func TestWPConfigDBParolaDogrula_BosParola(t *testing.T) {
	bos := strings.Replace(gercekWPConfig,
		"define( 'DB_PASSWORD', 'Db7RtQm2Xk9PwZa4Nc6V' );",
		"define( 'DB_PASSWORD', '' );", 1)
	dir := yazWPConfig(t, bos)
	if err := wpConfigDBParolaDogrula(dir, "Db7RtQm2Xk9PwZa4Nc6V"); err == nil {
		t.Fatal("boş DB_PASSWORD kabul edildi — sessiz --prompt yok sayımı yakalanamazdı")
	}
}

func TestWPConfigDBParolaDogrula_YanlisParola(t *testing.T) {
	dir := yazWPConfig(t, gercekWPConfig)
	if err := wpConfigDBParolaDogrula(dir, "BaskaBirParola"); err == nil {
		t.Fatal("yanlış parola kabul edildi")
	}
}

func TestWPConfigDBParolaDogrula_TanimYok(t *testing.T) {
	dir := yazWPConfig(t, "<?php\n$table_prefix = 'wp_';\n")
	if err := wpConfigDBParolaDogrula(dir, "x"); err == nil {
		t.Fatal("DB_PASSWORD tanımı olmayan dosya kabul edildi")
	}
}

func TestWPConfigDBParolaDogrula_DosyaYok(t *testing.T) {
	if err := wpConfigDBParolaDogrula(t.TempDir(), "x"); err == nil {
		t.Fatal("olmayan wp-config.php kabul edildi")
	}
}
