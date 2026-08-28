#!/usr/bin/env bash
# sanalcp-recovery-check için dış servislere dokunmayan sözleşme testi.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d /tmp/sanalcp-recovery-check.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT
MOCKBIN="$WORK/bin"; DBDIR="$WORK/db"
mkdir -p "$MOCKBIN" "$DBDIR"

cat > "$WORK/panel.sql" <<'EOF'
CREATE DATABASE /*!32312 IF NOT EXISTS*/ `panel`;
USE `panel`;
CREATE TABLE `users` (`id` bigint);
INSERT INTO `users` VALUES (1);
EOF
gzip -c "$WORK/panel.sql" > "$DBDIR/panel-2099-01-01-000000.sql.gz"

for cmd in systemctl curl nginx; do
  cat > "$MOCKBIN/$cmd" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
done
cat > "$MOCKBIN/mysql" <<'EOF'
#!/usr/bin/env bash
if [[ " $* " == *" information_schema.tables "* ]]; then echo 1; exit 0; fi
cat >/dev/null
exit 0
EOF
chmod +x "$MOCKBIN"/*

OUT="$(PATH="$MOCKBIN:$PATH" SANAL_DBDIR="$DBDIR" SANAL_HEALTH=http://test \
  bash "$ROOT/assets/ops/sanalcp-recovery-check" --restore-drill --json)"
grep -q '"ok":true' <<< "$OUT"
grep -q '"code":"restore_drill"' <<< "$OUT"
grep -q 'geçici DB' <<< "$OUT"
echo "✓ kurtarma sağlık kontrolü sözleşme testi geçti"
