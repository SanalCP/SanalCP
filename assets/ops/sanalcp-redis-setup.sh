#!/usr/bin/env bash
# sanalcp-redis-setup — per-tenant izole Redis/Valkey altyapısını kurar.
# Idempotent. Kurulumda çalıştırılır; panelin "Redis Cache aç" özelliği bunu gerektirir.
#
# 🔴 AlmaLinux 10 AppStream, Redis'in protokol-uyumlu topluluk devamı Valkey'i
# sunar; AlmaLinux 9 (ve öncesi) hâlâ "redis" paketini sağlar. İkisi RESP
# protokolü ve komut seti bakımından birebir aynı — yalnız paket/birim/binary
# adı ve config dizini değişir. Script boyunca KV değişkeni hangisi kuruluysa
# onu taşır; elle "valkey" yazılmaz.
set -uo pipefail
log(){ printf '  %s\n' "$*"; }

echo "════ Redis/Valkey + php-redis kurulumu ════"
PHP_REDIS_PKGS=""
for v in php74 php80 php81 php82 php83 php84 php85; do
  [ -d "/etc/opt/remi/$v" ] && PHP_REDIS_PKGS="$PHP_REDIS_PKGS ${v}-php-pecl-redis6"
done
KV=valkey
if ! dnf install -y valkey >/tmp/redis-setup.log 2>&1; then
  KV=redis
  dnf install -y redis >>/tmp/redis-setup.log 2>&1 || { log "✗ redis/valkey kurulamadı"; cat /tmp/redis-setup.log; exit 1; }
fi
dnf install -y php-pecl-redis6 $PHP_REDIS_PKGS >>/tmp/redis-setup.log 2>&1
log "$KV + php-redis kuruldu"

KV_BIN="${KV}-cli"
KV_ETC="/etc/$KV"
KV_CONF="$KV_ETC/$KV.conf"
KV_ACL="$KV_ETC/users.acl"

echo "════ Admin parolası + ACL dosyası ════"
ENV=/etc/sanalcp/env
ADMIN=$(grep -oP '^PANEL_REDIS_ADMIN_PASS=\K.*' "$ENV" 2>/dev/null)
if [ -z "$ADMIN" ]; then
  ADMIN=$(openssl rand -hex 24)
  echo "PANEL_REDIS_ADMIN_PASS=${ADMIN}" >> "$ENV"
  log "admin parolası üretildi ve env'e eklendi"
fi
# users.acl'de default admin satırı yoksa yaz (mevcut tenant ACL'lerini KORU)
if [ ! -f "$KV_ACL" ] || ! grep -q '^user default ' "$KV_ACL"; then
  printf 'user default on >%s ~* &* +@all\n' "$ADMIN" > "$KV_ACL"
  log "users.acl oluşturuldu"
fi
chown "$KV:$KV" "$KV_ACL" 2>/dev/null; chmod 640 "$KV_ACL"

echo "════ $KV.conf cache tuning ════"
if ! grep -q 'sanalcp-cache' "$KV_CONF"; then
cat >> "$KV_CONF" <<VK

# ===== sanalcp-cache =====
maxmemory 256mb
maxmemory-policy allkeys-lru
save ""
appendonly no
aclfile $KV_ACL
VK
  log "$KV.conf tuning eklendi"
fi
sed -i '/^requirepass /d' "$KV_CONF"   # aclfile ile çakışır

echo "════ SELinux (php-fpm → redis TCP) ════"
setsebool -P httpd_can_network_connect 1 2>/dev/null && log "httpd_can_network_connect=1"

echo "════ $KV enable + (re)start ════"
systemctl enable "$KV" >/dev/null 2>&1
systemctl restart "$KV"; sleep 2
if systemctl is-active --quiet "$KV" && REDISCLI_AUTH="$ADMIN" "$KV_BIN" PING 2>/dev/null | grep -q PONG; then
  log "✓ $KV ACTIVE + admin auth OK"
else
  log "✗ $KV başlatılamadı — journalctl -u $KV"
  exit 1
fi
echo "════════ ✓ Redis altyapısı hazır ($KV) ════════"
