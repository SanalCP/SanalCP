package mail

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"sanalcp/internal/dns"
	"sanalcp/internal/jailpath"
)

// gerekliServisler: e-postanın FİİLEN çalışması için ayakta olması gereken
// servisler. rspamd/opendkim KASITLI olarak listede değil — onlar olmadan da
// posta akar (yalnız spam filtresi/DKIM imzası olmaz), postfix'in milter
// ayarı milter_default_action=accept (bkz. assets/ops/sanalcp-mail-setup.sh).
var gerekliServisler = []string{"postfix", "dovecot"}

// MailAltyapisiVar: e-posta yığını sunucuda kurulu VE çalışıyor mu?
//
// 🔴 NEDEN GEREKLİ: MailUygula yalnızca DB satırı + maildir + DNS kaydı üretir;
// paket kurmaz, servis başlatmaz. Sunucu yöneticisi mail yığınını kapatmışsa
// (tek müşteri kalmayınca RAM için makul bir tercih) panel "etkinleştirildi"
// deyip DNS'e MX/SPF/DKIM yazar ama posta HİÇ çalışmazdı — sessiz arıza.
// Kullanıcının bunu ancak gönderilen postalar kaybolunca fark etmesi gerekirdi.
//
// eksik: kullanıcıya gösterilecek servis adları (boşsa altyapı hazırdır).
func MailAltyapisiVar(ctx context.Context) (eksik []string) {
	for _, s := range gerekliServisler {
		if !servisAktif(ctx, s) {
			eksik = append(eksik, s)
		}
	}
	return eksik
}

// servisAktif: testlerde değiştirilebilsin diye değişken (systemd'ye bağımlı
// bir kontrolü birim testinde çalıştırmak, ortama göre farklı sonuç verirdi).
var servisAktif = func(ctx context.Context, ad string) bool {
	out, err := exec.CommandContext(ctx, "systemctl", "is-active", ad).Output()
	return err == nil && strings.TrimSpace(string(out)) == "active"
}

// MailUygula: bir domain için maili etkinleştirir (idempotent) — mail_domains satırını
// oluşturur/günceller, Maildir kök dizinini tenant kullanıcısına ait olarak hazırlar,
// DNS varsayılanlarını (MX/SPF/DMARC/DKIM) tohumlar. provisioner.WAFUygula ile aynı
// "küçük, tek-amaçlı, domain-create/plan-değişimi/açık-eylemden çağrılan" şeklini izler.
func MailUygula(ctx context.Context, db *sql.DB, domainID int64) error {
	var alanAdi, sk, ipv4 string
	if err := db.QueryRowContext(ctx,
		`SELECT alan_adi, sistem_kullanici, COALESCE(ipv4,'') FROM domains WHERE id=?`, domainID).
		Scan(&alanAdi, &sk, &ipv4); err != nil {
		return fmt.Errorf("domain bulunamadı: %w", err)
	}
	uid, gid, err := uidGid(sk)
	if err != nil {
		return fmt.Errorf("linux kullanıcı bulunamadı (%s): %w", sk, err)
	}
	maildirRoot := filepath.Join("/home", sk, "mail")
	if err := os.MkdirAll(maildirRoot, 0o750); err != nil {
		return fmt.Errorf("maildir kök dizini: %w", err)
	}
	_ = os.Chown(maildirRoot, uid, gid)

	if _, err := db.ExecContext(ctx,
		`INSERT INTO mail_domains(domain_id, alan_adi, sistem_kullanici, uid_n, gid_n, maildir_root)
		 VALUES(?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE sistem_kullanici=VALUES(sistem_kullanici), uid_n=VALUES(uid_n),
		   gid_n=VALUES(gid_n), maildir_root=VALUES(maildir_root), durum='active'`,
		domainID, alanAdi, sk, uid, gid, maildirRoot); err != nil {
		return fmt.Errorf("mail_domains kayıt: %w", err)
	}

	// MX/SPF/DMARC/DKIM domain oluşturulurken zaten SeedDefaults ile tohumlanmış olabilir
	// (idempotent COUNT(*) guard'lı, dns.go:338). Mail domain oluşturulduktan SONRA
	// etkinleştirilmişse burada tekrar çağırmak eksik kalan kayıtları/DKIM anahtarını tamamlar.
	if _, err := dns.SeedDefaults(ctx, db, domainID, alanAdi, ipv4); err != nil {
		log.Printf("mail: dns.SeedDefaults(%s): %v", alanAdi, err)
	}
	if err := dns.WriteZone(ctx, db, domainID); err != nil {
		log.Printf("mail: dns.WriteZone(%s): %v", alanAdi, err)
	}
	return nil
}

// MailKaldir: domain için maili DEVRE DIŞI bırakır (soft-disable) — mailboxes satırları
// SİLİNMEZ, sadece mail_domains.durum='suspended' olur. Postfix/Dovecot SQL sorguları
// zaten "durum/status='active'" filtrelediği için bu tek UPDATE anında hem gelen postayı
// reddeder hem SMTP AUTH'u keser — servis restart GEREKMEZ.
func MailKaldir(ctx context.Context, db *sql.DB, domainID int64) error {
	_, err := db.ExecContext(ctx, `UPDATE mail_domains SET durum='suspended' WHERE domain_id=?`, domainID)
	return err
}

// TumunuKaldir: MailUygula'nın YIKICI karşıtı — domain için e-posta hizmetini
// tamamen kaldırır. MailKaldir (soft-disable) ile karıştırılmamalı: burada
// posta kutuları, yönlendiriciler, filtreler, otomatik yanıtlar ve DİSKTEKİ
// posta dosyaları GERİ DÖNÜŞSÜZ silinir.
//
// SİLME SIRASI ÖNEMLİ — önce DB, sonra disk:
// DB silinip disk silinemezse geriye yalnız sahipsiz dosyalar kalır ve hizmet
// kapalıdır (güvenli taraf). Tersi olsaydı (disk gidip DB kalsaydı) Dovecot
// var olmayan maildir'lere bakıp hata verirdi.
//
// CASCADE HARİTASI (bkz. migrations/0040_mail.sql, 0052, 0054):
//
//	mail_domains  -> mailboxes -> mail_autoresponders, mail_filters   (otomatik)
//	mail_aliases, mail_send_log, mail_spam_settings                   (domains'e
//	  bağlı, mail_domains'e DEĞİL → cascade OLMAZ, elle silinir)
//
// DNS'e (MX/SPF/DKIM/DMARC) KASITLI OLARAK DOKUNULMAZ: kullanıcı MX'i harici bir
// sağlayıcıya (ör. Google Workspace) çevirmiş olabilir ve o kayıtlar DNS
// sayfasından ayrıca yönetiliyor. Sessizce silmek, bırakmaktan daha kötü olurdu.
//
// diskHata nil değilse DB temizliği BAŞARILI olmuştur, yalnız dosya silme
// kısmen/tamamen başarısızdır — çağıran bunu kullanıcıya bildirmelidir.
func TumunuKaldir(ctx context.Context, db *sql.DB, domainID int64) (diskHata error, err error) {
	var sk string
	if err := db.QueryRowContext(ctx,
		`SELECT sistem_kullanici FROM domains WHERE id=?`, domainID).Scan(&sk); err != nil {
		return nil, fmt.Errorf("domain bulunamadı: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("işlem başlatılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// mail_domains'e cascade ETMEYENLER (hepsi domains(id)'e bağlı).
	for _, q := range []string{
		`DELETE FROM mail_aliases WHERE domain_id=?`,
		`DELETE FROM mail_send_log WHERE domain_id=?`,
		`DELETE FROM mail_spam_settings WHERE domain_id=?`,
	} {
		if _, err := tx.ExecContext(ctx, q, domainID); err != nil {
			return nil, fmt.Errorf("mail kayıtları silinemedi: %w", err)
		}
	}
	// Bu satırın silinmesi mailboxes'ı, o da autoresponder/filter'ları düşürür.
	if _, err := tx.ExecContext(ctx, `DELETE FROM mail_domains WHERE domain_id=?`, domainID); err != nil {
		return nil, fmt.Errorf("mail_domains silinemedi: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("işlem tamamlanamadı: %w", err)
	}

	// Disk: /home/<sk>/mail. 🔴 Panel ROOT çalışıyor — tenant'ın ev dizininde YOL
	// tabanlı silme (os.RemoveAll) yapılamaz: tenant "mail"i /etc'ye bakan bir
	// symlink'le değiştirip jail dışında silme yaptırabilirdi. jailpath.Sil
	// fd-göreli ve symlink takip etmeyen bir silme yapar (bkz. internal/jailpath).
	home, herr := jailpath.TenantHome(sk)
	if herr != nil {
		return fmt.Errorf("ev dizini bulunamadı (%s): %w", sk, herr), nil
	}
	if serr := jailpath.Sil(home, "mail"); serr != nil && !os.IsNotExist(serr) {
		return fmt.Errorf("posta dosyaları silinemedi: %w", serr), nil
	}
	return nil, nil
}

// KapatDomain: domain SİLİNİRKEN çağrılır (domains.Delete, redis.KapatDomain ile aynı
// noktadan). mail_domains/mailboxes/mail_aliases hepsi domains(id)'e ON DELETE CASCADE
// FK ile bağlı, yani DB satırları zaten otomatik silinir — bu fonksiyon bugün no-op,
// yalnızca cascade-DIŞI bir yan etki (ör. ileride bir servis reload'u) gerekirse diye
// aynı çağrı noktasını (ve simetriyi) koruyan bir genişletme yeri.
func KapatDomain(db *sql.DB, domainID int64, sk string) {}

func uidGid(u string) (int, int, error) {
	uu, err := user.Lookup(u)
	if err != nil {
		return 0, 0, err
	}
	uid, err := strconv.Atoi(uu.Uid)
	if err != nil {
		return 0, 0, err
	}
	gid, err := strconv.Atoi(uu.Gid)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}
