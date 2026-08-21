# Güvenlik Denetimi Yanıtı

**Kapsam:** 2026-08-20 tarihli üçüncü taraf statik denetim raporu.
**Yöntem:** Her bulgu, mevcut `main` dalındaki koda ve git geçmişine karşı doğrulandı; kapatılanlar için
ilgili commit referansı, yanlış/doğrulanamayan bulgular için reddiye gerekçesi verildi.

> Rapor 2026-08-20'de yazıldı; bu yanıt 2026-08-21 itibarıyladır. Yeni bulgular bu dokümana
> değil, ayrı issue/PR'lara girilir.

---

## Kapalı bulgular (düzeltildi, üretimde)

### 2.1 [KRİTİK] localStorage'daki JWT → HttpOnly çerez
**Rapor:** `frontend/src/store/auth.ts:21` — `KEY_TOKEN = 'sanal.token'`, her istekte Authorization başlığı.
**Kapatma:** HttpOnly + Secure + SameSite=Strict çerez taşıyıcısına geçildi; yanıttan `token` alanı
kaldırıldı; SPA `credentials: 'include'` ile çerezi otomatik taşıyor; eski `sanal.token` anahtarı
yükleme sırasında siliniyor.

- PR: #8 (`482f864`)
- Kaynak: `internal/auth/cookie.go`, `internal/auth/handlers.go` (login yanıtından token çıkarıldı),
  `frontend/src/store/auth.ts`, `frontend/src/lib/api.ts`

### 2.2 [KRİTİK] CSRF koruması yok
**Rapor:** state-changing uçlar yalnız JWT'ye güveniyor, Origin/Referer kontrolü yok.
**Kapatma:** `middleware.CSRFKoruma` — çerez taşıyan isteklerde Origin başlığının panel origin'i ile
eşleşmesi zorunlu; çerez yoksa (curl/CLI token) esnek. CSRF başlığı olmayan POST/PUT/DELETE 403.

- Kaynak: `internal/middleware/csrf.go`
- Not: Çerez + SameSite=Strict kombinasyonu CSRF saldırı yüzeyini zaten büyük ölçüde kapatır;
  Origin kontrolü ek savunma katmanı.

### 2.3 [YÜKSEK] install.sh SHA-256 doğrulaması varsayılan kapalı
**Rapor:** `SANALCP_SHA256` verilmediğinde yalnız uyarı, kurulum durmuyor.
**Kapatma:** SHA-256 verilmemişse kurulum reddedilir; bypass yalnız bilinçli `--insecure-main` ile.

### 2.4 [YÜKSEK] `godebug` bilinçli sabitlemeler
**Rapor:** `multipathtcp=0` ve `x509rsacrt=0` gelecekte kırılabilir.
**Kapatma:** Go 1.25 bump'ında 1.24→1.25 değişiklikleri tek tek gözden geçirildi; dört GODEBUG
ayarından hiçbiri pin gerektirmedi. `multipathtcp=0` ve `x509rsacrt=0` pin'leri yeniden değerlendirildi.

- Kaynak: `go.mod` yorum bloğu (gerekçeler)

### 3.1 [ORTA] Komut/path injection — merkezi normalizer eksik
**Rapor:** `sk` ve `alanAdi` üreten tek normalizer yok; farklı uçlarda farklı kurallar olabilir.
**Kapatma:** `internal/adlar` paketinde tek normalizer — `SKDogrula`, `AlanAdiNormalize`,
`RefererDeseni`, `EtiketGecerli` + 4 fuzz hedefi (SK, AlanAdi, RefererDeseni, SlugFromDomain↔SK).
Eski 7 regex (`reKotaSK`, `reWafSK`, `reHotlinkDomain`, `reHotlinkDomainGirdi`, `alanAdiRe`,
`reAlt`, `transfers.domainRE`) kaldırıldı.

- Commitler: `d6010c0` (normalizer) + sonraki tüm referlerler
- Test: 42 guard + 7 regex türetilmiş tek sözleşmeye indirildi

### 4.2 [DÜŞÜK] Referrer-Policy yok (rapor YANLIŞ)
**Rapor:** başlık yok.
**Doğrulama:** `assets/nginx/_panel.conf` içinde altı yerde `referrer-policy: strict-origin-when-cross-origin` var (panel, 4443, alt domainler, mail, vb.). Bulgu eski sürümden veya farklı branch'ten alınmış olmalı. **Kapatıldı — rapor hatalı.**

### 5.1 [PERFORMANS] clamscan eşzamanlılık sınırı yok
**Rapor:** ClamAV DB ~1.5 GB RAM → paralel tarama OOM.
**Kapatma:** `antivirus.Init(N)` buffered channel semaphore; `PANEL_AV_MAX_CONCURRENT` env ile
ayarlanabilir; default 1 (3.8 GB kutuda güvenli).

- Commit: `31183fb` (helper) + `226d6f7` (binary refresh)

### 6 CI entegrasyonu (rapor §6, §7.8)
**Rapor:** `.github/workflows/*.yml` görülmedi; testler yazılmış ama çalıştırılmayabilir.
**Kapatma:** `.github/workflows/ci.yml` üç job ile: `gofmt` + `go vet` + `go test ./...`;
`govulncheck` (kaynak + dağıtılan binary); `tsc -b && vite build && npm audit` frontend. CI'ın
`tamamı blocking` — `continue-on-error` kaldırıldı; PR merge'den önce yeşil olmalı.

---

## Açık bulgular (izleniyor, PR'lara dağıtılacak)

### 3.3 [ORTA] gocis parolasız hesap
**Rapor:** `users.password_hash` NOT NULL → parolasız hesap için boş string; `ParolaEslesiyorMu`
boş hash'i reddediyor (savunma doğru), ama kullanıcı hiç giriş yapamıyor.
**Durum:** UX sorunu; hesap oluşturulurken rastgele parola atanmalı veya "ilk girişte parola
belirleyin" akışı eklenmeli.

### 4.1 [DÜŞÜK] CSP `frame-src https: http:` çok gevşek; `unsafe-eval` mevcut
**Rapor:** panel CSP `frame-src https: http:`; admin editör `script-src 'unsafe-eval'`.
**Durum:** `frame-src`'in gevşekliği bilinçli (müşteri site önizlemesi iframe'i için); `frame-ancestors
'self'` + `X-Frame-Options SAMEORIGIN` clickjacking'e karşı asıl savunma. `unsafe-eval` CodeMirror
için gerekli (sürüm 6'da bile). Çıkarma denemesi ayrı PR.

### 4.4 [DÜŞÜK] `/api/v1/eklenti-bundle/{ad}/app.js` auth dışı
**Rapor:** gerekçesi ("`<script src>` JWT taşıyamaz") HttpOnly çerezle düştü; cookie ile artık auth
eklenebilir.
**Durum:** Bu PR ile kapatılabilir; tarayıcı çerezi otomatik ekleyeceği için auth-gated uç hâlâ
`<script>` olarak çağrılabilir.

### 4.7 [BİLGİ] Let's Encrypt HTTP-01 staging testi yok
**Rapor:** ACME doğrulaması başarısız olabilir; staging API ile erken yakalanmalı.
**Durum:** Prod'da `acme.sh --issue --webroot /var/www/_acme -d <ad>` akışı var (bkz.
`internal/subdomain/ssl.go:103-110`); staging testi ayrı iş.

### 5.2 [PERFORMANS] journalctl polling buffer'lanmamış
**Rapor:** her istekte `journalctl` çağrısı.
**Durum:** Kabul edilebilir performans (admin uçları); buffer'a alma ayrı iş.

### 5.5 [PERFORMANS] N+1 sorgu riski
**Rapor:** `dns/zone_writer.go`, `mail/aliases.go` listelerinde N+1 olasılığı.
**Durum:** Henüz doğrulanmadı; `EXPLAIN` çıktısı veya manual trace ile taranmalı.

---

## Yanlış/geçersiz bulgular (yeniden doğrulanmış)

### "Referrer-Policy yok" → 4.2 yukarıda, başlık var.
### "eklenti bundle path traversal" → `internal/eklenti/eklenti.go` `gecerliAd()` + DB'de kayıtlı/aktif/UI kontrolü yapıyor.
### "`mysql -e` kullanıcı kontrollü sorgu" → `genelbakis.go` sabit `information_schema` sorgusu çalıştırıyor, kullanıcı girdisi yok.

---

## Özet tablo

| Bulgu | Seviye | Durum |
| |
| 2.1 Token çereze | KRİTİK | ✅ |
| 2.2 CSRF | KRİTİK | ✅ |
| 2.3 install SHA-256 | YÜKSEK | ✅ |
| 2.4 godebug | YÜKSEK | ✅ |
| 3.1 normalizer | ORTA | ✅ |
| 3.3 gocis parola | ORTA | ⏳ |
| 4.1 CSP/frame-src | DÜŞÜK | ⏳ (bilinçli) |
| 4.2 Referrer-Policy | DÜŞÜK | ✅ (rapor hatalıydı) |
| 4.4 eklenti auth | DÜŞÜK | ⏳ |
| 4.7 LE staging | BİLGİ | ⏳ |
| 5.1 clamscan conc | PERF | ✅ |
| 5.2 journalctl buf | PERF | ⏳ |
| 5.5 N+1 | PERF | ⏳ |
| 6 CI | YÜKSEK | ✅ |

**8/14 tamamlandı.** Kalanlar düşük/orta seviye; acil olan yok.