#!/usr/bin/env bash
# SanalCP — one-line install (bootstrap)
#   curl -fsSL https://raw.githubusercontent.com/sanalcp/sanalcp/main/install.sh | bash
#
# This bootstrap downloads the whole repo (installer + prebuilt binary + configs)
# and runs sanalcp-install.sh.
#
# 🔴 SUPPLY-CHAIN: Varsayılan olarak bu akış yalnızca GitHub'ın TLS'ine güvenir —
# bu, GitHub hesabının/deposunun ele geçirilmesine (phishing, sızdırılmış token vb.)
# karşı koruma SAĞLAMAZ; sanalcp-install.sh'nin kontrol ettiği assets/SHA256SUMS de
# aynı tarball'ın İÇİNDE geldiği için kendine-referans bir kontroldür, dış dünyaya
# karşı güvence değildir.
#
# Ciddi bir kurulum için, SHA'yı ve hash'i bu depodan BAĞIMSIZ, güvendiğiniz bir
# kanaldan (proje web sitesi, imzalı duyuru, yayımcıyla doğrudan teyit) alıp verin:
#   curl -fsSL .../install.sh | SANALCP_REF=<commit-sha> SANALCP_SHA256=<hash> bash
# SANALCP_REF verilmezse hareketli "main" dalı indirilir (repoya push olur olmaz
# etkili olur — en zayıf mod). SANALCP_SHA256 verilirse indirilen arşivin bayt
# bütünlüğü doğrulanır, uyuşmazsa kurulum durur.
set -euo pipefail

REPO="sanalcp/sanalcp"
REF="${SANALCP_REF:-main}"
EXPECTED_SHA256="${SANALCP_SHA256:-}"

c_b="\033[1;34m"; c_g="\033[32m"; c_y="\033[33m"; c_r="\033[31m"; c_0="\033[0m"
[ -t 1 ] || { c_b=; c_g=; c_y=; c_r=; c_0=; }

[ "$(id -u)" = 0 ] || { echo -e "${c_r}✗ root required:  curl ... | sudo bash${c_0}"; exit 1; }
command -v curl >/dev/null 2>&1 || { echo -e "${c_r}✗ curl required${c_0}"; exit 1; }
# 🔴 Some minimal cloud VPS templates (seen on a bare AlmaLinux 9.8 image) ship
# without `tar` — everything else needed here (dnf, coreutils' sha256sum) is
# always present on a RHEL-family base install, but tar apparently isn't
# guaranteed. We already require root, so just install it instead of dying.
command -v tar >/dev/null 2>&1 || {
  echo -e "${c_y}! tar not found — installing it${c_0}"
  dnf install -y tar >/dev/null 2>&1
  command -v tar >/dev/null 2>&1 || { echo -e "${c_r}✗ tar required and could not be installed${c_0}"; exit 1; }
}
command -v sha256sum >/dev/null 2>&1 || { echo -e "${c_r}✗ sha256sum required${c_0}"; exit 1; }

if [ "$REF" = "main" ]; then
  echo -e "${c_y}! SANALCP_REF verilmedi — hareketli 'main' dalı indiriliyor (bkz. bu betiğin başındaki not).${c_0}"
fi
if [ -z "$EXPECTED_SHA256" ]; then
  echo -e "${c_y}! SANALCP_SHA256 verilmedi — indirilen arşivin bütünlüğü doğrulanamayacak.${c_0}"
fi

echo -e "${c_b}══ Downloading SanalCP (github.com/$REPO@$REF) ══${c_0}"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
TARBALL="$TMP/sanalcp.tar.gz"
if ! curl -fsSL "https://codeload.github.com/$REPO/tar.gz/$REF" -o "$TARBALL"; then
  echo -e "${c_r}✗ download failed — is the repo public / does ref '$REF' exist?${c_0}"; exit 1
fi

if [ -n "$EXPECTED_SHA256" ]; then
  ACTUAL_SHA256="$(sha256sum "$TARBALL" | cut -d' ' -f1)"
  if [ "$ACTUAL_SHA256" != "$EXPECTED_SHA256" ]; then
    echo -e "${c_r}✗ SHA-256 uyuşmazlığı — beklenen $EXPECTED_SHA256, alınan $ACTUAL_SHA256${c_0}"
    echo -e "${c_r}  İndirilen arşiv beklenenle eşleşmiyor; kurulum GÜVENLİK NEDENİYLE durduruldu.${c_0}"
    exit 1
  fi
  echo -e "${c_g}✓ SHA-256 doğrulandı${c_0}"
fi

tar xz -C "$TMP" -f "$TARBALL"
SRC=$(find "$TMP" -maxdepth 1 -mindepth 1 -type d | head -1)
[ -z "$SRC" ] && { echo -e "${c_r}✗ could not open package${c_0}"; exit 1; }
cd "$SRC" || { echo -e "${c_r}✗ could not open package${c_0}"; exit 1; }
chmod +x sanalcp-install.sh assets/sanalcp-server assets/sanalcp-seed-admin assets/ops/* 2>/dev/null || true

echo -e "${c_g}✓ downloaded — starting installation${c_0}\n"
exec bash sanalcp-install.sh "$@"
