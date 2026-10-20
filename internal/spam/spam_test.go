package spam

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/mailauth"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/store"
)

func newClassifier(t *testing.T) *Classifier {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "lfr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &Classifier{DB: db, Now: time.Now}
}

func parsed(from, subject, text, html string) *msg.Parsed {
	raw, _ := msg.Compose(msg.Draft{From: msg.Address{Email: from}, To: []msg.Address{{Email: "jo@example.com"}}, Subject: subject, Text: text, HTML: html})
	return msg.Parse(raw)
}

func goodInput(p *msg.Parsed) Input {
	return Input{
		IP: net.ParseIP("198.51.100.20"), Helo: "mail.friend.org", PTR: "mail.friend.org", PTRConfirmed: true,
		Auth:   mailauth.Result{SPF: "pass", SPFDomain: "friend.org", FromDomain: "friend.org", DKIM: []mailauth.DKIMResult{{Domain: "friend.org", Result: "pass"}}, DMARC: "pass"},
		Parsed: p, Received: time.Now(),
	}
}

func badInput(p *msg.Parsed) Input {
	return Input{
		IP: net.ParseIP("203.0.113.66"), Helo: "203.0.113.66", DNSBL: []string{"zen.spamhaus.org"},
		Auth:   mailauth.Result{SPF: "fail", SPFDomain: "bank.com", FromDomain: "bank.com", DMARC: "fail", DMARCPolicy: "none"},
		Parsed: p, Received: time.Now(),
	}
}

func TestRulesSeparateObviousCases(t *testing.T) {
	c := newClassifier(t)
	ctx := context.Background()
	good := c.Classify(ctx, goodInput(parsed("ann@friend.org", "Lunch tomorrow?", "Are you free at noon?", "")))
	bad := c.Classify(ctx, badInput(parsed("security@bank.com", "URGENT ACCOUNT SUSPENDED", "verify now", `<a href="http://evil.example/x">https://bank.com/login</a>`)))
	if good.P > 0.1 || bad.P < 0.98 {
		t.Fatalf("good %.3f %v, bad %.3f %v", good.P, good.Hits, bad.P, bad.Hits)
	}
	x := bad.Sample.X
	for _, f := range []string{"spf_fail", "dmarc_fail", "dnsbl", "no_ptr", "helo_bad", "subject_caps", "link_mismatch"} {
		if x[f] == 0 {
			t.Errorf("feature %s not set: %v", f, x)
		}
	}
}

func TestFeaturesDetails(t *testing.T) {
	raw, _ := msg.Compose(msg.Draft{From: msg.Address{Name: "support@paypal.com", Email: "x@scam.biz"}, Subject: "hi", Text: "see https://bit.ly/abc"})
	f := Features(Input{Parsed: msg.Parse(raw), Auth: mailauth.Result{FromDomain: "scam.biz"}, Helo: "box"})
	if f["from_spoof"] != 1 || f["shortener"] != 1 || f["helo_bad"] != 1 {
		t.Fatalf("features %v", f)
	}
	htmlOnly := msg.Parse([]byte("From: a@b.org\r\nContent-Type: text/html\r\n\r\n<p>hi</p>"))
	if Features(Input{Parsed: htmlOnly})["html_only"] != 1 {
		t.Fatal("html_only not set")
	}
	att := &msg.Parsed{Attachments: []msg.Attachment{{Name: "invoice.pdf.exe"}}}
	if Executable(att) != "invoice.pdf.exe" {
		t.Fatal("executable not detected")
	}
}

func TestBayesAndReputationLearn(t *testing.T) {
	c := newClassifier(t)
	ctx := context.Background()
	for i := 0; i < 25; i++ {
		sp := parsed(fmt.Sprintf("x%d@pills.biz", i), "cheap pills casino bonus", fmt.Sprintf("casino jackpot bonus winner pills offer %d", i), "")
		v := c.Classify(ctx, goodInput(sp))
		b, _ := json.Marshal(v.Sample)
		if err := c.Train(ctx, sp, string(b), "spam", ""); err != nil {
			t.Fatal(err)
		}
		hm := parsed(fmt.Sprintf("y%d@bakers.org", i), "sourdough starter question", fmt.Sprintf("my starter recipe needs flour water hydration %d", i), "")
		v = c.Classify(ctx, goodInput(hm))
		b, _ = json.Marshal(v.Sample)
		c.Train(ctx, hm, string(b), "ham", "")
	}
	st := c.Stats(ctx)
	if st.Spam != 25 || st.Ham != 25 || !st.BayesReady {
		t.Fatalf("stats %+v", st)
	}
	// Same authenticated infrastructure for both: only content can tell them apart.
	spamV := c.Classify(ctx, goodInput(parsed("new@other.biz", "casino bonus", "jackpot winner casino pills", "")))
	hamV := c.Classify(ctx, goodInput(parsed("new@other.org", "starter", "flour water hydration recipe", "")))
	if spamV.Bayes < 0.9 || hamV.Bayes > 0.1 || spamV.P <= hamV.P {
		t.Fatalf("spam bayes %.3f p %.3f, ham bayes %.3f p %.3f", spamV.Bayes, spamV.P, hamV.Bayes, hamV.P)
	}
	w := c.Weights(ctx)
	if w["bayes"] == Prior["bayes"] {
		t.Fatal("weights did not learn")
	}
}

func TestTrainCorrectionUndoes(t *testing.T) {
	c := newClassifier(t)
	ctx := context.Background()
	p := parsed("a@b.org", "hello", "words words", "")
	in := goodInput(p)
	v := c.Classify(ctx, in)
	b, _ := json.Marshal(v.Sample)
	c.Train(ctx, p, string(b), "spam", "")
	c.Train(ctx, p, string(b), "ham", "spam")
	st := c.Stats(ctx)
	if st.Spam != 0 || st.Ham != 1 {
		t.Fatalf("stats %+v", st)
	}
	var spam, ham int
	c.DB.R.QueryRow("SELECT spam, ham FROM reputation WHERE key = 'addr:a@b.org'").Scan(&spam, &ham)
	if spam != 0 || ham != 1 {
		t.Fatalf("reputation %d/%d", spam, ham)
	}
	if err := c.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	if st := c.Stats(ctx); st.Ham != 0 || st.Tokens != 0 {
		t.Fatalf("after reset %+v", st)
	}
}

func TestReputationKeys(t *testing.T) {
	in := goodInput(parsed("ann@mail.friend.org", "x", "y", ""))
	in.Auth.FromDomain = "mail.friend.org"
	keys := strings.Join(ReputationKeys(in), " ")
	if keys != "addr:ann@mail.friend.org dom:friend.org net:198.51.100.0/24" {
		t.Fatal(keys)
	}
	in.Auth.DMARC = "fail"
	if !strings.Contains(strings.Join(ReputationKeys(in), " "), "unauth:friend.org") {
		t.Fatal("unauthenticated domain shares reputation")
	}
}

func TestChi2Q(t *testing.T) {
	if q := chi2Q(0, 4); q != 1 {
		t.Fatal(q)
	}
	if q := chi2Q(100, 4); q > 1e-10 {
		t.Fatal(q)
	}
}

func FuzzFeatures(f *testing.F) {
	f.Add([]byte("From: \"x@\" <a@b.c>\r\nSubject: HELLO WORLD\r\nContent-Type: text/html\r\n\r\n<a href=\"http://bit.ly\">www.x.com</a>"), "helo")
	f.Fuzz(func(t *testing.T, raw []byte, helo string) {
		p := msg.Parse(raw)
		Features(Input{Parsed: p, Helo: helo, Auth: mailauth.Result{FromDomain: p.From.Email}})
		Tokens(p)
	})
}

func TestUnauthenticatedPlainMailIsNotJunk(t *testing.T) {
	c := newClassifier(t)
	p := msg.Parse([]byte("From: Ann <ann@friend.org>\r\nSubject: Frage\r\nMessage-ID: <a@friend.org>\r\n\r\nWie oft?\r\n"))
	v := c.Classify(context.Background(), Input{IP: net.ParseIP("198.51.100.9"), Helo: "mx.friend.org", Parsed: p,
		Auth: mailauth.Result{SPF: "none", FromDomain: "friend.org", DMARC: "none"}, Received: time.Now()})
	if v.P >= 0.6 {
		t.Fatalf("plain unauthenticated mail scored %.2f: %v", v.P, v.Hits)
	}
}

// Before any training, prize spam from an unauthenticated sender without
// reverse DNS lands in junk, and a friend's mail about a winner does not.
func TestColdStartContent(t *testing.T) {
	c := newClassifier(t)
	ctx := context.Background()
	unknown := Input{IP: net.ParseIP("198.51.100.99"), Helo: "mx.prizes.example",
		Auth:   mailauth.Result{SPF: "none", SPFDomain: "prizes.example", FromDomain: "prizes.example", DMARC: "none"},
		Parsed: parsed("winner@prizes.example", "URGENT!!! You WON $1,000,000 - claim NOW", "Dear Friend, you have won. Act now, 100% free, limited time, wire transfer.", ""), Received: time.Now()}
	if v := c.Classify(ctx, unknown); v.P < 0.6 {
		t.Errorf("prize spam scored %.2f (%v)", v.P, v.Hits)
	}
	if v := c.Classify(ctx, goodInput(parsed("ann@friend.org", "The bake-off", "Our rye was the winner on Saturday!", ""))); v.P > 0.3 {
		t.Errorf("a friend's mail scored %.2f (%v)", v.P, v.Hits)
	}
}

// Mail in the name of a domain this server signs for, without its
// signature, is forged.
func TestOwnDomainForgery(t *testing.T) {
	c := newClassifier(t)
	in := Input{IP: net.ParseIP("198.51.100.99"), Helo: "mx.attacker.example", PTR: "mx.attacker.example", PTRConfirmed: true,
		Auth:   mailauth.Result{SPF: "none", SPFDomain: "praxis.example", FromDomain: "praxis.example", DMARC: "none"},
		Parsed: parsed("lena@praxis.example", "Bitte heute noch überweisen", "Bitte die Rechnung an das neue Konto überweisen.", ""), Received: time.Now(), OwnDomain: true}
	if v := c.Classify(context.Background(), in); v.P < 0.6 {
		t.Errorf("forged own-domain mail scored %.2f (%v)", v.P, v.Hits)
	}
}
