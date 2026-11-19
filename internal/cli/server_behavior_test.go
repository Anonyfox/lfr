package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/config"
	"github.com/Anonyfox/lfr/internal/ctl"
	"github.com/Anonyfox/lfr/internal/events"
)

func cliOutput(t *testing.T, env map[string]string, args ...string) string {
	t.Helper()
	code, out, errs := run(t, env, "", args...)
	if code != 0 {
		t.Fatalf("%v exited %d: %s%s", args, code, out, errs)
	}
	return out
}

func decodeOutput[T any](t *testing.T, out string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("invalid output %q: %v", out, err)
	}
	return v
}

func requireText(t *testing.T, out string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(out, p) {
			t.Fatalf("missing %q in %q", p, out)
		}
	}
}

func TestServeSetupDiagnostics(t *testing.T) {
	dir := socketDir(t)
	env := map[string]string{"LFR_DATA": dir, "LFR_LOG": "debug"}
	code, _, errs := run(t, env, "", "serve")
	if code != ExitFailure {
		t.Fatalf("serve before setup: %d %s", code, errs)
	}
	requireText(t, errs, dir, "run lfr setup first")

	code, _, errs = run(t, env, "", "setup", "--ipv4", "203.0.113.7")
	if code != ExitUsage {
		t.Fatalf("setup without hostname: %d %s", code, errs)
	}
	requireText(t, errs, "--hostname is required")
	code, _, errs = run(t, env, "", "serve", "--smtp", "off", "--http", "off", "--https", "off", "--https-proxy-protocol=false")
	if code != ExitFailure {
		t.Fatalf("serve without hostname: %d %s", code, errs)
	}
	requireText(t, errs, "no hostname configured", "lfr config hostname")

	out := cliOutput(t, env, "setup", "--hostname", "mail.example.com")
	requireText(t, out, "Updated "+dir, "203.0.113.7", "Next: start the server")
}

func TestSystemdSetupReportsPrerequisite(t *testing.T) {
	// Even in root CI, no host utilities may run or unit file be installed.
	t.Setenv("PATH", t.TempDir())
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	dir := socketDir(t)
	code, _, errs := run(t, map[string]string{"LFR_DATA": dir}, "", "setup", "--hostname", "mail.example.com", "--ipv4", "203.0.113.7", "--systemd", "--user", u.Username)
	if code != ExitFailure {
		t.Fatalf("systemd setup: %d %s", code, errs)
	}
	if os.Geteuid() == 0 {
		requireText(t, errs, "chown", "executable file not found")
	} else {
		requireText(t, errs, "sudo lfr setup --systemd")
	}
}

func TestConfigChangeAndRestore(t *testing.T) {
	_, env := serve(t)
	get := func() config.Entry {
		t.Helper()
		return decodeOutput[config.Entry](t, cliOutput(t, env, "config", "relay.rate", "--json"))
	}
	initial := get()
	if initial.Name != "relay.rate" || initial.Value != "10" || !strings.Contains(initial.Help, "per second") {
		t.Fatalf("initial setting: %+v", initial)
	}
	out := cliOutput(t, env, "config")
	requireText(t, out, "SETTING", "VALUE", "10 (default)", "Next: lfr help config")
	change := decodeOutput[map[string]string](t, cliOutput(t, env, "config", "relay.rate", "25", "--json"))
	if change["name"] != "relay.rate" || change["value"] != "25" {
		t.Fatal(change)
	}
	if e := get(); e.Value != "25" {
		t.Fatalf("after change: %+v", e)
	}
	code, _, errs := run(t, env, "", "config", "relay.rate", "-1")
	if code != ExitUsage {
		t.Fatalf("negative rate: %d %s", code, errs)
	}
	if e := get(); e.Value != "25" {
		t.Fatalf("rejected change persisted: %+v", e)
	}
	if out := cliOutput(t, env, "config", "relay.rate", ""); !strings.Contains(out, "relay.rate =") {
		t.Fatal(out)
	}
	if restored := get(); restored != initial {
		t.Fatalf("restored setting: %+v", restored)
	}
}

func TestDomainOperatorOutput(t *testing.T) {
	_, env := serve(t)
	requireText(t, cliOutput(t, env, "domain", "ls"), "CATCH-ALL", "Next: lfr domain add example.com")
	cliOutput(t, env, "domain", "add", "example.com")
	cliOutput(t, env, "domain", "set", "example.com", "--catchall", "postmaster@example.com")
	out := cliOutput(t, env, "domain", "ls")
	requireText(t, out, "DKIM SELECTOR", "example.com", "postmaster")
	if strings.Contains(out, "Next: lfr domain add") {
		t.Fatal(out)
	}
	zone := cliOutput(t, env, "domain", "dns", "example.com", "--zone")
	requireText(t, zone, "MX", "mail.example.com", "_dmarc", "_domainkey")
	code, out, errs := run(t, env, "", "domain", "dns", "example.com", "--check")
	if code != ExitFailure || errs != "" {
		t.Fatalf("unpublished DNS: %d %s%s", code, out, errs)
	}
	requireText(t, out, "fail", "want:", "Next: publish the", "then check again")
	requireText(t, cliOutput(t, env, "domain", "set", "example.com", "--catchall", "off"), "Unknown addresses of example.com are refused again")
	ds := decodeOutput[[]api.DomainInfo](t, cliOutput(t, env, "domain", "ls", "--json"))
	if len(ds) != 1 || ds[0].Name != "example.com" || ds[0].Catchall != "" {
		t.Fatalf("catch-all after disabling: %+v", ds)
	}
}

func TestReaderExportPreservesConsent(t *testing.T) {
	_, env := serve(t)
	cliOutput(t, env, "domain", "add", "example.com")
	cliOutput(t, env, "list", "add", "news", "--name", "News", "--from", "news@example.com", "--address", "News, Weg 1, Berlin")
	consent := "Signed register, November"
	cliOutput(t, env, "reader", "add", "news", "ann@example.com", "--confirmed", "--consent", consent)
	export := func(args ...string) string {
		t.Helper()
		return cliOutput(t, env, append([]string{"reader", "ls", "news", "--csv"}, args...)...)
	}
	wire := decodeOutput[api.CSV](t, export("--json"))
	records, err := csv.NewReader(strings.NewReader(wire.CSV)).ReadAll()
	if err != nil || len(records) != 2 || wire.Rows != 1 {
		t.Fatalf("export: %+v %v", wire, err)
	}
	for field, want := range map[string]string{"email": "ann@example.com", "consent": consent, "status": "confirmed"} {
		idx := slices.Index(records[0], field)
		if idx < 0 || records[1][idx] != want {
			t.Fatalf("field %s: %v", field, records)
		}
	}
	if out := export(); out != wire.CSV {
		t.Fatalf("raw CSV differs: %q", out)
	}
	path := filepath.Join(t.TempDir(), "members.csv")
	result := decodeOutput[struct {
		File string
		Rows int
	}](t, export("--out", path, "--json"))
	if result.File != path || result.Rows != 1 {
		t.Fatalf("file result: %+v", result)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != wire.CSV {
		t.Fatalf("file export: %q %v", data, err)
	}
	requireText(t, export("--out", path), "Wrote 1 members to "+path)
	code, _, errs := run(t, env, "", "reader", "ls", "news", "--csv", "--out", filepath.Dir(path))
	if code != ExitFailure {
		t.Fatalf("export to directory: %d %s", code, errs)
	}
	requireText(t, errs, filepath.Dir(path))
}

func controlFixture(t *testing.T, register func(*ctl.Server)) map[string]string {
	t.Helper()
	dir := socketDir(t)
	l, err := ctl.Listen(filepath.Join(dir, "lfr.sock"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	srv := &ctl.Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	register(srv)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); srv.Serve(ctx, l) }()
	t.Cleanup(func() { cancel(); <-done })
	return map[string]string{"LFR_DATA": dir}
}

func TestEventsFollowFormatsEveryRecord(t *testing.T) {
	at := time.Date(2025, 1, 2, 15, 4, 5, 0, time.UTC)
	stream := []events.Event{
		{ID: 42, At: at, Kind: "signup", Detail: "news"},
		{ID: 43, At: at.Add(time.Second), Kind: "delivery", Detail: "mail delivered"},
	}
	env := controlFixture(t, func(s *ctl.Server) {
		s.Stream(api.OpWatch, func(_ context.Context, raw json.RawMessage, send func(any) error) error {
			var args api.Watch
			if err := json.Unmarshal(raw, &args); err != nil {
				return err
			}
			if !slices.Equal(args.Kinds, []string{"signup", "delivery"}) {
				t.Errorf("stream filters: %v", args.Kinds)
			}
			for _, e := range stream {
				if err := send(e); err != nil {
					return err
				}
			}
			return nil
		})
	})
	args := []string{"events", "--follow", "--kind", "signup", "--kind", "delivery"}
	human := strings.Split(strings.TrimSpace(cliOutput(t, env, args...)), "\n")
	if len(human) != len(stream) {
		t.Fatal(human)
	}
	for i, e := range stream {
		if strings.Join(strings.Fields(human[i]), " ") != e.At.Local().Format("15:04:05")+" "+e.Kind+" "+e.Detail {
			t.Fatal(human[i])
		}
	}
	lines := strings.Split(strings.TrimSpace(cliOutput(t, env, append(args, "--json")...)), "\n")
	if len(lines) != len(stream) {
		t.Fatal(lines)
	}
	for i, line := range lines {
		if got := decodeOutput[events.Event](t, line); got != stream[i] {
			t.Fatalf("record %d: %+v", i, got)
		}
	}
}

func TestAttachmentSavePaths(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	payload := []byte{0, 1, 255}
	env := controlFixture(t, func(s *ctl.Server) {
		s.Handle(api.OpMailAttachment, func(_ context.Context, raw json.RawMessage) (any, error) {
			var a api.Attachment
			if err := json.Unmarshal(raw, &a); err != nil {
				return nil, err
			}
			if a.ID != 31 || a.Index != 2 {
				t.Errorf("attachment request: %+v", a)
			}
			return api.File{Name: "../.report.bin", Type: "application/octet-stream", Data: payload}, nil
		})
	})
	args := []string{"mail", "read", "#31", "--attachment", "2"}
	for _, name := range []string{"report.bin", "report (1).bin"} {
		if err := os.WriteFile(name, []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	receipt := decodeOutput[struct {
		File, Type string
		Bytes      int
	}](t, cliOutput(t, env, append(args, "--json")...))
	if receipt.File != "report (2).bin" || receipt.Bytes != 3 || receipt.Type != "application/octet-stream" {
		t.Fatalf("save receipt: %+v", receipt)
	}
	for name, want := range map[string]string{"report.bin": "original", "report (1).bin": "original", receipt.File: string(payload)} {
		got, err := os.ReadFile(name)
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q %v", name, got, err)
		}
	}
	requireText(t, cliOutput(t, env, append(args, "-o", "report.bin")...), "Saved report.bin (3 bytes)")
	got, err := os.ReadFile("report.bin")
	if err != nil || !slices.Equal(got, payload) {
		t.Fatalf("explicit replacement: %v %v", got, err)
	}
	code, _, errs := run(t, env, "", append(args, "--out", dir)...)
	if code != ExitFailure {
		t.Fatalf("save to directory: %d %s", code, errs)
	}
	requireText(t, errs, dir)
}

func TestDisconnectedOperatorCommands(t *testing.T) {
	dir := socketDir(t)
	env := map[string]string{"LFR_DATA": dir}
	for _, cmd := range []string{
		"config", "config relay.rate", "config relay.rate 15",
		"domain ls", "domain dns example.com --zone", "domain set example.com --catchall off",
		"queue drop 2", "events --follow", "mail read 2 --attachment 1",
		"reader ls news --csv", "reader ls news --after 3", "issue show news/test",
		"mailbox set ann@example.com --name Ann", "mailbox alias info@example.com ann@example.com",
		"list set news --tracking=false", "reader unblock ann@example.com",
	} {
		t.Run(cmd, func(t *testing.T) {
			args := append(strings.Fields(cmd), "--json")
			code, out, errs := run(t, env, "", args...)
			e := decodeOutput[struct {
				Error, Code, Next string
				Exit              int
			}](t, out)
			if code != ExitNotRunning || e.Exit != code || e.Code != "not_running" {
				t.Fatalf("%v: %d %+v", args, code, e)
			}
			requireText(t, e.Error, "no server is running", dir)
			requireText(t, e.Next, "lfr serve --data "+dir)
			requireText(t, errs, dir)
		})
	}
}

func TestInterruptedCommandPreservesError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var out, errs strings.Builder
	code := Run(ctx, Env{Stdout: &out, Stderr: &errs}, []string{"queue", "drop", "-19", "--json"})
	e := decodeOutput[struct {
		Error, Code string
		Exit        int
	}](t, out.String())
	if code != ExitInterrupt || e.Exit != code || e.Code != "interrupted" || e.Error != `"-19" is not an ID` {
		t.Fatalf("interrupted command: %d %+v", code, e)
	}
	requireText(t, errs.String(), "lfr queue drop", e.Error)
}

func TestShortFlagDiagnostic(t *testing.T) {
	code, _, errs := run(t, nil, "", "queue", "drop", "-1x")
	if code != ExitUsage {
		t.Fatalf("short flag: %d %s", code, errs)
	}
	requireText(t, errs, "unknown flag -1x", "flags are spelled --name")
}

func TestTerminalStatusColors(t *testing.T) {
	device, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	t.Setenv("TERM", "xterm")
	t.Setenv("NO_COLOR", "")
	for status, code := range map[string]string{"ok": "32", "warning": "33", "warn": "33", "error": "31", "fail": "31", "skip": "2", "info": "36"} {
		if got := paint(device, status); got != "\x1b["+code+"m"+status+"\x1b[0m" {
			t.Errorf("%s: %q", status, got)
		}
	}
	if got := paint(device, "pending"); got != "pending" {
		t.Fatal(got)
	}
	t.Setenv("NO_COLOR", "1")
	if got := paint(device, "fail"); got != "fail" {
		t.Fatal(got)
	}
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	if got := paint(device, "ok"); got != "ok" {
		t.Fatal(got)
	}
}

func TestTableCellText(t *testing.T) {
	for _, tc := range []struct {
		text  string
		width int
		want  string
	}{
		{"  München\n am  See  ", 20, "München am See"},
		{"東京  café", 5, "東京 c…"},
		{"Über", 1, "…"},
	} {
		if got := cut(tc.text, tc.width); got != tc.want {
			t.Errorf("cut(%q,%d) = %q, want %q", tc.text, tc.width, got, tc.want)
		}
	}
}

func TestRelativeTimestamps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		for _, tc := range []struct {
			at   time.Time
			want string
		}{
			{time.Time{}, ""}, {now, "just now"}, {now.Add(-time.Minute), "1m ago"}, {now.Add(-time.Hour), "1h ago"}, {now.Add(-48 * time.Hour), "2d ago"},
			{now.AddDate(0, 0, -90), now.AddDate(0, 0, -90).Local().Format("2006-01-02")},
		} {
			if got := ago(tc.at); got != tc.want {
				t.Errorf("ago(%v) = %q, want %q", tc.at, got, tc.want)
			}
		}
	})
}

func TestQueueDropReceipts(t *testing.T) {
	env := controlFixture(t, func(s *ctl.Server) {
		s.Handle(api.OpQueueDrop, func(_ context.Context, raw json.RawMessage) (any, error) {
			var entry api.ID
			if err := json.Unmarshal(raw, &entry); err != nil {
				return nil, err
			}
			switch entry.ID {
			case 42, 43:
				return nil, nil
			default:
				return nil, &ctl.Error{Code: ctl.CodeNotFound, Msg: "queued entry missing"}
			}
		})
	})
	if out := cliOutput(t, env, "queue", "drop", "#42"); out != "Dropped 42\n" {
		t.Fatal(out)
	}
	receipt := decodeOutput[map[string]int64](t, cliOutput(t, env, "queue", "drop", "43", "--json"))
	if receipt["dropped"] != 43 {
		t.Fatal(receipt)
	}
	code, out, errs := run(t, env, "", "queue", "drop", "44", "--json")
	e := decodeOutput[struct {
		Error, Code string
		Exit        int
	}](t, out)
	if code != ExitFailure || e.Exit != code || e.Code != ctl.CodeNotFound || e.Error != "queued entry missing" {
		t.Fatalf("missing queue entry: %d %s%s", code, out, errs)
	}
}

func TestIssueLocalMediaResolvedBesideDraft(t *testing.T) {
	dir := t.TempDir()
	draft := filepath.Join(dir, "letter.md")
	markdown := "# A letter\n\n![Chart](chart.png)\n\n![Same chart](chart.png)\n\n![Remote](https://example.com/banner.png)\n"
	if err := os.WriteFile(draft, []byte(markdown), 0600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"LFR_DATA": socketDir(t)}
	code, _, errs := run(t, env, "", "issue", "put", "news", draft)
	if code != ExitFailure {
		t.Fatalf("missing media: %d %s", code, errs)
	}
	requireText(t, errs, "image chart.png", filepath.Join(dir, "chart.png"))
	content := []byte{0, 1, 2, 255}
	if err := os.WriteFile(filepath.Join(dir, "chart.png"), content, 0600); err != nil {
		t.Fatal(err)
	}
	files, err := localMedia(dir, markdown)
	if err != nil {
		t.Fatal(err)
	}
	if files == nil || len(*files) != 1 {
		t.Fatalf("expected only the local image, got %+v", files)
	}
	if f := (*files)[0]; f.Path != "chart.png" || !slices.Equal(f.Data, content) {
		t.Fatalf("image upload: %+v", f)
	}
}
