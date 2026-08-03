package auth

import "testing"

// Referans hash'ler yerelde `openssl passwd -1/-5/-6 -salt ...` ile üretildi
// (glibc'nin crypt(3) uygulamasıyla aynı algoritma) — önceden python3'ün
// `crypt` modülüne shell-out ederek doğrulanan yol, artık native
// github.com/GehirnInc/crypt kullanıyor; bu vektörler o değişimin glibc ile
// aynı sonucu ürettiğini kanıtlar.
func TestLegacyCryptDogrula(t *testing.T) {
	vakalar := []struct {
		ad     string
		parola string
		hash   string
	}{
		{"sha512 düz", "Hello world!", "$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1"},
		{"sha256 düz", "Hello world!", "$5$saltstring$5B8vYYiY.CVt1RlTTf8KbXBH3hsxY/GNooZaBBGWEc5"},
		{"md5crypt", "test", "$1$abcdefgh$irWbblnpmw.5z7wgBnprh0"},
		{"sha512 rounds=", "somepassword", "$6$rounds=5000$myawesomesalt$oi4ab666Oh4IeDdLzFKa0z.PHI2obvIG3TLOsvFSzfUq6plBrX4Lt2m3suhtRBtggU.SopmheoFlpYmXRLnv21"},
		{"sha256 rounds= + unicode parola", "Türkçe Şifre 123!", "$5$rounds=12000$anothersalt12345$Im9DbDr4rT0/o1WJ557mJ7DliozPOYUafR5grKgUUBB"},
		{"md5crypt basit", "password", "$1$12345678$o2n/JiO/h5VviOInWJ4OQ/"},
		{"sha512 200 karakterlik parola", string(make200As()), "$6$longpwsalt$H0PpC2mP68LSYXV/RMQOIY1jdYsPeRK3Xiys9jtCUqZcF6pxeLnGerVPwygBsnx8ODVjKIRc3aWNuss1DJKLa/"},
		{"sha512 özel karakterler", `p@\$\$w0rd!#%^&*()`, "$6$specialsalt$qX2ufE/n5Gjsu4uCahy0UJWd3Up11/H3NOnJMcWBEmtK/IhMqp7X9jkBiNv7wEGNn49jq2PWFHDUkiE7261tf."},
	}
	for _, v := range vakalar {
		t.Run(v.ad, func(t *testing.T) {
			if !legacyCryptDogrula(v.parola, v.hash) {
				t.Errorf("doğru parola reddedildi (hash=%s)", v.hash)
			}
			if legacyCryptDogrula(v.parola+"YANLIS", v.hash) {
				t.Errorf("yanlış parola kabul edildi (hash=%s)", v.hash)
			}
		})
	}
}

func TestLegacyCryptDogrulaDesteklenmeyenFormat(t *testing.T) {
	// crypt.NewFromHash tanınmayan prefix'te panic atar — legacyCryptDogrula
	// bunu IsHashSupported ile önceden eleyip false dönmeli, panic YAYMAMALI.
	desteklenmeyenler := []string{
		"", "duzmetin", "$y$foo", "$argon2id$v=19$...", "$bariz-bilinmeyen$xyz",
	}
	for _, h := range desteklenmeyenler {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("hash=%q panic attı: %v", h, r)
				}
			}()
			if legacyCryptDogrula("herhangi", h) {
				t.Errorf("hash=%q için beklenmedik kabul", h)
			}
		}()
	}
}

func make200As() []byte {
	b := make([]byte, 200)
	for i := range b {
		b[i] = 'a'
	}
	return b
}
