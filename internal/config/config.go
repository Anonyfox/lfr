// Package config is the typed view of the instance settings kept in the
// database. There is no config file: `lfr setup` writes the first values and
// `lfr config set` changes them, so the running server and the CLI never
// disagree about where configuration lives.
package config

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/addr"
	"github.com/Anonyfox/lfr/internal/store"
)

// Key describes one setting an operator may change.
type Key struct {
	Name, Default, Help string
	Secret              bool                // never printed back
	Check               func(string) error  // validates a new value
	Normalize           func(string) string // optional canonical form
}

func isInt(min, max int) func(string) error {
	return func(s string) error {
		n, err := strconv.Atoi(s)
		if err != nil || n < min || n > max {
			return fmt.Errorf("want a whole number from %d to %d", min, max)
		}
		return nil
	}
}

func oneOf(values ...string) func(string) error {
	return func(s string) error {
		for _, v := range values {
			if s == v {
				return nil
			}
		}
		return fmt.Errorf("want one of %s", strings.Join(values, ", "))
	}
}

func isFraction(s string) error {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 || f > 1 {
		return fmt.Errorf("want a number above 0 and at most 1, like 0.6")
	}
	return nil
}

func isIP(version int) func(string) error {
	return func(s string) error {
		if s == "" {
			return nil
		}
		ip := net.ParseIP(s)
		if ip == nil || (version == 4) != (ip.To4() != nil) {
			return fmt.Errorf("want a public IPv%d address", version)
		}
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() {
			return fmt.Errorf("%s is not public; DNS records point the world at it", s)
		}
		return nil
	}
}

func isBool(s string) error {
	if s != "true" && s != "false" {
		return fmt.Errorf("want true or false")
	}
	return nil
}

func isHost(s string) error {
	if s == "" {
		return nil
	}
	_, err := addr.Domain(s)
	return err
}

// Keys are every operator-facing setting, in display order.
var Keys = []Key{
	{Name: "hostname", Help: "Mail host name: MX target, HELO name and PTR of the server (e.g. mail.example.com).", Check: isHost},
	{Name: "ipv4", Check: isIP(4), Help: "Public IPv4 address of the server, used in DNS records."},
	{Name: "ipv6", Check: isIP(6), Help: "Public IPv6 address of the server, used in DNS records when set."},
	{Name: "acme.email", Help: "Contact address for Let's Encrypt (expiry notices)."},
	{Name: "acme.directory", Default: "https://acme-v02.api.letsencrypt.org/directory", Help: "ACME directory URL."},
	{Name: "mta_sts.mode", Default: "testing", Check: oneOf("testing", "enforce"), Help: "MTA-STS mode published for every domain: testing only asks senders for TLS reports; enforce makes them refuse to deliver without valid TLS. Switch once lfr reports shows clean TLS reports for a few weeks."},
	{Name: "relay.url", Help: "Outbound relay, e.g. smtps://user@email-smtp.eu-central-1.amazonaws.com:2465 or smtp://user@mail.smtp2go.com:2525 (STARTTLS). Empty: deliver directly to recipients' MX on port 25."},
	{Name: "relay.password", Secret: true, Help: "Password for the relay user."},
	{Name: "relay.spf", Help: "SPF mechanism of the relay, e.g. include:amazonses.com; added to every domain's SPF record."},
	{Name: "relay.sns_topic", Help: "Amazon SNS topic whose notifications the ses webhook accepts; set by the first confirmed subscription. Clear it to subscribe a new topic."},
	{Name: "relay.rate", Default: "10", Check: isInt(1, 1000), Help: "Maximum messages per second handed to the relay or sent directly."},
	{Name: "relay.connections", Default: "2", Check: isInt(1, 16), Help: "Parallel outbound connections."},
	{Name: "smtp.max_size", Default: "20971520", Check: isInt(1<<20, 1<<30), Help: "Largest accepted inbound message in bytes."},
	{Name: "smtp.max_conns", Default: "50", Check: isInt(1, 10000), Help: "Concurrent inbound SMTP connections."},
	{Name: "smtp.max_conns_per_ip", Default: "5", Check: isInt(1, 1000), Help: "Concurrent inbound SMTP connections per IP address."},
	{Name: "smtp.rate_per_ip", Default: "30", Check: isInt(1, 100000), Help: "Inbound SMTP connections per IP address per hour."},
	{Name: "smtp.early_talker", Default: "true", Check: isBool, Help: "Drop clients that talk before the greeting (a strong spambot signal); delays each connection by 0.5 s."},
	{Name: "spam.dnsbl", Default: "zen.spamhaus.org,bl.spamcop.net,psbl.surriel.com", Help: "DNS blocklists for connecting IPs, comma separated. Spamhaus needs a DQS key when your resolver is a large public one (use <key>.zen.dq.spamhaus.net)."},
	{Name: "spam.junk", Default: "0.6", Check: isFraction, Help: "Spam probability from which mail goes to junk."},
	{Name: "spam.reject", Default: "0.98", Check: isFraction, Help: "Spam probability from which mail is refused during SMTP."},
	{Name: "web.signup_per_ip", Default: "10", Check: isInt(1, 100000), Help: "Signups accepted per IP address per hour."},
	{Name: "web.confirm_per_list", Default: "200", Check: isInt(1, 1000000), Help: "Confirmation mails per list per hour; protects against list bombing."},
	{Name: "retention.trash_days", Default: "30", Check: isInt(1, 3650), Help: "Days before trash is purged."},
	{Name: "retention.junk_days", Default: "30", Check: isInt(1, 3650), Help: "Days before junk is purged."},
	{Name: "retention.unsubscribed_days", Default: "1095", Check: isInt(30, 36500), Help: "Days after unsubscribe before member IP addresses and browser details are cleared; suppression and consent records stay."},
	{Name: "retention.events_days", Default: "400", Check: isInt(30, 36500), Help: "Days opens, clicks and delivery events are kept."},
}

// Lookup finds a key by name.
func Lookup(name string) (Key, bool) {
	for _, k := range Keys {
		if k.Name == name {
			return k, true
		}
	}
	return Key{}, false
}

// Names lists every key name, sorted.
func Names() []string {
	var out []string
	for _, k := range Keys {
		out = append(out, k.Name)
	}
	sort.Strings(out)
	return out
}

const prefix = "cfg."

// Config reads and writes settings.
type Config struct{ DB *store.DB }

// Get returns a setting or its default.
func (c Config) Get(ctx context.Context, name string) (string, error) {
	k, ok := Lookup(name)
	if !ok {
		return "", fmt.Errorf("unknown setting %q", name)
	}
	return c.DB.Setting(ctx, prefix+name, k.Default)
}

// Set validates and stores a setting; an empty value restores the default.
func (c Config) Set(ctx context.Context, name, value string) error {
	k, ok := Lookup(name)
	if !ok {
		return fmt.Errorf("unknown setting %q", name)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return c.DB.DeleteSetting(ctx, prefix+name)
	}
	if k.Check != nil {
		if err := k.Check(value); err != nil {
			return fmt.Errorf("%s: %v", name, err)
		}
	}
	if k.Normalize != nil {
		value = k.Normalize(value)
	}
	if name == "hostname" {
		value, _ = addr.Domain(value)
	}
	return c.DB.SetSetting(ctx, prefix+name, value)
}

// Int reads a numeric setting, falling back to its default when unreadable.
func (c Config) Int(ctx context.Context, name string) int {
	v, _ := c.Get(ctx, name)
	n, err := strconv.Atoi(v)
	if err != nil {
		k, _ := Lookup(name)
		n, _ = strconv.Atoi(k.Default)
	}
	return n
}

// Float reads a decimal setting, falling back to its default.
func (c Config) Float(ctx context.Context, name string) float64 {
	v, _ := c.Get(ctx, name)
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		k, _ := Lookup(name)
		f, _ = strconv.ParseFloat(k.Default, 64)
	}
	return f
}

// Bool reads a boolean setting.
func (c Config) Bool(ctx context.Context, name string) bool {
	v, _ := c.Get(ctx, name)
	return v == "true"
}

// List reads a comma separated setting.
func (c Config) List(ctx context.Context, name string) []string {
	v, _ := c.Get(ctx, name)
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Entry is one setting as shown to the operator.
type Entry struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Default bool   `json:"default"`
	Help    string `json:"help"`
}

// All returns every setting, secrets masked.
func (c Config) All(ctx context.Context) ([]Entry, error) {
	stored, err := c.DB.Settings(ctx, prefix)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, k := range Keys {
		v, ok := stored[prefix+k.Name]
		if !ok {
			v = k.Default
		}
		if k.Secret && v != "" {
			v = "(set)"
		}
		out = append(out, Entry{Name: k.Name, Value: v, Default: !ok, Help: k.Help})
	}
	return out, nil
}

// Secret returns the instance's token secret, creating it on first use.
func (c Config) Secret(ctx context.Context) ([]byte, error) {
	v, err := c.DB.Setting(ctx, "secret", "")
	if err != nil {
		return nil, err
	}
	if v == "" {
		b := make([]byte, 32)
		rand.Read(b)
		v = hex.EncodeToString(b)
		if err := c.DB.SetSetting(ctx, "secret", v); err != nil {
			return nil, err
		}
	}
	return hex.DecodeString(v)
}

// Initialized reports when `lfr setup` completed, zero if never.
func (c Config) Initialized(ctx context.Context) time.Time {
	v, _ := c.DB.Setting(ctx, "initialized", "")
	n, _ := strconv.ParseInt(v, 10, 64)
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}
