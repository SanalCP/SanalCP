module sanalcp

go 1.24

// Go 1.24 dil sürümüne geçerken DAVRANIŞ değişikliklerini bilerek sabitliyoruz.
// (go direktifini bump'lamak, GODEBUG varsayılanlarını da 1.24'e taşır.)
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
// Bilerek YENİ davranışta bırakılanlar: rsa1024min (1024 bit altı RSA reddi —
// DKIM zaten 2048), tlsmlkem (yalnız monitor'ün dış probe'larını etkiler),
// x509usepolicies (CreateCertificate kullanılmıyor), randseednop (math/rand yok).
godebug (
	multipathtcp=0
	x509rsacrt=0
)

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/go-chi/chi/v5 v5.1.0
	github.com/go-sql-driver/mysql v1.8.1
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/openwall/yescrypt-go v1.0.0
	github.com/prometheus/client_golang v1.21.1
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	golang.org/x/crypto v0.31.0
	golang.org/x/sys v0.28.0
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	google.golang.org/protobuf v1.36.1 // indirect
)
