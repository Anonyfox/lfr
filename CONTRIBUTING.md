# Contributing

Go 1.26 or newer. No other tools are needed to build or test.

```sh
go build ./cmd/lfr          # the binary
go test -race ./...         # everything, including the end-to-end server tests
go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
```

Run a local server without root on high ports:

```sh
export LFR_DATA=/tmp/lfr-dev
go run ./cmd/lfr setup --hostname mail.example.com --ipv4 203.0.113.7
go run ./cmd/lfr serve --smtp 127.0.0.1:2525 --http 127.0.0.1:8080 --https 127.0.0.1:8443
```

[Architecture](docs/architecture.md) maps the packages; [AGENTS.md](AGENTS.md) lists the rules for
changes. CI runs formatting, vet, staticcheck, race-enabled tests on Linux and macOS, every fuzz target
briefly, cross-compiles and smoke-tests the binary. Tags `vX.Y.Z` publish binaries through GoReleaser.

## Fuzzing

Every parser of untrusted input (MIME, bounces, reports, webhooks, tokens, addresses, Markdown) has a
native Go fuzz target. Seed corpora and minimized crashes under `testdata/fuzz/` run with `go test`.

```sh
for pkg in $(go list ./internal/...); do
  for name in $(go test -list '^Fuzz' "$pkg" | grep '^Fuzz'); do
    go test -run '^$' -fuzz "^$name\$" -fuzztime 10s "$pkg" || exit 1
  done
done
```

A failure writes its input to `testdata/fuzz/FuzzXxx/`: fix the root cause and commit the file.

## Releasing

Update the version where docs name it, then tag from a green `main`:

```sh
git tag v0.1.0 && git push origin v0.1.0
```

The release workflow runs the tests, publishes `lfr_<os>_<arch>.tar.gz` for Linux and macOS (amd64,
arm64) with `checksums.txt`, and checks that the binary reports the tag. `install.sh` downloads the
latest release and verifies it against `checksums.txt`, so the repository must be public for the install
command to work for anyone else. `goreleaser release --snapshot --clean` builds the same archives
locally.

## The website

`site/` is an [lfm](https://lfm.cash) project, deployed to GitHub Pages by the pages workflow on every
change under `site/`. `lfm compile site` builds it, `lfm compile --release site` also checks it in a
browser. `SKILL.md` and `install.sh` are served at the site root from `site/assets/agents/`, together
with `llms.txt`; copy the root files there when they change (a test fails until they match).
