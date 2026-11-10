package cli

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/ctl"
)

// Supply wire responses at the local control boundary: diagnosing a broken
// relay must be deterministic and must never contact an external mail host.
func TestDiagnosticOutputAndExitStatus(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		exit         int
	}{{"healthy", "ok", 0}, {"operator pause", "warn", 0}, {"failure", "fail", ExitFailure}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "lfr.sock")
			listener, err := ctl.Listen(path, 0600)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			srv := &ctl.Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			check := api.Check{Name: "outbound", Status: tc.status, Detail: "relay authentication", Fix: "check relay password"}
			srv.Handle(api.OpDoctor, func(context.Context, json.RawMessage) (any, error) { return []api.Check{check}, nil })
			srv.Handle(api.OpRelayShow, func(context.Context, json.RawMessage) (any, error) {
				return api.RelayInfo{Mode: "relay", URL: "smtp://user@relay.example:2525", Webhooks: map[string]string{"generic": "https://mail.example/hooks/generic"}}, nil
			})
			srv.Handle(api.OpRelayTest, func(context.Context, json.RawMessage) (any, error) { return check, nil })
			srv.Handle(api.OpRelaySet, func(_ context.Context, raw json.RawMessage) (any, error) {
				var a api.RelaySet
				if err := json.Unmarshal(raw, &a); err != nil {
					return nil, err
				}
				if a.Password != "secret" || a.SPF != "include:relay.example" {
					t.Errorf("relay credentials/flags not transmitted: %+v", a)
				}
				return api.RelayInfo{Mode: "relay", URL: a.URL}, nil
			})
			srv.Handle(api.OpReports, func(context.Context, json.RawMessage) (any, error) {
				return []api.Report{
					{Kind: "dmarc", Domain: "example.com", Org: "Mailbox", Pass: 90, Fail: 2, Rows: json.RawMessage(`[{"count":2,"ip":"192.0.2.9","dkim":"fail","spf":"fail"}]`)},
					{Kind: "tlsrpt", Domain: "example.com", Org: "Mailbox", Fail: 3, Rows: json.RawMessage(`[{"failure-details":[{"failed-session-count":3,"receiving-mx-hostname":"mail.example.com","result-type":"certificate-expired"}]}]`)},
				}, nil
			})
			done := make(chan error, 1)
			go func() { done <- srv.Serve(ctx, listener) }()
			defer func() { cancel(); <-done }()
			env := map[string]string{"LFR_DATA": dir}
			for _, cmd := range []string{"doctor", "relay"} {
				for _, machine := range []bool{false, true} {
					args := []string{cmd}
					if machine {
						args = append(args, "--json")
					}
					code, out, errs := run(t, env, "", args...)
					expectedExit, want := tc.exit, "relay authentication"
					if cmd == "relay" && tc.status == "warn" {
						expectedExit = ExitFailure
					}
					if cmd == "relay" && machine {
						expectedExit, want = 0, "relay.example"
					}
					if code != expectedExit || errs != "" || !strings.Contains(out, want) {
						t.Fatalf("%v: %d %s%s", args, code, out, errs)
					}
					if machine && !json.Valid([]byte(out)) {
						t.Fatal(out)
					}
					if !machine && tc.status == "fail" && !strings.Contains(out, "check relay password") {
						t.Fatal("missing actionable fix")
					}
				}
			}
			code, out, errs := run(t, env, "secret\n", "relay", "smtp://user@relay.example:2525", "--spf", "include:relay.example")
			relayExit := tc.exit
			if tc.status == "warn" {
				relayExit = ExitFailure
			}
			if code != relayExit || !strings.Contains(out, "relay.example") {
				t.Fatalf("relay set: %d %s%s", code, out, errs)
			}
			code, out, errs = run(t, env, "", "reports")
			for _, want := range []string{"2 messages from 192.0.2.9", "certificate-expired", "lfr doctor"} {
				if code != 0 || !strings.Contains(out, want) {
					t.Fatalf("reports: %d %s%s; want %q", code, out, errs, want)
				}
			}
		})
	}
}
