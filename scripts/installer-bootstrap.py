#!/usr/bin/env python3
"""Sürüme sabitli /kur dosyasını üretir; sunucuda yalnız bu dosyayı yayımlar."""
import argparse
import datetime
import errno
import json
import os
from pathlib import Path
import re
import stat
import sys
import uuid

MAX_BYTES = 4096


def render(tag, ref, sha256, day=None):
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", tag):
        raise ValueError("Geçersiz sürüm etiketi")
    if not re.fullmatch(r"[a-f0-9]{40}", ref):
        raise ValueError("Geçersiz commit SHA")
    if not re.fullmatch(r"[a-f0-9]{64}", sha256):
        raise ValueError("Geçersiz arşiv SHA-256")
    day = day or datetime.datetime.now(datetime.timezone.utc).date().isoformat()
    datetime.date.fromisoformat(day)
    return f'''#!/usr/bin/env bash
# SanalCP — sanalcp.com/kur
# OTOMATİK ÜRETİLİR — elle düzenlemeyin. Kaynak: .github/workflows/release.yml
# Sürüm: {tag}  ·  Üretim: {day}
set -euo pipefail

SANALCP_REF={ref}
SANALCP_SHA256={sha256}
export SANALCP_REF SANALCP_SHA256

curl -fsSL "https://raw.githubusercontent.com/SanalCP/SanalCP/${{SANALCP_REF}}/install.sh" | bash -s -- "$@"
'''.encode()


def validate(data):
    if len(data) > MAX_BYTES:
        raise ValueError("Kurulum betiği boyut sınırını aşıyor")
    text = data.decode('utf-8')
    version = re.search(r'^# Sürüm: (v[0-9.]+)  ·  Üretim: ([0-9-]+)$', text, re.M)
    ref = re.search(r'^SANALCP_REF=([a-f0-9]{40})$', text, re.M)
    sha = re.search(r'^SANALCP_SHA256=([a-f0-9]{64})$', text, re.M)
    if not (version and ref and sha):
        raise ValueError("Sürüm, commit veya SHA-256 bilgisi eksik")
    if data != render(version[1], ref[1], sha[1], version[2]):
        raise ValueError("Kurulum betiği izin verilen şablonla eşleşmiyor")


def publish(data, target, backup_dir):
    validate(data)
    target = Path(target)
    backup_dir = Path(backup_dir)
    # Dizin bir symlink olamaz; tüm dosya işlemleri açılan dizine bağlıdır.
    directory = os.open(target.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    temporary = '.kur.' + uuid.uuid4().hex
    created = False
    try:
        original = os.open(target.name, os.O_RDONLY | os.O_NOFOLLOW, dir_fd=directory)
        with os.fdopen(original, 'rb') as old:
            metadata = os.fstat(old.fileno())
            if not stat.S_ISREG(metadata.st_mode):
                raise ValueError("Hedef normal bir dosya olmalı")
            backup_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
            backup = backup_dir / ('kur.' + datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '.' + uuid.uuid4().hex)
            backup_fd = os.open(backup, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(backup_fd, 'wb') as saved:
                saved.write(old.read())
            output = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600, dir_fd=directory)
            created = True
            with os.fdopen(output, 'wb') as new:
                new.write(data)
                new.flush()
                os.fchown(new.fileno(), metadata.st_uid, metadata.st_gid)
                os.fchmod(new.fileno(), stat.S_IMODE(metadata.st_mode))
                # Nginx'in site dosyasını okuyabilmesi için mevcut ACL korunur.
                try:
                    acl = os.getxattr(old.fileno(), 'system.posix_acl_access')
                except OSError as error:
                    if error.errno not in (errno.ENODATA, errno.ENOTSUP):
                        raise
                else:
                    os.setxattr(new.fileno(), 'system.posix_acl_access', acl)
                os.fsync(new.fileno())
            os.replace(temporary, target.name, src_dir_fd=directory, dst_dir_fd=directory)
            created = False
            os.fsync(directory)
    finally:
        if created:
            os.unlink(temporary, dir_fd=directory)
        os.close(directory)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    generate = commands.add_parser('generate')
    for name in ('tag', 'ref', 'sha256'):
        generate.add_argument('--' + name, required=True)
    commands.add_parser('publish')
    args = parser.parse_args()
    if args.command == 'generate':
        sys.stdout.buffer.write(render(args.tag, args.ref, args.sha256))
    else:
        # Yol ve yedek dizini SSH istemcisinden alınmaz; root'a ait yapılandırmadır.
        with open('/etc/sanalcp-installer-deploy.json') as config_file:
            config = json.load(config_file)
        publish(sys.stdin.buffer.read(MAX_BYTES + 1), config['target'], config['backup_dir'])
        print('✓ kur doğrulandı ve atomik olarak yayımlandı')


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError) as error:
        sys.exit(str(error))
