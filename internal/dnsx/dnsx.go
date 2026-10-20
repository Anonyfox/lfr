// Package dnsx is the DNS interface every lookup in lfr goes through, so
// tests run against a fake zone and production gets timeouts and an optional
// fixed resolver.
package dnsx

import (
	"context"
	"errors"
	"net"
	"time"
)

// Resolver is the subset of net.Resolver lfr uses.
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
	LookupAddr(ctx context.Context, addr string) ([]string, error)
	LookupCNAME(ctx context.Context, host string) (string, error)
}

// System returns the system resolver with a per-query timeout. When server
// is set ("1.1.1.1:53"), queries go there instead of /etc/resolv.conf.
func System(server string, timeout time.Duration) Resolver {
	r := &net.Resolver{PreferGo: true}
	if server != "" {
		d := net.Dialer{Timeout: timeout}
		r.Dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return d.DialContext(ctx, network, server)
		}
	}
	return timed{r: r, timeout: timeout}
}

type timed struct {
	r       *net.Resolver
	timeout time.Duration
}

func (t timed) ctx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, t.timeout)
}

func (t timed) LookupTXT(ctx context.Context, name string) ([]string, error) {
	ctx, cancel := t.ctx(ctx)
	defer cancel()
	return t.r.LookupTXT(ctx, name)
}

func (t timed) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	ctx, cancel := t.ctx(ctx)
	defer cancel()
	return t.r.LookupMX(ctx, name)
}

func (t timed) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	ctx, cancel := t.ctx(ctx)
	defer cancel()
	return t.r.LookupIPAddr(ctx, host)
}

func (t timed) LookupAddr(ctx context.Context, addr string) ([]string, error) {
	ctx, cancel := t.ctx(ctx)
	defer cancel()
	return t.r.LookupAddr(ctx, addr)
}

func (t timed) LookupCNAME(ctx context.Context, host string) (string, error) {
	ctx, cancel := t.ctx(ctx)
	defer cancel()
	return t.r.LookupCNAME(ctx, host)
}

// NotFound reports whether err means the name or record does not exist, as
// opposed to a temporary failure.
func NotFound(err error) bool {
	var de *net.DNSError
	return errors.As(err, &de) && de.IsNotFound
}
