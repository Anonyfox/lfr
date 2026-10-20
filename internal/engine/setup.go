package engine

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfr/internal/addr"
	"github.com/Anonyfox/lfr/internal/config"
	"github.com/Anonyfox/lfr/internal/store"
)

// SetupOptions are the first settings of a data directory.
type SetupOptions struct {
	Data      string
	Hostname  string
	IPv4      string // detected when empty
	IPv6      string
	ACMEEmail string
	Now       func() int64
}

// SetupResult reports what Setup did.
type SetupResult struct {
	Data     string `json:"data"`
	Hostname string `json:"hostname"`
	IPv4     string `json:"ipv4,omitempty"`
	IPv6     string `json:"ipv6,omitempty"`
	Existing bool   `json:"existing"`
}

// Setup creates or updates a data directory. Running it again keeps all data
// and only changes the settings given.
func Setup(ctx context.Context, o SetupOptions) (SetupResult, error) {
	res := SetupResult{Data: o.Data}
	if o.Data == "" {
		return res, fmt.Errorf("no data directory")
	}
	if err := os.MkdirAll(o.Data, 0o750); err != nil {
		return res, err
	}
	for _, d := range []string{BlobDir, SiteDir, CertDir} {
		if err := os.MkdirAll(filepath.Join(o.Data, d), 0o750); err != nil {
			return res, err
		}
	}
	_, err := os.Stat(filepath.Join(o.Data, DBFile))
	res.Existing = err == nil
	db, err := store.Open(ctx, filepath.Join(o.Data, DBFile))
	if err != nil {
		return res, err
	}
	defer db.Close()
	cfg := config.Config{DB: db}
	if _, err := cfg.Secret(ctx); err != nil {
		return res, err
	}
	if o.Hostname != "" {
		h, err := addr.Domain(o.Hostname)
		if err != nil {
			return res, fmt.Errorf("hostname: %v", err)
		}
		if strings.Count(h, ".") < 1 {
			return res, fmt.Errorf("hostname %s: use a full name like mail.example.com", h)
		}
		cfg.Set(ctx, "hostname", h)
	}
	if o.IPv4 == "" {
		if cur, _ := cfg.Get(ctx, "ipv4"); cur == "" {
			o.IPv4 = DetectIPv4()
		}
	}
	if o.IPv4 != "" {
		if ip := net.ParseIP(o.IPv4); ip == nil || ip.To4() == nil {
			return res, fmt.Errorf("ipv4 %q is not an IPv4 address", o.IPv4)
		}
		cfg.Set(ctx, "ipv4", o.IPv4)
	}
	if o.IPv6 != "" {
		if ip := net.ParseIP(o.IPv6); ip == nil || ip.To4() != nil {
			return res, fmt.Errorf("ipv6 %q is not an IPv6 address", o.IPv6)
		}
		cfg.Set(ctx, "ipv6", o.IPv6)
	}
	if o.ACMEEmail != "" {
		if _, err := addr.Email(o.ACMEEmail); err != nil {
			return res, fmt.Errorf("acme email: %v", err)
		}
		cfg.Set(ctx, "acme.email", o.ACMEEmail)
	}
	if v, _ := db.Setting(ctx, "initialized", ""); v == "" && o.Now != nil {
		db.SetSetting(ctx, "initialized", strconv.FormatInt(o.Now(), 10))
	}
	res.Hostname, _ = cfg.Get(ctx, "hostname")
	res.IPv4, _ = cfg.Get(ctx, "ipv4")
	res.IPv6, _ = cfg.Get(ctx, "ipv6")
	return res, nil
}

// DetectIPv4 finds the first public IPv4 address on a local interface, as
// cloud servers like DigitalOcean droplets have.
func DetectIPv4() string {
	ifaces, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range ifaces {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := n.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip[0] == 100 && ip[1]&0xc0 == 64 {
			continue
		}
		return ip.String()
	}
	return ""
}

// SystemdUnit is the service definition `lfr setup --systemd` installs.
func SystemdUnit(binary, data, user string) string {
	return `[Unit]
Description=lfr mail and newsletter server
After=network-online.target
Wants=network-online.target

[Service]
User=` + user + `
Group=` + user + `
ExecStart=` + binary + ` serve --data ` + data + `
Restart=always
RestartSec=5
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
ReadWritePaths=` + data + `
UMask=0007
Environment=GOMEMLIMIT=192MiB
MemoryMax=400M
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`
}
