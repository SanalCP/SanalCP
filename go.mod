module sanalcp

go 1.25.0

// Go 1.25 dil sürümüne geçerken DAVRANIŞ değişikliklerini bilerek sabitliyoruz.
// (go direktifini bump'lamak, GODEBUG varsayılanlarını da 1.25'e taşır.)
//
// 1.25'e çıkış zorunluydu: prometheus/client_golang v1.24.1, golang.org/x/crypto
// v0.55.0 ve golang.org/x/sys v0.47.0 `go >= 1.25.0` istiyor. 1.24 hattı ise
// artık yama almıyor (1.27.0 çıkınca 1.24 ve 1.25 destekten düştü) — bu yüzden
// binary go1.26 toolchain'i ile derleniyor. Dil sürümünün 1.25'te kalması
// sorun değil: stdlib açıklarını kapatan şey, derleyen toolchain'dir.
//
//   multipathtcp=0 — 1.24'te dinleyiciler varsayılan olarak MPTCP soketi olur.
//     Ölçüldü: 1.23 ile 0, 1.24 ile 3 MPTCP soketi (panel + cliSrv + mail policy).
//     Panel kendi nftables kural setini yönettiği için bu etkileşimi bilerek
//     açmak gerekir; şimdilik eski davranış (düz TCP) korunuyor.
//
//   x509rsacrt=0 — 1.24'te ParsePKCS1PrivateKey, RSA özel anahtarındaki bozuk
//     CRT değerlerini yeniden hesaplamak yerine HATA döndürür. Müşterinin daha
//     önce sorunsuz içe aktardığı bir sertifika anahtarı bu yüzden aniden
//     reddedilebilirdi (bkz. provisioner/imported_ssl.go, ssl_heal.go).
//
// Bilerek YENİ davranışta bırakılanlar (1.24): rsa1024min (1024 bit altı RSA
// reddi — DKIM zaten 2048), tlsmlkem (yalnız monitor'ün dış probe'larını
// etkiler), x509usepolicies (CreateCertificate kullanılmıyor), randseednop
// (math/rand yok).
//
// 1.25'te değişen dört ayar tek tek incelendi, hiçbiri pin gerektirmedi:
//   tlssha1=0 — TLS 1.2 el sıkışmasında SHA-1 imza algoritmaları reddedilir.
//     Tek Go TLS el sıkışması monitor.go:158'deki dış probe; tlsmlkem ile aynı
//     sınıf, o da yeni davranışta bırakılmıştı. ACME acme.sh (kabuk) üzerinden
//     yürüdüğü için sertifika alımı etkilenmez.
//   x509sha256skid=1 — CreateCertificate, SubjectKeyId'yi SHA-1 yerine SHA-256
//     ile üretir. CreateCertificate yalnız _test.go dosyalarında çağrılıyor.
//   updatemaxprocs=1 — runtime, GOMAXPROCS'u cgroup CPU limitinden günceller.
//     assets/systemd/sanalcp.service'te CPUQuota yok; CPUQuota yalnız kiracı
//     FPM unit'lerine yazılıyor (internal/kaynaklimit), panelin kendisine değil.
//   decoratemappings=1 — anonim bellek eşlemeleri /proc/self/maps'te adlandırılır.
//     Yalnız gözlemlenebilirlik; davranış etkisi yok.
godebug (
	multipathtcp=0
	x509rsacrt=0
)

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/go-chi/chi/v5 v5.3.2
	github.com/go-sql-driver/mysql v1.10.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/openwall/yescrypt-go v1.0.0
	github.com/prometheus/client_golang v1.24.1
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	golang.org/x/crypto v0.55.0
	golang.org/x/sys v0.47.0
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)
