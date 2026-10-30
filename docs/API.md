# SanalCP Yönetim API'si

Panelin yaptığı her şey HTTP API üzerinden de yapılabilir. Arayüz zaten bu API'yi
kullanır — yani ayrı, sınırlı bir "API sürümü" yoktur; **arayüzde görebildiğiniz her
işlem otomasyona da açıktır.**

- Taban adres: `https://<panel-adresiniz>:8443/api/v1`
- Kimlik doğrulama: `Authorization: Bearer <token>` (otomasyon için `scp_…` API
  token'ları). Tarayıcıdaki panel oturumu ayrı bir yoldur ve HttpOnly çerezle
  taşınır — JavaScript okuyamaz, bu yüzden arayüz Authorization başlığı kurmaz.
- İstek/yanıt gövdesi: JSON (`Content-Type: application/json`)

### Müşteri oturumu ve hesap güvenliği

`POST /musteri/login` gövdesi `{"kullanici":"...","parola":"...","kod":"..."}`.
2FA etkin ve kod eksikse yanıt `200 {"iki_fa_gerekli":true}` olur; oturum
çerezi verilmez. Aynı uç doğru parolayla ve yeni TOTP koduyla yeniden çağrılır.
Yanlış veya tekrar kullanılan kod `401`, okunamayan/bozuk 2FA durumu `500`
döndürür; bu durumlarda da oturum oluşturulmaz.

Kimliği doğrulanmış müşteri kendi hesabında şu uçları kullanabilir:

| Metot | Yol | Gövde / sonuç |
|---|---|---|
| `POST` | `/me/parola` | `{"mevcut":"...","yeni":"..."}` |
| `GET` | `/me/2fa/setup` | Secret, hesaba ait otpauth URI ve QR döndürür; etkinleştirmez |
| `POST` | `/me/2fa/enable` | `{"secret":"...","kod":"..."}` |
| `POST` | `/me/2fa/disable` | `{"kod":"..."}` |

Hedef hesap istek gövdesindeki kimlikten değil doğrulanmış oturumdan alınır.
Başarılı parola/2FA değişikliği tarayıcı oturumlarını iptal eder ve çerezi siler;
yeniden giriş gerekir. Etkinleştirmede kullanılan TOTP kodu tüketilir.
Müşteriye profil düzenleme (`PUT /me`) veya API token yönetimi yetkisi verilmez.
Çerezli değişiklik isteklerinde `Origin` başlığı zorunludur.

---

## 1. Token oluşturma

**Panel → Profil ve Tercihler → API Token'ları → Yeni token.**

Ham token **yalnız bir kez** gösterilir; sunucuda yalnızca SHA-256 özeti saklanır.
Kaybederseniz yenisini üretmeniz gerekir.

API ile de üretebilirsiniz. Panel oturumu **HttpOnly çerezle** taşındığı için
`/auth/login` yanıtında token dönmez; curl ile bir çerez kavanozu kullanmanız
gerekir. Panel oturumuyla yapılan state-changing isteklerde `Origin` başlığı
**zorunludur** (CSRF koruması):

```bash
PANEL=https://panel.example.com:8443

# 1) Giriş yap — oturum çerezi kavanoza yazılır
curl -sS -c cerez.txt -X POST "$PANEL/api/v1/auth/login" \
  -H "Content-Type: application/json" -H "Origin: $PANEL" \
  -d '{"kullanici":"admin","parola":"..."}'

# 2) Çerezle API token'ı üret
curl -sS -b cerez.txt -X POST "$PANEL/api/v1/me/api-tokenlari" \
  -H "Content-Type: application/json" -H "Origin: $PANEL" \
  -d '{"ad":"yedekleme scripti","gun_sonra":365}'

# 3) Çıkış yap — çerezi geçersiz kıl
curl -sS -b cerez.txt -X POST "$PANEL/api/v1/auth/cikis" -H "Origin: $PANEL"
rm -f cerez.txt
```

> Bu yalnızca **ilk token'ı almak** içindir. Sürekli otomasyonda oturum çerezi
> değil, üretilen `scp_…` token'ı kullanılmalıdır: token'ın kendi ömrü, kendi
> iptali ve kendi kullanım kaydı vardır; ayrıca oturum çerezinin tabi olduğu
> boşta-kalma zaman aşımından etkilenmez.

```json
{ "ok": true, "id": 3, "ad": "yedekleme scripti", "token": "scp_1a2b3c…" }
```

| Uç | Açıklama |
|---|---|
| `GET /me/api-tokenlari` | Kendi token'larınızı listeler (ham değer dönmez) |
| `POST /me/api-tokenlari` | Yeni token üretir. `gun_sonra: 0` = süresiz |
| `DELETE /me/api-tokenlari/{id}` | Token'ı iptal eder (anında geçersiz olur) |

Hesap başına en fazla **20** token tutulabilir.

---

## 2. Yetki modeli — okumadan başlamayın

**Token ayrı bir izin sistemi değildir; sahibinin kimliğidir.**

Bir istek geldiğinde panel token'ın sahibini bulur ve isteği *o kullanıcı oturum
açmış gibi* işler. Bunun doğrudan sonuçları:

- **Token, sahibinden fazlasını yapamaz.** Bayi hesabının token'ı yalnız o bayinin
  müşterilerini ve domainlerini görür; yönetici uçlarına erişemez.
- **Rol değişikliği anında yansır.** Hesabın rolü düşürülürse token'ın yetkisi de düşer.
- **Hesap askıya alınırsa token çalışmaz.**
- **Token iptal edilirse anında geçersizdir** (her istekte veritabanından doğrulanır).

Bu yüzden otomasyon için **ayrı ve en dar yetkili bir hesap** açmanız önerilir:
yedekleme script'i için yönetici token'ı yerine, yalnız ilgili müşterilere sahip bir
bayi hesabı kullanmak sızıntı halinde zararı sınırlar.

> ⚠️ **2FA uygulanmaz.** API token'ı, tanımı gereği ikinci faktör soramaz. Token'ı
> parola gibi saklayın; sürüm kontrolüne (git) **koymayın**, ortam değişkeni veya
> yalnız sahibinin okuyabildiği bir dosya kullanın.

> ⚠️ **Boşta oturum zaman aşımı uygulanmaz.** O kural interaktif oturumlar içindir.
> Token'ın ömrü kendi `gun_sonra` değeriyle yönetilir — uzun ömürlü token'lara son
> kullanma tarihi vermeniz önerilir.

Her token'ın **son kullanım zamanı ve IP'si** kaydedilir; token listesinden
beklenmedik kullanım fark edilebilir.

### Plan bazlı müşteri modülleri

`POST /plans` ve `PUT /plans/{id}` gövdeleri `features` nesnesini destekler.
Nesne verildiğinde sekiz alanın **tamamı** boolean olmalıdır:

```json
{
  "dns": false,
  "mail": true,
  "databases": true,
  "files": false,
  "ftp": true,
  "cron": true,
  "backups": true,
  "git": true
}
```

Plan oluştururken alanı göndermemek tüm modülleri açar; açıkça `null` göndermek
`400` döndürür. Güncellemede eksik veya `null` `features` mevcut izinleri korur.
Eksik/bilinmeyen anahtarlar ve boolean olmayan değerler `400` döndürür.
Planın diğer alanlarını korumak için önce mevcut planı okuyup tamamını
`PUT /plans/{id}` ile gönderin; uç yalnız özelliklere özel bir PATCH değildir.

`GET /domains/{id}/features`, sahiplik denetiminden sonra
`{"features":{...},"enforced":true}` döndürür. İzinler domainin planından, ek
alan adlarında ise ana hesabın güncel planından okunur. Eski planların SQL `NULL`
ayarı ve plansız hesaplar tüm modülleri açık tutar; bozuk kayıt veya okunamayan
politika müşteriye erişim vermez.

Kısıtlar `user` rolündeki müşteriye uygulanır. Yönetici/bayi için `enforced`
`false` olur; yanıt yine planın gerçek bayraklarını taşır ve mevcut sahiplik
denetimleri geçerlidir. İzin değişiklikleri yeni giriş gerektirmeden bir sonraki
istekte uygulanır.

Kapalı modül `403 {"hata":"...","code":"feature_disabled","feature":"files"}`,
okunamayan politika `503` döndürür. Veritabanı kimliğiyle silme/parola/phpMyAdmin
token uçları da aynı denetime tabidir. `dns`, domain Cloudflare/nameserver
işlemlerini; `git`, domain GitHub işlemlerini; `backups`, takvim/hedef/restore
işlerini de kapsar.

Bu izinler yönetim API'sini sınırlar; mevcut hizmetleri veya kimlik bilgilerini
kapatmaz. Otomatik yedekler, cron görevleri, Git webhook'ları ve uygulama CLI
erişimi değişmez. Bir modülü kapatmak, diğer bağımsız uygulama araçlarının
dosya/veritabanı işlemlerini genel olarak yasaklayan bir sandbox değildir.

### E-posta teslimat takibi

| Metot | Yol | Kapsam |
|---|---|---|
| `GET` | `/mail/deliveries` | Yönetici tümü; bayi yalnız müşterileri |
| `GET` | `/mail/deliveries/{id}` | Aynı kapsam; kapsam dışı `404` |
| `GET` | `/domains/{id}/mail/deliveries` | Domain sahibi/müşteri; `E-posta` planı gerekir |
| `GET` | `/domains/{id}/mail/deliveries/{id}` | Aynı alıcı kapsamı |

Filtreler: `sender`, `recipient` (harfi harfine alt dize; `%` ve `_` joker değildir),
`queue_id` (tam, büyük/küçük harf duyarlı), `status`
(`pending|deferred|delivered|bounced|forwarded`), `from`/`to` (`YYYY-MM-DD` veya UTC
RFC3339; son tarih günün sonunu içerir), `page`, `page_size` (en çok 100). Geçersiz
değer `400` döndürür. Yanıt `items`, `total`, `page`, `page_size` ve
`collector` (`state`, `warning`: `history_gap` veya `source_unavailable`,
`last_collected_at`) içerir. Detay yanıtı yalnız ilgili alıcının denemelerini ve
`events_truncated` bayrağını (son 1000 olay) taşır.

`delivered`, Postfix'in iletiyi sonraki sunucuya veya yerel posta kutusuna
devrettiğini gösterir; okunduğunu değil. `forwarded` ara adımdır. Boş sonuç
`collector.state/warning` ile birlikte okunmalıdır: kaynak hatası ve geçmiş
boşluğu "teslim edilmedi" anlamına gelmez.

### Posta kutusu kotası

| Metot | Yol | Açıklama |
|---|---|---|
| `POST` | `/domains/{id}/mail` | İsteğe bağlı `quota_mb`; verilmezse planın `mailbox_quota_mb` değeri |
| `PUT` | `/domains/{id}/mail/{mid}/quota` | `{"quota_mb": n}` — `0` sınırsız; `E-posta` planı gerekir |
| `GET` | `/domains/{id}/mail` | Kutular `quota_bytes` ve `used_bytes` (bilinmiyorsa `null`) taşır |

Müşteri, planın `mailbox_quota_mb` değeri 0'dan büyükse `1..plan` aralığında kalmalıdır
ve sınırsız seçemez; yönetici/bayi `0..1048576` MB belirleyebilir. Aralık dışı `400`,
planı çözülemeyen hesap `503` döndürür. `service_plans.mailbox_quota_mb` plan
`POST/PUT` gövdesinde gönderilir; güncellemede eksik alan mevcut değeri korur.
Plan kotasını düşürmek mevcut kutuları değiştirmez.

---

## 3. Hızlı başlangıç

```bash
export SANALCP="https://panel.example.com:8443/api/v1"
export TOKEN="scp_…"

# Kimliğinizi doğrulayın
curl -sS "$SANALCP/me" -H "Authorization: Bearer $TOKEN"
```

```json
{ "id": 1, "adi": "root", "rol": "admin", "eposta": "", "ad_soyad": "",
  "durum": "active", "iki_fa": true, "tercih_tema": "light", "tercih_dil": "tr" }
```

Panel self-signed sertifika kullanıyorsa (varsayılan kurulum) `curl -k` ekleyin ya da
panele bir alan adı + Let's Encrypt sertifikası tanımlayın.

---

## 4. Sık kullanılan uçlar

### Domainler

| Metot | Yol | Açıklama |
|---|---|---|
| `GET` | `/domains` | Domain listesi (kapsamınıza göre daraltılır) |
| `GET` | `/domains/{id}` | Tek domain |
| `POST` | `/domains` | Yeni domain |
| `DELETE` | `/domains/{id}` | Domain sil |
| `POST` | `/domains/{id}/askiya-al` | Askıya al (site 503 döner) |
| `POST` | `/domains/{id}/askidan-al` | Askıyı kaldır |
| `PUT` | `/domains/{id}/php` | PHP sürümünü değiştir |
| `GET` | `/domains/{id}/kaynak` | Disk/trafik/DB kullanımı ve limitleri |

Domain oluşturma:

```bash
curl -sS -X POST "$SANALCP/domains" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{
        "alan_adi": "ornek.com",
        "php_surum": "8.3",
        "site_tipi": "wordpress",
        "plan_id": 2
      }'
```

| Alan | Zorunlu | Not |
|---|---|---|
| `alan_adi` | evet | |
| `php_surum` | hayır | Boşsa sunucu varsayılanı |
| `site_tipi` | hayır | `php` (varsayılan) · `wordpress` · `statik` |
| `plan_id` | hayır | Boşsa varsayılan plan |
| `customer_id` | hayır | Mevcut bir müşteriye bağlar |
| `bayi_user_id` | hayır | **Yalnız yönetici** geçebilir; bayi çağırırsa yok sayılır |

### Yedekleme

| Metot | Yol | Açıklama |
|---|---|---|
| `GET` | `/domains/{id}/backups` | Yedek listesi |
| `POST` | `/domains/{id}/backups` | Yeni yedek al |
| `GET` | `/domains/{id}/backups/{bid}/indir` | Yedeği indir |
| `POST` | `/domains/{id}/backups/{bid}/geriyukle` | Arka planda geri yükleme işi başlat (`202`, `job_id`) |
| `GET` | `/domains/{id}/backup-restore-jobs/active` | Aktif geri yükleme işini bul |
| `GET` | `/domains/{id}/backup-restore-jobs/{jid}` | Geri yükleme işinin durumunu oku |
| `DELETE` | `/domains/{id}/backup-restore-jobs/{jid}` | Geri yükleme işini iptal et |
| `DELETE` | `/domains/{id}/backups/{bid}` | Yedeği sil |

### DNS

| Metot | Yol | Açıklama |
|---|---|---|
| `GET` | `/domains/{id}/dns` | Kayıtları listele |
| `POST` | `/domains/{id}/dns` | Kayıt ekle |
| `PUT` | `/domains/{id}/dns/{rid}` | Kayıt güncelle |
| `DELETE` | `/domains/{id}/dns/{rid}` | Kayıt sil |

### Hesaplar ve planlar

| Metot | Yol | Açıklama |
|---|---|---|
| `GET` | `/users` | Panel hesapları (kapsamınıza göre) |
| `POST` | `/users` | Hesap oluştur (bayi yalnız müşteri açabilir) |
| `GET` | `/customers` | Müşteri kayıtları |
| `GET` | `/plans` | Hizmet planları ve kaynak limitleri |

### Sunucu

| Metot | Yol | Açıklama |
|---|---|---|
| `GET` | `/system/usage` | CPU / bellek / disk / yük |
| `GET` | `/system/surum` | Panel sürümü ve derleme tarihi |
| `GET` | `/system/servisler` | Servis durumları |

Sağlık kontrolü `/api/v1` **altında değildir** ve kimlik gerektirmez:
`GET https://<panel>:8443/healthz`

---

## 5. Bu listede olmayan uçlar

Reverse proxy sitesinin Node.js/Python uygulama süreci için:

| Uç | Açıklama |
|---|---|
| `GET /domains/{id}/app-runtime` | Kayıt, çalışma durumu ve sunucudaki yorumlayıcılar |
| `PUT /domains/{id}/app-runtime` | `{"runtime":"node","interpreter":"/usr/bin/node","entrypoint":"public_html/server.js","health_path":"/health","port":3000,"enabled":false}` ile servis kur/güncelle; `interpreter` boşsa sunucu varsayılanı seçilir, sağlık yolu HTTP 2xx dönmeli, port sitenin reverse proxy portuyla aynı olmalı |
| `POST /domains/{id}/app-runtime/dependencies` | Node.js için lockfile üzerinden `npm ci --omit=dev`, Python için sanal ortamda `pip install -r requirements.txt` |
| `POST /domains/{id}/app-runtime/start` | Servisi başlat |
| `POST /domains/{id}/app-runtime/stop` | Servisi durdur |
| `POST /domains/{id}/app-runtime/restart` | Servisi yeniden başlat |
| `GET /domains/{id}/app-runtime/logs` | Son 100 servis günlük satırı |

Bu uçlar domain sahipliği kapsamını uygular. Giriş dosyası tenant'ın
`public_html` dizininde olmalıdır. Uygulama `127.0.0.1` üzerinde kendisine
verilen `PORT` değerini dinlemelidir.

Yönetilen uygulamada `POST /domains/{id}/git/pull` ve imzalı Git webhook'u,
Git hedefi `public_html` ise ayrı bir Git worktree içinde bağımlılıkları kurar,
Node.js için varsa build betiğini çalıştırır, servisi bu sürüme yönlendirir
ve `health_path` üzerinden HTTP 2xx bekler. Hata halinde önceki servis sürümü
geri yüklenir. `GET /domains/{id}/app-runtime` yanıtındaki `active_release`
etkin worktree yoludur. İlk klonlama uygulama başlatılmadan yapılmalıdır.

Yukarıdakiler en sık kullanılanlardır; panelin tamamı çok daha geniştir (e-posta
kutuları, SSL, cron, FTP, veritabanları, dosya yöneticisi, güvenlik duvarı, WordPress
araçları…). Hepsi aynı kimlik doğrulama ve yetki kurallarıyla çalışır.

Bir işlemin ucunu bulmanın en pratik yolu: **paneli tarayıcıda açın, geliştirici
araçlarında Network sekmesini açın ve işlemi arayüzden bir kez yapın.** Gördüğünüz
istek, script'inizde birebir kullanabileceğiniz istektir.

Kaynaktan bakmak isterseniz tüm rotalar tek dosyadadır: `cmd/server/main.go`.

---

## 6. Yanıtlar ve hatalar

Başarılı işlemler `200`/`201` ve JSON gövde döner. Hatalar HTTP durum kodu ve şu
biçimde bir gövdeyle gelir:

```json
{ "hata": "bu domain'e erişim yok" }
```

| Kod | Anlamı |
|---|---|
| `400` | Geçersiz gövde veya parametre |
| `401` | Token yok, geçersiz, iptal edilmiş veya süresi dolmuş |
| `403` | Kimlik geçerli ama bu işlem için yetki yok (rol/kapsam) |
| `404` | Kayıt yok — ya da **kapsamınız dışında** |
| `409` | Çakışma (ör. alan adı zaten var) |
| `500` | Sunucu hatası; panel günlüklerine bakın |

`404` ile `403` ayrımına güvenmeyin: kapsam dışı kayıtlar bilinçli olarak "yok" gibi
davranabilir — hangi kayıtların var olduğu bilgisi sızmasın diye.

---

## 7. Örnek: tüm domainlerin yedeğini alan script

```bash
#!/usr/bin/env bash
set -euo pipefail

SANALCP="https://panel.example.com:8443/api/v1"
TOKEN="${SANALCP_TOKEN:?SANALCP_TOKEN ortam değişkeni gerekli}"

cagir() { curl -sS -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" "$@"; }

cagir "$SANALCP/domains" | jq -r '.[] | "\(.id) \(.alan_adi)"' |
while read -r id alan; do
  echo "yedekleniyor: $alan"
  cagir -X POST "$SANALCP/domains/$id/backups" -d '{}' >/dev/null
done
```

Token'ı script'in içine yazmayın; ortam değişkeninden okuyun ve dosya izinlerini
`0600` tutun.
