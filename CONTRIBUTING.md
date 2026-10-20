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
