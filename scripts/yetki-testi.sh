#!/usr/bin/env bash
# yetki-testi.sh — çok kullanıcılı yetki modelinin CANLI uçtan uca sınaması.
#
# Birim testler (internal/middleware, internal/users) yetki KARARINI izole
# doğrular; bu betik ise gerçek sunucuda, gerçek token'larla ROTA
# SINIFLANDIRMASINI sınar: hangi ucun hangi role açık olduğu main.go'da
# elle yazılır ve birim testlerin göremediği tek yer orasıdır.
#
# Kullanım (sunucuda, root olarak):
#   scripts/yetki-testi.sh [panel_url]
# Varsayılan panel_url: https://localhost:8443
#
# Betik kendi test hesaplarını oluşturur ve ÇIKARKEN TEMİZLER. Var olan
# hesaplara ve domainlere dokunmaz; yalnız okuma/403 denemesi yapar.
set -uo pipefail

PANEL="${1:-https://localhost:8443}"
URL="$PANEL/api/v1"
# Oturum artık HttpOnly çerezle taşınıyor ve CSRFKoruma, çerez taşıyan
# state-changing isteklerde Origin/Referer ZORUNLU kılıyor (fail-closed).
# Betik tarayıcıyı taklit etmek zorunda: her isteğe panelin kendi origin'i
# eklenir, yoksa POST/PUT/DELETE testleri yetkiden değil CSRF'ten 403 alır ve
# "beklenen 403" sınamaları YANLIŞ SEBEPLE geçmiş görünürdü.
ORIGIN="$PANEL"
DB=panel

# Bayi başına ayrı çerez kavanozu — iki oturum birbirine karışmamalı.
JAR_A=$(mktemp); JAR_B=$(mktemp)
GECEN=0
KALAN=0

# Test hesapları — çakışmayı önlemek için sabit ve belirgin adlar.
BAYI_A=zz_test_bayi_a
BAYI_B=zz_test_bayi_b
PAROLA='ZzTest!Yetki123'

kirmizi() { printf '\033[31m%s\033[0m\n' "$1"; }
yesil()   { printf '\033[32m%s\033[0m\n' "$1"; }

# bekle <beklenen_kod> <açıklama> <curl argümanları...>
bekle() {
  local beklenen="$1" aciklama="$2"; shift 2
  local kod
  kod=$(curl -sk -o /dev/null -w '%{http_code}' -H "Origin: $ORIGIN" "$@")
  if [ "$kod" = "$beklenen" ]; then
    yesil "  ✓ $aciklama ($kod)"
    GECEN=$((GECEN+1))
  else
    kirmizi "  ✗ $aciklama — beklenen $beklenen, gelen $kod"
    KALAN=$((KALAN+1))
  fi
}

# sayi_bekle <beklenen_kayit_sayisi> <açıklama> <curl argümanları...>
sayi_bekle() {
  local beklenen="$1" aciklama="$2"; shift 2
  local n
  n=$(curl -sk "$@" | python3 -c 'import sys,json
try:
    d=json.load(sys.stdin); print(len(d) if isinstance(d,list) else -1)
except Exception:
    print(-1)')
  if [ "$n" = "$beklenen" ]; then
    yesil "  ✓ $aciklama ($n kayıt)"
    GECEN=$((GECEN+1))
  else
    kirmizi "  ✗ $aciklama — beklenen $beklenen kayıt, gelen $n"
    KALAN=$((KALAN+1))
  fi
}

temizle() {
  mysql -e "
    DELETE FROM reseller_limits WHERE user_id IN (SELECT id FROM users WHERE username IN ('$BAYI_A','$BAYI_B'));
    UPDATE customers SET owner_user_id=NULL WHERE owner_user_id IN (SELECT id FROM users WHERE username IN ('$BAYI_A','$BAYI_B'));
    DELETE FROM users WHERE username IN ('$BAYI_A','$BAYI_B');
  " "$DB" 2>/dev/null
  rm -f "$JAR_A" "$JAR_B"
}
trap temizle EXIT

echo "== hazırlık: test bayileri oluşturuluyor =="
temizle  # önceki yarım kalmış çalıştırmadan artık varsa

# Parola hash'i panelin kendi ucundan alınamaz (hesap açmak için önce hesap
# gerekir); bu yüzden root'un mevcut oturumu yerine doğrudan DB'ye yazıp
# panelin bcrypt doğrulamasını sınıyoruz. Hash sabit: '$PAROLA' için üretilmiş.
HASH='$2a$12$dAOUMGgYqB2r/n4aUjiu1uwcPk6ghTfkMFIXE53CV2Uiab/M06mYu'
mysql -e "
  INSERT INTO users(username,email,password_hash,role,full_name,status)
  VALUES('$BAYI_A','a@test.invalid','$HASH','reseller','ZZ Test Bayi A','active'),
        ('$BAYI_B','b@test.invalid','$HASH','reseller','ZZ Test Bayi B','active');
" "$DB" || { kirmizi "test bayileri oluşturulamadı"; exit 1; }

# Test bayilerine birer müşteri ata (geçici; çıkışta geri alınır).
#
# DOMAİNİ OLAN müşteriler seçilir. Eskiden MIN/MAX(customers.id) kullanılıyordu;
# domaini olmayan bir müşteri kaydı (ör. domaini silinmiş artık kayıt) listenin
# başına/sonuna denk geldiğinde yatay izolasyon testleri "0 kayıt" görüp
# yetki hatası sanılan sahte başarısızlıklar üretiyordu.
mysql -e "
  SET @a := (SELECT id FROM users WHERE username='$BAYI_A');
  SET @b := (SELECT id FROM users WHERE username='$BAYI_B');
  SET @ca := (SELECT MIN(customer_id) FROM domains WHERE customer_id IS NOT NULL);
  SET @cb := (SELECT MAX(customer_id) FROM domains WHERE customer_id IS NOT NULL);
  UPDATE customers SET owner_user_id=@a WHERE id=@ca;
  UPDATE customers SET owner_user_id=@b WHERE id=@cb AND @cb <> @ca;
" "$DB"

# Domaine bağlı müşteri sayısı — yatay testlerin ön koşulu.
MUSTERILI_DOMAIN=$(mysql -N -e \
  "SELECT COUNT(DISTINCT customer_id) FROM domains WHERE customer_id IS NOT NULL" "$DB")
if [ "${MUSTERILI_DOMAIN:-0}" -lt 2 ]; then
  kirmizi "UYARI: domaine bağlı yalnız $MUSTERILI_DOMAIN müşteri var —"
  kirmizi "       yatay izolasyon testleri anlamlı çalışmaz (en az 2 gerekir)."
fi

# giris <kullanici> <cerez_kavanozu> — oturum çerezini kavanoza yazar.
# Yanıt gövdesinde token YOKTUR; başarı ölçütü kavanoza çerez düşmesidir.
giris() {
  curl -sk -o /dev/null -c "$2" -X POST "$URL/auth/login" \
    -H 'Content-Type: application/json' -H "Origin: $ORIGIN" \
    -d "{\"kullanici\":\"$1\",\"parola\":\"$PAROLA\"}"
  grep -q 'sanalcp_oturum' "$2" 2>/dev/null
}

if ! giris "$BAYI_A" "$JAR_A" || ! giris "$BAYI_B" "$JAR_B"; then
  kirmizi "test bayileri giriş yapamadı — parola hash'i güncel mi?"
  exit 1
fi
AA="$JAR_A"
AB="$JAR_B"

echo
echo "== 1. DİKEY: bayi, admin uçlarına erişememeli =="
# NOT: yalnız GET rotası TANIMLI olan uçlar sınanır — tanımsız yol/metot 405
# döner ve bu yetki kararı değildir.
for uc in system/processes admin/system/loglar firewall paketler/kurulu audit \
          system/panel-domain paketler/durum; do
  bekle 403 "GET /$uc" -b "$AA" "$URL/$uc"
done
bekle 403 "POST /system/servis-islem" -X POST -b "$AA" -H 'Content-Type: application/json' \
  -d '{"birim":"nginx","aksiyon":"restart"}' "$URL/system/servis-islem"
bekle 403 "POST /system/reboot" -X POST -b "$AA" "$URL/system/reboot"
bekle 403 "POST /plans (plan oluşturma)" -X POST -b "$AA" -H 'Content-Type: application/json' \
  -d '{"ad":"zz"}' "$URL/plans"

echo
echo "== 2. Bayiye AÇIK olması gereken uçlar =="
for uc in me users customers domains plans php-surumler system/usage system/servisler \
          system/load-history genel/dns genel/ssl genel/mail genel/veritabanlari \
          wordpress/tumu admin/backups/ozet; do
  bekle 200 "GET /$uc" -b "$AA" "$URL/$uc"
done

echo
echo "== 2b. Oturum-çerezi TEK KAYNAK olan uçlar (HttpOnly çerez döneminde ="
echo "     Authorization başlığı taşınmaz; eskiyen yeniden-ayrıştırma yolu"
echo "     'oturum yok' döndürürdü. Bunlar, o sınıftaki yedi ucun kapsama"
echo "     alınmasıdır.) =="
# GET /dashboard-duzen — anasayfa açılışında çağrılır; burası 401 dönerse
# istemci kullanıcıyı giriş yapar yapmaz çıkartır ve panele hiç girilemez.
bekle 200 "GET /dashboard-duzen" -b "$AA" "$URL/dashboard-duzen"
# PUT /me — profil alanları. Boş gövde de PUT kabul eder, testin amacı
# kimliğin doğrulanmış context'ten çözülmesi; 200 = geçti, 401 = kimlik
# yine header'dan aranıyor demektir.
bekle 200 "PUT /me (profil)" -X PUT -b "$AA" -H 'Content-Type: application/json' \
  -d '{"ad_soyad":"ZZ Test Bayi A","eposta":"a@test.invalid","tercih_tema":"system","tercih_dil":"tr"}' \
  "$URL/me"
# PUT /dashboard-duzen — düzen kaydet.
bekle 200 "PUT /dashboard-duzen" -X PUT -b "$AA" -H 'Content-Type: application/json' \
  -d '{"duzen":"[]"}' "$URL/dashboard-duzen"
# GET /me/2fa/setup — yeni secret + otpauth URI döner.
bekle 200 "GET /me/2fa/setup" -b "$AA" "$URL/me/2fa/setup"
# POST /me/2fa/enable — geçersiz kod 400. Kimlik doğrulanmış ama gövde reddi:
# 401 yetki kapısından, 400 handler'dan, 200 kabul edilmiş koddan gelir.
bekle 400 "POST /me/2fa/enable (geçersiz kod)" -X POST -b "$AA" \
  -H 'Content-Type: application/json' -d '{"secret":"x","kod":"000000"}' \
  "$URL/me/2fa/enable"
# POST /me/2fa/disable — aynı ilke.
bekle 400 "POST /me/2fa/disable (kod yok)" -X POST -b "$AA" \
  -H 'Content-Type: application/json' -d '{"kod":""}' \
  "$URL/me/2fa/disable"
# POST /me/parola — YANLIŞ mevcut parola ile. 401 "mevcut parola hatalı" =
# kimlik doğrulandı + bcrypt karşılaştırması elendi. 401 BURADAN değil
# auth kapısından geliyorsa kimlik yine cookie'den değil header'dan
# aranıyor demektir — aynı sınıf hata. chpasswd / UPDATE users tetiklenmez.
bekle 401 "POST /me/parola (yanlış mevcut)" -X POST -b "$AA" \
  -H 'Content-Type: application/json' -d '{"mevcut":"yanlis-parola","yeni":"YeniParola123"}' \
  "$URL/me/parola"

echo
echo "== 3. YATAY: her bayi yalnız kendi kapsamını görmeli =="
sayi_bekle 1 "A /domains" -b "$AA" "$URL/domains"
sayi_bekle 1 "B /domains" -b "$AB" "$URL/domains"
sayi_bekle 1 "A /customers" -b "$AA" "$URL/customers"
sayi_bekle 1 "A /genel/dns" -b "$AA" "$URL/genel/dns"

echo
echo "== 4. YATAY: çapraz erişim reddedilmeli =="
A_DOM=$(curl -sk -b "$AA" "$URL/domains" | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d[0]["id"] if d else 0)')
B_DOM=$(curl -sk -b "$AB" "$URL/domains" | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d[0]["id"] if d else 0)')
if [ "$A_DOM" != "0" ] && [ "$B_DOM" != "0" ] && [ "$A_DOM" != "$B_DOM" ]; then
  bekle 403 "A -> B'nin domaini" -b "$AA" "$URL/domains/$B_DOM"
  bekle 403 "A -> B'nin DNS'i" -b "$AA" "$URL/domains/$B_DOM/dns"
  bekle 403 "B -> A'nın domaini" -b "$AB" "$URL/domains/$A_DOM"
  bekle 200 "A -> kendi domaini" -b "$AA" "$URL/domains/$A_DOM"
else
  kirmizi "  ! çapraz test atlandı (en az 2 farklı müşteriye bağlı domain gerekiyor)"
fi

echo
echo "== 5. DİKEY: bayi rol yükseltemez =="
bekle 403 "bayi admin hesabı açamaz" -X POST -b "$AA" -H 'Content-Type: application/json' \
  -d '{"kullanici_adi":"zz_sahte_admin","parola":"UzunParola123","rol":"admin"}' "$URL/users"
bekle 403 "bayi bayi hesabı açamaz" -X POST -b "$AA" -H 'Content-Type: application/json' \
  -d '{"kullanici_adi":"zz_sahte_bayi","parola":"UzunParola123","rol":"reseller"}' "$URL/users"

echo
echo "== 5b. Bayi KENDİ kotasını okuyamaz/değiştiremez =="
A_ID=$(mysql -N -B -e "SELECT id FROM users WHERE username='$BAYI_A'" "$DB")
bekle 403 "bayi kendi limitini okuyamaz" -b "$AA" "$URL/users/$A_ID/limitler"
bekle 403 "bayi kendi limitini yükseltemez" -X PUT -b "$AA" -H 'Content-Type: application/json' \
  -d '{"max_customer":9999,"max_domain":9999}' "$URL/users/$A_ID/limitler"
bekle 403 "bayi başka bayinin limitini göremez" -b "$AA" \
  "$URL/users/$(mysql -N -B -e "SELECT id FROM users WHERE username='$BAYI_B'" "$DB")/limitler"

echo
echo "== 6. root hesabı dokunulmaz =="
bekle 403 "bayi root'u silemez" -X DELETE -b "$AA" "$URL/users/1"
bekle 403 "bayi root'u askıya alamaz" -X POST -b "$AA" -H 'Content-Type: application/json' \
  -d '{"durum":"suspended"}' "$URL/users/1/durum"

echo
echo "== 7. Kimliksiz erişim =="
bekle 401 "token'sız /domains" "$URL/domains"
bekle 401 "token'sız /users" "$URL/users"
bekle 401 "geçersiz token" -H 'Authorization: Bearer gecersiz.token.dizisi' "$URL/domains"
# eklenti bundle: 0.9.2'de auth dışıydı (çerez taşıyıcıya geçince bu açık kapandı).
# token'sız çağrı 401, bayi 404 (kayıtlı/aktif/UI kontrolü handler'da).
bekle 401 "token'sız eklenti bundle" "$URL/eklenti-bundle/ornek-eklenti/app.js"
bekle 404 "bayi mevcut olmayan eklenti" -b "$AA" "$URL/eklenti-bundle/yok-boyle-eklenti/app.js"

echo
echo "======================================"
if [ "$KALAN" -eq 0 ]; then
  yesil "TÜM YETKİ TESTLERİ GEÇTİ ($GECEN test)"
  exit 0
fi
kirmizi "$KALAN test BAŞARISIZ, $GECEN test geçti"
exit 1
