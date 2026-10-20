package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/mailbox"
)

func files(c *call) ([]api.File, error) {
	var out []api.File
	total := 0
	for _, p := range c.all("attach") {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, api.File{Name: filepath.Base(p), Data: b})
		total += len(b)
	}
	// Base64 grows attachments by a third; refuse before uploading them.
	if total*4/3 > mailbox.MaxMessage {
		return nil, usage("the attachments come to %d MB encoded; Gmail accepts at most 25 MB and Outlook 20 MB, so the mail would bounce. Share large files as a link", total*4/3>>20)
	}
	return out, nil
}

// --- lifecycle ---

func printMessages(c *call, ms []mailbox.Summary, showBox bool) {
	var rows [][]string
	for _, m := range ms {
		flags := ""
		if !m.Seen {
			flags += "●"
		}
		if m.Flagged {
			flags += "★"
		}
		if m.Answered {
			flags += "↩"
		}
		if m.Attachments > 0 {
			flags += "📎"
		}
		from := m.From
		if m.FromName != "" {
			from = m.FromName
		}
		if m.Folder == "sent" && len(m.To) > 0 {
			from = "→ " + m.To[0]
		}
		row := []string{strconv.FormatInt(m.ID, 10), flags, ago(m.Date), cut(from, 26), cut(m.Subject, 60)}
		if showBox {
			row = append(row, m.Mailbox)
		}
		rows = append(rows, row)
	}
	head := []string{"id", "", "date", "from", "subject"}
	if showBox {
		head = append(head, "mailbox")
	}
	table(c.env.Stdout, head, rows)
}

func runMailList(c *call) error {
	q := api.MailList{Folder: c.str("folder"), Unread: c.has("unread"), Thread: c.str("thread"), Search: c.str("search")}
	if len(c.args) > 1 {
		return usage("one mailbox at most")
	}
	if len(c.args) == 1 {
		q.Mailbox = c.args[0]
	}
	var err error
	if q.Limit, err = intFlag(c, "limit"); err != nil {
		return err
	}
	if c.has("before") {
		if q.Before, err = id(c.str("before")); err != nil {
			return err
		}
	}
	var ms []mailbox.Summary
	if err := c.do(api.OpMailList, q, &ms); err != nil {
		return err
	}
	return c.result(ms, func() {
		if len(ms) == 0 {
			c.printf("No messages.\n")
			return
		}
		printMessages(c, ms, q.Mailbox == "" || q.Search != "")
		c.next("lfr mail read <id>")
	})
}

func runMailRead(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	if c.has("attachment") {
		c.args = append(c.args, c.str("attachment"))
		return runMailAttachment(c)
	}
	n, err := id(c.args[0])
	if err != nil {
		return err
	}
	if c.has("raw") {
		var raw map[string]string
		if err := c.do(api.OpMailRead, api.MailRead{ID: n, Raw: true}, &raw); err != nil {
			return err
		}
		return c.result(raw, func() { c.printf("%s", raw["raw"]) })
	}
	var m mailbox.Message
	if err := c.do(api.OpMailRead, api.MailRead{ID: n, KeepUnread: c.has("keep-unread")}, &m); err != nil {
		return err
	}
	return c.result(m, func() {
		from := m.From
		if m.FromName != "" {
			from = m.FromName + " <" + m.From + ">"
		}
		kv(c.env.Stdout, "id", strconv.FormatInt(m.ID, 10), "mailbox", m.Mailbox+" ("+m.Folder+")", "from", from, "reply-to", m.ReplyTo,
			"to", strings.Join(m.To, ", "), "cc", strings.Join(m.Cc, ", "), "date", when(m.Date), "subject", m.Subject)
		if m.SenderCheck != "" {
			c.printf("sender:   %s\n", m.SenderCheck)
		}
		for _, ev := range m.Events {
			c.printf("event:    %s\n", ev)
		}
		if m.Spam >= 0.3 {
			c.printf("spam:     %.0f%%\n", m.Spam*100)
		}
		for _, a := range m.Files {
			c.printf("attachment %d: %s (%s, %d bytes)\n", a.Index, a.Name, a.Type, a.Size)
		}
		c.printf("\n")
		if c.has("html") && m.HTML != "" {
			c.printf("%s\n", m.HTML)
		} else {
			c.printf("%s\n", strings.TrimRight(m.Text, "\n"))
		}
		c.next("lfr mail reply %d --md -   (or lfr mail mv %d archive | spam)", m.ID, m.ID)
	})
}

func runMailSend(c *call) error {
	if len(c.args) > 0 {
		return usage("unexpected argument %q; pass the body with --md, --text or --html", c.args[0])
	}
	a := api.MailSend{From: c.str("from"), To: c.all("to"), Cc: c.all("cc"), Bcc: c.all("bcc"), Subject: c.str("subject"), ReplyTo: c.str("reply-to")}
	if a.From == "" || len(a.To)+len(a.Cc)+len(a.Bcc) == 0 || a.Subject == "" {
		return usage("--from, --to and --subject are required")
	}
	var err error
	switch {
	case c.has("md"):
		a.Markdown, err = readArg(c, c.str("md"))
	case c.has("text") || c.has("html"):
		if c.has("text") {
			if a.Text, err = readArg(c, c.str("text")); err != nil {
				return err
			}
		}
		if c.has("html") {
			a.HTML, err = readArg(c, c.str("html"))
		}
	default:
		return usage("give the body with --md <file>, --text <file> or --html <file> (- reads stdin)")
	}
	if err != nil {
		return err
	}
	if a.Files, err = files(c); err != nil {
		return err
	}
	var s api.Sent
	if err := c.do(api.OpMailSend, a, &s); err != nil {
		return err
	}
	return c.result(s, func() {
		c.printf("Queued for %s (sent copy: message %d)\n", count(s.Queued, "recipient", "recipients"), s.ID)
		c.next("lfr events --follow --kind sent --kind failed --kind deferred shows delivery")
	})
}

func runMailReply(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	n, err := id(c.args[0])
	if err != nil {
		return err
	}
	a := api.MailReply{ID: n, All: c.has("all"), NoQuote: c.has("no-quote")}
	switch {
	case c.has("md"):
		a.Markdown, err = readArg(c, c.str("md"))
	case c.has("text"):
		a.Text, err = readArg(c, c.str("text"))
	default:
		return usage("give the reply with --md <file> or --text <file> (- reads stdin)")
	}
	if err != nil {
		return err
	}
	if a.Files, err = files(c); err != nil {
		return err
	}
	var s api.Sent
	if err := c.do(api.OpMailReply, a, &s); err != nil {
		return err
	}
	return c.result(s, func() { c.printf("Reply queued (sent copy: message %d)\n", s.ID) })
}

func runMailMove(c *call) error {
	if err := need(c, 2); err != nil {
		return err
	}
	ns, err := ids(c.args[:len(c.args)-1])
	if err != nil {
		return err
	}
	var res map[string]any
	if err := c.do(api.OpMailMove, api.MailMove{IDs: ns, Folder: c.args[len(c.args)-1]}, &res); err != nil {
		return err
	}
	return c.result(res, func() {
		c.printf("Moved %v to %v", res["moved"], res["folder"])
		if t, _ := res["trained"].(float64); t > 0 {
			c.printf(" (the spam filter learned from %v)", res["trained"])
		}
		c.printf("\n")
	})
}

func runMailFlag(c *call) error {
	if err := need(c, 2); err != nil {
		return err
	}
	ns, err := ids(c.args[:len(c.args)-1])
	if err != nil {
		return err
	}
	f := c.args[len(c.args)-1]
	on := true
	switch f {
	case "unseen", "unread":
		f, on = "seen", false
	case "unflagged":
		f, on = "flagged", false
	case "read":
		f = "seen"
	}
	if err := c.do(api.OpMailFlag, api.MailFlag{IDs: ns, Flag: f, On: on}, nil); err != nil {
		return err
	}
	return c.result(map[string]any{"ids": ns, "flag": f, "on": on}, func() { c.printf("Updated %d messages\n", len(ns)) })
}

func runMailDelete(c *call) error {
	if err := need(c, 1); err != nil {
		return err
	}
	ns, err := ids(c.args)
	if err != nil {
		return err
	}
	if err := c.do(api.OpMailDelete, api.MailDelete{IDs: ns, Purge: c.has("purge")}, nil); err != nil {
		return err
	}
	return c.result(map[string]any{"deleted": ns}, func() { c.printf("Deleted %d messages\n", len(ns)) })
}

func runMailAttachment(c *call) error {
	if err := exactly(c, 2); err != nil {
		return err
	}
	n, err := id(c.args[0])
	if err != nil {
		return err
	}
	idx, err := strconv.Atoi(c.args[1])
	if err != nil {
		return usage("%q is not an attachment index", c.args[1])
	}
	var f api.File
	if err := c.do(api.OpMailAttachment, api.Attachment{ID: n, Index: idx}, &f); err != nil {
		return err
	}
	out := c.str("out")
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC // --out: the user chose the path
	if out == "" {
		// The name comes from whoever sent the mail: never a path, never a
		// hidden file such as .zshrc, never over an existing file.
		out = savedName(f.Name, n, idx)
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
		for i := 1; ; i++ {
			if _, err := os.Lstat(out); os.IsNotExist(err) {
				break
			}
			ext := filepath.Ext(out)
			out = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(savedName(f.Name, n, idx), ext), i, ext)
		}
	}
	file, err := os.OpenFile(out, flags, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(f.Data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return c.result(map[string]any{"file": out, "bytes": len(f.Data), "type": f.Type}, func() { c.printf("Saved %s (%d bytes)\n", out, len(f.Data)) })
}

// savedName makes a sender-chosen attachment name safe to save in the
// current directory.
func savedName(name string, msgID int64, idx int) string {
	name = strings.TrimLeft(filepath.Base(strings.ReplaceAll(name, "\\", "/")), ". ")
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '/' || r == ':' {
			return '_'
		}
		return r
	}, name)
	if name == "" {
		return fmt.Sprintf("attachment-%d-%d", msgID, idx)
	}
	return name
}

func runMailSpam(c *call) error {
	if c.has("reset") {
		return runSpamReset(c)
	}
	return runSpamStats(c)
}
