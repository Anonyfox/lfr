# lfr

**Looking for readers.**

`lfr` runs mail for your domains from one binary on one small server: mailboxes, and newsletters with
their websites. An agent operates it through a CLI, on the server or over ssh; there is no admin UI, no
remote API and no monthly bill beyond the server (and a relay's cents per thousand mails where the
provider blocks port 25).

- **A mail server**: any `name@domain` in seconds, aliases, catch-alls, plus-addressing, forwarding,
  full-text search, replies, and a spam filter that learns from every message you move. DKIM (RSA and
  Ed25519), SPF, DMARC, MTA-STS and TLS reporting for every domain; `lfr domain add` prints exactly the
  DNS records to publish and `lfr doctor` checks them.
- **Newsletters**: lists with double opt-in and stored consent proof, a welcome issue, issues written in
  Markdown with their images, checked before sending, rendered per reader with one-click unsubscribe
  (RFC 8058) and unsubscribe by reply, click and reply counts, bounce and complaint handling that pauses
  a list before it hurts your domains. Each list gets a fast website (issues, archive, feed, signup) on
  its own host with HTTPS.
- **A writing floor**: every issue is held to its newsletter's brief and to rules against the habits of
  generated prose, with an offline estimate of whether AI-text detectors would flag it. The rules and
  the agent skills rest on built-in research (`lfr evidence`): 136 reader and writer voices, 20
  newsletters and 144 standards, studies and benchmarks, quoted verbatim.

Pure Go, SQLite, one process. About 25 MB of memory idle and an 18 MB binary; comfortable on a 512 MB
droplet with dozens of domains.

## Install

```sh
curl -fsSL https://lfr.email/install.sh | sh
```

Linux and macOS, amd64 and arm64. With Go 1.26 or newer: `go install github.com/Anonyfox/lfr/cmd/lfr@latest`.

## 1. A mail server

On the server (a DigitalOcean droplet named `mail.example.com`, for example):

```sh
sudo lfr setup --hostname mail.example.com --acme-email you@example.com --systemd
lfr domain add example.com            # prints every DNS record to publish
lfr domain dns example.com --check    # every record ok?
lfr doctor                            # reverse DNS, blocklists, outbound, certificates, disk, queue
```

`setup` creates `/var/lib/lfr`, a system user and a hardened systemd service allowed to bind ports 25, 80
and 443. On Cloudflare, set the records to "DNS only"; `lfr domain dns example.com --zone` prints them as
a zone file Cloudflare imports.

lfr delivers straight to recipients' mail servers when outbound port 25 is open. Many cloud providers
block it; DigitalOcean blocks ports 25, 465 and 587 on every droplet. Then lfr hands finished,
DKIM-signed messages to a relay on an alternate port for the last hop:

```sh
echo "$SES_SMTP_PASSWORD" | lfr relay smtps://AKIA...@email-smtp.eu-central-1.amazonaws.com:2465 --spf include:amazonses.com
lfr relay       # the route, a login test, and webhook URLs for the relay's bounce notifications
```

Amazon SES costs $0.10 to $0.16 per 1,000 messages; SMTP2GO has a free tier. See
[operations](docs/operations.md) for both.

## 2. Mailboxes

```sh
lfr mailbox add hello@example.com --name "Example"
lfr mailbox alias hi@example.com hello@example.com
lfr mail ls --unread
lfr mail read 42
lfr mail reply 42 --md - <<< "Thanks, Ann. Feed it twice a day."
lfr mail mv 43 spam             # teaches the spam filter
lfr events --follow             # new mail, bounces, signups as they happen
```

## 3. Your agent

```sh
lfr skills install              # setup, mailbox, research, concept, launch, issue, learn
export LFR_REMOTE=you@mail.example.com   # every command, from your laptop, over ssh
```

Every command accepts `--json`; exit codes are 0 (ok), 1 (failed), 2 (wrong usage or invalid input) and
3 (server not running). Human output ends with `Next:` and the step that usually follows.
[SKILL.md](SKILL.md) is the entry point for agents.

## 4. A newsletter

The list lives on the server; the newsletter's editorial work lives in a project folder:

```sh
lfr list add bakers --name "Bakers Weekly" --from news@example.com --host news.example.com
lfr init bakers --list bakers --owner you@example.com
cd bakers && lfr check          # names the next step and its skill, every time
```

`lfr check` walks a newsletter through research (readers' verbatim voices, peer newsletters), the brief
(`concept.md`: promise, cadence, format, length, subjects, voice, guardrails), the launch (`lfr push`
copies the promise, cadence and welcome issue to the list), and then every issue: plan, draft, fix,
test, approve, send, learn. Issues are Markdown files, with images next to them:

```markdown
---
subject: Why your starter smells like nail polish
preheader: It is hungry, not dead, and two feeds fix it
idea: I001
---
# Your starter is hungry

It smells of acetone because it ran out of food. Feed it twice a day.

![The starter after two feeds, doubled in its jar](images/starter-day-2.jpg)
```

```sh
lfr issue put issues/acetone.md                 # create or update by slug, upload its images
lfr issue test acetone you@example.com
lfr issue send acetone --at 2026-10-23T07:00:00+02:00
lfr issue show acetone --learn                  # the learnings.md entry, two days later
```

Readers sign up at `https://news.example.com/` (or any form that posts `email` to `/subscribe`),
confirm by clicking a button, and leave with one click. Published issues live at
`/issues/<slug>`. `lfr site bakers --eject site` copies the website templates into the project for editing and `lfr push` uploads them; `lfr rules`
lists every check with the research behind it.

## Documentation

- [Architecture](docs/architecture.md): packages, data, the request paths.
- [AGENTS.md](AGENTS.md): how to change things: a command, a migration, a rule, a skill.
- [Operations](docs/operations.md): DNS, relays, deliverability, backups, upgrades, troubleshooting.
- [Contributing](CONTRIBUTING.md): building, testing, fuzzing, releasing.
- [The lfr.email Grok Bot](docs/grok-bot.md): the handoff a Grok Bot follows to set up and share it.
- [The lfr.email Grok Bot](docs/grok-bot.md): the handoff a Grok Bot follows to set up and share it.

MIT licensed.
