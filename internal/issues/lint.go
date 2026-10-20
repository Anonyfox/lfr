package issues

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/Anonyfox/lfr/internal/editorial"
	"github.com/Anonyfox/lfr/internal/finding"
	"github.com/Anonyfox/lfr/internal/lists"
)

// Finding is one problem found before sending.
type Finding = finding.Finding

var (
	imgRE   = regexp.MustCompile(`(?i)<img\b[^>]*>`)
	hrefRE  = regexp.MustCompile(`(?i)\bhref\s*=\s*"([^"]*)"`)
	srcRE   = regexp.MustCompile(`(?i)\bsrc\s*=\s*"([^"]*)"`)
	shorten = map[string]bool{"bit.ly": true, "tinyurl.com": true, "t.co": true, "goo.gl": true, "ow.ly": true, "is.gd": true, "buff.ly": true, "rebrand.ly": true, "cutt.ly": true}
)

// gmailClip is the size from which Gmail hides the rest of a message behind "View entire message".
const gmailClip = 102 * 1024

// Lint checks an issue before it goes out.
func (s *Service) Lint(ctx context.Context, r *Renderer, id int64) ([]Finding, error) {
	w, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	l, err := s.Lists.ByID(ctx, w.ListID)
	if err != nil {
		return nil, err
	}
	raw, err := r.Message(l, w, Recipient{OutboundID: 1 << 40, MemberID: 1 << 40, Email: "reader@example.org", Tracked: true}, false)
	if err != nil {
		return []Finding{{Rule: "render", Severity: finding.Error, Message: err.Error(), Fix: "fix the Markdown or the list's email template"}}, nil
	}
	p, _ := r.prepare(l, w)
	return lint(l, w, p, len(raw), r.ListURL(l)+"/media/"), nil
}

func lint(l *lists.List, w *Issue, p *prepared, size int, own string) []Finding {
	out := editorial.Check(editorial.Issue{Subject: w.Subject, Preheader: w.Preheader, Markdown: w.Markdown, Language: l.Language}, editorial.Brief{})
	add := func(rule, sev, message, fix string) {
		out = append(out, Finding{Rule: rule, Severity: sev, Message: message, Fix: fix})
	}
	if size > gmailClip {
		add("size.clipped", finding.Error, fmt.Sprintf("message is %d KB; Gmail clips everything after 102 KB, hiding the unsubscribe link", size/1024), "shorten the issue or move long parts to the web version")
	} else if size > 80*1024 {
		add("size.large", finding.Warning, fmt.Sprintf("message is %d KB, close to Gmail's 102 KB clipping", size/1024), "keep it shorter")
	}
	for _, m := range p.missing {
		add("images.missing", finding.Error, "image "+m+" was never uploaded", "run lfr issue put from the folder that holds the Markdown and the image")
	}
	for _, m := range p.heavy {
		add("images.heavy", finding.Warning, fmt.Sprintf("image %s is %d KB; on a phone connection that delays the issue", m.Path, m.Size>>10), "resize it to the width it shows at (about 1200 pixels) and compress it")
	}
	for _, img := range imgRE.FindAllString(p.web, -1) {
		src := ""
		if m := srcRE.FindStringSubmatch(img); m != nil {
			src = m[1]
		}
		if src != "" && !strings.HasPrefix(src, "https://") {
			sev := finding.Error
			if strings.HasPrefix(src, "http://") {
				sev = finding.Warning
			}
			add("images.url", sev, "image "+src+" is not an absolute https URL; mail clients cannot load it", "host the image and link it with https://")
		} else if src != "" && !strings.HasPrefix(src, own) && !strings.HasPrefix(src, "data:") {
			add("images.remote", finding.Warning, "image "+src+" loads from another site: every reader who opens the issue or its archive page tells that site their IP address", "put the file next to the draft and reference it by path; lfr issue put uploads it")
		}
	}
	for _, m := range hrefRE.FindAllStringSubmatch(p.web, -1) {
		href := m[1]
		switch {
		case strings.HasPrefix(href, "mailto:"), strings.HasPrefix(href, "#"), strings.HasPrefix(href, "https://"):
		case strings.HasPrefix(href, "http://"):
			add("links.insecure", finding.Warning, "link "+href+" uses http", "use https:// where the site supports it")
		default:
			add("links.relative", finding.Error, "link "+href+" is not absolute; it cannot work in an email", "use the full https:// URL")
		}
		if u, err := url.Parse(href); err == nil && shorten[strings.TrimPrefix(u.Hostname(), "www.")] {
			add("links.shortener", finding.Warning, "link "+href+" uses a URL shortener, which spam filters distrust", "link the destination directly")
		}
	}
	if n := len([]rune(l.FromName)); n > 20 {
		add("list.sender", finding.Warning, fmt.Sprintf("the sender name %q has %d characters; inboxes cut it at about 20", l.FromName, n), "lfr list set "+l.Slug+" --from-name \"Short Name\"")
	}
	if l.Cadence == "" {
		add("list.cadence", finding.Warning, "the list does not say how often mail comes; readers who know what to expect stay and do not complain", "lfr list set "+l.Slug+" --cadence \"Every Friday morning\" (lfr push sets it from concept.md)")
	}
	if l.Address == "" {
		add("list.address", finding.Error, "the list has no sender address line; CAN-SPAM requires a postal address and German law (§5 DDG) an imprint", "lfr list set "+l.Slug+" --address \"Name, Street 1, 12345 City\"")
	}
	if l.ImprintURL == "" && strings.HasPrefix(l.Language, "de") {
		add("list.imprint", finding.Error, "German newsletters need a reachable imprint (Impressum, §5 DDG)", "lfr list set "+l.Slug+" --imprint-url https://example.com/impressum")
	}
	if l.PrivacyURL == "" {
		add("list.privacy", finding.Info, "no privacy policy link in the footer", "lfr list set "+l.Slug+" --privacy-url https://example.com/privacy")
	}
	if l.Tracking && !lists.ConsentsToTracking(l.ConsentText()) {
		add("list.tracking-consent", finding.Error, "open and click tracking needs the readers' consent (TDDDG §25, GDPR Art. 6), and the list's consent text does not mention it; readers who signed up without it are never measured", "lfr list set "+l.Slug+" --consent \"... including open and click statistics\" or --tracking=false")
	}
	finding.Sort(out)
	return out
}

// Rules describes the checks only the server can run, on the rendered
// message and the list's settings; editorial.Rules has the rest.
func Rules() []finding.Rule {
	return []finding.Rule{
		{ID: "size.clipped", Summary: "The message stays under Gmail's 102 KB clipping", Help: "Gmail hides everything after 102 KB of HTML behind \"View entire message\", including the unsubscribe link and the open pixel (lfr:E125, lfr:E074). Shorten the issue or move long parts to the web version."},
		{ID: "size.large", Summary: "The message is not close to clipping", Help: "Above 80 KB a few more links or a longer footer tip it over Gmail's 102 KB."},
		{ID: "images.missing", Summary: "Every local image was uploaded", Help: "Images the Markdown references by a path travel with lfr issue put and are served from the list's website; one that was never uploaded shows as broken."},
		{ID: "images.heavy", Summary: "Images are light", Help: "Most mail is read on phones (lfr:E122); NN/g advises keeping a newsletter small (lfr:E085). Above 300 KB an image slows the issue down."},
		{ID: "images.url", Summary: "Images load from absolute https URLs", Help: "Mail clients cannot resolve relative paths, and many block plain http images."},
		{ID: "images.remote", Summary: "Images come from the list's own site", Help: "An image from another server hands it every reader's IP address, which needs consent under GDPR (LG München I, 3 O 17493/20, on remotely loaded fonts); the list's website also refuses to show it. Upload images with the issue."},
		{ID: "links.insecure", Summary: "Links use https", Help: "Some clients and filters warn on http links."},
		{ID: "links.relative", Summary: "Links are absolute", Help: "A relative link has no site to resolve against inside an email."},
		{ID: "links.shortener", Summary: "No URL shorteners", Help: "Shorteners hide the destination; SpamAssassin scores abused shorteners (lfr:E073) and Google wants links readers understand (lfr:E006)."},
		{ID: "list.sender", Summary: "The sender name is short and stays the same", Help: "The sent-from line decides more than the subject (lfr:V116); NN/g advises at most 20 characters naming the newsletter (lfr:E085), and providers want a consistent sender (lfr:E021, lfr:E007)."},
		{ID: "list.cadence", Summary: "The list says how often mail comes", Help: "Every provider and best practice asks senders to state frequency at signup (lfr:E017, lfr:E051, lfr:E056, lfr:E083); unexpected frequency drives complaints (lfr:E050). lfr shows it on the signup form and in the confirmation mail."},
		{ID: "list.address", Summary: "Every footer names the sender and a postal address", Help: "CAN-SPAM requires a valid postal address (lfr:E047); German law an imprint (lfr:E044). lfr list set --address."},
		{ID: "list.imprint", Summary: "German newsletters link an imprint", Help: "lfr list set --imprint-url."},
		{ID: "list.privacy", Summary: "The footer links a privacy policy", Help: "lfr list set --privacy-url."},
		{ID: "list.tracking-consent", Summary: "Tracking in Germany has consent", Help: "Tracking pixels and links fall under the ePrivacy rules (lfr:E039, lfr:E043): the consent text must say so, or tracking stays off. Opens are unreliable anyway (lfr:E091, lfr:E094)."},
	}
}

// Blocking reports whether findings forbid sending.
func Blocking(f []Finding) bool { return finding.Blocking(f) }
