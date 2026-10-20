// Package addr normalizes email addresses, domain names and host names, the
// one place deciding what lfr accepts as an address.
package addr

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"golang.org/x/net/idna"
)

var idnaProfile = idna.New(idna.MapForLookup(), idna.BidiRule(), idna.StrictDomainName(true), idna.Transitional(false))

// Domain returns the lowercase ASCII (punycode) form of a domain or host name.
func Domain(s string) (string, error) {
	s = strings.TrimSuffix(strings.TrimSpace(s), ".")
	if s == "" {
		return "", Invalid("empty domain")
	}
	a, err := idnaProfile.ToASCII(s)
	if err != nil {
		return "", Invalid("invalid domain %q: %v", s, err)
	}
	a = strings.ToLower(a)
	if len(a) > 253 || !strings.Contains(a, ".") {
		return "", Invalid("invalid domain %q: needs a dot and at most 253 characters", s)
	}
	for _, label := range strings.Split(a, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", Invalid("invalid domain %q", s)
		}
	}
	return a, nil
}

// Local validates and lowercases a local part as lfr hands them out: letters,
// digits and . _ - + only, no leading or trailing dot.
func Local(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > 64 || s[0] == '.' || s[len(s)-1] == '.' || strings.Contains(s, "..") {
		return "", Invalid("invalid local part %q", s)
	}
	for _, c := range s {
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '.' || c == '_' || c == '-' || c == '+') {
			return "", Invalid("invalid local part %q: use letters, digits, . _ - +", s)
		}
	}
	return s, nil
}

// Address is a normalized mailbox address.
type Address struct{ Local, Domain string }

func (a Address) String() string { return a.Local + "@" + a.Domain }

// Parse accepts a bare address or "Name <addr>" and normalizes it. The local
// part keeps its characters (other servers' addresses may use anything RFC
// 5321 allows) but is lowercased, which every real mail system treats alike.
func Parse(s string) (Address, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Address{}, Invalid("empty address")
	}
	if strings.ContainsAny(s, "\r\n") {
		return Address{}, Invalid("invalid address %q", s)
	}
	m, err := mail.ParseAddress(s)
	if err != nil {
		return Address{}, Invalid("invalid address %q", s)
	}
	at := strings.LastIndexByte(m.Address, '@')
	if at <= 0 || at == len(m.Address)-1 {
		return Address{}, Invalid("invalid address %q", s)
	}
	local := strings.ToLower(m.Address[:at])
	if len(local) > 64 {
		return Address{}, Invalid("invalid address %q: local part too long", s)
	}
	d, err := Domain(m.Address[at+1:])
	if err != nil {
		return Address{}, Invalid("invalid address %q: %v", s, err)
	}
	return Address{Local: local, Domain: d}, nil
}

// Email parses and returns the normalized address string.
func Email(s string) (string, error) {
	a, err := Parse(s)
	if err != nil {
		return "", err
	}
	return a.String(), nil
}

// Base strips a +tag from a local part: "jo+news" → "jo".
func Base(local string) string {
	if i := strings.IndexByte(local, '+'); i > 0 {
		return local[:i]
	}
	return local
}

// DomainOf returns the domain of an address, or "" when it has none.
func DomainOf(email string) string {
	if i := strings.LastIndexByte(email, '@'); i >= 0 {
		return strings.ToLower(email[i+1:])
	}
	return ""
}

// ErrInvalid matches every error about a malformed address or domain, so
// callers can tell bad input from failures without reading the message.
var ErrInvalid = errors.New("invalid address")

type invalidError struct{ msg string }

func (e invalidError) Error() string        { return e.msg }
func (e invalidError) Is(target error) bool { return target == ErrInvalid }

// Invalid is an error about a malformed address that matches ErrInvalid.
func Invalid(format string, args ...any) error {
	return invalidError{fmt.Sprintf(format, args...)}
}
