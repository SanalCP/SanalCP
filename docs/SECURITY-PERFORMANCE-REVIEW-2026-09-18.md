# Güvenlik ve performans raporu doğrulaması

Tarih: 2026-09-18 · İncelenen commit: `f91c67b`

İlk rapor: `SECURITY-PERFORMANCE-ANALYSIS-2026-09-18.md`.

Sonuç: Gerçek ve acil açıklar var. Ancak ilk raporun bütün önem dereceleri ve düzeltme önerileri aynen uygulanmamalı. Öncelik, müşteri kontrollü girdilerin root süreçlerine ulaşmasını kapatmak; sonra süreç çökmesi, sır sızıntısı ve uzun işlem güvenilirliği.

## Yöntem ve sınırlar

- İlgili uygulama kodu, HTTP yetki zinciri, migration dosyaları, frontend polling kodu ve systemd tanımı karşılaştırıldı.
- Yerel MySQL istemcisine yalnız sabit metin yazdıran bir `\! printf` komutu gönderildi. Normal çağrı komutu çalıştırdı; `--binary-mode` eklenince `Unknown command '\!'` ile reddetti. Uygulama verisi değiştirilmedi.
- `rsync -a --delete` davranışı yalnız geçici test dizinlerinde tekrarlandı. Symlink hedefinin dışındaki test dizininde eski dosya silindi ve kaynak dosya oluşturuldu. Gerçek tenant veya sistem dizinleri hedeflenmedi.
- Bunlar CLI davranış testleridir; canlı panel üzerinden uçtan uca istismar testi yapılmadı. Kod değişikliği, dağıtım veya migration uygulanmadı.
- İlk rapordaki `go vet`, `go test ./...` ve `npm audit` sonuçları bu incelemede yeniden çalıştırılmadı. SQL performansı için canlı `EXPLAIN`/yük ölçümü yapılmadı; indeks değerlendirmeleri repository şemasına dayanıyor.

## Uygulama sırası

| Sıra | Öncelik | İş | Karar |
|---|---|---|---|
| 1 | P0 — acil | Güvenilmeyen SQL'in root istemcide işlenmesi | C1 doğrulandı; tüm import yolları birlikte kapatılmalı. |
| 2 | P0 — acil | Site kopyalama ve uygulama kurulumlarında tenant dışına dosya erişimi | C2 doğrulandı; ilk rapordaki orta seviye kurulum bulgusu aynı çalışmaya alınmalı. |
| 3 | P1 — ilk düzeltme paketi | WordPress durum handler'ında eşzamanlı map yazımı | H1 kesin kod hatası; tüm panel sürecini düşürebilir. |
| 4 | P1 | GitHub PAT'in URL içinde saklanması ve taşınması | H2 doğrulandı; mevcut kayıtlar ve clone edilmiş depolar da temizlenmeli. |
| 5 | P1 | Uzun işlemlerin 300 saniyelik request context'iyle çakışması | H4 kısmen doğru; senkron işlemler etkilenir, mevcut arka plan transferinin kendi 2 saatlik context'i var. |
| 6 | P1/P2 | TOTP denetiminin giriş yollarında tutarlı ve atomik olması | Müşteri bypass'ı koşullu; replay kontrolündeki yarış gerçek. |
| 7 | P2 — sonraki paket | Güvenlik olayları sorguları, indeksler ve varsayılan sayfalama | H5'in maliyet kaynağı doğrulandı; şiddeti veri hacmiyle ölçülmeli. |
| 8 | P2 | İş kuyruğu kapasitesi, pipe kapanışı, kilit ve timeout yönetimi | Somut kaynak yönetimi sorunları var. |
| 9 | P2/P3 | PMA iç uç kısıtlaması, oturum iptali ve küçük sağlamlaştırmalar | Faydalı; P0/P1 işlerinin önüne geçmemeli. |

## 1. SQL import — C1 doğru, önerilen tek satır yeterli kapsam değil

Kanıt: `internal/sqlimport/sqlimport.go:150`, `internal/backups/verify.go:134`, `internal/backups/restore.go:387`, `internal/transfers/handlers.go:1357`; servis `assets/systemd/sanalcp.service:12` içinde `User=root`.

`sqlimport.Uygula` düşük yetkili **veritabanı** kullanıcısını seçiyor; işletim sistemi kullanıcısını değiştirmiyor. `definerSuz` yalnız DEFINER temizliyor. Bu nedenle CLI kabuk komutu root süreç içinde çalışabilir. Müşteri SQL yükleme ve DB geri yükleme rotaları mevcut (`cmd/server/main.go:582,610`). Yedek doğrulama da müşteri erişimli (`:741`).

Yapılacaklar:

- Güvenilmeyen SQL okuyan tüm CLI çağrılarını ortak, güvenli bir çalıştırıcıda toplamak; noninteractive `--binary-mode` uygulamak. `--defaults-extra-file` ilk seçenek olma gereksinimini korumak.
- İstemciyi mümkün olduğunda tenant işletim sistemi kimliğinde, daraltılmış ortam ve dosya erişimiyle çalıştırmak.
- Restore yollarında DB yetkisini de sınırlandırmak. Özellikle `backups.importDatabase`, doğrudan `mysql database` çalıştırıyor; hedef DB adını doğrulamak, dump içindeki SQL'in diğer DB'lere erişmesini tek başına önlemez. `--binary-mode` SQL yetkilerini sınırlandırmaz.
- `hesaplar.MySQLRenameDB` ve staging kopyası da ortak çalıştırıcıyı kullanmalı; ancak sunucunun ürettiği `mysqldump` akışı ile doğrudan yüklenen saldırgan SQL aynı erişilebilirlikte kabul edilmemeli.
- Komut satırlarını regex ile silmek temel güvenlik sınırı yapılmamalı: çok satırlı SQL verisini bozabilir ve parser farklılıklarını güvenilir biçimde çözmez.

Kabul: Normal dump, trigger/routine/DELIMITER ve büyük satırlar çalışmalı; CLI komutları yerel işlem başlatamamalı; hedef dışı DB erişimi DB yetkileriyle reddedilmeli. MySQL/MariaDB desteklenen sürümlerinde test edilmeli.

Resmî dayanak: [MySQL istemci seçenekleri — binary-mode](https://dev.mysql.com/doc/refman/8.0/en/mysql-command-options.html).

## 2. Tenant dosya sınırı — C2 doğru, kurulum yolları da kapsama alınmalı

Kanıt: `internal/sitekopya/sitekopya.go:132,188`; müşteri rotaları `cmd/server/main.go:525-528`.

Kaynak symlink'i dışarıdan dosya okumaya/kopyalamaya; hedef symlink'i dışarıya yazmaya ve `--delete` nedeniyle silmeye yol açabilir. Dolayısıyla rapordaki kaynak/hedef anlatımı ayrılmalı, kritik sonuç korunmalı.

`internal/apps/handlers.go:157-162` ve `internal/wordpress/wordpress.go:408-413` içindeki root `MkdirAll/chown/restorecon` işlemleri de aynı güven sınırında. Özellikle üst yol bileşenindeki symlink nedeniyle risk yalnız çok dar bir yarış penceresinden ibaret değil. Her `chown -R` çağrısının her symlink'i izlediği varsayılmamalı; nihai bileşen ile üst yol bileşenleri farklı davranır.

Yapılacaklar: Kaynak/hedef için symlink güvenli yol işlemleri, tenant kimliğinde okuma/yazma, root ile kullanıcı kontrollü yollarda recursive işlem yapılmasının kaldırılması. Staging ve canlı site farklı sistem kullanıcıları olduğundan sadece `runuser` eklemek erişim hatasına yol açabilir; kaynak kullanıcısında okuma ve hedef kullanıcısında yazma veya kontrollü ara kopya tasarlanmalı.

İlk raporun örnek gösterdiği `backups/restore.go:229-263` tamamen güvenli bir kopyala-yapıştır çözümü değil: rsync tenant kimliğine düşüyor ama ardından root `chown -R` ve `restorecon -R` devam ediyor. Bu artık işlemler de incelenmeli.

Kabul: Kaynak/hedef symlink ve eşzamanlı yol değiştirme denemeleri tenant dışındaki dosyaları okuyamamalı/değiştirememeli; normal staging iki farklı sistem kullanıcısıyla çalışmalı.

## 3. WordPress map yarışı — H1 doğru

`internal/wordpress/toolkit.go:63-105`: beş goroutine aynı Go map'ine yazıyor. Ayrı anahtarlar güvenlik sağlamaz; `WaitGroup` yalnız bitişi bekler. Fatal map hatası HTTP Recoverer ile kurtarılamaz.

Sonuçları ayrı değişkenlerde toplayıp `Wait()` sonrasında map oluşturmak veya mutex kullanmak gerekir. Kabul: handler'ın eşzamanlı sonuç üretme yolunu çalıştıran `-race` testi. Normal testlerin geçmesi bu hatayı dışlamaz.

## 4. GitHub PAT — H2 doğru

`internal/github/github.go:355-370` PAT'i `https://TOKEN@github.com/...` olarak `git_repos.repo_url` içine yazıyor. `internal/git/git.go` bunu API modeli ve clone argümanı olarak kullanıyor. Mevcut `github_connections` şifrelemesi bu ikinci kopyayı korumuyor. Standart clone akışı token içeren origin URL'sini `.git/config` içinde de bırakabilir; mevcut deployment'lar buna göre kontrol edilmeli.

Temiz repo URL'si, ayrı şifreli sır saklama, argv'ye sır koymayan kimlik aktarımı ve log maskelemesi gerekiyor. `git -c http.extraHeader=...` içine token koymak da argv sızıntısını sürdürür. Eski DB kayıtları ve origin URL'leri taşınmalı; maruz kalmış PAT'ler yenilenmeli.

## 5. Uzun işler — H4'ün temel çakışması doğru, sonucu genellenmiş

`cmd/server/main.go:327` tüm router'a 300 saniye timeout uyguluyor. `internal/httpx/httpx.go:128` yalnız soket read/write deadline'larını değiştiriyor. Çocuk context'e 15/30 dakika vermek ebeveynin 5 dakikasını uzatmıyor.

Ancak “hepsi 5 dakikada sessiz rollback” doğru değil. Context'i izleyen SQL/komutlar durabilir; context'siz işlemler devam edebilir. `files.Download` düz `io.Copy` kullandığından context iptali tek başına aktarımı kesmez. `remoteCalistir` zaten `context.Background()` altında 2 saatlik iş context'i kullanıyor (`remote_job.go:145`).

Senkron upload/download rotaları için uygun route bazlı timeout; restore/deploy gibi uzun mutasyonlar için durum kaydı, iptal ve kontrollü shutdown içeren arka plan iş modeli önerilir. Bütün handler'lara körlemesine `context.Background()` eklenmemeli. Kabul: 300 saniyeyi aşan işler, istemci kopması ve yarım kalan işin toparlanması test edilmeli.

## 6. TOTP — kapsam düzeltmesi gerekli

`internal/musteri/musteri.go:65` TOTP okumuyor. Fakat `cmd/server/main.go:384-386` TOTP açma/kapama uçlarını bayi ve üstüyle sınırlıyor. Bu nedenle sıradan müşterinin mevcut UI'da açtığı 2FA'yı kolayca atladığı sonucu çıkmaz.

Somut koşullu senaryo: 2FA açık bayi/admin sonradan `user` rolüne düşürülürse `internal/users/handlers.go:379` TOTP durumunu temizlemiyor; müşteri giriş yolu bu etkin korumayı atlayabilir. Girişlerin ortak TOTP politikasına bağlanması gerekir. Böyle hesaplar varsa öncelik P1, yoksa P2.

`internal/auth/handlers.go:261` son adımı koşulsuz yazıyor ve UPDATE hatasını yok sayıyor. Eşzamanlı istekler aynı kodla geçebilir. `WHERE totp_last_step < ?` benzeri atomik tüketim ve `RowsAffected` denetimi gerekir. TOTP seed'inin şifreli saklanması da aynı pakete alınabilir; seed'in doğrulama için geri okunabilir olması gerekir, hash yeterli değildir.

## 7. Performans — H5 doğru yönde, önem derecesi ölçüme bağlı

`guvenlikolay.senkronize` istek içinde log okuma, altı agregasyon ve adaylara göre upsert yapıyor; işlem merkezi özeti bunu tekrar çağırıyor. `audit_log` migration'ında `ts`, actor ve action indeksleri var, IP indeksi yok; repository'de audit retention işi bulunamadı.

Polling sürekli 5 saniye değil: `frontend/src/components/IslemMerkezi.tsx:24-25` boşta 60 saniye, aktif iş varken 10 saniye; açık işlem panelinde aktif iş varken ayrıca 5 saniye.

Öneri: Tek arka plan üreticisi ve cache, çakışan yenilemeleri birleştirme, context'li sorgular; retention/arşiv politikasının tanımlanması. `audit_log(ip,ok,ts)` self-join için güçlü adaydır, nihai indeks gerçek sorgu planıyla seçilmeli. `db_accounts.db_user` indeksi migration 0036 ile kaldırılmış; tekrar unique değil, non-unique indeks düşünülmeli. İş tablolarında status/domain/time erişimlerine uygun birleşik indeksler ölçülmeli; yalnız bütün tarih alanlarına indeks eklenmemeli.

Domains/users/accounts listelerinde parametre verilmezse LIMIT eklenmiyor: doğrulandı. Varsayılan sınır getirirken mevcut dropdown ve toplu liste tüketicileri taşınmalı. Genelbakış, backup özeti ve WordPress toplu liste yolları da veri hacmiyle profillenmeli; sınırsız sorgu tek başına bugün ölçülmüş yavaşlık değildir.

## 8. Kaynak yönetimi — gerçek ama kapsamı sınırlı bulgular

| Bulgu | Doğrulama / yapılacak iş |
|---|---|
| Sınırsız transfer kuyruğu | `remote_job.go:89,145`: her istekte goroutine, semafor önünde sınırsız bekleme. Admin-only. Kapasiteli kuyruk, iş/durum kotası, tekrar gönderim kontrolü ve iptal eklenmeli. |
| Pipe sızıntısı | `remote_job.go:441`: reader için kapanış yok; Import erken dönerse writer bloklanabilir. `defer pr.Close()` ve iptal/hata aktarımı eklenmeli. |
| CVE mutex | `system/cve.go:262`: tarama boyunca kilit tutuluyor. Eski cache'i hemen döndürüp tek arka plan yenilemesi yapılmalı. Süre platforma göre değişir. |
| Grav/Matomo indirme kilidi | `indirici.go:35`: HTTP çağrısı kilit altında; doğru. Ancak istek `NewRequestWithContext` kullanıyor; “DefaultClient nedeniyle daima timeout'suz” sonucu yanlış. Kilit kapsamı daraltılmalı, açık ağ timeout'u eklenmeli. |
| Servis komutları | `system/servis.go:142`: context'siz `exec.Command`, hata da yutuluyor. Süre sınırı ve hata yönetimi eklenmeli; uygun durum sorguları gruplanmalı. |
| Context'siz DB | Auth profile/apitoken/dashboard içinde mevcut. Request süresi ve DB deadline'larıyla sınırlandırılmalı; her audit/background işlemi mekanik olarak request context'ine bağlanmamalı. |
| Rate-limit | Periyodik temizlik var; “hiç budanmayan map” değil. Pencere içinde anahtar sayısına üst sınır yok, restart sayacı siler. Kapasite limiti uygulanmalı; kalıcı/ortak sayaç çok süreçli dağıtım ihtiyacına göre seçilmeli. |

## 9. Düşürülmesi veya ayrıca kanıtlanması gereken iddialar

- **H3 / PMA:** `pma.go:114` sabit-zamanlı olmayan karşılaştırma gerçek; fakat uç kimlik doğrulamasız değil. İç servis sırrı ve geçerli, kısa ömürlü, atomik tüketilen token gerekiyor. Ağ üzerinden pratik timing istismarı gösterilmemiş. Constant-time kontrol, boyut/hız sınırı ve yalnız iç servis erişimi önerilir. Standart kullanıcı `RequireAuth` grubuna taşımak PHP'nin server-to-server akışını bozabilir; `assets/phpmyadmin/pma-signon.php:29-36` loopback ve iç header kullanıyor.
- **Logout:** Çerez siliniyor, ele geçirilmiş JWT sunucuda iptal olmuyor; gerçek tasarım sınırlaması. Her logout'ta `auth_version` artırmak bütün cihazları çıkarır. Tek oturum iptali gerekiyorsa session/jti kaydı veya denylist tasarlanmalı; acil yetki bypass'ı olarak değerlendirilmemeli.
- **Admin SSH / SSRF:** Private/loopback IP kabul ediliyor, ancak transfer uçları AdminOnly. Özel ağdan sunucu taşıma geçerli ürün ihtiyacı olabilir. Tanımlı güven sınırı ihlali gösterilmeden doğrudan SSRF açığı denmemeli; hedef politikası belirlenmeli.
- **lftp injection:** Bu ortamda lftp yok; istismar doğrulanmadı. Girdiler çift tırnak içinde ve tırnak/backslash kaçırılıyor. Yalnız newline bulunabilmesi, quoted alanı kırıp `!` çalıştırdığını kanıtlamaz. Gerçek parser ile izole test yapılmadan kesin açık veya acil iş listesine alınmamalı. Kontrol karakterlerini reddetmek ayrı sağlamlaştırmadır.
- **JWT exp/aud:** Parser exp zorunlu kılmıyor, ama Issue her token'a exp koyuyor ve imza doğrulanıyor. Saldırgan imzalı token'dan exp'yi çıkaramaz. Exp zorunluluğu eklemek iyi; aud ihtiyacı token'ın birden fazla serviste kullanımına bağlı.
- **PANEL_SECRET_KEY / SHA-256:** Installer `openssl rand -hex 32` üretiyor. Yüksek entropili rastgele anahtarı SHA-256 ile 32 byte'a dönüştürmek parola saklama problemindeki “tuzsuz hızlı hash” ile aynı açık değil. Mevcut standart üretim için düzeltme gerekmiyor; insan seçimi zayıf anahtar ayrı konudur.
- **crypto/rand hatası:** Mevcut `go.mod` Go 1.26; yerel `go doc crypto/rand.Read` bu fonksiyonun hata döndürmediğini, başarısızlıkta süreci durdurduğunu belirtiyor. Bu toolchain için hata yutulup sıfır token üretildiği iddiası geçerli değil.
- **chpasswd newline:** `auth/profile.go:118` doğrulama eksikliği gerçek. Ancak yol root panel kimliği, root girişinin açık olması ve mevcut root parolasını gerektiriyor. Genel müşteri→root açığı değil. `ParolaGecerli` ile CR/LF/NUL reddi uygulanmalı.
- **PECL baştaki tire:** `phpext.safeName` kabul ediyor, paket argv'ye gidiyor. Başlangıç harfi/rakam zorunluluğu eklenmeli; etkisi ayrıca test edilmeden root RCE diye sunulmamalı.
- **KapsamSQL alias:** Bugünkü sabit çağrılar üzerinden saldırgan girdisi gösterilmemiş; API tasarımı sağlamlaştırması.
- **URL'de webhook/reveal token:** Loglara düşebilme gerçek bir sır yönetimi konusu; erişim logu/redaction ayarı ve gerçek token ömrü incelenmeli. URL'de token olması tek başına herkese sızdığını göstermez.

İlk raporun “iyi durumda, dokunma” bölümü tam güvenlik garantisi sayılmamalı. Güvenli `jailpath` yardımcılarının bulunması, onları kullanmayan kurulum/kopyalama akışlarını korumuyor. Bu çalışma tüm SQL/XSS/arşiv yüzeylerini yeniden denetleyen kapsamlı bir pentest değildir.

## Önerilen teslim paketleri

1. **Acil güvenlik:** SQL import güven sınırı + site/staging/kurulum dosya sınırı. Tenant izolasyonu testleriyle teslim.
2. **Kararlılık ve sır koruması:** WordPress map yarışı + PAT taşıma/temizleme + uzun iş yaşam döngüsü + TOTP tutarlılığı.
3. **Ölçek ve sağlamlaştırma:** Güvenlik olayları cache/indeks, sayfalama geçişi, kuyruk/pipe/timeout, PMA ve oturum politikası.

İlk raporun önerdiği C1 → C2 → H1 başlangıcı doğru. H3'ü H2'nin önüne koymak için yeterli kanıt yok; PAT ve uzun işlem problemleri daha somut. Lftp ve SSH iddiaları doğrulanmış kritik açıklarla aynı statüde takip edilmemeli.
