# Concept: lfr

## Voices
- Subscriber-tiered pricing punishes people who send rarely to many: hundreds per year for one mail a month or a year. V016 V017 V023 V129 V145 V178
- The free tier decides who stays on Substack; every alternative costs money from the first subscriber. V003 V148 V149 V167 V170 V134
- Cheap VPS providers block outbound port 25, and support will not lift it; newcomers find out after installing. V041 V045 V087 V090 V091 V115
- A relay for the last hop only is the compromise experienced self-hosters settle on; storage and control stay home. V050 V058 V099 V100 V127 V034
- Deliverability fear dominates every thread, Microsoft worst, Gmail close behind; even perfect scores land in junk. V035 V036 V048 V074 V095 V097 V142 V143
- People ask for software that is handed the domains and figures out DNS, keys and certificates itself, and wonder why it does not exist. V038 V044 V076 V077 V071 V049
- Small means a single binary on a small VPS with little memory; resource-hungry suites are counted in MiB and resented. V069 V081 V082 V083 V126 V141 V063
- One install often serves many domains: personal, joke, family, clients. V042 V047 V116 V174
- People already lean on LLM agents to run and repair mail servers, and agents need a narrow, safe interface. V057 V058 V059 V060 V052 V139
- Growth-hack and venture vibes repel the people who just want to write in Markdown and mail it. V019 V151 V152 V018 V146 V177
- The plumbing is the hidden work: bounces, unsubscribe headers, opt-out handling; tools skip it and people find out late. V026 V084 V120 V105 V086
- Upgrades that break configs and relay settings that do not survive updates erode trust in self-hosting. V096 V100 V123 V057
- Owning the archive, the domain and the RSS feed matters to writers who left platforms. V156 V172 V150 V007

## Buyer
A developer-minded writer or small operator with one or more domains and a modest list (a hundred to fifteen thousand readers) who sends rarely and feels robbed by tier pricing (V016 V017 V145 V178). They arrive from a Hacker News, Lemmy or Mastodon thread about self-hosting email or leaving Substack (V035 V121 V155), already know listmonk with SES, Mail-in-a-Box or mox (V028 V042 V069), and have an AI agent in their terminal (V057 V058). They compare against Substack's free tier and Buttondown's simplicity (V148 V018). To install they must believe three things: mail will reach inboxes, setup is a matter of minutes for their agent, and the total stays near five dollars a month. The hesitation most likely to stop them is deliverability: the expert voice that says never run your own mail server today (V048 V128).

## Decision path
1. What is this, in one line? → Opening: newsletters, list websites and mailboxes for your domains, one binary on a small VPS, run by your agent.
2. Will my mail arrive? → The honest route: port 25 is blocked, a relay carries the last hop, SPF, DKIM and DMARC pass, shown as the headers a receiver writes.
3. How much work is it for my domain? → The setup sheet: the ten DNS records for the domain the visitor types, and the prompt their agent runs.
4. What does it really cost per month? → The itemised bill: server plus cents per thousand mails, next to the visitor's current tool.
5. What will my readers get? → The actual issue a reader receives, rendered, with the footer that says why they get it.
6. What stops it from breaking? → The built-in floor: double opt-in, one-click unsubscribe, bounce handling, complaint pause, no admin UI to upgrade.
7. How do I start? → One install line, the skills, the repository.

## Mechanism
Chosen: a setup sheet. The visitor types a domain, the number of readers and issues per month; the sheet fills with the ten DNS records lfr will print for that domain (keys marked as generated on their server), an itemised monthly cost, and the exact prompt to paste into their agent. It answers steps 2 to 4 with their own domain, shows the work honestly instead of hiding it (V071 V077), and gives them something to take away (no peer does: P001 P003 P015).
Rejected: a cost calculator alone: proves the price but hides the work people fear most (V038 V049).
Rejected: a live demo inbox: needs a hosted backend and promises a delivery the visitor's own setup may not match; honesty risk (V048).
Rejected: a setup video: passive, dates fast, and every peer with a video shows a dashboard (P010 P012).

## Offer
Product: lfr, an MIT-licensed single Go binary: SMTP, list websites, outbound queue and a CLI with skills for agents.
Price: free and open source; the reader pays only a VPS (about $4–6 a month) and a relay's cents per thousand mails.
Delivery: an install script that downloads the release for Linux or macOS, then `lfr setup`; skills install with `lfr skills install`.
Open: Is the repository public at github.com/Anonyfox/lfr with a first release before the page goes live?

## Facts
- DigitalOcean blocks outbound SMTP port 25 for new accounts, and support will not make an exception. V041 V087
- Vultr refuses to lift the port 25 block on young accounts and suggests waiting 60 days. V090
- Amazon SES charges $0.16 per 1,000 emails on its Essentials plan. P026
- SMTP2GO's free plan sends 1,000 emails a month. V100 V127
- Mailgun's smallest paid plan costs $35 a month, the floor for a self-hosted Ghost newsletter. V101 V102
- Mailchimp cost $475 a month for 100,000 subscribers, Sendy plus SES $10 per send to all of them. V022
- A fresh Stalwart install uses 188 MiB of memory; a Postfix, Dovecot and OpenDKIM stack 64 MiB. V083
- Idle, the lfr server holds about 25 MB of memory, and its stripped binary is 18 MB. https://github.com/Anonyfox/lfr/blob/main/docs/operations.md
- lfr prints ten DNS records per domain: A and SPF for the mail host, MX, SPF, two DKIM keys (RSA and Ed25519), DMARC, MTA-STS policy and host, TLS reporting. https://github.com/Anonyfox/lfr/blob/main/docs/operations.md
- Gmail requires SPF, DKIM, DMARC and one-click unsubscribe from senders of 5,000 mails a day and keeps spam rates under 0.3%. https://support.google.com/a/answer/81126
- Gmail clips messages above 102 KB behind "View entire message". https://www.litmus.com/blog/how-to-keep-gmail-from-clipping-your-emails
- SES rewrites the Message-ID header of relayed mail, which breaks a DKIM signature that covers it. V072
- A Mail-in-a-Box relay set in Postfix's main.cf does not survive an update. V100

## Axes
| Axis | Market | Voices | Chosen |
| --- | --- | --- | --- |
| Opening | platform, ownership or category claim P001 P005 P008 P011 | the concrete job and why it should exist V044 V038 | the job in one line and the monthly bill under it |
| Ground | dark ground, white text, one bright accent P001 P003 P014 P016 P018 | voices name paper, envelopes and the postman's job, not dashboards V125 V127 | light paper ground, ink text, postmark red |
| Imagery | product UI screenshots and setup videos P005 P006 P007 P010 | people struggle with DNS records and want to see them V071 V077 | the real artifacts: DNS records, received headers, a delivered email |
| Mechanism | sign-up or free trial P001 P003 P015 P021 | hand the job to software or an agent V038 V058 | the setup sheet with the agent prompt to copy |
| Proof | logos, user counts, ratings P001 P003 P019 P022 | distrust of hype and venture money V019 V151 V152 | measured numbers and authentication results, no logos |
| Price | tiers by subscribers P001 P003 P021 P020 | predictable cost, pay per send V002 V022 V145 | an itemised bill: server plus cents per thousand |
| Tone | hype or plain technical P001 P022 P006 P009 | candid about limits V122 V125 V151 | candid clerk: say what lfr cannot fix (new domains, Microsoft) |

## Sparks
- "$5 VPS": the price of the whole server is the unit people count in. V044 V063 V126 V171
- Port 25 blocked: the wall newcomers hit after installing. V041 V045 V087 V090
- Relay only the last hop: storage at home, sending through someone whose job is reputation. V050 V127 V058
- "Why doesn't this exist already?": software that is handed domains and does the rest. V044 V038
- Markdown in, mail out. V018 V146 V177

## Directions
1. Inbox hero: a phone showing a newsletter landing in the inbox, headline about owning your list. V044 → struck: first direction, and the inbox shot is what every newsletter platform shows.
  A split screen: writer's Markdown left, reader's inbox right, animated delivery.
2. Terminal theatre: a dark terminal replaying an agent installing lfr, green text. V058 → struck: second direction, and dark ground with one bright accent is the market baseline.
  The replay pauses for the visitor to type their domain.
3. Setup sheet: a postal dispatch form for the visitor's domain: records, bill, prompt, stamped with SPF, DKIM and DMARC pass. V044 V071 → chosen
  The sheet is the page: each section is a numbered box of the form; the stamps are the received headers of a real lfr mail.
4. Port 25 map: a world map of hosting providers that block port 25, with the relay route drawn as a detour. V045 V090 → struck: a strong spark but a single fact; the visitor's own domain is missing.
  Hover each provider for its block policy and the alternate ports.
5. Receipt: the page as a till receipt itemising what beehiiv charges against what lfr costs. V016 V145 → struck: proves only price, and pits lfr against one peer.
  The receipt prints line by line as the visitor sets readers and sends.
6. Postman's bag: illustrated mail sorting, each letter a feature. V127 → struck: illustration and mascot territory of P020 and P004.
  Letters sort themselves into inbox, junk and bounce.
7. Changelog wall: the page as a live log of what lfr did for its own newsletter this week. V086 → struck: needs a running public instance and data the page does not have yet.
  Each event links to the record behind it.
8. Benchmark board: memory and binary size against mailcow, Stalwart and mox. V083 V082 → struck: invites comparison on the axis lfr matters least for, and turns the page into a competitor attack.
  A gauge per server on a 512 MB droplet.
9. Prompt card: a single giant prompt the visitor copies into their agent, nothing else. V058 V059 → struck: slows first value for people who want to see the records and cost before trusting an agent.
  The prompt expands into what the agent will do, step by step.

## Concept
A setup sheet for the visitor's own domain, laid out like a postal dispatch form on paper: numbered boxes, typewriter fields, a postmark in the corner. Typing a domain fills every box at once: the ten records lfr will print, the server and relay lines of the monthly bill, and the prompt for their agent, ready to copy. The postmarks are not decoration but the authentication results a receiving server writes into an lfr issue (spf=pass, dkim=pass, dmarc=pass), so the proof sits where the doubt is (V048). Port 25 gets its own box with the relay route spelled out, because that is the wall people hit (V041 V045), and the bill shows the whole cost in the unit people count in, a $5 VPS (V044 V063). The sparks recur in the sheet's boxes: the price unit in the bill, the blocked port in the route box, the last-hop relay in the route line, the "why doesn't this exist" job in the opening.

## Spine
1. Header: the job in one line, the bill in one line under it, and the domain field that fills the sheet. V044 V016
2. Box 1, records: the ten DNS records for the typed domain, each with what it does; keys marked as generated on the server. V071 V077
3. Box 2, route: how mail leaves a VPS whose port 25 is blocked: the relay for the last hop, alternate ports, and the received headers stamped pass. V041 V045 V127 V072
  Open: Paste the Authentication-Results header Gmail wrote into a real lfr issue, so Box 2 can show it verbatim.
4. Box 3, bill: server, relay and domain lines for the visitor's readers and sends, next to what tier pricing charges for the same list. V016 V022 V145
5. Box 4, the issue: the email a reader receives, rendered, with the footer saying when and where they signed up and the one-click unsubscribe. V026 V120
6. Box 5, the floor: what lfr refuses to get wrong (double opt-in, unsubscribe, bounces, complaint pause, no tracking by default, no admin UI). V084 V105 V151
7. Box 6, the agent: the prompt to copy and what the skills teach it; the agent never sees a password it does not need. V058 V052 V139
8. Footer: install line, repository, licence, the honest limits. V125 V122

## Palette
| Role | Color | Source |
| --- | --- | --- |
| Paper ground | #f3ecdc | manila dispatch forms and envelopes, the postal idiom no peer uses (gap in research) |
| Ink text | #1f1b16 | typewriter ribbon black on paper |
| Faded ink | #4f463a | carbon copy text for notes and labels |
| Postmark red | #a3271f | cancellation stamps; used for stamps and the primary action |
| Airmail blue | #1d4a86 | airmail stripes; links and field focus |
| Form rule | #b9ab90 | printed lines of a form |
| Field ground | #fbf7ee | the white of a filled-in field |

## Type
| Role | Family | Source |
| --- | --- | --- |
| Everything | Courier Prime, Courier New, monospace | typewriter of filled-in dispatch forms; mono keeps DNS records, headers and prompts aligned (V071) |

## Form
A single sheet, max 72 characters wide, centered on the paper ground, with numbered boxes drawn as 1.5px ruled frames with square corners, like boxes on a printed form. Labels sit small and uppercase in the top-left corner of each box. Fields are typewriter text on a slightly whiter ground with a ruled baseline. No shadows, no rounded cards; the only depth is the rotated postmark stamps overlapping box edges. On phones the sheet fills the width with a 16px margin and boxes stack.

## Imagery
| Image | Job | Art direction | Source |
| --- | --- | --- | --- |
| Postmarks | proof: authentication results as stamps | circular red cancellation marks with "spf=pass", "dkim=pass", "dmarc=pass" and the date, slightly rotated, ink texture by uneven stroke V048 V143 | drawn as inline SVG |
| Delivered issue | show what readers get | the real HTML of an lfr issue in a framed phone-width box, light mode V026 | rendered by lfr |
| Share preview | share preview | the sheet's header with domain field filled, postmark in the corner | rendered from the page |

## Motion
Moment: when the visitor finishes typing a domain, the three postmarks press onto the sheet one after another, 120 ms apart, with a small rotation settle.
Feedback: every field updates as the visitor types; copy buttons change to "copied" for two seconds.
Reduced motion: the stamps are simply present; fields update without transitions.

## Copy
- Use: "$5 VPS", "port 25", "relay", "DNS records", "deliverability", "Markdown" V044 V045 V127 V071 V048 V146
- Avoid: "grow", "monetize", "creator economy", "AI-powered", "revolutionary", "seamless" V152 V151 V019
- Tone: a postal clerk filling the form for you: plain, exact, numbers before adjectives, and honest about what lfr cannot fix: a brand-new domain needs weeks, Microsoft may still junk mail, a relay is part of the route. V122 V125 V073
- Every claim is something the visitor can check: a command, a record, a header, a number. V074 V141

## Changelog
- 2026-10-20: first concept from research batch 2026-10-20 (178 voices, 26 peers).
- 2026-10-20: Box 2 shows the route without received headers until a real lfr issue reached Gmail; the postmarks stay as the stamp motif.
