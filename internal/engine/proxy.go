package engine

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// proxyListener accepts HAProxy's binary v2 preamble before TLS. Parsing is
// deferred to each HTTP connection goroutine, so a slow peer cannot hold Accept.
type proxyListener struct{ net.Listener }

func (l proxyListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		peer, ok := c.RemoteAddr().(*net.TCPAddr)
		if !ok || !(peer.IP.Equal(net.IPv4(127, 0, 0, 1)) || peer.IP.Equal(net.IPv6loopback)) {
			c.Close()
			continue
		}
		return &proxyConn{Conn: c, remote: c.RemoteAddr()}, nil
	}
}

type proxyConn struct {
	net.Conn
	once   sync.Once
	remote net.Addr
	err    error
}

func (c *proxyConn) init() {
	c.once.Do(func() {
		c.err = c.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if c.err == nil {
			c.remote, c.err = readProxyV2(c.Conn, c.remote)
		}
		if c.err != nil {
			c.Conn.Close()
		} else {
			c.err = c.Conn.SetReadDeadline(time.Time{})
		}
	})
}

func (c *proxyConn) Read(b []byte) (int, error) {
	c.init()
	if c.err != nil {
		return 0, c.err
	}
	return c.Conn.Read(b)
}

func (c *proxyConn) RemoteAddr() net.Addr { c.init(); return c.remote }

// readProxyV2 consumes exactly one bounded preamble, leaving TLS bytes unread.
// LOCAL keeps the transport peer; PROXY accepts only TCP over IPv4 or IPv6.
func readProxyV2(r io.Reader, peer net.Addr) (net.Addr, error) {
	var h [16]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return peer, err
	}
	if !bytes.Equal(h[:12], []byte("\r\n\r\n\x00\r\nQUIT\n")) || (h[12] != 0x20 && h[12] != 0x21) {
		return peer, fmt.Errorf("invalid PROXY v2 signature or command")
	}
	n := int64(binary.BigEndian.Uint16(h[14:]))
	if h[12] == 0x20 {
		_, err := io.CopyN(io.Discard, r, n)
		return peer, err
	}
	size := 0
	switch h[13] {
	case 0x11:
		size = 12
	case 0x21:
		size = 36
	default:
		return peer, fmt.Errorf("PROXY v2 requires TCP IPv4 or IPv6")
	}
	if n < int64(size) {
		return peer, fmt.Errorf("short PROXY v2 address")
	}
	var address [36]byte
	if _, err := io.ReadFull(r, address[:size]); err != nil {
		return peer, err
	}
	ipLen := 4
	if size == 36 {
		ipLen = 16
	}
	remote := &net.TCPAddr{IP: append(net.IP(nil), address[:ipLen]...), Port: int(binary.BigEndian.Uint16(address[2*ipLen:]))}
	if _, err := io.CopyN(io.Discard, r, n-int64(size)); err != nil {
		return peer, err
	}
	return remote, nil
}
