# Acil güvenlik düzeltmeleri — 2026-09-18

Sürüm: **0.9.67**. Kapsam: SQL içe aktarma ve tenant dosya izolasyonu güvenlik düzeltmeleri. Kurulu sunucuların bu sürüme güncellenmesi gerekir.

## SQL içe aktarma

- `sqlimport` istemcisinde `--binary-mode` ile yerel shell/source komutları, `--local-infile=0` ile istemcinin yerel dosya okuması kapatıldı.
- Yalnız üretilen credentials dosyası okunuyor (`--defaults-file` ilk seçenek); kullanıcı login dosyası ve ortamdan gelen sırlar dışlandı. `root` hedef DB kullanıcısı reddediliyor.
- Yedek geri yükleme/recovery artık domain ve DB ile sınırlandırılmış `db_accounts` kaydından kimlik çözüyor; root DB bağlantısına geri düşmüyor.
- Yedek doğrulama ortak güvenli importer'ı kullanıyor. Transfer importu yeni tenant'ın DB kimliğini kullanıyor; CREATE DATABASE/USE süzmesi yalnız uyumluluk için korunuyor.
- DB yeniden adlandırmadaki sunucu üretimi dump akışına da CLI/local-infile koruması eklendi.
- İstemci süreci hâlâ panelin işletim sistemi kimliğini kullanır; bu düzeltmede CLI komutları ve local infile kapatılmış, SQL yetkisi tenant DB hesabına indirilmiştir. DB hesabı birden çok şemaya yetkiliyse sınır o hesabın grant'leridir.

## Tenant dosya işlemleri

- Staging/canlı kopyalamada root `rsync --delete` ve root recursive chown kaldırıldı.
- Kaynak ve hedef dizinler openat2 ile açılıp dosya tanıtıcısıyla sabitleniyor. Kaynak tar işlemi kaynak UID/GID'siyle, hedef çıkarma hedef UID/GID'siyle çalışıyor; root ek grupları aktarılmıyor.
- Kaynak başarıyla özel bir geçici arşive alınmadan hedef temizlenmiyor. Symlink hedefleri okunmuyor; kopyalanan linkler link olarak kalıyor.
- Uygulama paketleri root'a özel geçici dizinde açılıyor; tenant dizinine yayınlama tenant kimliğiyle yapılıyor. Drupal, Joomla, PrestaShop, OpenCart, Grav, Matomo, Nextcloud, phpBB ve MediaWiki aynı yolu kullanıyor.
- Ortak uygulama/WordPress kurulum hazırlığı ve hata temizliği fd tabanlı jailpath işlemlerine geçirildi. WordPress bakım dosyaları ve kurucu yardımcı dosyaları da güvenli yazma/silme yolunu kullanıyor.
- Restore/transfer sonrasındaki root recursive metadata işlemleri kaldırıldı veya tenant yetkisine indirildi. Nextcloud veri dizini yalnız yeni bir dizin olarak, 0750 izinle oluşturuluyor.

## Doğrulama

- `go test ./...`
- `go vet ./...`
- `go test -race ./internal/jailpath ./internal/sqlimport ./internal/backups ./internal/transfers`
- `go build -o /tmp/sanalcp-security-p0-check ./cmd/server`
- `SANALCP_SQLIMPORT_IT=1 go test ./internal/sqlimport ./internal/backups -run Entegrasyon -v`
- `SANALCP_TENANT_IT=1 go test ./internal/jailpath -run 'TestEntegrasyonTenantCopyPinned|TestTenantCommand' -v`

Gerçek MariaDB testleri shell/source/local-infile reddini, normal dump/view/trigger/procedure importunu, hedef dışı SQL reddini ve yedek doğrulama yolunu kapsar. Tenant testleri iki geçici sistem kullanıcısını oluşturup temizler; farklı UID'ler arasında 0600 dosya aktarımı, sahiplik, eski dosyaların silinmesi, linklerin korunması ve işlem sırasında kaynak/hedef yolunun dış dizine symlink olarak değiştirilmesini kapsar. Paket yayınında aynı yol değiştirme ve web root izinleri kontrol edilir.

## Operasyonel etkiler

- Staging kopyalama geçici sıkıştırılmamış arşiv için ek disk alanı kullanır. Alan yetersizliği kaynak hazırlığında hata verir; hedef o aşamada temizlenmez.
- Eski yedeklerin DB kimliği panel metadata'sında yoksa restore artık güvenli biçimde reddedilir; root ile deneme yapılmaz. Eksik metadata onarılmalıdır.
- SELinux etkin gerçek dağıtımda normal kurulum/restore akışının ayrıca smoke testi gerekir; bu ortamda SELinux etiket politikası doğrulanmadı.
- Derlenen yeni backend dağıtılıp yeniden başlatılmadan çalışan servis bu değişiklikleri kullanmaz.
