package domains

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/store"
)

func newService(t *testing.T) *Service {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "lfr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &Service{DB: db, Now: func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }}
}

func TestAddGetRemove(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	d, err := s.Add(ctx, "Example.COM")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "example.com" || d.Selector != "lfr2610" || len(d.Signers()) != 2 {
		t.Fatalf("domain %+v", d)
	}
	if _, err := s.Add(ctx, "example.com"); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	var n int
	s.DB.R.QueryRow("SELECT count(*) FROM mailboxes WHERE local = 'postmaster'").Scan(&n)
	if n != 1 {
		t.Fatal("no postmaster mailbox")
	}
	again, err := s.Get(ctx, "example.com")
	if err != nil || again.RSAPublic() != d.RSAPublic() || again.EdPublic() != d.EdPublic() {
		t.Fatalf("reload changed keys: %v", err)
	}
	list, _ := s.List(ctx)
	if len(list) != 1 {
		t.Fatalf("list %d", len(list))
	}
	if err := s.Remove(ctx, "example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "example.com"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("after remove: %v", err)
	}
}

func TestRotate(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	d, _ := s.Add(ctx, "example.com")
	s.Now = func() time.Time { return time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC) }
	p, err := s.PrepareRotation(ctx, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if p.Selector != d.Selector || p.RSAPublic() != d.RSAPublic() || p.Next == nil {
		t.Fatal("prepared keys must not sign yet")
	}
	recs := Records(p, Instance{Hostname: "mail.example.com"}, nil)
	published := 0
	for _, r := range recs {
		if r.Name == p.Next.RSASelector()+"._domainkey.example.com" || r.Name == p.Selector+"r._domainkey.example.com" {
			published++
		}
	}
	if published != 2 {
		t.Fatalf("records must hold the current and the next key, got %d", published)
	}
	again, _ := s.PrepareRotation(ctx, "example.com")
	if again.Next.Selector != p.Next.Selector {
		t.Fatal("a second prepare must keep the pending keys")
	}
	r, err := s.Rotate(ctx, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if r.Selector != p.Next.Selector || r.RSAPublic() != p.Next.RSAPublic() || r.Next != nil {
		t.Fatal("rotation did not switch to the prepared keys")
	}
}

func TestRecords(t *testing.T) {
	s := newService(t)
	d, _ := s.Add(context.Background(), "example.com")
	inst := Instance{Hostname: "mail.example.com", IPv4: "203.0.113.7", RelaySPF: "include:amazonses.com"}
	recs := Records(d, inst, []string{"news.example.com"})
	want := map[string]string{
		"A mail.example.com":         "203.0.113.7",
		"MX example.com":             "mail.example.com",
		"TXT example.com":            "v=spf1 a:mail.example.com include:amazonses.com ~all",
		"TXT _dmarc.example.com":     "v=DMARC1; p=none; rua=mailto:dmarc-reports@example.com",
		"CNAME mta-sts.example.com":  "mail.example.com",
		"A news.example.com":         "203.0.113.7",
		"TXT _smtp._tls.example.com": "v=TLSRPTv1; rua=mailto:tls-reports@example.com",
	}
	got := map[string]string{}
	for _, r := range recs {
		got[r.Type+" "+r.Name] = r.Value
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if !strings.HasPrefix(got["TXT lfr2610r._domainkey.example.com"], "v=DKIM1; k=rsa; p=MII") {
		t.Error("RSA DKIM record missing")
	}
	other, _ := s.Add(context.Background(), "other.org")
	for _, r := range Records(other, inst, nil) {
		if r.Type == "A" {
			t.Errorf("other.org got the mail host A record: %+v", r)
		}
	}
	zone := Zone(recs)
	if !strings.Contains(zone, "example.com.\t300\tIN\tMX\t10 mail.example.com.") || !strings.Contains(zone, `" "`) {
		t.Errorf("zone:\n%s", zone)
	}
}
