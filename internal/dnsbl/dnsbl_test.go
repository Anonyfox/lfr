package dnsbl

import (
	"context"
	"net"
	"testing"

	"github.com/Anonyfox/lfr/internal/dnsx/dnsxtest"
)

func TestQueryNames(t *testing.T) {
	if q := Query(net.ParseIP("192.0.2.99"), "zen.spamhaus.org"); q != "99.2.0.192.zen.spamhaus.org" {
		t.Fatal(q)
	}
	if q := Query(net.ParseIP("2001:db8::1"), "l.test"); q != "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.l.test" {
		t.Fatal(q)
	}
}

func TestCheck(t *testing.T) {
	dns := dnsxtest.New()
	dns.SetIP("99.2.0.192.zen.spamhaus.org", "127.0.0.4")
	dns.SetIP("99.2.0.192.refusing.test", "127.255.255.254")
	dns.SetIP("98.2.0.192.zen.spamhaus.org", "127.255.255.252")
	c := &Checker{DNS: dns}
	lists := []string{"zen.spamhaus.org", "refusing.test", "clean.test"}
	if hits := c.Check(context.Background(), net.ParseIP("192.0.2.99"), lists); len(hits) != 1 || hits[0] != "zen.spamhaus.org" {
		t.Fatalf("hits %v", hits)
	}
	if hits := c.Check(context.Background(), net.ParseIP("192.0.2.98"), lists); len(hits) != 0 {
		t.Fatalf("error code counted as listing: %v", hits)
	}
	if hits := c.Check(context.Background(), net.ParseIP("10.0.0.1"), lists); hits != nil {
		t.Fatal("private address checked")
	}
}
