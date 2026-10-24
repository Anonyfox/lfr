// Package api defines the arguments and results of every control operation,
// shared by the CLI and the server so both always agree.
package api

import (
	"encoding/json"
	"time"

	"github.com/Anonyfox/lfr/internal/domains"
	"github.com/Anonyfox/lfr/internal/issues"
	"github.com/Anonyfox/lfr/internal/outbound"
)

// Op names.
const (
	OpStatus    = "status"
	OpDoctor    = "doctor"
	OpConfig    = "config.list"
	OpConfigGet = "config.get"
	OpConfigSet = "config.set"

	OpDomainAdd    = "domain.add"
	OpDomainList   = "domain.list"
	OpDomainRemove = "domain.remove"
	OpDomainDNS    = "domain.dns"
	OpDomainRotate = "domain.rotate"

	OpMailboxAdd    = "mailbox.add"
	OpMailboxList   = "mailbox.list"
	OpMailboxRemove = "mailbox.remove"
	OpMailboxRename = "mailbox.rename"
	OpMailboxSet    = "mailbox.set"
	OpAliasAdd      = "alias.add"
	OpAliasRemove   = "alias.remove"
	OpCatchall      = "catchall.set"

	OpMailList       = "mail.list"
	OpMailRead       = "mail.read"
	OpMailMove       = "mail.move"
	OpMailFlag       = "mail.flag"
	OpMailDelete     = "mail.delete"
	OpMailSend       = "mail.send"
	OpMailReply      = "mail.reply"
	OpMailAttachment = "mail.attachment"
	OpWatch          = "watch"
	OpEvents         = "events.list"

	OpListAdd     = "list.add"
	OpListList    = "list.list"
	OpListText    = "list.text"
	OpListTextSet = "list.text.set"
	OpListShow    = "list.show"
	OpListSet     = "list.set"
	OpListRemove  = "list.remove"
	OpListPause   = "list.pause"

	OpSubAdd        = "sub.add"
	OpSubList       = "sub.list"
	OpSubRemove     = "sub.remove"
	OpSubImport     = "sub.import"
	OpSubExport     = "sub.export"
	OpContactShow   = "contact.show"
	OpContactForget = "contact.forget"
	OpSuppress      = "contact.suppress"
	OpUnsuppress    = "contact.unsuppress"

	OpIssueCreate  = "issue.create"
	OpIssuePut     = "issue.put"
	OpIssueUpdate  = "issue.update"
	OpIssueList    = "issue.list"
	OpIssueShow    = "issue.show"
	OpIssueLint    = "issue.lint"
	OpIssuePreview = "issue.preview"
	OpIssueTest    = "issue.test"
	OpIssueSend    = "issue.send"
	OpIssueCancel  = "issue.cancel"
	OpIssueStats   = "issue.stats"
	OpIssueRemove  = "issue.remove"

	OpSitePut   = "site.put"
	OpSiteCheck = "site.check"

	OpRelaySet     = "relay.set"
	OpRelayShow    = "relay.show"
	OpRelayTest    = "relay.test"
	OpRelayWebhook = "relay.webhook"

	OpQueueList  = "queue.list"
	OpQueueRetry = "queue.retry"
	OpQueueDrop  = "queue.drop"

	OpSpamStats = "spam.stats"
	OpSpamReset = "spam.reset"
	OpReports   = "reports.list"
	OpBackup    = "backup"
)

// Name selects one thing by name, address or slug.
type Name struct {
	Name string `json:"name"`
}

// ID selects one thing by number.
type ID struct {
	ID int64 `json:"id"`
}

// IDs selects several messages.
type IDs struct {
	IDs []int64 `json:"ids"`
}

// Status is the overview `lfr status` shows.
type Status struct {
	Version   string          `json:"version"`
	Uptime    string          `json:"uptime"`
	Hostname  string          `json:"hostname"`
	IPv4      string          `json:"ipv4,omitempty"`
	Transport string          `json:"transport"`
	Domains   int             `json:"domains"`
	Mailboxes int             `json:"mailboxes"`
	Unread    int             `json:"unread"`
	Lists     int             `json:"lists"`
	Members   int             `json:"confirmed_members"`
	Queue     outbound.Status `json:"queue"`
	Paused    []string        `json:"paused,omitempty"` // "<list>: <reason>" for every list that holds its mail
	Listening []string        `json:"listening"`
	Next      []string        `json:"next,omitempty"`
}

// Check is one doctor result.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok, warn, fail, skip
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// ConfigSet changes a setting.
type ConfigSet struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// DomainInfo is a domain with what DNS must hold.
type DomainInfo struct {
	Name     string           `json:"name"`
	Selector string           `json:"selector"`
	Next     string           `json:"next_selector,omitempty"` // rotation prepared, not yet signing
	Catchall string           `json:"catchall,omitempty"`
	Created  time.Time        `json:"created"`
	Records  []domains.Record `json:"records,omitempty"`
	Zone     string           `json:"zone,omitempty"`
	Checks   []RecordCheck    `json:"checks,omitempty"`
	Notes    []string         `json:"notes,omitempty"`
}

// RecordCheck compares one expected record with live DNS.
type RecordCheck struct {
	Record domains.Record `json:"record"`
	Status string         `json:"status"` // ok, missing, wrong
	Found  []string       `json:"found,omitempty"`
}

// DomainDNS asks for a domain's records, optionally checked against live DNS.
type DomainDNS struct {
	Name  string `json:"name"`
	Check bool   `json:"check"`
}

// MailboxAdd creates a mailbox.
type MailboxAdd struct {
	Address string `json:"address"`
	Name    string `json:"name"`
}

// MailboxSet changes a mailbox; nil keeps a value.
type MailboxSet struct {
	Address string  `json:"address"`
	Name    *string `json:"name,omitempty"`
	Forward *string `json:"forward,omitempty"`
}

// Rename moves a mailbox to a new address.
type Rename struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Alias points an address at a mailbox.
type Alias struct {
	Alias  string `json:"alias"`
	Target string `json:"target"`
}

// Catchall sets or clears a domain's catch-all mailbox.
type Catchall struct {
	Domain string `json:"domain"`
	Target string `json:"target"`
}

// MailList selects messages; Search makes it a full-text search.
type MailList struct {
	Mailbox string `json:"mailbox,omitempty"`
	Folder  string `json:"folder,omitempty"`
	Unread  bool   `json:"unread,omitempty"`
	Thread  string `json:"thread,omitempty"`
	Search  string `json:"search,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Before  int64  `json:"before,omitempty"`
}

// MailRead reads one message.
type MailRead struct {
	ID         int64 `json:"id"`
	Raw        bool  `json:"raw,omitempty"`
	KeepUnread bool  `json:"keep_unread,omitempty"`
}

// MailMove moves messages; moving into or out of junk teaches the spam filter.
type MailMove struct {
	IDs    []int64 `json:"ids"`
	Folder string  `json:"folder"`
}

// MailFlag sets a flag.
type MailFlag struct {
	IDs  []int64 `json:"ids"`
	Flag string  `json:"flag"`
	On   bool    `json:"on"`
}

// MailDelete trashes or purges messages.
type MailDelete struct {
	IDs   []int64 `json:"ids"`
	Purge bool    `json:"purge,omitempty"`
}

// File is an attachment in transit.
type File struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	Data []byte `json:"data"`
}

// MailSend sends a message from a mailbox. Markdown, when given, becomes the
// HTML and text parts; otherwise Text and/or HTML are used as they are.
type MailSend struct {
	From     string   `json:"from"`
	To       []string `json:"to"`
	Cc       []string `json:"cc,omitempty"`
	Bcc      []string `json:"bcc,omitempty"`
	ReplyTo  string   `json:"reply_to,omitempty"`
	Subject  string   `json:"subject"`
	Markdown string   `json:"markdown,omitempty"`
	Text     string   `json:"text,omitempty"`
	HTML     string   `json:"html,omitempty"`
	Files    []File   `json:"files,omitempty"`
}

// MailReply answers a message.
type MailReply struct {
	ID       int64  `json:"id"`
	Markdown string `json:"markdown,omitempty"`
	Text     string `json:"text,omitempty"`
	HTML     string `json:"html,omitempty"`
	All      bool   `json:"all,omitempty"`
	NoQuote  bool   `json:"no_quote,omitempty"`
	Files    []File `json:"files,omitempty"`
}

// Sent identifies a sent message.
type Sent struct {
	ID      int64  `json:"id"`
	Queued  int    `json:"queued"`
	Message string `json:"message"`
}

// Attachment asks for one attachment.
type Attachment struct {
	ID    int64 `json:"id"`
	Index int   `json:"index"`
}

// Watch selects event kinds to stream; empty for all.
type Watch struct {
	Kinds []string `json:"kinds,omitempty"`
}

// Events lists stored events.
type Events struct {
	Since string   `json:"since,omitempty"` // duration like 24h, or RFC 3339
	Kinds []string `json:"kinds,omitempty"`
	List  string   `json:"list,omitempty"`
	Issue int64    `json:"issue,omitempty"`
	Limit int      `json:"limit,omitempty"`
}

// ListFields are the settable fields of a list.
type ListFields struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	From        *string `json:"from,omitempty"`
	FromName    *string `json:"from_name,omitempty"`
	ReplyTo     *string `json:"reply_to,omitempty"`
	Host        *string `json:"host,omitempty"`
	Language    *string `json:"language,omitempty"`
	Tracking    *bool   `json:"tracking,omitempty"`
	ImprintURL  *string `json:"imprint_url,omitempty"`
	PrivacyURL  *string `json:"privacy_url,omitempty"`
	Address     *string `json:"address,omitempty"`
	Consent     *string `json:"consent,omitempty"`
	Cadence     *string `json:"cadence,omitempty"`
	Headline    *string `json:"headline,omitempty"`
	Pitch       *string `json:"pitch,omitempty"`
	MinGap      *string `json:"min_gap,omitempty"` // a duration like 20h; 0 turns the guard off
}

// ListSet creates or changes a list.
type ListSet struct {
	Slug string `json:"slug"`
	ListFields
}

// ListPause pauses (with a reason) or resumes (empty reason) a list.
type ListPause struct {
	Slug   string `json:"slug"`
	Reason string `json:"reason"`
}

// SubAdd adds addresses to a list.
type SubAdd struct {
	List      string   `json:"list"`
	Emails    []string `json:"emails"`
	Confirmed bool     `json:"confirmed,omitempty"` // consent exists already: skip double opt-in
	Reconfirm bool     `json:"reconfirm,omitempty"` // readers already on the list confirm again
	Source    string   `json:"source,omitempty"`
	Consent   string   `json:"consent,omitempty"`
}

// SubAddResult counts what happened.
type SubAddResult struct {
	Added        int      `json:"added"`
	Existing     int      `json:"existing"`
	Confirmation int      `json:"confirmation_sent"`
	OptedOut     int      `json:"opted_out"` // left, complained, bounced or erased: kept off
	Reconfirm    int      `json:"reconfirm"` // on the list already, asked to confirm again
	Invalid      []string `json:"invalid,omitempty"`
}

// SubList selects members.
type SubList struct {
	List   string `json:"list"`
	Status string `json:"status,omitempty"`
	Search string `json:"search,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	After  int64  `json:"after,omitempty"`
}

// SubRemove unsubscribes an address from a list.
type SubRemove struct {
	List  string `json:"list"`
	Email string `json:"email"`
}

// SubImport imports a CSV.
type SubImport struct {
	List      string `json:"list"`
	CSV       string `json:"csv"`
	Confirmed bool   `json:"confirmed,omitempty"`
	Reconfirm bool   `json:"reconfirm,omitempty"`
	Source    string `json:"source,omitempty"`
	Consent   string `json:"consent,omitempty"`
}

// CSV is exported data.
type CSV struct {
	Rows int    `json:"rows"`
	CSV  string `json:"csv"`
}

// Suppress stops mail to an address.
type Suppress struct {
	Email  string   `json:"email,omitempty"`
	Emails []string `json:"emails,omitempty"` // many at once, e.g. an old platform's unsubscribed export
	Reason string   `json:"reason"`
}

// IssueRef names an issue by ID, or by its list and slug.
type IssueRef struct {
	ID   int64  `json:"id,omitempty"`
	List string `json:"list,omitempty"`
	Slug string `json:"slug,omitempty"`
}

// IssueDraft creates or changes an issue.
type IssueDraft struct {
	List      string `json:"list,omitempty"`
	ID        int64  `json:"id,omitempty"`
	Markdown  string `json:"markdown"`
	Subject   string `json:"subject,omitempty"`
	Slug      string `json:"slug,omitempty"`
	Preheader string `json:"preheader,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Welcome   bool   `json:"welcome,omitempty"` // issue.put: make it the list's welcome issue
	// issue.put: the images the Markdown references by a local path. nil
	// keeps the stored images; an empty list removes them.
	Media *[]issues.File `json:"media,omitempty"`
}

// IssuePut is the result of issue.put.
type IssuePut struct {
	Issue   issues.Issue `json:"issue"`
	Created bool         `json:"created"`
}

// IssueList selects a list's issues.
type IssueList struct {
	List string `json:"list"`
}

// IssueTest sends test copies.
type IssueTest struct {
	IssueRef
	To []string `json:"to"`
}

// IssueSend schedules an issue; At empty means now.
type IssueSend struct {
	IssueRef
	At    string `json:"at,omitempty"`
	Force bool   `json:"force,omitempty"` // send despite warnings (errors always block)
}

// Preview is a rendered issue.
type Preview struct {
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
	Size    int    `json:"size"`
	WebURL  string `json:"web_url"`
}

// RelaySet configures the outbound relay; an empty URL means direct delivery.
type RelaySet struct {
	URL      string `json:"url"`
	Password string `json:"password,omitempty"`
	SPF      string `json:"spf,omitempty"`
}

// RelayInfo describes outbound delivery.
type RelayInfo struct {
	Mode     string            `json:"mode"` // relay or direct
	URL      string            `json:"url,omitempty"`
	SPF      string            `json:"spf,omitempty"`
	Webhooks map[string]string `json:"webhooks"`
}

// QueueList selects queue entries.
type QueueList struct {
	Status string `json:"status,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// Reports selects stored reports.
type Reports struct {
	Kind   string `json:"kind,omitempty"`
	Domain string `json:"domain,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// Report is a stored DMARC or TLS report.
type Report struct {
	Kind     string          `json:"kind"`
	Org      string          `json:"org"`
	Domain   string          `json:"domain"`
	Begin    time.Time       `json:"begin"`
	End      time.Time       `json:"end"`
	Pass     int             `json:"pass"`
	Fail     int             `json:"fail"`
	Received time.Time       `json:"received"`
	Rows     json.RawMessage `json:"rows,omitempty"` // the report's rows as the reporter sent them
}

// ListTexts changes a validated batch of reader texts.
type ListTexts struct {
	Slug   string            `json:"slug"`
	Values map[string]string `json:"values"`
}

// BackupFrame carries one bounded chunk, file terminator, or final summary.
type BackupFrame struct {
	Path     string        `json:"path,omitempty"`
	Offset   int64         `json:"offset,omitempty"`
	Data     []byte        `json:"data,omitempty"`
	End      bool          `json:"end,omitempty"`
	Size     int64         `json:"size,omitempty"`
	SHA256   string        `json:"sha256,omitempty"`
	Summary  *BackupResult `json:"summary,omitempty"`
	Complete string        `json:"complete,omitempty"`
}

// BackupResult reports the backup.
type BackupResult struct {
	Dir   string `json:"dir"`
	Blobs int    `json:"blobs"`
	Bytes int64  `json:"bytes"`
}

// SitePut uploads a list's website templates and assets.
type SitePut struct {
	List  string            `json:"list"`
	Files map[string][]byte `json:"files"` // relative path → content
}
