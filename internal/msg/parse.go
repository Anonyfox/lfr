// Package msg reads and writes RFC 5322 messages: parsing what arrives
// (tolerant of the malformed mail spam and old clients produce), composing
// what lfr sends, and turning HTML into readable text.
package msg

import (
	"bytes"
	"io"
	"mime"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset" // decode non-UTF-8 charsets
)

// Address is a display name and address.
type Address struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

func (a Address) String() string {
	return (&mail.Address{Name: a.Name, Address: a.Email}).String()
}

// Attachment describes one attachment of a parsed message.
type Attachment struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Size  int    `json:"size"`
}

// Parsed is the readable content of a message.
type Parsed struct {
	From        Address      `json:"from"`
	To          []Address    `json:"to,omitempty"`
	Cc          []Address    `json:"cc,omitempty"`
	ReplyTo     []Address    `json:"reply_to,omitempty"`
	Subject     string       `json:"subject"`
	Date        time.Time    `json:"date"`
	MessageID   string       `json:"message_id,omitempty"`
	InReplyTo   string       `json:"in_reply_to,omitempty"`
	References  []string     `json:"references,omitempty"`
	Text        string       `json:"text"`
	HTML        string       `json:"html,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Header      Header       `json:"-"`
	Parts       []Part       `json:"-"` // every leaf part, for reports and bounces
}

// Part is one leaf of the MIME tree.
type Part struct {
	Type        string
	Disposition string
	Name        string
	Body        []byte // decoded
}

// Header is the top-level header, keys canonical.
type Header struct{ fields message.Header }

// Get returns the first value of a header field, decoded.
func (h Header) Get(key string) string {
	v, err := h.fields.Text(key)
	if err != nil {
		return h.fields.Get(key)
	}
	return v
}

// Has reports whether the field is present.
func (h Header) Has(key string) bool { return h.fields.Has(key) }

// Values returns every raw value of a field.
func (h Header) Values(key string) []string { return h.fields.Values(key) }

// Limits keep parsing bounded whatever arrives.
const (
	maxParts    = 200
	maxPartSize = 32 << 20
	maxText     = 1 << 20
)

// Parse reads raw. It never fails on content: unreadable parts are skipped
// and a message without a usable structure is read as plain text.
func Parse(raw []byte) *Parsed {
	p := &Parsed{}
	e, err := message.ReadWithOptions(bytes.NewReader(raw), &message.ReadOptions{MaxHeaderBytes: 1 << 20})
	if e == nil {
		// No parsable header at all: treat everything as text.
		p.Text = clean(string(raw))
		_ = err
		return p
	}
	p.Header = Header{e.Header}
	p.readHeader()
	p.walk(e, 0)
	for _, part := range p.Parts {
		if p.Text == "" && part.Type == "text/plain" && part.Disposition != "attachment" {
			p.Text = clean(string(part.Body))
		}
		if p.HTML == "" && part.Type == "text/html" && part.Disposition != "attachment" {
			p.HTML = string(part.Body)
		}
	}
	if p.Text == "" && p.HTML != "" {
		p.Text = HTMLToText(p.HTML)
	}
	if len(p.Text) > maxText {
		p.Text = truncate(p.Text, maxText)
	}
	return p
}

func (p *Parsed) readHeader() {
	h := p.Header
	if l := addressList(h.fields, "From"); len(l) > 0 {
		p.From = l[0]
	}
	p.To = addressList(h.fields, "To")
	p.Cc = addressList(h.fields, "Cc")
	p.ReplyTo = addressList(h.fields, "Reply-To")
	p.Subject = clean(h.Get("Subject"))
	if d, err := mail.ParseDate(h.fields.Get("Date")); err == nil {
		p.Date = d.UTC()
	}
	p.MessageID = firstID(h.fields.Get("Message-Id"))
	p.InReplyTo = firstID(h.fields.Get("In-Reply-To"))
	p.References = ids(h.fields.Get("References"))
}

func (p *Parsed) walk(e *message.Entity, depth int) {
	if depth > 20 || len(p.Parts) >= maxParts {
		return
	}
	if mr := e.MultipartReader(); mr != nil {
		for len(p.Parts) < maxParts {
			child, err := mr.NextPart()
			if child == nil || err != nil && !message.IsUnknownCharset(err) && !message.IsUnknownEncoding(err) {
				return
			}
			p.walk(child, depth+1)
		}
		return
	}
	ct, params, err := e.Header.ContentType()
	if err != nil || ct == "" {
		ct = "text/plain"
	}
	disp, dparams, _ := e.Header.ContentDisposition()
	name := dparams["filename"]
	if name == "" {
		name = params["name"]
	}
	if name != "" {
		if dec, err := new(mime.WordDecoder).DecodeHeader(name); err == nil {
			name = dec
		}
	}
	body, _ := io.ReadAll(io.LimitReader(e.Body, maxPartSize))
	part := Part{Type: strings.ToLower(ct), Disposition: strings.ToLower(disp), Name: name, Body: body}
	p.Parts = append(p.Parts, part)
	isBody := (part.Type == "text/plain" || part.Type == "text/html") && part.Disposition != "attachment" && name == ""
	if !isBody && !strings.HasPrefix(part.Type, "multipart/") {
		if name == "" {
			name = partName(part.Type, len(p.Parts))
		}
		p.Attachments = append(p.Attachments, Attachment{Index: len(p.Attachments), Name: name, Type: part.Type, Size: len(body)})
	}
}

// AttachmentData returns the content of attachment i of raw.
func AttachmentData(raw []byte, i int) (Attachment, []byte, bool) {
	p := Parse(raw)
	if i < 0 || i >= len(p.Attachments) {
		return Attachment{}, nil, false
	}
	n := -1
	for _, part := range p.Parts {
		isBody := (part.Type == "text/plain" || part.Type == "text/html") && part.Disposition != "attachment" && part.Name == ""
		if isBody || strings.HasPrefix(part.Type, "multipart/") {
			continue
		}
		n++
		if n == i {
			return p.Attachments[i], part.Body, true
		}
	}
	return Attachment{}, nil, false
}

func addressList(h message.Header, key string) []Address {
	v := h.Get(key)
	if v == "" {
		return nil
	}
	dec := &mail.AddressParser{WordDecoder: &mime.WordDecoder{CharsetReader: charsetReader}}
	list, err := dec.ParseList(v)
	if err != nil {
		// Salvage the first thing that looks like an address.
		if a, err := dec.Parse(v); err == nil {
			list = []*mail.Address{a}
		} else if s := looseAddress(v); s != "" {
			return []Address{{Email: s}}
		} else {
			return nil
		}
	}
	out := make([]Address, 0, len(list))
	for _, a := range list {
		out = append(out, Address{Name: clean(a.Name), Email: strings.ToLower(a.Address)})
	}
	return out
}

func charsetReader(charset string, r io.Reader) (io.Reader, error) {
	return message.CharsetReader(charset, r)
}

// looseAddress finds the first token with an @ in a broken address header.
func looseAddress(s string) string {
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(" <>,;\"'()", r) }) {
		if i := strings.IndexByte(f, '@'); i > 0 && i < len(f)-1 {
			return strings.ToLower(f)
		}
	}
	return ""
}

func ids(s string) []string {
	var out []string
	for {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			break
		}
		j := strings.IndexByte(s[i:], '>')
		if j < 0 {
			break
		}
		if id := strings.TrimSpace(s[i+1 : i+j]); id != "" && len(id) < 1000 {
			out = append(out, id)
		}
		s = s[i+j+1:]
		if len(out) >= 100 {
			break
		}
	}
	if len(out) == 0 {
		for _, f := range strings.Fields(s) {
			if strings.Contains(f, "@") && len(f) < 1000 {
				out = append(out, strings.Trim(f, "<>"))
			}
		}
	}
	return out
}

func firstID(s string) string {
	if l := ids(s); len(l) > 0 {
		return l[0]
	}
	return ""
}

// clean makes s valid UTF-8 without control characters other than newlines and tabs.
func clean(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r == '\r' || r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// Snippet is a one-line preview of text.
func Snippet(text string, n int) string {
	s := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n])) + "…"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// partName names an attachment that came without a name, by what it holds.
func partName(ct string, n int) string {
	switch strings.ToLower(ct) {
	case "message/delivery-status", "message/global-delivery-status":
		return "delivery-status.txt"
	case "text/rfc822-headers", "message/global-headers":
		return "original-headers.txt"
	case "message/feedback-report":
		return "feedback-report.txt"
	case "message/rfc822", "message/global":
		return "original-message.eml"
	}
	return "part" + itoa(n) + ext(ct)
}

func ext(ct string) string {
	if exts, _ := mime.ExtensionsByType(ct); len(exts) > 0 {
		return exts[0]
	}
	return ".bin"
}
