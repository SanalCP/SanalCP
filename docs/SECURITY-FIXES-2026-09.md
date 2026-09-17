# Eylül 2026 güvenlik düzeltmeleri

Bu düzeltmeler v0.9.66 sürümünde yer alır ve v0.9.65 kaynak kodu üzerinde
hazırlanmıştır. GitHub yayını, mevcut canlı sunucuların otomatik olarak
güncellendiği anlamına gelmez.

## Git dağıtımı

Deploy anahtarı bellekte üretilir. `.ssh` dizini sembolik bağ ise işlem reddedilir;
dosya yazımları açık dizin tanıtıcısı üzerinden yeni inode + atomik rename ile
yapılır. Var olan geçerli özel anahtar değiştirilmez. Özel anahtar sembolik bağ,
birden fazla hardlink, özel dosya veya başka bir kullanıcıya ait dosya ise
kullanılmaz. Git/restorecon yalnız tenant kimliği ve temiz ortamla çalışır.

SSH `StrictHostKeyChecking=yes` ve `BatchMode=yes` seçenekleri komut satırında
zorunludur; eski `.ssh/config` içindeki doğrulamayı kapatan ayarlar bu seçenekleri
geçersiz kılamaz. GitHub Ed25519 sunucu anahtarı
[GitHub'ın resmî meta verisinden](https://api.github.com/meta) alınmıştır ve
`.ssh/sanalcp_known_hosts` içinde hazırlanır. Anahtar değişirse doğrulama başarısız
olur; yeni anahtar güvenilir kanaldan doğrulanıp yazılımda güncellenmelidir.

GitHub dışındaki SSH sunucuları için doğrulanmış sunucu anahtarını tenant'ın
`.ssh/known_hosts` dosyasına veya sistemin `/etc/ssh/ssh_known_hosts` dosyasına
ekleyin. Standart dışı portlarda OpenSSH `[sunucu]:port` biçimini kullanın.
Anahtarı kontrol etmeden `ssh-keyscan` çıktısına güvenmeyin. HTTPS depolarında
sunucu anahtarı kaydı gerekmez.

## Dosya izinleri

Özyinelemeli chmod, `O_NOFOLLOW` ile açılmış dosyaya `Fstat` ve `Fchmod` uygular.
Dosya adının kontrol ile işlem arasında değiştirilmesi dışarıdaki bir hedefin
izinlerini değiştiremez. Sembolik bağlar ve özel dosyalar atlanır. Sonraki ACL
komutları fiziksel ağaç yürüyüşü (`setfacl -P`) ve tenant kimliğiyle çalışır.

## Şifreli dizinler ve mevcut kurulumlar

Yeni dosya adı `v2_d<domain-id>_<yolun-SHA256-değeri>` biçimindedir. `/a-b`,
`/a/b`, `/a_b` gibi yollar farklı parola dosyalarına sahiptir.

Panel başlangıcında eski ortak dosyalardaki kullanıcılar veritabanındaki
dizin/kullanıcı eşleşmelerine göre ayrılır ve nginx yapılandırmaları yenilenir.
Eski dosyalar geçiş sırasında boşaltılır; eski nginx worker'ları ortak
parolaları kabul etmeyi bırakır. Yeniden başlatma, tamamlanmamış nginx
yenilemesini tekrar dener. Yeni dosyalardaki mevcut parolalar korunur.

Aynı kullanıcı adı çakışan farklı dizinlerde kullanılmışsa eski dosyada yalnız
son yazılan hash vardır. Güvenle geri kazanılamayan bu kayıtlar `!` hash'iyle
kilitlenir. Yönetici ilgili dizinlerde kullanıcı parolalarını yeniden atamalıdır.
Eksik eski dosyalardaki kayıtlar da kilitlenir. Bu durum sunucu günlüğüne yazılır;
dizin koruması kaldırılmaz. Göç başarısız olursa panel API'si açılmaz, hata
günlüğe yazılır.

## Disk ölçümü

Domain disk hesaplama ve kaynak kullanımı uçları aynı ölçüm servisini kullanır:
60 saniye önbellek, aynı yol/ölçüm türü için bir iş, en fazla iki aktif `du`,
32 bekleyen/aktif iş ve 512 önbellek girdisi. İş başına kuyruk dahil üst sınır
60 saniyedir; başarısız ölçümler beş saniye boyunca yeniden denenmez.

Bir istemcinin bağlantıyı kapatması diğer bekleyenlerin ölçümünü iptal etmez;
ortak iş kendi süre sınırı içinde tamamlanır. Önceki başarılı değer varsa
kaynak özeti geçici hatalarda onu kullanır. Manuel disk ölçümü başarısızlıkta
503 döndürür. Görünen boyut (`du -sb`) ile tahsis edilmiş disk alanının
(`du -sm`) mevcut anlamları korunur; farklı ölçüm türleri ayrı önbelleklenir.

## Doğrulama

Regresyon testleri, Git dosyalarının sembolik bağla dışarı yönlendirilmesini,
chmod sırasında eşzamanlı bağ takasını, eski SSH ayarlarının geçersiz
kılınmasını, mevcut parola dosyalarının ayrıştırılmasını, disk önbelleğini,
eşzamanlılık sınırını, iptal ve hata sonrası davranışı kapsar.

```sh
go test ./...
go test -race ./internal/git ./internal/files ./internal/diskusage ./internal/sifrekoruma
go vet ./...
```

2026-09-17 canlı sunucu ortamı doğrulaması: `cloud.sanalcp.com` üzerinde
Linux 6.12.107+deb13-amd64 ile, yerelde derlenen test binary'leri geçici bir
dizinde çalıştırıldı. Git/SSH için üç, chmod için iki, disk ölçümü için beş ve
parola dosyası geçişi için dört üst düzey testin tamamı geçti. Veritabanı
geçiş testleri SQL mock kullandı; canlı veritabanına erişmedi. Test öncesinde
ve sonrasında SanalCP, nginx ve MariaDB aktifti. Geçici test dosyaları silindi.
Çalışan panel binary'si değiştirilmedi; gerçek API/güncelleme geçişi uygulanmadı.
