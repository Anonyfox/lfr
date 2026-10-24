package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfr/internal/ctl"
)

var commands []command

func init() {
	commands = []command{
		// Server.
		{path: "setup", summary: "Set up this server for mail: data directory, host name, systemd service", local: true, run: runSetup,
			details: `Creates the data directory (default /var/lib/lfr) with its database and folders.
Running it again keeps all data and only changes what you pass. With --systemd (as
root) it also creates the lfr system user and installs and starts the lfr service,
allowed to bind ports 25, 80 and 443 and nothing more.

The hostname is the mail host: the MX target and the server's reverse DNS name.
Name the server the same at your provider (on DigitalOcean: the droplet name).
Then: lfr domain add <domain>.`,
			flags: []flag{{name: "hostname", value: true, usage: "mail host name, e.g. mail.example.com"}, {name: "ipv4", value: true, usage: "public IPv4 (detected when omitted)"},
				{name: "ipv6", value: true, usage: "public IPv6, optional"}, {name: "acme-email", value: true, usage: "contact for Let's Encrypt expiry notices"},
				{name: "systemd", usage: "install and start the systemd service (needs root)"}, {name: "user", value: true, usage: "service user for --systemd (default lfr)"}}},
		{path: "serve", summary: "Run the server (SMTP, HTTP, HTTPS, queue, control socket)", local: true, run: runServe,
			details: "Normally started by systemd. Logs go to stderr; LFR_LOG=debug shows more.",
			flags: []flag{{name: "smtp", value: true, usage: "SMTP listen address (default :25, off to disable)"}, {name: "http", value: true, usage: "HTTP listen address (default :80)"},
				{name: "https", value: true, usage: "HTTPS listen address (default :443)"}}},
		{path: "status", summary: "What the server is doing, and the next step", run: runStatus},
		{path: "doctor", summary: "Check DNS, reverse DNS, blocklists, relay, certificates, disk and queue", run: runDoctor},
		{path: "config", args: "[name [value]]", summary: "List, read or change settings", run: runConfig,
			details: "Without arguments lists every setting. With a name, shows it and its help; with a value, sets it.\nAn empty value restores the default: lfr config relay.rate \"\""},
		{path: "backup", args: "<dir>", summary: "Write a consistent copy of all data into an empty directory", run: runBackup,
			details: "The CLI writes the copy. With --remote or LFR_REMOTE the data lands on the calling machine, not the server.\nThe CLI and server must run the same lfr version.\nTo restore, stop the service and use the copy as the data directory (lfr serve --data <dir>)."},
		{path: "events", summary: "What happened: deliveries, bounces, signups, mail; --follow streams it live", run: runEvents,
			details: "Kinds: received, subscribed, confirmed, unsubscribed, sent, deferred, failed, bounced, complained,\nopened, clicked, issue_started, issue_finished, paused.",
			flags: []flag{{name: "follow", usage: "stream new events as they happen (one JSON object per line with --json)"},
				{name: "since", value: true, usage: "24h, 7d or an RFC 3339 time"}, {name: "kind", value: true, multi: true, usage: "only these kinds"},
				{name: "list", value: true, usage: "one list"}, {name: "limit", value: true, usage: "how many (default 100)"}}},
		{path: "version", summary: "Print the version", local: true, run: func(c *call) error {
			return c.result(c.env.Build, func() { c.printf("lfr %s (%s, %s)\n", c.env.Build.Version, c.env.Build.Commit, c.env.Build.Date) })
		}},
		{path: "proxy", hidden: true, summary: "Connect stdin and stdout to the control socket (the server end of --remote)", local: true, run: runProxy,
			details: `lfr --remote user@host (or LFR_REMOTE=user@host) runs "ssh user@host lfr proxy" and speaks to the
server through it, so every command works from a laptop. The ssh user needs access to the socket
(the lfr group). LFR_REMOTE_DATA names the server's data directory when it is not the default.`},

		// Mail.
		{path: "domain add", args: "<domain>", summary: "Serve a domain; prints the DNS records to publish", run: runDomainAdd},
		{path: "domain ls", summary: "List domains", run: runDomainList},
		{path: "domain dns", args: "<domain>", summary: "Show a domain's DNS records", run: runDomainDNS,
			flags: []flag{{name: "check", usage: "look up every record and report missing or wrong ones"}, {name: "zone", usage: "print a BIND zone snippet (Cloudflare can import it)"}}},
		{path: "domain set", args: "<domain>", summary: "Change a domain: where mail for unknown addresses goes", run: runDomainSet,
			flags: []flag{{name: "catchall", value: true, usage: "a mailbox for mail to unknown addresses, or off (catch-alls attract spam)"}}},
		{path: "domain rotate", args: "<domain>", summary: "Prepare new DKIM keys; run again once they resolve to switch", run: runDomainRotate},
		{path: "domain rm", args: "<domain>", summary: "Stop serving a domain and delete its mailboxes, mail and lists", run: runDomainRemove,
			flags: []flag{{name: "yes", usage: "confirm deleting everything on the domain"}}},
		{path: "mailbox add", args: "<address>", summary: "Create a mailbox", run: runMailboxAdd, flags: []flag{{name: "name", value: true, usage: "display name used when sending"}}},
		{path: "mailbox alias", args: "<address> <mailbox>", summary: "Deliver another address of the same domain into a mailbox", run: runAliasAdd},
		{path: "mailbox ls", args: "[domain]", summary: "List mailboxes and aliases with unread counts", run: runMailboxList},
		{path: "mailbox set", args: "<address>", summary: "Change a mailbox's name or forwarding", run: runMailboxSet,
			flags: []flag{{name: "name", value: true, usage: "display name"}, {name: "forward", value: true, usage: "also send every inbox message to this address (empty to stop)"}}},
		{path: "mailbox mv", args: "<address> <new address>", summary: "Move a mailbox to a new address, keeping its mail", run: runMailboxRename},
		{path: "mailbox rm", args: "<address>", summary: "Remove an alias, or delete a mailbox with all its mail", run: runMailboxRemove, flags: []flag{{name: "yes", usage: "confirm deleting a mailbox"}}},
		{path: "mail ls", args: "[mailbox]", summary: "List messages, newest first", run: runMailList,
			details: `Without --search, lists a folder (inbox by default). --search matches every word as a prefix;
"quoted phrases" match exactly; from:ann and subject:starter limit a word to a field.`,
			flags: []flag{{name: "search", value: true, usage: "full-text search over subject, people and body"}, {name: "folder", value: true, usage: "inbox, junk, archive, sent, trash or all"},
				{name: "unread", usage: "only unread"}, {name: "thread", value: true, usage: "one conversation"}, {name: "limit", value: true, usage: "how many (default 50)"},
				{name: "before", value: true, usage: "only messages older than this ID (paging)"}}},
		{path: "mail read", args: "<id>", summary: "Show a message and mark it read", run: runMailRead,
			flags: []flag{{name: "raw", usage: "the complete message as received"}, {name: "html", usage: "the HTML part instead of text"}, {name: "keep-unread", usage: "do not mark as read"},
				{name: "attachment", value: true, usage: "save attachment number N instead"}, {name: "out", value: true, usage: "file for --attachment (default: its own name)"}}},
		{path: "mail send", summary: "Send a message from a mailbox", run: runMailSend,
			details: `The body comes from --md (Markdown, rendered to HTML and text), --text or --html;
"-" reads it from stdin. Bcc recipients receive the message without appearing in it.`,
			flags: []flag{{name: "from", value: true, usage: "a mailbox or alias here"}, {name: "to", value: true, multi: true, usage: "recipient (repeatable, or comma separated)"},
				{name: "cc", value: true, multi: true, usage: "copy recipient"}, {name: "bcc", value: true, multi: true, usage: "blind copy recipient"},
				{name: "subject", value: true, usage: "subject"}, {name: "md", value: true, usage: "Markdown body file, - for stdin"}, {name: "text", value: true, usage: "plain-text body file, - for stdin"},
				{name: "html", value: true, usage: "HTML body file"}, {name: "attach", value: true, multi: true, usage: "file to attach"}, {name: "reply-to", value: true, usage: "Reply-To address"}}},
		{path: "mail reply", args: "<id>", summary: "Answer a message from the mailbox that received it", run: runMailReply,
			flags: []flag{{name: "md", value: true, usage: "Markdown body file, - for stdin"}, {name: "text", value: true, usage: "plain-text body file, - for stdin"},
				{name: "all", usage: "reply to all recipients"}, {name: "no-quote", usage: "do not quote the original"}, {name: "attach", value: true, multi: true, usage: "file to attach"}}},
		{path: "mail mv", args: "<id>... <folder>", summary: "Move messages; into or out of junk teaches the spam filter", run: runMailMove},
		{path: "mail flag", args: "<id>... <flag>", summary: "Set or clear a flag: seen, unseen, flagged, unflagged, answered", run: runMailFlag},
		{path: "mail rm", args: "<id>...", summary: "Move messages to trash (from trash: delete for good)", run: runMailDelete, flags: []flag{{name: "purge", usage: "delete for good right away"}}},
		{path: "mail spam", summary: "What the spam filter learned; --reset forgets it", run: runMailSpam, flags: []flag{{name: "reset", usage: "forget everything the filter learned"}, {name: "yes", usage: "confirm --reset"}}},
		{path: "relay", args: "[url|off]", summary: "Show, set or turn off the relay for outgoing mail", run: runRelay,
			details: `Without an argument shows how mail leaves this server and the webhook URLs for the relay's bounce
and complaint notifications. A URL sets the relay and tests the login: smtps://user@host:port (TLS)
or smtp://user@host:port (STARTTLS). The password is read from stdin unless --password is given.
DigitalOcean blocks ports 25, 465 and 587; use the relay's alternate port (SES: 2465 or 2587,
SMTP2GO: 2525 or 8465). off delivers directly to recipients' servers (needs outbound port 25).`,
			flags: []flag{{name: "password", value: true, usage: "relay password (default: read from stdin)"}, {name: "spf", value: true, usage: "the relay's SPF mechanism, e.g. include:amazonses.com"}}},
		{path: "queue", summary: "Outgoing mail waiting, failed or bounced", run: runQueue,
			flags: []flag{{name: "status", value: true, usage: "queued (default), sent, failed, bounced or canceled"}, {name: "limit", value: true, usage: "how many"}}},
		{path: "queue retry", args: "[id]", summary: "Retry one delivery now (or every queued one)", run: runQueueRetry},
		{path: "queue drop", args: "<id>", summary: "Cancel a queued delivery", run: runQueueDrop},
		{path: "reports", summary: "DMARC and TLS reports other mail servers sent", run: runReports,
			flags: []flag{{name: "kind", value: true, usage: "dmarc or tlsrpt"}, {name: "domain", value: true, usage: "one domain"}, {name: "limit", value: true, usage: "how many"}}},

		// Newsletters.
		{path: "list add", args: "<slug>", summary: "Create a newsletter list", run: runListAdd, flags: listFlags(),
			details: `--from must be an address on a served domain; lfr creates a mailbox for it so replies arrive.
--host gives the list its own website (news.example.com); without it the site lives at
https://<hostname>/lists/<slug>/. Tracking (opens and clicks) is off by default: in Germany
it needs consent (TDDDG §25).`},
		{path: "list ls", summary: "List lists with reader counts", run: runListList},
		{path: "list text", args: "<slug> [<key> <value>]", summary: "Show or override reader texts for a list", run: runListText, flags: []flag{{name: "reset", usage: "reset the named key to its built-in text"}, {name: "import", value: true, usage: "flat JSON object of texts, - for stdin; empty values reset keys"}}},
		{path: "list show", args: "<slug>", summary: "Show a list", run: runListShow},
		{path: "list set", args: "<slug>", summary: "Change a list, or pause and resume sending", run: runListSet,
			flags: append(listFlags(), flag{name: "pause", value: true, usage: "stop sending, with the reason"}, flag{name: "resume", usage: "send again after a pause"})},
		{path: "list rm", args: "<slug>", summary: "Delete a list with its readers and issues", run: runListRemove, flags: []flag{{name: "yes", usage: "confirm"}}},
		{path: "reader add", args: "<list> <email>...", summary: "Subscribe addresses, or import a CSV (double opt-in unless --confirmed)", run: runReaderAdd,
			flags: []flag{{name: "csv", value: true, usage: "import addresses from a CSV file (column email, optional name), - for stdin"},
				{name: "confirmed", usage: "they consented elsewhere already: no confirmation mail"},
				{name: "reconfirm", usage: "ask readers already on the list to confirm again (a stale list): they get a confirmation mail and drop off unless they click"},
				{name: "source", value: true, usage: "where the addresses came from"},
				{name: "consent", value: true, usage: "the consent statement they agreed to"}}},
		{path: "reader ls", args: "<list>", summary: "List a list's readers, or export them with consent records", run: runReaderList,
			flags: []flag{{name: "csv", usage: "export every reader with consent records as CSV"}, {name: "out", value: true, usage: "file for --csv (default stdout)"},
				{name: "status", value: true, usage: "pending, confirmed or unsubscribed"}, {name: "search", value: true, usage: "part of the address"},
				{name: "limit", value: true, usage: "how many (default 100)"}, {name: "after", value: true, usage: "reader ID to continue after"}}},
		{path: "reader rm", args: "<list> <email>", summary: "Unsubscribe an address from a list", run: runSubRemove},
		{path: "reader show", args: "<email>", summary: "Everything known about an address, on every list", run: runContactShow},
		{path: "reader block", args: "<email>...", summary: "Never send to an address again, on any list", run: runSuppress, flags: []flag{{name: "reason", value: true, usage: "why"},
			{name: "csv", value: true, usage: "block every address in a CSV file, e.g. an old platform's unsubscribed or cleaned export"}},
			details: `Moving from another platform? Block its unsubscribed, cleaned and complained exports, so no
later import or forgotten list ever mails them:

  lfr reader block --csv unsubscribed_members_export.csv --reason "unsubscribed at Mailchimp"`},
		{path: "reader unblock", args: "<email>", summary: "Allow mail to a blocked address again", run: runUnsuppress},
		{path: "reader forget", args: "<email>", summary: "Erase an address and its history (GDPR)", run: runForget, flags: []flag{{name: "yes", usage: "confirm"}}},
		{path: "issue put", args: "[list] <file.md>", summary: "Create or update an issue from Markdown, uploading its images", run: runIssuePut,
			flags: append(issueFlags(), flag{name: "welcome", usage: "make it the welcome issue every new reader gets on confirming"}),
			details: `The subject is the first heading unless front matter or --subject sets it:

  ---
  subject: Starter rescue
  preheader: It is hungry, not dead
  ---
  # Your starter is hungry

The slug is the file name (issues/starter-rescue.md is starter-rescue) unless front matter or --slug
sets it. Inside a newsletter project the list comes from lfr.json. Images the file references by a
local path are uploaded and served from the list's website. A paragraph holding only a link whose
text ends in "→" becomes a button. An issue that went out cannot change; a changed draft needs a
new test copy.

An issue is named by its ID, as <list>/<slug>, or inside a newsletter project by its slug or file.`},
		{path: "issue ls", args: "[list]", summary: "List a list's issues", run: runIssueList},
		{path: "issue show", args: "<issue>", summary: "Show an issue: findings before it goes out, numbers after", run: runIssueShow,
			flags: []flag{{name: "html", value: true, usage: "write the email as a reader gets it to this HTML file"}, {name: "text", usage: "print the plain-text part"},
				{name: "learn", usage: "print the learnings.md entry of a sent issue"}}},
		{path: "issue test", args: "<issue> <email>...", summary: "Send test copies marked [Test]", run: runIssueTest},
		{path: "issue send", args: "<issue>", summary: "Send an issue to every confirmed reader, now or --at a time", run: runIssueSend,
			details: "Errors always block. Warnings, and another issue within the list's minimum gap (lfr list set --min-gap), block unless --force.",
			flags:   []flag{{name: "at", value: true, usage: "RFC 3339 time, e.g. 2026-10-20T08:00:00+02:00"}, {name: "force", usage: "send despite warnings and the minimum gap (errors always block)"}}},
		{path: "issue cancel", args: "<issue>", summary: "Unschedule an issue or stop one that is sending", run: runIssueCancel},
		{path: "issue rm", args: "<issue>", summary: "Delete an issue (also from the archive)", run: runIssueRemove},
		{path: "init", args: "<dir>", summary: "Start a newsletter project: research, brief, backlog and issues on disk", local: true, run: runInit,
			details: `A newsletter project is the editorial side of a list: research, the brief (concept.md) every issue
is held to, the welcome issue, a backlog of ideas, issue drafts and learnings, all plain text for git.
lfr init also installs the agent skills into the project. Then run lfr check: it names the one
next step and the skill that covers it, every time.`,
			flags: []flag{{name: "list", value: true, usage: "the list's slug on the server (default: the folder name)"},
				{name: "name", value: true, usage: "the newsletter's name"}, {name: "language", value: true, usage: "en or de (default en)"},
				{name: "owner", value: true, usage: "where test copies go"},
				{name: "autonomy", value: true, usage: "draft (the owner approves every send, default) or send (a clean check and a test copy suffice)"}}},
		{path: "check", args: "[dir]", summary: "Check a newsletter project and name the next step", local: true, run: runCheck,
			details: `Holds research, brief, backlog, welcome issue and drafts to their formats, every draft to the
brief and to the writing rules (lfr rules); with a server in reach, compares them with the list
(settings, welcome issue, drafts, test copies, schedule, numbers of sent issues). The last line is
the next step and its skill: research, concept, launch, learn, plan, draft, fix, test, approve,
send or wait.`,
			flags: []flag{{name: "offline", usage: "check the files only, without asking the server"}}},
		{path: "push", args: "[dir]", summary: "Copy the brief's promise and cadence and the welcome issue to the list", run: runPush},
		{path: "site", args: "<list> [dir]", summary: "Edit a list's website: copy the templates out, upload them, or check them", run: runSite,
			details: `A list's website is built from templates: layout, signup, home, archive, issue and message
(.html, Go html/template). custom.html is added to every page, email.html replaces the email
layout, and files in assets/ are served at /assets/.

  lfr site news --eject site    copy the built-in templates into ./site to edit them
  lfr site news site            upload ./site; it goes live only if every template parses
  lfr site news                 check the templates the server serves

In a newsletter project, lfr push uploads site/ as well.`,
			flags: []flag{{name: "eject", usage: "copy the built-in templates into a local folder (default site) for editing"}}},

		// Agents.
		{path: "skills", args: "[topic]", summary: "Print the skills that teach an agent lfr", local: true, run: runSkills},
		{path: "skills install", args: "[dir]", summary: "Install the skills where agents load them", local: true, run: runSkillsInstall,
			details: `Without flags lfr installs into the folders that already hold lfr skills, else .claude/skills when
Claude Code runs, else an existing agent folder, else .agents/skills (read by Codex, Cursor, OpenCode,
Gemini CLI and GitHub Copilot). Copies somebody edited are kept unless --force.`,
			flags: []flag{{name: "agents", usage: "install into .agents/skills"}, {name: "claude", usage: "install into .claude/skills"},
				{name: "global", usage: "install into the home directory"}, {name: "force", usage: "replace copies somebody edited"}}},
		{path: "rules", args: "[id]", summary: "The checks on issues, briefs and research, and why", local: true, run: runRules},
		{path: "evidence", args: "[dimension|id]", summary: "The research behind lfr's defaults, verbatim and citable", local: true, run: runEvidence,
			details: "Standards, studies, benchmarks and guides with exact quotes and sources, plus voices of readers and\nwriters and peer newsletters. Cite an item as lfr:E012 (or lfr:V007, lfr:P003)."},
	}
}

// sections order the overview; each top-level word belongs to one.
var sections = []struct {
	name  string
	words []string
}{
	{"Server", []string{"setup", "serve", "status", "doctor", "config", "backup", "events", "version"}},
	{"Mail", []string{"domain", "mailbox", "mail", "relay", "queue", "reports"}},
	{"Newsletters", []string{"list", "reader", "issue", "init", "check", "push", "site"}},
	{"Agents", []string{"skills", "rules", "evidence"}},
}

// groupSummary describes top-level words that only have subcommands.
var groupSummary = map[string]string{
	"domain":  "Serve domains and show the DNS records they need",
	"mailbox": "Create mailboxes and aliases, set forwarding",
	"mail":    "Read, search, send, answer and sort mail",
	"list":    "Newsletter lists: sender, website, cadence",
	"reader":  "A list's readers: add, import, export, block, forget",
	"issue":   "Write, test, send and count a newsletter's issues",
}

func need(c *call, n int) error {
	if len(c.args) < n {
		return usage("missing arguments")
	}
	return nil
}

func exactly(c *call, n int) error {
	if len(c.args) != n {
		if len(c.args) < n {
			return usage("missing arguments")
		}
		return usage("too many arguments")
	}
	return nil
}

func ids(args []string) ([]int64, error) {
	var out []int64
	for _, a := range args {
		n, err := strconv.ParseInt(strings.TrimPrefix(a, "#"), 10, 64)
		if err != nil || n <= 0 {
			return nil, usage("%q is not a message ID", a)
		}
		out = append(out, n)
	}
	return out, nil
}

func id(s string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimPrefix(s, "#"), 10, 64)
	if err != nil || n <= 0 {
		return 0, usage("%q is not an ID", s)
	}
	return n, nil
}

func intFlag(c *call, name string) (int, error) {
	if !c.has(name) {
		return 0, nil
	}
	n, err := strconv.Atoi(c.str(name))
	if err != nil {
		return 0, usage("--%s needs a number", name)
	}
	return n, nil
}

// readArg reads a file, or stdin for "-".
func readArg(c *call, path string) (string, error) {
	if path == "-" {
		b, err := io.ReadAll(c.env.Stdin)
		return string(b), err
	}
	b, err := os.ReadFile(path)
	return string(b), err
}

// count says "1 recipient" or "3 recipients".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// isNotFound reports a server answer that the named thing does not exist.
func isNotFound(err error) bool {
	var ce *ctl.Error
	return errors.As(err, &ce) && ce.Code == ctl.CodeNotFound
}
