---
name: lfr-launch
description: Launches an lfr newsletter once its concept is done - the list on the server, the welcome issue ([[.Project.Welcome]]), the signup page, legal footer details, and the first sends to a new domain. Use when lfr check names the launch step, when the list's settings drift from the brief, or when moving an existing list onto lfr. Not for server setup (lfr-setup) or regular issues (lfr-issue).
license: MIT
compatibility: Needs the lfr binary, on the server or anywhere with --remote user@host (LFR_REMOTE).
---

# Launching a newsletter

The concept is written; now the server has to say what it says. lfr check compares the list with the
brief and names each gap. The floor of a good list is built in and not optional: double opt-in, one-click
unsubscribe without login or survey, no "sorry to see you go" mail, unsubscribe by reply, soft-bounce
cleanup, a footer that tells each reader when and where they signed up, and an automatic pause when
complaints cross 0.3%. This skill sets up the rest.

## Workflow

```
- [ ] The domain serves and lfr doctor is clean (skill lfr-setup)
- [ ] The list exists with sender, name and website
- [ ] Legal details set: postal address, imprint and privacy links where required
- [ ] welcome.md written and clean in lfr check
- [ ] lfr push: promise, cadence and welcome issue on the server
- [ ] Signup page checked on a phone; a test signup confirmed end to end
- [ ] lfr check names the plan or draft step
```

## 1. The list

```
lfr list add <slug> --from <name>@<domain> --name "<newsletter>" --language <en|de> --host <news.domain>
```

- The slug is the one in [[.Project.Config]].
- `--from` is an address on a domain lfr serves. lfr creates a mailbox for it: replies land there, and
  readers notice when nobody answers (lfr:V112, lfr:E020). A no-reply sender is never right.
- The sender name is what readers see first and decides more than the subject (lfr:V116): the newsletter's
  or the owner's name, at most 20 characters (lfr:E085), the same on every issue (lfr:E021). Set it with
  `--from-name` when it differs from the list name.
- `--host` gives the list its own website (archive, signup, feed); point the name at the server
  (`lfr domain dns <domain>` lists the record). Without it the site lives under the mail host.
- Tracking stays off unless the owner wants it and the consent text covers it (lfr:E039, lfr:E043).
  Opens are unreliable anyway (lfr:E091, lfr:E094); lfr counts clicks and replies.

Legal: an address line in every footer (`--address "Name, Street 1, 12345 City"`), required by CAN-SPAM
(lfr:E047) and part of the German imprint (lfr:E044); for German lists also `--imprint-url`; a privacy link
with `--privacy-url`. Ask the owner for these; never invent an address.

## 2. The welcome issue

[[.Project.Welcome]] is an issue like any other, with front matter, and goes to each reader once, right after
they confirm. Write it from the brief's Welcome section:

```markdown
---
subject: <what the reader gets, not "Welcome!">
preheader: <when the first issue comes and what is in it>
---
<two or three short paragraphs: who writes, what comes and how often, when the next issue lands>

<the best two or three pieces from the archive, linked by what they are about>

<one question to answer by reply, and a promise that replies are read>
```

Keep it short, without selling (lfr:V047, lfr:V023). Say when the first regular issue comes (lfr:E083).
Ask why they subscribed: the answers are research (lfr:E117).

## 3. Push

```
lfr push
```

Copies the brief's promise (list description), its `Frequency:` line (shown at signup and in the
confirmation mail), the language and a minimum gap between issues to the list, and puts [[.Project.Welcome]]
on the server as its welcome issue. Run it again whenever the brief or the welcome issue change; lfr check
reports drift.

## 4. Signup page and first test

Open the list's website on a phone. The form asks for the address only and shows the frequency. Sign up
with an address you can read, confirm, and check that the welcome issue arrives with the right sender and a
working unsubscribe link. Embedding the form on another site: post `email` to `https://<host>/subscribe`
(form-encoded or JSON) and keep a hidden, empty `website` field as a bot trap; lfr answers every bot the
same as a person and limits signups per IP address and confirmation mails per hour.

The website shows the brief's Signup headline and pitch. To change its look, `lfr site <list> --eject
site` copies the templates into the project's `site/` folder; edit them (assets in `site/assets/`) and
`lfr push` uploads them, live only when every template parses.

Moving an existing list from another platform:

1. Export subscribed, unsubscribed and cleaned (bounced) readers separately; Mailchimp, Substack and
   others offer each.
2. `lfr reader block --csv <unsubscribed export> --reason "unsubscribed at <platform>"`, the same for
   the cleaned and complained exports: no later import ever mails them.
3. `lfr reader add <list> --csv <subscribed export> --confirmed --source "<platform>" --consent "<what
   they agreed to>"` only for readers who confirmed there; everyone else gets a confirmation mail
   without `--confirmed`. Opt-in time and IP columns (OPTIN_TIME, CONFIRM_TIME, OPTIN_IP, created_at,
   …) are kept as the consent proof and the date each footer names.
4. A list that has not heard from the sender for a year or more is stale: import it, then
   `lfr reader add <list> --csv <file> --reconfirm`. Everyone gets a confirmation mail; who does not
   click drops off after 30 days. Mailing it unasked bounces and gets the list paused.

Never import bought, scraped or guessed addresses (lfr:E008, lfr:E021).

## 5. The first weeks of a new domain

Mailbox providers trust a new domain slowly. Start with readers who asked for the newsletter, keep the
cadence steady, and grow volume gradually; M3AAWG counts about six weeks of warming (lfr:E053, lfr:E009).
Do not import a large old list and mail it all at once.

## Then

`lfr check` names the next step: plan the backlog, then the first issue (skill lfr-issue).
