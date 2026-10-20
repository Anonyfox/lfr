// Package token makes and checks the signed tokens in links and addresses:
// confirm, unsubscribe, open, click and bounce. A token carries its purpose
// and a few integer IDs, authenticated with HMAC-SHA256, so handlers verify
// it without a lookup and nobody can forge or enumerate one.
package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"strings"
)

// Kind is what a token authorizes.
type Kind byte

const (
	Confirm Kind = 'c' // confirm membership: member ID
	Unsub   Kind = 'u' // unsubscribe: member ID, issue ID (0 outside issues)
	Open    Kind = 'o' // open pixel: outbound ID
	Click   Kind = 'k' // click: outbound ID, link index
	Bounce  Kind = 'b' // envelope sender (VERP): outbound ID
	Message Kind = 'm' // Message-ID: outbound ID; it identifies, never authorizes
	Form    Kind = 'f' // signup form render time: unix seconds
	Hook    Kind = 'h' // webhook URL: constant 1
)

const macLen = 10 // 80 bits: unforgeable for this purpose, short enough for URLs and local parts

// enc is lowercase base32 without padding: tokens also travel as email
// local parts (bounce+<token>@...), which mail systems may change the case of.
var enc = base32.HexEncoding.WithPadding(base32.NoPadding)

// ErrInvalid is returned for tokens that are malformed or not signed by us.
var ErrInvalid = errors.New("invalid token")

// Signer signs and verifies tokens with one secret.
type Signer struct{ key []byte }

// New returns a Signer; the secret should be 32 random bytes.
func New(secret []byte) *Signer { return &Signer{key: append([]byte(nil), secret...)} }

func (s *Signer) mac(b []byte) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write(b)
	return m.Sum(nil)[:macLen]
}

// Make encodes kind and ids into a URL- and local-part-safe token.
func (s *Signer) Make(kind Kind, ids ...int64) string {
	b := []byte{byte(kind)}
	for _, id := range ids {
		b = binary.AppendUvarint(b, uint64(id))
	}
	b = append(b, s.mac(b)...)
	return strings.ToLower(enc.EncodeToString(b))
}

// Parse checks a token of the given kind and returns its ids.
func (s *Signer) Parse(kind Kind, tok string, n int) ([]int64, error) {
	if len(tok) > 80 {
		return nil, ErrInvalid
	}
	tok = strings.ToLower(tok)
	for i := 0; i < len(tok); i++ {
		// The decoder skips line breaks; only the canonical spelling may verify.
		if c := tok[i]; !('a' <= c && c <= 'v' || '0' <= c && c <= '9') {
			return nil, ErrInvalid
		}
	}
	b, err := enc.DecodeString(strings.ToUpper(tok))
	if err != nil || len(b) < 1+macLen || Kind(b[0]) != kind {
		return nil, ErrInvalid
	}
	body, sum := b[:len(b)-macLen], b[len(b)-macLen:]
	if !hmac.Equal(sum, s.mac(body)) || strings.ToLower(enc.EncodeToString(b)) != tok {
		return nil, ErrInvalid
	}
	rest := body[1:]
	ids := make([]int64, 0, n)
	for len(rest) > 0 {
		v, k := binary.Uvarint(rest)
		if k <= 0 {
			return nil, ErrInvalid
		}
		ids = append(ids, int64(v))
		rest = rest[k:]
	}
	if len(ids) != n {
		return nil, ErrInvalid
	}
	return ids, nil
}
