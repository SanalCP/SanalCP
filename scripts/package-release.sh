#!/usr/bin/env bash
# Kaynak, binary, frontend ve migration arşivini tek ve doğrulanabilir release
# halinde üretir. Elle assets/* güncellemek yerine yalnız bu script kullanılmalı.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

command -v go >/dev/null || { echo "go bulunamadı" >&2; exit 1; }
command -v npm >/dev/null || { echo "npm bulunamadı" >&2; exit 1; }
command -v tar >/dev/null || { echo "tar bulunamadı" >&2; exit 1; }

# Release binary'sindeki stdlib, go.mod'daki dil sürümünden değil bu betiği
# çalıştıran TOOLCHAIN'den gelir. v0.9.42 yereldeki go1.25.0 ile paketlenip
# kaynak taraması temiz olduğu hâlde binary taramasında 47 stdlib açığı taşıdı.
# CI'nin kullandığı güncel ve yamalı alt sınırı paketleme anında da zorla.
GO_TOOLCHAIN="$(go env GOVERSION)"
GO_NUM="${GO_TOOLCHAIN#go}"
GO_MAJOR="${GO_NUM%%.*}"
GO_REST="${GO_NUM#*.}"
GO_MINOR="${GO_REST%%.*}"
GO_PATCH="${GO_REST#*.}"; GO_PATCH="${GO_PATCH%%[^0-9]*}"
case "$GO_MAJOR.$GO_MINOR.$GO_PATCH" in
  *[!0-9.]*) echo "geçersiz Go toolchain sürümü: $GO_TOOLCHAIN" >&2; exit 1 ;;
esac
if [ "$GO_MAJOR" -lt 1 ] || { [ "$GO_MAJOR" -eq 1 ] && { [ "$GO_MINOR" -lt 26 ] || { [ "$GO_MINOR" -eq 26 ] && [ "$GO_PATCH" -lt 7 ]; }; }; }; then
  echo "release için Go >= 1.26.7 gerekli; bulunan: $GO_TOOLCHAIN" >&2
  echo "örnek: GOTOOLCHAIN=go1.26.7 ./scripts/package-release.sh" >&2
  exit 1
fi
echo "== Release toolchain: $GO_TOOLCHAIN =="

EPOCH="${SOURCE_DATE_EPOCH:-$(git log -1 --format=%ct 2>/dev/null || date +%s)}"
BUILD_DATE="$(date -u -d "@$EPOCH" +%Y-%m-%d)"

echo "== Go format/test/vet =="
unformatted="$(find cmd internal scripts -type f -name '*.go' -exec gofmt -l {} +)"
if [ -n "$unformatted" ]; then
  echo "gofmt gerekli dosyalar:" >&2
  echo "$unformatted" >&2
  echo "düzeltmek için: find cmd internal scripts -type f -name '*.go' -exec gofmt -w {} +" >&2
  exit 1
fi
go test ./...
go vet ./...

echo "== Frontend temiz kurulum + test + lint + build =="
(cd frontend && npm ci && npm test && npm run lint && npm run build)

echo "== Panel nginx vhost'u (kanonik kaynak -> asset) =="
# _panel.conf'un TEK kaynagi internal/nginxconf/_panel.conf'tur: binary'ye
# //go:embed ile gomulur ve panel kurulu vhost'u ondan gunceller (bkz.
# provisioner.HealPanelVhostOnStartup). assets/ altindaki kopya yalnizca
# installer icindir (sanalcp-install.sh) ve buradan URETILIR.
cp internal/nginxconf/_panel.conf assets/nginx/_panel.conf
cmp -s internal/nginxconf/_panel.conf assets/nginx/_panel.conf ||
  { echo "panel conf kopyalanamadi" >&2; exit 1; }

echo "== Linux amd64/v1 binary =="
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1 \
  go build -trimpath -buildvcs=false \
  -ldflags "-s -w -X main.buildDate=$BUILD_DATE" \
  -o assets/sanalcp-server ./cmd/server
if [ -f scripts/seed_admin.go ]; then
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1 \
    go build -trimpath -buildvcs=false -ldflags "-s -w" \
    -o assets/sanalcp-seed-admin scripts/seed_admin.go
fi
BUILT_WITH="$(go version -m assets/sanalcp-server | awk 'NR==1 {print $2}')"
[ "$BUILT_WITH" = "$GO_TOOLCHAIN" ] || {
  echo "binary toolchain uyuşmazlığı: beklenen $GO_TOOLCHAIN, bulunan $BUILT_WITH" >&2
  exit 1
}

echo "== Deterministik frontend/migration arşivleri =="
tar --sort=name --mtime="@$EPOCH" --owner=0 --group=0 --numeric-owner \
  -czf assets/frontend-dist.tar.gz -C frontend/dist .
tar --sort=name --mtime="@$EPOCH" --owner=0 --group=0 --numeric-owner \
  -czf assets/migrations.tar.gz -C migrations .

echo "== Sürüm damgası =="
# assets/SURUM manifestin (dolayısıyla imzanın) kapsamındadır: sanalcp-update
# bununla imzalı ama ESKİ bir release'e geri döndürme (downgrade) saldırısını
# reddeder.
SURUM="$(sed -n 's/^const SurumNo = "\(.*\)"$/\1/p' internal/system/usage.go)"
[ -n "$SURUM" ] || { echo "SurumNo okunamadı (internal/system/usage.go)" >&2; exit 1; }
printf '%s\n' "$SURUM" > assets/SURUM

echo "== Asset bütünlük manifesti =="
find assets -type f ! -name SHA256SUMS ! -name SHA256SUMS.sig -print0 |
  LC_ALL=C sort -z |
  xargs -0 sha256sum > assets/SHA256SUMS
sha256sum -c assets/SHA256SUMS

echo "== Release imzası =="
# 🔴 TEDARİK ZİNCİRİ: SHA256SUMS aynı arşivin içinden geldiği için tek başına
# dışarıya karşı güvence vermez. sanalcp-update manifestin imzasını, sunucuya
# kurulumda yerleştirilen /etc/sanalcp/release-signers açık anahtarıyla doğrular;
# GitHub hesabı/deposu ele geçirilse bile imzasız bir release kurulmaz.
# Özel anahtar depoda DEĞİL, yayımcının makinesinde durur.
IMZA_ANAHTARI="${SANALCP_IMZA_ANAHTARI:-$HOME/.config/sanalcp/release-imza}"
[ -f "$IMZA_ANAHTARI" ] || {
  echo "release imza anahtarı yok: $IMZA_ANAHTARI (SANALCP_IMZA_ANAHTARI ile verin)" >&2
  exit 1
}
rm -f assets/SHA256SUMS.sig
# Parola korumalı anahtar: ssh-agent'a yüklüyse (SSH_AUTH_SOCK) imza agent
# üzerinden atılır ve parola sorulmaz — ssh-keygen -Y sign, -f açık anahtar
# dosyasını gösterdiğinde özel yarıyı agent'tan kullanır. Değilse ssh-keygen
# parolayı terminalden sorar.
IMZA_KAYNAGI="$IMZA_ANAHTARI"
if [ -f "$IMZA_ANAHTARI.pub" ] && [ -n "${SSH_AUTH_SOCK:-}" ] &&
   ssh-add -L 2>/dev/null | grep -qF "$(cut -d' ' -f2 "$IMZA_ANAHTARI.pub")"; then
  IMZA_KAYNAGI="$IMZA_ANAHTARI.pub"
  echo "  imza ssh-agent üzerinden atılıyor"
fi
ssh-keygen -Y sign -q -f "$IMZA_KAYNAGI" -n sanalcp-release assets/SHA256SUMS
ssh-keygen -Y verify -f assets/release-signers -I sanalcp-release -n sanalcp-release \
  -s assets/SHA256SUMS.sig < assets/SHA256SUMS >/dev/null ||
  { echo "imza assets/release-signers ile doğrulanamadı (yanlış anahtar?)" >&2; exit 1; }

# Paket eski migration/frontend taşıyorsa burada yayın kesilir.
src_migrations="$(find migrations -maxdepth 1 -type f -name '*.sql' -printf '%f\n' | LC_ALL=C sort)"
tar_migrations="$(tar -tzf assets/migrations.tar.gz | sed 's#^\./##' | sed '/^$/d' | LC_ALL=C sort)"
[ "$src_migrations" = "$tar_migrations" ] || {
  echo "migrations.tar.gz kaynakla eşleşmiyor" >&2
  exit 1
}
frontend_files="$(tar -tzf assets/frontend-dist.tar.gz)"
grep -qE '(^|/)index\.html$' <<<"$frontend_files" ||
  { echo "frontend index.html eksik" >&2; exit 1; }
grep -qE '(^|/)assets/index-.*\.js$' <<<"$frontend_files" ||
  { echo "frontend JS bundle eksik" >&2; exit 1; }

echo "✓ Release assetleri tek kaynak durumundan üretildi ve doğrulandı."
