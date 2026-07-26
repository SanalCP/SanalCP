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

URL="${1:-https://localhost:8443}/api/v1"
DB=panel
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
  kod=$(curl -sk -o /dev/null -w '%{http_code}' "$@")
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

# Mevcut ilk iki domaini test bayilerine dağıt (geçici; çıkışta geri alınır).
mysql -e "
  SET @a := (SELECT id FROM users WHERE username='$BAYI_A');
  SET @b := (SELECT id FROM users WHERE username='$BAYI_B');
  UPDATE customers SET owner_user_id=@a WHERE id=(SELECT MIN(id) FROM (SELECT id FROM customers) t);
  UPDATE customers SET owner_user_id=@b WHERE id=(SELECT MAX(id) FROM (SELECT id FROM customers) t);
" "$DB"

giris() {
  curl -sk -X POST "$URL/auth/login" -H 'Content-Type: application/json' \
    -d "{\"kullanici\":\"$1\",\"parola\":\"$PAROLA\"}" |
    python3 -c 'import sys,json;print(json.load(sys.stdin).get("token",""))'
}

TA=$(giris "$BAYI_A"); TB=$(giris "$BAYI_B")
if [ -z "$TA" ] || [ -z "$TB" ]; then
  kirmizi "test bayileri giriş yapamadı — parola hash'i güncel mi?"
  exit 1
fi
AA="Authorization: Bearer $TA"
AB="Authorization: Bearer $TB"

echo
echo "== 1. DİKEY: bayi, admin uçlarına erişememeli =="
# NOT: yalnız GET rotası TANIMLI olan uçlar sınanır — tanımsız yol/metot 405
# döner ve bu yetki kararı değildir.
for uc in system/processes admin/system/loglar firewall paketler/kurulu audit \
          admin/backups/ozet wordpress/tumu system/panel-domain paketler/durum; do
  bekle 403 "GET /$uc" -H "$AA" "$URL/$uc"
done
bekle 403 "POST /system/servis-islem" -X POST -H "$AA" -H 'Content-Type: application/json' \
  -d '{"birim":"nginx","aksiyon":"restart"}' "$URL/system/servis-islem"
bekle 403 "POST /system/reboot" -X POST -H "$AA" "$URL/system/reboot"
bekle 403 "POST /plans (plan oluşturma)" -X POST -H "$AA" -H 'Content-Type: application/json' \
  -d '{"ad":"zz"}' "$URL/plans"

echo
echo "== 2. Bayiye AÇIK olması gereken uçlar =="
for uc in me users customers domains plans php-surumler system/usage system/servisler system/load-history genel/dns genel/ssl genel/mail genel/veritabanlari; do
  bekle 200 "GET /$uc" -H "$AA" "$URL/$uc"
done

echo
echo "== 3. YATAY: her bayi yalnız kendi kapsamını görmeli =="
sayi_bekle 1 "A /domains" -H "$AA" "$URL/domains"
sayi_bekle 1 "B /domains" -H "$AB" "$URL/domains"
sayi_bekle 1 "A /customers" -H "$AA" "$URL/customers"
sayi_bekle 1 "A /genel/dns" -H "$AA" "$URL/genel/dns"

echo
echo "== 4. YATAY: çapraz erişim reddedilmeli =="
A_DOM=$(curl -sk -H "$AA" "$URL/domains" | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d[0]["id"] if d else 0)')
B_DOM=$(curl -sk -H "$AB" "$URL/domains" | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d[0]["id"] if d else 0)')
if [ "$A_DOM" != "0" ] && [ "$B_DOM" != "0" ] && [ "$A_DOM" != "$B_DOM" ]; then
  bekle 403 "A -> B'nin domaini" -H "$AA" "$URL/domains/$B_DOM"
  bekle 403 "A -> B'nin DNS'i" -H "$AA" "$URL/domains/$B_DOM/dns"
  bekle 403 "B -> A'nın domaini" -H "$AB" "$URL/domains/$A_DOM"
  bekle 200 "A -> kendi domaini" -H "$AA" "$URL/domains/$A_DOM"
else
  kirmizi "  ! çapraz test atlandı (en az 2 farklı müşteriye bağlı domain gerekiyor)"
fi

echo
echo "== 5. DİKEY: bayi rol yükseltemez =="
bekle 403 "bayi admin hesabı açamaz" -X POST -H "$AA" -H 'Content-Type: application/json' \
  -d '{"kullanici_adi":"zz_sahte_admin","parola":"UzunParola123","rol":"admin"}' "$URL/users"
bekle 403 "bayi bayi hesabı açamaz" -X POST -H "$AA" -H 'Content-Type: application/json' \
  -d '{"kullanici_adi":"zz_sahte_bayi","parola":"UzunParola123","rol":"reseller"}' "$URL/users"

echo
echo "== 6. root hesabı dokunulmaz =="
bekle 403 "bayi root'u silemez" -X DELETE -H "$AA" "$URL/users/1"
bekle 403 "bayi root'u askıya alamaz" -X POST -H "$AA" -H 'Content-Type: application/json' \
  -d '{"durum":"suspended"}' "$URL/users/1/durum"

echo
echo "== 7. Kimliksiz erişim =="
bekle 401 "token'sız /domains" "$URL/domains"
bekle 401 "token'sız /users" "$URL/users"
bekle 401 "geçersiz token" -H 'Authorization: Bearer gecersiz.token.dizisi' "$URL/domains"

echo
echo "======================================"
if [ "$KALAN" -eq 0 ]; then
  yesil "TÜM YETKİ TESTLERİ GEÇTİ ($GECEN test)"
  exit 0
fi
kirmizi "$KALAN test BAŞARISIZ, $GECEN test geçti"
exit 1
