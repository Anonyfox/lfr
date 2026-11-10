package outbound

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/dnsx/dnsxtest"
	"github.com/Anonyfox/lfr/internal/store"
)

func TestOperatorRetryAndDropPreserveCompletedDeliveries(t *testing.T) {
	q, d, c := newQueue(t, &fakeTransport{})
	ctx := context.Background()
	for _, state := range []string{"queued", "failed", "sent", "bounced", "canceled"} {
		if err := q.Enqueue(ctx, Item{Kind: KindMail, DomainID: d.ID, Rcpt: state + "@example.org"}); err != nil {
			t.Fatal(err)
		}
		if _, err := q.DB.W.ExecContext(ctx, "UPDATE outbound SET status = ?, next_at = ?, last_error = 'test failure' WHERE rcpt = ?", state, c.Now().Add(time.Hour).Unix(), state+"@example.org"); err != nil {
			t.Fatal(err)
		}
	}
	queued, err := q.List(ctx, "", 0)
	if err != nil || len(queued) != 1 || queued[0].Rcpt != "queued@example.org" {
		t.Fatalf("default queue: %+v %v", queued, err)
	}
	failed, err := q.List(ctx, "failed", 1)
	if err != nil || len(failed) != 1 || failed[0].Error != "test failure" {
		t.Fatalf("failed list: %+v %v", failed, err)
	}
	c.Add(24 * time.Hour)
	if n, err := q.Retry(ctx, 0); err != nil || n != 1 {
		t.Fatalf("bulk retry: %d %v", n, err)
	}
	if n, err := q.Retry(ctx, failed[0].ID); err != nil || n != 1 {
		t.Fatalf("failed retry: %d %v", n, err)
	}
	got, err := q.List(ctx, "queued", 1001)
	if err != nil || len(got) != 2 || got[0].ID != failed[0].ID || !got[0].Created.Equal(c.Now()) || !got[0].Next.Equal(c.Now()) {
		t.Fatalf("retry lifetime: %+v %v", got, err)
	}
	for _, state := range []string{"sent", "bounced", "canceled"} {
		rows, err := q.List(ctx, state, 1)
		if err != nil || len(rows) != 1 {
			t.Fatalf("%s: %+v %v", state, rows, err)
		}
		if _, err := q.Retry(ctx, rows[0].ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("retried %s: %v", state, err)
		}
	}
	if err := q.Drop(ctx, queued[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := q.Drop(ctx, queued[0].ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("dropped completed entry: %v", err)
	}
	remaining, err := q.List(ctx, "queued", 10)
	if err != nil || len(remaining) != 1 || remaining[0].ID != failed[0].ID {
		t.Fatalf("drop affected other rows: %+v %v", remaining, err)
	}
}

type policyTransport func(*http.Request) (*http.Response, error)

func (f policyTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSTSFetchChecksAdvertisementAndHTTPStatus(t *testing.T) {
	for _, tc := range []struct {
		name, txt, body string
		status          int
		want            bool
		requests        int
	}{
		{"unadvertised", "other TXT", "", 200, false, 0},
		{"valid", "v=STSv1; id=1", "version: STSv1\nmode: enforce\nmx: mx.example.org\nmax_age: 86400\n", 200, true, 1},
		{"redirect", "v=STSv1; id=1", "", 302, false, 1},
		{"missing policy", "v=STSv1; id=1", "", 404, false, 1},
		{"invalid policy", "v=STSv1; id=1", "version: broken\nmode: enforce\n", 200, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dns := dnsxtest.New()
			dns.SetTXT("_mta-sts.example.org", tc.txt)
			calls := 0
			c := STSCache{DNS: dns, Client: &http.Client{Transport: policyTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != "https://mta-sts.example.org/.well-known/mta-sts.txt" || r.Method != "GET" {
					t.Errorf("policy request: %s %s", r.Method, r.URL)
				}
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}}
			for i := 0; i < 2; i++ {
				p := c.Get(context.Background(), "example.org")
				if (p != nil) != tc.want || p != nil && (!p.Enforce() || !p.Matches("mx.example.org")) {
					t.Fatalf("policy: %+v", p)
				}
			}
			if calls != tc.requests {
				t.Fatalf("requests %d, want %d", calls, tc.requests)
			}
		})
	}
}
