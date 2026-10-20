---
name: lfr-research
description: Collects verbatim voices of a newsletter's future readers and analyzes the newsletters they already read - into [[.Project.Voices]] and [[.Project.Peers]] of an lfr newsletter project. Use when lfr check names the research step, before writing or rebuilding a concept, when replies or numbers raise a question the research cannot answer, or when asked what readers of a topic want from a newsletter. Not for writing the concept or issues; those cite these files.
license: MIT
compatibility: Works with any toolset. Uses web search, page fetching, a browser or data connectors when available, and asks the owner for sources otherwise.
---

# Research for a newsletter

Asked for a newsletter about a topic, a model writes the typical newsletter: a friendly greeting, five
links, a sign-off. Readers already get that one, and they leave it. What breaks the pattern is what real
readers of this topic say they want, skip and resent, and how the newsletters they already read are built.
This skill collects both into two files that every decision in the concept cites by ID.

General newsletter craft (what Gmail requires, what cadence does to unsubscribes, how long subject lines
can be) is already researched and built into lfr: `lfr evidence` lists it, `lfr evidence <dimension>`
prints it verbatim, and the concept cites it as `lfr:E012`. Do not research craft again here. Research
**this** newsletter's readers and peers.

## Rules that always hold

1. **Verbatim or nothing.** Copy the passage exactly as written, typos and emoji included; mark every cut
   with `…`. A paraphrase turns a reader's voice into model text, which is what this research exists to
   avoid.
2. **Never write from memory.** Every item comes from a source opened in this session or supplied by the
   owner. A gap is recorded as unreachable, never filled.
3. **Read before write, append only.** Open the existing file first and add a dated batch. Entries are
   never edited, reordered or deleted: the concept, the backlog and the issues cite their IDs.
4. **Every item is retrievable**: URL, platform, author type, post date as precise as the source shows,
   fetch date and method.
5. **Public access only.** Never log in, bypass a wall or scrape at volume. A browser where the owner is
   signed in counts as logging in; use it only when the owner allows it.
6. **Observations, not interpretation.** Voices are what people wrote; peers are what issues show. Themes
   and choices belong in [[.Project.Concept]].
7. Some fetch tools return a model's summary instead of the page's text. Quotes from such output are not
   verbatim: use another way to read the page, or record the platform as unreachable.

## Step 0: access and mode

Say in one line which access you have (a connector, fetching pages, a browser, or only what the owner
pastes), then pick the mode:

- **From zero** when a file is missing or below its minimum: collect until the minimums hold.
- **Ongoing** when something new arrives: replies to an issue, an unexpected unsubscribe spike, a new peer,
  a pillar that needs depth. Append one batch named after its trigger.

The owner's own material counts: existing newsletters they wrote, replies they received, posts and talks.
Ask for it; record it `via user`. Replies to issues (`lfr issue show <issue> --learn` lists them) are the
closest voices there are: platform `reply (<newsletter>)`, author `reader`, source `<lfr:mail/ID>` with
the message ID, never the reader's address or name.

## Workflow

```
- [ ] Access stated; mode chosen; both files read; next free IDs noted
- [ ] Readers named: who would subscribe, where they talk, in which language
- [ ] Queries planned, including ones aimed at the opposite view
- [ ] Voices collected and written as a batch
- [ ] Peer newsletters read (2–3 recent issues each, plus the signup page); grid, baseline, gaps updated
- [ ] lfr check shows no research finding
- [ ] Shortfalls and unreachable platforms reported to the owner
```

**Voices.** Search where the future readers talk about the topic itself, not about newsletters: forums,
communities, Q&A threads, reviews, comment sections, social posts, in the readers' language. Collect what
they struggle with, what they already read and skip, what they wish someone would explain, the words they
use, and what they resent (ads, hype, length, jargon, beginners' content, experts' content). Add voices
about newsletters in this niche where they exist ("I unsubscribed from X because …").

- at most 40% of a batch from one platform and at most two items per author;
- praise, complaints, rituals and questions, recent and older;
- at least one query per batch aimed at the opposite of what most voices say.

**Peers.** The newsletters, digests, blogs with mail, and podcasts with show notes that readers of this
topic already get or would compare against. Read two or three recent issues of each from its public
archive and its signup page, and record all ten facts. Count words with a tool (`wc -w` on the issue's
text), never by eye. Then recount the grid, update the baseline (what most peers do: the newsletter this
niche already has) and list the gaps no peer fills.

**Minimums from zero:** [[.Project.MinVoices]] voices from [[.Project.MinPlatforms]] platforms and [[.Project.MinPeers]] peers.
**Stop** when the minimums hold and the last 10 voices brought no new platform and no new kind of need, or
at twice the minimums. A niche that truly cannot meet them gets `shortfall: <what and why>` in the batch
heading.

## [[.Project.Voices]] format

```markdown
# Voices: <newsletter>

## Batch <YYYY-MM-DD> · <from zero | ongoing: trigger> · queries: "<query>", "<query>" · unreachable: <platforms or none>

### V<NNN> · <platform> (<place>) · <[[.Project.Authors]]> · posted <YYYY-MM-DD | YYYY-MM | YYYY | unknown> · fetched <YYYY-MM-DD> · via <[[.Project.Methods]]>
> <exact text>
> <next line of the same text>
<https://source.url>
Observer note: <only when there is something to observe: votes, replies, context>
```

- IDs continue across batches (`V001`, `V002`, …) and are never reused.
- Author: `reader` would subscribe; `writer` writes about this topic (a peer's author, a blogger);
  `adjacent` lives close to it; `expert` answers professionally; `brand` speaks for a company.
- Posted: as precise as the source shows. "5 years ago" seen on 2026-10-20 is `2021`; never add a day or
  month the source does not give.
- Every quote line starts with `> `, blank ones with `>`. Fields never contain ` · `.

## [[.Project.Peers]] format

```markdown
# Peers: <newsletter>

## Batch <YYYY-MM-DD> · <from zero | ongoing: trigger>

### P<NNN> · <newsletter name> · https://archive.or.issue.url · fetched <YYYY-MM-DD> · via <[[.Project.Methods]]>
- Cadence: <from archive dates, e.g. weekly, Tuesdays>
- Length: <words of 2–3 issues you counted, e.g. 640 / 820 / 710>
- Structure: <sections of a typical issue, in order>
- Subject: <the pattern, with 2–3 exact subjects in quotes>
- Opening: <how an issue starts; quote its first words>
- Voice: <person, formality, humour, how readers are addressed>
- Visual: <plain text | light HTML | designed; images per issue; header>
- Links: <links per issue and how they are presented>
- Signup: <promise, proof, fields, incentive, whether frequency is stated>
- Takeaway: <what a reader gets that makes them stay>

### Grid after this batch

| Column | Value | Peers |
| --- | --- | --- |
| Cadence | weekly | 5 |

### Baseline

<One paragraph: the most frequent value per fact.>

### Gaps

- <a value no peer uses that readers ask for>
```

All ten facts, in this order, one line each; `none` or `unknown` when an issue shows nothing. Group values
into grid buckets you name; each column's counts add up to the number of peers in the file.

## Evidence (optional)

Facts that matter for this newsletter but not for every newsletter (a niche's regulation, a platform's
rules, a survey of this audience) go into [[.Project.Evidence]], cited as `E001`:

```markdown
# Evidence: <newsletter>

## Batch <YYYY-MM-DD> · from zero · queries: "<query>" · unreachable: <none>

### E<NNN> · <publisher> (<work>) · <[[.Project.EvidenceKinds]]> · published <YYYY-MM-DD | YYYY-MM | YYYY | unknown> · fetched <YYYY-MM-DD> · via <[[.Project.Methods]]>
> <exact sentences>
<https://source.url>
Applies: <dimensions, comma separated>
```

Dimensions: [[.Project.Dimensions]].

## Validate

Run `lfr check`. It parses all three files and reports every `research.*` finding with its line: fix
each one. Then check yourself that no earlier batch changed, that the minimums and the 40% platform limit
hold, and that every grid column adds up. The last line of `lfr check` names the next step, normally the
concept (skill lfr-concept).

## Gotchas

- Search snippets are not the post. Open the page.
- Newsletter archives often hide issues behind a "subscribe to read" wall: that is a wall; record the peer
  from what is public, or pick another.
- Substack archives have JSON at `/api/v1/archive?sort=new&limit=20`; a post's text is in its page.
- A comment without its own link uses the page's URL; the place names the thread.
- Interface text (ratings, "Reply" buttons) is not the author's words.
