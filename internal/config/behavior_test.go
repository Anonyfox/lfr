package config_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/Anonyfox/lfr/internal/config"
	"github.com/Anonyfox/lfr/internal/store"
)

func TestSettingsValidateBeforePersisting(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "lfr.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := config.Config{DB: db}
	for _, tc := range []struct{ key, valid, invalid, want string }{
		{"hostname", " MAIL.Example.COM ", "bad host", "mail.example.com"},
		{"ipv4", "203.0.113.7", "127.0.0.1", "203.0.113.7"},
		{"ipv6", "2001:db8::1", "192.0.2.1", "2001:db8::1"},
		{"relay.rate", "25", "0", "25"},
		{"relay.connections", "16", "17", "16"},
		{"smtp.early_talker", "false", "yes", "false"},
		{"spam.junk", "0.7", "1.1", "0.7"},
		{"mta_sts.mode", "enforce", "off", "enforce"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			if err := c.Set(ctx, tc.key, tc.valid); err != nil {
				t.Fatal(err)
			}
			if err := c.Set(ctx, tc.key, tc.invalid); err == nil {
				t.Fatal("invalid setting accepted")
			}
			if got, err := c.Get(ctx, tc.key); err != nil || got != tc.want {
				t.Fatalf("lost valid setting: %q %v", got, err)
			}
			if err := c.Set(ctx, tc.key, ""); err != nil {
				t.Fatal(err)
			}
			k, _ := config.Lookup(tc.key)
			if got, err := c.Get(ctx, tc.key); err != nil || got != k.Default {
				t.Fatalf("reset: %q %v", got, err)
			}
		})
	}
	for _, ip := range []string{"garbage", "10.0.0.1", "0.0.0.0", "::1"} {
		if err := c.Set(ctx, "ipv4", ip); err == nil {
			t.Errorf("accepted %s", ip)
		}
	}
	if err := c.Set(ctx, "unknown", "x"); err == nil {
		t.Fatal("unknown key accepted")
	}
	if _, err := c.Get(ctx, "unknown"); err == nil {
		t.Fatal("unknown key read")
	}
	if err := c.Set(ctx, "relay.password", "private-password"); err != nil {
		t.Fatal(err)
	}
	entries, err := c.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == "relay.password" && (e.Value != "(set)" || e.Default) {
			t.Fatalf("secret exposed: %+v", e)
		}
	}
	a, err := c.Secret(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Secret(ctx)
	if err != nil || len(a) != 32 || !bytes.Equal(a, b) {
		t.Fatalf("unstable signing secret: %v", err)
	}
	// A corrupted legacy setting must not disable rate limits or spam thresholds.
	if err := db.SetSetting(ctx, "cfg.relay.rate", "broken"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(ctx, "cfg.spam.junk", "broken"); err != nil {
		t.Fatal(err)
	}
	if c.Int(ctx, "relay.rate") != 10 || c.Float(ctx, "spam.junk") != 0.6 {
		t.Fatal("corrupt settings did not fall back")
	}
	if err := c.Set(ctx, "spam.dnsbl", " a.example, ,b.example "); err != nil {
		t.Fatal(err)
	}
	if got := c.List(ctx, "spam.dnsbl"); len(got) != 2 || got[1] != "b.example" {
		t.Fatalf("DNSBL list: %v", got)
	}
}
