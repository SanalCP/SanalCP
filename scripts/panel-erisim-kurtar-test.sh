#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d /tmp/sanalcp-panel-erisim.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT

cat > "$WORK/mysql" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" > "$SANALCP_TEST_OUT"
SH
chmod +x "$WORK/mysql"
SANALCP_TEST_OUT="$WORK/out" SANALCP_MYSQL_BIN="$WORK/mysql" SANALCP_EFFECTIVE_UID=0 \
  bash "$ROOT/assets/ops/sanalcp-panel-erisim-kurtar" --disable > "$WORK/stdout"
grep -q "UPDATE panel_ayarlari SET erisim_cidrleri=NULL" "$WORK/out"
grep -q "kısıtı kapatıldı" "$WORK/stdout"

if SANALCP_TEST_OUT="$WORK/out2" SANALCP_MYSQL_BIN="$WORK/mysql" SANALCP_EFFECTIVE_UID=1000 \
  bash "$ROOT/assets/ops/sanalcp-panel-erisim-kurtar" --disable >/dev/null 2>&1; then
  echo "root olmayan kullanıcı kabul edildi" >&2; exit 1
fi
test ! -e "$WORK/out2"
echo "✓ panel erişim CLI kurtarma testi geçti"
