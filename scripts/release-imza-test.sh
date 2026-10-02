#!/usr/bin/env bash
# sanalcp-update'in release imza doğrulama bloğunu izole sınar. Gerçek /etc,
# /opt veya systemd'ye dokunmaz; geçici anahtarlarla imzalanmış sahte release
# üretir ve betikteki bloğu (iki işaret satırı arası) birebir çalıştırır.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d /tmp/sanalcp-imza-test.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT

BLOK="$WORK/blok.sh"
awk '/^# 🔴 TEDARİK ZİNCİRİ: SHA256SUMS indirilen/{p=1} p{print} p&&/^  IMZA_TOFU=1$/{getline; print; exit}' \
  "$ROOT/assets/ops/sanalcp-update" > "$BLOK"
grep -q 'IMZA_TOFU=1' "$BLOK" || { echo "doğrulama bloğu betikte bulunamadı" >&2; exit 1; }

ssh-keygen -q -t ed25519 -N '' -C test -f "$WORK/dogru"
ssh-keygen -q -t ed25519 -N '' -C test -f "$WORK/yabanci"
signers() { printf 'sanalcp-release namespaces="sanalcp-release" %s\n' "$(cut -d' ' -f1,2 "$1.pub")"; }

yeni_release() { # $1 = imzalayan anahtar ("" = imzasız)
  rm -rf "$WORK/rel"; mkdir -p "$WORK/rel/assets"
  printf 'binary\n' > "$WORK/rel/assets/sanalcp-server"
  signers "$WORK/dogru" > "$WORK/rel/assets/release-signers"
  (cd "$WORK/rel" && sha256sum assets/sanalcp-server assets/release-signers > assets/SHA256SUMS)
  [ -z "$1" ] || ssh-keygen -Y sign -q -f "$1" -n sanalcp-release "$WORK/rel/assets/SHA256SUMS"
}

calistir() { # $1 = SIGNERS yolu; çıkış kodu döner
  (
    A="$WORK/rel/assets"; SIGNERS="$1"
    t() { printf '%s' "$1"; }
    ok() { :; }; warn() { :; }
    die() { echo "DIE: $*" >&2; exit 1; }
    # shellcheck source=/dev/null
    . "$BLOK"
  ) >/dev/null 2>&1
}

signers "$WORK/dogru" > "$WORK/kurulu-signers"
hata=0
beklenen() { # $1 = açıklama, $2 = beklenen (0/1), $3 = gerçek
  if [ "$2" = "$3" ]; then echo "✓ $1"; else echo "✗ $1 (beklenen $2, alınan $3)"; hata=1; fi
}

yeni_release "$WORK/dogru";   r=0; calistir "$WORK/kurulu-signers" || r=$?; beklenen "doğru anahtarla imzalı release kabul" 0 "$r"
yeni_release "$WORK/yabanci"; r=0; calistir "$WORK/kurulu-signers" || r=$?; beklenen "yabancı anahtarla imzalı release RED" 1 "$r"
yeni_release "";              r=0; calistir "$WORK/kurulu-signers" || r=$?; beklenen "imzasız release RED" 1 "$r"
yeni_release "$WORK/dogru"; printf 'kurcalandi\n' >> "$WORK/rel/assets/SHA256SUMS"
r=0; calistir "$WORK/kurulu-signers" || r=$?; beklenen "imzadan sonra değiştirilmiş manifest RED" 1 "$r"
# Güven çapası yokken (eski kurulum): arşivdeki anahtarla tutarlı imza kabul, imzasız red.
yeni_release "$WORK/dogru";   r=0; calistir "$WORK/yok" || r=$?; beklenen "ilk kullanım: tutarlı imza kabul" 0 "$r"
yeni_release "";              r=0; calistir "$WORK/yok" || r=$?; beklenen "ilk kullanım: imzasız RED" 1 "$r"
# Saldırgan arşivdeki anahtarı kendi anahtarıyla değiştirirse, kurulu çapa onu reddeder.
yeni_release "$WORK/yabanci"; signers "$WORK/yabanci" > "$WORK/rel/assets/release-signers"
(cd "$WORK/rel" && sha256sum assets/sanalcp-server assets/release-signers > assets/SHA256SUMS)
rm -f "$WORK/rel/assets/SHA256SUMS.sig"; ssh-keygen -Y sign -q -f "$WORK/yabanci" -n sanalcp-release "$WORK/rel/assets/SHA256SUMS"
r=0; calistir "$WORK/kurulu-signers" || r=$?; beklenen "arşivdeki anahtarı değiştiren saldırgan RED" 1 "$r"

exit "$hata"
