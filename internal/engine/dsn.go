package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Anonyfox/lfr/internal/addr"
	"github.com/Anonyfox/lfr/internal/mailbox"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/outbound"
)

// onFailed handles a delivery that failed for good.
func (e *Engine) onFailed(ctx context.Context, r outbound.Row, f *outbound.Failure) {
	if row, ok := e.outboundRow(ctx, r.ID); ok && row.contactID != 0 && f.Bounce {
		e.suppressAfterBounce(ctx, row, f.Error())
	}
	if r.Kind == outbound.KindMail {
		status := f.Enhanced
		if status == "" {
			status = "5.0.0"
		}
		e.notifySender(ctx, r, status, f.Error())
	}
}

// onGiveUp handles a delivery the queue stopped retrying.
func (e *Engine) onGiveUp(ctx context.Context, r outbound.Row, reason string) {
	if row, ok := e.outboundRow(ctx, r.ID); ok && row.contactID != 0 {
		e.softBounced(ctx, row, reason)
	}
	if r.Kind == outbound.KindMail {
		e.notifySender(ctx, r, "4.4.7", reason)
	}
}

// notifySender files a delivery status notification (RFC 3464) into the
// mailbox that sent or forwarded a message lfr could not deliver, as RFC 5321
// §6.1 requires once a message was accepted. It is filed, not sent, so it
// can never bounce or loop.
func (e *Engine) notifySender(ctx context.Context, r outbound.Row, status, diagnostic string) {
	id, ok, err := e.Mailbox.Resolve(ctx, r.EnvFrom)
	if err != nil || !ok {
		return
	}
	original, err := e.Blobs.Get(r.Blob)
	if err != nil {
		original = nil
	}
	header := original
	if i := bytes.Index(original, []byte("\r\n\r\n")); i >= 0 {
		header = original[:i+2]
	}
	host := e.Hostname(ctx)
	now := e.Opts.Now()
	b := make([]byte, 12)
	rand.Read(b)
	boundary := "dsn-" + hex.EncodeToString(b)
	action := "failed"
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", msg.FormatAddress("Mail Delivery System", "postmaster@"+addr.DomainOf(r.EnvFrom)))
	fmt.Fprintf(&buf, "To: %s\r\n", r.EnvFrom)
	fmt.Fprintf(&buf, "Subject: Undelivered mail to %s\r\n", r.Rcpt)
	fmt.Fprintf(&buf, "Date: %s\r\n", now.Format(time.RFC1123Z))
	fmt.Fprintf(&buf, "Message-ID: <%s>\r\n", msg.NewMessageID(host))
	buf.WriteString("Auto-Submitted: auto-replied\r\nMIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: multipart/report; report-type=delivery-status; boundary=%q\r\n\r\n", boundary)
	fmt.Fprintf(&buf, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n", boundary)
	fmt.Fprintf(&buf, "Your message to %s could not be delivered.\r\n\r\nThe receiving side said: %s\r\n\r\n", r.Rcpt, diagnostic)
	fmt.Fprintf(&buf, "--%s\r\nContent-Type: message/delivery-status\r\n\r\n", boundary)
	fmt.Fprintf(&buf, "Reporting-MTA: dns; %s\r\nArrival-Date: %s\r\n\r\n", host, r.Created.Format(time.RFC1123Z))
	fmt.Fprintf(&buf, "Final-Recipient: rfc822; %s\r\nAction: %s\r\nStatus: %s\r\nDiagnostic-Code: smtp; %s\r\n\r\n", r.Rcpt, action, status, oneLine(diagnostic))
	fmt.Fprintf(&buf, "--%s\r\nContent-Type: text/rfc822-headers\r\n\r\n", boundary)
	buf.Write(header)
	fmt.Fprintf(&buf, "\r\n--%s--\r\n", boundary)
	if _, err := e.Mailbox.Deliver(ctx, mailbox.Delivery{MailboxID: id, Folder: "inbox", Raw: buf.Bytes()}); err != nil {
		e.Log.Warn("delivery notice", "mailbox", r.EnvFrom, "err", err)
	}
}

func oneLine(s string) string {
	return string(bytes.Join(bytes.Fields([]byte(s)), []byte(" ")))
}
