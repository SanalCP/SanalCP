# Demo paneli (demo.sanalcp.com) betikleri

Bu dizindeki dosyalar **yalnız demo VPS'i** içindir ve bilerek `assets/ops/`
dışında tutulur: `sanalcp-install.sh` ve `sanalcp-update` bunları üretim
sunucularına kopyalamaz.

| Dosya | Kurulum yeri |
|---|---|
| `sanalcp-demo-reset` | `/usr/local/bin/sanalcp-demo-reset` (0755) |
| `sanalcp-demo-reset.service` | `/etc/systemd/system/` |
| `sanalcp-demo-reset.timer` | `/etc/systemd/system/` (`systemctl enable --now sanalcp-demo-reset.timer`) |

Tohum dump'ı `/var/backups/sanalcp/demo/demoseed.sql.gz` elle alınır:

    mysqldump --single-transaction --databases panel | gzip -c > /var/backups/sanalcp/demo/demoseed.sql.gz

Tasarım: `docs/superpowers/specs/2026-08-24-demo-panel-design.md`.
