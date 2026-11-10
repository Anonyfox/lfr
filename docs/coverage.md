# Coverage work on `coverage`

Base: `main` at `bd3a3f5`. Statement coverage uses each package's own tests,
including packages without tests; no `-coverpkg` expansion or exclusions.
The total is weighted by statements, not an average of package percentages.
`internal/api` has no executable statements and is omitted from the table.

## Measurement

```sh
go test -count=1 -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

The initial run used `go test -coverprofile=… ./...` (no cached coverage results).
It measured **67.1%**, rather than the estimated 59%. Final coverage is
**80.1%**: 8,194/12,208 → 9,792/12,223 covered statements.

| Package | Before | After |
| --- | ---: | ---: |
| `cmd/lfr` | 0.0% | 0.0% |
| `internal/addr` | 84.6% | 84.6% |
| `internal/aitext` | 90.0% | 90.0% |
| `internal/backup` | 79.1% | 79.1% |
| `internal/blob` | 79.6% | 79.6% |
| `internal/certs` | 68.6% | 68.6% |
| `internal/cli` | 42.2% | 77.3% |
| `internal/concept` | 78.7% | 78.7% |
| `internal/config` | 26.9% | 83.7% |
| `internal/craft` | 76.9% | 76.9% |
| `internal/ctl` | 53.3% | 78.0% |
| `internal/dnsbl` | 88.2% | 88.2% |
| `internal/dnsx` | 0.0% | 0.0% |
| `internal/dnsx/dnsxtest` | 0.0% | 0.0% |
| `internal/domains` | 82.8% | 82.8% |
| `internal/editorial` | 88.2% | 88.2% |
| `internal/emailmd` | 67.2% | 94.5% |
| `internal/engine` | 61.6% | 76.4% |
| `internal/events` | 0.0% | 92.5% |
| `internal/feedback` | 85.6% | 85.6% |
| `internal/finding` | 0.0% | 0.0% |
| `internal/i18n` | 60.4% | 60.4% |
| `internal/inbound` | 84.4% | 84.4% |
| `internal/issues` | 66.7% | 80.8% |
| `internal/lists` | 77.6% | 86.2% |
| `internal/mailauth` | 81.0% | 96.0% |
| `internal/mailbox` | 73.1% | 81.0% |
| `internal/msg` | 79.2% | 84.4% |
| `internal/outbound` | 73.9% | 81.2% |
| `internal/project` | 79.5% | 79.5% |
| `internal/prose` | 82.2% | 82.2% |
| `internal/research` | 80.5% | 80.5% |
| `internal/skills` | 0.0% | 86.6% |
| `internal/smtptest` | 0.0% | 0.0% |
| `internal/spam` | 91.3% | 91.3% |
| `internal/store` | 80.6% | 80.6% |
| `internal/token` | 90.6% | 90.6% |
| `internal/web` | 72.4% | 80.3% |
| **Total** | **67.1%** | **80.1%** |

## Initial targets

The largest uncovered behavior-heavy functions included:

| Function | Initially uncovered statements |
| --- | ---: |
| `engine.register` (control operation handlers) | 207 |
| `cli.runPush` | 45 |
| `skills.Install` | 44 |
| `cli.runReports` | 38 |
| `project.Check` | 37 |
| `cli.runSkillsInstall` | 35 |
| `engine.Doctor` | 33 |
| `web.hook` | 32 |
| `cli.runMailAttachment` | 31 |
| `issues.SetMedia` | 29 |
| `events.Bus.List` | 28 |
| `lists.Service.Update` | 27 |

Tests target control operations, installation preservation, rendered output,
queue transitions, consent and suppression, and invalid input. Existing SMTP,
DKIM, signup, unsubscribe, bounce and complaint integration tests remain in use.

## Added tests

42 test functions; **2,135 added Go test lines**, including five lines changing
an existing doctor's fixture; **71 fixture lines** for a valid newsletter brief.
Table cases and workflow assertions are additional to the function count.
Shared existing server, database, DNS and queue harnesses are reused.

- CLI: real local server workflows for mailbox moves, flags, deletion, attachments,
  CSV suppression, issue rendering/scheduling/canceling, learning output, project
  publishing, site upload, key rotation preparation, and startup advice.
- CLI diagnostics: local control responses verify failure exit codes, actionable
  report output and valid JSON without contacting a relay or public MX.
- Engine: typed operations and rejection paths, incoming DMARC/TLS report
  deduplication and filtering, rotation waiting for DNS, served-host redirects,
  trusted proxy handling, secret redaction, topic pinning, and local delivery notices.
- SMTP: reset and successive transactions cannot leak senders or recipients.
- Authentication: inherited/strict DMARC policy, zero-percent policy, malformed
  policy, null senders, missing DKIM keys and temporary DNS failures.
- Content: media replacement and test-copy invalidation, scoped media URLs,
  archive ordering, welcome-role changes, canceling queued batches, opt-out checks
  at render time, legacy charsets and bounded UTF-8 text.
- State: configuration validation and secret persistence, transactional event
  publication/rollback, bounded slow subscribers, safe skill upgrades, mailbox
  retention, list edits and deletion preserving global suppression.
- Outbound/web: operator retries preserve completed deliveries, dropped rows stay
  canceled, MTA-STS advertisement/fetch/cache behavior, signup and webhook guards,
  uploaded assets and cache headers.

No production testing seam or dependency was added. The five-day retry callback
is invoked directly to test its delivery notice, avoiding time-based waiting.
MTA-STS uses its existing injectable HTTP transport; DNS uses existing fake zones.
SSH tests replace the executable with local scripts.

## Bug fixed

Flag-parser failures previously returned only human text even with `--json`.
They now use the existing structured error path. One focused table-driven
regression checks flags before/after an error, output-looking option values,
and literals after `--`. Other behavior is unchanged.

## Runtime and gates

- Full coverage run wall time: **37.950 s → 11.998 s**.
- Engine package reported time: **16.739 s → 8.693 s**.
- CLI package reported time: **2.517 s → 2.550 s**.
- These are observed runs on a shared machine, including build work, not a
  controlled benchmark. Final coverage uses `-count=1`; final race gate can reuse
  successful package results. The initial race build took 63.275 s and failed
  only on an incorrect new confirmation-budget assertion; the corrected full
  gate passed in 12.931 s.
- An existing engine doctor's fixture probed a public MX. It now diagnoses an
  invalid configured relay offline, removing that external probe and timeout.
  Added tests use fake DNS, in-memory HTTP transports and local sockets only.
- `gofmt -l .`: empty.
- `git diff --check`: passed.
- `go vet ./...`: passed.
- staticcheck **v0.8.2**: passed, using cached modules with network disabled.
  A temporary module file supplied the tool requirement; repository `go.mod` and
  `go.sum` are unchanged.
- `go test ./...`: passed.
- `go test -race ./...`: passed.

Default Go build and module caches were used throughout. No push, tag or release.
