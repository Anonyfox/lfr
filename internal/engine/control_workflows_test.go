package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Anonyfox/lfr/internal/api"
)

func TestControlMailAndReaderLifecycle(t *testing.T) {
	h := start(t)
	ctx := context.Background()
	for _, s := range []struct{ name, op, args, want string }{
		{"domain", api.OpDomainAdd, `{"name":"example.com"}`, `example.com`},
		{"mailbox", api.OpMailboxAdd, `{"address":"jo@example.com","name":"Jo"}`, `jo@example.com`},
		{"forward", api.OpMailboxSet, `{"address":"jo@example.com","forward":"reader@other.org"}`, `reader@other.org`},
		{"rename", api.OpMailboxRename, `{"from":"jo@example.com","to":"joseph@example.com"}`, `joseph@example.com`},
		{"alias", api.OpAliasAdd, `{"alias":"hi@example.com","target":"joseph@example.com"}`, ``},
		{"remove alias", api.OpAliasRemove, `{"name":"hi@example.com"}`, ``},
		{"send", api.OpMailSend, `{"from":"joseph@example.com","to":["a@other.org"],"subject":"Meeting","markdown":"# Meeting\n\nRead **notes**.","files":[{"name":"notes.txt","data":"bm90ZXM="}]}`, `queued`},
		{"read", api.OpMailRead, `{"id":1,"keep_unread":true}`, `Meeting`},
		{"raw", api.OpMailRead, `{"id":1,"raw":true}`, `notes.txt`},
		{"attachment", api.OpMailAttachment, `{"id":1,"index":0}`, `bm90ZXM=`},
		{"flag", api.OpMailFlag, `{"ids":[1],"flag":"flagged","on":true}`, `changed`},
		{"archive", api.OpMailMove, `{"ids":[1],"folder":"archive"}`, `archive`},
		{"reply", api.OpMailReply, `{"id":1,"text":"Thanks","all":true}`, `queued`},
		{"trash", api.OpMailDelete, `{"ids":[1]}`, `deleted`},
		{"purge", api.OpMailDelete, `{"ids":[1],"purge":true}`, `deleted`},
		{"spam stats", api.OpSpamStats, `{}`, `weights`},
		{"reset", api.OpSpamReset, `{}`, ``},
		{"list", api.OpListAdd, `{"slug":"news","name":"News","from":"news@example.com","address":"Weg 1, Berlin"}`, `news`},
		{"list show", api.OpListShow, `{"name":"news"}`, `News`},
		{"import", api.OpSubImport, `{"list":"news","csv":"email,name\na@reader.org,Ann\n","confirmed":true,"consent":"Paper form"}`, `"added":1`},
		{"export", api.OpSubExport, `{"name":"news"}`, `Paper form`},
		{"contact", api.OpContactShow, `{"name":"a@reader.org"}`, `confirmed`},
		{"unsubscribe", api.OpSubRemove, `{"list":"news","email":"a@reader.org"}`, `unsubscribed`},
		{"block batch", api.OpSuppress, `{"emails":["a@reader.org","bad"],"reason":"reader request"}`, `bad`},
		{"unblock", api.OpUnsuppress, `{"name":"a@reader.org"}`, ``},
		{"forget", api.OpContactForget, `{"name":"a@reader.org"}`, ``},
		{"preview draft", api.OpIssuePut, `{"list":"news","slug":"hello","markdown":"# Hello\n\nUseful **news**."}`, `hello`},
		{"list issues", api.OpIssueList, `{"list":"news"}`, `hello`},
		{"preview", api.OpIssuePreview, `{"list":"news","slug":"hello"}`, `<strong>news</strong>`},
		{"test", api.OpIssueTest, `{"list":"news","slug":"hello","to":["owner@reader.org"]}`, ``},
		{"schedule", api.OpIssueSend, `{"list":"news","slug":"hello","at":"2099-01-01T10:00:00Z","force":true}`, `scheduled`},
		{"cancel", api.OpIssueCancel, `{"list":"news","slug":"hello"}`, `draft`},
		{"stats", api.OpIssueStats, `{"list":"news","slug":"hello"}`, `recipients`},
		{"remove issue", api.OpIssueRemove, `{"list":"news","slug":"hello"}`, ``},
		{"events after erasure", api.OpEvents, `{"since":"7d","list":"news"}`, `[]`},
		{"reports", api.OpReports, `{"kind":"dmarc","domain":"example.com"}`, `[]`},
		{"retry", api.OpQueueRetry, `{}`, `retrying`},
		{"relay off", api.OpRelaySet, `{"url":""}`, `direct`},
		{"webhooks", api.OpRelayWebhook, `{}`, `generic`},
		{"remove list", api.OpListRemove, `{"name":"news"}`, ``},
		{"remove mailbox", api.OpMailboxRemove, `{"name":"joseph@example.com"}`, ``},
		{"remove domain", api.OpDomainRemove, `{"name":"example.com"}`, ``},
	} {
		t.Run(s.name, func(t *testing.T) {
			var out json.RawMessage
			if err := h.c.Call(ctx, s.op, json.RawMessage(s.args), &out); err != nil {
				t.Fatalf("%s: %v", s.op, err)
			}
			var decoded any
			if err := json.Unmarshal(out, &decoded); err != nil {
				t.Fatal(err)
			}
			raw := string(out)
			if s.name == "preview" {
				var p api.Preview
				if err := json.Unmarshal(out, &p); err != nil {
					t.Fatal(err)
				}
				raw = p.HTML
			}
			if !strings.Contains(raw, s.want) {
				t.Fatalf("want %q in %s", s.want, raw)
			}
		})
		if t.Failed() {
			return
		}
	}
}
