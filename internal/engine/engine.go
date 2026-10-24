// Package engine assembles lfr: it opens the data directory, wires the
// services together, runs the SMTP and HTTP servers, the delivery queue and
// the scheduler, and answers the CLI over the control socket.
package engine

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"modernc.org/sqlite"

	"github.com/Anonyfox/lfr/internal/addr"
	"github.com/Anonyfox/lfr/internal/blob"
	"github.com/Anonyfox/lfr/internal/certs"
	"github.com/Anonyfox/lfr/internal/config"
	"github.com/Anonyfox/lfr/internal/ctl"
	"github.com/Anonyfox/lfr/internal/dnsbl"
	"github.com/Anonyfox/lfr/internal/dnsx"
	"github.com/Anonyfox/lfr/internal/domains"
	"github.com/Anonyfox/lfr/internal/events"
	"github.com/Anonyfox/lfr/internal/inbound"
	"github.com/Anonyfox/lfr/internal/issues"
	"github.com/Anonyfox/lfr/internal/lists"
	"github.com/Anonyfox/lfr/internal/mailbox"
	"github.com/Anonyfox/lfr/internal/outbound"
	"github.com/Anonyfox/lfr/internal/spam"
	"github.com/Anonyfox/lfr/internal/store"
	"github.com/Anonyfox/lfr/internal/token"
	"github.com/Anonyfox/lfr/internal/web"
)

// Paths inside the data directory.
const (
	DBFile    = "lfr.db"
	Socket    = "lfr.sock"
	BlobDir   = "msg"
	SiteDir   = "sites"
	CertDir   = "certs"
	SocketEnv = "LFR_SOCKET"
)

// Options are process-level settings of `lfr serve`.
type Options struct {
	Data      string
	SMTPAddr  string // ":25"
	HTTPAddr  string // ":80"
	HTTPSAddr string // ":443"
	Version   string
	Log       *slog.Logger
	// For tests and special setups.
	DNS        dnsx.Resolver
	Transport  outbound.Transport
	Now        func() time.Time
	SocketMode os.FileMode
}

// Engine is a running lfr.
type Engine struct {
	backupMu sync.Mutex // serializes backups and excludes blob garbage collection

	unsubLinks map[int64]time.Time // member → when an unsubscribe link last went out by mail

	Opts     Options
	DB       *store.DB
	Blobs    *blob.Store
	Config   config.Config
	Tokens   *token.Signer
	Events   *events.Bus
	Domains  *domains.Service
	Mailbox  *mailbox.Service
	Lists    *lists.Service
	Issues   *issues.Service
	Renderer *issues.Renderer
	Queue    *outbound.Queue
	Spam     *spam.Classifier
	Sites    *web.Sites
	Web      *web.Server
	Inbound  *inbound.Server
	Certs    *certs.Manager
	Ctl      *ctl.Server
	DNS      dnsx.Resolver
	Log      *slog.Logger

	started   time.Time
	mu        sync.Mutex
	listening []string
	stsCache  *outbound.STSCache
}

// Open prepares an engine on an initialized data directory.
func Open(ctx context.Context, o Options) (*Engine, error) {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.SocketMode == 0 {
		o.SocketMode = 0o660
	}
	if _, err := os.Stat(filepath.Join(o.Data, DBFile)); err != nil {
		return nil, fmt.Errorf("no lfr data in %s: run lfr setup first", o.Data)
	}
	db, err := store.Open(ctx, filepath.Join(o.Data, DBFile))
	if err != nil {
		return nil, err
	}
	blobs, err := blob.Open(filepath.Join(o.Data, BlobDir))
	if err != nil {
		db.Close()
		return nil, err
	}
	e := &Engine{Opts: o, DB: db, Blobs: blobs, Config: config.Config{DB: db}, Log: o.Log}
	secret, err := e.Config.Secret(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}
	e.Tokens = token.New(secret)
	e.DNS = o.DNS
	if e.DNS == nil {
		e.DNS = dnsx.System("", 8*time.Second)
	}
	now := o.Now
	e.Events = &events.Bus{DB: db, Now: now}
	e.Domains = &domains.Service{DB: db, Now: now}
	e.Mailbox = &mailbox.Service{DB: db, Blobs: blobs, Now: now}
	e.Lists = &lists.Service{DB: db, Events: e.Events, Now: now}
	e.Spam = &spam.Classifier{DB: db, Now: now}
	e.Sites = &web.Sites{Dir: filepath.Join(o.Data, SiteDir)}
	e.Queue = &outbound.Queue{
		DB: db, Domains: e.Domains, Events: e.Events, Now: now, Log: o.Log,
		Transport: e.transport,
		Rate: func(ctx context.Context) (int, int) {
			return e.Config.Int(ctx, "relay.rate"), e.Config.Int(ctx, "relay.connections")
		},
		Bounce: func(r outbound.Row, domain string) string {
			return "bounce+" + e.Tokens.Make(token.Bounce, r.ID) + "@" + domain
		},
	}
	e.Issues = &issues.Service{DB: db, Lists: e.Lists, Events: e.Events, Queue: e.Queue, Blobs: blobs, Now: now}
	e.Renderer = &issues.Renderer{
		Tokens: e.Tokens, Now: now,
		ListURL: e.ListURL, BaseURL: e.BaseURL,
		Layout: func(l *lists.List) *template.Template { return e.Sites.Email(l.Slug) },
		Media: func(id int64) map[string]issues.Media {
			m, _ := e.Issues.Media(context.Background(), id)
			return m
		},
	}
	e.Queue.Builders = map[string]outbound.Builder{
		outbound.KindIssue:   e.Issues.Builder(e.Renderer),
		outbound.KindConfirm: e.buildConfirm,
		outbound.KindMail: func(ctx context.Context, r outbound.Row) ([]byte, error) {
			raw, err := e.Blobs.Get(r.Blob)
			if err != nil {
				return nil, fmt.Errorf("%w: message content is gone", outbound.ErrSkip)
			}
			return raw, nil
		},
	}
	e.Queue.Hooks = outbound.Hooks{Failed: e.onFailed, GaveUp: e.onGiveUp}
	e.Certs = &certs.Manager{
		Dir:      filepath.Join(o.Data, CertDir),
		Email:    func(ctx context.Context) string { v, _ := e.Config.Get(ctx, "acme.email"); return v },
		DirURL:   func(ctx context.Context) string { v, _ := e.Config.Get(ctx, "acme.directory"); return v },
		Allowed:  e.allowedHost,
		Hostname: e.Hostname,
	}
	e.Web = &web.Server{
		DB: db, Lists: e.Lists, Issues: e.Issues, Renderer: e.Renderer, Tokens: e.Tokens, Events: e.Events,
		Config: e.Config, Sites: e.Sites, Now: now, Log: o.Log,
		MailHost: e.Hostname, MTASTS: e.mtaSTS, Signup: e.Signup, Feedback: e.onWebhook, SNSTopic: e.snsTopic,
	}
	e.Inbound = &inbound.Server{
		Hostname: e.Hostname, Config: e.Config, DNS: e.DNS, DNSBL: &dnsbl.Checker{DNS: e.DNS}, Log: o.Log, Now: now,
		Accept: e.accept, System: system, Deliver: e.deliver, KnownContact: e.knownContact,
		Local:    func(ctx context.Context, d string) bool { _, err := e.Domains.Get(ctx, d); return err == nil },
		Classify: func(ctx context.Context, in spam.Input) spam.Verdict { return e.Spam.Classify(ctx, in) },
	}
	e.Ctl = &ctl.Server{Log: o.Log, Classify: classify}
	e.register()
	return e, nil
}

// Close releases the database.
func (e *Engine) Close() error { return e.DB.Close() }

func classify(err error) string {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return ctl.CodeNotFound
	case errors.Is(err, domains.ErrExists), errors.Is(err, mailbox.ErrExists), strings.Contains(err.Error(), "already exists"):
		return ctl.CodeConflict
	case internalError(err):
		return ctl.CodeInternal
	}
	// Everything else is a domain package refusing the request as asked,
	// whatever its wording: the caller can change the input.
	return ctl.CodeInvalid
}

// internalError reports failures of the machinery rather than of the
// request: the database, the disk, the network, a cancelled operation.
func internalError(err error) bool {
	var pathErr *fs.PathError
	var sqlErr *sqlite.Error
	var netErr net.Error
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, sql.ErrConnDone) || errors.Is(err, sql.ErrTxDone) ||
		errors.As(err, &pathErr) || errors.As(err, &sqlErr) || errors.As(err, &netErr)
}

// Hostname is the mail host name.
func (e *Engine) Hostname(ctx context.Context) string {
	h, _ := e.Config.Get(ctx, "hostname")
	return h
}

// ListURL is the base URL of a list's site.
func (e *Engine) ListURL(l *lists.List) string {
	if l.Host != "" {
		return "https://" + l.Host
	}
	return "https://" + e.Hostname(context.Background()) + "/lists/" + l.Slug
}

// BaseURL is where a list's token links (confirm, unsubscribe, tracking) live.
func (e *Engine) BaseURL(l *lists.List) string {
	if l.Host != "" {
		return "https://" + l.Host
	}
	return "https://" + e.Hostname(context.Background())
}

func (e *Engine) allowedHost(ctx context.Context, host string) bool {
	host = strings.ToLower(host)
	if host == "" {
		return false
	}
	if host == e.Hostname(ctx) {
		return true
	}
	if _, err := e.Lists.ByHost(ctx, host); err == nil {
		return true
	}
	if d, ok := strings.CutPrefix(host, "mta-sts."); ok {
		if _, err := e.Domains.Get(ctx, d); err == nil {
			return true
		}
	}
	return false
}

func (e *Engine) mtaSTS(ctx context.Context, domain string) (string, bool) {
	if _, err := e.Domains.Get(ctx, domain); err != nil {
		return "", false
	}
	return domains.MTASTSPolicy(e.Instance(ctx)), true
}

// Instance is what DNS records point at.
func (e *Engine) Instance(ctx context.Context) domains.Instance {
	ip4, _ := e.Config.Get(ctx, "ipv4")
	ip6, _ := e.Config.Get(ctx, "ipv6")
	spf, _ := e.Config.Get(ctx, "relay.spf")
	mode, _ := e.Config.Get(ctx, "mta_sts.mode")
	return domains.Instance{Hostname: e.Hostname(ctx), IPv4: ip4, IPv6: ip6, RelaySPF: spf, STSMode: mode}
}

// transport builds the outbound transport from the settings.
func (e *Engine) transport(ctx context.Context) (outbound.Transport, string, error) {
	if e.Opts.Transport != nil {
		return localFirst{e.Opts.Transport, e}, "test", nil
	}
	url, _ := e.Config.Get(ctx, "relay.url")
	pass, _ := e.Config.Get(ctx, "relay.password")
	host := e.Hostname(ctx)
	if url == "" {
		ip6, _ := e.Config.Get(ctx, "ipv6")
		return localFirst{&outbound.Direct{LocalName: host, DNS: e.DNS, STS: e.sts(), IPv6: ip6 != ""}, e}, "direct|" + host + "|" + ip6, nil
	}
	r, err := outbound.ParseRelay(url, pass, host)
	if err != nil {
		return nil, "", err
	}
	return localFirst{r, e}, url + "|" + pass + "|" + host, nil
}

func (e *Engine) sts() *outbound.STSCache {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stsCache == nil {
		e.stsCache = &outbound.STSCache{DNS: e.DNS}
	}
	return e.stsCache
}

func system(rcpt string) bool {
	local := strings.ToLower(rcpt)
	if i := strings.LastIndexByte(local, '@'); i >= 0 {
		local = local[:i]
	}
	return strings.HasPrefix(local, "bounce+") || strings.HasPrefix(local, "unsub+") || local == domains.DMARCReports || local == domains.TLSReports
}

// Run serves until ctx ends.
func (e *Engine) Run(ctx context.Context) error {
	e.started = e.Opts.Now()
	if e.Hostname(ctx) == "" {
		return errors.New("no hostname configured: lfr config hostname mail.example.com")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sock := socketPath(e.Opts.Data)
	cl, err := ctl.Listen(sock, e.Opts.SocketMode)
	if err != nil {
		return fmt.Errorf("control socket: %w", err)
	}
	var smtpL, httpL, httpsL net.Listener
	listen := func(addr, what string) (net.Listener, error) {
		if addr == "" || addr == "off" {
			return nil, nil
		}
		l, err := net.Listen("tcp", addr)
		if err != nil {
			hint := ""
			if strings.Contains(err.Error(), "permission denied") {
				hint = " (ports below 1024 need root or CAP_NET_BIND_SERVICE; the systemd unit from lfr setup grants it)"
			}
			return nil, fmt.Errorf("%s on %s: %w%s", what, addr, err, hint)
		}
		e.mu.Lock()
		e.listening = append(e.listening, what+" "+l.Addr().String())
		e.mu.Unlock()
		return l, nil
	}
	if smtpL, err = listen(e.Opts.SMTPAddr, "smtp"); err != nil {
		cl.Close()
		return err
	}
	if httpL, err = listen(e.Opts.HTTPAddr, "http"); err != nil {
		cl.Close()
		return err
	}
	if httpsL, err = listen(e.Opts.HTTPSAddr, "https"); err != nil {
		cl.Close()
		return err
	}
	tlsConf := e.Certs.TLSConfig()
	e.Inbound.TLS = &tls.Config{GetCertificate: e.Certs.SMTPCertificate, MinVersion: tls.VersionTLS12}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	run := func(name string, fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil && ctx.Err() == nil {
				errs <- fmt.Errorf("%s: %w", name, err)
			}
		}()
	}
	run("control", func() error { return e.Ctl.Serve(ctx, cl) })
	run("queue", func() error { return e.Queue.Run(ctx) })
	run("scheduler", func() error { return e.schedule(ctx) })
	if smtpL != nil {
		run("smtp", func() error { return e.Inbound.Serve(ctx, smtpL) })
	}
	var servers []*http.Server
	newHTTP := func(h http.Handler) *http.Server {
		s := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second,
			IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 64 << 10, ErrorLog: slog.NewLogLogger(e.Log.Handler(), slog.LevelDebug)}
		servers = append(servers, s)
		return s
	}
	if httpL != nil {
		s := newHTTP(e.Certs.Challenge(http.HandlerFunc(e.plainHTTP)))
		run("http", func() error { return ignoreClosed(s.Serve(httpL)) })
	}
	if httpsL != nil {
		s := newHTTP(e.Certs.Challenge(e.Web))
		s.TLSConfig = tlsConf
		run("https", func() error { return ignoreClosed(s.Serve(tls.NewListener(httpsL, tlsConf))) })
	}
	e.Log.Info("lfr running", "version", e.Opts.Version, "hostname", e.Hostname(ctx), "listening", strings.Join(e.listening, ", "), "socket", sock)
	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errs:
		e.Log.Error("stopping", "err", runErr)
	}
	cancel()
	sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
	for _, s := range servers {
		s.Shutdown(sctx)
	}
	scancel()
	wg.Wait()
	os.Remove(sock)
	return runErr
}

func ignoreClosed(err error) error {
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

// plainHTTP serves port 80: Cloudflare's flexible mode talks HTTP to the
// origin, so its requests are served; everyone else is sent to HTTPS. With
// HTTPS off (a TLS proxy in front, or a local trial) nothing is redirected.
func (e *Engine) plainHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" || viaCloudflare(r) || e.Opts.HTTPSAddr == "off" {
		e.Web.ServeHTTP(w, r)
		return
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if _, err := addr.Domain(host); err != nil || !e.allowedHost(r.Context(), host) {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusPermanentRedirect)
}

func viaCloudflare(r *http.Request) bool {
	return r.Header.Get("CF-Connecting-IP") != "" && r.Header.Get("CF-Ray") != "" && web.FromCloudflare(r.RemoteAddr)
}

// socketPath is where the control socket lives; LFR_SOCKET overrides it.
func socketPath(data string) string {
	if p := os.Getenv(SocketEnv); p != "" {
		return p
	}
	return filepath.Join(data, Socket)
}

// SocketPath is exported for the CLI.
func SocketPath(data string) string { return socketPath(data) }

// schedule runs the issue scheduler often and housekeeping hourly.
func (e *Engine) schedule(ctx context.Context) error {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	house := time.NewTicker(time.Hour)
	defer house.Stop()
	e.housekeep(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			if err := e.Issues.Tick(ctx); err != nil && ctx.Err() == nil {
				e.Log.Error("issue scheduler", "err", err)
			}
		case <-house.C:
			e.housekeep(ctx)
		}
	}
}
