# The list websites: design note

lfr serves one small website per list from `internal/web/templates/`: the front page with the signup
form, the archive, each published issue, and the pages a reader lands on from a link in a mail. This
note records why they look the way they do. The inputs were lfr's own: the craft research in
`internal/craft/research/`, the lfr.email site and its concept, and the README.

## The pages and their jobs

| Template | Who arrives | What they came to do | States |
| --- | --- | --- | --- |
| `home` | someone sent the link, a search, a forwarded issue | decide whether to subscribe; skim what was sent | with issues and "All issues"; no issue yet; with or without cadence; plain or tracking consent |
| `archive` | a reader looking for an older issue | find it by date or subject | the list of every published issue |
| `issue` | a reader from "View in browser", the feed, a search | read one issue; maybe subscribe | the letter itself: headings, images, lists, quotes, tables, code, button links |
| `message` | a reader who clicked a link in a mail | finish one step: confirm, unsubscribe, or learn why a link failed | check inbox, confirm (button), confirmed, unsubscribe (button), unsubscribed, invalid link, error, too many attempts |
| `signup` | (part of home, archive and issue) | subscribe with one field | always with the consent line, with or without cadence |
| `layout` | every page | find the list's name, feed, imprint and privacy | with or without imprint and privacy links |

## What the inputs say

- lfr means "looking for readers"; lfr.email dresses it as a dispatch form: grey-blue ground, typewriter
  ink, postmark red, airmail blue (site/concept.md).
- Readers subscribe to hear from a person (V015) and want the substance in the mail, not a teaser
  (V017). A newsletter is a letter.
- Readers who leave want one click, not a preferences maze (V001-V003); readers forget the
  confirmation step of double opt-in (V066), so the "check your inbox" page has one job and nothing else.
- No pop-ups, no ads (V021, V088); an archive with a feed is how the writing stays findable (V018, V062).
- 99.96% of one platform's mails fail contrast (E063); small text takes effort to read (E066), and touch targets want 44 px (E122).
- Replies are the honest signal (E112): the reader writes back.

## Directions

1. ~~A quiet reading column, neutral sans, one accent colour.~~ First written; every newsletter host.
2. ~~An editorial magazine: big serif headline, drop caps, pull quotes.~~ Second written; Substack's
   and Ghost's house style.
3. ~~Plain-text letter in monospace at 72 columns.~~ Fits any mail product; mono hurts long reading.
4. ~~A product signup card with gradient and shadow.~~ Fits any product at all.
5. ~~Airmail stripes along every edge.~~ The first postal cliché any mail tool would pick.
6. ~~A library card catalogue for the archive.~~ Fits any archive; says nothing about mail.
7. ~~The lfr.email dispatch form, typewritten, on every list site.~~ lfr's own voice would drown the
   writer's; the list sites belong to the newsletter, not to lfr.
8. **Aerogramme.** Each issue is a letter on the pale blue, lightweight airmail sheet that folds into
   its own envelope; the signup is a reply card; the pages a mail link opens are the notices of
   registered post, since double opt-in is exactly a signature for delivery. Postal like
   lfr.email, but quiet enough to carry another writer's newsletter.
9. A correspondence register: the archive as the ledger of letters sent, dated in a margin column.
   Kept as the archive's form inside direction 8.
10. ~~A telegram: short capitals and stop marks.~~ Unreadable for issues of 800 words.

Chosen: 8, with 9 for the archive.

## Choices

Likelihoods are estimates of how often a page generator picks the option for a newsletter site.

**Type.** System sans for everything (40%), Inter from a font service (20%), Georgia body with a sans
heading (12%), `ui-serif` or Charter (6%), monospace (6%), an old-style book face from the system
(4%), a humanist sans stack such as Seravek or Gill Sans (3%), a slab serif stack (2%), a Didone (2%).
**Chosen: an old-style book face for the letter (Iowan Old Style, Palatino; 4%) and a slab for the
letterhead, headings, buttons and the date column (Rockwell; 2%).** Letters are set in book faces;
forms and postal printing in slabs. Both are installed on macOS, iOS and Windows (Palatino Linotype,
Rockwell with Office or Rockwell Nova); elsewhere the stacks fall back to P052 or DejaVu Serif and
the system serif. No web font: a list site costs no request beyond its own HTML.

**Colour roles.** White with grey and an indigo or blue accent (35%), cream with terracotta (12%),
stone neutrals with teal, the old default (10%), black and white with one neon (8%), navy with cyan
in dark mode (6%), manila with postmark red, the old lfr.email palette (2%), **aerogramme blue with
blue-black ink and postmark red (under 1%)**. Roles:

| Role | Light | Dark | From |
| --- | --- | --- | --- |
| Ground (`--ground`), the page | `#e6edf4` | `#0f1620` | aerogramme blue |
| Panel (`--panel`), cards and fields | `#f6f9fb` | `#172231` | the inside of the fold |
| Ink, text | `#142033` | `#e3eaf3` | blue-black fountain-pen ink |
| Faded ink, dates and notes | `#46546a` | `#a6b3c4` | diluted ink |
| Rule, lines and borders | `#9fb0c4` | `#3a4a5f` | printed form lines |
| Postmark, the one action | `#a3271f` | `#f08a7d` | cancellation stamps, as on lfr.email |
| Airmail, links | `#1d4a86` | `#9cc0f0` | airmail blue, as on lfr.email |

Body text has at least 11:1 against its ground in both schemes, faded ink at least 6:1, the button
label at least 6.9:1.

**Radius and shape.** 8 to 12 px rounded cards (45%), 4 px corners (15%), pills (15%), sharp
corners with heavy borders (10%), double rules from printed forms (3%), square corners with
perforated edges (2%). **Chosen: square corners everywhere; the reply card and the notices carry a
3 px ink top rule, the letterhead a double rule. No shadows.** The perforated top edge was removed on
2026-10-25: paper, notebook and receipt styling is banned on every page, and lfm's `design.ground`
fails torn, zigzag and perforated edges.

**Density.** An airy 680 px column with large gaps (40%), a card grid (20%), a dense three-column
magazine (5%), **a letter measure (about 34 em, 19 px text on desktop, 18 px on phones) with a
register for the archive: dates in their own column (3%)**. Chosen for reading at arm's length on a
phone, where most mail is read (E122).

## Rules for later changes

- One action colour; it marks the button that does the page's job.
- Headings and the date column in the slab, prose in the book face, nothing in monospace but code.
- `TestPagesAvoidStockDesign` renders every state and fails on the fonts and colours page generators
  default to; the values are copied from lfm's stock list.
