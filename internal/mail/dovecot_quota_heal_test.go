package mail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Önceki sürümlerle kurulmuş sunucularda DİSKTE duran userdb sorguları (kota alanı yok).
const (
	eskiUserQuery23 = `user_query = SELECT CONCAT('/home/',md.sistem_kullanici,'/mail/',m.local_part) AS home, md.uid_n AS uid, md.gid_n AS gid, CONCAT('maildir:/home/',md.sistem_kullanici,'/mail/',m.local_part) AS mail FROM mailboxes m JOIN mail_domains md ON md.id=m.mail_domain_id WHERE m.email='%u' AND m.status='active' AND md.durum='active'`
	eskiUserQuery24 = `  query = SELECT CONCAT('/home/',md.sistem_kullanici,'/mail/',m.local_part) AS home, md.uid_n AS uid, md.gid_n AS gid FROM mailboxes m JOIN mail_domains md ON md.id=m.mail_domain_id WHERE m.email='%{user}' AND m.status='active' AND md.durum='active'`
	passdbQuery24   = `  query = SELECT email AS user, password_hash AS password FROM mailboxes WHERE email='%{user}' AND status='active'`

	kotaAlani23 = ", CONCAT('*:bytes=',m.quota_bytes) AS quota_rule"
	kotaAlani24 = ", CONCAT(m.quota_bytes,'B') AS quota_storage_size"
)

func eskiSQL23() string {
	return "driver = mysql\nconnect = host=127.0.0.1 dbname=panel user=mailro password=GIZLI\n" +
		"default_pass_scheme = SHA512-CRYPT\n" + eskiUserQuery23 + "\niterate_query = SELECT email AS user FROM mailboxes WHERE status='active'\n"
}

func eskiAna24() string {
	return "protocols = imap lmtp\nmail_driver = maildir\nmail_path = %{home}\n" +
		"passdb sql {\n" + passdbQuery24 + "\n}\nuserdb sql {\n" + eskiUserQuery24 + "\n}\n"
}

func ana23() string { return "protocols = imap lmtp\nmail_location = maildir:~/\n" }

type kotaOrtam struct {
	ana, sql, dropin string
	komutlar         []string
	yenilemeSayisi   int
	dogrulaHata      error
	eklentiVar       bool
	// dogrulaninca: `doveconf -n` çalıştığı ANDA diskte ne olduğunu yakalar.
	dogrulamadaSQL, dogrulamadaDropin string
}

func yeniKotaOrtam(t *testing.T, ana, sqlIcerik string, sqlAnaDosya bool) *kotaOrtam {
	t.Helper()
	dizin := t.TempDir()
	o := &kotaOrtam{
		ana: filepath.Join(dizin, "10-sanalcp-mail.conf"), sql: filepath.Join(dizin, "dovecot-sql.conf.ext"),
		dropin: filepath.Join(dizin, "95-sanalcp-quota.conf"), eklentiVar: true,
	}
	plug := filepath.Join(dizin, "modules")
	if err := os.MkdirAll(plug, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"lib10_quota_plugin.so", "lib11_imap_quota_plugin.so"} {
		if err := os.WriteFile(filepath.Join(plug, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if sqlAnaDosya { // 2.4: sorgu ana dosyada; şifre içerir → 0640
		if err := os.WriteFile(o.ana, []byte(sqlIcerik), 0o640); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := os.WriteFile(o.ana, []byte(ana), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(o.sql, []byte(sqlIcerik), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(o.ana, map[bool]os.FileMode{true: 0o640, false: 0o644}[sqlAnaDosya]); err != nil {
		t.Fatal(err)
	}
	if !sqlAnaDosya { // WriteFile izni umask'a tabi; fixture umask'tan bağımsız olmalı
		if err := os.Chmod(o.sql, 0o640); err != nil {
			t.Fatal(err)
		}
	}

	eskiAna, eskiSQL, eskiDrop, eskiKomut, eskiYenile := dovecotSanalCPConf, dovecotSQLConf, dovecotKotaConf, dovecotKomut, dovecotYenile
	dovecotSanalCPConf, dovecotSQLConf, dovecotKotaConf = o.ana, o.sql, o.dropin
	dovecotKomut = func(_ context.Context, arglar ...string) ([]byte, error) {
		o.komutlar = append(o.komutlar, strings.Join(arglar, " "))
		if len(arglar) == 2 && arglar[0] == "-h" && arglar[1] == "mail_plugin_dir" {
			if !o.eklentiVar {
				return []byte(filepath.Join(dizin, "yok") + "\n"), nil
			}
			return []byte(plug + "\n"), nil
		}
		if len(arglar) == 1 && arglar[0] == "-n" {
			b, _ := os.ReadFile(o.sql)
			o.dogrulamadaSQL = string(b)
			b, _ = os.ReadFile(o.dropin)
			o.dogrulamadaDropin = string(b)
			return []byte("doveconf: Fatal: hata\nikinci satır"), o.dogrulaHata
		}
		return nil, errors.New("beklenmeyen komut")
	}
	dovecotYenile = func(context.Context) error { o.yenilemeSayisi++; return nil }
	t.Cleanup(func() {
		dovecotSanalCPConf, dovecotSQLConf, dovecotKotaConf, dovecotKomut, dovecotYenile = eskiAna, eskiSQL, eskiDrop, eskiKomut, eskiYenile
	})
	return o
}

func oku(t *testing.T, yol string) string {
	t.Helper()
	b, err := os.ReadFile(yol)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ---- sorgu yaması ----

func TestKotaSorgusuEkleTarihselSorgular(t *testing.T) {
	yeni, degisti, tamam := kotaSorgusuEkle(dovecot23, eskiSQL23())
	if !degisti || !tamam || !strings.Contains(yeni, "AS mail"+kotaAlani23+" FROM mailboxes m JOIN") {
		t.Fatalf("2.3 yaması: degisti=%v tamam=%v\n%s", degisti, tamam, yeni)
	}
	// Diğer satırlar (özellikle DB parolası) olduğu gibi kalır.
	if !strings.Contains(yeni, "password=GIZLI") || !strings.Contains(yeni, "iterate_query") {
		t.Errorf("yama diğer satırları bozdu:\n%s", yeni)
	}
	if _, degisti, tamam := kotaSorgusuEkle(dovecot23, yeni); degisti || !tamam {
		t.Errorf("2.3 ikinci uygulama: degisti=%v tamam=%v", degisti, tamam)
	}

	yeni24, degisti, tamam := kotaSorgusuEkle(dovecot24, eskiAna24())
	if !degisti || !tamam || !strings.Contains(yeni24, "AS gid"+kotaAlani24+" FROM mailboxes m JOIN") {
		t.Fatalf("2.4 yaması: degisti=%v tamam=%v\n%s", degisti, tamam, yeni24)
	}
	// passdb sorgusu kota alanı ALMAMALI.
	if !strings.Contains(yeni24, passdbQuery24+"\n") {
		t.Errorf("2.4 passdb sorgusu değişti:\n%s", yeni24)
	}
	if _, degisti, tamam := kotaSorgusuEkle(dovecot24, yeni24); degisti || !tamam {
		t.Errorf("2.4 ikinci uygulama: degisti=%v tamam=%v", degisti, tamam)
	}
}

func TestKotaSorgusuEkleOzellestirilmisSorguyaDokunmaz(t *testing.T) {
	ozel := "user_query = SELECT home, uid, gid FROM my_users WHERE u='%u'\n"
	if yeni, degisti, tamam := kotaSorgusuEkle(dovecot23, ozel); degisti || tamam || yeni != ozel {
		t.Errorf("özel 2.3 sorgusu değişti: %v %v %q", degisti, tamam, yeni)
	}
	bolunmus := "user_query = SELECT home, \\\n  uid FROM mailboxes m JOIN mail_domains md ON 1\n"
	if _, degisti, tamam := kotaSorgusuEkle(dovecot23, bolunmus); degisti || tamam {
		t.Errorf("satır-devamlı sorgu yamalandı: %v %v", degisti, tamam)
	}
}

// Onarım, YENİ kurulumla BİREBİR aynı sonucu üretmeli: aksi hâlde aynı
// sürümde iki farklı userdb sorgusu dolaşır.
func TestKotaYamasiTazeKurulumlaAyni(t *testing.T) {
	for _, tc := range []struct {
		ad, sablon, alan string
		surum            dovecotSurum
	}{
		{"2.3", "dovecot-sql.conf.ext.tmpl", kotaAlani23, dovecot23},
		{"2.4", "10-sanalcp-mail-2.4.conf.tmpl", kotaAlani24, dovecot24},
	} {
		taze := sablonOku(t, tc.sablon)
		if !strings.Contains(taze, tc.alan) {
			t.Fatalf("%s: şablonda kota alanı yok", tc.ad)
		}
		eski := strings.Replace(taze, tc.alan, "", 1)
		yeni, degisti, tamam := kotaSorgusuEkle(tc.surum, eski)
		if !degisti || !tamam || yeni != taze {
			t.Errorf("%s: onarılmış dosya taze kurulumdan farklı (degisti=%v tamam=%v)", tc.ad, degisti, tamam)
		}
	}
}

// Drop-in sabitleri, kurulum betiğinin kopyaladığı şablonlarla aynı olmalı.
func TestKotaDropinSabitleriSablonlaAyni(t *testing.T) {
	if dovecotKota23 != sablonOku(t, "95-sanalcp-quota.conf.tmpl") {
		t.Error("2.3 drop-in sabiti şablondan sapmış")
	}
	if dovecotKota24 != sablonOku(t, "95-sanalcp-quota-2.4.conf.tmpl") {
		t.Error("2.4 drop-in sabiti şablondan sapmış")
	}
}

// ---- onarım akışı ----

func TestHealDovecotQuota23(t *testing.T) {
	o := yeniKotaOrtam(t, ana23(), eskiSQL23(), false)

	HealDovecotQuota(context.Background())

	sql := oku(t, o.sql)
	if !strings.Contains(sql, kotaAlani23) {
		t.Fatalf("userdb sorgusuna kota eklenmedi:\n%s", sql)
	}
	if oku(t, o.dropin) != dovecotKota23 {
		t.Error("2.3 kota drop-in'i yazılmadı")
	}
	if fi, _ := os.Stat(o.sql); fi.Mode().Perm() != 0o640 {
		t.Errorf("SQL dosyasının izni korunmadı: %v", fi.Mode().Perm())
	}
	// Doğrulama, dosyalar YAZILDIKTAN sonra yapılmalı (yoksa geri almayı tetiklemez).
	if o.dogrulamadaSQL != sql || o.dogrulamadaDropin != dovecotKota23 {
		t.Error("doveconf, yeni dosyalar yazılmadan çalıştı")
	}
	if o.yenilemeSayisi != 1 {
		t.Errorf("reload sayısı=%d, 1 bekleniyordu", o.yenilemeSayisi)
	}

	// İdempotent: ikinci açılış hiçbir şey yazmaz, reload etmez.
	komutlar := len(o.komutlar)
	HealDovecotQuota(context.Background())
	if o.yenilemeSayisi != 1 || len(o.komutlar) != komutlar {
		t.Errorf("ikinci çalıştırma iş yaptı: reload=%d komut=%v", o.yenilemeSayisi, o.komutlar[komutlar:])
	}
	if oku(t, o.sql) != sql {
		t.Error("ikinci çalıştırma SQL dosyasını değiştirdi")
	}
}

func TestHealDovecotQuota24(t *testing.T) {
	o := yeniKotaOrtam(t, "", eskiAna24(), true)

	HealDovecotQuota(context.Background())

	ana := oku(t, o.ana)
	if !strings.Contains(ana, kotaAlani24) {
		t.Fatalf("2.4 userdb sorgusuna kota eklenmedi:\n%s", ana)
	}
	if oku(t, o.dropin) != dovecotKota24 {
		t.Error("2.4 kota drop-in'i yazılmadı")
	}
	// DB parolası içeren dosya: izin (0640) korunmalı.
	if fi, _ := os.Stat(o.ana); fi.Mode().Perm() != 0o640 {
		t.Errorf("ana dosya izni korunmadı: %v", fi.Mode().Perm())
	}
	if _, err := os.Stat(o.sql); err == nil {
		t.Error("2.4 kurulumunda 2.3 SQL dosyası oluşturuldu")
	}
	if o.yenilemeSayisi != 1 {
		t.Errorf("reload sayısı=%d", o.yenilemeSayisi)
	}
	HealDovecotQuota(context.Background())
	if o.yenilemeSayisi != 1 || oku(t, o.ana) != ana {
		t.Error("2.4 ikinci çalıştırma idempotent değil")
	}
}

// 🔴 Doğrulama başarısızsa HER İKİ dosya eski haline dönmeli ve Dovecot'a dokunulmamalı.
func TestHealDovecotQuotaDogrulamaBasarisizsaGeriAlir(t *testing.T) {
	for _, tc := range []struct {
		ad, ana, sql string
		sqlAna       bool
	}{
		{"2.3", ana23(), eskiSQL23(), false},
		{"2.4", "", eskiAna24(), true},
	} {
		t.Run(tc.ad, func(t *testing.T) {
			o := yeniKotaOrtam(t, tc.ana, tc.sql, tc.sqlAna)
			o.dogrulaHata = errors.New("exit status 89")
			sqlYol := o.sql
			if tc.sqlAna {
				sqlYol = o.ana
			}

			HealDovecotQuota(context.Background())

			if oku(t, sqlYol) != tc.sql {
				t.Error("userdb dosyası eski haline dönmedi")
			}
			if _, err := os.Stat(o.dropin); err == nil {
				t.Error("kota drop-in'i geri alınmadı")
			}
			if o.yenilemeSayisi != 0 {
				t.Error("geçersiz yapılandırma varken Dovecot reload edildi")
			}
		})
	}
}

func TestHealDovecotQuotaEklentiYoksaDokunmaz(t *testing.T) {
	o := yeniKotaOrtam(t, ana23(), eskiSQL23(), false)
	o.eklentiVar = false

	HealDovecotQuota(context.Background())

	if oku(t, o.sql) != eskiSQL23() {
		t.Error("eklenti yokken SQL dosyası değişti")
	}
	if _, err := os.Stat(o.dropin); err == nil {
		t.Error("eklenti yokken drop-in yazıldı")
	}
	if o.yenilemeSayisi != 0 {
		t.Error("reload edildi")
	}
}

func TestHealDovecotQuotaOzellestirilmisSorguyaDokunmaz(t *testing.T) {
	ozel := "driver = mysql\nuser_query = SELECT home, uid, gid FROM my_users WHERE u='%u'\n"
	o := yeniKotaOrtam(t, ana23(), ozel, false)

	HealDovecotQuota(context.Background())

	if oku(t, o.sql) != ozel {
		t.Error("tanınmayan sorgu değiştirildi")
	}
	// Limit userdb'den gelmiyorsa eklentiyi açmanın anlamı yok.
	if _, err := os.Stat(o.dropin); err == nil {
		t.Error("sorgu yamalanamamışken drop-in yazıldı")
	}
}

func TestHealDovecotQuotaYabanciKurulumaDokunmaz(t *testing.T) {
	o := yeniKotaOrtam(t, ana23(), eskiSQL23(), false)
	if err := os.Remove(o.ana); err != nil { // SanalCP drop-in'i yok
		t.Fatal(err)
	}

	HealDovecotQuota(context.Background())

	if oku(t, o.sql) != eskiSQL23() || len(o.komutlar) != 0 || o.yenilemeSayisi != 0 {
		t.Error("SanalCP kurulumu olmayan Dovecot'a dokunuldu")
	}
}

// Yarım kalmış durum: drop-in var (elle düzenlenmiş olabilir) ama sorgu yamalı
// değil → yalnız sorgu tamamlanır, mevcut drop-in EZİLMEZ.
func TestHealDovecotQuotaMevcutDropinEzilmez(t *testing.T) {
	o := yeniKotaOrtam(t, ana23(), eskiSQL23(), false)
	ozelDropin := "# elle düzenlendi\nplugin {\n  quota = maildir:User quota\n}\n"
	if err := os.WriteFile(o.dropin, []byte(ozelDropin), 0o644); err != nil {
		t.Fatal(err)
	}

	HealDovecotQuota(context.Background())

	if oku(t, o.dropin) != ozelDropin {
		t.Error("mevcut drop-in ezildi")
	}
	if !strings.Contains(oku(t, o.sql), kotaAlani23) || o.yenilemeSayisi != 1 {
		t.Error("sorgu tamamlanmadı")
	}
}

// ---- şablon çıktıları ----

// Her iki Dovecot sürümü için userdb, kutunun limitini DOĞRU alan adıyla
// döndürmeli (2.3 quota_rule, 2.4 quota_storage_size) ve eklentiyi açan
// drop-in kendi sürümünün sözdiziminde olmalı.
func TestKotaSablonlari(t *testing.T) {
	sql23 := yalnizAyarSatirlari(sablonOku(t, "dovecot-sql.conf.ext.tmpl"))
	if !strings.Contains(sql23, "CONCAT('*:bytes=',m.quota_bytes) AS quota_rule") {
		t.Error("2.3 user_query kutu limitini quota_rule olarak döndürmüyor")
	}
	ana24 := yalnizAyarSatirlari(sablonOku(t, "10-sanalcp-mail-2.4.conf.tmpl"))
	if !strings.Contains(ana24, "CONCAT(m.quota_bytes,'B') AS quota_storage_size") {
		t.Error("2.4 userdb sorgusu kutu limitini quota_storage_size olarak döndürmüyor")
	}
	// passdb sorgusu limit taşımaz (prefetch yok; userdb ayrı sorgulanır).
	if strings.Count(ana24, "quota_storage_size") != 1 {
		t.Error("2.4: quota_storage_size yalnız userdb sorgusunda olmalı")
	}

	k23 := yalnizAyarSatirlari(sablonOku(t, "95-sanalcp-quota.conf.tmpl"))
	for _, gerekli := range []string{"quota = maildir:User quota", "mail_plugins = $mail_plugins quota imap_quota", "protocol lmtp", "lmtp_rcpt_check_quota = yes"} {
		if !strings.Contains(k23, gerekli) {
			t.Errorf("2.3 kota drop-in'inde %q yok", gerekli)
		}
	}
	for _, olmamali := range []string{"mail_plugins {", "quota \"User quota\"", "storage_grace"} {
		if strings.Contains(k23, olmamali) {
			t.Errorf("2.3 kota drop-in'inde 2.4'e ait %q var", olmamali)
		}
	}

	k24 := yalnizAyarSatirlari(sablonOku(t, "95-sanalcp-quota-2.4.conf.tmpl"))
	for _, gerekli := range []string{"mail_plugins {", "quota = yes", "imap_quota = yes", "quota \"User quota\"", "driver = maildir", "lmtp_rcpt_check_quota = yes"} {
		if !strings.Contains(k24, gerekli) {
			t.Errorf("2.4 kota drop-in'inde %q yok", gerekli)
		}
	}
	for _, olmamali := range []string{"plugin {", "quota = maildir", "mail_plugins = ", "quota_grace"} {
		if strings.Contains(k24, olmamali) {
			t.Errorf("2.4 kota drop-in'inde 2.3'e ait %q var", olmamali)
		}
	}
}
