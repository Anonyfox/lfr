-- Times are unix seconds. Email addresses and domains are stored lowercased.

CREATE TABLE settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
) STRICT;

CREATE TABLE domains (
	id       INTEGER PRIMARY KEY,
	name     TEXT NOT NULL UNIQUE,
	selector TEXT NOT NULL,          -- DKIM selector stem: <stem>r (RSA) and <stem>e (Ed25519)
	dkim_rsa BLOB NOT NULL,          -- PKCS#8 DER
	dkim_ed  BLOB NOT NULL,          -- PKCS#8 DER
	next_selector TEXT NOT NULL DEFAULT '', -- keys of a rotation, published before they sign
	next_rsa BLOB,
	next_ed  BLOB,
	catchall INTEGER REFERENCES mailboxes(id) ON DELETE SET NULL,
	created  INTEGER NOT NULL
) STRICT;

CREATE TABLE mailboxes (
	id        INTEGER PRIMARY KEY,
	domain_id INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
	local     TEXT NOT NULL,
	name      TEXT NOT NULL DEFAULT '',  -- display name when sending
	forward   TEXT NOT NULL DEFAULT '',  -- copy every inbox message to this address
	created   INTEGER NOT NULL,
	UNIQUE (domain_id, local)
) STRICT;

CREATE TABLE aliases (
	domain_id  INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
	local      TEXT NOT NULL,
	mailbox_id INTEGER NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
	PRIMARY KEY (domain_id, local)
) STRICT;

CREATE TABLE messages (
	id          INTEGER PRIMARY KEY,
	mailbox_id  INTEGER NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
	folder      TEXT NOT NULL CHECK (folder IN ('inbox', 'junk', 'archive', 'sent', 'trash')),
	blob        TEXT NOT NULL,            -- sha256 of the raw message in the blob store
	size        INTEGER NOT NULL,
	received    INTEGER NOT NULL,
	date        INTEGER NOT NULL,         -- the Date header, or received
	message_id  TEXT NOT NULL DEFAULT '',
	in_reply_to TEXT NOT NULL DEFAULT '',
	refs        TEXT NOT NULL DEFAULT '', -- References, space separated
	thread      TEXT NOT NULL DEFAULT '',
	from_addr   TEXT NOT NULL DEFAULT '',
	from_name   TEXT NOT NULL DEFAULT '',
	to_addrs    TEXT NOT NULL DEFAULT '', -- comma separated
	cc_addrs    TEXT NOT NULL DEFAULT '',
	reply_to    TEXT NOT NULL DEFAULT '',
	subject     TEXT NOT NULL DEFAULT '',
	snippet     TEXT NOT NULL DEFAULT '',
	attachments INTEGER NOT NULL DEFAULT 0,
	seen        INTEGER NOT NULL DEFAULT 0,
	flagged     INTEGER NOT NULL DEFAULT 0,
	answered    INTEGER NOT NULL DEFAULT 0,
	spam        REAL NOT NULL DEFAULT 0,  -- spam probability at delivery
	verdict     TEXT NOT NULL DEFAULT '', -- JSON: authentication results and rule hits
	features    TEXT NOT NULL DEFAULT '', -- JSON feature vector, for learning
	trained     TEXT NOT NULL DEFAULT '' CHECK (trained IN ('', 'spam', 'ham'))
) STRICT;
CREATE INDEX messages_box ON messages (mailbox_id, folder, received DESC);
CREATE INDEX messages_thread ON messages (thread);
CREATE INDEX messages_blob ON messages (blob);
CREATE INDEX messages_msgid ON messages (message_id);

CREATE VIRTUAL TABLE messages_fts USING fts5 (
	subject, people, body,
	content = '', contentless_delete = 1,
	tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TABLE lists (
	id          INTEGER PRIMARY KEY,
	domain_id   INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
	slug        TEXT NOT NULL UNIQUE,
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	from_local  TEXT NOT NULL,
	from_name   TEXT NOT NULL,
	reply_to    TEXT NOT NULL DEFAULT '',
	host        TEXT NOT NULL DEFAULT '',  -- web host of the list's site
	language    TEXT NOT NULL DEFAULT 'en',
	tracking    INTEGER NOT NULL DEFAULT 0,
	imprint_url TEXT NOT NULL DEFAULT '',
	privacy_url TEXT NOT NULL DEFAULT '',
	address     TEXT NOT NULL DEFAULT '',  -- sender identity line in every footer
	consent     TEXT NOT NULL DEFAULT '',  -- consent statement shown by the signup form
	paused      TEXT NOT NULL DEFAULT '',  -- reason sending is paused, empty when active
	resumed     INTEGER NOT NULL DEFAULT 0, -- last resume: list health counts from here
	old_hosts   TEXT NOT NULL DEFAULT '',  -- former website hosts, comma separated: links in sent mail keep working
	cadence     TEXT NOT NULL DEFAULT '',  -- how often mail comes, as promised at signup
	headline    TEXT NOT NULL DEFAULT '',  -- the website's first line (the brief's Signup Headline:)
	pitch       TEXT NOT NULL DEFAULT '',  -- one or two sentences under it (Pitch:)
	min_gap     INTEGER NOT NULL DEFAULT 72000, -- seconds that must pass between two issues
	created     INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX lists_host ON lists (host) WHERE host != '';

-- Addresses erased on request (GDPR Art. 17), kept only as a hash so an
-- import cannot bring them back.
CREATE TABLE forgotten (
	hash TEXT PRIMARY KEY,
	at   INTEGER NOT NULL
) STRICT;

CREATE TABLE contacts (
	id              INTEGER PRIMARY KEY,
	email           TEXT NOT NULL UNIQUE,
	name            TEXT NOT NULL DEFAULT '',
	created         INTEGER NOT NULL,
	suppressed      INTEGER NOT NULL DEFAULT 0,  -- when the address stopped receiving anything
	suppress_reason TEXT NOT NULL DEFAULT '',
	soft_bounces    INTEGER NOT NULL DEFAULT 0,  -- soft bounces in a row
	first_soft      INTEGER NOT NULL DEFAULT 0   -- when the first of them happened
) STRICT;

CREATE TABLE members (
	id           INTEGER PRIMARY KEY,
	list_id      INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
	contact_id   INTEGER NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
	status       TEXT NOT NULL CHECK (status IN ('pending', 'confirmed', 'unsubscribed')),
	source       TEXT NOT NULL DEFAULT '',
	consent      TEXT NOT NULL DEFAULT '',
	signup_at    INTEGER NOT NULL,
	signup_ip    TEXT NOT NULL DEFAULT '',
	signup_ua    TEXT NOT NULL DEFAULT '',
	confirm_at   INTEGER NOT NULL DEFAULT 0,
	confirm_ip   TEXT NOT NULL DEFAULT '',
	confirm_sent INTEGER NOT NULL DEFAULT 0,
	unsub_at     INTEGER NOT NULL DEFAULT 0,
	unsub_how    TEXT NOT NULL DEFAULT '',
	UNIQUE (list_id, contact_id)
) STRICT;
CREATE INDEX members_list ON members (list_id, status);
CREATE INDEX members_contact ON members (contact_id);

CREATE TABLE issues (
	id        INTEGER PRIMARY KEY,
	list_id   INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
	slug      TEXT NOT NULL,
	subject   TEXT NOT NULL,
	preheader TEXT NOT NULL DEFAULT '',
	summary   TEXT NOT NULL DEFAULT '',
	markdown  TEXT NOT NULL,
	status    TEXT NOT NULL CHECK (status IN ('draft', 'scheduled', 'sending', 'sent', 'canceled')),
	scheduled INTEGER NOT NULL DEFAULT 0,
	started   INTEGER NOT NULL DEFAULT 0,
	finished  INTEGER NOT NULL DEFAULT 0,
	tested    INTEGER NOT NULL DEFAULT 0,  -- when this content last went out as a test copy
	welcome   INTEGER NOT NULL DEFAULT 0,  -- 1: sent to each member on confirming, never broadcast
	stats     TEXT NOT NULL DEFAULT '',    -- numbers kept when per-recipient rows are pruned
	created   INTEGER NOT NULL,
	updated   INTEGER NOT NULL,
	UNIQUE (list_id, slug)
) STRICT;
CREATE INDEX issues_status ON issues (status, scheduled);
CREATE UNIQUE INDEX issues_welcome ON issues (list_id) WHERE welcome = 1;

-- Images an issue's Markdown references by a local path; the bytes are blobs.
CREATE TABLE media (
	issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
	path     TEXT NOT NULL,     -- as the Markdown writes it
	hash     TEXT NOT NULL,     -- the blob
	type     TEXT NOT NULL CHECK (type IN ('image/jpeg', 'image/png', 'image/gif')),
	size     INTEGER NOT NULL,
	PRIMARY KEY (issue_id, path)
) STRICT;
CREATE INDEX media_hash ON media (hash);

CREATE TABLE outbound (
	id         INTEGER PRIMARY KEY,
	kind       TEXT NOT NULL CHECK (kind IN ('issue', 'confirm', 'mail')),
	domain_id  INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
	issue_id   INTEGER REFERENCES issues(id) ON DELETE CASCADE,
	contact_id INTEGER REFERENCES contacts(id) ON DELETE CASCADE,
	member_id  INTEGER REFERENCES members(id) ON DELETE CASCADE,
	blob       TEXT NOT NULL DEFAULT '',     -- kind mail: the unsigned raw message
	env_from   TEXT NOT NULL,
	rcpt       TEXT NOT NULL,
	status     TEXT NOT NULL CHECK (status IN ('queued', 'sending', 'sent', 'failed', 'bounced', 'canceled')),
	attempts   INTEGER NOT NULL DEFAULT 0,
	next_at    INTEGER NOT NULL,
	last_error TEXT NOT NULL DEFAULT '',
	created    INTEGER NOT NULL,
	sent_at    INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX outbound_due ON outbound (next_at) WHERE status = 'queued';
CREATE UNIQUE INDEX outbound_issue ON outbound (issue_id, contact_id) WHERE kind = 'issue';
CREATE INDEX outbound_status ON outbound (status, created);
CREATE INDEX outbound_rcpt ON outbound (rcpt);              -- known contacts, webhooks, erasure
CREATE INDEX outbound_contact ON outbound (contact_id) WHERE contact_id IS NOT NULL;
CREATE INDEX outbound_member ON outbound (member_id) WHERE member_id IS NOT NULL;
CREATE INDEX outbound_blob ON outbound (blob) WHERE blob != '';  -- blob sweep

CREATE TABLE events (
	id          INTEGER PRIMARY KEY,
	at          INTEGER NOT NULL,
	kind        TEXT NOT NULL,
	list_id     INTEGER,
	issue_id    INTEGER,
	contact_id  INTEGER,
	outbound_id INTEGER,
	message_id  INTEGER,
	detail      TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX events_issue ON events (issue_id, kind) WHERE issue_id IS NOT NULL;
CREATE INDEX events_list ON events (list_id, kind, at) WHERE list_id IS NOT NULL;
CREATE INDEX events_at ON events (at);
CREATE INDEX events_contact ON events (contact_id, kind, at) WHERE contact_id IS NOT NULL;  -- signup cap, erasure
CREATE INDEX events_outbound ON events (outbound_id, kind) WHERE outbound_id IS NOT NULL;   -- opens and clicks

CREATE TABLE bayes (
	token INTEGER PRIMARY KEY,  -- 64-bit hash of the token
	spam  INTEGER NOT NULL,
	ham   INTEGER NOT NULL
) STRICT;

CREATE TABLE reputation (
	key     TEXT PRIMARY KEY,   -- "addr:...", "domain:...", "net:..."
	spam    INTEGER NOT NULL,
	ham     INTEGER NOT NULL,
	updated INTEGER NOT NULL
) STRICT;

CREATE TABLE reports (
	id       INTEGER PRIMARY KEY,
	kind     TEXT NOT NULL CHECK (kind IN ('dmarc', 'tlsrpt')),
	org      TEXT NOT NULL,
	report   TEXT NOT NULL,      -- the reporter's report id
	domain   TEXT NOT NULL,
	begin_at INTEGER NOT NULL,
	end_at   INTEGER NOT NULL,
	received INTEGER NOT NULL,
	pass     INTEGER NOT NULL,
	fail     INTEGER NOT NULL,
	detail   TEXT NOT NULL,      -- JSON rows
	UNIQUE (kind, org, report)
) STRICT;
