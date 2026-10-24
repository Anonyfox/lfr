package issues

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"maps"
	"mime"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfr/internal/emailmd"
	"github.com/Anonyfox/lfr/internal/finding"
	"github.com/Anonyfox/lfr/internal/lists"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/outbound"
	"github.com/Anonyfox/lfr/internal/token"
)

// Links are the per-recipient URLs of one message.
type Links struct {
	Unsub     string // https URL of the unsubscribe page (POST target for RFC 8058)
	UnsubMail string // mailto: address with token
	Pixel     string // open tracking image, empty without tracking
	Click     func(i int) string
	Web       string // web version
}

// Renderer turns issues into messages. URLs come from the engine because
// they depend on the list's host and the server's host name.
type Renderer struct {
	Tokens  *token.Signer
	ListURL func(l *lists.List) string // base URL of a list's site, no trailing slash
	BaseURL func(l *lists.List) string // where token links live (the list's site or the mail host)
	Layout  func(l *lists.List) *template.Template
	Media   func(issueID int64) map[string]Media // an issue's uploaded images; nil for none
	Now     func() time.Time

	mu    sync.Mutex
	cache map[int64]*prepared
}

// prepared is an issue rendered once with placeholders for per-recipient links.
type prepared struct {
	updated time.Time
	key     string   // the list settings and images it was rendered with
	missing []string // local images that were never uploaded
	heavy   []Media  // images above HeavyMedia
	links   []string
	html    string // email HTML with %%LFR_*%% placeholders
	text    string
	web     string // HTML fragment for the archive
}

const (
	// Placeholders use only characters no HTML or URL escaping changes.
	phUnsub     = "LFR__UNSUB__"
	phUnsubMail = "LFR__UNSUBMAIL__"
	phPixel     = "LFR__PIXEL__"
	phWeb       = "LFR__WEB__"
	phWhy       = "LFR__WHY__"
)

// footerMuted is the footer's text color: 7:1 on the page background, where
// the common light gray fails WCAG's 4.5:1 (lfr:E061, lfr:E063).
const footerMuted = "#52525b"

func phClick(i int) string { return "LFR__CLICK" + strconv.Itoa(i) + "__" }

// IssueURL is the web version of an issue.
func (r *Renderer) IssueURL(l *lists.List, w *Issue) string {
	return r.ListURL(l) + "/issues/" + w.Slug
}

// MediaURL is where the website serves an issue's image.
func (r *Renderer) MediaURL(l *lists.List, m Media) string {
	return r.ListURL(l) + "/media/" + m.Name()
}

func (r *Renderer) prepare(l *lists.List, w *Issue) (*prepared, error) {
	var media map[string]Media
	if r.Media != nil {
		media = r.Media(w.ID)
	}
	// The cached copy holds the list's footer, sender and links too: a new
	// postal address or imprint must reach the very next send.
	var key strings.Builder
	fmt.Fprintf(&key, "%q|%q|%q|%q|%q|%q|%q|%t|%q|%q|%q|%q|%q;", l.Name, l.Domain, l.Slug, l.From, l.FromName, l.ReplyTo, l.Host,
		l.Tracking, l.Language, l.ImprintURL, l.PrivacyURL, l.Address, l.Cadence)
	for _, k := range slices.Sorted(maps.Keys(l.Texts)) {
		fmt.Fprintf(&key, "%q=%q;", k, l.Texts[k])
	}
	key.WriteString(r.ListURL(l) + ";")
	for _, path := range slices.Sorted(maps.Keys(media)) {
		key.WriteString(path + "=" + media[path].Hash + ";")
	}
	r.mu.Lock()
	if r.cache == nil {
		r.cache = map[int64]*prepared{}
	}
	if p, ok := r.cache[w.ID]; ok && p.updated.Equal(w.Updated) && p.key == key.String() {
		r.mu.Unlock()
		return p, nil
	}
	r.mu.Unlock()
	doc, err := emailmd.Render(w.Markdown)
	if err != nil {
		return nil, err
	}
	var missing []string
	var heavy []Media
	doc.HTML = emailmd.RewriteImages(doc.HTML, func(src string) string {
		if !emailmd.Local(src) {
			return src
		}
		m, ok := media[src]
		if !ok {
			missing = append(missing, src)
			return src
		}
		if m.Size > HeavyMedia {
			heavy = append(heavy, m)
		}
		return r.MediaURL(l, m)
	})
	body := emailmd.Inline(doc.HTML, emailmd.DefaultTheme)
	if l.Tracking {
		body = emailmd.RewriteLinks(body, func(i int, _ string) string { return phClick(i) })
	}
	lang := l.Language
	footer := footerHTML(l, r.ListURL(l), !w.Welcome)
	layout := emailmd.Layout{
		Lang: lang, Title: w.Subject, Preheader: w.Preheader, Brand: l.Name, BrandURL: r.ListURL(l),
		Body: template.HTML(body), FooterHTML: template.HTML(footer), Theme: emailmd.DefaultTheme,
	}
	if l.Tracking {
		layout.Pixel = phPixel
	}
	var out string
	if t := r.layout(l); t != nil {
		var b strings.Builder
		if err := t.Execute(&b, layout); err != nil {
			return nil, fmt.Errorf("email template of %s: %w", l.Slug, err)
		}
		out = b.String()
	} else if out, err = emailmd.Email(layout); err != nil {
		return nil, err
	}
	p := &prepared{
		updated: w.Updated, key: key.String(), missing: missing, heavy: heavy, links: doc.Links, html: out,
		text: textPreheader(w.Preheader) + emailmd.Text(w.Markdown) + "\n\n-- \n" + footerText(l, r.ListURL(l), !w.Welcome),
		web:  emailmd.MarkButtons(doc.HTML),
	}
	r.mu.Lock()
	if len(r.cache) > 50 {
		r.cache = map[int64]*prepared{}
	}
	r.cache[w.ID] = p
	r.mu.Unlock()
	return p, nil
}

func (r *Renderer) layout(l *lists.List) *template.Template {
	if r.Layout == nil {
		return nil
	}
	return r.Layout(l)
}

// footerHTML is the legal footer. web links the issue's archive page, which
// the welcome issue does not have.
func footerHTML(l *lists.List, listURL string, web bool) string {
	esc := html.EscapeString
	link := func(href, text string) string {
		return `<a href="` + href + `" style="color:` + footerMuted + `;text-decoration:underline;">` + esc(text) + `</a>`
	}
	var b strings.Builder
	b.WriteString(`<p style="margin:0 0 8px;">` + phWhy + `</p>`)
	b.WriteString(`<p style="margin:0 0 8px;">` + link(phUnsub, l.Text("footer.unsub")))
	if web {
		b.WriteString(" · " + link(phWeb, l.Text("footer.web")))
	}
	if l.ImprintURL != "" {
		b.WriteString(" · " + link(esc(l.ImprintURL), l.Text("footer.imprint")))
	}
	if l.PrivacyURL != "" {
		b.WriteString(" · " + link(esc(l.PrivacyURL), l.Text("footer.privacy")))
	}
	b.WriteString("</p>")
	host := strings.TrimPrefix(strings.TrimPrefix(listURL, "https://"), "http://")
	b.WriteString(`<p style="margin:0 0 8px;">` + fmt.Sprintf(esc(l.Text("footer.forwarded")), link(esc(listURL)+"/", host)) + `</p>`)
	if l.Address != "" {
		b.WriteString(`<p style="margin:0;">` + strings.ReplaceAll(esc(l.Address), "\n", "<br>") + `</p>`)
	}
	return b.String()
}

func footerText(l *lists.List, listURL string, web bool) string {
	lines := []string{phWhy, l.Text("footer.unsub") + ": " + phUnsub}
	if web {
		lines = append(lines, l.Text("footer.web")+": "+phWeb)
	}
	if l.ImprintURL != "" {
		lines = append(lines, l.Text("footer.imprint")+": "+l.ImprintURL)
	}
	if l.PrivacyURL != "" {
		lines = append(lines, l.Text("footer.privacy")+": "+l.PrivacyURL)
	}
	lines = append(lines, fmt.Sprintf(l.Text("footer.forwarded"), listURL+"/"))
	if l.Address != "" {
		lines = append(lines, l.Address)
	}
	return strings.Join(lines, "\n")
}

// why tells a reader how they came onto the list: readers who remember
// signing up unsubscribe instead of reporting spam (lfr:V004, lfr:V092).
func why(l *lists.List, rc Recipient) string {
	if rc.SignupAt.IsZero() {
		return fmt.Sprintf(l.Text("footer.why"), l.Name)
	}
	day := rc.SignupAt.UTC().Format(l.Text("date"))
	if place := signupPlace(rc.Source, l); place != "" {
		return fmt.Sprintf(l.Text("footer.since.at"), l.Name, day, place)
	}
	return fmt.Sprintf(l.Text("footer.since"), l.Name, day)
}

// signupPlace is the host a web signup came from, or "".
func signupPlace(source string, l *lists.List) string {
	rest, ok := strings.CutPrefix(source, "web")
	if !ok {
		return ""
	}
	rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
	if u, err := url.Parse(rest); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	if l.Host != "" {
		return l.Host
	}
	return ""
}

// Recipient is who a message is rendered for.
type Recipient struct {
	OutboundID int64
	MemberID   int64
	Email      string
	Name       string
	SignupAt   time.Time // zero for test copies
	Source     string    // where the signup came from ("web: https://…")
	Tracked    bool      // the reader consented to open and click measurement
}

// Message builds the complete unsigned message for one recipient.
func (r *Renderer) Message(l *lists.List, w *Issue, rc Recipient, test bool) ([]byte, error) {
	p, err := r.prepare(l, w)
	if err != nil {
		return nil, err
	}
	base := r.BaseURL(l)
	unsubTok := r.Tokens.Make(token.Unsub, rc.MemberID, w.ID)
	links := Links{
		Unsub:     base + "/u/" + unsubTok,
		UnsubMail: "unsub+" + unsubTok + "@" + l.Domain,
		Web:       r.IssueURL(l, w),
	}
	pairs := []string{phUnsub, links.Unsub, phUnsubMail, links.UnsubMail, phWeb, links.Web}
	reason := why(l, rc)
	textPairs := append(append([]string{}, pairs...), phWhy, reason)
	pairs = append(pairs, phWhy, html.EscapeString(reason))
	track := l.Tracking && rc.OutboundID != 0 && rc.Tracked
	if track {
		pairs = append(pairs, phPixel, base+"/o/"+r.Tokens.Make(token.Open, rc.OutboundID)+".gif")
		for i := range p.links {
			pairs = append(pairs, phClick(i), base+"/c/"+r.Tokens.Make(token.Click, rc.OutboundID, int64(i)))
		}
	} else {
		pairs = append(pairs, phPixel, "")
		for i, u := range p.links {
			pairs = append(pairs, phClick(i), html.EscapeString(u))
		}
	}
	rep := strings.NewReplacer(pairs...)
	htmlBody := rep.Replace(p.html)
	if !track {
		htmlBody = strings.Replace(htmlBody, `<img src="" width="1" height="1" alt="" style="display:block;width:1px;height:1px;border:0;">`, "", 1)
	}
	subject := w.Subject
	if test {
		subject = "[Test] " + subject
	}
	msgID := ""
	if rc.OutboundID != 0 {
		msgID = r.Tokens.Make(token.Message, rc.OutboundID) + "@" + l.Domain // stable across retries
	}
	d := msg.Draft{
		From:      msg.Address{Name: l.FromName, Email: l.From},
		To:        []msg.Address{{Name: rc.Name, Email: rc.Email}},
		Subject:   subject,
		Date:      r.Now(),
		MessageID: msgID,
		Text:      strings.NewReplacer(textPairs...).Replace(p.text),
		HTML:      htmlBody,
		Header: [][2]string{
			{"List-Id", listID(l)},
			{"Precedence", "list"}, // auto-responders stay quiet (RFC 3834 §2)
			{"List-Unsubscribe", "<" + links.Unsub + ">, <mailto:" + links.UnsubMail + "?subject=unsubscribe>"},
			{"List-Unsubscribe-Post", "List-Unsubscribe=One-Click"},
			{"Feedback-ID", fmt.Sprintf("w%d:%s:%s:lfr", w.ID, l.Slug, l.Domain)},
			{"List-Archive", "<" + r.ListURL(l) + ">"},
		},
	}
	if l.ReplyTo != "" {
		d.ReplyTo = []msg.Address{{Email: l.ReplyTo}}
	}
	return msg.Compose(d)
}

// listID is the List-Id field (RFC 2919): the list's name as a phrase,
// encoded when it goes beyond ASCII, and its identifier.
func listID(l *lists.List) string {
	name := strings.NewReplacer(`"`, "", `\`, "", "\r", "", "\n", "").Replace(l.Name)
	for _, r := range name {
		if r > 127 {
			name = mime.QEncoding.Encode("utf-8", name)
			return fmt.Sprintf("%s <%s.%s>", name, l.Slug, l.Domain)
		}
	}
	return fmt.Sprintf("%q <%s.%s>", name, l.Slug, l.Domain)
}

// WebHTML is the archive version of an issue: semantic HTML without tracking.
func (r *Renderer) WebHTML(l *lists.List, w *Issue) (template.HTML, error) {
	p, err := r.prepare(l, w)
	if err != nil {
		return "", err
	}
	return template.HTML(p.web), nil
}

// Link returns the target of link i of an issue, for click redirects.
func (r *Renderer) Link(l *lists.List, w *Issue, i int) (string, bool) {
	p, err := r.prepare(l, w)
	if err != nil || i < 0 || i >= len(p.links) {
		return "", false
	}
	return p.links[i], true
}

// Builder returns the outbound builder for issue rows.
func (s *Service) Builder(r *Renderer) outbound.Builder {
	return func(ctx context.Context, row outbound.Row) ([]byte, error) {
		w, err := s.Get(ctx, row.IssueID)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", outbound.ErrSkip, err)
		}
		l, err := s.Lists.ByID(ctx, w.ListID)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", outbound.ErrSkip, err)
		}
		rc := Recipient{OutboundID: row.ID, Email: row.Rcpt}
		test := row.ContactID == 0
		if !test {
			m, err := s.Lists.Member(ctx, row.MemberID)
			if err != nil {
				return nil, fmt.Errorf("%w: member gone", outbound.ErrSkip)
			}
			if m.Status != "confirmed" {
				return nil, fmt.Errorf("%w: member is %s", outbound.ErrSkip, m.Status)
			}
			if m.Suppressed != "" {
				return nil, fmt.Errorf("%w: address suppressed (%s)", outbound.ErrSkip, m.Suppressed)
			}
			rc.MemberID, rc.Name, rc.SignupAt, rc.Source = m.ID, m.Name, m.SignupAt, m.Source
			rc.Tracked = lists.ConsentsToTracking(m.Consent)
			// The welcome issue goes out on every confirmation, without a
			// send command that checks it first. While it has an error,
			// such as a footer without the postal address, it waits and
			// retries; once the list is fixed the reader gets it.
			if w.Welcome {
				fs, err := s.Lint(ctx, r, w.ID)
				if err != nil {
					return nil, err
				}
				for _, f := range fs {
					if f.Severity == finding.Error {
						return nil, fmt.Errorf("welcome issue waits for a fix: %s (%s)", f.Message, f.Fix)
					}
				}
			}
		}
		return r.Message(l, w, rc, test)
	}
}

// Test queues the issue to addresses outside the list, marked [Test], without tracking.
func (s *Service) Test(ctx context.Context, id int64, to []string) error {
	w, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	l, err := s.Lists.ByID(ctx, w.ListID)
	if err != nil {
		return err
	}
	var items []outbound.Item
	for _, t := range to {
		items = append(items, outbound.Item{Kind: outbound.KindIssue, DomainID: l.DomainID, IssueID: w.ID, Rcpt: t})
	}
	if err := s.Queue.Enqueue(ctx, items...); err != nil {
		return err
	}
	_, err = s.DB.W.ExecContext(ctx, "UPDATE issues SET tested = ? WHERE id = ?", s.Now().Unix(), w.ID)
	return err
}

// textPreheader starts the plain-text part like the HTML one: the preheader
// is visible content, not hidden markup (lfr:E006).
func textPreheader(p string) string {
	if p = strings.TrimSpace(p); p == "" {
		return ""
	}
	return p + "\n\n"
}
