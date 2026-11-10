package msg

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestUnnamedReportPartsRemainDownloadable(t *testing.T) {
	for _, tc := range []struct{ typ, name string }{
		{"message/delivery-status", "delivery-status.txt"},
		{"text/rfc822-headers", "original-headers.txt"},
		{"message/feedback-report", "feedback-report.txt"},
		{"message/rfc822", "original-message.eml"},
		{"application/x-lfr-unknown", "part1.bin"},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			raw := []byte("Subject: Report\r\nContent-Type: " + tc.typ + "\r\n\r\nOriginal report content")
			p := Parse(raw)
			if len(p.Attachments) != 1 || p.Attachments[0].Name != tc.name {
				t.Fatalf("parts: %+v", p.Attachments)
			}
			a, b, ok := AttachmentData(raw, 0)
			if !ok || a.Name != tc.name || string(b) != "Original report content" {
				t.Fatalf("download: %+v %q %v", a, b, ok)
			}
		})
	}
}

func TestLegacyHeaderCharsetAndBoundedUTF8Text(t *testing.T) {
	p := Parse([]byte("From: =?iso-8859-1?Q?J=F6rg?= <jo@example.org>\r\nSubject: hello\r\n\r\nbody"))
	if p.From.Name != "Jörg" {
		t.Fatalf("legacy display name: %+v", p.From)
	}
	text := strings.Repeat("a", maxText-1) + "ä" + strings.Repeat("b", 20)
	p = Parse([]byte("Subject: bounded\r\n\r\n" + text))
	if len(p.Text) > maxText || !utf8.ValidString(p.Text) || strings.HasSuffix(p.Text, "�") {
		t.Fatalf("invalid truncation: bytes=%d", len(p.Text))
	}
}
