#!/usr/bin/env bash
# sanalcp-install — turns a blank server into a full SanalCP install.
#
# Supported: AlmaLinux 9.4+/10 (Rocky/RHEL equivalent)  ·  Debian 13/12, Ubuntu 26.04/24.04
#
# The body is SHARED between both OS families. Everything that differs is resolved
# by the thin wrappers defined in step 0 (pkg_kur / paket_ad / servis_ad / repo_php_ekle).
# 🔴 Do NOT fork this file per distribution: two installers always drift apart, and
# the drift is only discovered on a customer's server.
#
# Designed to be idempotent (safe to re-run). Run as root.
#
#   ./sanalcp-install.sh [--admin-kullanici <k>] [--admin-parola <p>] [--admin-eposta <e>] [--lang tr|en] [--no-reboot]
#
# assets/ must sit next to this script:
#   sanalcp-server  sanalcp-seed-admin  frontend-dist.tar.gz
#   migrations.tar.gz  nginx/*  php-fpm/*  phpmyadmin/*  systemd/*  ops/*  ssh/*
set -uo pipefail

# BASH_SOURCE (not $0): when the parity test SOURCES this script, $0 is "bash"
# and assets/ would be looked up in the caller's directory.
HERE="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
A="$HERE/assets"
ADMIN_PAROLA=""; ADMIN_EPOSTA="admin@local"; PANEL_LANG=""; ADMIN_KULLANICI="admin"
# Kurulum sonu yeniden başlatma: varsayılan AÇIK (bkz. adım 16). Otomasyon/CI
# için --no-reboot veya SANALCP_REBOOT=0.
REBOOT_ET="${SANALCP_REBOOT:-1}"
while [ $# -gt 0 ]; do case "$1" in
  --admin-kullanici) shift; ADMIN_KULLANICI="$1" ;;
  --admin-parola) shift; ADMIN_PAROLA="$1" ;;
  --admin-eposta) shift; ADMIN_EPOSTA="$1" ;;
  --lang) shift; PANEL_LANG="$1" ;;
  --no-reboot) REBOOT_ET="0" ;;
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

# 'root' is NOT a valid panel admin name. auth.KullaniciRootMu() matches it
# case-insensitively and routes that login to the legacy /etc/shadow path --
# which this installer disables in step 13. sanalcp-seed-admin would happily
# rewrite the id=1 row's bcrypt hash, but nothing would ever read it: the
# printed password could not log in and no other admin would exist. Rejected
# here, up front, instead of 15 minutes later in step 13.
# Trim surrounding whitespace first: auth.KullaniciRootMu() applies
# strings.TrimSpace before comparing, so " root" reaches the shadow path just
# like "root" does. Without the trim, the case below would wave it through.
ADMIN_KULLANICI="${ADMIN_KULLANICI#"${ADMIN_KULLANICI%%[![:space:]]*}"}"
ADMIN_KULLANICI="${ADMIN_KULLANICI%"${ADMIN_KULLANICI##*[![:space:]]}"}"
case "${ADMIN_KULLANICI,,}" in
  root) die "--admin-kullanici root is not allowed: 'root' is the legacy panel shadow-login account this installer disables, so the generated password would never work -- pick another name" ;;
esac

if [ -z "${SANALCP_TANIM_TESTI:-}" ]; then
[ "$(id -u)" = 0 ] || die "root required"
[ -d "$A" ] || die "assets/ not found ($A)"
if [ -f "$A/SHA256SUMS" ]; then
  (cd "$HERE" && sha256sum -c assets/SHA256SUMS >/dev/null) ||
    die "release asset integrity check failed"
  ok "release SHA-256 integrity"
else
  die "assets/SHA256SUMS not found — missing or hand-assembled release"
fi
fi
# ============ 0) OS FAMILY + WRAPPERS ============
# 🔴 The family detection and the whole package/service name table live in ONE
# place: assets/ops/sanalcp-ortak.sh. The ops tools (redis-setup, mail-setup,
# repair…) source the very same file from /usr/local/bin/sanalcp-ortak once
# installed, so the installer and the tools can never disagree about what a
# package is called. That single shell table is checked against the Go side
# (internal/osfam) by internal/osfam/installer_paritesi_test.go.
ORTAK="$A/ops/sanalcp-ortak.sh"
[ -f "$ORTAK" ] || die "assets/ops/sanalcp-ortak.sh not found — incomplete release"
# shellcheck source=assets/ops/sanalcp-ortak.sh
. "$ORTAK"
if rhel_mi; then
  ok "detected OS: ${PRETTY_NAME:-unknown} (RHEL family, EL${EL_MAJOR})"
else
  # apt must never open a dialog (dpkg conffile prompts, needrestart's service
  # picker) — an unattended `curl | bash` install would hang forever there.
  export DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a NEEDRESTART_SUSPEND=1
  ok "detected OS: ${PRETTY_NAME:-unknown} (Debian family, ${OS_KODADI:-?})"
  # Debian 12 is a SECONDARY target: supported, but its security updates come from
  # the LTS team (narrower scope than normal stable) — say so once, up front.
  case "$OS_ID/$OS_KODADI" in
    debian/bookworm) warn "Debian 12 is a secondary target (LTS security support until 30 Jun 2028) — Debian 13 is recommended for new installs" ;;
  esac
fi

# havuz_kur <source pool file> <target name>: installs a panel-internal FPM pool
# (phpMyAdmin, Roundcube) into the system PHP's pool directory.
#
# The shipped pool files are written for RHEL. On Debian three things differ and
# all three are silent failures if missed:
#   user/group      apache -> www-data   (the user doesn't exist -> FPM won't start)
#   listen.owner    nginx  -> www-data   (wrong owner -> nginx can't open the socket -> 502)
#   mysql socket    /var/lib/mysql/mysql.sock -> /run/mysqld/mysqld.sock
#
# 🔴 The socket PATH itself (/run/php-fpm/<name>.sock) is deliberately NOT changed:
# it is baked into the canonical panel vhost (internal/nginxconf/_panel.conf), which
# the panel re-writes from the embedded copy at every startup. Rewriting the path
# here would either be reverted on the next start, or make the file count as
# "admin-customised" and permanently cut it off from security updates. The
# directory is created by tmpfiles.d instead (see below).
havuz_kur(){
  local src="$1" ad="$2"
  mkdir -p "$SYS_PHP_POOL_DIR"
  if debian_mi; then
    sed -e "s/^user = apache/user = $WEB_USER/" \
        -e "s/^group = apache/group = $WEB_USER/" \
        -e "s/^listen.owner = nginx/listen.owner = $WEB_USER/" \
        -e "s/^listen.group = nginx/listen.group = $WEB_USER/" \
        -e "s#/var/lib/mysql/mysql.sock#$MYSQL_SOCK#g" \
        "$src" > "$SYS_PHP_POOL_DIR/$ad"
    chmod 0644 "$SYS_PHP_POOL_DIR/$ad"
  else
    install -m 0644 "$src" "$SYS_PHP_POOL_DIR/$ad"
  fi
}
if debian_mi; then
  # /run/php-fpm doesn't exist on Debian (sury's own pools use /run/php), but the
  # panel vhost expects the phpMyAdmin/Roundcube sockets there. tmpfiles recreates
  # it on every boot, before any service starts.
  printf 'd /run/php-fpm 0755 root root -\n' > /etc/tmpfiles.d/sanalcp-php-fpm.conf
  systemd-tmpfiles --create /etc/tmpfiles.d/sanalcp-php-fpm.conf >/dev/null 2>&1 || mkdir -p /run/php-fpm
fi

# ---- PHP version + extension naming ----
# Remi: version token is "83", package is "php83-php-fpm".
# sury:  version token is "8.3", package is "php8.3-fpm".
# The extension bundles are NOT a 1:1 translation either: on sury `pdo` and `json`
# are built in (no separate package) and mysqlnd is called `mysql`, while `curl`
# IS a separate package there (on RHEL it comes with the base php rpm).
if debian_mi; then
  PHP_VERS="7.4 8.0 8.1 8.2 8.3 8.4 8.5"
  PHP_EXT="fpm cli mysql mbstring bcmath intl gd soap opcache xml zip pgsql ldap curl"
  PHP_REDIS_EXT="redis"
  BASE_PKGS="php8.3 php8.3-fpm php8.3-cli php8.3-mysql php8.3-mbstring php8.3-xml php8.3-zip php8.3-curl php8.3-redis"
else
  PHP_VERS="74 80 81 82 83 84 85"
  PHP_EXT="fpm cli mysqlnd mbstring bcmath intl gd soap opcache pdo xml zip pgsql ldap"
  PHP_REDIS_EXT="pecl-redis6"
  BASE_PKGS="php php-fpm php-cli php-mysqlnd php-mbstring php-json php-pecl-zip php-pecl-redis6"
fi
php_pkg(){ # <version token> <extension> -> real package name
  if debian_mi; then echo "php$1-$2"; else echo "php$1-php-$2"; fi
}

# TEST HOOK: `SANALCP_TANIM_TESTI=1 . ./sanalcp-install.sh` defines the name
# resolution above and returns WITHOUT touching the system, so the mapping can be
# tested against internal/osfam (scripts/installer-adlar-testi.sh). A wrong package
# or unit name is only discovered on a real server otherwise.
if [ -n "${SANALCP_TANIM_TESTI:-}" ]; then return 0 2>/dev/null || exit 0; fi

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

# ============ 1) REPOSITORIES ============
# Multi-version PHP comes from a third-party repo on BOTH families: Remi on RHEL,
# deb.sury.org on Debian/Ubuntu. Neither distro ships more than one PHP version.
if rhel_mi; then
  step "1) Repositories (EPEL + Remi + CRB)"
  dnf install -y epel-release >/dev/null 2>&1 && ok "EPEL"
  rpm -q remi-release >/dev/null 2>&1 || dnf install -y "https://rpms.remirepo.net/enterprise/remi-release-${EL_MAJOR}.rpm" >/dev/null 2>&1
  rpm -q remi-release >/dev/null 2>&1 && ok "Remi (EL${EL_MAJOR})" || die "could not add Remi"
  # `config-manager` is a plugin (dnf-plugins-core on EL9 / dnf5-plugins on EL10)
  # that isn't guaranteed to be preinstalled on a minimal cloud image — the
  # virtual provide resolves to the right package on either dnf4 or dnf5.
  dnf install -y 'dnf-command(config-manager)' >/dev/null 2>&1
  dnf config-manager --set-enabled crb >/dev/null 2>&1 && ok "CRB"
else
  step "1) Repositories (deb.sury.org PHP)"
  depo_yenile
  pkg_kur ca-certificates curl gnupg apt-transport-https lsb-release >/dev/null 2>&1
  # 🔴 The key goes to /usr/share/keyrings + signed-by, NOT apt-key: apt-key is
  # deprecated and, worse, a key added there is trusted for EVERY repo on the
  # system — a compromised sury key could then sign a replacement for any distro
  # package. signed-by scopes the trust to this one repo.
  SURY_KEY=/usr/share/keyrings/deb.sury.org-php.gpg
  if [ ! -s "$SURY_KEY" ]; then
    TMPKEY=$(mktemp)
    if download https://packages.sury.org/php/apt.gpg "$TMPKEY" && [ -s "$TMPKEY" ]; then
      install -m 0644 "$TMPKEY" "$SURY_KEY"
    fi
    rm -f "$TMPKEY"
  fi
  [ -s "$SURY_KEY" ] || die "could not fetch the deb.sury.org signing key — multi-version PHP would be unavailable"
  # sury publishes: bullseye bookworm trixie | jammy noble resolute. If this
  # release isn't among them the repo would 404 on every apt update, so fail loudly.
  case "$OS_KODADI" in
    bullseye|bookworm|trixie|jammy|noble|resolute) : ;;
    *) die "deb.sury.org has no PHP repo for '${OS_KODADI:-unknown}' — supported: Debian 12/13, Ubuntu 22.04/24.04/26.04" ;;
  esac
  echo "deb [signed-by=$SURY_KEY] https://packages.sury.org/php/ $OS_KODADI main" > /etc/apt/sources.list.d/sanalcp-sury-php.list

  # 🔴 NEVER pipe apt-cache into `grep -q` here. This script runs under
  # `set -o pipefail`, and `grep -q` exits the instant it matches — which closes
  # the pipe, hands apt-cache a SIGPIPE, and makes the PIPELINE return 141 even
  # though the match SUCCEEDED. Capture into a variable and match on that.
  #
  # Found live on Ubuntu 24.04. It passed on Debian 12/13 and Ubuntu 26.04 purely
  # because there `php8.3-fpm` comes from sury alone, so apt-cache's output fits
  # in the pipe buffer and it finishes writing before grep exits. On noble the
  # package also exists in noble, noble-updates and noble-security, the output is
  # longer, and the race flips — the install died at step 1 with a message
  # blaming /etc/apt/sources.list.d, which was perfectly correct all along.
  #
  # Same trap as `quotaon | grep` (internal/kaynaklimit/kota_ext4.go) — third
  # time in this codebase, hence the sweep in scripts/pipefail-grep-taramasi.sh.
  #
  # The retry is kept for a different, real reason: sury is a SINGLE third-party
  # host and the whole install depends on it.
  sury_ok=0
  for deneme in 1 2 3; do
    depo_yenile || true
    pol=$(apt-cache policy php8.3-fpm 2>/dev/null || true)
    case "$pol" in *sury.org*) sury_ok=1 ;; esac
    [ "$sury_ok" = 1 ] && break
    [ "$deneme" -lt 3 ] && { warn "deb.sury.org index not ready (attempt $deneme/3) — retrying in $((deneme * 5))s"; sleep $((deneme * 5)); }
  done
  if [ "$sury_ok" = 1 ]; then
    ok "deb.sury.org PHP ($OS_KODADI)"
  else
    # Report what apt ACTUALLY said, then distinguish the two real causes.
    [ -n "${DEPO_SON_CIKTI:-}" ] && printf '%s\n' "$DEPO_SON_CIKTI" >&2
    case "$pol" in
      *"Version table"*)
        die "php8.3-fpm resolves, but NOT to deb.sury.org — another repo is winning; check apt pinning and /etc/apt/sources.list.d/sanalcp-sury-php.list" ;;
    esac
    die "could not read the deb.sury.org index for '$OS_KODADI' after 3 attempts — see the apt output above (network/proxy/DNS or sury outage)"
  fi
fi

# ============ 2) BASE PACKAGES ============
step "2) Base packages"
# 🔴 valkey is intentionally NOT in this batch: it doesn't exist on EL9 (package
# is still "redis" there) and not on Debian 12 either (see internal/osfam —
# redis-server is used there). Step 11's sanalcp-redis-setup installs whichever
# one is available. A single missing package name in a batch install can abort
# the WHOLE transaction, so this would have taken down nginx/mariadb too.
BASE_SYS="$(paket_ad web) $(paket_ad db) certbot python3-certbot-nginx \
  $(paket_ad antivirus) $(paket_ad av-guncel) $(paket_ad apache-ara) tar openssl \
  jq $(paket_ad dns) $(paket_ad dns-arac) nftables unzip zip $(paket_ad cron) \
  $(paket_ad kota-xfs) $(paket_ad kota-ext) sudo bubblewrap rsync git curl acl lftp sshpass"
if debian_mi; then
  # Servis/timer kapalı kalır; panel bunu güvenlik taraması ve kendi kontrollü
  # zamanlayıcısı için çağırır.
  BASE_SYS="$BASE_SYS unattended-upgrades"
fi
if rhel_mi; then
  # httpd = the optional Apache backend (RHEL only, see osfam.ApacheBackendDestekli).
  # policycoreutils-python-utils/setools-console are SELinux tooling — Debian has none.
  BASE_SYS="$BASE_SYS $(paket_ad apache) mod_proxy_html policycoreutils-python-utils setools-console"
fi
pkg_kur $BASE_SYS \
  && ok "nginx, mariadb, certbot, clamav, bind, nftables, unzip/zip, bubblewrap, acl, lftp, sshpass, tools" \
  || die "base package install"

# RAR extractor (file manager .rar extract) — PRIMARY: bsdtar (libarchive, in the
# base repo, reliably reads RAR/RAR5 and itself rejects path-traversal). 🔴 NOTE:
# AlmaLinux 10's default `7z` (7-Zip 26.02) does NOT include the RAR codec →
# unusable here. Fall back to unar/unrar if bsdtar is unavailable.
if command -v bsdtar >/dev/null 2>&1 || command -v unar >/dev/null 2>&1 || command -v unrar >/dev/null 2>&1; then
  ok "RAR extractor available ($(command -v bsdtar unar unrar 2>/dev/null | head -1))"
elif pkg_kur "$(paket_ad bsdtar)"; then
  ok "bsdtar (libarchive — rar/rar5/zip/7z extract)"
elif pkg_kur unar || pkg_kur unrar; then
  ok "unar/unrar (rar extract)"
else
  warn "could not install a RAR extractor — file manager .rar extract disabled (zip/tar still work)"
fi

# ============ 2b) DISK QUOTA (per-tenant disk + inode — CloudLinux parity) ============
# Files are owned c_<sk>:c_<sk>, so USER quota maps 1:1 onto a tenant and can't be
# escaped. Which backend is used depends on the ROOT FILESYSTEM, not on the distro
# (AlmaLinux installs on ext4 too, Debian on XFS) — the panel picks it by statfs
# magic in internal/kaynaklimit/kota.go. Keep the kernel flags below identical to
# the ones that file reports, otherwise the panel asks for a reboot that fixes nothing.
#
#   XFS  -> rootflags=uquota     · grub2-mkconfig · grubby --update-kernel=ALL (BLS)
#   extN -> rootflags=usrquota   · grub-mkconfig
#
# Quota can only be turned on at MOUNT time (no live remount for the root fs) →
# the flag is written to GRUB and takes effect after a ONE-TIME reboot.
step "2b) Disk quota"
pkg_kur "$(paket_ad kota-xfs)" "$(paket_ad kota-ext)" && ok "quota tools (xfsprogs + quota)" || warn "quota packages skipped"
ROOTFS_TYPE=$(findmnt -no FSTYPE / 2>/dev/null || echo "")
ROOTFS_OPTS=$(findmnt -no OPTIONS / 2>/dev/null || echo "")
case "$ROOTFS_TYPE" in
  xfs)              KOTA_FLAG="rootflags=uquota" ;;
  ext2|ext3|ext4)   KOTA_FLAG="rootflags=usrquota" ;;
  *)                KOTA_FLAG="" ;;
esac
if [ -z "$KOTA_FLAG" ]; then
  warn "root fs is '$ROOTFS_TYPE' — disk quota is not supported there (only XFS and ext2/3/4); the panel will report quota as unavailable"
else
  KOTA_MOUNTTA=0
  echo "$ROOTFS_OPTS" | grep -qwE 'usrquota|uquota|quota' && KOTA_MOUNTTA=1  # sigpipe-ok: echo builtin
  if [ "$KOTA_MOUNTTA" = 1 ]; then
    ok "root fs user quota already active ($ROOTFS_TYPE)"
  elif grep -q "$KOTA_FLAG" /etc/default/grub 2>/dev/null; then
    ok "GRUB $KOTA_FLAG already present"
    warn "Disk quota needs a ONE-TIME reboot to take effect (root fs quota can't be enabled via remount)."
  else
    if grep -q '^GRUB_CMDLINE_LINUX=' /etc/default/grub 2>/dev/null; then
      sed -i "s/^\(GRUB_CMDLINE_LINUX=\"[^\"]*\)\"/\1 $KOTA_FLAG\"/" /etc/default/grub
    else
      echo "GRUB_CMDLINE_LINUX=\"$KOTA_FLAG\"" >> /etc/default/grub
    fi
    if rhel_mi; then
      # Also update existing boot entries (BLS) + regenerate grub.cfg (BIOS + EFI).
      command -v grubby >/dev/null 2>&1 && grubby --update-kernel=ALL --args="$KOTA_FLAG" >/dev/null 2>&1 || true
      grub2-mkconfig -o /boot/grub2/grub.cfg >/dev/null 2>&1 || true
      for cfg in /boot/efi/EFI/*/grub.cfg; do [ -f "$cfg" ] && grub2-mkconfig -o "$cfg" >/dev/null 2>&1 || true; done
    else
      # Debian has no BLS entries; update-grub is a thin wrapper over grub-mkconfig.
      if command -v update-grub >/dev/null 2>&1; then update-grub >/dev/null 2>&1 || true
      else grub-mkconfig -o /boot/grub/grub.cfg >/dev/null 2>&1 || true; fi
    fi
    ok "GRUB $KOTA_FLAG added (root is $ROOTFS_TYPE)"
    warn "Disk quota needs a ONE-TIME reboot to take effect (root fs quota can't be enabled via remount)."
  fi

  # 🔴 The unit below is written on EVERY run, including when quota is already
  # active on the mount. It used to sit inside the "quota not set up yet" branch,
  # so a server that had already rebooted never received a corrected unit — the
  # bug in item 5 of §5f would have survived every sanalcp-update forever.
  # ext2/3/4 only. Two DIFFERENT jobs, and they have different lifetimes:
  #
  #   quotacheck  → ONCE, to build /aquota.user (minutes on a large disk).
  #   quotaon     → EVERY boot, or enforcement is silently lost.
  #
  # 🔴 The unit used to carry `ConditionPathExists=!/aquota.user` for the whole
  # service, so from the SECOND boot on systemd skipped it — including the
  # quotaon call — and the server came up with accounting but NO enforcement.
  # Found live on Debian 13 (Faz 5b) only because the box was rebooted twice;
  # a single reboot looks perfectly healthy. Affects Debian 12 the same way.
  #
  # Why the distro's own unit does not cover us: systemd-fstab-generator wires
  # quotaon up from the QUOTA OPTIONS IN /etc/fstab, and the root filesystem
  # gets its usrquota from the kernel cmdline (rootflags=) instead — so nothing
  # pulls quotaon-root.service in. `quotaon.service` does not even exist on
  # Debian (the package ships quota.service / quotaon@.service).
  if [ "$ROOTFS_TYPE" != "xfs" ] && command -v quotacheck >/dev/null 2>&1; then
    # 🔴 KERNEL KOTA FORMATI: ext2/3/4 kotası `quota_v2` (vfsv2) modülünü ister.
    # Ubuntu bunu çekirdek yapılandırmasında MODÜL olarak veriyor
    # (CONFIG_QFMT_V2=m) ve modülü linux-modules-extra-* paketine koyuyor —
    # bulut/minimal imajlar o paketi KURMUYOR. Sonuç, Ubuntu 24.04'te canlı
    # görüldü: mount usrquota taşıyor, /aquota.user üretiliyor, birim başarıyla
    # koşuyor, ama `quotaon` şunu diyor ve kota hiç açılmıyor:
    #
    #   quotaon: Quota format not supported in kernel.
    #
    # Debian 12/13 ve Ubuntu 26.04'te modül mevcut olduğu için orada görünmedi.
    if ! modprobe quota_v2 >/dev/null 2>&1; then
      pkg_kur "linux-modules-extra-$(uname -r)" >/dev/null 2>&1 || true
      modprobe quota_v2 >/dev/null 2>&1 || true
    fi
    if modprobe quota_v2 >/dev/null 2>&1 || lsmod 2>/dev/null | grep -q quota_v2; then  # sigpipe-ok: lsmod kısa çıktı
      # Her açılışta yüklensin (birimin ExecStartPre'si de ayrıca deniyor).
      printf 'quota_v2\n' > /etc/modules-load.d/sanalcp-quota.conf
      ok "kernel quota format (quota_v2) available"
    else
      warn "kernel quota format (quota_v2) NOT available — disk quota ENFORCEMENT will not work on this kernel; accounting-only. Install linux-modules-extra-$(uname -r) (Ubuntu) or use a kernel with CONFIG_QFMT_V2."
    fi
    cat > /etc/systemd/system/sanalcp-quotacheck.service <<'QC'
[Unit]
Description=SanalCP — ext quota accounting files (once) + enforcement (every boot)
DefaultDependencies=no
After=local-fs.target

[Service]
Type=oneshot
RemainAfterExit=yes
# The vfsv2 quota format is a MODULE on Ubuntu; without it quotaon fails with
# "Quota format not supported in kernel". "-" = do not fail the unit if the
# module is built in (modprobe then has nothing to do).
ExecStartPre=-/sbin/modprobe quota_v2
# quotacheck only when the accounting file is missing — it is expensive.
ExecStart=/bin/sh -c '[ -f /aquota.user ] || /usr/sbin/quotacheck -cum /'
# quotaon on EVERY boot. Already-on is not an error worth failing the unit for
# (and `quotaon` exit codes are unreliable in both directions — see
# internal/kaynaklimit/kota_ext4.go).
ExecStart=/bin/sh -c '/usr/sbin/quotaon -u / 2>/dev/null || true'

[Install]
WantedBy=local-fs.target
QC
    systemctl daemon-reload >/dev/null 2>&1
    systemctl enable sanalcp-quotacheck.service >/dev/null 2>&1 \
      && ok "quota unit armed (quotacheck once + quotaon on every boot)" \
      || warn "could not arm quota unit — after the reboot run: quotacheck -cum / && quotaon -u /"
    # Mount already carries quota: enforcement can be restored RIGHT NOW, no
    # reboot needed. This is the path that repairs an existing server whose
    # quotaon was lost on its second boot. Not run before the reboot, so
    # quotacheck never scans a filesystem that has no quota active yet.
    if [ "$KOTA_MOUNTTA" = 1 ]; then
      # 🔴 restart, START DEĞİL. Birim `RemainAfterExit=yes` taşıyor; açılışta bir
      # kez koştuğu için zaten "active" görünür ve `systemctl start` HİÇBİR ŞEY
      # YAPMAZ. Onarım yolu bu yüzden sessizce etkisiz kalıyordu (Ubuntu 24.04'te
      # canlı görüldü: modül kuruldu, birim "active", kota yine kapalı).
      # `enable --now`ın apt'te no-op olmasıyla aynı sınıf hata.
      systemctl restart sanalcp-quotacheck.service >/dev/null 2>&1 || true
      case "$(quotaon -p -u / 2>/dev/null || true)" in
        *"is on"*) ok "quota enforcement active (quotaon)" ;;
        *)         warn "quotaon still reports off — check: quotacheck -cum / && quotaon -u /" ;;
      esac
    fi
  fi
fi

# ============ 2c) FIREWALLD ============
# 🔴 SanalCP manages its own nftables-based firewall (internal/guvenlikduvari —
# critical-port protection + admin-panel whitelist/ban rules). Some minimal
# AlmaLinux 9 cloud images ship with firewalld ACTIVE by default, and its
# restrictive public zone (only ssh/dhcpv6-client/cockpit) runs IN PARALLEL
# with SanalCP's own nftables table — even though the panel's own rules default
# to "policy accept" (allow everything), firewalld independently blocks every
# other port (8443/80/443/53/21/25/587/993...). Found live on an AlmaLinux 9.8
# test VPS: the panel worked over localhost right after install, but was
# completely unreachable from outside after a reboot — firewalld was silently
# doing this the whole time. SanalCP should be the only firewall in play.
# On Debian/Ubuntu the same trap has a different name: ufw. Ubuntu cloud images
# ship it installed and some providers enable it — its default deny-incoming
# policy blocks 8443/80/443/53/21/25/587/993 exactly like firewalld does.
step "2c) Distro firewall (firewalld / ufw)"
KAPATILDI=0
for fw in firewalld ufw; do
  if systemctl is-active --quiet "$fw" 2>/dev/null; then
    systemctl disable --now "$fw" >/dev/null 2>&1 \
      && { ok "$fw disabled (SanalCP manages its own nftables firewall)"; KAPATILDI=1; } \
      || warn "could not disable $fw — it may block panel/site access"
  fi
done
[ "$KAPATILDI" = 0 ] && ok "no distro firewall active"

# ============ 3) PHP (multi-version + base + wp-cli) ============
step "3) PHP versions ($( debian_mi && echo sury || echo Remi ) + base) + wp-cli"
if rhel_mi; then
  # 🔴 BEFORE the PHP batch install: disable dnf's auto-lock sources (if
  #    dnf-automatic/makecache timers are on, a bulk "dnf install" can hit lock
  #    contention / false negatives). Managed panel updates handle this themselves;
  #    auto-update stays OFF (avoids lock contention + surprise patches).
  systemctl disable --now dnf-automatic.timer dnf-makecache.timer >/dev/null 2>&1 || true
  # 🔴 On AlmaLinux 10 the AppStream `php` package IS 8.3 natively — no module
  # selection needed (RHEL 10 dropped PHP modularity). AlmaLinux 9 (and older)
  # still distributes PHP via module streams whose DEFAULT stream is an older
  # version, not 8.3. The panel's phpMap (internal/provisioner/provisioner.go)
  # hardcodes "system php = 8.3" for every OS — so on EL9 we explicitly pin
  # Remi's own php:remi-8.3 module stream to make the base `php` package
  # resolve to 8.3 there too, keeping that invariant true across both OSes
  # without any Go-side change.
  if [ "$EL_MAJOR" -lt 10 ]; then
    dnf module reset -y php >/dev/null 2>&1
    dnf module enable -y php:remi-8.3 >/dev/null 2>&1 \
      && ok "PHP module stream pinned to remi-8.3 (EL${EL_MAJOR})" \
      || warn "could not pin PHP module stream to 8.3 — base php version may not match what the panel expects"
  fi
else
  # Debian's own unattended-upgrades would fight the installer for the dpkg lock
  # mid-run; the panel manages its own updates. Same reasoning as the dnf timers.
  systemctl disable --now unattended-upgrades.service apt-daily.timer apt-daily-upgrade.timer >/dev/null 2>&1 || true
fi
pkg_kur $BASE_PKGS && ok "base php + php-redis"
# Turn a silent mismatch into a visible one: the panel assumes system php=8.3
# unconditionally (webmail, phpMyAdmin, and any domain set to PHP "8.3" use
# the base php-fpm pool, not a per-version one).
BASE_PHP_VER=$(php -r 'echo PHP_MAJOR_VERSION.".".PHP_MINOR_VERSION;' 2>/dev/null || echo "?")
if [ "$BASE_PHP_VER" = "8.3" ]; then
  ok "system PHP is 8.3 (what webmail/phpMyAdmin/'8.3' domains expect)"
elif rhel_mi; then
  warn "system PHP is $BASE_PHP_VER, expected 8.3 — webmail, phpMyAdmin and any domain set to PHP '8.3' may not work. Fix manually: dnf module reset php -y && dnf module enable php:remi-8.3 -y && dnf install -y $BASE_PKGS"
else
  # Debian'da `php` bir alternatives sembolik linkidir ve sürüm döngüsünden
  # sonra 8.3'e sabitlenir (aşağıya bakın) — bu aşamadaki değer geçicidir.
  warn "the default 'php' CLI is $BASE_PHP_VER at this point — it is pinned to 8.3 after the version loop"
fi
for v in $PHP_VERS; do
  pkgs=""; for e in $PHP_EXT $PHP_REDIS_EXT; do pkgs="$pkgs $(php_pkg "$v" "$e")"; done
  if pkg_kur $pkgs; then
    ok "php$v (+redis)"
  else
    # 🔴 Tek eksik paket TÜM sürümü düşürmesin. Batch install ya hep ya hiçtir:
    # sury/bookworm'da php8.5-opcache yok ve bu, 8.5'in diğer 14 eklentisinin de
    # kurulmamasına yol açıyordu (canlı Debian 12 testinde bulundu). Batch
    # başarısızsa tek tek denenir, eksikler ADIYLA raporlanır.
    eksik=""
    for p1 in $pkgs; do pkg_kur "$p1" || eksik="$eksik $p1"; done
    if [ -n "$eksik" ]; then warn "php$v kuruldu, bulunamayan paket(ler):$eksik"
    else ok "php$v (+redis)"; fi
  fi
done
# 🔴 Sistem PHP'si 8.3 OLMALI (panel phpMap'i her OS için böyle varsayar).
# Debian'da update-alternatives "en yeni"yi seçer: php8.4/8.5 kurulunca `php`
# CLI oraya kayar ve wp-cli/composer 8.3 yerine onu kullanır. RHEL'de böyle bir
# sorun yok (temel `php` paketi zaten 8.3). Sürüm döngüsünden SONRA sabitlenir.
if debian_mi && [ -x /usr/bin/php8.3 ]; then
  update-alternatives --set php /usr/bin/php8.3 >/dev/null 2>&1
  CLIV=$(php -r 'echo PHP_MAJOR_VERSION.".".PHP_MINOR_VERSION;' 2>/dev/null || echo "?")
  [ "$CLIV" = "8.3" ] && ok "php CLI pinned to 8.3 (wp-cli/composer parity with RHEL)" \
                      || warn "php CLI is $CLIV, expected 8.3 — wp-cli/composer will use it"
fi
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
[ -f "$A/php-fpm/roundcube.conf" ] && havuz_kur "$A/php-fpm/roundcube.conf" roundcube.conf
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
# 🔴 Debian ships an enabled default site that also claims `default_server` on
# :80 — nginx -t then fails with "duplicate default server" and NOTHING starts.
if debian_mi && [ -e /etc/nginx/sites-enabled/default ]; then
  rm -f /etc/nginx/sites-enabled/default && ok "Debian default site removed (it would collide with _default80.conf)"
fi
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
  # 🔴 group = the user the pma FPM pool runs as (apache on RHEL, www-data on
  # Debian) — the pool reads this file; nobody else may.
  chown "root:$(rhel_mi && echo apache || echo "$WEB_USER")" /etc/sanalcp/pma-internal.token 2>/dev/null || true
  chmod 640 /etc/sanalcp/pma-internal.token
fi
havuz_kur "$A/php-fpm/phpmyadmin.conf" phpmyadmin.conf
mkdir -p /var/lib/phpmyadmin/{tmp,sessions}
chown -R "$WEB_USER:$WEB_USER" /opt/phpmyadmin /var/lib/phpmyadmin 2>/dev/null
if rhel_mi; then
  restorecon -R /opt/phpmyadmin /var/lib/phpmyadmin >/dev/null 2>&1
  setsebool -P httpd_can_network_connect_db 1 >/dev/null 2>&1
fi
ok "phpMyAdmin pool + config + permissions"

# ============ 10) systemd + services ============
step "10) systemd + services"
cp "$A/systemd/sanalcp.service" /etc/systemd/system/sanalcp.service
# Daily panel DB backup (03:30) — copying the file is NOT enough, it's enabled
# --now below; otherwise the timer never fires and the install would silently end
# up with no backups.
for u in sanalcp-db-backup.service sanalcp-db-backup.timer sanalcp-recovery-check.service sanalcp-recovery-check.timer; do
  [ -f "$A/systemd/$u" ] && cp "$A/systemd/$u" "/etc/systemd/system/$u"
done
systemctl daemon-reload
if [ -f /etc/systemd/system/sanalcp-db-backup.timer ]; then
  systemctl enable --now sanalcp-db-backup.timer >/dev/null 2>&1
  systemctl is-active --quiet sanalcp-db-backup.timer \
    && ok "daily panel DB backup ACTIVE (03:30 → /var/backups/sanalcp/db, 14 days)" \
    || warn "DB backup timer failed to start — daily panel DB backup may not run"
fi
if [ -f /etc/systemd/system/sanalcp-recovery-check.timer ] && [ -x /usr/local/bin/sanalcp-recovery-check ]; then
  systemctl enable --now sanalcp-recovery-check.timer >/dev/null 2>&1
  systemctl is-active --quiet sanalcp-recovery-check.timer \
    && ok "monthly backup restore drill timer active" \
    || warn "monthly backup restore drill timer could not be started"
fi
# 🔴 enable + RESTART (svc_hazirla): the pools were written in step 6/9, and on
# Debian php-fpm has been running since apt installed it — "enable --now" would
# be a no-op and the pools would never be loaded (found live on Debian 12).
svc_hazirla "$SYS_PHP_SVC"

# 🔴 A per-version FPM master is only started if that version actually has a
# TENANT pool (c_*.conf). Reason found live on Debian 13: apt starts AND enables
# every phpX.Y-fpm at package install, so a fresh box ran 7 masters — 6 of them
# with no pool at all, costing ~197 MB and coming back on every boot. dnf never
# starts them, so this was a Debian-only regression.
#
# The check is "has tenant pools", NOT "is Debian": on an existing server with
# live tenants on 8.1 the service MUST keep running, and an update re-running
# this installer must not knock those sites offline. The panel re-enables a
# version on demand when a tenant falls back to the shared pool
# (provisioner.paylasilanFPMHazirla → enable + reload-or-restart).
atil_fpm=0
for v in $PHP_VERS; do
  if debian_mi; then svc="php$v-fpm";     pooldir="/etc/php/$v/fpm/pool.d"
  else                svc="php$v-php-fpm"; pooldir="/etc/opt/remi/php$v/php-fpm.d"; fi
  # The system PHP was already handled above; never touch it here (on Debian
  # $PHP_VERS contains 8.3, whose pool dir holds phpmyadmin/roundcube — not
  # c_*.conf — so an unguarded loop would disable the panel's own FPM).
  [ "$svc" = "$SYS_PHP_SVC" ] && continue
  if ls "$pooldir"/c_*.conf >/dev/null 2>&1; then
    svc_hazirla "$svc"
  else
    systemctl disable --now "$svc" >/dev/null 2>&1 && atil_fpm=$((atil_fpm+1))
  fi
done
if [ "$atil_fpm" -gt 0 ]; then
  ok "php-fpm (system $SYS_PHP_SVC active; $atil_fpm idle per-version master stopped — started on demand)"
else
  ok "php-fpm (system 8.3 + per-version pools, config reloaded)"
fi

# ---- named (DNS server) — nameserver for tenant domains ----
# 🔴 Paths and the service name differ per family; they MUST match
# internal/dns/yollar.go, otherwise the panel writes zone files somewhere named
# never reads and DNS silently does nothing.
#   RHEL:   /etc/named.conf · /var/named       · user named · unit named
#   Debian: /etc/bind/*     · /var/lib/bind    · user bind  · unit bind9
# /var/lib/bind is not an arbitrary choice on Debian: bind9 ships an AppArmor
# profile and that is one of the few directories named is allowed to write.
mkdir -p "$DNS_CONF_DIR" "$DNS_ZONE_DIR"
if rhel_mi; then
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
      echo "include \"$DNS_INCLUDE\";" >> "$NC"
  fi
else
  # Debian keeps the options in a separate file that named.conf already includes.
  NCO=/etc/bind/named.conf.options
  if [ -f "$NCO" ]; then
    cp -a "$NCO" "$NCO.sanal-bak" 2>/dev/null || true
    if grep -qE '^\s*recursion\s' "$NCO"; then
      sed -i -E 's/^\s*recursion\s+yes\s*;/\trecursion no;/' "$NCO"
    else
      # insert right after the opening "options {" line — authoritative only.
      sed -i '0,/^options[[:space:]]*{/s//options {\n\trecursion no;\n\tallow-query { any; };/' "$NCO"
    fi
  fi
  grep -q 'sanalcp-zones.conf' "$DNS_MAIN_CONF" 2>/dev/null || \
    echo "include \"$DNS_INCLUDE\";" >> "$DNS_MAIN_CONF"
fi
# panel zone include file (starts empty; fills up as domains are added)
[ -f "$DNS_INCLUDE" ] || printf '// sanalcp — auto-generated\n' > "$DNS_INCLUDE"
chown "root:$DNS_USER" "$DNS_INCLUDE" 2>/dev/null || true
chmod 640 "$DNS_INCLUDE" 2>/dev/null || true
chown "$DNS_USER:$DNS_USER" "$DNS_ZONE_DIR" 2>/dev/null || true
# zone files live under $DNS_ZONE_DIR (SELinux named_zone_t context is REQUIRED on RHEL)
rhel_mi && restorecon -R "$DNS_ZONE_DIR" "$DNS_CONF_DIR" >/dev/null 2>&1 || true
if named-checkconf >/dev/null 2>&1; then
  # Same reason as php-fpm: on Debian bind9 is already running (apt started it)
  # and would never re-read the include we just added.
  svc_hazirla "$(servis_ad dns)"
  systemctl is-active --quiet "$(servis_ad dns)" && ok "named (authoritative DNS, :53 open, recursion off)" || warn "named failed to start"
else
  warn "named-checkconf error — check DNS config manually"
fi

# ---- acme.sh (Let's Encrypt SSL) — the panel calls /root/.acme.sh/acme.sh ----
# LE requires a valid-looking email (@ + dot). If admin@local etc. isn't valid,
# register without a contact instead.
AEMAIL="$ADMIN_EPOSTA"; echo "$AEMAIL" | grep -qE '@[^@]+\.[^@]+$' || AEMAIL=""  # sigpipe-ok: echo builtin
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
# 🔴 RHEL only. On Debian the Apache layout is completely different
# (sites-available + a2ensite, different module names) and the panel disables the
# backend there (osfam.ApacheBackendDestekli) — installing it would leave a
# service running that nothing can use.
if rhel_mi && [ -f /etc/httpd/conf/httpd.conf ]; then
  # nginx listens on :80, so Apache listens on 127.0.0.1:10080 (mod_proxy_fcgi → php-fpm)
  if grep -qE "^Listen 80$" /etc/httpd/conf/httpd.conf; then
    sed -i "s/^Listen 80$/Listen 127.0.0.1:10080/" /etc/httpd/conf/httpd.conf
  elif ! grep -qE "^Listen 127.0.0.1:10080" /etc/httpd/conf/httpd.conf; then
    echo "Listen 127.0.0.1:10080" >> /etc/httpd/conf/httpd.conf
  fi
  cikti_esler "http_port_t.*\b10080\b" semanage port -l || \
    semanage port -a -t http_port_t -p tcp 10080 2>/dev/null || \
    semanage port -m -t http_port_t -p tcp 10080 2>/dev/null
  if apachectl configtest >/dev/null 2>&1; then
    systemctl enable --now httpd >/dev/null 2>&1 && ok "httpd (Apache backend :10080, mod_proxy_fcgi)" || warn "httpd failed to start"
  else warn "httpd configtest failed — check the Apache backend manually"; fi
elif debian_mi; then
  ok "Apache backend not installed (unsupported on Debian in v1 — nginx handles every site)"
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
# Start cron NOW + enable it (AlmaLinux's preset only enables it, doesn't start it
# until reboot → the backup cron wouldn't run until the first reboot). enable --now
# is idempotent. Unit name: crond on RHEL, cron on Debian.
systemctl enable --now "$(servis_ad cron)" >/dev/null 2>&1
systemctl is-active --quiet "$(servis_ad cron)" && ok "daily backup cron + cron ACTIVE (03:00 UTC)" || warn "cron failed to start — the backup cron may not run"

# SELinux — RHEL only; Debian has no SELinux (AppArmor doesn't need any of this).
if rhel_mi; then
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
    cikti_esler "/run/php-fpm-\[" semanage fcontext -l || \
      semanage fcontext -a -t httpd_var_run_t "/run/php-fpm-[^/]+(/.*)?" 2>/dev/null || true
    ok "SELinux fcontext: per-tenant php-fpm socket (httpd_var_run_t)"
  fi
fi

# ============ 11) Valkey + tuning ============
step "11) Valkey (Redis) + performance tuning"
command -v sanalcp-redis-setup >/dev/null 2>&1 && sanalcp-redis-setup >/dev/null 2>&1 && ok "sanalcp-redis-setup" || warn "redis-setup skipped"
command -v sanalcp-optimize >/dev/null 2>&1 && sanalcp-optimize >/dev/null 2>&1 && ok "sanalcp-optimize" || warn "optimize skipped"

# ============ 12) Start panel (migrations run at startup) ============
step "12) Starting the panel"
# 🔴 svc_hazirla (enable + RESTART): tekrar kurulumda binary diske YENİ yazılır
# ama "enable --now" çalışan ESKİ süreci öldürmez — disk yeni, bellek eski olur
# ve fark ancak bir sonraki reboot'ta ortaya çıkar. Faz 5a'da tam olarak bu
# yaşandı: düzeltilmiş süreç ayakta kaldı, reboot sonrası eski binary açıldı.
svc_hazirla sanalcp; sleep 3
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
# 🔴 The installer now creates a real admin account in the `users` table —
# THAT is the panel's primary login going forward. The legacy root/shadow
# login path (username 'root' + this server's root password) ships DISABLED
# on new installs (panel_ayarlari.root_girisi_acik=0) and can be re-enabled
# from Panel Settings if needed. SSH root access is completely unaffected —
# this only touches the panel's web login.
step "13) Admin access"
DSN="panel:${DBPASS}@tcp(127.0.0.1:3306)/panel?parseTime=true"
if [ -x /opt/sanalcp/bin/sanalcp-seed-admin ]; then
  # auxiliary users record (ownership/audit); backs the root/shadow fallback
  # path (default-off — see below), NOT the primary login anymore
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
# Gerçek admin hesabı — panelin BİRİNCİL giriş yolu artık bu (bkz.
# docs/superpowers/specs/2026-08-20-panel-auth-root-ayirma-design.md).
# Yukarıdaki root tohumlaması yerinde kalıyor: root/shadow yolu Panel
# Ayarları'ndan tekrar açılabilir ve o yol users.id=1 satırına bağlı.
if [ -z "$ADMIN_PAROLA" ]; then
  ADMIN_PAROLA=$(openssl rand -base64 18 | tr -d '/+=' | cut -c1-20)
fi
[ -x /opt/sanalcp/bin/sanalcp-seed-admin ] || die "sanalcp-seed-admin bulunamadı — panel admin hesabı oluşturulamaz, kurulum durduruldu"
/opt/sanalcp/bin/sanalcp-seed-admin -dsn "$DSN" -kullanici "$ADMIN_KULLANICI" \
  -parola "$ADMIN_PAROLA" -eposta "$ADMIN_EPOSTA" -dil "$PANEL_LANG" >/dev/null 2>&1 \
  && ok "admin account created" || die "admin account could not be created"

# Root/shadow giriş yolunu KAPAT. Migration bunu 1 (açık) olarak ekliyor ki
# mevcut kurulumlar kilitlenmesin; yeni kurulumda ise girecek gerçek bir admin
# zaten var, o yüzden kapalı başlıyoruz.
mysql panel -e "UPDATE panel_ayarlari SET root_girisi_acik=0 WHERE id=1;" >/dev/null 2>&1 || true
# 🔴 The UPDATE must be VERIFIED, not assumed. The root_girisi_acik column is
# created by migration 0069, which runs at the first panel start (step 12); if
# that start failed or the migration did not land, the UPDATE above is a no-op
# and the install would ship with the flag at its DEFAULT 1 (root login OPEN) --
# silently breaking the very promise this branch makes. Read the value back and
# stop the install if it is not 0.
# NOTE: no `mysql ... | grep -q` pipeline here -- with `set -o pipefail` that
# pattern silently yields false negatives (see internal/osfam/pipefail_grep_test.go).
ROOT_GIRISI_DEGER="$(mysql -N -B panel -e "SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1;" 2>/dev/null)"
case "$ROOT_GIRISI_DEGER" in
  0) ok "panel root login disabled (SSH root access is unaffected)" ;;
  *) die "could not disable panel root login (root_girisi_acik='$ROOT_GIRISI_DEGER') -- migration 0069 probably did not run; check 'journalctl -u sanalcp' and re-run the installer" ;;
esac

echo
echo "  ╔══════════════════════════════════════════════════════════════╗"
echo "  ║  PANEL LOGIN — bu parola BİR KEZ gösterilir, kaydedin        ║"
echo "  ╚══════════════════════════════════════════════════════════════╝"
echo "    kullanıcı : $ADMIN_KULLANICI"
echo "    parola    : $ADMIN_PAROLA"
echo
echo "  Bu parola hiçbir dosyaya yazılmadı. Kaybederseniz SSH ile:"
echo "    DSN=\$(grep -m1 '^PANEL_DB_DSN=' /etc/sanalcp/env | cut -d= -f2-)"
echo "    /opt/sanalcp/bin/sanalcp-seed-admin -dsn \"\$DSN\" \\"
echo "      -kullanici $ADMIN_KULLANICI -parola '<yeni-parola>'"
echo
echo "  Panel root girişini SSH'tan geri açmak (acil durum):"
echo "    mysql panel -e \"UPDATE panel_ayarlari SET root_girisi_acik=1 WHERE id=1;\""
echo

# Shell history hardening — the panel login IS this server's root password (see
# internal/auth/handlers.go:rootShadowHash), so operators routinely handle that
# password in a root shell. AlmaLinux/Debian default to HISTCONTROL=ignoredups,
# which still records EVERY unique line — a password pasted at a prompt that was
# not actually reading a password lands in ~/.bash_history in cleartext and stays
# there. `ignoreboth` adds `ignorespace`: any command typed with a LEADING SPACE
# is never written to history.
install -d -m 0755 /etc/profile.d
cat > /etc/profile.d/sanalcp-history.sh <<'HISTEOF'
# SanalCP: commands starting with a SPACE are kept out of shell history
# (ignorespace) + repeated commands collapse to one entry (ignoredups).
# When you must paste a password or token into the shell, prefix the line
# with a space so it is never persisted to ~/.bash_history.
export HISTCONTROL=ignoreboth
HISTEOF
chmod 0644 /etc/profile.d/sanalcp-history.sh
ok "shell history hardening (HISTCONTROL=ignoreboth)"

# ============ 14) Permission repair ============
step "14) Permission/SELinux repair"
command -v sanalcp-repair >/dev/null 2>&1 && sanalcp-repair --quiet >/dev/null 2>&1 && ok "sanalcp-repair" || warn "repair skipped"

# ============ 15) VERIFICATION ============
step "15) Verification"
IP=$(hostname -I 2>/dev/null | awk '{print $1}')
CODE=$(curl -sk -o /dev/null -w '%{http_code}' https://127.0.0.1:8443/ 2>/dev/null)
API=$(curl -sk -o /dev/null -w '%{http_code}' https://127.0.0.1:8443/api/v1/domains 2>/dev/null)
# 🔴 Localhost reachability alone doesn't catch an external firewall (firewalld,
# cloud provider security group) silently blocking everything — exactly what
# happened on the AlmaLinux 9.8 test VPS this section is built for. Check the
# public IP too, over a real TCP connection, so a misconfigured firewall shows
# up here instead of as a confusing "installed fine but I can't reach it" report.
if [ -n "$IP" ]; then
  EXT_CODE=$(curl -sk -o /dev/null -w '%{http_code}' --connect-timeout 5 "https://${IP}:8443/" 2>/dev/null)
  if [ "$EXT_CODE" = "200" ]; then
    ok "panel reachable from outside (https://${IP}:8443/ → HTTP 200)"
  else
    warn "panel is NOT reachable from outside (https://${IP}:8443/ → '${EXT_CODE:-no response}') — check your cloud provider's firewall/security group; SanalCP's own firewall (nftables) and firewalld were already handled by this installer"
  fi
fi
# valkey vs redis: EL9 and Debian 12 still ship redis (see internal/osfam).
KV_SVC=$(debian_mi && echo valkey-server || echo valkey)
cikti_esler "$KV_SVC" systemctl list-unit-files --no-legend "$KV_SVC.service" || \
  KV_SVC=$(debian_mi && echo redis-server || echo redis)
echo -e "  services: $(systemctl is-active "$(servis_ad db)" "$(servis_ad web)" "$KV_SVC" "$SYS_PHP_SVC" "$(servis_ad dns)" "$(servis_ad ftp)" sanalcp "$(servis_ad cron)" | tr '\n' ' ')"
echo -e "  panel :8443 → HTTP $CODE   ·   API (auth) → HTTP $API   ·   DNS :53 → $(systemctl is-active "$(servis_ad dns)")   ·   FTP :21 → $(systemctl is-active "$(servis_ad ftp)")"
echo -e "  tools: SSL/acme.sh $([ -x /root/.acme.sh/acme.sh ] && echo ✓ || echo ✗)   ·   firewall/nft $(command -v nft >/dev/null && echo ✓ || echo ✗)   ·   unzip/zip $(command -v unzip >/dev/null && command -v zip >/dev/null && echo ✓ || echo ✗)   ·   composer $(command -v composer >/dev/null && echo ✓ || echo ✗)   ·   apache backend $(rhel_mi && systemctl is-active httpd || echo "n/a (Debian)")"
echo -e "  isolation: plan-driven resource limits (cgroup slice) + per-tenant PHP-FPM (CageFS equivalent) READY   ·   bubblewrap $(command -v bwrap >/dev/null && echo ✓ || echo ✗)"
echo
echo -e "${c_g}═══════════════════════════════════════════════${c_0}"
echo -e "${c_g} ✓ SanalCP installation complete${c_0}"
echo -e "   Panel:    ${c_b}https://${IP:-SERVER_IP}:8443${c_0}"
echo -e "   Username: ${c_b}${ADMIN_KULLANICI}${c_0}   Password: ${c_b}${ADMIN_PAROLA}${c_0}"
echo -e "   ${c_y}Write this password down — it was not saved to any file.${c_0}"
echo -e "   (panel login for 'root' is DISABLED on new installs; SSH root access is unaffected)"
if [ -n "${KOTA_FLAG:-}" ] && ! cikti_esler '(^|,)(usrquota|uquota|quota)(,|$)' findmnt -no OPTIONS /; then
  echo -e "   ${c_y}Disk quota: $KOTA_FLAG was written to GRUB — takes effect after a ONE-TIME reboot.${c_0}"
fi
echo -e "${c_g}═══════════════════════════════════════════════${c_0}"

# ============ 16) ONE-TIME REBOOT ============
# 🔴 Yeni kurulumda ilk yeniden başlatma OPSİYONEL DEĞİL. Disk kotası GRUB'a
# yazılan $KOTA_FLAG (rootflags=usrquota / uquota) ile geliyor ve o bayrak
# çekirdeğe ancak reboot'ta ulaşıyor: o ana kadar panel plan limitlerini
# GÖSTERİR ama sistem HİÇBİRİNİ UYGULAMAZ — sessiz ve tehlikeli bir ara durum
# (aynı sınıf hata Debian 12/13 ve Ubuntu 24.04'te canlıda yaşandı, bkz. 0.8.0
# duyurusu). Ayrıca kurulum sırasında gelen çekirdek/systemd güncellemeleri ve
# yeni yazılan tüm unit'lerin temiz açılışta gerçekten ayağa kalktığı ancak bu
# reboot'tan sonra doğrulanmış olur. Bu yüzden kurulum artık reboot'u kendi
# üstleniyor; operatörün "sonra yaparım" deyip unutmasına bırakılmıyor.
#
# Sıralama önemli: bu blok panel admin parolasının basıldığı kutudan SONRA
# çalışır ve TTY varsa tuşa basılmadan reboot etmez — parola ekranda kaybolmaz.
if [ "$REBOOT_ET" = "0" ]; then
  echo
  warn "--no-reboot / SANALCP_REBOOT=0 — reboot the server yourself with 'reboot'; until then disk quota limits are NOT enforced"
else
  echo
  if [ "$PANEL_LANG" = "en" ]; then
    R1="The server must reboot ONCE to finish setup (disk quota takes effect at boot)."
    R2="Press any key to reboot now  ·  Ctrl+C to cancel (then run 'reboot' yourself)"
    R3="No TTY — rebooting in 10 seconds (use --no-reboot to skip)"
  else
    R1="Kurulumun tamamlanması için sunucunun BİR KEZ yeniden başlatılması gerekiyor (disk kotası açılışta devreye girer)."
    R2="Yeniden başlatmak için bir tuşa basın  ·  İptal için Ctrl+C (sonra kendiniz 'reboot' çalıştırın)"
    R3="TTY yok — 10 saniye içinde yeniden başlatılıyor (atlamak için --no-reboot)"
  fi
  echo -e "  ${c_y}${R1}${c_0}"
  # curl | bash akışında stdin BETİĞİN KENDİSİ — `read` /dev/tty'ye yönlendirilmezse
  # betik gövdesinden okur ve hiç beklemeden geçer (adım 0'daki dil seçimiyle aynı tuzak).
  #
  # 🔴 `[ -r /dev/tty ]` BURADA YETMEZ: denetleyen terminali olmayan bir süreçte
  # (ssh host 'curl ... | bash', cron, systemd, nohup) düğüm görünür ve okunabilir
  # görünür, ama AÇMAK ENXIO ile başarısız olur. O durumda `read < /dev/tty` anında
  # hata verir ve akış uyarısız/beklemesiz doğrudan reboot'a düşerdi. Bu yüzden
  # gerçekten AÇARAK sınıyoruz; başarısızsa TTY'siz kola geçiyoruz.
  if { exec 3</dev/tty; } 2>/dev/null; then
    printf "  %s " "$R2"
    read -rsn1 <&3 || true
    exec 3<&-
    echo
  else
    warn "$R3"
    sleep 10
  fi
  ok "rebooting…"
  # systemctl yoksa/çalışmıyorsa (konteyner benzeri ortam) düz reboot'a düş.
  systemctl reboot 2>/dev/null || reboot
fi
