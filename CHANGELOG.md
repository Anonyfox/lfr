# Changelog

## 0.2.4 (2026-10-24)

- Security: the release archives stored their files as uid and gid 1001 (the build machine's user),
  and install.sh run as root kept that owner. On a server where uid 1001 is a real user, that user
  could replace the binary the service runs as root. The archives now hold root-owned (0/0) entries,
  and install.sh ignores the archive's owner and writes a binary owned by the installing user, root
  under sudo. Running the new install.sh replaces a binary installed by 0.2.3 or older with a
  root-owned one. CI checks both: archive entries and a root install from an archive owned by 1001.
- install.sh suggests `lfr setup` only when it runs as root, and `--systemd` only where systemd runs.
  A user install points to `LFR_REMOTE` for driving a server over ssh.
- install.sh warns when another lfr of a different version comes first on PATH, with both paths and
  versions.
- lfr creates `sites/` and `certs/` in the data directory on start. A backup leaves out empty folders,
  so a restored backup now gets the same layout as a fresh setup. Restores worked before too: lfr
  creates `sites/` when the first site is uploaded.

## 0.2.3 (2026-10-24)

- Backups need a CLI of the server's version. Since 0.2.2 the server streams the backup to the CLI,
  so a CLI from 0.2.1 or older cannot back up a 0.2.2 server. Such a CLI now gets an error that says
  so and how to upgrade; on 0.2.2 it failed without explaining why. Upgrade the CLI on every machine
  that runs `lfr backup`, including the one that uses `--remote`.
- `lfr backup --help` says the copy lands on the machine that runs the CLI, also with `--remote` or
  `LFR_REMOTE`.
- install.sh prints `sudo systemctl restart lfr` when it upgrades a server that already has the
  systemd unit, instead of the setup hint.
- `lfr doctor` reports a relay webhook that has not received anything as "info: configured, no
  notification yet" instead of "skip".
- docs/operations.md: going back below 0.2.2 is not guarded; those binaries open a newer database
  without a check.
- Release notes come from this file.

## 0.2.2 (2026-10-24)

- `lfr doctor` reports the relay webhook as ok once a notification has arrived, with the provider and
  time. Before, it always said "skip", even when the webhook worked.
- Per-list reader texts: `lfr list text <slug>` shows every text a list sends or shows to readers;
  `lfr list text <slug> <key> <value>`, `--reset` and `--import <file|->` override them. This covers
  formal German, Dutch and any other language. `lfr list add` and `lfr list set --language` warn when a
  language has no texts yet.
- `retention.unsubscribed_days` (default 1095): signup and confirmation IP addresses and the browser
  of unsubscribed members are cleared after that many days. Status, dates and consent text stay.
- `lfr backup <dir>` streams the backup to the CLI, which writes and verifies every file. The
  directory can be any path the caller can write, and `--remote` backups land on the local machine.
  The server's sandbox no longer makes backups outside /var/lib/lfr fail.
- lfr refuses to open a database written by a newer version instead of running on it.
- New table `list_texts` (migration 002). Going back to 0.2.1 works; it ignores the table.
- One install line everywhere: `curl -fsSL https://lfr.email/install.sh | sudo sh` on the server.
