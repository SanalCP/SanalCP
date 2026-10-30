package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// migrationIfadeleri: SQL dosyasını ifadelere bölerken tırnak/yorum
// farkındalığını korumalı. Bu test yoksa literali içinde ';' olan bir INSERT
// (ör. seed verisi "a;b") üretimde iki ifadeye bölünür ve migration patlar.
func TestMigrationIfadeleri(t *testing.T) {
	testler := []struct {
		ad     string
		govde  string
		beklen []string
	}{
		{
			ad:     "basit bölme",
			govde:  "CREATE TABLE t (id INT);\nINSERT INTO t VALUES (1);",
			beklen: []string{"CREATE TABLE t (id INT)", "INSERT INTO t VALUES (1)"},
		},
		{
			ad:     "literal içinde noktalı virgül bölünmez",
			govde:  "INSERT INTO t VALUES ('a;b');",
			beklen: []string{"INSERT INTO t VALUES ('a;b')"},
		},
		{
			ad:     "çiftleme kaçışı literal içinde tutulur",
			govde:  "INSERT INTO t VALUES ('it''s;ok');SELECT 1",
			beklen: []string{"INSERT INTO t VALUES ('it''s;ok')", "SELECT 1"},
		},
		{
			ad:     "backslash kaçışı tırnağı kapatmaz",
			govde:  `INSERT INTO t VALUES ('don\'t;stop');SELECT 1`,
			beklen: []string{`INSERT INTO t VALUES ('don\'t;stop')`, "SELECT 1"},
		},
		{
			ad:     "satır içi yorum ifadeyi kirletmez",
			govde:  "CREATE TABLE t (id INT); -- açıklama; burada\nINSERT INTO t VALUES (1);",
			beklen: []string{"CREATE TABLE t (id INT)", "INSERT INTO t VALUES (1)"},
		},
		{
			ad:     "yorum içindeki tırnak literal açmaz",
			govde:  "CREATE TABLE t (\n  ad VARCHAR(64) -- makine adı: 'ai'\n);\nINSERT INTO t VALUES ('x');",
			beklen: []string{"CREATE TABLE t (\n  ad VARCHAR(64)\n)", "INSERT INTO t VALUES ('x')"},
		},
		{
			ad:     "blok yorum atlanır",
			govde:  "/* bu; yorum */\nSELECT 1; /* yorum; daha */ SELECT 2",
			beklen: []string{"SELECT 1", "SELECT 2"},
		},
		{
			ad:     "çalıştırılabilir blok yorum korunur",
			govde:  "/*!40101 SET NAMES utf8 */;",
			beklen: []string{"/*!40101 SET NAMES utf8 */"},
		},
		{
			ad:     "boş ve yorum-only parçalar elenir",
			govde:  "-- başlık\n\n-- ikinci yorum\n",
			beklen: nil,
		},
		{
			ad:     "backtick içinde noktalı virgül bölünmez",
			govde:  "SELECT `a;b` FROM t;SELECT 2",
			beklen: []string{"SELECT `a;b` FROM t", "SELECT 2"},
		},
		{
			ad:     "sonradan noktalı virgülsüz ifade de alınır",
			govde:  "SELECT 1;\nSELECT 2",
			beklen: []string{"SELECT 1", "SELECT 2"},
		},
	}
	for _, tc := range testler {
		t.Run(tc.ad, func(t *testing.T) {
			got := migrationIfadeleri(tc.govde)
			if !reflect.DeepEqual(got, tc.beklen) {
				t.Errorf("migrationIfadeleri:\n  gelen:   %#v\n  beklen:  %#v", got, tc.beklen)
			}
		})
	}
}

// TestMigrasyonDosyalariTemizAyrisir: depodaki TÜM migration dosyaları ayrıştırılınca
// boş/yorum-kalıntısı ifade çıkmamalı. Gelecekteki bir dosyada ayrıştırıcıyı
// bozacak içerik (ör. yorum içinde gömülü ';') buraya takılır.
func TestMigrasyonDosyalariTemizAyrisir(t *testing.T) {
	dosyalar, err := filepath.Glob("../../migrations/*.sql")
	if err != nil || len(dosyalar) == 0 {
		t.Fatalf("migrations/*.sql bulunamadı: %v", err)
	}
	for _, yol := range dosyalar {
		govde, err := os.ReadFile(yol)
		if err != nil {
			t.Fatalf("%s okunamadı: %v", yol, err)
		}
		ifadeler := migrationIfadeleri(string(govde))
		if len(ifadeler) == 0 {
			t.Errorf("%s hiçbir ifade üretmedi", yol)
		}
		for _, s := range ifadeler {
			if strings.HasPrefix(s, "--") || strings.HasPrefix(s, "#") {
				t.Errorf("%s: yorum kalıntısı ifade üretildi: %.60q", yol, s)
			}
		}
	}
}
