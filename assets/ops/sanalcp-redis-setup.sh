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

# Aile tespiti + paket/servis adları (installer ile AYNI tablo).
ORTAK=/usr/local/bin/sanalcp-ortak
[ -f "$ORTAK" ] || ORTAK=/opt/sanalcp/src/scripts/sanalcp-ortak.sh
# shellcheck source=/dev/null
. "$ORTAK" 2>/dev/null || { echo "  ✗ sanalcp-ortak bulunamadı — panel güncellemesi eksik kalmış olabilir"; exit 1; }

echo "════ Redis/Valkey + php-redis kurulumu ════"

# Kurulu PHP sürümleri için redis eklentisi. Eklentinin paket adı ailelere göre
# değişir: Remi'de "php83-php-pecl-redis6", sury'de "php8.3-redis".
PHP_REDIS_EXT=$(debian_mi && echo redis || echo pecl-redis6)
PHP_REDIS_PKGS=""
for v in $(php_kurulu_surumler); do
  PHP_REDIS_PKGS="$PHP_REDIS_PKGS $(php_pkg "$v" "$PHP_REDIS_EXT")"
done

# 🔴 Paket adı üç değişkene birden bağlı: aile + dağıtım sürümü + kurulu olan.
# valkey AlmaLinux 9'da ve Debian 12'de YOKTUR (bkz. internal/osfam) — ikisinde
# de redis'e düşülür. paket_ad bunu bilir; yine de kurulum başarısız olursa
# çalışma anında redis'e düşülüyor (örn. valkey'i sunmayan bir ara sürüm).
KV_PKG=$(paket_ad cache)
if ! pkg_kur "$KV_PKG"; then
  KV_PKG=$(debian_mi && echo redis-server || echo redis)
  pkg_kur "$KV_PKG" || { log "✗ redis/valkey kurulamadı"; exit 1; }
fi
KV="${KV_PKG%-server}"        # valkey-server -> valkey · redis -> redis
# Sistem PHP'sinin redis eklentisi: RHEL'de temel "php" paketinin eklentisi
# (php-pecl-redis6), Debian'da sürüm adlı paket (php8.3-redis).
SYS_REDIS_PKG=$(debian_mi && echo php8.3-redis || echo php-pecl-redis6)
# shellcheck disable=SC2086
pkg_kur "$SYS_REDIS_PKG" $PHP_REDIS_PKGS
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

if selinux_var; then
  echo "════ SELinux (php-fpm → redis TCP) ════"
  setsebool -P httpd_can_network_connect 1 2>/dev/null && log "httpd_can_network_connect=1"
fi

echo "════ $KV_PKG enable + (re)start ════"
# Birim adı = paket adı (RHEL "valkey"/"redis", Debian "valkey-server"/
# "redis-server"). servis_ad yerine KURULAN paketten türetiliyor: yukarıdaki
# çalışma-anı fallback'i devreye girdiyse tablo hâlâ valkey der, sistemde ise
# redis vardır.
KV_SVC="$KV_PKG"
# Rakip cache servisi (redis vs valkey) varsa 6379'u ondan al — bkz.
# sanalcp-ortak.sh:rakip_onbellek_kapat.
rakip_onbellek_kapat
systemctl enable "$KV_SVC" >/dev/null 2>&1
systemctl restart "$KV_SVC"; sleep 2
if systemctl is-active --quiet "$KV_SVC" && REDISCLI_AUTH="$ADMIN" "$KV_BIN" PING 2>/dev/null | grep -q PONG; then
  log "✓ $KV_SVC ACTIVE + admin auth OK"
else
  log "✗ $KV_SVC başlatılamadı — journalctl -u $KV_SVC"
  exit 1
fi
echo "════════ ✓ Redis altyapısı hazır ($KV) ════════"
