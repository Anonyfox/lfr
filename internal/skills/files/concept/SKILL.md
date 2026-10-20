---
name: lfr-concept
description: Writes or revises [[.Project.Concept]], the editorial brief of an lfr newsletter - reader, promise, format, cadence, length, subjects, voice, pillars, signup, welcome, growth and guardrails - from the project's research and lfr's built-in craft evidence. Use when lfr check names the concept step, when an existing newsletter is rebuilt, or when learnings show the brief no longer fits. Not for collecting research (lfr-research) or writing issues (lfr-issue).
license: MIT
compatibility: Works with any toolset; needs the lfr binary for lfr check and lfr evidence.
---

# The concept of a newsletter

Every issue is held to [[.Project.Concept]]: lfr check reads its machine lines (cadence, word range, subject
length, phrases to use and avoid, pillars, guardrails) and flags drafts that drift. The brief turns
research into decisions. A decision without a cited source is taste, and taste drifts toward the typical
newsletter that readers already ignore.

## Before writing

1. `lfr check` must show the research complete. Read [[.Project.Voices]] and [[.Project.Peers]] in full.
2. Read the built-in craft evidence for every dimension you decide: `lfr evidence frequency`,
   `lfr evidence subject`, `lfr evidence welcome`, … (`lfr evidence` lists them). Cite it as `lfr:E012`.
3. Rebuilding a newsletter that exists? Read its last issues and numbers first and write a Diagnosis.
4. Collect what only the owner can decide (name, how often they can really write, personal stories,
   money) as `Open:` lines. lfr check reports each until the owner answered and the line is gone.

## Format

Sections in this order: [[.Project.Sections]]. A missing section is a decision nobody made.

```markdown
# Concept: <newsletter>

## Diagnosis
<only when rebuilding: what works, what is generic, why readers stay or leave, with numbers>

## Reader
<who subscribes, what they already read, where they come from, what they want from each issue. Cite V IDs.>

## Promise
<one sentence, at most 160 characters: what readers get, how often. It becomes the list's description.>
<why it is credible: who writes it, from what experience. Cite.>

## Voices
- <coded observation> V012, V031
- <at least [[.Project.MinVoiceNotes]] observations, each citing the voices behind it>

## Axes
| Axis | Market | Readers | Chosen |
| --- | --- | --- | --- |
| <length> | <what peers do, P IDs> | <what readers say, V IDs> | <the decision> |
<at least [[.Project.MinAxes]] axes where the market clusters and readers want something else>

## Format
1. <part>: <its job>
<[[.Project.MinFormat]] to [[.Project.MaxFormat]] numbered parts of a typical issue>

## Cadence
Every: <daily | weekly | biweekly | monthly | 3d | 2w>
Day: <weekday in English, optional>
Time: <HH:MM, optional>
Zone: <IANA zone, e.g. Europe/Berlin>
<why the owner can keep this for a year. Cite.>

## Length
Words: <min>-<max>
<why. Cite.>

## Subject
Max: <characters, 20 to 100>
<the style in words>
Example: "<subject>"
<at least [[.Project.MinExamples]] Example lines>

## Voice
<tone in prose: person, formality, humour, how readers are addressed>
Use: "<readers' own word>", "<…>"
Avoid: "<word the issues must not say>", "<…>"

## Pillars
- <name>: <what it covers>
<[[.Project.MinPillars]] to [[.Project.MaxPillars]] recurring themes; backlog ideas name one>

## Signup
Headline: <the website's first line>
Pitch: <one or two sentences under it>
Frequency: <how often mail comes, as the signup form says it, e.g. "Every Friday morning, five minutes to read">

## Welcome
<what welcome.md says and why>

## Growth
<how readers find the newsletter without paid ads>

## Guardrails
Unsubscribes: <percent per issue that triggers a review, e.g. 0.5%>
Complaints: <percent, e.g. 0.1%>
<what happens when an issue crosses them>

## Changelog
- <YYYY-MM-DD>: <what changed and the research batch or learning behind it>
```

Machine lines (`Every:`, `Words:`, `Max:`, `Example:`, `Use:`, `Avoid:`, `Headline:`, `Pitch:`,
`Frequency:`, `Unsubscribes:`, `Complaints:`) must be parseable; lfr check names any that are not.
Quoted phrases in `Use:` and `Avoid:` are matched in every issue.

## Deciding, section by section

The defaults below come from lfr's research; the project's own research overrides them when its readers
say otherwise. Cite whichever you follow.

**Reader and promise.** Name one reader precisely enough that a backlog idea is clearly for them or not.
A narrow niche is easier to write for and to grow (lfr:V125). The promise says what and how often: readers
want to know what to expect before they subscribe (lfr:E017, lfr:E056), and unexpected content is a top
reason to leave (lfr:E086).

**Format.** Put the substance in the email; teasers that send readers to a website are resented
(lfr:V017, lfr:V052). At most two full articles per issue; more than five topics become short headlines with
two to four sentence blurbs (lfr:E085). A fixed format lets readers know what they get and makes the issue
faster to assemble (lfr:V126); a stable section order is what the newsletters readers keep have in common
(lfr:P004, lfr:P013, lfr:P016). End with something to answer: replies are the honest signal (lfr:E112).

**Cadence.** Choose what the owner can keep for a year, then promise exactly that. Weekly is the most
common (lfr:E113) and the most common way writers burn out (lfr:V100, lfr:V103, lfr:V104); a weekly deadline
on a weekend broke one (lfr:V100); a monthly issue with room to breathe saved another (lfr:V105). A dense
monthly is valued (lfr:V055). Too much mail is the reason readers cite most for leaving (lfr:E134), and
raising the frequency later breaks the promise (lfr:E017, lfr:E050). Weekdays differ little (lfr:E105,
lfr:E113); pick a day and time readers have room, keep them steady, and send consistently (lfr:E009).
Say when the next issue comes (lfr:P010) and announce planned breaks before they happen (lfr:P012);
newsletters that say weekly and send every other week quietly lose that trust (lfr:P011, lfr:P015).
lfr refuses two issues within the list's minimum gap; `lfr push` sets it from the cadence.

**Length.** Shorter is better on phones, where most mail is read (lfr:E085, lfr:E122). Set the range
from the peers' baseline and what readers say about length, not from what is easy to fill.

**Subject.** Front-load the point: phones show 25 to 40 characters, desktops about 60 (lfr:E088,
lfr:E127). No emoji (lfr:E089, lfr:E106, lfr:V008), no first names (lfr:E138, lfr:E107), no fake Re: or
Fwd: (lfr:E006). Deceptive subjects are illegal (lfr:E045, lfr:E046). Write examples in the style the
issues should follow, from the readers' words.

**Voice.** The owner's voice, in the readers' words. Ask the owner for three things they wrote for
people, not for search engines: emails, posts, an old newsletter. Their own sentences are the strongest
guard against the generic register every model drifts to; describe what they do (how long their
sentences run, how they open, their humour, what they never say) and carry their characteristic phrases
into `Use:`. Readers unsubscribe from newsletters that read as
AI-written (lfr:V113, lfr:V015), and an automated AI newsletter grew nothing (lfr:V122). `Use:` holds
words readers use for their problem; `Avoid:` holds hype, jargon readers mock, and phrases from peers
readers resent.

**Signup.** Ask for the email address only (lfr:E083) and state the frequency and what comes first
(lfr:E083). Skip pop-ups (lfr:E083, lfr:V088). lfr always confirms by double opt-in: slower growth, far
better readers (lfr:V079, lfr:V073), the standard every provider and Spamhaus ask for (lfr:E050, lfr:E059).
Link two or three sample issues from the signup page; few peers do (lfr:P002, lfr:P005).

**Welcome.** One welcome issue, sent on confirmation (lfr:E083): say what comes and when, give the best of
the archive (lfr:V071), invite a reply and answer it (lfr:V112, lfr:E117). No sales pitch, no "welcome to
the family" (lfr:V047).

**Growth.** No paid ads. What works: mentions in other newsletters the readers read (lfr:V084, lfr:E115),
a few high-effort pieces that get shared (lfr:V085), being useful where readers talk (lfr:V084), an archive
that search engines and feed readers find (lfr:E118, lfr:V018), and a subscribe link in every issue for
forwarded copies (lfr:E086; lfr puts it in the footer).

**Guardrails.** Benchmarks put unsubscribes per issue around 0.15% to 0.5% (lfr:E098, lfr:E100, lfr:E103);
Gmail wants complaints below 0.1% and penalises 0.3% (lfr:E003, lfr:E014), where lfr pauses the list on
its own. Unsubscribes do not hurt deliverability (lfr:E134); complaints do. Never judge readers by opens:
privacy features fake or hide them (lfr:E091, lfr:E094), and readers who block tracking still read
(lfr:V075, lfr:V099).

## Validate

`lfr check` parses the brief, checks every cited ID against the research and lfr's evidence, and reports
open questions. Fix every `concept.*` finding. Its last line names the next step, normally the launch
(skill lfr-launch).

## Revising

When learnings, replies or new research contradict the brief, change it and add a Changelog line naming
the learning or batch. Never change the cadence silently: tell readers in the next issue.
