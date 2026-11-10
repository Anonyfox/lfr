package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnsafeOrMalformedCommandsReturnActionableJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"mail requires recipient", []string{"mail", "send", "--from", "jo@example.com", "--subject", "Hi"}, "--to"},
		{"mail requires body", []string{"mail", "send", "--from", "jo@example.com", "--to", "a@reader.org", "--subject", "Hi"}, "body"},
		{"mail does not take positional body", []string{"mail", "send", "hello"}, "--md"},
		{"paging id", []string{"mail", "ls", "--before", "oops"}, "ID"},
		{"limit", []string{"mail", "ls", "--limit", "many"}, "number"},
		{"move ids", []string{"mail", "mv", "oops", "archive"}, "ID"},
		{"flag ids", []string{"mail", "flag", "oops", "seen"}, "ID"},
		{"delete ids", []string{"mail", "rm", "oops"}, "ID"},
		{"attachment index", []string{"mail", "read", "1", "--attachment", "oops"}, "index"},
		{"reply requires body", []string{"mail", "reply", "1"}, "--md"},
		{"queue id", []string{"queue", "drop", "oops"}, "ID"},
		{"queue retry id", []string{"queue", "retry", "oops"}, "ID"},
		{"spam reset confirmation", []string{"mail", "spam", "--reset"}, "--yes"},
		{"reader erasure confirmation", []string{"reader", "forget", "a@reader.org"}, "--yes"},
		{"domain removal confirmation", []string{"domain", "rm", "example.com"}, "--yes"},
		{"unknown issue reference", []string{"issue", "show", "unknown"}, "issue"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, tc.args...), "--json")
			code, out, errs := run(t, nil, "", args...)
			var got struct {
				Error, Code, Next string
				Exit              int
			}
			if err := json.Unmarshal([]byte(out), &got); err != nil || code != ExitUsage || got.Exit != code || got.Code != "usage" || got.Next == "" || !strings.Contains(strings.ToLower(got.Error), strings.ToLower(tc.want)) {
				t.Fatalf("%v: %d %s%s", args, code, out, errs)
			}
		})
	}
}

func TestFlagParserErrorsHonorJSONWithoutMistakingValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		json bool
	}{
		{"output flag after error", []string{"list", "set", "news", "--tracking=perhaps", "--json"}, true},
		{"leading output flag", []string{"--json", "mail", "ls", "--unknown"}, true},
		{"output-looking value", []string{"mail", "send", "--subject", "--json", "--unknown"}, false},
		{"literal after terminator", []string{"mail", "ls", "--unknown", "--", "--json"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errs := run(t, nil, "", tc.args...)
			if code != ExitUsage || !strings.Contains(errs, "lfr ") {
				t.Fatalf("%d %s%s", code, out, errs)
			}
			if !tc.json {
				if out != "" {
					t.Fatal(out)
				}
				return
			}
			var got struct {
				Code, Error, Next string
				Exit              int
			}
			if err := json.Unmarshal([]byte(out), &got); err != nil || got.Code != "usage" || got.Exit != ExitUsage || got.Error == "" || got.Next == "" {
				t.Fatalf("missing structured parser error: %s", out)
			}
		})
	}
}
