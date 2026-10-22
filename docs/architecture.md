# Architecture

`lfr serve` is one process: an SMTP server on port 25, HTTP on 80 and HTTPS on 443, a delivery queue, a
scheduler and the control socket the CLI talks to. All state is in one data directory.

```
/var/lib/lfr/
  lfr.db        SQLite: settings, domains, mailboxes, messages (+ FTS5 index), lists, members,
                issues, outbound queue, events, spam model, reports
  msg/ab/…eml   raw messages, immutable, named by SHA-256
  sites/<list>/ template overrides and assets of each list's website
  certs/        Let's Encrypt account and certificates
  lfr.sock      control socket (0660): the only management interface
```

## Packages

| Package | Responsibility |
| --- | --- |
| `cli` | Commands, flags, help, human and JSON output, one file per noun (`server`, `domain`, `mailbox`, `mail`, `delivery`, `list`, `reader`, `issue`, `site`); the newsletter-project front end (`newsletter.go`) reads files on disk. Calls operations; never touches the database. |
| `api` | Names and argument/result types of every control operation, shared by CLI and server. |
| `ctl` | The control protocol: one JSON request per unix-socket connection, plus streams (`watch`); `lfr proxy` carries it over ssh for `--remote`. |
| `engine` | Opens the data directory, wires services, runs listeners and loops, implements operations (`ops.go`), delivery routing and feedback (`deliver.go`), local delivery to its own domains (`local.go`), delivery notices (`dsn.go`), status and doctor (`doctor.go`), DNS record checks (`records.go`) and housekeeping (`housekeep.go`). |
| `store` | SQLite with WAL, one writer and a reader pool, embedded migrations, settings. |
| `config` | Typed settings with defaults, validation and help. |
| `blob` | Content-addressed raw message files, written atomically. |
| `domains` | Domains, DKIM keys (RSA-2048 and Ed25519), the DNS records each domain needs. |
| `mailbox` | Mailboxes, aliases, catch-alls, resolution of recipients, folders, flags, search, send, reply, forward. |
| `msg` | MIME parsing (tolerant of broken mail), composing, header rewriting, HTML to text. |
| `emailmd` | Markdown to inlined email HTML, web HTML and plain text; the email layout. |
| `lists` | Lists, contacts, memberships, double opt-in, consent records, suppression, import/export, health pauses. |
| `issues` | Issues: drafts, front matter, lint, per-recipient rendering with signed links, scheduling, stats. |
| `outbound` | The persistent delivery queue, relay and direct (MX, STARTTLS, MTA-STS) transports, retries, bounces. |
| `inbound` | The SMTP server: connection limits, early-talker detection, recipient checks, authentication, scoring. |
| `mailauth` | SPF, DKIM and DMARC verification with alignment; DKIM signing. |
| `spam` | Rule features, Bayes (Robinson–Fisher), reputation, and logistic weights that learn from moves. |
| `dnsbl` | DNS blocklist lookups that tell listings from refusal codes. |
| `feedback` | Parsers for bounces (RFC 3464), complaints (RFC 5965), DMARC and TLS reports, relay webhooks. |
| `web` | List sites (templates uploaded with `lfr site` or `lfr push`, validated before they go live; their look is explained in [web-design.md](web-design.md)), signup/confirm/unsubscribe, tracking redirects, feeds, MTA-STS policies, webhooks (SES verified by SNS signature). |
| `certs` | Let's Encrypt via autocert with a self-signed fallback and failure backoff. |
| `token` | HMAC-signed, case-insensitive tokens for every link and VERP address. |
| `events` | The event log and its live fan-out to `lfr events --follow`. |
| `finding` | The one shape of every check result: rule, severity, place, message, fix. |
| `editorial` | Editorial rules on an issue's subject, preheader, length, links, images and copy, with a brief; includes `prose`. |
| `prose` | The writing floor: stock phrases and structural habits of generated prose, concreteness, paragraph and sentence shape, readability, repetition of earlier issues. |
| `aitext` | Offline estimate of whether AI-text detectors would flag prose (vendored from lfm), with literal prefilters that keep checks in milliseconds. |
| `research` | Parser and checks for verbatim research files: voices, peers, evidence. |
| `concept` | Parser of a newsletter's brief (`concept.md`) and its machine lines; cadence slots. |
| `project` | Newsletter projects on disk: scaffold, backlog, drafts, learnings, and `Check`, the phase engine. |
| `craft` | lfr's own research on newsletter craft, embedded and citable as `lfr:E012`. |
| `addr`, `dnsx`, `i18n`, `skills` | Address normalization, the DNS interface, reader-facing texts (en, de), embedded agent skills. |
| `dnsx/dnsxtest`, `smtptest` | Test helpers: an in-memory DNS zone and an SMTP server. Never in the binary. |

## How a message travels

**In.** `inbound` accepts the connection if per-IP and global limits allow, waits half a second for
early talkers, checks every `RCPT` against `engine.accept` (mailboxes, aliases, catch-all, VERP and
unsubscribe tokens, report addresses), then on `DATA` refuses executables, applies DMARC `p=reject`,
checks SPF/DKIM/DMARC, PTR, DNSBLs and scores spam. Messages above `spam.reject` are refused in SMTP;
the rest go to `engine.deliver`, which files them (junk above `spam.junk`), processes bounces,
complaints, reports and mailed unsubscribes, and forwards when a mailbox asks for it (never bounces or
automatic mail, never twice through the same mailbox).

**Out.** Every message is an `outbound` row first: an issue recipient, a confirmation, or a mailbox send
(whose raw content is a blob). The queue renders the message at send time (so unsubscribes and
suppressions in the meantime still apply), DKIM-signs it twice, and hands it to the relay or the
recipient's MX; mail for the server's own domains is delivered on the spot. A person's mail and
confirmations go before issue batches. Each row is leased before it is sent, so a crash never sends it
twice. Temporary failures retry with backoff for five days; a refused recipient (and only that) is a
hard bounce and suppresses the address on every list; lists whose complaint or bounce rate passes 0.3%
or 5% pause themselves; an outage of the relay holds everything without using up attempts.

**Links.** Unsubscribe, confirm, click, open and bounce links carry tokens signed with the instance
secret, so they need no lookup to verify and cannot be guessed. Confirming and unsubscribing on the
website take a button press: link scanners follow every link but do not press buttons.

## Tests

Each package tests itself; `engine_test.go` runs the whole server: real SMTP in, the control socket,
HTTP handlers, and a sink transport whose messages are verified with lfr's own DKIM, SPF and DMARC
checks against a fake DNS zone built from `domain add`. `cli_test.go` drives the CLI against a live
server. Parsers of untrusted input have fuzz targets.
