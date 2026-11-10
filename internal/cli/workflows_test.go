package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Keep one server for the workflow: each assertion checks a user-visible
// transition, and later reads check that mutations actually persisted.
func TestOperatorWorkflows(t *testing.T) {
	_, env := serve(t)
	step := func(stdin, want string, args ...string) string {
		t.Helper()
		code, out, errs := run(t, env, stdin, args...)
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("%v: exit %d\n%s%s; want %q", args, code, out, errs, want)
		}
		if slicesJSON(args) && !json.Valid([]byte(out)) {
			t.Fatalf("invalid JSON: %s", out)
		}
		return out
	}
	step("", "_dmarc.example.com", "domain", "add", "example.com")
	step("", "IN\tMX", "domain", "dns", "example.com", "--zone")
	step("", "example.com", "domain", "dns", "example.com")
	step("", "Created", "mailbox", "add", "jo@example.com", "--name", "Jo")
	step("", "", "mailbox", "set", "jo@example.com", "--name", "Joseph", "--forward", "elsewhere@other.org")
	step("", "Joseph", "mailbox", "ls")
	step("", "", "mailbox", "mv", "jo@example.com", "joseph@example.com")
	step("", "joseph@example.com", "mailbox", "ls", "--json")
	attachment := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(attachment, []byte("field notes"), 0600); err != nil {
		t.Fatal(err)
	}
	step("# Meeting\n\nRead **these notes**.", "Queued", "mail", "send", "--from", "joseph@example.com", "--to", "reader@other.org", "--subject", "Meeting", "--md", "-", "--attach", attachment)
	step("", "Meeting", "mail", "ls", "--folder", "sent")
	step("", "filename=notes.txt", "mail", "read", "1", "--raw")
	step("", "<strong>these notes</strong>", "mail", "read", "1", "--html")
	saved := filepath.Join(t.TempDir(), "saved.txt")
	step("", "Saved", "mail", "read", "1", "--attachment", "0", "--out", saved)
	if b, err := os.ReadFile(saved); err != nil || string(b) != "field notes" {
		t.Fatalf("attachment: %q %v", b, err)
	}
	step("", "Updated", "mail", "flag", "1", "flagged")
	step("", "Updated", "mail", "flag", "1", "unseen")
	step("", "Moved", "mail", "mv", "1", "archive")
	step("", "Meeting", "mail", "ls", "--folder", "archive", "--unread")
	step("Thanks for the notes.", "Reply queued", "mail", "reply", "1", "--text", "-", "--all", "--no-quote")
	step("", "Deleted", "mail", "rm", "1")
	step("", "Meeting", "mail", "ls", "--folder", "trash")
	step("", "Deleted", "mail", "rm", "1", "--purge")
	step("", "No messages", "mail", "ls", "--folder", "trash")
	step("", "", "mail", "spam")
	step("", "", "mail", "spam", "--reset", "--yes")
	step("", "", "queue", "retry")
	step("", "", "reports")
	step("", "", "relay", "off")
	step("", "/lists/news/", "list", "add", "news", "--name", "News", "--from", "news@example.com", "--address", "News, Weg 1, Berlin")
	step("", "News", "list", "ls")
	step("", "News", "list", "show", "news")
	step("", "1 added", "reader", "add", "news", "a@reader.org", "--confirmed", "--consent", "Signed paper form")
	step("", "a@reader.org", "reader", "show", "a@reader.org")
	step("", "a@reader.org", "reader", "ls", "news", "--status", "confirmed")
	step("", "", "reader", "rm", "news", "a@reader.org")
	step("", "unsubscribed", "reader", "ls", "news", "--status", "unsubscribed", "--json")
	step("", "", "reader", "block", "a@reader.org", "--reason", "Requested by reader")
	step("", "Requested by reader", "reader", "show", "a@reader.org")
	step("", "", "reader", "unblock", "a@reader.org")
	step("", "", "reader", "forget", "a@reader.org", "--yes")
	step("# Hello\n\nA useful **update**.", "Created issue", "issue", "put", "news", "-", "--slug", "hello")
	step("", "hello", "issue", "ls", "news")
	preview := filepath.Join(t.TempDir(), "preview.html")
	step("", "A useful update", "issue", "show", "news/hello", "--text", "--html", preview)
	if b, err := os.ReadFile(preview); err != nil || !strings.Contains(string(b), "<strong>update</strong>") {
		t.Fatalf("preview: %s %v", b, err)
	}
	step("", "Test copies queued", "issue", "test", "news/hello", "owner@reader.org")
	step("", "scheduled", "issue", "send", "news/hello", "--at", "2099-01-01T10:00:00Z", "--force")
	step("", "draft", "issue", "cancel", "news/hello")
	step("", "Removed issue", "issue", "rm", "news/hello")
	site := filepath.Join(t.TempDir(), "site")
	step("", "", "site", "news", site, "--eject")
	step("", "", "site", "news", site)
	step("", "", "site", "news")
	step("", "", "list", "rm", "news", "--yes")
	step("", "", "mailbox", "rm", "joseph@example.com", "--yes")
	step("", "", "domain", "rm", "example.com", "--yes")
}

func slicesJSON(args []string) bool {
	for _, s := range args {
		if s == "--json" {
			return true
		}
	}
	return false
}

func TestSentIssueLearningOutputKeepsTrackingOffExplicit(t *testing.T) {
	_, env := serve(t)
	must := func(stdin, want string, args ...string) string {
		t.Helper()
		code, out, errs := run(t, env, stdin, args...)
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("%v: %d %s%s", args, code, out, errs)
		}
		return out
	}
	must("", "", "domain", "add", "example.com")
	must("", "", "list", "add", "news", "--name", "News", "--from", "news@example.com", "--address", "Weg 1, Berlin")
	must("", "1 added", "reader", "add", "news", "a@reader.org", "--confirmed", "--consent", "Paper form")
	must("# Starter rescue\n\nFeed it twice a day.", "Created issue", "issue", "put", "news", "-")
	must("", "", "issue", "send", "news/starter-rescue", "--force")
	must("", "not measured (tracking off)", "issue", "show", "news/starter-rescue")
	must("", "- Keep:", "issue", "show", "news/starter-rescue", "--learn")
	out := must("", `"numbers"`, "issue", "show", "news/starter-rescue", "--learn", "--json")
	var got struct {
		Line    string
		Numbers map[string]any
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || !strings.Contains(got.Line, "starter-rescue") || got.Numbers == nil {
		t.Fatalf("learning JSON: %s", out)
	}
}

func TestBareCommandExplainsMachineState(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"LFR_DATA": dir}
	code, out, errs := run(t, env, "")
	if code != 0 || !strings.Contains(out, "No lfr data") || !strings.Contains(out, "Next:") {
		t.Fatalf("fresh machine: %d %s%s", code, out, errs)
	}
	code, out, errs = run(t, env, "", "setup", "--hostname", "mail.example.com", "--ipv4", "203.0.113.7")
	if code != 0 {
		t.Fatalf("setup: %d %s%s", code, out, errs)
	}
	code, out, errs = run(t, env, "")
	if code != 0 || !strings.Contains(out, "server is not running") || !strings.Contains(out, "lfr serve --data") {
		t.Fatalf("stopped server: %d %s%s", code, out, errs)
	}
	_, env = serve(t)
	code, out, errs = run(t, env, "")
	if code != 0 || !strings.Contains(out, "Running as mail.example.com") || !strings.Contains(out, "Next:") {
		t.Fatalf("running server: %d %s%s", code, out, errs)
	}
}

func TestCSVBlockUsesTheEmailColumnAndRotationStagesKeys(t *testing.T) {
	_, env := serve(t)
	code, out, errs := run(t, env, "", "domain", "add", "example.com")
	if code != 0 {
		t.Fatalf("%d %s%s", code, out, errs)
	}
	code, out, errs = run(t, env, "", "domain", "rotate", "example.com")
	if code != 0 || !strings.Contains(out, "_domainkey.example.com") {
		t.Fatalf("rotation: %d %s%s", code, out, errs)
	}
	csv := filepath.Join(t.TempDir(), "blocked.csv")
	if err := os.WriteFile(csv, []byte("Name,E-mail\nAnn,ann@reader.org\nMissing\nBlank,\nBo, bo@reader.org \n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, errs = run(t, env, "", "reader", "block", "--csv", csv, "--reason", "Imported opt-outs", "--json")
	var res struct {
		Added   int
		Invalid []string
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil || code != 0 || res.Added != 2 || len(res.Invalid) != 0 {
		t.Fatalf("CSV block: %d %s%s", code, out, errs)
	}
	for _, email := range []string{"ann@reader.org", "bo@reader.org"} {
		code, out, errs = run(t, env, "", "reader", "show", email)
		if code != 0 || !strings.Contains(out, "Imported opt-outs") {
			t.Fatalf("blocked contact: %d %s%s", code, out, errs)
		}
	}
	// Headerless exports use their first column.
	if err := os.WriteFile(csv, []byte("cy@reader.org,Third\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, errs = run(t, env, "", "reader", "block", "--csv", csv)
	if code != 0 || !strings.Contains(out, "cy@reader.org") {
		t.Fatalf("headerless CSV: %d %s%s", code, out, errs)
	}
}
