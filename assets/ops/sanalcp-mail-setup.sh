#!/usr/bin/env bash
# sanalcp-mail-setup — Postfix + Dovecot + OpenDKIM sanal posta kutusu altyapısını
# + Roundcube webmail'i kurar/onarır. Idempotent. Kurulumda çalıştırılır; panelin
# "E-posta" özelliği bunu gerektirir.
#
# Postfix/Dovecot panel DB'sini (mail_domains/mailboxes/mail_aliases) CANLI MySQL sorgusuyla
# okur — bu yüzden panelde kutu ekleme/silme/askıya-alma servis restart'sız anında etkilidir.
# Roundcube panelin mailboxes tablosuna DOKUNMAZ — Dovecot IMAP'e karşı doğrudan (SSO'suz,
# kullanıcı kendi e-posta+parolasını girer) kimlik doğrular; kendi adres defteri/tercih
# verisi için ayrı, küçük bir "roundcube" DB'si kullanır.
set -uo pipefail
log(){ printf '  %s\n' "$*"; }

# TMPL: config template'lerinin okunacağı yer. Üretimde /usr/local/bin'e install edilen
# betiğin yanında assets/ YOKTUR — install.sh assets/mail/*'i kalıcı olarak
# /opt/sanalcp/src/mail-templates'e kopyalar (migrations/scripts ile aynı desen).
# Repo checkout'undan doğrudan çalıştırılıyorsa (yerel geliştirme/test) yanındaki assets/'e düşer.
TMPL="/opt/sanalcp/src/mail-templates"
if [ ! -d "$TMPL" ]; then
  HERE="$(cd "$(dirname "$0")" && pwd)"
  for cand in "$HERE/../assets/mail" "$HERE/assets/mail"; do
    [ -d "$cand" ] && TMPL="$cand" && break
  done
fi
[ -d "$TMPL" ] || { log "✗ mail template dizini bulunamadı ($TMPL)"; exit 1; }
ENV=/etc/sanalcp/env

# Aile tespiti + paket adları (installer ile AYNI tablo).
ORTAK=/usr/local/bin/sanalcp-ortak
[ -f "$ORTAK" ] || ORTAK=/opt/sanalcp/src/scripts/sanalcp-ortak.sh
# shellcheck source=/dev/null
. "$ORTAK" 2>/dev/null || { log "✗ sanalcp-ortak bulunamadı"; exit 1; }

echo "════ Postfix + Dovecot + OpenDKIM + Rspamd paketleri ════"
# GERÇEK VPS'TE BULUNDU: postfix-mysql / dovecot-mysql AYRI paketler — temel postfix/dovecot
# paketleri MySQL sorgu-harita desteğini İÇERMİYOR. Bunlar olmadan servisler "active" görünür
# (systemctl başarıyla başlatır) ama her sorguda sessizce başarısız olur: Postfix
# "unsupported dictionary type: mysql" der, Dovecot auth süreci "Unknown database driver
# 'mysql'" ile crash-loop'a girer (auth soketi var ama arkasındaki süreç sürekli ölür) —
# yani TÜM sanal posta kutusu doğrulaması sessizce bozuk kalır, hiçbir hata dışarı sızmaz.
# Debian'da dovecot TEK paket değil: çekirdek + protokol paketleri ayrıdır
# (dovecot-imapd/lmtpd olmadan IMAP ve LMTP teslimatı hiç yoktur) ve Sieve
# desteği "pigeonhole" adıyla değil dovecot-sieve/managesieved olarak gelir.
if debian_mi; then
  MAIL_PKGS="postfix postfix-mysql dovecot-core dovecot-imapd dovecot-lmtpd \
             dovecot-mysql dovecot-sieve dovecot-managesieved opendkim opendkim-tools"
else
  MAIL_PKGS="postfix postfix-mysql dovecot dovecot-mysql dovecot-pigeonhole opendkim"
fi
# shellcheck disable=SC2086
pkg_kur $MAIL_PKGS \
  && log "postfix(+mysql) + dovecot(+mysql/Sieve) + opendkim kuruldu" || { log "kurulum uyarı (bazı paketler zaten olabilir)"; }

# Rspamd: Debian/Ubuntu'da dağıtımın kendi deposunda var (trixie: 3.12.1) —
# üçüncü parti depoya GEREK YOK. RHEL'de yok; resmi rspamd deposu EL 8/9/10
# paketlerini yayınlıyor ve EPEL bazı çalışma bağımlılıklarını sağlıyor.
if ! command -v rspamadm >/dev/null 2>&1 && rhel_mi; then
  pkg_kur epel-release || true
  EL_VERSION="${EL_MAJOR:-9}"
  curl -fsSL -o /etc/yum.repos.d/rspamd.repo \
    "https://rspamd.com/rpm-stable/centos-${EL_VERSION}/rspamd.repo"
fi
pkg_kur rspamd || { log "✗ rspamd kurulamadı"; exit 1; }
# 🔴 rspamd, panelin per-tenant cache Redis/Valkey'iyle AYNI INSTANCE'I paylaşır
# (ikisi de 127.0.0.1:6379) — sanalcp-redis-setup panelin cache sunucusunu bu
# betikten ÖNCE (kurulumda bir adım önce) zaten kurup çalıştırdı. AlmaLinux
# 9.8'de hem redis HEM valkey paketleri AYNI ANDA mevcut: burada AYRICA birini
# kurup restart edersek ikisi aynı 6379 portunu tutmaya çalışır ve ikincisi
# "Address already in use" ile çöker — canlı AlmaLinux 9.8 testinde tam bu
# şekilde bulundu (redis.service "control process exited with error code").
# Bu yüzden burada kurmuyoruz/restart etmiyoruz — hangisi ZATEN ÇALIŞIYORSA onu
# kullanıyoruz; ikisi de çalışmıyorsa (redis-setup hiç koşmamışsa/başarısızsa)
# ancak o zaman kendimiz kuruyoruz.
KV_SERVICE=""
for aday in $(paket_ad cache) $(debian_mi && echo redis-server || echo redis); do
  systemctl is-active --quiet "$aday" && { KV_SERVICE="$aday"; break; }
done
if [ -z "$KV_SERVICE" ]; then
  KV_SERVICE=$(paket_ad cache)
  if ! pkg_kur "$KV_SERVICE"; then
    KV_SERVICE=$(debian_mi && echo redis-server || echo redis)
    pkg_kur "$KV_SERVICE" || { log "✗ Redis/Valkey kurulamadı"; exit 1; }
  fi
  systemctl enable --now "$KV_SERVICE" >/dev/null 2>&1
fi
log "rspamd + ${KV_SERVICE} (paylaşılan instance) kullanılıyor"

echo "════ mailro (salt-okunur) DB parolası ════"
DBPASS=$(grep -oP '^PANEL_MAIL_DB_PASS=\K.*' "$ENV" 2>/dev/null)
if [ -z "$DBPASS" ]; then
  DBPASS=$(openssl rand -hex 24)
  echo "PANEL_MAIL_DB_PASS=${DBPASS}" >> "$ENV"
  log "mailro parolası üretildi ve env'e eklendi"
fi

echo "════ mailro DB kullanıcısı (yalnızca SELECT — Postfix/Dovecot panel DB'sine yazamaz) ════"
mysql -u root <<SQL 2>/dev/null
CREATE USER IF NOT EXISTS 'mailro'@'127.0.0.1' IDENTIFIED BY '${DBPASS}';
ALTER USER 'mailro'@'127.0.0.1' IDENTIFIED BY '${DBPASS}';
GRANT SELECT ON panel.mail_domains TO 'mailro'@'127.0.0.1';
GRANT SELECT ON panel.mailboxes TO 'mailro'@'127.0.0.1';
GRANT SELECT ON panel.mail_aliases TO 'mailro'@'127.0.0.1';
FLUSH PRIVILEGES;
SQL
log "mailro kullanıcı + SELECT izinleri"

echo "════ Postfix: virtual-mailbox MySQL harita dosyaları ════"
for f in mysql-virtual-domains.cf mysql-virtual-mailboxes.cf mysql-virtual-uid.cf mysql-virtual-gid.cf mysql-virtual-aliases.cf; do
  sed "s/__PANEL_MAIL_DB_PASS__/${DBPASS}/" "$TMPL/postfix/${f}.tmpl" > "/etc/postfix/${f}"
  chown root:postfix "/etc/postfix/${f}"
  chmod 640 "/etc/postfix/${f}"
done
log "5 mysql-virtual-*.cf dosyası yazıldı (root:postfix 0640)"

echo "════ Postfix: main.cf / master.cf ════"
if ! grep -q 'sanalcp-mail' /etc/postfix/main.cf; then
  # Alma/RHEL stok main.cf bu dört anahtarı zaten tanımlar. Aynı anahtarları alta
  # eklemek çalışsa da her postconf/postfix çağrısında "overriding earlier entry"
  # üretir; önce stok tanımları kaldırıp tek bir kanonik blok yaz.
  postconf -X inet_interfaces
  postconf -X mydestination
  postconf -X smtpd_tls_cert_file
  postconf -X smtpd_tls_key_file
  cat "$TMPL/postfix/main.cf.append" >> /etc/postfix/main.cf
  log "main.cf'e sanalcp-mail bloğu eklendi"
fi
if ! grep -qE '^submission\s+inet' /etc/postfix/master.cf; then
  cat "$TMPL/postfix/master.cf.append" >> /etc/postfix/master.cf
  log "master.cf'e submission (587) servisi eklendi"
fi

echo "════ Dovecot: SQL auth + drop-in config ════"
sed "s/__PANEL_MAIL_DB_PASS__/${DBPASS}/" "$TMPL/dovecot/dovecot-sql.conf.ext.tmpl" > /etc/dovecot/dovecot-sql.conf.ext
chown root:dovecot /etc/dovecot/dovecot-sql.conf.ext
chmod 640 /etc/dovecot/dovecot-sql.conf.ext
cp "$TMPL/dovecot/10-sanalcp-mail.conf.tmpl" /etc/dovecot/conf.d/10-sanalcp-mail.conf
# Stok PAM passdb'sini kapat: kutular sanaldır (SQL passdb). Açık kalırsa her
# girişte önce PAM denenir, kullanıcı sistemde olmadığı için pam_unix gecikme
# uygular (ölçüldü: ~1.4-2.1 sn/giriş) ve IMAP'ten sistem hesaplarına parola
# denenebilir hâle gelir. Idempotent: yalnız aktif satırı yorumlar.
sed -i 's|^!include auth-system\.conf\.ext[[:space:]]*$|#!include auth-system.conf.ext  # SanalCP: kutular sanal (SQL passdb)|' \
  /etc/dovecot/conf.d/10-auth.conf 2>/dev/null || true
log "dovecot-sql.conf.ext + conf.d/10-sanalcp-mail.conf"

echo "════ OpenDKIM ════"
mkdir -p /etc/opendkim/keys
touch /etc/opendkim/KeyTable /etc/opendkim/SigningTable
if [ ! -f /etc/opendkim/TrustedHosts ]; then
  printf '127.0.0.1\nlocalhost\n' > /etc/opendkim/TrustedHosts
fi
chown -R opendkim:opendkim /etc/opendkim
# GERÇEK VPS'TE DOĞRULANDI: RHEL/AlmaLinux opendkim paketi /etc/opendkim/opendkim.conf
# DEĞİL, /etc/opendkim.conf'u okur (/etc/sysconfig/opendkim: OPTIONS="-x /etc/opendkim.conf").
# Yanlış yola yazmak servisi hiç etkilemeden stok config'i (KeyFile=.../default.private,
# yoksa exit 78/CONFIG) çalıştırmaya devam ettirir — sessiz başarısızlık.
cp "$TMPL/opendkim/opendkim.conf.tmpl" /etc/opendkim.conf
chown root:opendkim /etc/opendkim.conf
# 🔴 Debian: Socket ayarı iki yerden EZİLEBİLİR.
#   1) /etc/systemd/system/opendkim.service.d/override.conf — varsa ExecStart'ı
#      "-p <socket>" ile yeniden tanımlar ve opendkim.conf'taki Socket satırını
#      GEÇERSİZ kılar. (Stok birim yalnız "ExecStart=/usr/sbin/opendkim"dir;
#      bu drop-in'i /lib/opendkim/opendkim.service.generate üretir.)
#   2) /etc/default/opendkim — systemd birimi bunu OKUMAZ (dosyanın kendi notu:
#      "legacy configuration file"), ama yukarıdaki üretici onu KAYNAK alır.
# Postfix smtpd_milters inet:127.0.0.1:8891'i beklediği için, socket unix'e
# kayarsa DKIM imzalama hiçbir hata vermeden hiç çalışmaz.
if debian_mi; then
  ODK_DROPIN=/etc/systemd/system/opendkim.service.d/override.conf
  if [ -f "$ODK_DROPIN" ] && grep -qE '^ExecStart=.*[[:space:]]-p[[:space:]]' "$ODK_DROPIN"; then
    mv "$ODK_DROPIN" "${ODK_DROPIN}.sanal-devredisi"
    systemctl daemon-reload >/dev/null 2>&1
    log "opendkim systemd drop-in'i devre dışı bırakıldı (socket'i eziyordu) → ${ODK_DROPIN}.sanal-devredisi"
  fi
  # Üretici sonradan çalıştırılırsa aynı tuzağı kurmasın.
  if [ -f /etc/default/opendkim ] && grep -qE '^[[:space:]]*SOCKET=' /etc/default/opendkim; then
    sed -i 's/^[[:space:]]*SOCKET=/#SOCKET=/' /etc/default/opendkim
    log "/etc/default/opendkim içindeki SOCKET= yorumlandı (opendkim.conf yetkili)"
  fi
fi
log "/etc/opendkim.conf + KeyTable/SigningTable/TrustedHosts (boş, panel DKIM ürettikçe dolar)"

echo "════ Rspamd + Redis ════"
mkdir -p /etc/rspamd/local.d
for f in redis.conf actions.conf milter_headers.conf; do
  cp "$TMPL/rspamd/$f" "/etc/rspamd/local.d/$f"
  chown root:root "/etc/rspamd/local.d/$f"
  chmod 644 "/etc/rspamd/local.d/$f"
done
touch /etc/rspamd/local.d/settings.conf
chown root:root /etc/rspamd/local.d/settings.conf
chmod 644 /etc/rspamd/local.d/settings.conf
# Var olan kurulumları da düzelt: template bloğu daha önce yalnız OpenDKIM içeriyordu.
postconf -e 'smtpd_milters=inet:127.0.0.1:8891, inet:127.0.0.1:11332'
postconf -e 'non_smtpd_milters=inet:127.0.0.1:8891, inet:127.0.0.1:11332'
postconf -e 'milter_protocol=6'
postconf -e 'milter_default_action=accept'
postconf -e 'smtpd_end_of_data_restrictions=check_policy_service inet:127.0.0.1:10040'
postconf -e 'smtpd_policy_service_default_action=DUNNO'
systemctl enable "$KV_SERVICE" rspamd >/dev/null 2>&1
# 🔴 "$KV_SERVICE" burada RESTART EDİLMEZ: panelin per-tenant cache'iyle
# paylaşılan aynı instance (bkz. yukarıdaki not) — zaten çalışıyor, yeniden
# başlatmak yalnızca panelin cache bağlantılarını gereksiz yere kesintiye
# uğratır. `enable` (boot'ta otomatik başlasın diye) zaten çalışan bir serviste
# no-op'tur ve restart TETİKLEMEZ.
if ! rspamadm configtest >/tmp/rspamd-configtest.log 2>&1; then
  log "✗ Rspamd config geçersiz"; cat /tmp/rspamd-configtest.log; exit 1
fi
systemctl restart rspamd
log "rspamd milter 127.0.0.1:11332 + Redis etkin"

echo "════ Maildir kök dizinleri (mevcut aktif mail_domains için, varsa) ════"
# GÜVENLİK/SIRALAMA: bu betik sanalcp-install.sh'ta panel İLK KEZ başlatıldıktan
# (migration'lar uygulandıktan) SONRA çağrılır — ftp-setup ile birebir aynı sebep: aşağıdaki
# GRANT SELECT ifadeleri mail_domains/mailboxes/mail_aliases tabloları yokken ERROR 1146
# ile patlar (gerçek MariaDB'ye karşı doğrulandı). Elle/farklı sırada çalıştırırsan önce
# panelin en az bir kez ayağa kalkıp migration'ları uygulamış olduğundan emin ol.
mysql -u root -N -e "SELECT sistem_kullanici FROM panel.mail_domains WHERE durum='active'" 2>/dev/null | while read -r sk; do
  [ -n "$sk" ] || continue
  mkdir -p "/home/${sk}/mail"
  chown "${sk}:${sk}" "/home/${sk}/mail" 2>/dev/null
done

if selinux_var; then
echo "════ SELinux ════"
setsebool -P httpd_can_network_connect_db 1 2>/dev/null && log "httpd_can_network_connect_db=1"
if true; then
  log "UYARI: SELinux enforcing — postfix_t/dovecot_t'nin /etc/pki/sanalcp ve /home/*/mail" \
      "okuma/yazmasında AVC red'i olabilir. 'ausearch -m avc -ts recent' ile kontrol et; gerekirse" \
      "'sanalcp-repair --only mail' veya elle semanage/setsebool ile düzelt."
fi
fi

echo "════ postfix + dovecot + opendkim + rspamd enable + (re)start ════"
systemctl enable postfix dovecot opendkim rspamd "$KV_SERVICE" >/dev/null 2>&1
if ! postfix check >/tmp/mail-postfix-check.log 2>&1; then
  log "✗ postfix check başarısız — /tmp/mail-postfix-check.log"; cat /tmp/mail-postfix-check.log
  exit 1
fi
systemctl restart opendkim; sleep 1
systemctl restart dovecot; sleep 1
systemctl restart postfix; sleep 2

OK=1
systemctl is-active --quiet postfix  || { log "✗ postfix başlatılamadı — journalctl -u postfix"; OK=0; }
systemctl is-active --quiet dovecot  || { log "✗ dovecot başlatılamadı — journalctl -u dovecot"; OK=0; }
systemctl is-active --quiet opendkim || { log "✗ opendkim başlatılamadı — journalctl -u opendkim"; OK=0; }
systemctl is-active --quiet rspamd   || { log "✗ rspamd başlatılamadı — journalctl -u rspamd"; OK=0; }
systemctl is-active --quiet "$KV_SERVICE" || { log "✗ ${KV_SERVICE} başlatılamadı"; OK=0; }
if [ "$OK" = 1 ]; then
  log "✓ postfix + dovecot + opendkim + rspamd + ${KV_SERVICE} ACTIVE"
else
  exit 1
fi

echo "════ Roundcube webmail (/webmail/) ════"
# GERÇEK VPS'TE BULUNDU: php-intl olmadan Roundcube 1.7 giriş anında "Undefined constant
# INTL_IDNA_VARIANT_UTS46" ile 500 verir (IDN alan adı dönüşümü intl eklentisini zorunlu
# kılıyor). Bu, phpMyAdmin'in kullandığı TEMEL sistem PHP'sine ait, remi'nin per-domain
# PHP paketlerinden BAĞIMSIZ.
# Sistem PHP'si: RHEL'de "php-intl", Debian'da sürüm adlı "php8.3-intl".
pkg_kur "$(debian_mi && echo php8.3-intl || echo php-intl)" >/dev/null 2>&1
RCVER=1.7.2
mkdir -p /opt/roundcube
if [ ! -f /opt/roundcube/index.php ]; then
  RCTMP=$(mktemp -d)
  if curl -fsSL -o "$RCTMP/roundcube.tar.gz" \
       "https://github.com/roundcube/roundcubemail/releases/download/${RCVER}/roundcubemail-${RCVER}-complete.tar.gz" \
     && tar xzf "$RCTMP/roundcube.tar.gz" -C /opt/roundcube --strip-components=1; then
    log "roundcube ${RCVER} indirildi + açıldı"
  else
    log "✗ roundcube indirilemedi (ağ?) — webmail atlanıyor, Postfix/Dovecot/SMTP ETKİLENMEZ"
  fi
  rm -rf "$RCTMP"
fi

if [ -f /opt/roundcube/index.php ]; then
  RCDBPASS=$(grep -oP '^PANEL_ROUNDCUBE_DB_PASS=\K.*' "$ENV" 2>/dev/null)
  if [ -z "$RCDBPASS" ]; then
    RCDBPASS=$(openssl rand -hex 24)
    echo "PANEL_ROUNDCUBE_DB_PASS=${RCDBPASS}" >> "$ENV"
  fi
  mysql -u root <<SQL 2>/dev/null
CREATE DATABASE IF NOT EXISTS roundcube CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'roundcube'@'localhost' IDENTIFIED BY '${RCDBPASS}';
ALTER USER 'roundcube'@'localhost' IDENTIFIED BY '${RCDBPASS}';
GRANT ALL PRIVILEGES ON roundcube.* TO 'roundcube'@'localhost';
FLUSH PRIVILEGES;
SQL
  # Şema yalnızca DB boşsa uygulanır (initial.sql CREATE TABLE'da IF NOT EXISTS yok — tekrar
  # çalıştırmak hataya düşer, o yüzden idempotency'yi burada "tablo var mı" kontrolüyle sağlıyoruz).
  RCTBL=$(mysql -u root -N -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='roundcube'" 2>/dev/null)
  if [ "${RCTBL:-0}" = "0" ] && [ -f /opt/roundcube/SQL/mysql.initial.sql ]; then
    mysql -u root roundcube < /opt/roundcube/SQL/mysql.initial.sql 2>/dev/null && log "roundcube DB şeması uygulandı"
  fi

  DESKEY=$(grep -oP '^PANEL_ROUNDCUBE_DES_KEY=\K.*' "$ENV" 2>/dev/null)
  if [ -z "$DESKEY" ]; then
    DESKEY=$(openssl rand -hex 16)
    echo "PANEL_ROUNDCUBE_DES_KEY=${DESKEY}" >> "$ENV"
  fi
  mkdir -p /opt/roundcube/config
  sed -e "s/DB_PASS_BURAYA/${RCDBPASS}/" -e "s/DES_KEY_BURAYA/${DESKEY}/" \
    "$TMPL/roundcube/config.inc.php.tmpl" > /opt/roundcube/config/config.inc.php
  chown root:apache /opt/roundcube/config/config.inc.php
  chmod 640 /opt/roundcube/config/config.inc.php

  mkdir -p /var/lib/roundcube/sessions /var/lib/roundcube/temp
  chown -R apache:apache /opt/roundcube /var/lib/roundcube
  restorecon -R /opt/roundcube /var/lib/roundcube >/dev/null 2>&1
  # php-fpm pool'u (assets/php-fpm/roundcube.conf) install.sh'ın "ARTIFACT DEPLOY" adımında
  # zaten $SYS_PHP_POOL_DIR/roundcube.conf'a kopyalanmış olmalı (phpmyadmin.conf ile aynı
  # desen). Birim adı da aileye göre değişir: php-fpm ↔ php8.3-fpm.
  systemctl reload "$SYS_PHP_SVC" >/dev/null 2>&1 || systemctl restart "$SYS_PHP_SVC" >/dev/null 2>&1
  log "✓ roundcube yapılandırıldı — https://<sunucu>:8443/webmail/"
fi

echo "════════ ✓ Mail altyapısı hazır ════════"
