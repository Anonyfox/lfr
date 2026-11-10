package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/ctl"
)

func TestControlRejectsInvalidMutations(t *testing.T) {
	h := start(t)
	ctx := context.Background()
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, nil)
	h.call(t, api.OpListAdd, json.RawMessage(`{"slug":"news","name":"News","from":"news@example.com"}`), nil)
	for _, tc := range []struct{ name, op, args, code string }{
		{"wrong argument shape", api.OpListSet, `{"slug":[]}`, ctl.CodeInvalid},
		{"unknown setting", api.OpConfigGet, `{"name":"missing"}`, ctl.CodeInvalid},
		{"set unknown setting", api.OpConfigSet, `{"name":"missing","value":"1"}`, ctl.CodeInvalid},
		{"invalid setting", api.OpConfigSet, `{"name":"relay.rate","value":"0"}`, ctl.CodeInvalid},
		{"invalid gap", api.OpListSet, `{"slug":"news","min_gap":"soon"}`, ctl.CodeInvalid},
		{"import requires consent", api.OpSubImport, `{"list":"news","confirmed":true,"csv":"email\na@reader.org\n"}`, ctl.CodeInvalid},
		{"import contradicts itself", api.OpSubImport, `{"list":"news","confirmed":true,"reconfirm":true,"consent":"paper","csv":"email\na@reader.org\n"}`, ctl.CodeInvalid},
		{"broken csv", api.OpSubImport, `{"list":"news","csv":"email\n\"broken"}`, ctl.CodeInvalid},
		{"add requires consent", api.OpSubAdd, `{"list":"news","confirmed":true,"emails":["a@reader.org"]}`, ctl.CodeInvalid},
		{"invalid image", api.OpIssuePut, `{"list":"news","markdown":"# News","media":[{"path":"bad.png","data":"bm90ZXM="}]}`, ctl.CodeInvalid},
		{"test requires address", api.OpIssueTest, `{"id":1}`, ctl.CodeInvalid},
		{"reference requires slug", api.OpIssueShow, `{"list":"news"}`, ctl.CodeInvalid},
		{"reference requires existing list", api.OpIssueShow, `{"list":"missing","slug":"news"}`, ctl.CodeNotFound},
		{"missing issue", api.OpIssueShow, `{"list":"news","slug":"missing"}`, ctl.CodeNotFound},
		{"missing attachment", api.OpMailAttachment, `{"id":999,"index":0}`, ctl.CodeNotFound},
		{"unknown folder", api.OpMailMove, `{"ids":[1],"folder":"random"}`, ctl.CodeInvalid},
		{"invalid relay", api.OpRelaySet, `{"url":"https://relay.example"}`, ctl.CodeInvalid},
		{"invalid since", api.OpEvents, `{"since":"yesterday"}`, ctl.CodeInvalid},
		{"unknown list events", api.OpEvents, `{"list":"missing"}`, ctl.CodeNotFound},
		{"unknown site", api.OpSiteCheck, `{"name":"missing"}`, ctl.CodeNotFound},
		{"broken template", api.OpSitePut, `{"list":"news","files":{"custom.html":"e3s="}}`, ctl.CodeInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := h.c.Call(ctx, tc.op, json.RawMessage(tc.args), nil)
			var ce *ctl.Error
			if !errors.As(err, &ce) || ce.Code != tc.code {
				t.Fatalf("want %s; got %v", tc.code, err)
			}
		})
	}
	var members []any
	h.call(t, api.OpSubList, json.RawMessage(`{"list":"news"}`), &members)
	if len(members) != 0 {
		t.Fatal("invalid imports added readers")
	}
	var issues []any
	h.call(t, api.OpIssueList, json.RawMessage(`{"list":"news"}`), &issues)
	if len(issues) != 0 {
		t.Fatal("invalid media created issue")
	}
}
