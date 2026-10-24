// Package web serves everything lfr shows to readers: each list's small
// site (latest issues, archive, feed, signup form), the confirm and
// unsubscribe pages, click and open tracking, MTA-STS policies and relay
// webhooks. There is no admin interface; operators use the CLI.
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/Anonyfox/lfr/internal/addr"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfr/internal/config"
	"github.com/Anonyfox/lfr/internal/events"
	"github.com/Anonyfox/lfr/internal/feedback"
	"github.com/Anonyfox/lfr/internal/issues"
	"github.com/Anonyfox/lfr/internal/lists"
	"github.com/Anonyfox/lfr/internal/store"
	"github.com/Anonyfox/lfr/internal/token"
)

// Server is the HTTP handler.
type Server struct {
	DB       *store.DB
	Lists    *lists.Service
	Issues   *issues.Service
	Renderer *issues.Renderer
	Tokens   *token.Signer
	Events   *events.Bus
	Config   config.Config
	Sites    *Sites
	Now      func() time.Time
	Log      *slog.Logger

	MailHost  func(ctx context.Context) string
	MTASTS    func(ctx context.Context, domain string) (string, bool) // policy text when lfr serves domain
	Signup    func(ctx context.Context, l *lists.List, email string, s lists.Signup) error
	Feedback  func(ctx context.Context, evs []feedback.Event)
	SNSClient *http.Client // fetches SNS certificates and confirms subscriptions
	// SNSTopic accepts the topic of a verified SNS message: the first one
	// confirmed is remembered, later ones must match it.
	SNSTopic func(ctx context.Context, arn string, confirm bool) bool

	snsOnce sync.Once
	sns     *feedback.SNSVerifier

	hookMu       sync.Mutex
	hookRecorded time.Time

	limits limiter
}

// HookPath is the secret webhook path relays post bounces and complaints to.
func (s *Server) HookPath(provider string) string {
	return "/hooks/" + s.Tokens.Make(token.Hook, 1) + "/" + provider
}

func host(r *http.Request) string {
	h := r.Host
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	return strings.ToLower(strings.TrimSuffix(h, "."))
}

// ServeHTTP routes by host, then path.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if v := recover(); v != nil {
			s.Log.Error("web panic", "path", r.URL.Path, "panic", v)
			http.Error(w, "internal error", 500)
		}
	}()
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("X-Frame-Options", "DENY")
	// Nothing loads from third parties: each would learn every visitor's IP.
	h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; form-action *; frame-ancestors 'none'; base-uri 'none'")
	if r.TLS != nil {
		h.Set("Strict-Transport-Security", "max-age=31536000")
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost && r.Method != http.MethodOptions {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()
	hostname := host(r)
	path := r.URL.Path

	if strings.HasPrefix(hostname, "mta-sts.") {
		if path == "/.well-known/mta-sts.txt" {
			if policy, ok := s.MTASTS(ctx, strings.TrimPrefix(hostname, "mta-sts.")); ok {
				h.Set("Content-Type", "text/plain; charset=utf-8")
				h.Set("Cache-Control", "max-age=3600")
				io.WriteString(w, policy)
				return
			}
		}
		http.NotFound(w, r)
		return
	}
	switch {
	case path == "/healthz":
		io.WriteString(w, "ok\n")
		return
	case path == "/robots.txt":
		h.Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, "User-agent: *\nDisallow: /u/\nDisallow: /confirm/\nDisallow: /c/\nDisallow: /o/\nDisallow: /hooks/\n")
		return
	case strings.HasPrefix(path, "/confirm/"):
		s.confirm(w, r, strings.TrimPrefix(path, "/confirm/"))
		return
	case strings.HasPrefix(path, "/u/"):
		s.unsubscribe(w, r, strings.TrimPrefix(path, "/u/"))
		return
	case strings.HasPrefix(path, "/o/"):
		s.open(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/o/"), ".gif"))
		return
	case strings.HasPrefix(path, "/c/"):
		s.click(w, r, strings.TrimPrefix(path, "/c/"))
		return
	case strings.HasPrefix(path, "/hooks/"):
		s.hook(w, r, strings.TrimPrefix(path, "/hooks/"))
		return
	}
	if l, err := s.Lists.ByHost(ctx, hostname); err == nil {
		s.site(w, r, l, "", path)
		return
	}
	if hostname == s.MailHost(ctx) {
		if rest, ok := strings.CutPrefix(path, "/lists/"); ok {
			slug, sub, _ := strings.Cut(rest, "/")
			if l, err := s.Lists.Get(ctx, slug); err == nil && l.Host == "" {
				s.site(w, r, l, "/lists/"+l.Slug, "/"+sub)
				return
			}
		}
		if path == "/" {
			h.Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintf(w, "%s\n\nMail server operated with lfr. Contact: postmaster@%s\n", hostname, mailDomain(hostname))
			return
		}
	}
	http.NotFound(w, r)
}

func mailDomain(h string) string {
	if i := strings.IndexByte(h, '.'); i > 0 && strings.Count(h, ".") > 1 {
		return h[i+1:]
	}
	return h
}

// page is the data every template gets.
type page struct {
	Lang, Title, Description, Canonical, Robots string
	ListName, HomeURL, FeedURL, ArchiveURL      string
	ImprintURL, PrivacyURL                      string
	SignupURL, FormToken, Consent, Cadence      string
	Headline, Pitch                             string // the front page's first line and lede
	Issues                                      []issueItem
	More                                        bool
	Issue                                       *issueItem
	Heading, Body, Action, Button               string
	Home                                        bool // the list's front page shows its name as the heading
}

type issueItem struct {
	URL, Subject, Summary, Date string
	Body                        template.HTML
}

func (s *Server) base(l *lists.List) page {
	home := s.Renderer.ListURL(l)
	headline, pitch := l.Headline, l.Pitch
	if headline == "" {
		headline = l.Name
	}
	if pitch == "" {
		pitch = l.Description
	}
	// The form posts to a path, so it works behind any proxy and on plain
	// HTTP in a local trial; the page's own origin decides the scheme.
	signup := "/subscribe"
	if u, err := url.Parse(home); err == nil && u.Path != "" {
		signup = u.Path + "/subscribe"
	}
	return page{Headline: headline, Pitch: pitch,
		Lang: l.Language, ListName: l.Name, HomeURL: home + "/", FeedURL: home + "/feed.xml", ArchiveURL: home + "/archive",
		ImprintURL: l.ImprintURL, PrivacyURL: l.PrivacyURL, SignupURL: signup,
		FormToken: s.Tokens.Make(token.Form, s.Now().Unix()), Consent: l.ConsentText(), Description: l.Description, Cadence: l.Cadence,
	}
}

func (s *Server) render(w http.ResponseWriter, l *lists.List, name string, status int, p page) {
	t, err := s.Sites.Page(l.Slug, name)
	if err != nil {
		s.Log.Warn("site template", "list", l.Slug, "err", err)
	}
	if t == nil {
		http.Error(w, "template missing", 500)
		return
	}
	t, err = t.Clone()
	if err != nil {
		http.Error(w, "template error", 500)
		return
	}
	t = t.Funcs(template.FuncMap{"t": func(_ string, key string) string { return l.Text(key) }})
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", p); err != nil {
		s.Log.Error("render", "list", l.Slug, "page", name, "err", err)
		http.Error(w, "template error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

func (s *Server) message(w http.ResponseWriter, l *lists.List, status int, key string, arg string, action, button string) {
	p := s.base(l)
	p.Robots = "noindex"
	p.Heading = l.Text("page." + key + ".title")
	body := l.Text("page." + key + ".body")
	if strings.Contains(body, "%s") {
		body = fmt.Sprintf(body, arg)
	}
	p.Title, p.Body, p.Action, p.Button = p.Heading+" · "+l.Name, body, action, button
	s.render(w, l, "message", status, p)
}

func (s *Server) items(l *lists.List, ws []*issues.Issue) []issueItem {
	out := make([]issueItem, 0, len(ws))
	for _, wv := range ws {
		out = append(out, issueItem{URL: s.Renderer.IssueURL(l, wv), Subject: wv.Subject, Summary: wv.Summary, Date: dateOf(wv, l.Text("date"))})
	}
	return out
}

func dateOf(w *issues.Issue, layout string) string {
	t := w.Started
	if t.IsZero() {
		t = w.Created
	}
	return t.Format(layout)
}

func (s *Server) site(w http.ResponseWriter, r *http.Request, l *lists.List, prefix, path string) {
	ctx := r.Context()
	switch {
	case path == "/" || path == "":
		ws, _ := s.Issues.List(ctx, l.ID, true, 11)
		p := s.base(l)
		p.Title, p.Canonical, p.Home = l.Name, s.Renderer.ListURL(l)+"/", true
		p.Issues = s.items(l, ws)
		if len(p.Issues) > 10 {
			p.Issues, p.More = p.Issues[:10], true
		}
		s.render(w, l, "home", 200, p)
	case path == "/archive":
		ws, _ := s.Issues.List(ctx, l.ID, true, 1000)
		p := s.base(l)
		p.Title = l.Text("home.archive") + " · " + l.Name
		p.Issues = s.items(l, ws)
		s.render(w, l, "archive", 200, p)
	case strings.HasPrefix(path, "/media/"):
		data, typ, err := s.Issues.MediaFile(ctx, l.ID, strings.TrimPrefix(path, "/media/"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		// Content-addressed: the name changes when the image does.
		w.Header().Set("Content-Type", typ)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Write(data)
	case strings.HasPrefix(path, "/issues/"):
		wv, err := s.Issues.BySlug(ctx, l.ID, strings.TrimPrefix(path, "/issues/"))
		if err != nil || (wv.Status != "sent" && wv.Status != "sending") {
			http.NotFound(w, r)
			return
		}
		body, err := s.Renderer.WebHTML(l, wv)
		if err != nil {
			http.Error(w, "render error", 500)
			return
		}
		p := s.base(l)
		p.Title, p.Description, p.Canonical = wv.Subject+" · "+l.Name, wv.Summary, s.Renderer.IssueURL(l, wv)
		item := s.items(l, []*issues.Issue{wv})[0]
		item.Body = body
		p.Issue = &item
		s.render(w, l, "issue", 200, p)
	case path == "/feed.xml":
		s.feed(w, r, l)
	case path == "/subscribe":
		if r.Method == http.MethodOptions {
			cors(w)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Redirect(w, r, s.Renderer.ListURL(l)+"/", http.StatusSeeOther)
			return
		}
		s.subscribe(w, r, l)
	case strings.HasPrefix(path, "/assets/"):
		if p, ok := s.Sites.Asset(l.Slug, strings.TrimPrefix(path, "/assets/")); ok {
			w.Header().Set("Cache-Control", "public, max-age=3600")
			http.ServeFile(w, r, p)
			return
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") || strings.Contains(r.Header.Get("Content-Type"), "application/json")
}

// subscribe handles signups. Every outcome a bot could learn from (honeypot,
// too fast, already subscribed) looks the same as success.
func (s *Server) subscribe(w http.ResponseWriter, r *http.Request, l *lists.List) {
	ctx := r.Context()
	cors(w)
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var email, honeypot, ts, source string
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var in struct {
			Email, Website, Source string
			TS                     string `json:"Ts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			s.reply(w, r, l, http.StatusBadRequest, "invalid")
			return
		}
		email, honeypot, ts, source = in.Email, in.Website, in.TS, in.Source
	} else {
		if err := r.ParseForm(); err != nil {
			s.reply(w, r, l, http.StatusBadRequest, "invalid")
			return
		}
		email, honeypot, ts, source = r.PostForm.Get("email"), r.PostForm.Get("website"), r.PostForm.Get("ts"), r.PostForm.Get("source")
	}
	ip := clientIP(r)
	now := s.Now()
	perIP := s.Config.Int(ctx, "web.signup_per_ip")
	if !s.limits.allow("signup:"+ip, perIP, time.Hour, now) || !s.limits.allow("signup-net:"+network(ip), 3*perIP, time.Hour, now) {
		s.reply(w, r, l, http.StatusTooManyRequests, "limit")
		return
	}
	if honeypot != "" {
		s.reply(w, r, l, http.StatusOK, "check")
		return
	}
	if ts != "" {
		ids, err := s.Tokens.Parse(token.Form, ts, 1)
		if err != nil {
			s.reply(w, r, l, http.StatusOK, "check")
			return
		}
		age := now.Sub(time.Unix(ids[0], 0))
		if age < 2*time.Second || age > 7*24*time.Hour {
			s.reply(w, r, l, http.StatusOK, "check")
			return
		}
	}
	if l.Paused != "" {
		s.reply(w, r, l, http.StatusServiceUnavailable, "error")
		return
	}
	src := "web"
	if source = strings.TrimSpace(source); source != "" {
		src = "web: " + trimTo(source, 100)
	} else if ref := r.Header.Get("Referer"); ref != "" {
		src = "web: " + trimTo(ref, 200)
	}
	err := s.Signup(ctx, l, email, lists.Signup{IP: ip, UserAgent: r.UserAgent(), Source: src})
	switch {
	case err == nil:
		s.reply(w, r, l, http.StatusOK, "check")
	case errors.Is(err, ErrLimited):
		s.reply(w, r, l, http.StatusTooManyRequests, "limit")
	case errors.Is(err, addr.ErrInvalid):
		s.reply(w, r, l, http.StatusBadRequest, "invalid")
	default:
		s.Log.Error("signup", "list", l.Slug, "err", err)
		s.reply(w, r, l, http.StatusInternalServerError, "error")
	}
}

// ErrLimited is returned by Signup when a list sends too many confirmations.
var ErrLimited = errors.New("too many signups")

func (s *Server) reply(w http.ResponseWriter, r *http.Request, l *lists.List, status int, key string) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"ok": status < 300, "status": key, "message": l.Text("page." + key + ".body")})
		return
	}
	s.message(w, l, status, key, l.Name, "", "")
}

// network is the /24 (IPv4) or /48 (IPv6) an address belongs to: one
// attacker usually holds many addresses of one network.
func network(ip string) string {
	a := net.ParseIP(ip)
	if a == nil {
		return ip
	}
	if v4 := a.To4(); v4 != nil {
		return v4.Mask(net.CIDRMask(24, 32)).String()
	}
	return a.Mask(net.CIDRMask(48, 128)).String()
}

func (s *Server) confirm(w http.ResponseWriter, r *http.Request, tok string) {
	ctx := r.Context()
	ids, err := s.Tokens.Parse(token.Confirm, tok, 2)
	if err != nil {
		s.plainInvalid(w)
		return
	}
	m, err := s.Lists.Member(ctx, ids[0])
	if err != nil || m.SignupAt.Unix() != ids[1] {
		s.plainInvalid(w)
		return
	}
	l, err := s.Lists.ByID(ctx, m.ListID)
	if err != nil {
		s.plainInvalid(w)
		return
	}
	if r.Method != http.MethodPost {
		// Link scanners follow every link; only a click on the button confirms.
		if m.Status == "confirmed" {
			s.message(w, l, 200, "confirmed", l.Name, "", "")
			return
		}
		s.message(w, l, 200, "confirm", l.Name, r.URL.Path, l.Text("confirm.action"))
		return
	}
	was := m.Status
	if m, err = s.Lists.Confirm(ctx, m.ID, clientIP(r)); err != nil {
		s.message(w, l, 410, "invalid", "", "", "")
		return
	}
	if was != "confirmed" {
		if err := s.Issues.Welcome(ctx, l, m); err != nil {
			s.Log.Error("welcome issue", "list", l.Slug, "err", err)
		}
	}
	s.message(w, l, 200, "confirmed", l.Name, "", "")
}

func (s *Server) unsubscribe(w http.ResponseWriter, r *http.Request, tok string) {
	ctx := r.Context()
	ids, err := s.Tokens.Parse(token.Unsub, tok, 2)
	if err != nil {
		s.plainInvalid(w)
		return
	}
	m, err := s.Lists.Member(ctx, ids[0])
	if err != nil {
		// The reader was erased or the list deleted: they get nothing more
		// either way, which is what an unsubscribe asks for. A mail
		// provider's one-click POST must not read as a failure.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("You will not receive anything more.\n"))
		return
	}
	l, err := s.Lists.ByID(ctx, m.ListID)
	if err != nil {
		s.plainInvalid(w)
		return
	}
	if r.Method != http.MethodPost {
		if m.Status == "unsubscribed" {
			s.message(w, l, 200, "unsubbed", l.Name, "", "")
			return
		}
		s.message(w, l, 200, "unsub", l.Name, r.URL.Path, l.Text("page.unsub.action"))
		return
	}
	how := "link"
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	// RFC 8058 §3.1 posts multipart/form-data; some providers send a
	// urlencoded body.
	if err := r.ParseMultipartForm(16 << 10); err != nil {
		r.ParseForm()
	}
	if r.FormValue("List-Unsubscribe") == "One-Click" {
		how = "one-click"
	}
	if _, err := s.Lists.Unsubscribe(ctx, m.ID, ids[1], how); err != nil {
		s.Log.Error("unsubscribe", "member", m.ID, "err", err)
		s.message(w, l, 500, "error", "", "", "")
		return
	}
	s.message(w, l, 200, "unsubbed", l.Name, "", "")
}

func (s *Server) plainInvalid(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	io.WriteString(w, "This link does not work. It may be incomplete or out of date.\n")
}

// gif is a transparent 1x1 GIF.
var gif = []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")

type delivery struct {
	issueID, contactID, listID int64
}

func (s *Server) delivery(ctx context.Context, outboundID int64) (delivery, bool) {
	var d delivery
	err := s.DB.R.QueryRowContext(ctx, `SELECT coalesce(o.issue_id, 0), coalesce(o.contact_id, 0), coalesce(w.list_id, 0)
		FROM outbound o LEFT JOIN issues w ON w.id = o.issue_id WHERE o.id = ?`, outboundID).Scan(&d.issueID, &d.contactID, &d.listID)
	return d, err == nil && d.issueID != 0 && d.contactID != 0
}

// machine reports user agents of proxies and scanners that fetch images and
// links without a person: their opens are recorded as such.
func machine(ua string) bool {
	l := strings.ToLower(ua)
	for _, m := range []string{"googleimageproxy", "yahoomailproxy", "bot", "crawler", "spider", "scanner", "barracuda", "proofpoint", "mimecast", "safelinks", "python", "curl", "wget", "go-http-client"} {
		if strings.Contains(l, m) {
			return true
		}
	}
	return l == "" || l == "mozilla/5.0"
}

func (s *Server) open(w http.ResponseWriter, r *http.Request, tok string) {
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	defer w.Write(gif)
	ids, err := s.Tokens.Parse(token.Open, tok, 1)
	if err != nil {
		return
	}
	ctx := r.Context()
	d, ok := s.delivery(ctx, ids[0])
	if !ok {
		return
	}
	var seen int
	s.DB.R.QueryRowContext(ctx, "SELECT count(*) FROM events WHERE outbound_id = ? AND kind = 'opened'", ids[0]).Scan(&seen)
	if seen > 0 {
		return
	}
	detail := "human"
	if machine(r.UserAgent()) {
		detail = "machine"
	}
	s.Events.Record(ctx, events.Event{Kind: events.Opened, ListID: d.listID, IssueID: d.issueID, ContactID: d.contactID, OutboundID: ids[0], Detail: detail})
}

func (s *Server) click(w http.ResponseWriter, r *http.Request, tok string) {
	ctx := r.Context()
	ids, err := s.Tokens.Parse(token.Click, tok, 2)
	if err != nil {
		s.plainInvalid(w)
		return
	}
	d, ok := s.delivery(ctx, ids[0])
	if !ok {
		s.plainInvalid(w)
		return
	}
	wv, err := s.Issues.Get(ctx, d.issueID)
	if err != nil {
		s.plainInvalid(w)
		return
	}
	l, err := s.Lists.ByID(ctx, wv.ListID)
	if err != nil {
		s.plainInvalid(w)
		return
	}
	target, ok := s.Renderer.Link(l, wv, int(ids[1]))
	if !ok {
		s.plainInvalid(w)
		return
	}
	var seen int
	var sent int64
	s.DB.R.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM events WHERE outbound_id = ?1 AND kind = 'clicked' AND detail = ?2),
		(SELECT sent_at FROM outbound WHERE id = ?1)`, ids[0], target).Scan(&seen, &sent)
	// Link scanners follow every link moments after delivery; people do
	// not click within seconds (lfr:E096).
	scanner := sent > 0 && s.Now().Sub(time.Unix(sent, 0)) < 10*time.Second
	if seen == 0 && !machine(r.UserAgent()) && !scanner {
		s.Lists.Engaged(ctx, d.contactID)
		s.Events.Record(ctx, events.Event{Kind: events.Clicked, ListID: d.listID, IssueID: d.issueID, ContactID: d.contactID, OutboundID: ids[0], Detail: target})
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) hook(w http.ResponseWriter, r *http.Request, rest string) {
	tok, provider, _ := strings.Cut(rest, "/")
	if _, err := s.Tokens.Parse(token.Hook, tok, 1); err != nil || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	var evs []feedback.Event
	switch provider {
	case "ses":
		// Amazon SES reports through SNS, which signs every message.
		s.snsOnce.Do(func() { s.sns = &feedback.SNSVerifier{Client: s.SNSClient} })
		m, err := s.sns.Verify(r.Context(), body)
		if err != nil {
			s.Log.Warn("webhook", "provider", provider, "err", err)
			http.Error(w, "unverified", http.StatusForbidden)
			return
		}
		if s.SNSTopic != nil && !s.SNSTopic(r.Context(), m.TopicArn, m.Type == "SubscriptionConfirmation") {
			s.Log.Warn("webhook from an unknown SNS topic", "topic", m.TopicArn)
			http.Error(w, "unknown topic", http.StatusForbidden)
			return
		}
		switch m.Type {
		case "SubscriptionConfirmation":
			client := s.SNSClient
			if client == nil {
				client = &http.Client{Timeout: 15 * time.Second}
			}
			resp, err := client.Get(m.SubscribeURL)
			if err != nil {
				s.Log.Warn("SNS subscription confirmation", "err", err)
				http.Error(w, "confirmation failed", http.StatusBadGateway)
				return
			}
			resp.Body.Close()
			s.Log.Info("confirmed SNS subscription", "topic", m.TopicArn)
		case "Notification":
			if evs, err = feedback.SESEvents(m); err != nil {
				http.Error(w, "unreadable", http.StatusBadRequest)
				return
			}
		}
	case "smtp2go", "generic":
		if evs, err = feedback.Webhook(r.Header.Get("Content-Type"), body); err != nil {
			s.Log.Warn("webhook", "provider", provider, "err", err)
			http.Error(w, "unreadable", http.StatusBadRequest)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
	if len(evs) > 0 {
		s.Feedback(r.Context(), evs)
	}
	s.recordHook(r.Context(), provider)
	w.WriteHeader(http.StatusNoContent)
}

// recordHook persists receipt at most once a minute, including across restarts.
func (s *Server) recordHook(ctx context.Context, provider string) {
	s.hookMu.Lock()
	defer s.hookMu.Unlock()
	now := s.Now()
	if s.hookRecorded.IsZero() {
		v, _ := s.DB.Setting(ctx, "relay.last_hook", "")
		at, _, _ := strings.Cut(v, " ")
		s.hookRecorded, _ = time.Parse(time.RFC3339, at)
	}
	if !s.hookRecorded.IsZero() && now.Sub(s.hookRecorded) < time.Minute {
		return
	}
	if err := s.DB.SetSetting(ctx, "relay.last_hook", now.UTC().Format(time.RFC3339)+" "+provider); err != nil {
		s.Log.Warn("record webhook receipt", "err", err)
	}
	s.hookRecorded = now
}

type atomFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Author  atomAuthor  `xml:"author"` // required by RFC 4287 §4.1.1
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
}

type atomEntry struct {
	Title   string   `xml:"title"`
	ID      string   `xml:"id"`
	Updated string   `xml:"updated"`
	Link    atomLink `xml:"link"`
	Summary string   `xml:"summary,omitempty"`
	Content struct {
		Type string `xml:"type,attr"`
		Body string `xml:",chardata"`
	} `xml:"content"`
}

func (s *Server) feed(w http.ResponseWriter, r *http.Request, l *lists.List) {
	ws, _ := s.Issues.List(r.Context(), l.ID, true, 30)
	home := s.Renderer.ListURL(l)
	author := l.FromName
	if author == "" {
		author = l.Name
	}
	f := atomFeed{Title: l.Name, ID: home + "/", Author: atomAuthor{Name: author}, Links: []atomLink{{Href: home + "/"}, {Href: home + "/feed.xml", Rel: "self"}}}
	f.Updated = l.Created.Format(time.RFC3339)
	for i, wv := range ws {
		t := wv.Started
		if t.IsZero() {
			t = wv.Created
		}
		if i == 0 {
			f.Updated = t.Format(time.RFC3339)
		}
		e := atomEntry{Title: wv.Subject, ID: s.Renderer.IssueURL(l, wv), Updated: t.Format(time.RFC3339), Link: atomLink{Href: s.Renderer.IssueURL(l, wv)}, Summary: wv.Summary}
		body, _ := s.Renderer.WebHTML(l, wv)
		e.Content.Type, e.Content.Body = "html", string(body)
		f.Entries = append(f.Entries, e)
	}
	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=600")
	io.WriteString(w, xml.Header)
	xml.NewEncoder(w).Encode(f)
}

func trimTo(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
