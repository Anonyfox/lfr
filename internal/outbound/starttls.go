package outbound

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/textproto"
	"strings"
	"time"
)

// upgradeConn runs greeting, EHLO and STARTTLS on conn itself, so lfr
// introduces itself with its own host name before TLS (go-smtp's own STARTTLS
// helper would say "localhost"). It returns a connection that replays a
// greeting, ready for smtp.NewClient, and whether TLS is active. Without
// STARTTLS on offer the plain connection is returned.
func upgradeConn(ctx context.Context, conn net.Conn, localName string, cfg *tls.Config) (net.Conn, bool, error) {
	deadline := time.Now().Add(5 * time.Minute) // RFC 5321 §4.5.3.2.1
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	defer conn.SetDeadline(time.Time{})
	tp := textproto.NewConn(conn)
	if _, _, err := tp.ReadResponse(220); err != nil {
		return nil, false, handshake(err)
	}
	if err := tp.PrintfLine("EHLO %s", localName); err != nil {
		return nil, false, err
	}
	_, msg, err := tp.ReadResponse(250)
	if err != nil {
		// An old server that does not know EHLO gets HELO (RFC 5321 §3.2),
		// and with it no STARTTLS.
		if te, ok := err.(*textproto.Error); ok && (te.Code == 500 || te.Code == 502) {
			if err := tp.PrintfLine("HELO %s", localName); err != nil {
				return nil, false, err
			}
			if _, _, err := tp.ReadResponse(250); err != nil {
				return nil, false, handshake(err)
			}
			return replay(conn, conn), false, nil
		}
		return nil, false, handshake(err)
	}
	offered := false
	for _, line := range strings.Split(msg, "\n") {
		if strings.EqualFold(strings.TrimSpace(line), "STARTTLS") {
			offered = true
		}
	}
	if !offered {
		return replay(conn, conn), false, nil
	}
	if err := tp.PrintfLine("STARTTLS"); err != nil {
		return nil, false, err
	}
	if _, _, err := tp.ReadResponse(220); err != nil {
		return nil, false, handshake(err)
	}
	tc := tls.Client(conn, cfg)
	if err := tc.HandshakeContext(ctx); err != nil {
		return nil, false, err
	}
	return replay(tc, tc), true, nil
}

// handshake classifies a refusal before MAIL FROM: it is about this host,
// so the next MX is still worth a try.
func handshake(err error) error {
	if te, ok := err.(*textproto.Error); ok {
		return &Failure{Code: te.Code, Temporary: te.Code < 500, Message: te.Msg, Handshake: true}
	}
	return err
}

// replayConn makes the client library see a fresh greeting.
type replayConn struct {
	net.Conn
	r io.Reader
}

func (c *replayConn) Read(p []byte) (int, error) { return c.r.Read(p) }

func replay(conn net.Conn, r io.Reader) net.Conn {
	return &replayConn{Conn: conn, r: io.MultiReader(strings.NewReader("220 ready\r\n"), r)}
}
