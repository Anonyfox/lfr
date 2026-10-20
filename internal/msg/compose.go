package msg

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"io"
	"mime"
	"net/mail"
	"strings"
	"time"

	"github.com/emersion/go-message"
	gomail "github.com/emersion/go-message/mail"
)

// File is an attachment to compose.
type File struct {
	Name string
	Type string // detected from Name when empty
	Data []byte
}

// Draft is a message to compose.
type Draft struct {
	From       Address
	To, Cc     []Address
	ReplyTo    []Address
	Subject    string
	Date       time.Time
	MessageID  string // without angle brackets; generated when empty
	InReplyTo  string
	References []string
	Header     [][2]string // extra fields, in order (e.g. List-Unsubscribe)
	Text       string
	HTML       string
	Files      []File
}

// NewMessageID makes a unique Message-ID on domain.
func NewMessageID(domain string) string {
	b := make([]byte, 15)
	rand.Read(b)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)) + "@" + domain
}

func toMail(l []Address) []*gomail.Address {
	out := make([]*gomail.Address, len(l))
	for i, a := range l {
		out[i] = &gomail.Address{Name: a.Name, Address: a.Email}
	}
	return out
}

// Compose renders d as a MIME message with CRLF line endings: text only,
// multipart/alternative when HTML is present, multipart/mixed with files.
func Compose(d Draft) ([]byte, error) {
	if d.From.Email == "" {
		return nil, fmt.Errorf("compose: no sender")
	}
	var h gomail.Header
	h.SetAddressList("From", toMail([]Address{d.From}))
	if len(d.To) > 0 {
		h.SetAddressList("To", toMail(d.To))
	}
	if len(d.Cc) > 0 {
		h.SetAddressList("Cc", toMail(d.Cc))
	}
	if len(d.ReplyTo) > 0 {
		h.SetAddressList("Reply-To", toMail(d.ReplyTo))
	}
	h.SetSubject(strings.NewReplacer("\r", "", "\n", " ").Replace(d.Subject))
	if d.Date.IsZero() {
		d.Date = time.Now()
	}
	h.SetDate(d.Date)
	if d.MessageID == "" {
		d.MessageID = NewMessageID(domainOf(d.From.Email))
	}
	h.SetMessageID(d.MessageID)
	if d.InReplyTo != "" {
		h.SetMsgIDList("In-Reply-To", []string{d.InReplyTo})
	}
	if len(d.References) > 0 {
		h.SetMsgIDList("References", d.References)
	}
	for _, kv := range d.Header {
		h.Set(kv[0], strings.NewReplacer("\r", "", "\n", "").Replace(kv[1]))
	}
	h.Set("MIME-Version", "1.0")

	var buf bytes.Buffer
	text := normalizeNewlines(d.Text)
	switch {
	case len(d.Files) == 0 && d.HTML == "":
		h.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
		w, err := gomail.CreateSingleInlineWriter(&buf, h)
		if err != nil {
			return nil, err
		}
		io.WriteString(w, text)
		if err := w.Close(); err != nil {
			return nil, err
		}
	case len(d.Files) == 0:
		w, err := gomail.CreateInlineWriter(&buf, h)
		if err != nil {
			return nil, err
		}
		if err := writeAlternatives(w, text, d.HTML); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	default:
		w, err := gomail.CreateWriter(&buf, h)
		if err != nil {
			return nil, err
		}
		iw, err := w.CreateInline()
		if err != nil {
			return nil, err
		}
		if err := writeAlternatives(iw, text, d.HTML); err != nil {
			return nil, err
		}
		iw.Close()
		for _, f := range d.Files {
			var ah gomail.AttachmentHeader
			ct := f.Type
			if ct == "" {
				ct = mime.TypeByExtension(extOf(f.Name))
			}
			if ct == "" {
				ct = "application/octet-stream"
			}
			ah.SetContentType(ct, nil)
			// RFC 2231 for names beyond ASCII; an encoded word inside a
			// quoted parameter is forbidden (RFC 2047 §5).
			ah.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.Name}))
			aw, err := w.CreateAttachment(ah)
			if err != nil {
				return nil, err
			}
			aw.Write(f.Data)
			aw.Close()
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	}
	return Fold(buf.Bytes()), nil
}

// Fold wraps header fields longer than 78 characters at whitespace, so
// lines with encoded words stay within RFC 2047's 76 characters and every
// line within RFC 5322 §2.1.1's 78 where a break is possible. Only the top
// header of raw changes; raw has CRLF line endings.
func Fold(raw []byte) []byte {
	end := bytes.Index(raw, []byte("\r\n\r\n"))
	if end < 0 {
		return raw
	}
	lines := bytes.Split(raw[:end], []byte("\r\n"))
	long := false
	for _, l := range lines {
		if len(l) > 78 {
			long = true
			break
		}
	}
	if !long {
		return raw
	}
	var out bytes.Buffer
	out.Grow(len(raw) + 64)
	for i, l := range lines {
		if i > 0 {
			out.WriteString("\r\n")
		}
		for len(l) > 78 {
			// The last space before column 78, but never the one that
			// separates the field name from its value.
			cut := bytes.LastIndexByte(l[:78], ' ')
			if colon := bytes.IndexByte(l, ':'); cut <= colon+1 {
				next := bytes.IndexByte(l[78:], ' ')
				if next < 0 {
					break
				}
				cut = 78 + next
			}
			out.Write(l[:cut])
			out.WriteString("\r\n")
			l = l[cut:] // keeps the space, which starts the continuation
		}
		out.Write(l)
	}
	out.Write(raw[end:])
	return out.Bytes()
}

func writeAlternatives(w *gomail.InlineWriter, text, html string) error {
	var th gomail.InlineHeader
	th.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
	pw, err := w.CreatePart(th)
	if err != nil {
		return err
	}
	io.WriteString(pw, text)
	pw.Close()
	if html == "" {
		return nil
	}
	var hh gomail.InlineHeader
	hh.SetContentType("text/html", map[string]string{"charset": "utf-8"})
	pw, err = w.CreatePart(hh)
	if err != nil {
		return err
	}
	io.WriteString(pw, normalizeNewlines(html))
	return pw.Close()
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}

func domainOf(email string) string {
	if i := strings.LastIndexByte(email, '@'); i >= 0 {
		return email[i+1:]
	}
	return "localhost"
}

func extOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i:]
	}
	return ""
}

// Rewrite replaces or removes top-level header fields of raw and returns the
// new message; the body is untouched. Fields in set replace every existing
// field of that name; names in drop are removed.
func Rewrite(raw []byte, set [][2]string, drop []string) ([]byte, error) {
	e, err := message.Read(bytes.NewReader(raw))
	if e == nil {
		return nil, err
	}
	remove := map[string]bool{}
	for _, d := range drop {
		remove[strings.ToLower(d)] = true
	}
	for _, kv := range set {
		remove[strings.ToLower(kv[0])] = true
	}
	headerEnd := bytes.Index(raw, []byte("\r\n\r\n"))
	sep := 4
	if headerEnd < 0 {
		headerEnd = bytes.Index(raw, []byte("\n\n"))
		sep = 2
	}
	if headerEnd < 0 {
		return nil, fmt.Errorf("rewrite: message has no body separator")
	}
	var out bytes.Buffer
	for _, kv := range set {
		var h message.Header
		h.SetText(kv[0], kv[1])
		f := h.Fields()
		f.Next()
		v, _ := f.Raw()
		out.Write(v)
	}
	// Copy the original header line by line, skipping removed fields with their continuations.
	lines := strings.SplitAfter(string(raw[:headerEnd]), "\n")
	skipping := false
	for _, l := range lines {
		if l == "" {
			continue
		}
		if l[0] == ' ' || l[0] == '\t' {
			if !skipping {
				out.WriteString(strings.TrimRight(l, "\r\n") + "\r\n")
			}
			continue
		}
		name, _, _ := strings.Cut(l, ":")
		skipping = remove[strings.ToLower(strings.TrimSpace(name))]
		if !skipping {
			out.WriteString(strings.TrimRight(l, "\r\n") + "\r\n")
		}
	}
	out.WriteString("\r\n")
	out.Write(raw[headerEnd+sep:])
	return out.Bytes(), nil
}

// FormatAddress renders "Name <email>".
func FormatAddress(name, email string) string {
	return (&mail.Address{Name: name, Address: email}).String()
}
