#!/usr/bin/env bash
# Kaynak, binary, frontend ve migration arşivini tek ve doğrulanabilir release
# halinde üretir. Elle assets/* güncellemek yerine yalnız bu script kullanılmalı.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

command -v go >/dev/null || { echo "go bulunamadı" >&2; exit 1; }
command -v npm >/dev/null || { echo "npm bulunamadı" >&2; exit 1; }
command -v tar >/dev/null || { echo "tar bulunamadı" >&2; exit 1; }

EPOCH="${SOURCE_DATE_EPOCH:-$(git log -1 --format=%ct 2>/dev/null || date +%s)}"
BUILD_DATE="$(date -u -d "@$EPOCH" +%Y-%m-%d)"

echo "== Go test/vet =="
go test ./...
go vet ./...

echo "== Frontend temiz kurulum + build =="
(cd frontend && npm ci && npm run build)

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

echo "== Deterministik frontend/migration arşivleri =="
tar --sort=name --mtime="@$EPOCH" --owner=0 --group=0 --numeric-owner \
  -czf assets/frontend-dist.tar.gz -C frontend/dist .
tar --sort=name --mtime="@$EPOCH" --owner=0 --group=0 --numeric-owner \
  -czf assets/migrations.tar.gz -C migrations .

echo "== Asset bütünlük manifesti =="
find assets -type f ! -name SHA256SUMS -print0 |
  LC_ALL=C sort -z |
  xargs -0 sha256sum > assets/SHA256SUMS
sha256sum -c assets/SHA256SUMS

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
