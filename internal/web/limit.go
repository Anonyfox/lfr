package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// limiter counts events per key in fixed windows.
type limiter struct {
	mu      sync.Mutex
	windows map[string]*window
}

type window struct {
	start time.Time
	n     int
}

// allow records one event for key and reports whether it stays within max per period.
func (l *limiter) allow(key string, max int, period time.Duration, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.windows == nil {
		l.windows = map[string]*window{}
	}
	if len(l.windows) > 50000 {
		for k, w := range l.windows {
			if now.Sub(w.start) > period {
				delete(l.windows, k)
			}
		}
	}
	w := l.windows[key]
	if w == nil || now.Sub(w.start) > period {
		w = &window{start: now}
		l.windows[key] = w
	}
	w.n++
	return w.n <= max
}

// cloudflare are Cloudflare's published edge ranges (cloudflare.com/ips).
// Only requests from them may name the client with CF-Connecting-IP.
var cloudflare = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{
		"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22", "141.101.64.0/18", "108.162.192.0/18",
		"190.93.240.0/20", "188.114.96.0/20", "197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
		"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
		"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32", "2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
	} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

func fromCloudflare(ip net.IP) bool {
	for _, n := range cloudflare {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP is the visitor's address, through the Cloudflare proxy when present.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip != nil && fromCloudflare(ip) {
		if cf := net.ParseIP(strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))); cf != nil {
			return cf.String()
		}
	}
	if ip == nil {
		return host
	}
	return ip.String()
}

// FromCloudflare reports whether a remote address ("ip:port") is a Cloudflare edge.
func FromCloudflare(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	return ip != nil && fromCloudflare(ip)
}
