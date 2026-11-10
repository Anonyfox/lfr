package engine

import (
	"context"
	"testing"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/msg"
)

func TestIncomingReportsAreStoredOnceAndFilterable(t *testing.T) {
	h := start(t)
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, nil)
	docs := []struct {
		kind, address, name, typ, body string
		pass, fail                     int
	}{
		{"dmarc", "dmarc-reports@example.com", "report.xml", "application/xml", `<feedback><report_metadata><org_name>Provider</org_name><report_id>report-1</report_id><date_range><begin>1760832000</begin><end>1760918399</end></date_range></report_metadata><policy_published><domain>example.com</domain></policy_published><record><row><source_ip>203.0.113.7</source_ip><count>40</count><policy_evaluated><dkim>pass</dkim><spf>pass</spf></policy_evaluated></row></record></feedback>`, 40, 0},
		{"tlsrpt", "tls-reports@example.com", "report.json", "application/tlsrpt+json", `{"organization-name":"Provider","date-range":{"start-datetime":"2026-10-18T00:00:00Z","end-datetime":"2026-10-18T23:59:59Z"},"report-id":"report-2","policies":[{"policy":{"policy-domain":"example.com"},"summary":{"total-successful-session-count":12,"total-failure-session-count":1}}]}`, 12, 1},
	}
	for _, d := range docs {
		t.Run(d.kind, func(t *testing.T) {
			raw, err := msg.Compose(msg.Draft{From: msg.Address{Email: "reports@provider.org"}, To: []msg.Address{{Email: d.address}}, Subject: "Report", Text: "Report attached", Files: []msg.File{{Name: d.name, Type: d.typ, Data: []byte(d.body)}}})
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := h.smtpSend("reports@provider.org", []string{d.address}, string(raw)); err != nil {
					t.Fatal(err)
				}
			}
			var reports []api.Report
			h.call(t, api.OpReports, api.Reports{Kind: d.kind, Domain: "example.com", Limit: 1}, &reports)
			if len(reports) != 1 || reports[0].Pass != d.pass || reports[0].Fail != d.fail || reports[0].Org != "Provider" || len(reports[0].Rows) == 0 {
				t.Fatalf("reports: %+v", reports)
			}
		})
	}
	var all []api.Report
	h.call(t, api.OpReports, api.Reports{}, &all)
	if len(all) != 2 {
		t.Fatalf("duplicate reports persisted: %+v", all)
	}
	var none []api.Report
	h.call(t, api.OpReports, api.Reports{Domain: "other.org"}, &none)
	if len(none) != 0 {
		t.Fatal("domain filter ignored")
	}
}

func TestRotationWaitsForBothPublishedKeys(t *testing.T) {
	h := start(t)
	var domain api.DomainInfo
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, &domain)
	h.publish(t, domain)
	var prepared api.DomainInfo
	h.call(t, api.OpDomainRotate, api.Name{Name: "example.com"}, &prepared)
	if prepared.Next == "" || prepared.Selector != domain.Selector {
		t.Fatalf("rotation switched before DNS: %+v", prepared)
	}
	if err := h.c.Call(context.Background(), api.OpDomainRotate, api.Name{Name: "example.com"}, nil); err == nil {
		t.Fatal("unpublished keys activated")
	}
	h.publish(t, prepared)
	var rotated api.DomainInfo
	h.call(t, api.OpDomainRotate, api.Name{Name: "example.com"}, &rotated)
	if rotated.Selector != prepared.Next || rotated.Next != "" {
		t.Fatalf("published keys not activated: %+v", rotated)
	}
}
