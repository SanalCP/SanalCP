<p align="center">
  <a href="https://github.com/sanalcp/sanalcp"><b>🌐 GitHub</b></a> &nbsp;·&nbsp;
  <a href="README.md">Türkçe</a> &nbsp;·&nbsp;
  <a href="README.en.md">English</a>
</p>

# SanalCP

**A free and open source hosting control panel.** Turns a blank **AlmaLinux, Debian or Ubuntu** server into a complete hosting stack with a single command — nginx + MariaDB + multi-version PHP + Valkey (Redis) + email + phpMyAdmin + firewall, all installed and configured automatically.

> **Free forever.** There is no pro edition, no feature gating, no license key, and no
> user/domain limit. The entire panel is **MIT** licensed — commercial use included,
> without restriction. We do not plan to release a paid edition.

## Why another panel?

Free panels (HestiaCP, CloudPanel, CWP, FastPanel) largely solve the hosting problem. What
they don't solve is the core problem of shared hosting: **resource isolation.** When one
customer's site consumes the CPU, the disk or the database, everyone on the server suffers.
The commercial answer to this is CloudLinux (CageFS + LVE + MySQL Governor), and it costs money.

SanalCP builds that layer **using the kernel's own facilities, for free**:

| Layer | How | CloudLinux equivalent |
|---|---|---|
| **CPU / RAM / task limits** | a systemd slice per domain — `CPUQuota`, `MemoryMax`, `TasksMax` | LVE |
| **Disk I/O limits** | absolute read/write MB/s + IOPS throttling inside the slice | LVE (io/iops) |
| **Database limits** | native MariaDB `MAX_USER_CONNECTIONS`, `MAX_QUERIES_PER_HOUR`, `MAX_UPDATES_PER_HOUR` | MySQL Governor |
| **Disk quota** | per-tenant XFS disk + inode quota | — |
| **Filesystem isolation** | a separate Linux user per domain + a separate PHP-FPM pool + `open_basedir` + tenant-owned `session.save_path` / `upload_tmp_dir` | CageFS |

Limits are attached to service plans: change a value on the plan and the panel re-applies it
to the affected tenants. **No kernel module, no patched kernel, no extra license.**

On top of that:

- **Automatic intrusion blocking** is included — authentication attacks against SSH, mail
  and FTP are watched and offending IPs are temporarily blocked in nftables (a fail2ban
  equivalent, with no extra service to install). See below.
- **A full management API** — everything the panel does can be done over HTTP; it is the
  very same API the UI uses, not a restricted subset. A token acts with its owner's role.
  See **[docs/API.md](docs/API.md)**.
- **Runs with SELinux `Enforcing`** — it never asks you to disable it.
- **A full email stack** is included (Postfix + Dovecot + OpenDKIM + Roundcube, automatic DKIM/SPF/DMARC).
- **A single static Go binary** plus a React UI; mobile friendly.
- **Updates are reversible**: a database dump is mandatory before migrations, and if the new
  version doesn't come up healthy, both the binary **and** the database roll back automatically.

> ### ⚠️ This is a 0.x (beta) release — read before installing
>
> SanalCP is a young project (first release: July 2026), built by a two-person team. The code
> is open, tested, and security has been taken seriously — but it does **not yet** have the
> field experience of panels that have been around for years.
>
> - **Recommended:** test/development servers, your own projects, evaluation.
> - **Not recommended:** carrying revenue-generating customer workloads without a backup.
>   Try it on a separate server first.
> - **The installer is destructive:** it assumes a blank server. Do not run it on a
>   machine with services already running on it.
>
> Bug reports and criticism are explicitly welcome: [open an issue](https://github.com/sanalcp/sanalcp/issues).

## What we don't do (yet)

Being honest beats disappointing you later. These do **not** exist today; they are on the roadmap:

| Missing | Status |
|---|---|
| **Node.js / Python application support** | PHP (7.4 – 8.5) and static sites only. |
| **Slave / cluster DNS** | Primary DNS on a single server. `allow-transfer` is disabled (to prevent zone enumeration). |
| **Broad application catalog** | One-click installation supports WordPress, Joomla, PrestaShop, Drupal, OpenCart, Matomo, Grav, Nextcloud, MediaWiki, and phpBB; more PHP applications are planned. |

> **There is no WHMCS module and none is planned** — WHMCS is a paid product and we prefer to
> stay free. Since the panel is MIT licensed and ships a full management API
> ([docs/API.md](docs/API.md)), anyone who needs that integration can write it.

## One-line install

On a clean server (min. 2 GB RAM), as **root**:

```bash
curl -fsSL https://sanalcp.com/kur | bash
```

That URL serves a script **pinned to the released commit and to the archive's
SHA-256**; installation does not start unless the downloaded package verifies.

The one-liner that pulls the `main` branch directly **no longer works without an
integrity check** — `install.sh` refuses to install unless a SHA-256 is supplied.
For a pinned install, obtain the hash from a channel you trust that is
independent of this repository:

```bash
curl -fsSL https://raw.githubusercontent.com/sanalcp/sanalcp/main/install.sh \
  | SANALCP_REF=<commit-sha> SANALCP_SHA256=<hash> bash
```

To deliberately skip verification (trusting GitHub's TLS alone) you must pass
`SANALCP_INSECURE=1`; this is not recommended for production servers.

Installation takes ~5-10 minutes (package downloads). When it finishes, the panel URL and login credentials are printed to the screen.

## After install

- **Panel:** `https://SERVER_IP:8443` (self-signed certificate — click through the browser warning)
- **First login:** the administrator username and password **printed on screen** at the end
  of the install (username defaults to `admin`; override with `--admin-kullanici` /
  `--admin-parola`)

> ⚠️ **That password is shown once and is never written to any file.** Save the box from
> the installer output. If you lose it, see "Recovery" below.

After the first login, enabling **Profile → Two-Factor Authentication (2FA)** is recommended.

## Authentication and account model

The panel has **two separate password worlds**. Understanding this matters for your security.

### 1. Panel accounts — the primary login

You log in with **real panel accounts** from the `users` table. Their passwords are stored
in the panel database using **bcrypt (cost 12)** and have **nothing to do with** the
server's root password.

The installer creates that account for you and prints its credentials **to the screen only**
(never to disk). The username defaults to `admin`; you can change it with
`--admin-kullanici` — except that `root` is rejected, because that name belongs to the
separate path described in 2 below.

Further administrator/reseller accounts are created from **Users → New account**.

### 2. `root` — the break-glass path, **disabled by default**

For the username `root` the panel looks the password up in `/etc/shadow` rather than in the
database (yescrypt natively in Go; a fallback path handles legacy sha512/sha256/md5crypt).
Locked (`!`, `!!`, `*`) or empty-password accounts are **never** accepted.

This path **ships disabled on new installs** — so that panel login does not depend on the
server's root password and the audit log can tell operators apart. On **existing** installs
it stays **enabled** after an update, so nothing changes behind your back; closing it is
your call.

Where to toggle it: **Tools & Settings → "Panel login with the server root password".**

- When you disable it, the panel requires an **active non-root administrator account** to
  exist — you cannot lock yourself out in a single step.
- Disabling also **revokes root's existing sessions and API tokens.**

> 🔵 **Your SSH root access is NOT affected by this setting.** The flag concerns the
> `:8443` panel login only; the server's root password, the `sshd` configuration and SSH
> root login are untouched. As long as you can reach the server, your recovery path stays
> open (see "Recovery").

### Recovery — when you cannot log in

Both paths are fixed over SSH; connect to the server as root:

```bash
# Read the DSN from the env file (it contains a password — don't echo it)
DSN=$(grep -m1 '^PANEL_DB_DSN=' /etc/sanalcp/env | cut -d= -f2-)

# 1) Reset the administrator password (creates the account if missing)
/opt/sanalcp/bin/sanalcp-seed-admin -dsn "$DSN" -kullanici admin -parola 'NEW_PASSWORD'

# 2) Lost your 2FA device — seed-admin does NOT touch 2FA, clear it separately
mysql panel -e "UPDATE users SET totp_enabled=0, totp_secret='' WHERE username='admin';"

# 3) Break-glass: re-enable the panel's root login (when you cannot get in at all)
mysql panel -e "UPDATE panel_ayarlari SET root_girisi_acik=1 WHERE id=1;"
```

> ⚠️ `sanalcp-seed-admin` resets **the password only**. If 2FA is enabled, login still
> stops at the 2FA step; a lost TOTP also needs the second command.

Roles:

| Role | Scope |
|---|---|
| **Administrator (admin)** | The whole server. There can be more than one; the last active admin cannot be demoted. |
| **Reseller** | Only their own customers and their domains. Has its own quotas; can only create customer accounts. |
| **Customer (user)** | A single domain. Cannot log in to the management panel; uses `/cp` for their own domain panel. |

### Login security

- **2FA (TOTP)** — for all panel accounts; set up via QR code, protected against code replay.
  If 2FA state cannot be read, login is **denied** (fail-closed).
- **Brute-force protection** — per-IP sliding window + progressive delay + lockout.
  The client IP cannot be spoofed via reverse-proxy headers.
- **Audit log** — successful/failed logins, account operations and permission changes are recorded.
- **Idle session timeout** — `Tools & Settings → Server Maintenance` (off by default).
- Failed logins return an identical response on both branches so usernames are not leaked.

## What it installs

| Component | Detail |
|---|---|
| **Web** | nginx (panel on :8443 + customer sites on :80/:443) |
| **PHP** | 7.4 / 8.2 / 8.3 / 8.4 / 8.5 (remi) — each domain picks its own version, per-domain FPM pool |
| **Database** | MariaDB 10.11 (`panel` DB) + phpMyAdmin (`/pma/`) |
| **Cache** | Valkey (Redis) — per-tenant isolated object cache (auto-connects to WordPress) |
| **Email** | Postfix + Dovecot + OpenDKIM — SMTP AUTH (587), IMAP, automatic DKIM/SPF/DMARC; webmail (Roundcube, `/webmail/`) |
| **Security** | nftables firewall, SELinux-compatible, ClamAV |
| **Performance** | Automatic MariaDB + nginx + OPcache tuning (`sanalcp-optimize`) |

## Panel features

- Domain / subdomain management, DNS editing, bulk operations
- One-click **WordPress**, **Joomla**, **PrestaShop**, **Drupal**, **OpenCart**, **Matomo**, **Grav**, **Nextcloud**, **MediaWiki**, and **phpBB** installation, version detection, and core updates where supported
- Per-tenant **Redis object cache** (toggle on/off, auto-wires into WordPress)
- **Email hosting**: a mailbox per domain, authenticated SMTP sending (for PHPMailer / application integrations), automatic DKIM/SPF/DMARC DNS records, webmail — see below for details
- **Custom vhost mode** (admin only): full nginx vhost editing per domain, for routing needs the template's single-root model can't express — see below for details
- **Firewall** UI (IP ban / whitelist / port closing + ready-made templates)
- **Automatic intrusion blocking** (fail2ban equivalent) — see below
- **Management API** — personal access tokens for automation ([docs/API.md](docs/API.md))
- Backup manager, monitoring/logs, statistics
- Service plans and resource limits (new domains default to the **Starter** plan)

## Automatic Intrusion Blocking

**Tools & Settings → Firewall → Automatic Intrusion Blocking.**
**It is OFF by default** — nothing changes in your setup unless you turn it on.

The panel watches authentication failures from system services via journald. When an IP
crosses your threshold within your window, it is blocked in nftables for the duration you
choose. No extra service (fail2ban, Python) is installed — the watcher lives inside the
panel's own binary.

| Watched | What is caught |
|---|---|
| **sshd** | wrong password, invalid user, max auth attempts exceeded, preauth disconnects |
| **dovecot** (IMAP/POP3) | failed login attempts |
| **postfix** | SASL (SMTP AUTH) authentication failures |
| **pure-ftpd** | failed FTP logins |

Defaults: **5 failures within 10 minutes → a 60 minute ban.** All three are configurable.

### Lockout protections

A wrong rule can lock you out of your own server, so the following guarantees apply:

- **Allow-listed (whitelist) IPs and CIDRs are never blocked.** Adding your own static IP
  to the allow list is recommended.
- **The server's own addresses, loopback and link-local are never blocked.**
- **Bans expire**, and an expired rule stops taking effect immediately — a ban never becomes
  permanent, even if the panel is down.
- **Open sessions are not dropped**: because `ct state established,related accept` sits at the
  top of the nftables ruleset, a ban only affects **new** connections. Even if your own IP is
  blocked, your currently open SSH session stays up.
- Automatic bans are listed in the firewall screen with an **"Automatic"** badge and their
  expiry time, and can be removed individually or all at once via **"Clear automatic bans"**.

> Counters are kept in memory and reset when the panel restarts. This is deliberate — on
> failure it errs toward under-banning rather than over-banning.

## Email (Mail Hosting)

You can open mailboxes for any domain directly from the panel — a self-hosted email system built on Postfix + Dovecot + OpenDKIM (no dependency on a third-party SMTP provider).

- From the **domain page → Email** tab, first enable mail for the domain (MX/SPF/DKIM/DMARC records are added to DNS automatically), then create mailboxes.
- **SMTP AUTH (587, STARTTLS)** — an authenticated sending endpoint that application libraries like PHPMailer or Nodemailer can connect to directly. It is not an open relay; only your own mailbox credentials can be used to send.
- **DKIM signing is automatic** — outgoing mail is signed the moment a mailbox is created, no extra setup needed.
- **Webmail**: access your mailbox from a browser via `https://SERVER_IP:8443/webmail/` (Roundcube) — log in with your mailbox address and password.
- Abuse-prevention rate limiting (per connection/message) and SASL brute-force protection are included.
- Note: inbound mail (port 25) is blocked at the network level by default on some hosting providers (as an anti-spam measure) — if inbound mail isn't reaching your server, ask your provider to open port 25. Outbound SMTP AUTH (587) is unaffected by this.

## Custom Vhost Mode

The panel's standard settings (security headers, caching, the "extra directives" field) are enough for most sites. But sometimes you need a layout the single-`root` template simply can't express — for example, one application at a domain's root and a different one under a subpath like `/blog`.

**Custom Vhost Mode** (Domain → Hosting & DNS → Apache & nginx → "Custom Vhost Mode", admin only) lets you view and edit the full nginx vhost file for that reason:

- When you open it, you start from the **file that's actually running right now** — not a blank box.
- On save, it's validated with `nginx -t` — an invalid configuration is never applied to the live server; both the database and the running file stay safe.
- **Once you turn it on, the panel never touches that file again** — automatic actions like SSL renewal or PHP version changes will use your saved content instead of the template for that domain. This means if you remove the Let's Encrypt validation block (`/.well-known/acme-challenge/`) from the file, certificate renewal will start failing after 90 days.
- If the domain is suspended, the "suspended" page is always shown, even in custom vhost mode — this safety behavior cannot be bypassed.
- Turning it off does not delete your saved content — turn it back on and you pick up where you left off.

## System requirements

- **AlmaLinux 10** or **AlmaLinux 9.4+** (matching RHEL / Rocky releases also work)
- or **Debian 13 / 12**, **Ubuntu 26.04 LTS / 24.04 LTS** — all four verified on live servers
- At least **2 GB RAM**, 2 vCPUs (for 5 PHP versions + MariaDB + Valkey)
- Root access + an internet connection
- Disk quotas work on both **XFS** and **ext4** root filesystems. On ext4 quotas need
  `rootflags=usrquota` on the kernel command line and **one reboot**; the installer sets
  this up and tells you on screen.

## Post-install helper tools

Installation places these tools in `/usr/local/bin`:

```bash
sanalcp-update        # safely update the panel from GitHub (see below)
sanalcp-optimize      # re-tune MariaDB/nginx/PHP for the server's resources
sanalcp-redis-setup   # install/repair the Valkey (Redis) infrastructure
sanalcp-wp-redis <sk> # connect/disconnect Redis cache for a domain's WordPress
sanalcp-repair        # permission / SELinux / ownership repair (idempotent)
sanalcp-db-backup     # take a compressed dump of the panel DB (see below)
```

## Backups

### Panel database (`panel`)

A **daily automatic backup** is included from install — you don't need to set anything up:

| | |
|---|---|
| **When** | Every day at **03:30** (`sanalcp-db-backup.timer`, ±5 min random jitter) |
| **Where** | `/var/backups/sanalcp/db/panel-<DATE>.sql.gz` (directory `0700`, dump `0600`) |
| **Retention** | **14 days** — older backups are removed automatically |
| **Scope** | the `panel` schema + routines/triggers/events (`mysqldump --single-transaction` → lock-free consistent snapshot) |

To take a manual backup (prints the path of the resulting file):

```bash
sanalcp-db-backup
# /var/backups/sanalcp/db/panel-2026-07-17-143052.sql.gz
```

To check the timer's status / see when it next runs:

```bash
systemctl list-timers sanalcp-db-backup.timer
systemctl status sanalcp-db-backup.timer
journalctl -u sanalcp-db-backup -n 20    # log of recent backups
```

To restore a backup:

```bash
systemctl stop sanalcp
zcat /var/backups/sanalcp/db/panel-2026-07-17-143052.sql.gz | mysql
systemctl start sanalcp
```

> Backups are **fail-closed**: if gzip integrity can't be verified, or the file is suspiciously small, the dump does **not** get named `panel-*.sql.gz` — a half-written dump never looks like a valid backup.

### Automatic pre-update backup

`sanalcp-update` takes a full dump of the panel DB **before applying migrations**. If the dump fails, **the update never starts** (a migration without a backup is refused). See the "Update" section below for details.

### Customer sites

Customer sites + databases are backed up by a separate process: `sanalcp-backup-all` (cron, daily at 03:00 UTC, `/var/backups/sanalcp/<system_user>/`, 14-day retention). The panel DB backup never touches these directories.

## Update (SSH / CLI)

On an installed panel, over SSH as root, a single command:

```bash
sanalcp-update            # pull the latest release from GitHub → swap binary+frontend+migrations → restart
sanalcp-update --dry-run  # show what it would do first (no changes)
sanalcp-update --force    # re-apply even if the binary is unchanged
sanalcp-update --branch X # use a different branch
```

- **Safe and data-preserving:** `/etc/sanalcp/env` (JWT/DB/Redis secrets), the MariaDB `panel` database, and `/home/c_*` customer sites are **never deleted**. Unlike `install.sh`, it does not generate new secrets.
- New migrations are applied **automatically and idempotently** when the service restarts.
- If the binary hasn't changed (SHA matches), nothing happens.
- **A full dump of the panel DB is taken before migrations run** → `/var/backups/sanalcp/db/`.
- **Fail-closed:** if the dump fails, the update **never starts** — the binary, frontend, and migrations are left untouched. A migration without a backup is never accepted.
- If the new version doesn't start up healthy, it **automatically rolls back to the previous binary _and_ the pre-update DB** (no write loss, since the panel is already stopped at that point).

> Deploying your own fork: build from source (`GOAMD64=v1 go build` + `npm run build`), update `assets/sanalcp-server` + `assets/frontend-dist.tar.gz`, push to your repo — `sanalcp-update` on your servers will pull the new release. **Always build the binary with `GOAMD64=v1`** (see the warning under "Backend (Go)" below) — otherwise the panel won't start on older customer-server CPUs.

## Notes

- Installation is **not idempotent** — every run generates new secrets (JWT/DB password). Use `sanalcp-repair` / `sanalcp-optimize` instead of re-running it.
- The panel is served over HTTP/2 + self-signed SSL on :8443; a real domain with Let's Encrypt can be added through the panel itself.

---

## Building from source & development

This project is **fully open source** (MIT). Instead of installing the prebuilt binary, you can build and develop from source yourself — contributions are welcome.

### Requirements

- **Go 1.26+** (backend — the go.mod language version is 1.25; the release binary is built with the 1.26 line, since the 1.24 and 1.25 lines went out of support when 1.27.0 shipped)
- **Node.js 20+** and **npm** (frontend)
- MariaDB/MySQL access to run it (the backend applies migrations + seeds the admin on startup)

### Backend (Go)

> ⚠️ **The release binary must be built with `GOAMD64=v1`.** AlmaLinux 10 (go1.26+) defaults to producing `GOAMD64=v3`; a binary built with v3 will simply **not run** on older/common customer CPUs that lack v3 microarchitecture support (AVX2 etc.), failing with `"This program can only be run on AMD64 processors with v3 microarchitecture support"`. `assets/sanalcp-server` must always be built with `GOAMD64=v1`
> (use `scripts/build-assets.sh` for convenience — it already pins this).

```bash
# build a single static binary (GOAMD64=v1 is REQUIRED for old-CPU compatibility)
CGO_ENABLED=0 GOAMD64=v1 go build -o sanalcp-server ./cmd/server

# run it (with environment variables)
PANEL_JWT_SECRET="$(openssl rand -hex 32)" \
PANEL_DB_DSN="root@unix(/var/lib/mysql/mysql.sock)/panel" \
./sanalcp-server
```

The backend API lives under `/api/v1`; health check at `/healthz`. Panel login uses real accounts from the `users` table; the `root` / `/etc/shadow` path is a break-glass route that can be switched off with a flag (see "Authentication and account model"). In development you can seed the admin account with `scripts/seed_admin.go`:

```bash
go run scripts/seed_admin.go -dsn '<DSN>' -kullanici admin -parola 'YOUR_CHOSEN_PASSWORD'
# or: the PANEL_SEED_PAROLA env var
```

### Frontend (React + Vite + TypeScript)

```bash
cd frontend
npm install
npm run dev        # dev server on :5185 (proxies /api → VITE_API_PROXY)
npm run build      # production build → frontend/dist/
```

Set where the dev server proxies the backend to via `VITE_API_PROXY` (default `http://localhost:8080`):

```bash
VITE_API_PROXY=http://localhost:8080 npm run dev
```

### Repository layout

```
cmd/server/       Go entry point (main)
internal/         Backend packages (domains, wordpress, dns, redis, guvenlikduvari, github, backups, ...)
frontend/src/     React UI (pages/, components/, lib/)
migrations/       SQL schema migrations (applied on startup)
scripts/          Ops helper scripts (optimize, repair, redis-setup, seed_admin, ...)
assets/           Prebuilt release artifacts used by the installer
install.sh        One-line bootstrap (downloads the repo → runs sanalcp-install.sh)
```

> The prebuilt binary and `frontend-dist.tar.gz` under `assets/` exist so the `curl | bash` install works without building from source. When you publish your own changes, update these from the `go build` / `npm run build` output above.

## Contributing & license

- Contributions (issues / PRs) are welcome.
- License: **MIT** — see [LICENSE](LICENSE). You may use, modify, distribute, and use it in your own product.

## Updating

To update the panel to the latest release, on the server:

```bash
sanalcp-update              # install the latest release
sanalcp-update --dry-run    # only show what it would do
sanalcp-update --force      # re-apply even if it's the same version
```

You can also update from inside the panel: **Tools & Settings → Panel Update → "Check for and install updates"**.

The update **preserves** (never touches): `/etc/sanalcp/env` (JWT/DB/Redis secrets), the MariaDB `panel` database + all customer data, and `/home/c_*` sites.

Before applying migrations, the update takes a full dump of the panel DB into `/var/backups/sanalcp/db/`. If the dump fails, the update **never starts** (a migration without a backup is refused). If the new version doesn't come up healthy, it automatically **rolls back to the previous binary + the pre-update DB**.

### If you get "sanalcp-update: command not found"

If you installed your panel before the update tool was added to the distribution, the command won't exist on your server yet. Since the only way to get the tool is the tool itself, this is a one-time chicken-and-egg problem — fix it with:

```bash
curl -fsSL https://raw.githubusercontent.com/sanalcp/sanalcp/main/assets/ops/sanalcp-update \
  -o /usr/local/bin/sanalcp-update && chmod +x /usr/local/bin/sanalcp-update

sanalcp-update
```

You only need to do this **once**: every time `sanalcp-update` runs, it reinstalls all the tools under `assets/ops/` into `/usr/local/bin`, keeping itself up to date too. After this, you can also use the **Panel Update** button inside the panel.

> The in-panel update button **downloads the tool automatically** if it's missing — so clicking it is enough on its own; the command above is only for cases where you can't reach the panel at all.
