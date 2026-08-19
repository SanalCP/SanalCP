#!/usr/bin/env bash
# sanalcp-ortak — OS family detection + name resolution, SHARED by the installer
# and every ops tool. This file only DEFINES things; running it does nothing.
#
# Source it:
#   . /usr/local/bin/sanalcp-ortak            # on an installed server
#   . "$A/ops/sanalcp-ortak.sh"               # from an extracted release (installer)
#
# 🔴 WHY THIS FILE EXISTS: the same package/service table would otherwise be
# copy-pasted into sanalcp-install.sh and six ops scripts. Every copy is a place
# where "dnf install bind" survives a Debian port review. There is exactly ONE
# shell-side table — this one — and it is checked against the Go side
# (internal/osfam) by internal/osfam/installer_paritesi_test.go.
#
# Keep it POSIX-ish bash, no external deps: it is sourced by scripts that run
# before anything is installed.

# ---- OS family ----
# SANALCP_OS_RELEASE: test-only seam (the parity test feeds a fake os-release).
. "${SANALCP_OS_RELEASE:-/etc/os-release}" 2>/dev/null || true
OS_ID="${ID:-}"; OS_SURUM="${VERSION_ID:-}"; OS_KODADI="${VERSION_CODENAME:-}"
case "$OS_ID" in
  debian|ubuntu)                  OS_AILE=debian ;;
  almalinux|rocky|rhel|centos|ol) OS_AILE=rhel ;;
  *) case " ${ID_LIKE:-} " in
       *debian*)        OS_AILE=debian ;;
       *rhel*|*fedora*) OS_AILE=rhel ;;
       *)               OS_AILE=rhel ;;   # historical default
     esac ;;
esac
debian_mi(){ [ "$OS_AILE" = debian ]; }
rhel_mi(){   [ "$OS_AILE" = rhel ]; }

# EL major ("platform:el9" / "platform:el10") — RHEL family only.
EL_MAJOR=""
if rhel_mi; then
  EL_MAJOR=$(printf '%s' "${PLATFORM_ID:-platform:el10}" | sed 's/.*el//')
  case "$EL_MAJOR" in ''|*[!0-9]*) EL_MAJOR=10 ;; esac
fi

# valkey_yok: does this release LACK the valkey-server package? Mirrors
# osfam.valkeyYok — keep the two in step (internal/osfam parity test).
#
# Valkey forked from Redis in March 2024, so every archive frozen before that
# ships redis-server only:
#   Debian 12 bookworm (2023) no · Debian 13 trixie yes
#   Ubuntu 22.04 jammy no · Ubuntu 24.04 noble no · Ubuntu 24.10+ / 26.04 yes
#
# 🔴 Ubuntu used to return "has valkey" unconditionally here. Wrong for 24.04:
# noble's archive has no valkey-server at all (verified against
# archive.ubuntu.com main+universe), so the install would lose its cache layer.
valkey_yok(){
  case "$OS_KODADI" in
    bookworm|bullseye|buster|stretch|jammy|noble)          return 0 ;;
    trixie|forky|sid|oracular|plucky|questing|resolute)    return 1 ;;
  esac
  if [ "$OS_ID" = ubuntu ]; then
    # Ubuntu sürümü YIL.AY: 24.04 ve öncesi valkey'siz, 24.10 ilki.
    case "${OS_SURUM%%.*}" in
      2[0-3]|1?) return 0 ;;
      24) case "${OS_SURUM#*.}" in 04|0[0-4]) return 0 ;; *) return 1 ;; esac ;;
      *)  return 1 ;;
    esac
  fi
  case "${OS_SURUM%%.*}" in 9|10|11|12) return 0 ;; esac
  return 1
}

# rakip_onbellek_kapat: 6379'u yalnız BİZİM seçtiğimiz cache servisi tutsun.
#
# 🔴 NEDEN GEREKLİ (Ubuntu 26.04'te canlı bulundu): Ubuntu'nun rspamd paketi
#
#     Recommends: redis-server
#
# yazar — Debian 13'teki gibi `valkey-server | redis-server` ALTERNATİFLİ değil.
# apt Recommends'i varsayılan olarak kurduğu için, biz valkey-server kurup
# ayağa kaldırdıktan SONRA gelen rspamd kurulumu redis-server'ı da getiriyor
# (ve apt onu enable edip başlatıyor).
#
# Sonuç sinsi: o an valkey 6379'u tuttuğu için redis başlayamaz ve kurulum
# sağlıklı görünür. Ama İKİSİ DE enable olduğundan bir sonraki açılışta portu
# hangisinin kapacağı YARIŞA kalır. Ubuntu 26.04'te reboot sonrası redis kazandı:
# valkey crash-loop'a girdi (bind: Address already in use), panelin önbellek
# ayarları (maxmemory/allkeys-lru/acl) valkey.conf'ta kaldığı için hiç
# uygulanmadı ve panel cache servisini "failed" gördü.
#
# Paket KALDIRILMIYOR — rspamd'ın Recommends'i memnun kalsın; yalnız rakip
# servis disable+stop ediliyor, sonra bizimki portu geri alıyor.
rakip_onbellek_kapat(){
  local benim rakip
  benim=$(servis_ad cache)
  case "$benim" in
    valkey-server|valkey) rakip=redis-server ;;
    redis-server|redis)   rakip=valkey-server ;;
    *) return 0 ;;
  esac
  systemctl cat "$rakip" >/dev/null 2>&1 || return 0   # kurulu değil: yapacak iş yok
  systemctl disable --now "$rakip" >/dev/null 2>&1 || true
  # Rakip portu bırakmış olabilir; bizimki devralsın (zaten ayaktaysa zararsız).
  systemctl restart "$benim" >/dev/null 2>&1 || true
  return 0
}

# cikti_esler <ERE deseni> <komut> [arg...] : komutun çıktısında desen var mı?
#
# 🔴 `komut | grep -q desen` KULLANMAYIN. Bu betikler `set -o pipefail` ile
# koşuyor ve `grep -q` eşleşmeyi bulduğu ANDA çıkar; bu boruyu kapatır, üretici
# SIGPIPE alır ve boru hattı 141 döner — EŞLEŞME BAŞARILI OLSA BİLE. Sonuç
# sessiz bir yanlış-olumsuz: kontrol "yok" der, oysa vardır.
#
# Üretici ne kadar çok/yavaş yazarsa risk o kadar büyük. Bu yüzden aynı kod
# Debian'da geçip Ubuntu'da düşebiliyor: çıktı uzunluğu değişince yarış çevriliyor.
# Canlı örnekler:
#   · Ubuntu 24.04'te `apt-cache policy php8.3-fpm | grep -q sury.org` rc=141
#     döndü ve kurulum adım 1'de, doğru olan sources.list dosyasını suçlayarak öldü.
#   · Kabul testinde `ss -lntp | grep -q 127.0.0.1:8891` OpenDKIM'i dinlemiyor
#     gösterdi; OpenDKIM o sırada dinliyordu.
#   · Aynı sınıf daha önce `quotaon | grep` için de yaşandı
#     (internal/kaynaklimit/kota_ext4.go).
#
# Çıktı önce DEĞİŞKENE alınır, eşleştirme bash'in kendi `=~` operatörüyle
# yapılır — hiç boru yok, dolayısıyla SIGPIPE de yok.
cikti_esler(){
  local desen="$1"; shift
  local cikti
  cikti=$("$@" 2>/dev/null || true)
  [[ "$cikti" =~ $desen ]]
}

# ---- package manager ----
pkg_kur(){ # install, quiet; non-zero on failure
  if debian_mi; then
    DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a \
      apt-get install -y -o Dpkg::Options::=--force-confold -o Dpkg::Options::=--force-confdef "$@" >/dev/null 2>&1
  else
    dnf install -y "$@" >/dev/null 2>&1
  fi
}
pkg_kurulu(){
  if debian_mi; then
    # Boru YOK: cikti_esler ile aynı gerekçe (pipefail + grep -q = SIGPIPE tuzağı).
    local durum; durum=$(dpkg-query -W -f='${Status}' "$1" 2>/dev/null || true)
    [[ "$durum" == "install ok installed"* ]]
  else
    rpm -q "$1" >/dev/null 2>&1
  fi
}
# depo_yenile: paket indekslerini yeniler.
#
# 🔴 Çıkış kodunu ve apt/dnf'in KENDİ hata metnini korur (DEPO_SON_CIKTI).
# Eskiden her şey /dev/null'a gidiyordu; Ubuntu 24.04 kurulumunda
# packages.sury.org'a erişim geçici olarak başarısız oldu, hata kaybolduğu için
# kurulum "repo eklendi ama php8.3-fpm ona çözülmüyor — sources.list'i kontrol
# edin" diyerek YANLIŞ yeri suçladı. Dosya kusursuzdu; sorun ağdaydı.
DEPO_SON_CIKTI=""
depo_yenile(){
  local cikti rc
  if debian_mi; then cikti=$(apt-get update -qq 2>&1); rc=$?
  else                cikti=$(dnf makecache -q 2>&1);  rc=$?; fi
  DEPO_SON_CIKTI="$cikti"
  return "$rc"
}

# ---- logical name -> real package / systemd unit ----
paket_ad(){
  case "$1" in
    web)        echo nginx ;;
    db)         echo mariadb-server ;;
    dns)        debian_mi && echo bind9         || echo bind ;;
    dns-arac)   debian_mi && echo bind9-utils   || echo bind-utils ;;
    ftp)        debian_mi && echo pure-ftpd-mysql || echo pure-ftpd ;;
    cache)      if debian_mi; then valkey_yok && echo redis-server || echo valkey-server
                else echo valkey; fi ;;
    antivirus)  debian_mi && echo clamav-daemon    || echo clamav ;;
    av-guncel)  debian_mi && echo clamav-freshclam || echo clamav-update ;;
    cron)       debian_mi && echo cron          || echo cronie ;;
    apache)     debian_mi && echo apache2       || echo httpd ;;
    apache-ara) debian_mi && echo apache2-utils || echo httpd-tools ;;
    bsdtar)     debian_mi && echo libarchive-tools || echo bsdtar ;;
    ssh)        echo openssh-server ;;
    kota-xfs)   echo xfsprogs ;;
    kota-ext)   echo quota ;;
    *)          echo "$1" ;;
  esac
}
servis_ad(){
  case "$1" in
    web)    echo nginx ;;
    db)     echo mariadb ;;
    dns)    debian_mi && echo bind9   || echo named ;;
    ftp)    debian_mi && echo pure-ftpd-mysql || echo pure-ftpd ;;
    cache)  if debian_mi; then valkey_yok && echo redis-server || echo valkey-server
            else echo valkey; fi ;;
    antivirus) debian_mi && echo clamav-daemon || echo clamd@scan ;;
    cron)   debian_mi && echo cron    || echo crond ;;
    apache) debian_mi && echo apache2 || echo httpd ;;
    # 🔴 Debian's SSH unit is ssh.service; sshd.service is only an ALIAS and
    # journald records the real name, so `journalctl -u sshd` comes back EMPTY.
    ssh)    debian_mi && echo ssh     || echo sshd ;;
    *)      echo "$1" ;;
  esac
}

# WEB_USER: the user nginx runs as — PHP-FPM pools' listen.owner/group. A wrong
# value means nginx can't open the FPM socket and EVERY site 502s.
WEB_USER=$(debian_mi && echo www-data || echo nginx)

# ---- BIND layout (mirror of internal/dns/yollar.go) ----
# Debian's bind9 ships an AppArmor profile: named may write under /var/lib/bind,
# NOT under /var/named.
if debian_mi; then
  DNS_ZONE_DIR=/var/lib/bind; DNS_CONF_DIR=/etc/bind;  DNS_USER=bind
  DNS_MAIN_CONF=/etc/bind/named.conf
else
  DNS_ZONE_DIR=/var/named;    DNS_CONF_DIR=/etc/named; DNS_USER=named
  DNS_MAIN_CONF=/etc/named.conf
fi
DNS_INCLUDE="$DNS_CONF_DIR/sanalcp-zones.conf"

# ---- system PHP: phpMyAdmin + webmail run on it (mirror of provisioner.SistemPHP*) ----
if debian_mi; then
  SYS_PHP_POOL_DIR=/etc/php/8.3/fpm/pool.d; SYS_PHP_SVC=php8.3-fpm
  MYSQL_SOCK=/run/mysqld/mysqld.sock
else
  SYS_PHP_POOL_DIR=/etc/php-fpm.d;          SYS_PHP_SVC=php-fpm
  MYSQL_SOCK=/var/lib/mysql/mysql.sock
fi

# MariaDB drop-in config dizini: RHEL /etc/my.cnf.d, Debian
# /etc/mysql/mariadb.conf.d. 🔴 Yanlış dizine yazmak SESSİZ başarısızlıktır —
# dosya oluşur (ya da oluşmaz), MariaDB onu hiç okumaz ve tuning uygulanmamış
# olmasına rağmen "yazıldı" denir. Var olan dizin tercih edilir.
MYSQL_CONF_DIR=""
for d in $(debian_mi && echo "/etc/mysql/mariadb.conf.d /etc/mysql/conf.d /etc/my.cnf.d" \
                    || echo "/etc/my.cnf.d /etc/mysql/mariadb.conf.d /etc/mysql/conf.d"); do
  [ -d "$d" ] && { MYSQL_CONF_DIR="$d"; break; }
done
[ -n "$MYSQL_CONF_DIR" ] || MYSQL_CONF_DIR=$(debian_mi && echo /etc/mysql/mariadb.conf.d || echo /etc/my.cnf.d)

# php_pkg <version token> <extension> -> real package name
#   Remi: "83" + "fpm" -> php83-php-fpm      sury: "8.3" + "fpm" -> php8.3-fpm
php_pkg(){ if debian_mi; then echo "php$1-$2"; else echo "php$1-php-$2"; fi; }

# php_kurulu_surumler: PHP versions actually installed, as version TOKENS for the
# current family ("83 84" on Remi, "8.3 8.4" on sury). Empty output = none found.
php_kurulu_surumler(){
  if debian_mi; then
    ls -d /etc/php/*/fpm 2>/dev/null | sed 's#/etc/php/##; s#/fpm##'
  else
    ls -d /etc/opt/remi/php* 2>/dev/null | sed 's#/etc/opt/remi/php##'
  fi
}

# svc_hazirla <unit...>: enable + RESTART.
#
# 🔴 "systemctl enable --now" YETMEZ ve bu fark yalnız Debian'da görünür:
# apt, paketi kurarken servisi BAŞLATIR (dnf başlatmaz). Biz config'i kurulumun
# ilerleyen adımlarında yazdığımız için servis o sırada çoktan çalışıyordur ve
# "enable --now" çalışan bir birimde NO-OP'tur → yazdığımız config asla
# yüklenmez. Faz 5a canlı testinde tam olarak bu oldu: php-fpm havuzları ve
# named include'u diske yazıldı, süreçler onları hiç görmedi.
svc_hazirla(){
  local u
  for u in "$@"; do
    systemctl enable "$u" >/dev/null 2>&1
    systemctl restart "$u" >/dev/null 2>&1
  done
}

# selinux_var: is SELinux actually in play? (Debian: never.)
selinux_var(){ rhel_mi && command -v getenforce >/dev/null 2>&1 && [ "$(getenforce 2>/dev/null)" != "Disabled" ]; }
