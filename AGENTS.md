# Working on lfr

lfr is one Go binary that runs a mail and newsletter server for many small domains and is operated only
through its CLI. Read [docs/architecture.md](docs/architecture.md) before changing code; it maps every
package and the paths a message takes.

## Principles

- **Runs unattended for years on a $6 server.** No background work without a bound, no unbounded
  memory, no data loss on a crash. Prefer a slower simple path over a clever fragile one.
- **One process, one database.** SQLite (WAL, one writer connection, a few readers) holds all state;
  raw messages are immutable files named by their SHA-256. Settings live in the database and change
  through `lfr config set`; there is no config file.
- **Local control only.** The CLI talks to the server over `<data>/lfr.sock`, from elsewhere only through
  ssh (`--remote`, which runs `lfr proxy` on the server). Never add a network management port, admin UI or
  remote API.
- **Agent-first CLI.** Every command takes `--json`, uses the exit codes in `internal/cli`, explains
  errors in one sentence with the fix, and prints `Next:` with the usual following step.
- **Dependencies earn their place.** Current set: emersion go-smtp, go-sasl, go-message, go-msgauth;
  blitiri spf; modernc sqlite; goldmark; golang.org/x/{net,crypto,text}. Ask before adding one.
- **Deliverability and law are features.** Signed tokens for every link, RFC 8058 one-click
  unsubscribe, double opt-in with consent proof, suppression after hard bounces and complaints,
  tracking off by default (TDDDG §25). Do not weaken any of these.

- **Floor in code, ceiling in skills.** What every newsletter must get right is enforced by the server
  and by `lfr check` (rules in `editorial`, `issues`, `concept`, `research`, `project`); what makes one
  excellent lives in the skills under `internal/skills/files`. Both cite lfr's research (`internal/craft`)
  as `lfr:E012`; tests fail when a citation, a command or a flag in a skill does not exist.
- **Research is verbatim.** Files in `internal/craft/research` are quotes from sources opened by a person
  or agent, never written from memory; append batches, never edit entries.

## Layout

`cmd/lfr` wires signals and exit codes. Everything else is in `internal/`, one package per
responsibility; `engine` assembles them and registers the control operations. `cli` has two halves: a
client that renders and calls the server's operations (`api`, `ctl`), and the newsletter-project front
end that reads files on disk (`project`, `concept`, `research`, `editorial`, `craft`) for `init`,
`check`, `push`, `rules` and `evidence`. Dependencies point one way: `cli → api, ctl, project and the
file packages, engine (setup/serve only)`; `engine → everything`; domain packages never import
`engine`, `cli` or `web`.

## Changing things

- **New CLI command**: add the operation name and argument types to `internal/api`, the handler in
  `internal/engine/ops.go`, the command in `internal/cli/commands.go`. `TestEveryCommandHasHelpAndUniquePath`
  checks help output; extend `TestAgainstServer` for behaviour.
- **Schema change**: add `internal/store/migrations/NNN_name.sql`. Never edit a shipped migration.
- **New setting**: add it to `config.Keys` with a default, a check and help text.
- **Reader-facing text**: add English and German to `internal/i18n`.
- **Parsers of untrusted input** (anything from SMTP, HTTP or a relay) need a fuzz target.
- **New check rule**: give it an ID, a summary and help citing the research in the package's `Rules()`;
  `lfr rules` and `TestRuleCitations` pick it up.
- **Skill text**: edit `internal/skills/files/<topic>/SKILL.md`; numbers and file names come from
  `project.Facts()` as `[[.Project.X]]`, never typed by hand. `TestSkillsMatchTheBinary` checks it.
- **The website** is an lfm project in `site/` (`lfm compile`); `.github/workflows/pages.yml` deploys it.

## Verify before you say done

```sh
gofmt -l . && go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
go test -race ./...
```

`internal/engine/engine_test.go` runs the whole server over real SMTP, the control socket and HTTP,
including DKIM, SPF and DMARC verification of lfr's own mail; keep it passing and extend it for any
change to delivery, signup or feedback handling.
