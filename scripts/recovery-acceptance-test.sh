#!/usr/bin/env bash
# İzole kurtarma kabul testi. Gerçek systemd, MariaDB, /opt veya /home'a dokunmaz.
# Sağlıksız canonical sürüm senaryosunu zorlar ve core+DB rollback sözleşmesini sınar.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d /tmp/sanalcp-recovery-test.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT

PREFIX="$WORK/opt/sanalcp"
ASSETROOT="$WORK/release"
ASSETS="$ASSETROOT/assets"
MOCKBIN="$WORK/mockbin"
STATE="$WORK/state"
mkdir -p "$PREFIX/bin" "$PREFIX/frontend-dist/assets" "$PREFIX/src/migrations" \
  "$ASSETS/ops" "$MOCKBIN" "$STATE"

# Eski kurulum durumu.
printf '\177ELFOLD' > "$PREFIX/bin/sanalcp-server"
truncate -s 1200000 "$PREFIX/bin/sanalcp-server"
chmod +x "$PREFIX/bin/sanalcp-server"
printf 'old frontend\n' > "$PREFIX/frontend-dist/index.html"
printf 'old js\n' > "$PREFIX/frontend-dist/assets/index-old.js"
printf '%s\n' 'SELECT 1;' > "$PREFIX/src/migrations/001_old.sql"
OLD_BIN_SHA="$(sha256sum "$PREFIX/bin/sanalcp-server" | cut -d' ' -f1)"

# Canonical fakat sağlık kontrolünden geçmeyecek yeni release.
printf '\177ELFNEW' > "$ASSETS/sanalcp-server"
truncate -s 1200000 "$ASSETS/sanalcp-server"
chmod +x "$ASSETS/sanalcp-server"
mkdir -p "$WORK/new-frontend/assets" "$WORK/new-migrations"
printf 'new frontend\n' > "$WORK/new-frontend/index.html"
printf 'new js\n' > "$WORK/new-frontend/assets/index-new.js"
printf '%s\n' 'SELECT 2;' > "$WORK/new-migrations/002_new.sql"
tar czf "$ASSETS/frontend-dist.tar.gz" -C "$WORK/new-frontend" .
tar czf "$ASSETS/migrations.tar.gz" -C "$WORK/new-migrations" .
cp "$ROOT/assets/ops/sanalcp-restore" "$ASSETS/ops/sanalcp-restore"
(cd "$ASSETROOT" && find assets -type f ! -name SHA256SUMS -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > assets/SHA256SUMS)

# Geçerli görünen pre-repair dump ve onu döndüren yedek aracı.
printf '%s\n' 'CREATE DATABASE panel;' | gzip -c > "$STATE/predump.sql.gz"
cat > "$MOCKBIN/db-backup" <<EOF
#!/usr/bin/env bash
printf '%s\\n' '$STATE/predump.sql.gz'
EOF
cat > "$MOCKBIN/systemctl" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
  is-active) exit 0 ;;
  *) exit 0 ;;
esac
EOF
cat > "$MOCKBIN/curl" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
cat > "$MOCKBIN/mysql" <<EOF
#!/usr/bin/env bash
cat > '$STATE/mysql-input.sql'
EOF
# GitHub Actions runner'ı root değildir. Test gerçek /opt, systemd veya MariaDB'ye
# dokunmadığı hâlde restore'un üretim root kapısı doğal olarak erken çıkardı.
# Yalnız izole PATH içinde `id -u`yu root gibi göster; diğer id çağrılarını gerçek
# binary'ye geçir.
cat > "$MOCKBIN/id" <<'EOF'
#!/usr/bin/env bash
if [ "${1:-}" = "-u" ]; then echo 0; else exec /usr/bin/id "$@"; fi
EOF
chmod +x "$MOCKBIN"/*

set +e
PATH="$MOCKBIN:$PATH" \
SANAL_ASSETS_OVERRIDE="$ASSETS" \
SANAL_PREFIX="$PREFIX" \
SANAL_OPSBIN="$WORK/opsbin" \
SANAL_SCRIPTS="$PREFIX/src/scripts" \
SANAL_DBBK="$MOCKBIN/db-backup" \
SANAL_SVC="sanalcp-test" \
SANAL_HEALTH="http://127.0.0.1:1/healthz" \
SANAL_HEALTH_RETRIES=1 SANAL_HEALTH_SLEEP=0 SANAL_ROLLBACK_SLEEP=0 \
  bash "$ROOT/assets/ops/sanalcp-restore" > "$STATE/output.log" 2>&1
RC=$?
set -e

# Restore, rollback yaptığını bildirmek için non-zero çıkmalıdır.
[ "$RC" -ne 0 ] || { echo "✗ sağlıksız release başarı sayıldı" >&2; exit 1; }
[ "$(sha256sum "$PREFIX/bin/sanalcp-server" | cut -d' ' -f1)" = "$OLD_BIN_SHA" ] || \
  { echo "✗ binary geri alınmadı" >&2; exit 1; }
grep -qx 'old frontend' "$PREFIX/frontend-dist/index.html" || \
  { echo "✗ frontend geri alınmadı" >&2; exit 1; }
[ -f "$PREFIX/src/migrations/001_old.sql" ] && [ ! -e "$PREFIX/src/migrations/002_new.sql" ] || \
  { echo "✗ migration dizini geri alınmadı" >&2; exit 1; }
grep -q 'CREATE DATABASE panel' "$STATE/mysql-input.sql" || \
  { echo "✗ DB dump geri yükleme hattına verilmedi" >&2; exit 1; }
grep -q 'rollback yapıldı' "$STATE/output.log" || \
  { echo "✗ rollback sonucu raporlanmadı" >&2; exit 1; }

echo "✓ kurtarma kabul testi geçti: binary + frontend + migration + DB rollback"
