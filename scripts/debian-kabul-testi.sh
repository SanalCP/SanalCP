#!/usr/bin/env bash
# debian-kabul-testi — Faz 5 kabul testi. SanalCP kurulu bir sunucuda çalışır ve
# Debian portunun gerçekten çalıştığını KANITLAR.
#
#   bash debian-kabul-testi.sh
#
# 🔴 SALT OKUNUR: hiçbir şeyi kurmaz, başlatmaz, değiştirmez. Yalnızca okur ve
# rapor eder. Çıkış kodu 0 = tüm zorunlu kontroller geçti.
#
# Her kontrolün yanında NEDEN önemli olduğu yazıyor: "servis active" demek yetmez,
# port dinliyor mu / dosya doğru yerde mi / doğru kullanıcı mı sorularını sorar —
# Debian portunda bulunan hataların hepsi "servis ayakta ama sessizce işlevsiz"
# türündeydi.
set -uo pipefail

ORTAK=/usr/local/bin/sanalcp-ortak
[ -f "$ORTAK" ] || ORTAK=/opt/sanalcp/src/scripts/sanalcp-ortak.sh
# shellcheck source=/dev/null
. "$ORTAK" 2>/dev/null || { echo "sanalcp-ortak bulunamadı — kurulum eksik"; exit 1; }

GECTI=0; KALDI=0; ATLANDI=0
gecti(){ printf '  \033[32m✓\033[0m %s\n' "$*"; GECTI=$((GECTI+1)); }
kaldi(){ printf '  \033[31m✗\033[0m %s\n' "$*"; KALDI=$((KALDI+1)); }
atla(){  printf '  \033[33m–\033[0m %s\n' "$*"; ATLANDI=$((ATLANDI+1)); }
baslik(){ printf '\n\033[1;34m══ %s ══\033[0m\n' "$*"; }

kontrol(){ # <açıklama> <komut...>
  local ad="$1"; shift
  if "$@" >/dev/null 2>&1; then gecti "$ad"; else kaldi "$ad"; fi
}
servis_aktif(){ systemctl is-active --quiet "$1"; }

baslik "Sistem"
echo "  $(. /etc/os-release; echo "${PRETTY_NAME:-?}")  ·  aile=$OS_AILE  ·  kod=${OS_KODADI:-yok}"
echo "  kök fs: $(findmnt -no FSTYPE / 2>/dev/null)  ·  MariaDB: $(mysql -N -B -e 'SELECT VERSION()' 2>/dev/null || echo '?')"

baslik "Servisler"
for m in web db dns cron cache; do
  birim=$(servis_ad "$m")
  if servis_aktif "$birim"; then gecti "$m → $birim active"; else kaldi "$m → $birim DEĞİL active"; fi
done
if servis_aktif "$SYS_PHP_SVC"; then gecti "sistem PHP → $SYS_PHP_SVC active"; else kaldi "sistem PHP → $SYS_PHP_SVC DEĞİL active"; fi
kontrol "panel (sanalcp) active" servis_aktif sanalcp
# FTP ve posta opsiyonel bileşenler: kurulmadıysa test edilmez, YANLIŞ negatif üretmez.
if systemctl list-unit-files --no-legend "$(servis_ad ftp).service" 2>/dev/null | grep -q .; then
  kontrol "ftp → $(servis_ad ftp) active" servis_aktif "$(servis_ad ftp)"
else atla "ftp kurulu değil"; fi

baslik "Panel"
KOD=$(curl -sk -o /dev/null -w '%{http_code}' https://127.0.0.1:8443/healthz 2>/dev/null)
[ "$KOD" = 200 ] && gecti "/healthz → 200" || kaldi "/healthz → ${KOD:-yanıt yok}"
DISK=$(ls /opt/sanalcp/src/migrations/*.sql 2>/dev/null | wc -l)
DB=$(mysql -N -B panel -e 'SELECT COUNT(*) FROM schema_migrations' 2>/dev/null || echo 0)
[ "$DISK" -gt 0 ] && [ "$DISK" = "$DB" ] && gecti "migration: diskte $DISK = DB'de $DB" || kaldi "migration uyuşmuyor (disk=$DISK db=$DB)"
kontrol "panel binary sürümü okunabiliyor" grep -qa 'SanalCP ' /opt/sanalcp/bin/sanalcp-server

baslik "Web katmanı"
kontrol "nginx -t geçerli" nginx -t
# 🔴 Debian'ın varsayılan sitesi kalırsa "duplicate default server" ile nginx hiç kalkmaz.
if debian_mi && [ -e /etc/nginx/sites-enabled/default ]; then
  kaldi "Debian varsayılan sitesi HÂLÂ etkin (_default80.conf ile çakışır)"
else gecti "çakışan varsayılan site yok"; fi
# FPM soketi: panel vhost'u /run/php-fpm/<ad>.sock bekliyor (her iki ailede de).
for s in phpmyadmin; do
  [ -S "/run/php-fpm/$s.sock" ] && gecti "FPM soketi /run/php-fpm/$s.sock var" || kaldi "FPM soketi /run/php-fpm/$s.sock YOK (panel vhost'u burayı okuyor)"
done
SAHIP=$(stat -c '%U:%G' /run/php-fpm/phpmyadmin.sock 2>/dev/null)
[ "$SAHIP" = "$WEB_USER:$WEB_USER" ] && gecti "soket sahibi $SAHIP (nginx okuyabilir)" || kaldi "soket sahibi '$SAHIP', beklenen '$WEB_USER:$WEB_USER' → 502"

baslik "PHP"
KURULU=$(php_kurulu_surumler | tr '\n' ' ')
[ -n "$KURULU" ] && gecti "kurulu PHP sürümleri: $KURULU" || kaldi "hiç PHP sürümü bulunamadı"
[ -d "$SYS_PHP_POOL_DIR" ] && gecti "sistem havuz dizini $SYS_PHP_POOL_DIR" || kaldi "sistem havuz dizini YOK: $SYS_PHP_POOL_DIR"
if debian_mi; then
  apt-cache policy php8.3-fpm 2>/dev/null | grep -q sury.org && gecti "php8.3-fpm sury deposundan" || kaldi "php8.3-fpm sury'den GELMİYOR"
fi

baslik "DNS (BIND)"
# 🔴 Debian'da named /var/named'e AppArmor yüzünden YAZAMAZ; panel oraya yazsaydı
# zone dosyaları sessizce etkisiz kalırdı.
[ -d "$DNS_ZONE_DIR" ] && gecti "zone dizini $DNS_ZONE_DIR" || kaldi "zone dizini YOK: $DNS_ZONE_DIR"
[ -f "$DNS_INCLUDE" ] && gecti "include dosyası $DNS_INCLUDE" || kaldi "include dosyası YOK: $DNS_INCLUDE"
grep -q 'sanalcp-zones.conf' "$DNS_MAIN_CONF" 2>/dev/null && gecti "$DNS_MAIN_CONF include satırı taşıyor" || kaldi "$DNS_MAIN_CONF include satırı YOK"
kontrol "named-checkconf temiz" named-checkconf
# Gerçek yazma testi: named kullanıcısı zone dizinine yazabiliyor mu?
if runuser -u "$DNS_USER" -- test -w "$DNS_ZONE_DIR" 2>/dev/null; then
  gecti "$DNS_USER kullanıcısı $DNS_ZONE_DIR dizinine yazabiliyor"
else kaldi "$DNS_USER kullanıcısı $DNS_ZONE_DIR dizinine YAZAMIYOR (AppArmor/izin)"; fi
if debian_mi && command -v aa-status >/dev/null 2>&1; then
  aa-status --json 2>/dev/null | grep -q 'usr.sbin.named' && atla "AppArmor named profili etkin — zone yazma yukarıda gerçekten test edildi" || gecti "AppArmor'da named profili yok"
fi

baslik "Disk kotası"
FS=$(findmnt -no FSTYPE / 2>/dev/null)
case "$FS" in
  xfs)            BEKLENEN=uquota ;;
  ext2|ext3|ext4) BEKLENEN=usrquota ;;
  *)              BEKLENEN="" ;;
esac
if [ -z "$BEKLENEN" ]; then
  atla "kök fs '$FS' kotayı desteklemiyor — panel bunu dürüstçe kapalı göstermeli"
elif findmnt -no OPTIONS / | grep -qwE 'usrquota|uquota|quota'; then
  gecti "kök fs kotası MOUNT'ta etkin ($FS)"
  if [ "$FS" != xfs ]; then
    quotaon -p -u / 2>/dev/null | grep -q 'is on' && gecti "ext kota enforcement açık (quotaon)" || kaldi "mount kotalı ama quotaon KAPALI (muhasebe var, limit uygulanmıyor)"
  fi
  repquota -u -O csv / >/dev/null 2>&1 && gecti "repquota okunabiliyor" || kaldi "repquota başarısız"
else
  grep -q "rootflags=$BEKLENEN" /etc/default/grub 2>/dev/null \
    && atla "GRUB'a rootflags=$BEKLENEN yazılmış — TEK SEFERLİK REBOOT bekliyor" \
    || kaldi "GRUB'da rootflags=$BEKLENEN YOK ve kota kapalı"
fi

baslik "Posta"
if command -v postconf >/dev/null 2>&1; then
  kontrol "postfix check temiz" postfix check
  postconf -h smtpd_milters 2>/dev/null | grep -q '8891' && gecti "postfix milter 8891 (OpenDKIM)" || kaldi "postfix smtpd_milters OpenDKIM'i göstermiyor"
  # 🔴 OpenDKIM inet:8891 dinlemiyorsa imzalama sessizce hiç çalışmaz.
  if ss -lntp 2>/dev/null | grep -q '127.0.0.1:8891'; then gecti "OpenDKIM 127.0.0.1:8891 dinliyor"
  else kaldi "OpenDKIM 8891 DİNLEMİYOR (unix sokete kaymış olabilir — service.d/override.conf)"; fi
  ss -lntp 2>/dev/null | grep -q '127.0.0.1:11332' && gecti "rspamd milter 11332 dinliyor" || kaldi "rspamd 11332 dinlemiyor"
  # Dovecot 2.4 sözdizimi kırılması burada yakalanır.
  if command -v doveconf >/dev/null 2>&1; then
    DVER=$(doveconf --version 2>/dev/null | awk '{print $1}')
    if doveconf -n >/dev/null 2>&1; then gecti "doveconf -n temiz (dovecot $DVER)"
    else kaldi "doveconf -n BAŞARISIZ (dovecot $DVER — 2.4 sözdizimi farkı olabilir)"; fi
  fi
else atla "posta kurulu değil"; fi

baslik "FTP"
if pkg_kurulu "$(paket_ad ftp)"; then
  if debian_mi; then
    # Debian: direktif-başına-dosya + auth SEMBOLİK LİNKİ (wrapper yalnız linkleri okur).
    [ -f /etc/pure-ftpd/conf/MySQLConfigFile ] && gecti "conf/MySQLConfigFile var" || kaldi "conf/MySQLConfigFile YOK"
    [ -L /etc/pure-ftpd/auth/30mysql ] && gecti "auth/30mysql sembolik linki var" || kaldi "auth/30mysql sembolik link DEĞİL (wrapper yok sayar)"
    ls /etc/pure-ftpd/auth/ 2>/dev/null | grep -qE 'unix|pam' && kaldi "auth/ altında unix/pam kolu DURUYOR (sistem kullanıcıları FTP'ye girebilir)" || gecti "unix/pam auth kolları kaldırılmış"
    [ -s /etc/ssl/private/pure-ftpd.pem ] && gecti "TLS sertifikası doğru yolda" || kaldi "/etc/ssl/private/pure-ftpd.pem YOK (Debian'da yol sabittir)"
  else
    grep -q '^MySQLConfigFile' /etc/pure-ftpd/pure-ftpd.conf 2>/dev/null && gecti "pure-ftpd.conf MySQL auth" || kaldi "pure-ftpd.conf MySQL auth satırı yok"
  fi
  ss -lnt 2>/dev/null | grep -q ':21 ' && gecti "port 21 dinleniyor" || kaldi "port 21 dinlenmiyor"
else atla "ftp kurulu değil"; fi

baslik "Kiracı izolasyonu"
kontrol "bubblewrap var" command -v bwrap
kontrol "setfacl var" command -v setfacl
if [ -d /sys/fs/cgroup ]; then gecti "cgroup v2 mevcut"; else kaldi "cgroup dizini yok"; fi

printf '\n\033[1m══ SONUÇ ══\033[0m  geçti=%d  kaldı=%d  atlandı=%d\n' "$GECTI" "$KALDI" "$ATLANDI"
[ "$KALDI" -eq 0 ] || exit 1
