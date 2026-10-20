// Package feedback reads what the mail world reports back: delivery status
// notifications (bounces), abuse feedback reports (spam complaints),
// DMARC aggregate and TLS reports, and relay webhooks. It only parses; the
// engine decides what a bounce or complaint changes.
package feedback

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/msg"
)

// Recipient is one recipient's outcome in a bounce.
type Recipient struct {
	Email      string `json:"email"`
	Action     string `json:"action"` // failed, delayed, delivered, relayed, expanded
	Status     string `json:"status"` // e.g. 5.1.1
	Diagnostic string `json:"diagnostic,omitempty"`
}

// Hard reports a permanent failure.
func (r Recipient) Hard() bool {
	return r.Action == "failed" && strings.HasPrefix(r.Status, "5")
}

// DSN extracts recipients from a delivery status notification (RFC 3464),
// and the Message-ID of the original message when it is included.
func DSN(p *msg.Parsed) (recipients []Recipient, originalID string, ok bool) {
	for _, part := range p.Parts {
		switch part.Type {
		case "message/delivery-status", "message/global-delivery-status":
			recipients = parseStatus(part.Body)
			ok = true
		case "message/rfc822", "text/rfc822-headers", "message/global", "message/global-headers":
			if originalID == "" {
				originalID = headerID(part.Body)
			}
		}
	}
	return recipients, originalID, ok
}

func parseStatus(b []byte) []Recipient {
	var out []Recipient
	// Per-message fields, then one block per recipient, separated by blank lines.
	r := textproto.NewReader(bufio.NewReader(bytes.NewReader(append(bytes.TrimLeft(b, "\r\n"), "\r\n\r\n"...))))
	first := true
	for i := 0; i < 1000; i++ {
		h, err := r.ReadMIMEHeader()
		if len(h) > 0 && !first || len(h) > 0 && h.Get("Final-Recipient") != "" {
			rcpt := Recipient{
				Email:      addrField(h.Get("Final-Recipient")),
				Action:     strings.ToLower(strings.TrimSpace(h.Get("Action"))),
				Status:     firstField(h.Get("Status")),
				Diagnostic: strings.TrimSpace(h.Get("Diagnostic-Code")),
			}
			if rcpt.Email == "" {
				rcpt.Email = addrField(h.Get("Original-Recipient"))
			}
			if rcpt.Email != "" {
				out = append(out, rcpt)
			}
		}
		if len(h) > 0 {
			first = false
		}
		if err != nil {
			break
		}
	}
	return out
}

func firstField(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

// addrField reads "rfc822; user@example.com".
func addrField(v string) string {
	if _, a, ok := strings.Cut(v, ";"); ok {
		v = a
	}
	v = strings.Trim(strings.TrimSpace(v), "<>")
	if !strings.Contains(v, "@") {
		return ""
	}
	return strings.ToLower(v)
}

func headerID(b []byte) string {
	r := textproto.NewReader(bufio.NewReader(bytes.NewReader(b)))
	h, _ := r.ReadMIMEHeader()
	return strings.Trim(strings.TrimSpace(h.Get("Message-Id")), "<>")
}

// Complaint is an abuse report (RFC 5965).
type Complaint struct {
	Type       string // abuse, fraud, ...
	Recipient  string // the complaining address, when the reporter discloses it
	OriginalID string // Message-ID of the reported message
	ListUnsub  string // its List-Unsubscribe header, which carries lfr's token
}

// ARF extracts a feedback report.
func ARF(p *msg.Parsed) (Complaint, bool) {
	var c Complaint
	found := false
	for _, part := range p.Parts {
		switch part.Type {
		case "message/feedback-report":
			found = true
			r := textproto.NewReader(bufio.NewReader(bytes.NewReader(append(part.Body, "\r\n\r\n"...))))
			h, _ := r.ReadMIMEHeader()
			c.Type = strings.ToLower(strings.TrimSpace(h.Get("Feedback-Type")))
			c.Recipient = addrField(h.Get("Original-Rcpt-To"))
		case "message/rfc822", "text/rfc822-headers":
			r := textproto.NewReader(bufio.NewReader(bytes.NewReader(part.Body)))
			h, _ := r.ReadMIMEHeader()
			c.OriginalID = strings.Trim(strings.TrimSpace(h.Get("Message-Id")), "<>")
			c.ListUnsub = h.Get("List-Unsubscribe")
			if c.Recipient == "" {
				if to := addrField(h.Get("To")); to != "" {
					c.Recipient = to
				}
			}
		}
	}
	return c, found
}

// Report is a summarized aggregate report.
type Report struct {
	Kind   string          `json:"kind"` // dmarc, tlsrpt
	Org    string          `json:"org"`
	ID     string          `json:"report_id"`
	Domain string          `json:"domain"`
	Begin  time.Time       `json:"begin"`
	End    time.Time       `json:"end"`
	Pass   int             `json:"pass"`
	Fail   int             `json:"fail"`
	Rows   json.RawMessage `json:"rows"`
}

// ErrNoReport means the message carries no readable report.
var ErrNoReport = errors.New("no report found")

const maxReport = 20 << 20

// unpack returns the report document inside a part: plain, gzip or zip.
func unpack(name string, b []byte) []byte {
	switch {
	case len(b) > 2 && b[0] == 0x1f && b[1] == 0x8b:
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil
		}
		out, _ := io.ReadAll(io.LimitReader(zr, maxReport))
		return out
	case len(b) > 4 && string(b[:4]) == "PK\x03\x04":
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil || len(zr.File) == 0 {
			return nil
		}
		f, err := zr.File[0].Open()
		if err != nil {
			return nil
		}
		defer f.Close()
		out, _ := io.ReadAll(io.LimitReader(f, maxReport))
		return out
	}
	return b
}

type dmarcXML struct {
	Meta struct {
		Org   string `xml:"org_name"`
		ID    string `xml:"report_id"`
		Range struct {
			Begin int64 `xml:"begin"`
			End   int64 `xml:"end"`
		} `xml:"date_range"`
	} `xml:"report_metadata"`
	Policy struct {
		Domain string `xml:"domain"`
	} `xml:"policy_published"`
	Records []struct {
		Row struct {
			IP     string `xml:"source_ip"`
			Count  int    `xml:"count"`
			Policy struct {
				Disposition string `xml:"disposition"`
				DKIM        string `xml:"dkim"`
				SPF         string `xml:"spf"`
			} `xml:"policy_evaluated"`
		} `xml:"row"`
		Identifiers struct {
			HeaderFrom string `xml:"header_from"`
		} `xml:"identifiers"`
	} `xml:"record"`
}

// DMARCRow is one source in a DMARC aggregate report.
type DMARCRow struct {
	IP          string `json:"ip"`
	Count       int    `json:"count"`
	DKIM        string `json:"dkim"`
	SPF         string `json:"spf"`
	Disposition string `json:"disposition"`
}

// DMARC reads an aggregate report from a message.
func DMARC(p *msg.Parsed) (Report, error) {
	for _, part := range p.Parts {
		doc := unpack(part.Name, part.Body)
		if !bytes.Contains(doc, []byte("<feedback")) {
			continue
		}
		var x dmarcXML
		if err := xml.Unmarshal(doc, &x); err != nil {
			continue
		}
		r := Report{Kind: "dmarc", Org: x.Meta.Org, ID: x.Meta.ID, Domain: strings.ToLower(x.Policy.Domain),
			Begin: time.Unix(x.Meta.Range.Begin, 0).UTC(), End: time.Unix(x.Meta.Range.End, 0).UTC()}
		var rows []DMARCRow
		for _, rec := range x.Records {
			row := DMARCRow{IP: rec.Row.IP, Count: rec.Row.Count, DKIM: rec.Row.Policy.DKIM, SPF: rec.Row.Policy.SPF, Disposition: rec.Row.Policy.Disposition}
			rows = append(rows, row)
			if row.DKIM == "pass" || row.SPF == "pass" {
				r.Pass += row.Count
			} else {
				r.Fail += row.Count
			}
		}
		r.Rows, _ = json.Marshal(rows)
		if r.Org == "" || r.ID == "" {
			continue
		}
		return r, nil
	}
	return Report{}, ErrNoReport
}

type tlsJSON struct {
	Org   string `json:"organization-name"`
	ID    string `json:"report-id"`
	Range struct {
		Start time.Time `json:"start-datetime"`
		End   time.Time `json:"end-datetime"`
	} `json:"date-range"`
	Policies []struct {
		Policy struct {
			Type   string `json:"policy-type"`
			Domain string `json:"policy-domain"`
		} `json:"policy"`
		Summary struct {
			OK   int `json:"total-successful-session-count"`
			Fail int `json:"total-failure-session-count"`
		} `json:"summary"`
		Failures json.RawMessage `json:"failure-details"`
	} `json:"policies"`
}

// TLSRPT reads an SMTP TLS report (RFC 8460) from a message.
func TLSRPT(p *msg.Parsed) (Report, error) {
	for _, part := range p.Parts {
		doc := unpack(part.Name, part.Body)
		if !bytes.Contains(doc, []byte("organization-name")) {
			continue
		}
		var x tlsJSON
		if err := json.Unmarshal(doc, &x); err != nil {
			continue
		}
		r := Report{Kind: "tlsrpt", Org: x.Org, ID: x.ID, Begin: x.Range.Start.UTC(), End: x.Range.End.UTC()}
		for _, pol := range x.Policies {
			if r.Domain == "" {
				r.Domain = strings.ToLower(pol.Policy.Domain)
			}
			r.Pass += pol.Summary.OK
			r.Fail += pol.Summary.Fail
		}
		r.Rows, _ = json.Marshal(x.Policies)
		if r.Org == "" || r.ID == "" {
			continue
		}
		return r, nil
	}
	return Report{}, ErrNoReport
}

// Event is a bounce, complaint or unsubscribe reported by a relay webhook.
type Event struct {
	Kind   string `json:"kind"` // bounce, complaint, unsubscribe
	Email  string `json:"email"`
	Hard   bool   `json:"hard"`
	Note   string `json:"note,omitempty"`
	Source string `json:"source,omitempty"` // the envelope sender of the original message, when the relay reports it
}

// Webhook reads an SMTP2GO or generic {"type","email"} notification, as JSON
// or form data. Amazon SES notifications arrive signed through SNS and are
// read with SNSVerifier and SESEvents instead.
func Webhook(contentType string, body []byte) ([]Event, error) {
	body = bytes.TrimSpace(body)
	if strings.Contains(contentType, "application/x-www-form-urlencoded") || len(body) > 0 && body[0] != '{' && body[0] != '[' {
		v, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}
		m := map[string]any{}
		for k := range v {
			m[k] = v.Get(k)
		}
		return generic(m), nil
	}
	var top map[string]any
	if err := json.Unmarshal(body, &top); err != nil {
		var list []map[string]any
		if json.Unmarshal(body, &list) == nil {
			var out []Event
			for _, m := range list {
				out = append(out, generic(m)...)
			}
			return out, nil
		}
		return nil, err
	}
	return generic(top), nil
}

func ses(m map[string]any) []Event {
	kind := str(m, "notificationType")
	if kind == "" {
		kind = str(m, "eventType")
	}
	mail, _ := m["mail"].(map[string]any)
	source := strings.ToLower(str(mail, "source"))
	var out []Event
	switch kind {
	case "Bounce":
		b, _ := m["bounce"].(map[string]any)
		hard := str(b, "bounceType") == "Permanent"
		for _, r := range list(b, "bouncedRecipients") {
			out = append(out, Event{Kind: "bounce", Email: strings.ToLower(str(r, "emailAddress")), Hard: hard, Note: str(r, "diagnosticCode")})
		}
	case "Complaint":
		c, _ := m["complaint"].(map[string]any)
		for _, r := range list(c, "complainedRecipients") {
			out = append(out, Event{Kind: "complaint", Email: strings.ToLower(str(r, "emailAddress")), Note: str(c, "complaintFeedbackType")})
		}
	}
	for i := range out {
		out[i].Source = source
	}
	return out
}

func generic(m map[string]any) []Event {
	kind := strings.ToLower(first(m, "type", "event", "event_type", "Type"))
	email := strings.ToLower(first(m, "email", "rcpt", "recipient", "to", "address"))
	if email == "" || !strings.Contains(email, "@") {
		return nil
	}
	e := Event{Email: email}
	switch {
	case strings.Contains(kind, "bounce") || kind == "failed" || kind == "dropped":
		e.Kind = "bounce"
		bt := strings.ToLower(first(m, "bounce", "bounce_type", "bounceType", "severity"))
		e.Hard = strings.Contains(kind, "hard") || bt == "hard" || bt == "permanent" || bt == "" && !strings.Contains(kind, "soft")
	case strings.Contains(kind, "spam") || strings.Contains(kind, "complain") || strings.Contains(kind, "abuse"):
		e.Kind = "complaint"
	case strings.Contains(kind, "unsub"):
		e.Kind = "unsubscribe"
	default:
		return nil
	}
	e.Note = first(m, "reason", "description", "message", "diagnostic")
	return []Event{e}
}

func first(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := str(m, k); s != "" {
			return s
		}
	}
	return ""
}

func str(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	switch v := m[k].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

func list(m map[string]any, k string) []map[string]any {
	raw, _ := m[k].([]any)
	var out []map[string]any
	for _, x := range raw {
		if mm, ok := x.(map[string]any); ok {
			out = append(out, mm)
		}
	}
	return out
}
