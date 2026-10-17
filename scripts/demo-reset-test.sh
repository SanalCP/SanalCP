#!/usr/bin/env bash
# scripts/demo/sanalcp-demo-reset için dış servislere dokunmayan sözleşme testi.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d /tmp/sanalcp-demo-reset.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT
MOCKBIN="$WORK/bin"; LOG="$WORK/cagrilar.log"
mkdir -p "$MOCKBIN"

printf 'CREATE DATABASE IF NOT EXISTS `panel`;\nUSE `panel`;\nCREATE TABLE `users` (`id` bigint);\n' |
  gzip -c > "$WORK/demoseed.sql.gz"

for cmd in systemctl journalctl; do
  printf '#!/usr/bin/env bash\necho "%s $*" >> "$MOCK_LOG"\n' "$cmd" > "$MOCKBIN/$cmd"
done
# mysql: demo bayrağı sorgusuna MOCK_DEMO döner; diğer çağrıların argümanını ve
# stdin'deki SQL'in ilk satırını kaydeder (geri yükleme stdin'den gelir).
cat > "$MOCKBIN/mysql" <<'EOF'
#!/usr/bin/env bash
if [[ "$*" == *demo_modu_acik* ]]; then echo "$MOCK_DEMO"; exit 0; fi
if [[ "$*" == *-e* ]]; then echo "mysql-e $*" >> "$MOCK_LOG"; exit 0; fi
echo "mysql-stdin $(head -1)" >> "$MOCK_LOG"; cat >/dev/null
EOF
chmod +x "$MOCKBIN"/*

calistir() { # $1=demo bayrağı, $2=dump yolu
  : > "$LOG"
  PATH="$MOCKBIN:$PATH" MOCK_LOG="$LOG" MOCK_DEMO="$1" SANAL_DEMO_DUMP="$2" \
    bash "$ROOT/scripts/demo/sanalcp-demo-reset" >/dev/null 2>&1
}
satir() { grep -n -m1 -- "$1" "$LOG" | cut -d: -f1 || true; }
hata() { echo "✗ $*" >&2; echo "--- çağrılar:" >&2; cat "$LOG" >&2; exit 1; }

# 1) Tohum eski şemadan alınmış olabilir. Dump'ta olmayan (sonradan eklenmiş)
#    tablolar yeni kolonlarıyla kalırsa schema_migrations eski haline döndüğü için
#    migration'lar yeniden çalışır ve "Duplicate column" ile panel crash döngüsüne
#    girer. Geri yüklemeden ÖNCE panel veritabanı tamamen silinip yeniden kurulmalı.
calistir 1 "$WORK/demoseed.sql.gz" || hata "demo modunda reset başarısız"
DUR=$(satir "systemctl stop sanalcp"); DROP=$(satir "DROP DATABASE"); YUKLE=$(satir "mysql-stdin")
BASLA=$(satir "systemctl start sanalcp")
[ -n "$DUR" ] && [ -n "$DROP" ] && [ -n "$YUKLE" ] && [ -n "$BASLA" ] ||
  hata "beklenen adım eksik (stop=$DUR drop=$DROP yükle=$YUKLE start=$BASLA)"
[ "$DUR" -lt "$DROP" ] && [ "$DROP" -lt "$YUKLE" ] && [ "$YUKLE" -lt "$BASLA" ] ||
  hata "sıra yanlış: stop → DROP/CREATE → geri yükleme → start olmalı"
grep -q "CREATE DATABASE panel CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci" "$LOG" ||
  hata "panel veritabanı kurulumdaki karakter setiyle yeniden oluşturulmadı"

# 2) Bozuk dump: veritabanı silindikten sonra geri yükleme patlarsa panel boş
#    kalır. Dump panel durdurulmadan doğrulanmalı.
echo "bozuk" > "$WORK/bozuk.sql.gz"
if calistir 1 "$WORK/bozuk.sql.gz"; then hata "bozuk dump ile reset başarılı döndü"; fi
grep -qE "systemctl stop|DROP DATABASE" "$LOG" && hata "bozuk dump'ta panel durduruldu / DB silindi"

# 3) Demo modu kapalı (gerçek panel): hiçbir şeye dokunulmamalı.
if calistir 0 "$WORK/demoseed.sql.gz"; then hata "demo modu kapalıyken reset başarılı döndü"; fi
[ -s "$LOG" ] && hata "demo modu kapalıyken komut çalıştırıldı"

echo "✓ demo-reset sözleşme testi geçti"
