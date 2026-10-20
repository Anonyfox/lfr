// Package dnsbl looks up connecting IP addresses in DNS blocklists, telling
// a listing apart from the error codes lists return to resolvers they refuse.
package dnsbl

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfr/internal/dnsx"
)

// Checker queries lists and caches answers for an hour.
type Checker struct {
	DNS dnsx.Resolver

	mu    sync.Mutex
	cache map[string]entry
}

type entry struct {
	hits    []string
	expires time.Time
}

// Query is the name to look up for ip in list.
func Query(ip net.IP, list string) string {
	if v4 := ip.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.%d.%d.%s", v4[3], v4[2], v4[1], v4[0], list)
	}
	ip16 := ip.To16()
	var b strings.Builder
	for i := len(ip16) - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "%x.%x.", ip16[i]&0xf, ip16[i]>>4)
	}
	return b.String() + list
}

// Listed decides whether an answer means "listed". Lists answer inside
// 127.0.0.0/8; 127.255.255.x (Spamhaus) and 127.0.0.1 are errors or tests.
func Listed(addrs []net.IPAddr) bool {
	for _, a := range addrs {
		v4 := a.IP.To4()
		if v4 == nil || v4[0] != 127 {
			continue
		}
		if v4[1] == 255 && v4[2] == 255 || v4.Equal(net.IPv4(127, 0, 0, 1)) {
			continue
		}
		return true
	}
	return false
}

// Check returns the lists that list ip. Private and loopback addresses are never checked.
func (c *Checker) Check(ctx context.Context, ip net.IP, lists []string) []string {
	if len(lists) == 0 || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
		return nil
	}
	key := ip.String() + "|" + strings.Join(lists, ",")
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[string]entry{}
	}
	if e, ok := c.cache[key]; ok && time.Now().Before(e.expires) {
		c.mu.Unlock()
		return e.hits
	}
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var mu sync.Mutex
	var hits []string
	uncertain := false
	var wg sync.WaitGroup
	for _, l := range lists {
		wg.Add(1)
		go func(l string) {
			defer wg.Done()
			addrs, err := c.DNS.LookupIPAddr(ctx, Query(ip, l))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil && Listed(addrs):
				hits = append(hits, l)
			case err != nil && !dnsx.NotFound(err):
				uncertain = true // a timeout says nothing about the address
			}
		}(l)
	}
	wg.Wait()
	ttl := time.Hour
	if uncertain {
		ttl = time.Minute
	}
	c.mu.Lock()
	if len(c.cache) > 20000 {
		c.cache = map[string]entry{}
	}
	c.cache[key] = entry{hits: hits, expires: time.Now().Add(ttl)}
	c.mu.Unlock()
	return hits
}
