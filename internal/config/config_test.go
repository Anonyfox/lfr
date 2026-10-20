package config

import "testing"

func TestChecks(t *testing.T) {
	for _, c := range []struct {
		key, value string
		ok         bool
	}{
		{"ipv4", "203.0.113.7", true}, {"ipv4", "192.168.1.2", false}, {"ipv4", "2001:db8::1", false}, {"ipv4", "", true},
		{"ipv6", "2a03:b0c0::1", true}, {"ipv6", "203.0.113.7", false},
		{"spam.junk", "0.6", true}, {"spam.junk", "abc", false}, {"spam.reject", "1.5", false},
		{"mta_sts.mode", "enforce", true}, {"mta_sts.mode", "strict", false},
		{"relay.rate", "14", true}, {"relay.rate", "0", false},
		{"smtp.early_talker", "maybe", false},
	} {
		k, found := Lookup(c.key)
		if !found {
			t.Fatalf("no key %s", c.key)
		}
		err := error(nil)
		if k.Check != nil {
			err = k.Check(c.value)
		}
		if (err == nil) != c.ok {
			t.Errorf("%s = %q: %v", c.key, c.value, err)
		}
	}
	for _, k := range Keys {
		if k.Help == "" {
			t.Errorf("%s has no help", k.Name)
		}
	}
}
