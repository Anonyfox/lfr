package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/Anonyfox/lfr/internal/dnsx/dnsxtest"
	"github.com/Anonyfox/lfr/internal/domains"
)

func TestTXTStatus(t *testing.T) {
	dns := dnsxtest.New()
	for i := range 12 {
		dns.TXT[fmt.Sprintf("n%d.example", i)] = []string{"v=spf1 a -all"}
	}
	dns.TXT["amazonses.com"] = []string{"v=spf1 ip4:199.255.192.0/22 -all"}
	e := &Engine{DNS: dns}
	ctx := context.Background()
	spf := domains.Record{Value: "v=spf1 a:mail.x.com include:amazonses.com ~all"}
	dkim := domains.Record{Value: "v=DKIM1; k=ed25519; p=AbC="}
	dmarc := domains.Record{Value: "v=DMARC1; p=none; rua=mailto:dmarc-reports@x.com"}
	sts := domains.Record{Value: "v=STSv1; id=abc"}
	many := "v=spf1 a:mail.x.com include:amazonses.com"
	for i := range 10 {
		many += fmt.Sprintf(" include:n%d.example", i)
	}
	for _, c := range []struct {
		r    domains.Record
		txts []string
		want string
	}{
		{spf, []string{"v=spf1 a:mail.x.com include:amazonses.com ~all"}, "ok"},
		{spf, []string{"v=spf1 mx a:mail.x.com include:amazonses.com -all"}, "ok"},
		{spf, []string{"v=spf1 a:mail.x.com.evil include:amazonses.com ~all"}, "wrong"},
		{spf, []string{"v=spf1 a:mail.x.com include:amazonses.com +all"}, "wrong"},
		{spf, []string{"v=spf1 a:mail.x.com include:amazonses.com ~all", "v=spf1 -all"}, "wrong"},
		{spf, []string{many + " ~all"}, "wrong"},
		{spf, []string{"google-site-verification=x"}, "missing"},
		{dkim, []string{"v=DKIM1; k=ed25519; p=AbC="}, "ok"},
		{dkim, []string{"v=DKIM1; p=AbC="}, "wrong"},
		{dkim, []string{"v=DKIM1; k=ed25519; p=AbC=x"}, "wrong"},
		{dmarc, []string{"v=DMARC1; p=quarantine; rua=mailto:me@elsewhere.org"}, "ok"},
		{dmarc, []string{"v=DMARC1; p=reject", "v=DMARC1; p=none"}, "wrong"},
		{dmarc, []string{"v=DMARC1; p=maybe"}, "wrong"},
		{sts, []string{"v=STSv1; id=old"}, "wrong"},
		{sts, []string{"v=STSv1;id=abc"}, "ok"},
	} {
		if got := e.txtStatus(ctx, c.r, c.txts); got != c.want {
			t.Errorf("%q against %q: %s, want %s", c.txts, c.r.Value, got, c.want)
		}
	}
}
