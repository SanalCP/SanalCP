#!/usr/bin/env bash
# sanalcp-install — turns a blank AlmaLinux 10 server into a full SanalCP install.
# Designed to be idempotent (safe to re-run). Run as root.
#
#   ./sanalcp-install.sh [--admin-parola <p>] [--admin-eposta <e>] [--lang tr|en]
#
# assets/ must sit next to this script:
#   sanalcp-server  sanalcp-seed-admin  frontend-dist.tar.gz
#   migrations.tar.gz  nginx/*  php-fpm/*  phpmyadmin/*  systemd/*  ops/*  ssh/*
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
A="$HERE/assets"
ADMIN_PAROLA=""; ADMIN_EPOSTA="admin@local"; PANEL_LANG=""
while [ $# -gt 0 ]; do case "$1" in
  --admin-parola) shift; ADMIN_PAROLA="$1" ;;
  --admin-eposta) shift; ADMIN_EPOSTA="$1" ;;
  --lang) shift; PANEL_LANG="$1" ;;
  *) echo "unknown argument: $1"; exit 2 ;;
esac; shift; done

c_g="\033[32m"; c_y="\033[33m"; c_r="\033[31m"; c_b="\033[1;34m"; c_0="\033[0m"
[ -t 1 ] || { c_g=; c_y=; c_r=; c_b=; c_0=; }
step(){ echo -e "\n${c_b}══ $* ══${c_0}"; }
ok(){ echo -e "  ${c_g}✓${c_0} $*"; }
warn(){ echo -e "  ${c_y}!${c_0} $*"; }
die(){ echo -e "  ${c_r}✗ $*${c_0}"; exit 1; }
# curl'un önce IPv6 seçtiği bazı VPS ağlarında AAAA çözümlemesi sessizce boş
# indirmeye yol açabilir: normal denemeden sonra IPv4'e düş.
download(){
  local url="$1" out="$2"
  curl -fsSL --retry 3 --connect-timeout 15 -o "$out" "$url" ||
    curl -4fsSL --retry 3 --connect-timeout 15 -o "$out" "$url"
}

[ "$(id -u)" = 0 ] || die "root required"
[ -d "$A" ] || die "assets/ not found ($A)"
if [ -f "$A/SHA256SUMS" ]; then
  (cd "$HERE" && sha256sum -c assets/SHA256SUMS >/dev/null) ||
    die "release asset integrity check failed"
  ok "release SHA-256 integrity"
else
  die "assets/SHA256SUMS not found — missing or hand-assembled release"
fi
grep -qiE "AlmaLinux|Rocky|Red Hat|CentOS" /etc/os-release || warn "expected AlmaLinux/RHEL10 — continuing anyway"

# ============ 0) PANEL LANGUAGE ============
# curl | bash pipes the SCRIPT into stdin, not the user's keystrokes — so `read`
# must be pointed at /dev/tty explicitly, or it would read from the script body
# itself. If there's no TTY (fully unattended run) fall back to Turkish (project
# default) without prompting.
if [ -z "$PANEL_LANG" ]; then
  if [ -r /dev/tty ]; then
    echo -e "${c_b}Select the panel's default language / Panelin varsayılan dilini seçin:${c_0}"
    echo "  [1] English"
    echo "  [2] Türkçe"
    printf "  > "
    LANG_CHOICE=""
    read -r LANG_CHOICE < /dev/tty || true
    LANG_CHOICE="${LANG_CHOICE%$'\r'}"
    # Yalnız açıkça İngilizce seçimi "en" yapar; tanınmayan/boş girdi de dahil
    # geri kalan her şey Türkçe'ye düşer (projenin geneldeki varsayılanıyla tutarlı —
    # bkz. TTY olmayan koldaki PANEL_LANG="tr").
    case "$LANG_CHOICE" in
      1|en|EN|English|english) PANEL_LANG="en" ;;
      *) PANEL_LANG="tr" ;;
    esac
  else
    PANEL_LANG="tr"
  fi
fi
ok "panel default language: $PANEL_LANG"

PHP_VERS="74 80 81 82 83 84 85"
PHP_EXT="fpm cli mysqlnd mbstring bcmath intl gd soap opcache pdo xml zip pgsql ldap"

# ============ 1) REPOSITORIES ============
step "1) Repositories (EPEL + Remi + CRB)"
dnf install -y epel-release >/dev/null 2>&1 && ok "EPEL"
rpm -q remi-release >/dev/null 2>&1 || dnf install -y https://rpms.remirepo.net/enterprise/remi-release-10.rpm >/dev/null 2>&1
rpm -q remi-release >/dev/null 2>&1 && ok "Remi" || die "could not add Remi"
dnf config-manager --set-enabled crb >/dev/null 2>&1 && ok "CRB"

# ============ 2) BASE PACKAGES ============
step "2) Base packages"
dnf install -y nginx httpd mariadb-server valkey certbot python3-certbot-nginx \
  clamav clamav-update httpd-tools mod_proxy_html tar openssl policycoreutils-python-utils \
  setools-console jq bind bind-utils nftables unzip zip cronie xfsprogs sudo \
  bubblewrap rsync git curl acl lftp sshpass >/dev/null 2>&1 \
  && ok "nginx, httpd, mariadb, valkey, certbot, clamav, bind, nftables, unzip/zip, bubblewrap, acl, lftp, sshpass, tools" || die "base package install"

# RAR extractor (file manager .rar extract) — PRIMARY: bsdtar (libarchive, in the
# base repo, reliably reads RAR/RAR5 and itself rejects path-traversal). 🔴 NOTE:
# AlmaLinux 10's default `7z` (7-Zip 26.02) does NOT include the RAR codec →
# unusable here. Fall back to unar/unrar if bsdtar is unavailable.
if command -v bsdtar >/dev/null 2>&1 || command -v unar >/dev/null 2>&1 || command -v unrar >/dev/null 2>&1; then
  ok "RAR extractor available ($(command -v bsdtar unar unrar 2>/dev/null | head -1))"
elif dnf install -y bsdtar >/dev/null 2>&1; then
  ok "bsdtar (libarchive — rar/rar5/zip/7z extract)"
elif dnf install -y unar >/dev/null 2>&1 || dnf install -y unrar >/dev/null 2>&1; then
  ok "unar/unrar (rar extract)"
else
  warn "could not install a RAR extractor — file manager .rar extract disabled (zip/tar still work)"
fi

# ============ 2b) DISK QUOTA (XFS user quota — CloudLinux parity) ============
# Per-tenant disk + inode quota is enforced via XFS *user* quota (files are owned
# c_<sk>:c_<sk> → user quota maps 1:1 + escape-proof). If root fs is XFS with
# `noquota`, the quota can only be turned on at MOUNT time (no live remount) → write
# `rootflags=uquota` to GRUB. On a fresh install the quota comes up active after the
# post-install reboot.
step "2b) Disk quota (XFS user quota)"
dnf install -y quota xfsprogs >/dev/null 2>&1 && ok "quota + xfsprogs" || warn "quota packages skipped"
ROOTFS_TYPE=$(findmnt -no FSTYPE / 2>/dev/null || echo "")
ROOTFS_OPTS=$(findmnt -no OPTIONS / 2>/dev/null || echo "")
if [ "$ROOTFS_TYPE" != "xfs" ]; then
  warn "root fs is not XFS ($ROOTFS_TYPE) — XFS disk quota skipped"
elif echo "$ROOTFS_OPTS" | grep -qwE 'usrquota|uquota|quota'; then
  ok "root XFS user quota already active"
else
  if grep -q 'rootflags=uquota' /etc/default/grub 2>/dev/null; then
    ok "GRUB rootflags=uquota already present"
  else
    if grep -q '^GRUB_CMDLINE_LINUX=' /etc/default/grub 2>/dev/null; then
      sed -i 's/^\(GRUB_CMDLINE_LINUX="[^"]*\)"/\1 rootflags=uquota"/' /etc/default/grub
    else
      echo 'GRUB_CMDLINE_LINUX="rootflags=uquota"' >> /etc/default/grub
    fi
    # Also update existing boot entries (BLS) + regenerate grub.cfg (BIOS + EFI).
    command -v grubby >/dev/null 2>&1 && grubby --update-kernel=ALL --args="rootflags=uquota" >/dev/null 2>&1 || true
    grub2-mkconfig -o /boot/grub2/grub.cfg >/dev/null 2>&1 || true
    for cfg in /boot/efi/EFI/*/grub.cfg; do [ -f "$cfg" ] && grub2-mkconfig -o "$cfg" >/dev/null 2>&1 || true; done
    ok "GRUB rootflags=uquota added (root is XFS)"
  fi
  warn "Disk quota needs a ONE-TIME reboot to take effect (root fs quota can't be enabled via remount)."
fi

# ============ 3) PHP (5 versions + base + wp-cli) ============
step "3) PHP versions (5 Remi + base) + wp-cli"
BASE_PKGS="php php-fpm php-cli php-mysqlnd php-mbstring php-json php-pecl-zip php-pecl-redis6"
# 🔴 BEFORE the PHP batch install: disable dnf's auto-lock sources (if
#    dnf-automatic/makecache timers are on, a bulk "dnf install" can hit lock
#    contention / false negatives). Managed panel updates handle this themselves;
#    auto-update stays OFF (avoids lock contention + surprise patches).
systemctl disable --now dnf-automatic.timer dnf-makecache.timer >/dev/null 2>&1 || true
dnf install -y $BASE_PKGS >/dev/null 2>&1 && ok "base php + php-redis"
for v in $PHP_VERS; do
  pkgs=""; for e in $PHP_EXT; do pkgs="$pkgs php$v-php-$e"; done
  dnf install -y $pkgs php$v-php-pecl-redis6 >/dev/null 2>&1 && ok "php$v (+redis)" || warn "php$v — some packages skipped"
done
if [ ! -x /usr/local/bin/wp ]; then
  curl -fsSL -o /usr/local/bin/wp https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar 2>/dev/null \
    && chmod +x /usr/local/bin/wp && ok "wp-cli" || warn "wp-cli download failed (needed for WordPress features)"
else ok "wp-cli (already present)"; fi

# ============ 4) MARIADB ============
step "4) MariaDB"
systemctl enable --now mariadb >/dev/null 2>&1; sleep 2
systemctl is-active --quiet mariadb || die "MariaDB did not start"
# Idempotent re-run: if /etc/sanalcp/env already exists, KEEP the existing secrets
# (DB password, JWT secret, redis admin password) — don't regenerate them.
# Otherwise every re-run would change the MariaDB panel password, but the
# CURRENTLY RUNNING panel process (which only reads the env file at startup)
# wouldn't know — once its idle DB connections time out, new connections fail with
# "Access denied", breaking every query including login (2FA's fail-closed check
# then shows the misleading "could not verify 2FA status" — the real cause is the
# password mismatch).
if [ -s /etc/sanalcp/env ]; then
  DBPASS=$(sed -n 's/^PANEL_DB_DSN=panel:\([^@]*\)@.*/\1/p' /etc/sanalcp/env)
  JWT=$(sed -n 's/^PANEL_JWT_SECRET=//p' /etc/sanalcp/env)
  SECRETKEY=$(sed -n 's/^PANEL_SECRET_KEY=//p' /etc/sanalcp/env)
  RADMIN=$(sed -n 's/^PANEL_REDIS_ADMIN_PASS=//p' /etc/sanalcp/env)
fi
[ -n "${DBPASS:-}" ]    || DBPASS=$(openssl rand -hex 16)
[ -n "${JWT:-}" ]       || JWT=$(openssl rand -hex 32)
[ -n "${SECRETKEY:-}" ] || SECRETKEY=$(openssl rand -hex 32)
[ -n "${RADMIN:-}" ]    || RADMIN=$(openssl rand -hex 24)
mysql -u root <<SQL
CREATE DATABASE IF NOT EXISTS panel CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'panel'@'127.0.0.1' IDENTIFIED BY '$DBPASS';
ALTER USER 'panel'@'127.0.0.1' IDENTIFIED BY '$DBPASS';
GRANT ALL PRIVILEGES ON panel.* TO 'panel'@'127.0.0.1';
FLUSH PRIVILEGES;
SQL
ok "panel DB + user (panel@127.0.0.1)"

# ============ 5) DIRECTORIES + ENV ============
step "5) Directories + env"
mkdir -p /opt/sanalcp/bin /opt/sanalcp/frontend-dist /opt/sanalcp/src/migrations \
         /opt/sanalcp/src/mail-templates /opt/sanalcp/src/scripts /opt/sanalcp/pma-signon \
         /etc/sanalcp /etc/ssl/sanalcp
cat > /etc/sanalcp/env <<ENV
PANEL_LISTEN=127.0.0.1:8080
PANEL_ENV=production
PANEL_DB_DSN=panel:${DBPASS}@tcp(127.0.0.1:3306)/panel?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci
PANEL_JWT_SECRET=${JWT}
PANEL_JWT_LIFETIME_SEC=43200
PANEL_SECRET_KEY=${SECRETKEY}
PANEL_REDIS_ADMIN_PASS=${RADMIN}
ENV
chmod 600 /etc/sanalcp/env
ok "/etc/sanalcp/env (JWT + secret key + DB DSN + Redis admin — kept if present, generated otherwise)"

# ============ 6) ARTIFACT DEPLOY ============
step "6) Panel binary + frontend + migrations"
install -m 0755 "$A/sanalcp-server" /opt/sanalcp/bin/sanalcp-server
[ -f "$A/sanalcp-seed-admin" ] && install -m 0755 "$A/sanalcp-seed-admin" /opt/sanalcp/bin/sanalcp-seed-admin
tar xzf "$A/frontend-dist.tar.gz" -C /opt/sanalcp/frontend-dist && ok "frontend-dist"
tar xzf "$A/migrations.tar.gz" -C /opt/sanalcp/src/migrations && ok "migrations ($(ls /opt/sanalcp/src/migrations/*.sql 2>/dev/null | wc -l) sql)"
[ -d "$A/mail" ] && cp -r "$A/mail/"* /opt/sanalcp/src/mail-templates/ && ok "mail config templates (postfix/dovecot/opendkim/roundcube)"
[ -f "$A/php-fpm/roundcube.conf" ] && install -m 0644 "$A/php-fpm/roundcube.conf" /etc/php-fpm.d/roundcube.conf
# ops tools + signon
for t in "$A"/ops/*; do
  bn=$(basename "$t"); nm="${bn%.sh}"
  install -m 0755 "$t" "/usr/local/bin/$nm" 2>/dev/null
done
cp "$A/ops/"* /opt/sanalcp/src/scripts/ 2>/dev/null
ok "ops tools (/usr/local/bin: update, optimize, redis-setup, ftp-setup, backup-all, repair, jail, wp-redis)"
# sshaccess.EnsureInfra reads this from /opt/sanalcp/src/scripts/50-sanal-jail.conf and
# applies it to /etc/ssh/sshd_config.d/ (at panel startup, idempotent) — here we just
# place the source file.
[ -f "$A/ssh/50-sanal-jail.conf" ] && install -m 0644 "$A/ssh/50-sanal-jail.conf" /opt/sanalcp/src/scripts/50-sanal-jail.conf \
  && ok "SSH jail sshd_config template (/opt/sanalcp/src/scripts)"

# ============ 7) PANEL SSL (self-signed) ============
step "7) Panel SSL (:8443 self-signed)"
if [ ! -f /etc/ssl/sanalcp/panel.crt ]; then
  openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
    -keyout /etc/ssl/sanalcp/panel.key -out /etc/ssl/sanalcp/panel.crt \
    -subj "/CN=sanalcp" >/dev/null 2>&1
fi
chmod 600 /etc/ssl/sanalcp/panel.key
ok "panel.crt / panel.key"

# ============ 8) NGINX ============
step "8) nginx (panel vhost + phpMyAdmin + perf)"
# http-level setting (client_max_body_size 10240m) — idempotent.
# NOTE: server_names_hash_bucket_size is NOT added here — sanalcp-optimize's
# 00-perf.conf already has it; adding it again here would make nginx -t fail with
# a "duplicate directive" error.
grep -q "client_max_body_size 10240m" /etc/nginx/nginx.conf || \
  sed -i '/^http {/a\    client_max_body_size 10240m;' /etc/nginx/nginx.conf
cp "$A/nginx/_panel.conf"     /etc/nginx/conf.d/_panel.conf
cp "$A/nginx/_default80.conf" /etc/nginx/conf.d/_default80.conf
cp "$A/nginx/_default443.conf" /etc/nginx/conf.d/_default443.conf
cp "$A/nginx/php-fpm.conf"    /etc/nginx/conf.d/php-fpm.conf 2>/dev/null
nginx -t >/dev/null 2>&1 && ok "nginx -t OK" || { nginx -t; die "nginx config error"; }

# ============ 9) phpMyAdmin ============
step "9) phpMyAdmin"
mkdir -p /opt/phpmyadmin   # create FIRST (otherwise strip-components extract fails)
if [ ! -f /opt/phpmyadmin/index.php ]; then
  TMP=$(mktemp -d)
  if curl -fsSL -o "$TMP/pma.tar.gz" https://www.phpmyadmin.net/downloads/phpMyAdmin-latest-all-languages.tar.gz \
     && tar xzf "$TMP/pma.tar.gz" -C /opt/phpmyadmin --strip-components=1; then
    ok "phpMyAdmin downloaded + extracted"
  else warn "phpMyAdmin download failed (network?) — fix later with: sanalcp-repair"; fi
  rm -rf "$TMP"
fi
if [ -f "$A/phpmyadmin/config.inc.php" ]; then
  BLOWFISH=$(openssl rand -hex 16)           # fresh — not a prod secret
  PMACTRL=$(openssl rand -hex 16)            # pma control user password (fresh)
  sed -e "s/BLOWFISH_SECRET_BURAYA/$BLOWFISH/g" -e "s/PMA_CONTROL_PASS_BURAYA/$PMACTRL/g" \
    "$A/phpmyadmin/config.inc.php" > /opt/phpmyadmin/config.inc.php
  # pma control user + phpmyadmin DB + pmadb tables (advanced features)
  mysql -u root <<SQL 2>/dev/null
CREATE DATABASE IF NOT EXISTS phpmyadmin;
CREATE USER IF NOT EXISTS 'pma'@'127.0.0.1' IDENTIFIED BY '$PMACTRL';
CREATE USER IF NOT EXISTS 'pma'@'localhost' IDENTIFIED BY '$PMACTRL';
ALTER USER 'pma'@'127.0.0.1' IDENTIFIED BY '$PMACTRL';
ALTER USER 'pma'@'localhost' IDENTIFIED BY '$PMACTRL';
GRANT ALL PRIVILEGES ON phpmyadmin.* TO 'pma'@'127.0.0.1', 'pma'@'localhost';
FLUSH PRIVILEGES;
SQL
  [ -f /opt/phpmyadmin/sql/create_tables.sql ] && mysql -u root phpmyadmin < /opt/phpmyadmin/sql/create_tables.sql 2>/dev/null
fi
[ -f "$A/phpmyadmin/pma-signon.php" ] && cp "$A/phpmyadmin/pma-signon.php" /opt/sanalcp/pma-signon/ 2>/dev/null
# pma internal-auth token (pma-signon.php + the panel API read the same file → the
# random value must match). Generate if missing (root:apache 0640 → the pma FPM
# pool [apache] can read it, no one else can). Never touch an existing one.
if [ ! -s /etc/sanalcp/pma-internal.token ]; then
  openssl rand -hex 32 > /etc/sanalcp/pma-internal.token
  chown root:apache /etc/sanalcp/pma-internal.token 2>/dev/null || true
  chmod 640 /etc/sanalcp/pma-internal.token
fi
cp "$A/php-fpm/phpmyadmin.conf" /etc/php-fpm.d/phpmyadmin.conf
mkdir -p /var/lib/phpmyadmin/{tmp,sessions}
chown -R nginx:nginx /opt/phpmyadmin /var/lib/phpmyadmin 2>/dev/null
restorecon -R /opt/phpmyadmin /var/lib/phpmyadmin >/dev/null 2>&1
setsebool -P httpd_can_network_connect_db 1 >/dev/null 2>&1
ok "phpMyAdmin pool + config + permissions"

# ============ 10) systemd + services ============
step "10) systemd + services"
cp "$A/systemd/sanalcp.service" /etc/systemd/system/sanalcp.service
# Daily panel DB backup (03:30) — copying the file is NOT enough, it's enabled
# --now below; otherwise the timer never fires and the install would silently end
# up with no backups.
for u in sanalcp-db-backup.service sanalcp-db-backup.timer; do
  [ -f "$A/systemd/$u" ] && cp "$A/systemd/$u" "/etc/systemd/system/$u"
done
systemctl daemon-reload
if [ -f /etc/systemd/system/sanalcp-db-backup.timer ]; then
  systemctl enable --now sanalcp-db-backup.timer >/dev/null 2>&1
  systemctl is-active --quiet sanalcp-db-backup.timer \
    && ok "daily panel DB backup ACTIVE (03:30 → /var/backups/sanalcp/db, 14 days)" \
    || warn "DB backup timer failed to start — daily panel DB backup may not run"
fi
systemctl enable --now php-fpm >/dev/null 2>&1
for v in $PHP_VERS; do systemctl enable --now php$v-php-fpm >/dev/null 2>&1; done
ok "php-fpm (base + 5 versions)"

# ---- named (DNS server) — nameserver for tenant domains ----
NC=/etc/named.conf
if [ -f "$NC" ]; then
  cp -a "$NC" "$NC.sanal-bak" 2>/dev/null || true
  # allow external queries: listen on all interfaces (default is 127.0.0.1 only)
  sed -i -E 's/listen-on port 53 \{[^}]*\}/listen-on port 53 { any; }/' "$NC"
  sed -i -E 's/listen-on-v6 port 53 \{[^}]*\}/listen-on-v6 port 53 { any; }/' "$NC"
  # don't be an open resolver (DNS amplification) — authoritative only
  sed -i -E 's/recursion yes/recursion no/' "$NC"
  # panel zone include (WriteZone fills this in) — idempotent
  grep -q 'sanalcp-zones.conf' "$NC" || \
    echo 'include "/etc/named/sanalcp-zones.conf";' >> "$NC"
fi
# panel zone include file (starts empty; fills up as domains are added)
mkdir -p /etc/named
[ -f /etc/named/sanalcp-zones.conf ] || \
  printf '// sanalcp — auto-generated\n' > /etc/named/sanalcp-zones.conf
chown root:named /etc/named/sanalcp-zones.conf 2>/dev/null || true
chmod 640 /etc/named/sanalcp-zones.conf 2>/dev/null || true
# zone files live under /var/named (SELinux named_zone_t context is REQUIRED)
restorecon -R /var/named /etc/named >/dev/null 2>&1 || true
if named-checkconf >/dev/null 2>&1; then
  systemctl enable --now named >/dev/null 2>&1 && ok "named (authoritative DNS, :53 open, recursion off)" || warn "named failed to start"
else
  warn "named-checkconf error — check DNS config manually"
fi

# ---- acme.sh (Let's Encrypt SSL) — the panel calls /root/.acme.sh/acme.sh ----
# LE requires a valid-looking email (@ + dot). If admin@local etc. isn't valid,
# register without a contact instead.
AEMAIL="$ADMIN_EPOSTA"; echo "$AEMAIL" | grep -qE '@[^@]+\.[^@]+$' || AEMAIL=""
if [ ! -x /root/.acme.sh/acme.sh ]; then
  ACME_INSTALLER=$(mktemp)
  if download https://get.acme.sh "$ACME_INSTALLER"; then
    if [ -n "$AEMAIL" ]; then sh "$ACME_INSTALLER" email="$AEMAIL" >/dev/null 2>&1 || true
    else sh "$ACME_INSTALLER" >/dev/null 2>&1 || true; fi
  fi
  rm -f "$ACME_INSTALLER"
fi
if [ -x /root/.acme.sh/acme.sh ]; then
  /root/.acme.sh/acme.sh --set-default-ca --server letsencrypt >/dev/null 2>&1
  # Register the LE account NOW (with a valid email if we have one, otherwise
  # without a contact) — so `--issue` doesn't fail later. The "@ + dot" regex
  # can't catch INVALID (non-public-suffix) TLDs like .local/.test/.internal — LE
  # rejects those with invalidContact, AND acme.sh stores that email in BOTH
  # account.conf's ACCOUNT_EMAIL and ca/*/directory/ca.conf's CA_EMAIL (even if
  # registration failed). Every later acme.sh call (even without -m) checks
  # CA_EMAIL first, then ACCOUNT_EMAIL — so a plain "--register-account" retry is
  # NOT enough, it reuses the same broken value and repeats the same error,
  # permanently preventing the panel from ever getting an SSL cert (even once a
  # real custom domain is set). If registering with the email fails, clear the
  # value from both files and retry without a contact.
  if [ -n "$AEMAIL" ] && ! /root/.acme.sh/acme.sh --register-account -m "$AEMAIL" --server letsencrypt >/dev/null 2>&1; then
    sed -i "s/^ACCOUNT_EMAIL=.*/ACCOUNT_EMAIL=''/" /root/.acme.sh/account.conf 2>/dev/null || true
    sed -i "s/^CA_EMAIL=.*/CA_EMAIL=''/" /root/.acme.sh/ca/*/directory/ca.conf 2>/dev/null || true
    /root/.acme.sh/acme.sh --register-account --server letsencrypt >/dev/null 2>&1
  elif [ -z "$AEMAIL" ]; then
    /root/.acme.sh/acme.sh --register-account --server letsencrypt >/dev/null 2>&1
  fi
  ok "acme.sh (Let's Encrypt CA + account registered + auto-renew cron)"
else
  warn "acme.sh install failed — for Let's Encrypt SSL, install manually: curl https://get.acme.sh | sh"
fi

# ---- httpd (Apache backend — for the web_backend=apache option, behind nginx) ----
# nginx listens on :80, so Apache listens on 127.0.0.1:10080 (mod_proxy_fcgi → php-fpm)
if [ -f /etc/httpd/conf/httpd.conf ]; then
  if grep -qE "^Listen 80$" /etc/httpd/conf/httpd.conf; then
    sed -i "s/^Listen 80$/Listen 127.0.0.1:10080/" /etc/httpd/conf/httpd.conf
  elif ! grep -qE "^Listen 127.0.0.1:10080" /etc/httpd/conf/httpd.conf; then
    echo "Listen 127.0.0.1:10080" >> /etc/httpd/conf/httpd.conf
  fi
  semanage port -l 2>/dev/null | grep -qE "http_port_t.*\b10080\b" || \
    semanage port -a -t http_port_t -p tcp 10080 2>/dev/null || \
    semanage port -m -t http_port_t -p tcp 10080 2>/dev/null
  if apachectl configtest >/dev/null 2>&1; then
    systemctl enable --now httpd >/dev/null 2>&1 && ok "httpd (Apache backend :10080, mod_proxy_fcgi)" || warn "httpd failed to start"
  else warn "httpd configtest failed — check the Apache backend manually"; fi
fi

# ---- composer (per-domain PHP dependency management) ----
# The installer is a PHP script run as root; it must NOT be run without signature
# verification. The expected SHA-384 comes from composer.github.io (GitHub Pages)
# — a different source than getcomposer.org, so compromising one server alone
# isn't enough to silently run a malicious installer.
if [ ! -x /usr/local/bin/composer ]; then
  COMPOSER_INSTALLER=$(mktemp --suffix=.php)
  COMPOSER_SIG=$(mktemp)
  if download https://getcomposer.org/installer "$COMPOSER_INSTALLER" &&
     download https://composer.github.io/installer.sig "$COMPOSER_SIG"; then
    beklenen=$(tr -d '[:space:]' < "$COMPOSER_SIG")
    gercek=$(php -r "echo hash_file('sha384', '$COMPOSER_INSTALLER');" 2>/dev/null)
    if [ -n "$beklenen" ] && [ "$beklenen" = "$gercek" ]; then
      php "$COMPOSER_INSTALLER" --install-dir=/usr/local/bin --filename=composer >/dev/null 2>&1 || true
    else
      warn "composer installer signature could not be verified — install skipped"
    fi
  fi
  rm -f "$COMPOSER_INSTALLER" "$COMPOSER_SIG"
fi
[ -x /usr/local/bin/composer ] && ok "composer ($(/usr/local/bin/composer --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1))" || warn "composer install failed"

# ---- daily backup cron (sanalcp-backup-all at 03:00 UTC) ----
cat > /etc/cron.d/sanalcp-backup <<'CRON'
# sanalcp — daily scheduled backup, 03:00 UTC
SHELL=/bin/bash
PATH=/usr/local/bin:/usr/bin:/bin
0 3 * * * root /usr/local/bin/sanalcp-backup-all
CRON
# Start crond NOW + enable it (AlmaLinux's preset only enables it, doesn't start it
# until reboot → the backup cron wouldn't run until the first reboot). enable --now
# is idempotent.
systemctl enable --now crond >/dev/null 2>&1
systemctl is-active --quiet crond && ok "daily backup cron + crond ACTIVE (03:00 UTC)" || warn "crond failed to start — the backup cron may not run"

# SELinux
setsebool -P httpd_can_network_connect 1 >/dev/null 2>&1 && ok "SELinux httpd_can_network_connect"
# Batch5A: nginx (httpd_t) needs to read tenant home content (public_html) — with
# these booleans OFF, try_files thinks the file doesn't exist → every site 404s.
# (Also guaranteed at panel startup by ensureHTTPDHomeBooleans; this line is for
# first boot.)
setsebool -P httpd_enable_homedirs=on httpd_read_user_content=on >/dev/null 2>&1 && ok "SELinux httpd home read (homedirs + user_content)"
restorecon -R /opt/sanalcp/bin /opt/sanalcp/frontend-dist >/dev/null 2>&1
# Batch5A: SELinux fcontext (httpd_var_run_t) for per-tenant php-fpm socket dirs
# under /run/php-fpm-<sk>/. The existing /run/php-fpm(/.*)? rule doesn't cover the
# hyphenated path → nginx→FPM 500s. Idempotent.
# (Also guaranteed at panel startup by ensureFPMSELinuxFcontext; this line covers
# the state before first boot.)
if command -v getenforce >/dev/null 2>&1 && [ "$(getenforce)" != "Disabled" ] && command -v semanage >/dev/null 2>&1; then
  semanage fcontext -l 2>/dev/null | grep -q "/run/php-fpm-\[" || \
    semanage fcontext -a -t httpd_var_run_t "/run/php-fpm-[^/]+(/.*)?" 2>/dev/null || true
  ok "SELinux fcontext: per-tenant php-fpm socket (httpd_var_run_t)"
fi

# ============ 11) Valkey + tuning ============
step "11) Valkey (Redis) + performance tuning"
command -v sanalcp-redis-setup >/dev/null 2>&1 && sanalcp-redis-setup >/dev/null 2>&1 && ok "sanalcp-redis-setup" || warn "redis-setup skipped"
command -v sanalcp-optimize >/dev/null 2>&1 && sanalcp-optimize >/dev/null 2>&1 && ok "sanalcp-optimize" || warn "optimize skipped"

# ============ 12) Start panel (migrations run at startup) ============
step "12) Starting the panel"
systemctl enable --now sanalcp >/dev/null 2>&1; sleep 3
systemctl enable --now nginx >/dev/null 2>&1; systemctl restart nginx >/dev/null 2>&1
if systemctl is-active --quiet sanalcp; then ok "sanalcp ACTIVE"; else journalctl -u sanalcp --no-pager -n 20; die "panel failed to start"; fi

# ---- FTP setup (Pure-FTPd) — runs NOW: the migration has already created the
# ftp_accounts table ---- (not in step 11, because GRANT SELECT ON
# panel.ftp_accounts failed while the table didn't exist yet)
sleep 2
command -v sanalcp-ftp-setup >/dev/null 2>&1 && sanalcp-ftp-setup >/dev/null 2>&1 && ok "sanalcp-ftp-setup (Pure-FTPd, MySQL backend)" || warn "ftp-setup skipped"

# ---- Mail server (Postfix/Dovecot/OpenDKIM) — placed here for the SAME reason as
# ftp-setup: GRANT SELECT ON panel.mail_domains/mailboxes/mail_aliases fails until
# the migration creates those tables (first panel startup).
command -v sanalcp-mail-setup >/dev/null 2>&1 && sanalcp-mail-setup >/dev/null 2>&1 && ok "sanalcp-mail-setup (Postfix/Dovecot/OpenDKIM)" || warn "mail-setup skipped"

# ============ 13) Admin access ============
# 🔴 Panel admin login = the server's ROOT user (PAM/shadow verification).
# There is NO separate panel password. Login: username 'root' + this server's
# root password.
step "13) Admin access (root + PAM)"
DSN="panel:${DBPASS}@tcp(127.0.0.1:3306)/panel?parseTime=true"
if [ -x /opt/sanalcp/bin/sanalcp-seed-admin ]; then
  # auxiliary users record (ownership/audit); login is still verified via root+PAM
  /opt/sanalcp/bin/sanalcp-seed-admin -dsn "$DSN" -kullanici root \
    -parola "$(openssl rand -hex 16)" -eposta "$ADMIN_EPOSTA" -dil "$PANEL_LANG" >/dev/null 2>&1 \
    && ok "admin record ready" || warn "seed skipped (not critical)"
fi
# root profile should start EMPTY — clear the seed-admin's placeholder
# 'admin@local'/'Sistem Yöneticisi' values (the user fills these in from Profile)
mysql panel -e "UPDATE users SET email='', full_name='' WHERE username='root' AND email='admin@local';" >/dev/null 2>&1 || true
# Panel's server-side default language (used by the login screen before anyone is
# authenticated) — the value picked in step 0.
mysql panel -e "UPDATE panel_ayarlari SET varsayilan_dil='$PANEL_LANG' WHERE id=1;" >/dev/null 2>&1 \
  && ok "panel default language set to '$PANEL_LANG'" || warn "could not set panel default language"
ok "Login: username 'root' + this server's root password"

# ============ 14) Permission repair ============
step "14) Permission/SELinux repair"
command -v sanalcp-repair >/dev/null 2>&1 && sanalcp-repair --quiet >/dev/null 2>&1 && ok "sanalcp-repair" || warn "repair skipped"

# ============ 15) VERIFICATION ============
step "15) Verification"
IP=$(hostname -I 2>/dev/null | awk '{print $1}')
CODE=$(curl -sk -o /dev/null -w '%{http_code}' https://127.0.0.1:8443/ 2>/dev/null)
API=$(curl -sk -o /dev/null -w '%{http_code}' https://127.0.0.1:8443/api/v1/domains 2>/dev/null)
echo -e "  services: $(systemctl is-active mariadb nginx valkey php-fpm named pure-ftpd sanalcp crond | tr '\n' ' ')"
echo -e "  panel :8443 → HTTP $CODE   ·   API (auth) → HTTP $API   ·   DNS :53 → $(systemctl is-active named)   ·   FTP :21 → $(systemctl is-active pure-ftpd)"
echo -e "  tools: SSL/acme.sh $([ -x /root/.acme.sh/acme.sh ] && echo ✓ || echo ✗)   ·   firewall/nft $(command -v nft >/dev/null && echo ✓ || echo ✗)   ·   unzip/zip $(command -v unzip >/dev/null && command -v zip >/dev/null && echo ✓ || echo ✗)   ·   composer $(command -v composer >/dev/null && echo ✓ || echo ✗)   ·   apache/httpd $(systemctl is-active httpd)"
echo -e "  isolation: plan-driven resource limits (cgroup slice) + per-tenant PHP-FPM (CageFS equivalent) READY   ·   bubblewrap $(command -v bwrap >/dev/null && echo ✓ || echo ✗)"
echo
echo -e "${c_g}═══════════════════════════════════════════════${c_0}"
echo -e "${c_g} ✓ SanalCP installation complete${c_0}"
echo -e "   Panel:    ${c_b}https://${IP:-SERVER_IP}:8443${c_0}"
echo -e "   Username: ${c_b}root${c_0}   Password: ${c_b}this server's root password${c_0}"
echo -e "   (panel admin login verifies against the server's root account via PAM)"
if [ "$(findmnt -no FSTYPE / 2>/dev/null)" = "xfs" ] && ! findmnt -no OPTIONS / 2>/dev/null | grep -qwE 'usrquota|uquota|quota'; then
  echo -e "   ${c_y}Disk quota: rootflags=uquota was written to GRUB — takes effect after a ONE-TIME reboot.${c_0}"
fi
echo -e "${c_g}═══════════════════════════════════════════════${c_0}"
