# Operations

## Server

Any Linux server with a public IPv4 works; 512 MB of memory is plenty. Name it after the mail host
(`mail.example.com`): most providers, DigitalOcean included, derive the reverse DNS (PTR) from the name,
and receiving servers compare it with the name lfr greets them with.

```sh
curl -fsSL https://lfr.email/install.sh | sudo sh
sudo lfr setup --hostname mail.example.com --acme-email you@example.com --systemd
```

Without sudo the script installs to `~/.local/bin`, which is enough on a machine that only drives a
server over ssh. `lfr setup --systemd` needs it in `/usr/local/bin`, because the service cannot see
home directories.

The service runs as user `lfr`, may bind ports 25, 80 and 443, can write only `/var/lib/lfr`, and is
capped at 400 MB. Add the user your agent runs as to the `lfr` group to use the CLI without sudo:
`sudo usermod -aG lfr agent`. Logs: `journalctl -u lfr`.

Open ports 25, 80 and 443 in any firewall. Port 80 is needed for Let's Encrypt and redirects to HTTPS.

Measured on 2026-10-20: an idle server holds about 25 MB of memory (resident set size) with one domain
and one list.

**From a laptop.** The agent and the newsletter files usually live elsewhere. With ssh access as a
member of the `lfr` group, `LFR_REMOTE=user@server lfr <command>` (or `--remote`) runs every command
against the server: lfr starts `lfr proxy` over ssh and reuses the connection for a minute.

## DNS

`lfr domain add example.com` prints every record: A and SPF for the mail host when it lives under the
domain, MX, SPF, two DKIM keys (RSA and Ed25519), DMARC, the MTA-STS policy record and host, TLS
reporting, plus an A record for each list website. Publish them with "DNS only" on Cloudflare: mail
records must not be proxied, and the mail host and `mta-sts.` host need their own certificates. List
websites (`news.example.com`) may be proxied once their certificate exists; set Cloudflare's SSL mode to
"Full (strict)".

- **DMARC** starts at `p=none` with reports to `dmarc-reports@<domain>`, which lfr reads itself
  (`lfr reports`). After a few weeks without failures, change it to `p=quarantine`.
- **DKIM** keys rotate in two steps. `lfr domain rotate <domain>` creates new keys that sign nothing
  yet and prints their records; publish them, and once `lfr domain dns <domain> --check` shows them,
  run `lfr domain rotate <domain>` again to switch. Keep the old records a week, then remove them.
- **MTA-STS** starts in `testing` mode: senders report TLS problems (`lfr reports`) but still deliver.
  After a few weeks of clean reports, `lfr config mta_sts.mode enforce`; the policy id changes with it,
  so update the `_mta-sts` record `lfr domain dns` prints.
- **Several domains** share the mail host: every domain's MX points at it.

Check with `lfr domain dns <domain> --check` and `lfr doctor`.

## Outbound delivery

**Direct** (no relay configured): lfr connects to each recipient's MX on port 25, uses STARTTLS when
offered and requires valid TLS where the domain publishes an enforcing MTA-STS policy. Needs outbound
port 25 and a clean, warmed-up IP with matching reverse DNS. `lfr relay` checks the port.

**Relay**: lfr builds and DKIM-signs every message itself and hands it over by authenticated SMTP. The
signature leaves out the fields relays rewrite (Date and Message-ID at SES, their own Feedback-ID), so
it survives the relay, and DMARC passes through its alignment whatever envelope the relay uses. The
relay's SPF mechanism (`--spf`) only matters when the relay sends with your domain in the envelope (a
custom MAIL FROM domain at SES, a return-path CNAME at SMTP2GO); it does no harm otherwise.

- **Amazon SES** (cheapest at volume): verify each domain in SES (it shows its own DKIM CNAMEs; add
  them), leave the sandbox, create SMTP credentials, then
  `lfr relay smtps://<SMTP user>@email-smtp.<region>.amazonaws.com:2465 --spf include:amazonses.com`.
  For bounces and complaints, create an SNS topic with an HTTPS subscription to the `ses` URL from
  `lfr relay`; lfr checks Amazon's signature on every message, confirms the subscription itself and
  from then on accepts only that topic. To move to another topic, `lfr config relay.sns_topic ""` first.
- **SMTP2GO** (free tier): add the sender domain, create an SMTP user, then
  `lfr relay smtp://<user>@mail.smtp2go.com:2525 --spf include:spf.smtp2go.com`, and add a webhook
  for bounces and spam complaints to the `smtp2go` URL from `lfr relay`. That URL is the only secret
  here, and lfr acts only on notifications about mail it sent to that address. Turn off the relay's own
  click and open tracking and unsubscribe footers; lfr handles those.

Rate: `lfr config relay.rate 14` matches SES's default sending quota per second.

## Deliverability checklist

1. `lfr doctor` shows no failures; reverse DNS matches the hostname.
2. Each list has `--address` (sender identity), and German lists an `--imprint-url`.
3. Start small: send the first issues to engaged readers, grow volume over weeks.
4. Register domains with Google Postmaster Tools and (for direct delivery) the IP with Microsoft SNDS.
5. Watch `lfr issue show` and `lfr events --kind complained --kind bounced`. A list pauses itself at
   0.3% complaints or 5% bounces; find the cause before `lfr list set <list> --resume`.

## Spam filtering

New servers start with rules (authentication, blocklists, header and link anomalies, reputation).
Moving messages teaches the filter: `lfr mail mv <id> spam` for missed spam, `lfr mail mv <id> inbox` for
good mail in junk; replies count as good mail. After 20 of each, Bayes joins in. `lfr mail spam` shows
what it learned.

Spamhaus refuses queries from large public resolvers. If the server's resolver is one of them, register
for a free Spamhaus DQS key and set `lfr config spam.dnsbl <key>.zen.dq.spamhaus.net,bl.spamcop.net`.

## Backups and upgrades

`lfr backup /var/backups/lfr-$(date +%F)` writes a consistent copy (database, messages, site templates)
into an empty directory while the server runs. The server streams it and the CLI writes and verifies
it, so the directory can be any path the caller can write; with `--remote user@host` it lands on your
machine. Restore only a backup that holds BACKUP-COMPLETE (written last): stop the service and copy the
directory back to `/var/lib/lfr` (owned by `lfr`).

Upgrade: install the new binary (same command as installing), then `sudo systemctl restart lfr`.
Upgrade the CLI on every machine that runs `lfr backup` or `--remote` too: a CLI older than 0.2.2
cannot back up a 0.2.2 server and gets an error saying so.
Migrations run on start; there is no downgrade. From 0.2.2 on, lfr refuses a database written by a
newer version. Older binaries have no such guard: 0.2.1 and earlier open a newer database without a
word. That is harmless for 0.2.2's schema 2 (they ignore the list_texts table, so text overrides are
not applied), but it is not guaranteed for later schema changes.

## Troubleshooting

| Symptom | Look at |
| --- | --- |
| Mail is not arriving | `lfr domain dns <d> --check` (MX), port 25 open inbound, `journalctl -u lfr` |
| Messages stay queued | `lfr queue`: the last error says why; `lfr relay` |
| Mail lands in recipients' spam | `lfr doctor`, `lfr reports`, DMARC alignment, content (`lfr issue show`) |
| HTTPS certificate missing | DNS of the host points here, port 80 open, `lfr doctor` (tls lines) |
| A list stopped sending | `lfr list show <list>`: the pause reason |

## Privacy and data retention

Hourly housekeeping clears signup and confirmation IP addresses and the signup browser for
unsubscribed members after `retention.unsubscribed_days` (default 1095 days; configurable from 30 to
36500). The member row, status, dates, unsubscribe method and consent text stay, preserving
suppression and a minimal consent record. Unconfirmed signups are deleted after their confirmation
link expires.

`lfr reader forget` does not remove messages from that address in mailboxes, existing backups or
journald logs.

## Per-list reader texts

English and informal German are built in; other languages fall back to English. Use
`lfr list text <slug>` to see every effective text and its source. Overrides apply to confirmation
mail, signup and unsubscribe pages, consent, footers, archive labels and dates.

For formal German, for example:

```sh
lfr list text news confirm.subject 'Bitte bestätigen Sie Ihr Abonnement von %s'
lfr list text news confirm.subject --reset
lfr list text news --import nl.json
```

An import is a flat JSON object, such as
`{"confirm.subject":"Bevestig uw abonnement op %s","form.submit":"Aanmelden"}`. lfr checks the whole
file before it changes anything; an empty value resets its key and keys the file leaves out stay as
they are. Keep the English `%s` placeholders in order and write a literal percent sign as `%%`. `date`
is a Go layout such as `2-1-2006`; month names in a layout always print in English, so use digits for
other languages.
