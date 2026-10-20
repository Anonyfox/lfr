package outbound

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfr/internal/dnsx"
)

// STSPolicy is a recipient domain's MTA-STS policy (RFC 8461).
type STSPolicy struct {
	Mode   string // enforce, testing, none
	MX     []string
	MaxAge time.Duration
}

// Enforce reports whether the policy must be followed.
func (p *STSPolicy) Enforce() bool { return p != nil && p.Mode == "enforce" }

// Matches reports whether host is an allowed MX; "*.example.com" matches one label.
func (p *STSPolicy) Matches(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, m := range p.MX {
		m = strings.ToLower(m)
		if m == host {
			return true
		}
		if strings.HasPrefix(m, "*.") {
			if i := strings.IndexByte(host, '.'); i > 0 && host[i:] == m[1:] {
				return true
			}
		}
	}
	return false
}

// ParseSTSPolicy reads a policy file.
func ParseSTSPolicy(r io.Reader) *STSPolicy {
	p := &STSPolicy{}
	sc := bufio.NewScanner(io.LimitReader(r, 64<<10))
	version := false
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "version":
			version = v == "STSv1"
		case "mode":
			p.Mode = v
		case "mx":
			p.MX = append(p.MX, v)
		case "max_age":
			n, _ := strconv.Atoi(v)
			p.MaxAge = time.Duration(n) * time.Second
		}
	}
	if !version || p.Mode == "" {
		return nil
	}
	return p
}

// STSCache fetches and remembers MTA-STS policies.
type STSCache struct {
	DNS    dnsx.Resolver
	Client *http.Client

	mu    sync.Mutex
	cache map[string]stsEntry
}

type stsEntry struct {
	policy  *STSPolicy
	refresh time.Time // when to look again
	valid   time.Time // fetched plus max_age: how long the policy binds
}

// Get returns domain's policy, or nil when it has none or it cannot be fetched.
func (c *STSCache) Get(ctx context.Context, domain string) *STSPolicy {
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[string]stsEntry{}
	}
	e, ok := c.cache[domain]
	c.mu.Unlock()
	now := time.Now()
	if ok && now.Before(e.refresh) {
		return e.policy
	}
	p := c.fetch(ctx, domain)
	next := stsEntry{policy: p, refresh: now.Add(time.Hour)}
	switch {
	case p != nil:
		next.refresh = now.Add(min(max(p.MaxAge, time.Hour), 24*time.Hour))
		next.valid = now.Add(p.MaxAge)
	case ok && e.policy != nil && now.Before(e.valid):
		// A cached policy binds until its max_age runs out, even when the
		// record or the policy host cannot be reached (RFC 8461 §5.1); an
		// attacker blocking the fetch must not downgrade TLS.
		next.policy, next.valid = e.policy, e.valid
	}
	p = next.policy
	c.mu.Lock()
	c.cache[domain] = next
	if len(c.cache) > 10000 {
		for k, v := range c.cache {
			if now.After(v.refresh) && now.After(v.valid) {
				delete(c.cache, k)
			}
		}
	}
	c.mu.Unlock()
	return p
}

func (c *STSCache) fetch(ctx context.Context, domain string) *STSPolicy {
	txts, err := c.DNS.LookupTXT(ctx, "_mta-sts."+domain)
	if err != nil {
		return nil
	}
	found := false
	for _, t := range txts {
		if strings.HasPrefix(strings.TrimSpace(t), "v=STSv1") {
			found = true
		}
	}
	if !found {
		return nil
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://mta-sts."+domain+"/.well-known/mta-sts.txt", nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	return ParseSTSPolicy(resp.Body)
}
