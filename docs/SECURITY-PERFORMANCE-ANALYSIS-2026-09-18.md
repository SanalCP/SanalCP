# SanalCP Güvenlik & Performans Analizi

> Tarih: 2026-09-18 · Kapsam: 71k satır Go + React/TS · Yöntem: 4 paralel derin denetim + elle doğrulama
> Araç durumu: `go vet` temiz, `go test ./...` temiz, `npm audit` 0 açık.

## KRİTİK

**C1 — `mysql` istemcisiyle root RCE (`\!` / `system` kaçışı)**
Kullanıcı SQL dump'ı root `mysql` istemcisine stdin olarak veriliyor. MySQL CLI, dump içindeki `\! komut` satırlarını **yerel kabukta** çalıştırır — panele bağlı DB kullanıcısı ne olursa olsun, istemci process'i root olduğu için root olur. Bu makinede doğrulandı:
```
printf '\! id > /tmp/x\n' | mysql -N -B   # /tmp/x -> uid=0(root)
```
Noktalar: `internal/sqlimport/sqlimport.go:150`, `internal/backups/restore.go:387`, `internal/backups/verify.go:134` (sadece "doğrula" demek bile yeter), `internal/transfers/handlers.go:1357`, `internal/hesaplar/hesaplar.go:435`. Müşteri erişimli yollar: `main.go:610` (SQL yükle), `main.go:582` (DB geri yükle), `main.go:740` (yedekten geri dön).
**Düzeltme:** tüm `mysql` çağrılarına `--binary-mode` ekle (`\!` bloklanır), ayrıca `definerSuz`'a `\!`/`system`/`source` satır filtresi.

**C2 — `sitekopya` root `rsync -a --delete` ile symlink jailescape**
`internal/sitekopya/sitekopya.go:132` ve `:188` root olarak, path tabanlı, `jailpath`/`tenantKomut` **kullanmadan** çalışıyor. Kiracı kendi `public_html`'ini `/etc`'e symlink yaparsa `--delete` root yetkisiyle `/etc`'e yazar/siler (`chown -R` de aynı). Aynı hatanın doğru örneği `backups/restore.go:229-263`'te zaten var.
**Düzeltme:** kaynak/hedefi `jailpath.DizinDogrula` ile doğrula ve `rsync`'i `tenantKomut` ile kiracı kullanıcısında çalıştır.

## YÜKSEK

| # | Bulgu | Yer | Düzeltme |
|---|---|---|---|
| H1 | **Panel çökerten race**: 5 goroutine aynı `out` map'ine farklı anahtar da olsa eşzamanlı yazıyor → `fatal error: concurrent map writes` (Recoverer yakalayamaz) | `internal/wordpress/toolkit.go:63-105` | mutex veya goroutine başına yerel değişken + `wg.Wait()` sonrası ata |
| H2 | **GitHub PAT düz metin**: `repo_url`'e gömülüyor, API'den UI'a dönüyor, `git clone` argv'sinde `ps`'te görünüyor | `internal/github/github.go:355-370`, `internal/git/git.go:56,198` | PAT'i `secretcrypt` ile sakla, clone anında `GIT_ASKPASS`/extraHeader ile ver, yanıtlarda temizle |
| H3 | PMA redeem'de `auth != expected` sabit-zamanlı değil; uç `RequireAuth` + erişim/hız limiti **dışında**, başarılı istek DB kimlik bilgisi döndürüyor | `internal/pma/pma.go:114`, `cmd/server/main.go:341` | `subtle.ConstantTimeCompare` + ucu kısıtlı gruba al |
| H4 | chi `Timeout(300s)` request context'i iptal ediyor; `ExtendDeadline` yalnız soket süresini uzatıyor → 30dk'lık aktarım/geri yükleme 5dk'da sessizce rollback | `cmd/server/main.go:327`, `transfers/handlers.go:149,223`, `files/files.go:184` | uzun işlerde `context.WithTimeout(context.Background(), …)` kullan |
| H5 | `guvenlikolay.senkronize` her istekte 2MB log + 6 agregasyon sorgusu çalıştırıyor; frontend 5 sn'de poll ediyor. `audit_log` self-join'inde `ip` indeksi yok, retention yok | `internal/guvenlikolay/handlers.go:91-183`, `islemmerkezi/handlers.go:124` | arka plan ticker + cache; `audit_log(ip,ok,ts)` indeksi + temizlik |

## ORTA

- **Eksik indeksler:** `db_accounts.db_user` (0036'da unique düşürülmüş), `import_jobs/remote_transfer_jobs/laravel_deploy_jobs/av_taramalar` zaman kolonları, `av_bulgular(karantina,istisna,created_at)`.
- **Sayfalama opsiyonel:** parametre yoksa tüm satırlar dönüyor — `domains/handlers.go:157`, `users/handlers.go:159`, `accounts/accounts.go:70`; ayrıca `genelbakis`, `backups.Ozet`, `wordpress.TumListe` sınırsız.
- **Rate-limit bellekte:** restart'ta IP/hesap kilitleri sıfırlanıyor; anahtar başına map'ler 45 dk boyunca sınırsız büyüyor (`middleware/ratelimit.go:51,242`).
- **Çıkışta oturum iptali yok:** `auth_version` artırılmıyor, JWT süresi dolana kadar geçerli (`auth/handlers.go:289`).
- **Müşteri girişi TOTP'yi yok sayıyor** (`musteri/musteri.go:65-84`).
- **Kilit altında yavaş işler:** `system/cve.go:262` mutex'i 4.5 dk tarama boyunca tutuyor; `grav/matomo/*/indirici.go` HTTP indirmeyi kilit altında yapıyor (timeout'suz `http.DefaultClient`).
- **Sınırsız kuyruk:** `remote_job.go:89` her istekte goroutine açıyor; semafor 2 olsa da bekleyenler sınırsız.
- **SSRF:** `transfers/remote.go:312` loopback/RFC1918 kabul ediyor (admin), SSH pivot'u.
- **lftp script injection:** `backups/destination.go:204-294` yalnız `\` ve `"` kaçırıyor, `\n` ile `! komut` enjeksiyonu (SUSPICIOUS, lftp yok).
- **`runOut` context'siz** (`system/servis.go:142`): `/system/servisler` istek başına ~13 sıralı `systemctl`, asılırsa handler takılır.
- **Request yolunda context'siz DB** (`auth/profile.go`, `auth/apitoken.go`, `auth/dashboard.go`).
- **`io.Pipe` goroutine sızıntısı** erken dönüşte (`transfers/remote_job.go:441`).
- **TOCTOU:** app/wordpress kurulumlarında `os.MkdirAll` + root `chown -R`/`restorecon -R` symlink takasına açık.

## DÜŞÜK

- JWT'de `exp` zorunlu değil, `aud` yok; `PANEL_SECRET_KEY` tuzsuz tek SHA-256 (`config/config.go:72`).
- TOTP seed düz metin, replay güncellemesi atomik değil.
- `crypto/rand` hataları yutuluyor (webhook secret `git.go:79`, pma token `pma.go:27`).
- `KapsamSQL` alias'ı doğrulamıyor (bugün hepsi `"d"`, ileride risk).
- `chpasswd`'a `\n` enjeksiyonu (`auth/profile.go:118`).
- `phpext` baştaki `-` kabul ediyor → `pecl` argüman enjeksiyonu.
- Git webhook/mail reveal token URL path'inde, nginx access log'a düşüyor.

## İyi durumda olanlar (dokunma)

SQL injection bulunamadı (tüm dinamik tanımlayıcılar allowlist'li); frontend'de XSS yüzeyi yok, token `localStorage`'da değil, HttpOnly+SameSite=Strict+CSRF mevcut; `jailpath`/`safeio` openat2 tabanlı dosya erişimi doğru; arşiv çıkarma zip-slip'e kapalı; `secretcrypt` AES-256-GCM doğru nonce'lu; bcrypt cost 12; API token'ları 256-bit + SHA-256; `httpx.ClientIP` XFF spoof'a dirençli.

**Öncelik sırası:** C1 (`--binary-mode`, tek satırlık en yüksek etki) → C2 → H1 → H3/H2.
