package wordpress

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDurumToplaParalel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "wp-content"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wp-content", ".sanal-bakim"), []byte("bakım"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	result := make(chan map[string]any, 1)
	go func() {
		result <- durumTopla(ctx, "c_test", dir, func(ctx context.Context, sk string, args ...string) ([]byte, error) {
			if sk != "c_test" || !strings.Contains(strings.Join(args, " "), "--path="+dir) {
				t.Error("tenant/path aktarılmadı")
			}
			key := strings.Join(args[:2], " ")
			started <- key
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			switch key {
			case "core version":
				return []byte(" 6.8.3\n"), nil
			case "core check-update":
				return []byte(`[{"version":"6.9"}]`), nil
			case "eval echo PHP_VERSION;":
				return []byte("8.3.0\n"), nil
			case "db size":
				return []byte("12.5\n"), nil
			default:
				t.Errorf("beklenmeyen komut: %v", args)
				return nil, errors.New("komut")
			}
		})
	}()
	// Bütün komutlar başlamadan hiçbir sonuç dönmez: seri çalışmaya dönüşmek
	// testi başarısız yapar, eşzamanlı sonuçlar ise -race ile denetlenir.
	seen := map[string]bool{}
	for range 4 {
		select {
		case key := <-started:
			seen[key] = true
		case <-ctx.Done():
			t.Fatal("komutlar paralel başlamadı")
		}
	}
	close(release)
	if len(seen) != 4 {
		t.Fatalf("komutlar eksik/tekrarlı: %v", seen)
	}
	got := <-result
	want := map[string]any{"surum": "6.8.3", "guncelleme_var": true, "hedef_surum": "6.9", "php": "8.3.0", "db_mb": "12.5", "bakim": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("durum=%v, beklenen=%v", got, want)
	}
}

func TestDurumToplaVarsayilanlar(t *testing.T) {
	for _, tc := range []struct {
		name, update string
		err          error
	}{
		{"güncelleme yok", "[]", nil},
		{"boş yanıt", "", nil},
		{"bozuk JSON", "{", nil},
		{"komut hatası", `[{"version":"6.9"}]`, errors.New("wp-cli başarısız")},
		{"iptal", "", context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := durumTopla(context.Background(), "c_test", t.TempDir(), func(_ context.Context, _ string, args ...string) ([]byte, error) {
				if args[0] == "core" && args[1] == "check-update" {
					return []byte(tc.update), tc.err
				}
				return nil, errors.New("wp-cli başarısız")
			})
			want := map[string]any{"surum": "", "guncelleme_var": false, "hedef_surum": "", "php": "", "db_mb": "", "bakim": false}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("varsayılanlar değişti: %v", got)
			}
		})
	}
}
