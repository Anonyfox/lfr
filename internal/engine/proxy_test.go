package engine

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func proxyHeader(command, family byte, payload []byte) []byte {
	h := append([]byte("\r\n\r\n\x00\r\nQUIT\n"), command, family, 0, 0)
	binary.BigEndian.PutUint16(h[14:], uint16(len(payload)))
	return append(h, payload...)
}

type peerListener struct {
	net.Listener
	peer string
}
type peerConn struct {
	net.Conn
	peer net.Addr
}

func (c peerConn) RemoteAddr() net.Addr { return c.peer }
func (l peerListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	a, err := net.ResolveTCPAddr("tcp", l.peer)
	if err != nil {
		c.Close()
		return nil, err
	}
	return peerConn{c, a}, nil
}

func TestHTTPSProxyProtocol(t *testing.T) {
	ipv4 := []byte{203, 0, 113, 9, 127, 0, 0, 1, 0x30, 0x39, 1, 0xbb}
	ipv6 := append(append([]byte{}, net.ParseIP("2001:db8::9").To16()...), net.ParseIP("::1").To16()...)
	ipv6 = append(ipv6, 0x30, 0x39, 1, 0xbb)
	badSignature := proxyHeader(0x21, 0x11, ipv4)
	badSignature[0] = 'X'
	maxLocal := proxyHeader(0x20, 0, make([]byte, 65535))
	for _, tc := range []struct {
		name, peer, want string
		enabled          bool
		header           []byte
	}{
		{"off", "192.0.2.1:123", "192.0.2.1:123", false, nil},
		{"ipv4", "127.0.0.1:123", "203.0.113.9:12345", true, proxyHeader(0x21, 0x11, ipv4)},
		{"ipv6", "[::1]:123", "[2001:db8::9]:12345", true, proxyHeader(0x21, 0x21, ipv6)},
		{"tlv", "127.0.0.1:123", "203.0.113.9:12345", true, proxyHeader(0x21, 0x11, append(append([]byte{}, ipv4...), 0xe0, 0, 2, 1, 2))},
		{"local", "[::1]:123", "[::1]:123", true, proxyHeader(0x20, 0, nil)},
		{"local bounded payload", "127.0.0.1:123", "127.0.0.1:123", true, maxLocal},
		{"external", "192.0.2.1:123", "", true, proxyHeader(0x21, 0x11, ipv4)},
		{"other loopback", "127.0.0.2:123", "", true, proxyHeader(0x21, 0x11, ipv4)},
		{"missing header", "127.0.0.1:123", "", true, nil},
		{"signature", "127.0.0.1:123", "", true, badSignature},
		{"version", "127.0.0.1:123", "", true, proxyHeader(0x11, 0x11, ipv4)},
		{"command", "127.0.0.1:123", "", true, proxyHeader(0x22, 0x11, ipv4)},
		{"udp", "127.0.0.1:123", "", true, proxyHeader(0x21, 0x12, ipv4)},
		{"unix", "127.0.0.1:123", "", true, proxyHeader(0x21, 0x31, ipv4)},
		{"short ipv4", "127.0.0.1:123", "", true, proxyHeader(0x21, 0x11, ipv4[:11])},
		{"short ipv6", "127.0.0.1:123", "", true, proxyHeader(0x21, 0x21, ipv6[:35])},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, r.RemoteAddr) }))
			srv.Listener = peerListener{srv.Listener, tc.peer}
			if tc.enabled {
				srv.Listener = proxyListener{srv.Listener}
			}
			srv.StartTLS()
			defer srv.Close()
			raw, err := net.Dial("tcp", srv.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			if err := raw.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			_, _ = raw.Write(tc.header)
			c := tls.Client(raw, &tls.Config{InsecureSkipVerify: true})
			err = c.Handshake()
			if tc.want == "" {
				if err == nil {
					t.Fatal("accepted invalid or untrusted header")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(c, "GET / HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n"); err != nil {
				t.Fatal(err)
			}
			resp, err := http.ReadResponse(bufio.NewReader(c), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			b, err := io.ReadAll(resp.Body)
			if err != nil || string(b) != tc.want {
				t.Fatalf("remote=%q err=%v want=%s", b, err, tc.want)
			}
		})
	}
}

func FuzzProxyV2(f *testing.F) {
	f.Add(proxyHeader(0x21, 0x11, []byte{203, 0, 113, 9, 127, 0, 0, 1, 0, 80, 1, 187}))
	f.Add(proxyHeader(0x20, 0, nil))
	f.Add([]byte("invalid"))
	f.Fuzz(func(t *testing.T, b []byte) {
		peer := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 123}
		r := bytes.NewReader(b)
		addr, err := readProxyV2(r, peer)
		if err == nil {
			if addr == nil || !strings.Contains(addr.String(), ":") {
				t.Fatal("invalid remote address")
			}
			// The parser must leave every byte beyond its declared header unread.
			n := 16 + int(binary.BigEndian.Uint16(b[14:16]))
			if r.Len() != len(b)-n {
				t.Fatal("consumed TLS payload")
			}
		}
	})
}
