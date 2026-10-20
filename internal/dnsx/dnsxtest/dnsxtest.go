// Package dnsxtest is an in-memory DNS zone for tests.
package dnsxtest

import (
	"context"
	"net"
	"strings"
	"sync"
)

// Fake is an in-memory zone for tests. Names are matched case-insensitively
// without trailing dots.
type Fake struct {
	mu    sync.Mutex
	TXT   map[string][]string
	MX    map[string][]*net.MX
	IP    map[string][]net.IP
	PTR   map[string][]string
	CNAME map[string]string
	Fail  map[string]bool // names that answer with a temporary error
}

// New returns an empty zone.
func New() *Fake {
	return &Fake{TXT: map[string][]string{}, MX: map[string][]*net.MX{}, IP: map[string][]net.IP{}, PTR: map[string][]string{}, CNAME: map[string]string{}, Fail: map[string]bool{}}
}

func key(s string) string { return strings.ToLower(strings.TrimSuffix(s, ".")) }

func (f *Fake) check(name string) error {
	if f.Fail[key(name)] {
		return &net.DNSError{Err: "server misbehaving", Name: name, IsTemporary: true}
	}
	return nil
}

func notFound(name string) error {
	return &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func (f *Fake) LookupTXT(_ context.Context, name string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.check(name); err != nil {
		return nil, err
	}
	if v, ok := f.TXT[key(name)]; ok {
		return v, nil
	}
	return nil, notFound(name)
}

func (f *Fake) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.check(name); err != nil {
		return nil, err
	}
	if v, ok := f.MX[key(name)]; ok {
		return v, nil
	}
	return nil, notFound(name)
}

func (f *Fake) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.check(host); err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		return []net.IPAddr{{IP: ip}}, nil
	}
	v, ok := f.IP[key(host)]
	if !ok {
		return nil, notFound(host)
	}
	out := make([]net.IPAddr, len(v))
	for i, ip := range v {
		out[i] = net.IPAddr{IP: ip}
	}
	return out, nil
}

func (f *Fake) LookupAddr(_ context.Context, addr string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.PTR[addr]; ok {
		return v, nil
	}
	return nil, notFound(addr)
}

func (f *Fake) LookupCNAME(_ context.Context, host string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.CNAME[key(host)]; ok {
		return v + ".", nil
	}
	if _, ok := f.IP[key(host)]; ok {
		return key(host) + ".", nil
	}
	return "", notFound(host)
}

// Set helpers for tests.
func (f *Fake) SetTXT(name string, v ...string) { f.mu.Lock(); f.TXT[key(name)] = v; f.mu.Unlock() }
func (f *Fake) SetIP(name string, ips ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range ips {
		f.IP[key(name)] = append(f.IP[key(name)], net.ParseIP(s))
	}
}
func (f *Fake) SetMX(name, host string, pref uint16) {
	f.mu.Lock()
	f.MX[key(name)] = append(f.MX[key(name)], &net.MX{Host: host + ".", Pref: pref})
	f.mu.Unlock()
}
func (f *Fake) SetPTR(ip string, names ...string) { f.mu.Lock(); f.PTR[ip] = names; f.mu.Unlock() }
