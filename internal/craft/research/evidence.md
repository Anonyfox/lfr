# Evidence: newsletter craft

## Batch 2026-10-20 · from zero · queries: "Gmail email sender guidelines", "Gmail sender guidelines FAQ", "Yahoo sender best practices", "Yahoo sender FAQ", "Outlook high-volume sender requirements", "Outlook.com postmaster", "iCloud Mail postmaster", "RFC 8058", "RFC 2369", "RFC 2919", "RFC 3464", "RFC 3463", "RFC 5965", "RFC 6376", "RFC 7208", "RFC 7489", "RFC 8461", "RFC 3834", "RFC 5322", "RFC 2046", "GDPR Art. 4 Art. 7 Art. 21 recital 32", "UWG § 7", "TDDDG § 25", "DDG § 5", "DDG § 6", "BGH I ZR 164/09 Double-opt-in", "EDPB Guidelines 2/2023 tracking pixel", "15 U.S.C. 7704", "CASL S.C. 2010 c. 23 s. 6 s. 10 s. 11", "M3AAWG Sender BCP 4.0", "Spamhaus marketing email FAQ", "Spamhaus confirmed opt-in", "Spamhaus subscription bombing", "Spamhaus spamtraps", "WCAG 2.2 1.4.3", "Email Markup Consortium accessibility report 2026", "Litmus accessible emails", "Litmus preview text", "Litmus dark mode", "caniemail prefers-color-scheme", "Litmus plain text", "SpamAssassin rules html uribl url_shortener", "Mailchimp Gmail clipping 102KB", "Outlook block picture downloads", "Gmail turn images on or off", "Apple Mail Privacy Protection" · opposite-view queries: "hidden preheader deliverability" (Litmus vs Gmail), "HTML-only spam score" (Litmus vs SpamAssassin), "confirmed opt-in insufficient against list bombing" (Spamhaus) · unreachable: FTC CAN-SPAM Compliance Guide (ftc.gov returned 404 to curl on all URL variants; statute text used instead), eCFR 16 CFR 316 (API returned empty), CRTC CASL FAQ crtc.gc.ca (empty response; statute text used instead), Email on Acid Gmail clipping article (403), Litmus Gmail clipping article (404), M3AAWG "Help! I Hit a Spam Trap" English version (only the Japanese PDF was linked; not translated, not used), BGH juris database (redirected to homepage; openJur full text used), Reddit (blocked), X/Twitter (login wall)

### E001 · Google (Email sender guidelines) · standard · published unknown · fetched 2026-10-20 · via fetch
> Starting February 1, 2024, all email senders who send email to Gmail accounts must meet the requirements in this section.
> …
> Set up SPF or DKIM email authentication for your sending domains.
> Ensure that sending domains or IPs have valid forward and reverse DNS records, also referred to as PTR records. Learn more
> Use a TLS connection for transmitting email. …
> Keep spam rates reported in Postmaster Tools below 0.3%. Learn more about spam rates
> Format messages according to the Internet Message Format standard, RFC 5322.
> Don’t impersonate Gmail From: headers. Gmail will begin using a DMARC quarantine enforcement policy, and impersonating Gmail From: headers might impact your email delivery.
> …
> Important: Sending to personal Gmail accounts requires a DKIM key of 1024 bits or longer. For security reasons, we recommend using a 2048-bit key if your domain provider supports this.
<https://support.google.com/a/answer/81126?hl=en>
Applies: deliverability
Observer note: Date: undated page (requirements effective February 1, 2024).

### E002 · Google (Email sender guidelines) · standard · published unknown · fetched 2026-10-20 · via fetch
> Starting February 1, 2024, email senders who send more than 5,000 messages per day to Gmail accounts must meet the requirements in this section.
> Set up SPF and DKIM email authentication for your domain.
> Set up DMARC email authentication for your sending domain. Your DMARC enforcement policy can be set to none. Learn more
> …
> For direct email, the domain in the sender's From: header must be aligned with either the SPF domain or the DKIM domain. This is required to pass DMARC alignment.
> Marketing messages and subscribed messages must support one-click unsubscribe, and include a clearly visible unsubscribe link in the message body. Learn more
<https://support.google.com/a/answer/81126?hl=en>
Applies: deliverability, unsubscribe
Observer note: Date: undated page (requirements effective February 1, 2024).

### E003 · Google (Email sender guidelines) · standard · published unknown · fetched 2026-10-20 · via fetch
> Keep spam rates reported in Postmaster Tools below 0.10% and avoid ever reaching a spam rate of 0.30% or higher. Learn more
> Maintaining a low spam rate helps senders be more resilient to occasional spikes in user feedback.
> Maintaining a high spam rate leads to increased spam classification. It can take time for improvements in spam rate to reflect positively on spam classification.
<https://support.google.com/a/answer/81126?hl=en>
Applies: deliverability, measurement

### E004 · Google (Email sender guidelines) · standard · published unknown · fetched 2026-10-20 · via fetch
> Make it easy to subscribe
> To help ensure recipients are engaged:
> Make sure recipients opt in to get messages from you.
> Confirm each recipient's email address before subscribing them.
> Periodically send messages to confirm that recipients want to stay subscribed.
> Consider unsubscribing recipients who don’t open or read your messages.
<https://support.google.com/a/answer/81126?hl=en>
Applies: consent, signup, hygiene, retention

### E005 · Google (Email sender guidelines) · standard · published unknown · fetched 2026-10-20 · via fetch
> To set up one-click unsubscribe for Gmail messages, include both of these headers in outgoing messages:
> List-Unsubscribe-Post: List-Unsubscribe=One-Click
> List-Unsubscribe: <https://solarmora.com/unsubscribe/example>
> …
> These unsubscribe options can also be used but they should not replace one-click unsubscribe:
> Let recipients review the individual mailing lists they’re subscribed to. Let them unsubscribe from lists individually, or all lists at once.
> Automatically unsubscribe recipients who have multiple bounced messages.
<https://support.google.com/a/answer/81126?hl=en>
Applies: unsubscribe, hygiene

### E006 · Google (Email sender guidelines) · standard · published unknown · fetched 2026-10-20 · via fetch
> Make sure every message includes a valid Message-ID.
> Make sure single-instance message headers are included only once in a message. Examples of single-instance headers include From:, To:, Subject:, and Date:.
> …
> For example, don’t send messages with subject lines starting with Re: or Fwd: unless the messages are actual replies or forwards, and include only actual senders and recipients in From: and To: headers.
> Don't use emojis or other non-standard characters to imitate graphic elements in messages, with the intent to deceive or influence recipients. …
> Don’t use HTML and CSS to hide content in your messages. Hiding content might cause messages to be marked as spam.
> Web links in the message body should be visible and easy to understand. Recipients should know what to expect when they click a link.
<https://support.google.com/a/answer/81126?hl=en>
Applies: subject, links, layout, preheader, deliverability
Observer note: contrast with E067 (Litmus says hidden preheader content no longer hurts deliverability).

### E007 · Google (Email sender guidelines) · standard · published unknown · fetched 2026-10-20 · via fetch
> Sender display names should be used exclusively to identify the sender.
> Display names should reflect a consistent, clear, and accurate statement of the sender's identity, name and/or organization.
> Don’t include subject or message content in display names.
> …
> The display name should not include the recipient’s name and should not imply a message reply or threaded conversation.
<https://support.google.com/a/answer/81126?hl=en>
Applies: subject, personalization, deliverability

### E008 · Google (Email sender guidelines) · standard · published unknown · fetched 2026-10-20 · via fetch
> Messages of the same category should have the same From: email address.
> …
> Don't mix different types of content in the same message. For example, don't include promotions in sales receipt messages.
> …
> Don't purchase email addresses from other companies.
> Don't send messages to people who didn't sign up to get messages from you. …
> Avoid opt-in forms that are checked by default and that automatically subscribe users.
<https://support.google.com/a/answer/81126?hl=en>
Applies: consent, signup, structure, deliverability

### E009 · Google (Email sender guidelines) · standard · published unknown · fetched 2026-10-20 · via fetch
> Frequency of sending email: You can increase the sending volume more quickly when you send daily instead of weekly.
> …
> Send email at a consistent rate. Avoid sending email in bursts.
> Start with a low sending volume to engaged users, and slowly increase the volume over time.
> …
> Avoid introducing sudden volume spikes if you do not have a history of sending large volumes. For example, immediately doubling previously sent volumes suddenly could result in rate limiting or reputation drops.
> …
> When you exceed your quota, Gmail typically rejects messages with SMTP error code 4.7.28. If you get this error:
> Do not send email for at least 10 minutes.
<https://support.google.com/a/answer/81126?hl=en>
Applies: deliverability, frequency

### E010 · Google (Email sender guidelines) · standard · published unknown · fetched 2026-10-20 · via fetch
> Open rate
> Google doesn't track open rates.
> Google can't verify the accuracy of open rates reported by third parties.
> Low open rates aren't necessarily an accurate indicator of deliverability or spam classification issues.
<https://support.google.com/a/answer/81126?hl=en>
Applies: measurement

### E011 · Google (Email sender guidelines FAQ) · standard · published unknown · fetched 2026-10-20 · via fetch
> A bulk sender is any email sender that sends close to 5,000 messages or more to personal Gmail accounts within a 24-hour period. Messages sent from the same primary domain count toward the 5,000 limit.
> Senders who meet the above criteria at least once are permanently considered bulk senders.
> …
> Starting November 2025, Gmail is ramping up its enforcement on non-compliant traffic. Messages that fail to meet the email sender requirements will experience disruptions, including temporary and permanent rejections.
<https://support.google.com/a/answer/14229414?hl=en>
Applies: deliverability

### E012 · Google (Email sender guidelines FAQ) · standard · published unknown · fetched 2026-10-20 · via fetch
> No. One-click unsubscribe is required only for marketing and promotional messages. Transactional messages are excluded from this requirement. Some examples of transactional messages are password reset messages, reservation confirmations, and form submission confirmations.
> …
> To reduce spam reports, protect your sending reputation, and keep your email lists healthy, we recommend that you fulfill unsubscribe requests within 48 hours, a reasonable timeline for removing recipients from a mailing list.
<https://support.google.com/a/answer/14229414?hl=en>
Applies: unsubscribe

### E013 · Google (Email sender guidelines FAQ) · standard · published unknown · fetched 2026-10-20 · via fetch
> List-Unsubscribe headers unsubscribe users directly by removing them from the mailing list. Other types of one-click unsubscribe, such as mailto and URL unsubscribe links, don’t meet our one-click unsubscribe requirement.
> …
> To meet our one-click unsubscribe requirement, you must use List-Unsubscribe email headers in all commercial, promotional messages, as described in RFC 8058 and in our Email sender guidelines, and include one HTTPS URL in the List-unsubscribe: header.
> …
> One-click unsubscribe links that link to a landing or other type of web page don’t comply with RFC 8058.
<https://support.google.com/a/answer/14229414?hl=en>
Applies: unsubscribe, links

### E014 · Google (Email sender guidelines FAQ) · standard · published unknown · fetched 2026-10-20 · via fetch
> Spam rate is calculated daily. …
> The user-reported spam rate’s impact on delivery is graduated, and rates of 0.3% or higher have an even greater negative impact on email inbox delivery. Even today, user-reported spam rates greater than 0.1% have a negative impact on email inbox delivery for bulk senders.
> Beginning June 2024, bulk senders with a user-reported spam rate greater than 0.3% will be ineligible for mitigation.
> …
> Bulk senders will be eligible for mitigation when their spam rates remain below 0.3% for 7 consecutive days.
<https://support.google.com/a/answer/14229414?hl=en>
Applies: deliverability, measurement

### E015 · Google (Email sender guidelines FAQ) · standard · published unknown · fetched 2026-10-20 · via fetch
> To ensure reliable authentication, we recommend all senders fully align DMARC to both SPF and DKIM. It’s likely that DMARC alignment with both SPF and DKIM will eventually be a sender requirement.
> …
> DMARC alignment isn’t required for forwarded or mailing list messages, which are sometimes referred to as indirect messages.
<https://support.google.com/a/answer/14229414?hl=en>
Applies: deliverability

### E016 · Yahoo (Sender Requirements & Recommendations) · standard · published unknown · fetched 2026-10-20 · via fetch
> Requirements for Bulk Senders:
> …
> Implement a functioning list-unsubscribe header, which supports one-click unsubscribe for marketing and subscribed messages
> The Post (RFC 8058) method is highly recommended
> The mail-to: method is acceptable
> Have a clearly visible unsubscribe link in the email body - this may direct to a preference page
> Honor unsubscribes within 2 days
> Keep spam complaint rates low
> Keep your spam rate below 0.3%
> Spam rate is calculated in our system based on mail delivered to the inbox - keep this in mind when referencing CFL data and calculating the rate in your own system
<https://senders.yahooinc.com/best-practices/>
Applies: unsubscribe, deliverability, measurement
Observer note: Yahoo accepts mailto for the header; Google (E013) says mailto does not meet its requirement. Date: undated page (enforcement beginning February 2024).

### E017 · Yahoo (Sender Requirements & Recommendations) · standard · published unknown · fetched 2026-10-20 · via fetch
> Honor the frequency of the list's intent. Don't start sending daily emails to subscribers of your weekly or monthly mailing.
> …
> When users subscribe to your mailing list, send them an email asking them to click to confirm their opt-in. This will improve the experience for users (who won't sign up accidentally or get signed up maliciously) and for your list (which won't contain uninterested people, fake email addresses, or most robots).
> Set recipient expectations clearly when users subscribe. Let them know what mail to expect, how often it will be sent, and what it will look like.
> …
> Monitor hard and soft bounces as well as inactive recipients.
> Reduce the number of invalid recipients by using double/confirm opt-in.
> Remove invalid recipients from your list promptly.
> Consider sending a reconfirmation email to inactive subscribers periodically.
<https://senders.yahooinc.com/best-practices/>
Applies: frequency, signup, consent, hygiene, retention

### E018 · Yahoo (Sender FAQs) · standard · published unknown · fetched 2026-10-20 · via fetch
> A “bulk” sender is classified as an email sender sending a significant volume of mail. We will not specify a volume threshold.
<https://senders.yahooinc.com/faqs/>
Applies: deliverability
Observer note: Google (E011) and Microsoft (E019) name 5,000 per day; Yahoo declines to name a number.

### E019 · Microsoft (Microsoft Defender for Office 365 Blog: Strengthening Email Ecosystem: Outlook’s Requirements for High‐Volume Senders) · standard · published 2025-04-02 · fetched 2026-10-20 · via fetch
> For domains sending over 5,000 emails per day, Outlook will soon require compliance with SPF, DKIM, DMARC. Non‐compliant messages will first be routed to Junk. If issues remain unresolved, they may eventually be rejected.
> …
> At least p=none and align with either SPF or DKIM (preferably both).
> …
> The rejected messages will be designated as "550; 5.7.515 Access denied, sending domain [SendingDomain] does not meet the required authentication level."
> …
> If you exceed 10 DNS lookups, your SPF check might fail.
> …
> Aim to remove inactive or invalid addresses regularly—monthly or quarterly. This lowers bounce rates, cuts costs, and reduces spam complaints.
<https://techcommunity.microsoft.com/blog/microsoftdefenderforoffice365blog/strengthening-email-ecosystem-outlook%E2%80%99s-new-requirements-for-high%E2%80%90volume-senders/4399730>
Applies: deliverability, hygiene

### E020 · Microsoft (Microsoft Defender for Office 365 Blog: Outlook’s Requirements for High‐Volume Senders) · standard · published 2025-04-02 · fetched 2026-10-20 · via fetch
> Compliant P2 (Primary) Sender Addresses: Ensure the “From” or “Reply‐To” address is valid, reflects the true sending domain, and can receive replies.
> Functional Unsubscribe Links: Provide an easy, clearly visible way for recipients to opt out of further messages, particularly for marketing or bulk mail.
> List Hygiene & Bounce Management: Remove invalid addresses regularly to reduce spam complaints, bounces, and wasted messages.
> Transparent Mailing Practices: Use accurate subject lines, avoid deceptive headers, and ensure your recipients have consented to receive your messages.
<https://techcommunity.microsoft.com/blog/microsoftdefenderforoffice365blog/strengthening-email-ecosystem-outlook%E2%80%99s-new-requirements-for-high%E2%80%90volume-senders/4399730>
Applies: engagement, unsubscribe, hygiene, subject, consent

### E021 · Apple (Postmaster information for iCloud Mail) · standard · published 2025-02-25 · fetched 2026-10-20 · via fetch
> All requirements must be met in order to send a bulk email. If not, the email will be rejected.
> Send only to recipients who explicitly subscribed to your emails (not list purchases, list rentals, or email appends).
> Offer an unsubscribe link, so that the recipient can unsubscribe immediately.
> …
> Use a consistent From: name and address to clearly identify your brand.
> …
> The sending domain must publish their Domain-based Message Authentication, Reporting, and Conformance (DMARC) policy.
> Track the temporary and permanent SMTP errors from our mail servers and act on them accordingly.
> Have a standard policy for handling bouncing addresses.
> Periodically remove inactive subscribers from your list.
> Don't reactivate email addresses that are already on your unsubscribe or suppression list.
> …
> We don't offer a feedback loop (FBL).
<https://support.apple.com/en-us/102322>
Applies: consent, unsubscribe, hygiene, deliverability

### E022 · IETF (RFC 8058 Signaling One-Click Functionality for List Email Headers) · standard · published 2017-01 · fetched 2026-10-20 · via fetch
> A mail sender that wishes to enable one-click unsubscriptions places one List-Unsubscribe header field and one List-Unsubscribe-Post header field in the message. The List-Unsubscribe header field MUST contain one HTTPS URI. It MAY contain other non-HTTP/S URIs such as MAILTO:. The List-Unsubscribe-Post header MUST contain the single key/value pair "List-Unsubscribe=One-Click". As described below, the message MUST have a valid DomainKeys Identified Mail (DKIM) signature that covers at least the List-Unsubscribe and List-Unsubscribe-Post headers.
>
> The URI in the List-Unsubscribe header MUST contain enough information to identify the mail recipient and the list from which the recipient is to be removed, so that the unsubscription process can complete automatically.
<https://www.rfc-editor.org/rfc/rfc8058.txt>
Applies: unsubscribe, deliverability
Observer note: RFC line wrapping joined into single lines; text otherwise unchanged.

### E023 · IETF (RFC 8058 Signaling One-Click Functionality for List Email Headers) · standard · published 2017-01 · fetched 2026-10-20 · via fetch
> The POST request MUST NOT include cookies, HTTP authorization, or any other context information.
> …
> The URI SHOULD include an opaque identifier or another hard-to-forge component in addition to, or instead of, the plaintext names of the list and the subscriber. The server handling the unsubscription SHOULD verify that the opaque or hard-to-forge component is valid.
> …
> The mail sender MUST NOT return an HTTPS redirect, since redirected POST actions have historically not worked reliably, and many browsers have turned redirected HTTP POSTs into GETs.
> …
> The mail receiver MUST NOT perform a POST on the HTTPS URI without user consent.
<https://www.rfc-editor.org/rfc/rfc8058.txt>
Applies: unsubscribe, links

### E024 · IETF (RFC 2369 The Use of URLs as Meta-Syntax for Core Mail List Commands and their Transport through Message Header Fields) · standard · published 1998-07 · fetched 2026-10-20 · via fetch
> MTAs generating the header fields SHOULD usually include a mailto based command, in addition to any other protocols used, in order to support users who do not have access to non-mail-based protocols.
> …
> The fields implemented on a given list SHOULD be included on all messages distributed by the list (including command responses to individual users), and on other messages where the message clearly applies to one distinct list. There MUST be no more than one of each field present in any given message.
>
> These fields MUST only be generated by mailing lists, not end users.
<https://www.rfc-editor.org/rfc/rfc2369.txt>
Applies: unsubscribe, structure

### E025 · IETF (RFC 2919 List-Id: A Structured Field and Namespace for the Identification of Mailing Lists) · standard · published 2001-03 · fetched 2026-10-20 · via fetch
> the owner of a mailing list MUST NOT generate list identifiers in any domain namespace for which they do not have authority.
> …
> In addition, a list identifier (list-id) MUST NOT be longer than 255 octets in length, for future compatibility.
> …
> This header SHOULD be included on all messages distributed by the list (including command responses to individual users), and on other messages where the message clearly applies to this particular distinct list. There MUST be no more than one of each field present in any given message.
> …
> As such the mailing list administrator SHOULD avoid changing the list identifier even when the host serving the list changes.
<https://www.rfc-editor.org/rfc/rfc2919.txt>
Applies: structure
Observer note: the last quoted sentence continues across a page break in the RFC; the words "changes." are taken from the following page.

### E026 · IETF (RFC 3834 Recommendations for Automatic Responses to Electronic Mail) · standard · published 2004-08 · fetched 2026-10-20 · via fetch
> An automatic responder MUST NOT blindly send a response for every message received.
> …
> Automatic responses SHOULD NOT be issued in response to any message which contains an Auto-Submitted header field (see below), where that field has any value other than "no".
> …
> Similarly, Service Responders SHOULD NOT cause unwanted side-effects (such as subscribing the sender to a mailing list) without reasonable assurance that the request was authorized by the affected party.
> …
> The auto-generated keyword:
> - SHOULD be used on messages generated by automatic (often periodic) processes (such as UNIX "cron jobs") which are not direct responses to other messages,
<https://www.rfc-editor.org/rfc/rfc3834.txt>
Applies: engagement, signup, consent

### E027 · IETF (RFC 3464 An Extensible Message Format for Delivery Status Notifications) · standard · published 2003-01 · fetched 2026-10-20 · via fetch
> The DSN MUST be addressed (in both the message header and the transport envelope) to the return address from the transport envelope which accompanied the original message for which the DSN was generated.
> …
> Whenever an SMTP transaction is used to send a DSN, the MAIL FROM command MUST use a NULL return address, i.e., "MAIL FROM:<>".
<https://www.rfc-editor.org/rfc/rfc3464.txt>
Applies: hygiene

### E028 · IETF (RFC 3463 Enhanced Mail System Status Codes) · standard · published 2003-01 · fetched 2026-10-20 · via fetch
> 4.XXX.XXX Persistent Transient Failure
>
> A persistent transient failure is one in which the message as sent is valid, but persistence of some temporary condition has caused abandonment or delay of attempts to send the message. If this code accompanies a delivery failure report, sending in the future may be successful.
>
> 5.XXX.XXX Permanent Failure
>
> A permanent failure is one which is not likely to be resolved by resending the message in the current form. Some change to the message or the destination must be made for successful delivery.
<https://www.rfc-editor.org/rfc/rfc3463.txt>
Applies: hygiene

### E029 · IETF (RFC 5965 An Extensible Format for Email Feedback Reports) · standard · published 2010-08 · fetched 2026-10-20 · via fetch
> To satisfy the requirements, an email feedback report is defined as a [MIME] message with a top-level MIME content type of "multipart/report" (as defined in [REPORT]). The following apply:
>
> a. The "report-type" parameter of the "multipart/report" type is set to "feedback-report";
> …
> e. Except as discussed below, each feedback report MUST be related to only a single email message.
<https://www.rfc-editor.org/rfc/rfc5965.txt>
Applies: hygiene, measurement

### E030 · IETF (RFC 6376 DomainKeys Identified Mail (DKIM) Signatures) · standard · published 2011-09 · fetched 2026-10-20 · via fetch
> Signers MUST implement and SHOULD sign using rsa-sha256.
> …
> Since short RSA keys more easily succumb to off-line attacks, Signers MUST use RSA keys of at least 1024 bits for long-lived keys.
> …
> The From header field MUST be signed (that is, included in the "h=" tag of the resulting DKIM-Signature header field).
<https://www.rfc-editor.org/rfc/rfc6376.txt>
Applies: deliverability

### E031 · IETF (RFC 7208 Sender Policy Framework (SPF)) · standard · published 2014-04 · fetched 2026-10-20 · via fetch
> multiple SPF records are not permitted for the same owner name.
> …
> SPF implementations MUST limit the total number of those terms to 10 during SPF evaluation, to avoid unreasonable load on the DNS. If this limit is exceeded, the implementation MUST return "permerror".
> …
> SPF implementations SHOULD limit "void lookups" to two.
<https://www.rfc-editor.org/rfc/rfc7208.txt>
Applies: deliverability

### E032 · IETF (RFC 7489 Domain-based Message Authentication, Reporting, and Conformance (DMARC)) · standard · published 2015-03 · fetched 2026-10-20 · via fetch
> A message satisfies the DMARC checks if at least one of the supported authentication mechanisms:
>
> 1. produces a "pass" result, and
>
> 2. produces that result based on an identifier that is in alignment, as defined in Section 3.
> …
> none: The Domain Owner requests no specific action be taken regarding delivery of messages.
>
> quarantine: The Domain Owner wishes to have email that fails the DMARC mechanism check be treated by Mail Receivers as suspicious.
> …
> reject: The Domain Owner wishes for Mail Receivers to reject email that fails the DMARC mechanism check. Rejection SHOULD occur during the SMTP transaction.
<https://www.rfc-editor.org/rfc/rfc7489.txt>
Applies: deliverability

### E033 · IETF (RFC 8461 SMTP MTA Strict Transport Security (MTA-STS)) · standard · published 2018-09 · fetched 2026-10-20 · via fetch
> Thus, for a Policy Domain of "example.com", the full URL is "https://mta-sts.example.com/.well-known/mta-sts.txt".
> …
> "max_age": Max lifetime of the policy (plaintext non-negative integer seconds, maximum value of 31557600).
> …
> "enforce": In this mode, Sending MTAs MUST NOT deliver the message to hosts that fail MX matching or certificate validation or that do not support STARTTLS.
<https://www.rfc-editor.org/rfc/rfc8461.txt>
Applies: deliverability

### E034 · IETF (RFC 5322 Internet Message Format) · standard · published 2008-10 · fetched 2026-10-20 · via fetch
> There are two limits that this specification places on the number of characters in a line. Each line of characters MUST be no more than 998 characters, and SHOULD be no more than 78 characters, excluding the CRLF.
<https://www.rfc-editor.org/rfc/rfc5322.txt>
Applies: plaintext, layout

### E035 · IETF (RFC 2046 MIME Part Two: Media Types) · standard · published 1996-11 · fetched 2026-10-20 · via fetch
> In general, user agents that compose "multipart/alternative" entities must place the body parts in increasing order of preference, that is, with the preferred format last. For fancy text, the sending user agent should put the plainest format first and the richest format last. Receiving user agents should pick and display the last format they are capable of displaying.
<https://www.rfc-editor.org/rfc/rfc2046.txt>
Applies: plaintext

### E036 · European Union (Regulation (EU) 2016/679 GDPR, Art. 4(11) and Recital 32) · standard · published 2016-04-27 · fetched 2026-10-20 · via fetch
> ‘consent’ of the data subject means any freely given, specific, informed and unambiguous indication of the data subject's wishes by which he or she, by a statement or by a clear affirmative action, signifies agreement to the processing of personal data relating to him or her;
> …
> Silence, pre-ticked boxes or inactivity should not therefore constitute consent. Consent should cover all processing activities carried out for the same purpose or purposes. When the processing has multiple purposes, consent should be given for all of them.
<https://eur-lex.europa.eu/legal-content/EN/TXT/HTML/?uri=CELEX:32016R0679>
Applies: consent, signup, legal

### E037 · European Union (Regulation (EU) 2016/679 GDPR, Art. 7 Conditions for consent) · standard · published 2016-04-27 · fetched 2026-10-20 · via fetch
> 1. Where processing is based on consent, the controller shall be able to demonstrate that the data subject has consented to processing of his or her personal data.
> 2. If the data subject's consent is given in the context of a written declaration which also concerns other matters, the request for consent shall be presented in a manner which is clearly distinguishable from the other matters, in an intelligible and easily accessible form, using clear and plain language. …
> 3. The data subject shall have the right to withdraw his or her consent at any time. … It shall be as easy to withdraw as to give consent.
<https://eur-lex.europa.eu/legal-content/EN/TXT/HTML/?uri=CELEX:32016R0679>
Applies: consent, unsubscribe, legal

### E038 · European Union (Regulation (EU) 2016/679 GDPR, Art. 21(2)) · standard · published 2016-04-27 · fetched 2026-10-20 · via fetch
> 2. Where personal data are processed for direct marketing purposes, the data subject shall have the right to object at any time to processing of personal data concerning him or her for such marketing, which includes profiling to the extent that it is related to such direct marketing.
<https://eur-lex.europa.eu/legal-content/EN/TXT/HTML/?uri=CELEX:32016R0679>
Applies: unsubscribe, legal

### E039 · European Data Protection Board (Guidelines 2/2023 on Technical Scope of Art. 5(3) of ePrivacy Directive, Version 2.0) · standard · published 2024-10-07 · fetched 2026-10-20 · via fetch
> In the case of an email, the sender may include a tracking pixel to detect when the receiver reads the email.
> …
> Both tracking links and tracking pixels can be distributed through a wide variety of channels, for example through emails, websites, or even, in the case of tracking links, through any kind of text messaging systems. That distribution to the user’s terminal equipment does constitute storage, at the very least through the caching mechanism of the client-side software. As such, Article 5(3) ePD is applicable, even if this storage is not permanent.
<https://www.edpb.europa.eu/system/files/2024-10/edpb_guidelines_202302_technical_scope_art_53_eprivacydirective_v2_en_0.pdf>
Applies: measurement, consent, legal
Observer note: PDF, paragraphs 48 and 50.

### E040 · Bundesministerium der Justiz (Gesetz gegen den unlauteren Wettbewerb, § 7 Abs. 2) · standard · published unknown · fetched 2026-10-20 · via fetch
> (2) Eine unzumutbare Belästigung ist stets anzunehmen
> …
> 2.
> bei Werbung unter Verwendung einer automatischen Anrufmaschine, eines Faxgerätes oder elektronischer Post, ohne dass eine vorherige ausdrückliche Einwilligung des Adressaten vorliegt, oder
> 3.
> bei Werbung mit einer Nachricht,
> a)
> bei der die Identität des Absenders, in dessen Auftrag die Nachricht übermittelt wird, verschleiert oder verheimlicht wird oder
> …
> c)
> bei der keine gültige Adresse vorhanden ist, an die der Empfänger eine Aufforderung zur Einstellung solcher Nachrichten richten kann, ohne dass hierfür andere als die Übermittlungskosten nach den Basistarifen entstehen.
<https://www.gesetze-im-internet.de/uwg_2004/__7.html>
Applies: consent, unsubscribe, legal
Observer note: Date: undated page (current consolidated text).

### E041 · Bundesministerium der Justiz (Gesetz gegen den unlauteren Wettbewerb, § 7 Abs. 3) · standard · published unknown · fetched 2026-10-20 · via fetch
> (3) Abweichend von Absatz 2 Nummer 2 ist eine unzumutbare Belästigung bei einer Werbung unter Verwendung elektronischer Post nicht anzunehmen, wenn
> 1.
> ein Unternehmer im Zusammenhang mit dem Verkauf einer Ware oder Dienstleistung von dem Kunden dessen elektronische Postadresse erhalten hat,
> 2.
> der Unternehmer die Adresse zur Direktwerbung für eigene ähnliche Waren oder Dienstleistungen verwendet,
> 3.
> der Kunde der Verwendung nicht widersprochen hat und
> 4.
> der Kunde bei Erhebung der Adresse und bei jeder Verwendung klar und deutlich darauf hingewiesen wird, dass er der Verwendung jederzeit widersprechen kann, ohne dass hierfür andere als die Übermittlungskosten nach den Basistarifen entstehen.
<https://www.gesetze-im-internet.de/uwg_2004/__7.html>
Applies: consent, unsubscribe, legal
Observer note: Date: undated page (current consolidated text).

### E042 · Bundesgerichtshof via openJur (Urteil vom 10.02.2011, I ZR 164/09, Double-opt-in-Verfahren) · standard · published 2011-02-10 · fetched 2026-10-20 · via fetch
> a) Geht ein Teilnahmeantrag elektronisch ein, so kann dessen Absender durch eine E-Mail um Bestätigung seines Teilnahmewunsches gebeten werden. Nach Eingang der erbetenen Bestätigung kann angenommen werden, dass der Antrag tatsächlich von der angegebenen E-Mail-Adresse stammt. Hat der Verbraucher durch Setzen eines Häkchens in dem Teilnahmeformular bestätigt, dass er mit der Übersendung von Werbung einverstanden ist, ist grundsätzlich hinreichend dokumentiert, dass er in E-Mail-Werbung an diese E-Mail-Adresse ausdrücklich eingewilligt hat …
> …
> Kann der Verbraucher darlegen, dass die Bestätigung nicht von ihm stammt, war die Werbezusendung auch dann wettbewerbswidrig, wenn die E-Mail-Adresse im Doubleoptin-Verfahren gewonnen wurde.
<https://openjur.de/u/168642.html>
Applies: consent, signup, legal
Observer note: the case concerned telephone advertising; the court held the double opt-in confirmation insufficient as proof of consent to calls, while treating it as documentation for e-mail advertising.

### E043 · Bundesministerium der Justiz (Telekommunikation-Digitale-Dienste-Datenschutz-Gesetz, § 25 Abs. 1) · standard · published unknown · fetched 2026-10-20 · via fetch
> (1) Die Speicherung von Informationen in der Endeinrichtung des Endnutzers oder der Zugriff auf Informationen, die bereits in der Endeinrichtung gespeichert sind, sind nur zulässig, wenn der Endnutzer auf der Grundlage von klaren und umfassenden Informationen eingewilligt hat. Die Information des Endnutzers und die Einwilligung haben gemäß der Verordnung (EU) 2016/679 zu erfolgen.
<https://www.gesetze-im-internet.de/ttdsg/__25.html>
Applies: measurement, consent, legal
Observer note: the statute itself does not mention e-mail or pixels; E039 is the EU-level text applying Art. 5(3) ePD (which § 25 implements) to e-mail tracking pixels. Date: undated page (current consolidated text).

### E044 · Bundesministerium der Justiz (Digitale-Dienste-Gesetz, § 5 Abs. 1 Allgemeine Informationspflichten) · standard · published unknown · fetched 2026-10-20 · via fetch
> (1) Diensteanbieter haben für geschäftsmäßige, in der Regel gegen Entgelt angebotene digitale Dienste folgende Informationen, die leicht erkennbar und unmittelbar erreichbar sein müssen, ständig verfügbar zu halten:
> 1.
> den Namen und die Anschrift, unter der sie niedergelassen sind, bei juristischen Personen zusätzlich die Rechtsform, den Vertretungsberechtigten …
> 2.
> Angaben, die eine schnelle elektronische Kontaktaufnahme und eine unmittelbare Kommunikation mit ihnen ermöglichen, einschließlich der Adresse für die elektronische Post,
> …
> 4.
> die Angabe des Handelsregisters oder ähnlicher Register, in das sie eingetragen sind, und die entsprechende Registernummer,
> …
> 6.
> in Fällen, in denen sie eine Umsatzsteueridentifikationsnummer nach § 27a Absatz 1 Satz 1, 2 oder 3 des Umsatzsteuergesetzes oder eine Wirtschafts-Identifikationsnummer nach § 139c Absatz 1 der Abgabenordnung besitzen, die Angabe dieser Nummer,
<https://www.gesetze-im-internet.de/ddg/__5.html>
Applies: legal
Observer note: Date: undated page (current consolidated text).

### E045 · Bundesministerium der Justiz (Digitale-Dienste-Gesetz, § 6 Abs. 2) · standard · published unknown · fetched 2026-10-20 · via fetch
> (2) Werden kommerzielle Kommunikationen per elektronischer Post versandt, darf in der Kopf- und in der Betreffzeile weder der Absender noch der kommerzielle Charakter der Nachricht verschleiert oder verheimlicht werden. Ein Verschleiern oder Verheimlichen liegt dann vor, wenn die Kopf- oder die Betreffzeile absichtlich so gestaltet ist, dass der Empfänger vor Einsichtnahme in den Inhalt der Kommunikation keine oder irreführende Informationen über die tatsächliche Identität des Absenders oder den kommerziellen Charakter der Nachricht erhält.
<https://www.gesetze-im-internet.de/ddg/__6.html>
Applies: subject, legal
Observer note: Date: undated page (current consolidated text).

### E046 · Cornell Legal Information Institute (15 U.S. Code § 7704(a)(1)–(2), CAN-SPAM) · standard · published unknown · fetched 2026-10-20 · via fetch
> a “from” line (the line identifying or purporting to identify a person initiating the message) that accurately identifies any person who initiated the message shall not be considered materially false or materially misleading; and
> …
> (2) Prohibition of deceptive subject headings
> It is unlawful for any person to initiate the transmission to a protected computer of a commercial electronic mail message if such person has actual knowledge, or knowledge fairly implied on the basis of objective circumstances, that a subject heading of the message would be likely to mislead a recipient, acting reasonably under the circumstances, about a material fact regarding the contents or subject matter of the message …
<https://www.law.cornell.edu/uscode/text/15/7704>
Applies: subject, legal
Observer note: Date: undated page (statute text).

### E047 · Cornell Legal Information Institute (15 U.S. Code § 7704(a)(3)–(5), CAN-SPAM) · standard · published unknown · fetched 2026-10-20 · via fetch
> (ii)
> remains capable of receiving such messages or communications for no less than 30 days after the transmission of the original message.
> …
> (i)
> for the sender to initiate the transmission to the recipient, more than 10 business days after the receipt of such request, of a commercial electronic mail message that falls within the scope of the request;
> …
> (A) It is unlawful for any person to initiate the transmission of any commercial electronic mail message to a protected computer unless the message provides—
> (i)
> clear and conspicuous identification that the message is an advertisement or solicitation;
> (ii)
> clear and conspicuous notice of the opportunity under paragraph (3) to decline to receive further commercial electronic mail messages from the sender; and
> (iii)
> a valid physical postal address of the sender.
> (B)
> Subparagraph (A)(i) does not apply to the transmission of a commercial electronic mail message if the recipient has given prior affirmative consent to receipt of the message.
<https://www.law.cornell.edu/uscode/text/15/7704>
Applies: unsubscribe, legal
Observer note: Date: undated page (statute text).

### E048 · Government of Canada, Justice Laws (Canada's Anti-Spam Legislation S.C. 2010, c. 23, s. 6) · standard · published unknown · fetched 2026-10-20 · via fetch
> 6 (1) It is prohibited to send or cause or permit to be sent to an electronic address a commercial electronic message unless
> (a) the person to whom the message is sent has consented to receiving it, whether the consent is express or implied; and
> (b) the message complies with subsection (2).
> …
> (2) The message must be in a form that conforms to the prescribed requirements and must
> (a) set out prescribed information that identifies the person who sent the message and the person — if different — on whose behalf it is sent;
> (b) set out information enabling the person to whom the message is sent to readily contact one of the persons referred to in paragraph (a); and
> (c) set out an unsubscribe mechanism in accordance with subsection 11(1).
<https://laws-lois.justice.gc.ca/eng/acts/E-1.6/page-1.html>
Applies: consent, unsubscribe, legal
Observer note: Date: act current to 2026-09-30.

### E049 · Government of Canada, Justice Laws (Canada's Anti-Spam Legislation S.C. 2010, c. 23, s. 10 and 11) · standard · published unknown · fetched 2026-10-20 · via fetch
> 10 (1) A person who seeks express consent for the doing of an act described in any of sections 6 to 8 must, when requesting consent, set out clearly and simply the following information:
> (a) the purpose or purposes for which the consent is being sought;
> …
> (2) The person who sends the commercial electronic message and the person — if different — on whose behalf it is sent must ensure that the electronic address or World Wide Web page referred to in paragraph (1)(b) is valid for a minimum of 60 days after the message has been sent.
> (3) The person who sent the commercial electronic message and the person — if different — on whose behalf the message was sent must ensure that effect is given to an indication sent in accordance with paragraph (1)(b) without delay, and in any event no later than 10 business days after the indication has been sent, without any further action being required on the part of the person who so indicated.
<https://laws-lois.justice.gc.ca/eng/acts/E-1.6/page-2.html>
Applies: consent, signup, unsubscribe, legal
Observer note: the subsections (2) and (3) quoted are of section 11. Date: act current to 2026-09-30.

### E050 · M3AAWG (Sender Best Common Practices Version 4.0) · guide · published 2026-08 · fetched 2026-10-20 · via fetch
> Single opt-in should be used with extreme caution as it allows unconfirmed addresses to be added to mailing lists. This means typos or outright forgeries can be added unchecked, creating invalid consent and leading to complaints, bounces, and eventual degradation of a sender’s reputation.
> …
> 4. The sender should notify recipients of the potential frequency and type of communications, and should give recipients a choice in each of these areas. Unexpected increases in the frequency of messages often leads to increased abuse complaints.
> …
> 7. At the time of address collection, a sender should consider what other types of data should be retained regarding the sign-up, such as IP address, date of collection, and the website or event where the collection occurred.
<https://www.m3aawg.org/sites/default/files/doc_files/m3aawg-sender-best-common-practices-aug-27-2026--1-.pdf>
Applies: consent, signup, frequency, legal
Observer note: PDF; replaces version 3.0 (February 2015).

### E051 · M3AAWG (Sender Best Common Practices Version 4.0) · guide · published 2026-08 · fetched 2026-10-20 · via fetch
> 1. A confirmation message should be sent (see 2.a, b above) immediately following an end user’s submitting their address to a list, or as soon as possible thereafter (within 24 hours).
> …
> 2. The message should include notification of the frequency and type of content of the mailings.
> a. The message should use the same “from” address as future messages so recipients may add it to their address book or whitelist, if desired.
> …
> Confirmed opt-in (also called “double opt-in” or “closed-loop subscription”) is the highest standard opt-in best practice. … recipients of the confirmation message are required to take an affirmative action to be added to the list.
> …
> b. The confirmation email should be simple and free of advertising to prevent it being identified and filtered as messaging abuse.
> c. If a user updates their email address in your system, the new address should also be confirmed in this same way.
<https://www.m3aawg.org/sites/default/files/doc_files/m3aawg-sender-best-common-practices-aug-27-2026--1-.pdf>
Applies: signup, welcome, consent, frequency

### E052 · M3AAWG (Sender Best Common Practices Version 4.0, 4.2 List Bombing) · guide · published 2026-08 · fetched 2026-10-20 · via fetch
> List bombing relies on a script filling out hundreds of subscription forms to one email address.
> …
> 1. Add a CAPTCHA to the form that will not submit without it being properly completed.
> …
> 3. Add in rate limits preventing forms from being submitted multiple times by the same IP over a period of time.
> 4. A sender or ESP hosting multiple forms should monitor for the same address being added to multiple different forms.
> …
> 6. Add blank fields to the form that only a bot might fill out, but a human would never see.
> …
> 7. Create a field that looks at the time stamp or generated key for the page load. If the submission time is less than a reasonable time you think it might take for someone to fill it out, or if that time is missing entirely, toss the submission.
> …
> 10. Consider adding a Form-Sub header for receiving mailbox providers.
<https://www.m3aawg.org/sites/default/files/doc_files/m3aawg-sender-best-common-practices-aug-27-2026--1-.pdf>
Applies: signup

### E053 · M3AAWG (Sender Best Common Practices Version 4.0, Domain Warming) · guide · published 2026-08 · fetched 2026-10-20 · via fetch
> When sending from a new subdomain, to be safe, domain warming is recommended even if the organizational level domain already has an established reputation. Artificial warming practices are not recommended and may be illegal.
> …
> Start low and slow; increase sending volume slowly. (Consider 6 weeks of warm-up as an average.)
> Spread messages out over time. Do not send your daily batch all at once.
> …
> It’s preferable to send campaigns on a regular basis until the domain is sufficiently warmed up.
<https://www.m3aawg.org/sites/default/files/doc_files/m3aawg-sender-best-common-practices-aug-27-2026--1-.pdf>
Applies: deliverability, frequency

### E054 · M3AAWG (Sender Best Common Practices Version 4.0) · guide · published 2026-08 · fetched 2026-10-20 · via fetch
> It is a recommended best practice that addresses should be removed from a list if they consistently bounce with any failure code or descriptive text over multiple consecutive campaigns. How many campaigns and over what length of time are up to the individual sender but generally it is considered a best practice to remove an address from the list if it bounces consecutively at least two times over two weeks or more.
<https://www.m3aawg.org/sites/default/files/doc_files/m3aawg-sender-best-common-practices-aug-27-2026--1-.pdf>
Applies: hygiene
Observer note: Spamhaus (E057) states three sequential bounces instead of two.

### E055 · M3AAWG (Sender Best Common Practices Version 4.0, 4.6 Unsubscribe) · guide · published 2026-08 · fetched 2026-10-20 · via fetch
> 2. Senders must process all unsubscribe requests without delay to remain in compliance with all applicable laws and as a sign of respect for the recipient.
> …
> 4. Senders should have the capability to process email-based unsubscribe requests via the From and Reply-to addresses in the outbound email.
> …
> 12. Subscribers must be able to unsubscribe without having to log in to a preference center or experience any other form of security challenge.
> …
> 14. The subscriber’s email address should not be visible within the unsubscribe link itself. Links should utilize encoded references to protect personal data rather than displaying them in clear text.
<https://www.m3aawg.org/sites/default/files/doc_files/m3aawg-sender-best-common-practices-aug-27-2026--1-.pdf>
Applies: unsubscribe, engagement, links

### E056 · Spamhaus (FAQs: Marketing Email) · guide · published unknown · fetched 2026-10-20 · via fetch
> Spamhaus believes the only way email will stay a valid and useful channel is if users only receive mail they asked for. No email should be sent until and unless there is direct and verifiable permission. Our standard recommendation is to only send mail to addresses that have gone through a confirmed opt-in process.
> …
> Truth in advertising: The policies and nature of the e-mail program should be stated at the point of subscription. Appropriate expectations should be set and delivered: how often, what kind, what topics and content, etc.
<https://www.spamhaus.org/faqs/marketing-email/>
Applies: consent, signup, frequency

### E057 · Spamhaus (FAQs: Marketing Email) · guide · published unknown · fetched 2026-10-20 · via fetch
> Stagnant lists provoke high complaint rates when they are reactivated, even from truly COI addresses. Addresses are constantly abandoned or re-used. For most commercial lists, a good rule of thumb is to mail at least once per week and remove any address with three sequential bounces, or that provoke sequential bounces for more than two weeks concurrently.
> …
> Secure Webforms: As of October 2016, webforms that accept email subscriptions need to be protected in some manner due to systemic abuse. CAPTCHA is a good solution but there are others.
> …
> SMTP “5xy” codes mean “Do not try again”. SMTP “4xy” codes – also known as temp fails – mean “try again later” …
<https://www.spamhaus.org/faqs/marketing-email/>
Applies: hygiene, frequency, signup

### E058 · Spamhaus (Subscription Bombing: COI, CAPTCHA, and the Next Generation of Mail Bombs) · guide · published 2016-09-16 · fetched 2026-10-20 · via fetch
> One company saw nine specific addresses signed up over 9,000 times over the course of two weeks, creating 81,000 confirmation emails.
> …
> These signups were made possible by the fact that many web forms use Single Opt-In (SOI) and accept all subscriptions without any verification, though in this case even using Confirmed Opt-In (COI) didn't help much because the volume of confirmation emails alone was enough to cause a substantial problem.
> …
> The single best thing that can be done to secure a form and avoid becoming an attack vector is to put a CAPTCHA on it. … Even using COI in this situation is not sufficient, as the sheer volume of confirmation emails can be completely overwhelming. Use CAPTCHA plus COI to protect your mailing list subscription form!
<https://www.spamhaus.org/resource-hub/threat-intelligence/subscription-bombing-coi-captcha-and-the-next-generation-of-mail-bombs/>
Applies: signup, consent

### E059 · Spamhaus (Confirmed opt-in or opt-out?) · guide · published 2022-02-15 · fetched 2026-10-20 · via fetch
> Confirmed Opt-In (COI) is the gold standard.
> …
> You won’t regularly hit spamtraps – Spamtraps do not interact with email, so you significantly reduce the likelihood of poisoning a contact list with a trap.
> …
> It makes investigations easier to resolve. When an ISP, filter vendor or reputation provider refuses a sender’s email, they often require proof of consent as part of the investigation.
<https://www.spamhaus.org/resource-hub/deliverability/confirmed-opt-in-or-opt-out/>
Applies: consent, signup, hygiene

### E060 · Spamhaus (Spamtraps – fix the problem, not the symptom) · guide · published 2022-02-15 · fetched 2026-10-20 · via fetch
> We strongly urge people to view spamtraps as proof of a data collection or hygiene issue and not be misled into conducting a hunt for spamtraps.
> …
> Typo Domain Traps
> These are traps at domains that are similar to common domains: yaaho.com or ynail.com, homail.com, etc. … Employing COI at the data collection point is a perfect way to avoid your lists being polluted by these nuisance traps.
> Dead Address Traps
> “Dead” traps were once-valid email addresses that ISPs have turned off. All mail to these addresses is rejected with a hard bounce for a period of time, often 12 months or more. After consistently rejecting mail for a pre-determined period, the addresses are silently turned back on in the form of spamtraps.
<https://www.spamhaus.org/resource-hub/deliverability/spamtraps-fix-the-problem-not-the-symptom/>
Applies: hygiene, signup

### E061 · W3C (Understanding Success Criterion 1.4.3: Contrast (Minimum), WCAG 2.2) · standard · published unknown · fetched 2026-10-20 · via fetch
> The visual presentation of text and images of text has a contrast ratio of at least 4.5:1, except for the following:
> …
> The 3:1 and 4.5:1 contrast ratios referenced in this success criterion are intended to be treated as threshold values. When comparing the computed contrast ratio to the Success Criterion ratio, the computed values should not be rounded (e.g., 4.499:1 would not meet the 4.5:1 threshold).
<https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html>
Applies: accessibility, darkmode

### E062 · Email Markup Consortium (Accessibility Report 2026) · study · published 2026-05-21 · fetched 2026-10-20 · via fetch
> The 2026 Email Accessibility Report reveals an industry at a technical standstill. Despite a marginal uptick, the email ecosystem remains in a state of systemic failure: 99.88% of the 376,348 emails analyzed this year contain ‘Serious’ or ‘Critical’ accessibility defects, leaving only a 0.002% pass rate represented by just eight emails from three brands.
> …
> No email clients support all HTML/CSS accessibility features tested on “Can I Email?”
<https://emailmarkup.org/en/reports/accessibility/2026/>
Applies: accessibility
Observer note: sample 376,404 emails collected May 2025 to May 2026, 376,348 analyzed; tested with the Parcel accessibility checker (automated). Date: last modified.

### E063 · Email Markup Consortium (Accessibility Report 2026) · study · published 2026-05-21 · fetched 2026-10-20 · via fetch
> We detected 6,909 Substack emails, 1,468 Shopify emails, and 2,189 Beehiiv emails. These have only been tracked for the last six months, however emails from these senders may still appear in the full twelve months of data used elsewhere in this report. 100% of the emails found from each of these platforms failed our testing.
> …
> 99.96% of emails had color contrast issues (Element has insufficient color contrast). Only 3 emails out of the 6,909 didn’t have this issue.
> …
> 99.59% of emails had non-descriptive link text (Link text should be descriptive). Such a high percentage suggests something systemic, such as poor default link text. Only 9 emails out of 2,189 did not have this issue.
<https://emailmarkup.org/en/reports/accessibility/2026/>
Applies: accessibility, links
Observer note: the 99.96% contrast figure is for Substack; the 99.59% link-text figure is for Beehiiv. Date: last modified.

### E064 · Email Markup Consortium (Accessibility Report 2026) · study · published 2026-05-21 · fetched 2026-10-20 · via fetch
> (1) Serious: Content inside the body should be wrapped in a dir attribute
> Issue present in 97.41% (366,558) of emails tested.
> …
> (2) Serious: Content inside the body should be wrapped in a lang attribute
> Issue present in 95.66% (359,950) of emails tested.
> …
> A number of email clients will remove the language setting on the <html> element so we need to also include it on any direct children of the body element.
> …
> (3) Serious: Tables used for formatting should have the role attribute set to presentation or none
> Issue present in 83.78% (315,272) of emails tested.
> …
> (4) Mild: Page should contain a level-one heading
> Issue present in 74.23% (279,325) of emails tested.
> …
> (5) Serious: Links must have discernible text
> Issue present in 71.23% (268,049) of emails tested.
<https://emailmarkup.org/en/reports/accessibility/2026/>
Applies: accessibility, structure, links
Observer note: Date: last modified.

### E065 · Email Markup Consortium (Accessibility Report 2026) · study · published 2026-05-21 · fetched 2026-10-20 · via fetch
> (7) Serious: Element has insufficient color contrast
> Issue present in 58.48% (220,038) of emails tested.
> …
> Adjust either the text color or the background-color to ensure color contrast of at least 4.5:1.
> (8) Critical: Images must have alternate text
> Issue present in 47.88% (180,171) of emails tested.
> …
> An alt="" attribute is always needed, but in some cases can be left blank, when the image is purely for decoration and not adding content, meaning or function to the email.
<https://emailmarkup.org/en/reports/accessibility/2026/>
Applies: accessibility, images
Observer note: Date: last modified.

### E066 · Litmus (2026 Guide to Creating Accessible Emails) · guide · published 2026-03-24 · fetched 2026-10-20 · via fetch
> Anything smaller than 14 pixels on a desktop or laptop screen requires some effort to read.
> …
> Use media queries to increase the minimum font size from 14 to 16 pixels on smaller devices to help users read your email:
> …
> We recommend choosing a line height that’s 1.5 times more than your font size.
> …
> Text that is left-aligned has been proven to be easier to read for all.
> …
> To help screen readers understand the difference between <table> elements that hold content and those that are purely for design, use role=”presentation” on each table that holds content the subscriber needs to read. You only need to add it to each <table> element, not every <td>.
<https://www.litmus.com/blog/ultimate-guide-accessible-emails/>
Applies: accessibility, layout, mobile

### E067 · Litmus (The Ultimate Guide to Preview Text Support) · guide · published 2024-11-08 · fetched 2026-10-20 · via fetch
> While email clients typically pull preview text from the first lines of copy in your email, there are a few that will pull it from an image ALT text.
> …
> In the past, hiding content in your email was looked down upon and considered not good practice. The preheader text had to be visible in your email to not land in the spam folder. But as email has evolved, hidden content has become much more common and doesn’t hurt your email deliverability like it used to.
> …
> Character limit varies by client and device. As your subscriber can change preview text settings in their email client, they can see anywhere from 5 lines of preview text (about 278 characters) to 0 lines of preview text. …
> With no way to track how much preview text shows up, we recommend keeping the preview text under 90 characters.
<https://www.litmus.com/blog/the-ultimate-guide-to-preview-text-support/>
Applies: preheader, deliverability
Observer note: disagrees with Gmail's rule in E006 ("Don’t use HTML and CSS to hide content in your messages").

### E068 · Litmus (The Ultimate Guide to Dark Mode for Email Marketers) · guide · published 2025-02-27 · fetched 2026-10-20 · via fetch
> At the moment, there appear to be three fundamentally different types of color schemes that email clients use to apply Dark Mode to emails.
> …
> The first Dark Mode theme is what I like to call a “Partial Color Invert”. It only detects areas with light backgrounds and inverts them so the light backgrounds are dark, while the dark text becomes light.
> …
> The Full Color Invert is the most invasive color scheme: It not only inverts the areas with light backgrounds, but impacts dark backgrounds as well.
> So if you already designed your emails to have a dark theme, this scheme will ironically force them to become light. Unfortunately, this is currently the tactic used by some of the more popular email clients, such as Gmail app (iOS), Outlook 2021 (Windows), Office 365 (Windows), and Windows Mail.
<https://www.litmus.com/blog/the-ultimate-guide-to-dark-mode-for-email-marketers/>
Applies: darkmode
Observer note: the page's support chart is dated "as of February 2025"; it lists Apple Mail, Gmail Desktop, AOL and Yahoo mail under "No color changes", Outlook.com under partial invert.

### E069 · Can I email (@media (prefers-color-scheme)) · benchmark · published unknown · fetched 2026-10-20 · via fetch
> @media (prefers-color-scheme)
> This media query allows to theme for system light and dark mode.
> Estimated Support About
> 41.86% supported
<https://www.caniemail.com/features/css-at-media-prefers-color-scheme/>
Applies: darkmode
Observer note: per-client support grid is rendered as version/date cells without supported/unsupported labels in the extracted text, so only the aggregate is quoted.

### E070 · Litmus (Why You Shouldn’t Dismiss Plain Text Emails) · guide · published 2022-08-31 · fetched 2026-10-20 · via fetch
> Spam filters like to see a plain text alternative. HTML-only emails are a red flag for spam filters. A lazy spammer wouldn’t take time to create a plain text alternative (with web safe fonts), so make sure you do!
> …
> If an email client or app can only show the plain text version of an email, but no plain text version exists, most email clients will either show the raw HTML of the message or try to format it into plain text.
> …
> We also found that a number of companies are technically sending in multi-part MIME, but the plain text version is completely blank.
<https://www.litmus.com/blog/best-practices-for-plain-text-emails-a-look-at-why-theyre-important/>
Applies: plaintext, deliverability
Observer note: contrast with E071 (SpamAssassin default score for HTML-only is 0.1 of a 5-point threshold).

### E071 · Apache SpamAssassin (rules/20_body_tests.cf, rules/50_scores.cf, rules/10_default_prefs.cf, trunk) · standard · published unknown · fetched 2026-10-20 · via fetch
> describe MIME_HTML_ONLY		Message only has text/html MIME parts
> …
> score MIME_HTML_ONLY 0.1
> …
> # How many points before a mail is considered spam.
> required_score           5
<https://raw.githubusercontent.com/apache/spamassassin/trunk/rules/20_body_tests.cf>
Applies: plaintext, deliverability
Observer note: score line from <https://raw.githubusercontent.com/apache/spamassassin/trunk/rules/50_scores.cf>; threshold from <https://raw.githubusercontent.com/apache/spamassassin/trunk/rules/10_default_prefs.cf>. Kind "standard" used for a widely deployed filter's rule definitions. Date: undated (repository trunk).

### E072 · Apache SpamAssassin (rules/20_html_tests.cf and rules/50_scores.cf, trunk) · standard · published unknown · fetched 2026-10-20 · via fetch
> describe HTML_IMAGE_ONLY_04	HTML: images with 0-400 bytes of words
> describe HTML_IMAGE_ONLY_08	HTML: images with 400-800 bytes of words
> …
> describe HTML_FONT_LOW_CONTRAST	HTML font color similar or identical to background
> …
> score HTML_FONT_LOW_CONTRAST 0.713 0.001 0.786 0.001
> …
> score HTML_IMAGE_ONLY_04 1.680 0.342 1.799 1.172
> score HTML_IMAGE_ONLY_08 0.585 1.781 1.845 1.651
<https://raw.githubusercontent.com/apache/spamassassin/trunk/rules/20_html_tests.cf>
Applies: images, accessibility, deliverability
Observer note: score lines from <https://raw.githubusercontent.com/apache/spamassassin/trunk/rules/50_scores.cf>; the four columns are scores for the four network/Bayes rule sets. Date: undated (repository trunk).

### E073 · Apache SpamAssassin (rules/25_uribl.cf, rules/25_url_shortener.cf, rules/50_scores.cf, trunk) · standard · published unknown · fetched 2026-10-20 · via fetch
> describe        URIBL_DBL_SPAM   	Contains a spam URL listed in the Spamhaus DBL blocklist
> …
> describe        URIBL_BLACK     Contains an URL listed in the URIBL blacklist
> …
> score URIBL_BLACK 0 1.7 0 1.7 # n=0 n=2
> …
> score URIBL_DBL_SPAM 	0 2.5 0 2.5
> …
> describe URL_SHORTENER_CHAINED Message contains shortened URL chained to other shorteners
> …
> describe URL_SHORTENER_DISABLED Message contains shortened URL that has been disabled due to abuse
> …
> score URL_SHORTENER_DISABLED    2
<https://raw.githubusercontent.com/apache/spamassassin/trunk/rules/25_uribl.cf>
Applies: links, deliverability
Observer note: shortener lines from <https://raw.githubusercontent.com/apache/spamassassin/trunk/rules/25_url_shortener.cf> (242 url_shortener domain entries); scores from 50_scores.cf. Date: undated (repository trunk).

### E074 · Mailchimp (Gmail is clipping my email) · guide · published unknown · fetched 2026-10-20 · via fetch
> Gmail clips emails that have a message size larger than 102KB, and hides the full content behind a “View entire message” link. This interferes with the way that Mailchimp tracks opens.
> …
> When Gmail clips the message, it also clips the code that tracks opens. Keep emails smaller than 102KB so that content isn’t hidden and all email opens are tracked.
<https://mailchimp.com/en/help/gmail-is-clipping-my-email/>
Applies: length, measurement
Observer note: no Google page stating the 102KB figure was found; this is a vendor statement.

### E075 · Microsoft (Block or unblock automatic picture downloads in classic Outlook email messages) · standard · published unknown · fetched 2026-10-20 · via fetch
> Blocking pictures can help protect your computer. Microsoft Outlook is configured by default to block automatic picture downloads from the Internet.
> …
> Helping you avoid tracking pixels: invisible images that can tell a sender you've read the email. (Note: tracking pixels aren't the same as a read receipt.)
<https://support.microsoft.com/en-us/office/block-or-unblock-automatic-picture-downloads-in-email-messages-15e08854-6808-49b1-9a0a-50b81f2d617a>
Applies: images, measurement
Observer note: applies to Outlook for Microsoft 365, 2024, 2021, 2019, 2016 (classic Outlook).

### E076 · Google (Gmail Help: Turn images on or off in Gmail) · standard · published unknown · fetched 2026-10-20 · via fetch
> By default, when you get an email with an image, you’ll see the image automatically.
> …
> Note: If Gmail thinks a sender or message is suspicious, you won’t see images automatically. Instead, you'll be asked if you want to see the image.
> …
> Senders can’t use image loading to get information about your computer or location.
> Senders can’t use the image to set or read cookies in your browser.
<https://support.google.com/mail/answer/145919?hl=en>
Applies: images, measurement
Observer note: Gmail (images on by default) and classic Outlook (E075, blocked by default) differ.

### E077 · Apple (iPhone User Guide: Use Mail Privacy Protection on iPhone) · standard · published unknown · fetched 2026-10-20 · via fetch
> In the Mail app , turn on Mail Privacy Protection to make it harder for senders to learn about your Mail activity. Mail Privacy Protection hides your IP address so senders can’t link it to your other online activity or determine your exact location. It also prevents senders from seeing if you’ve opened the email message they sent you.
> Note: Mail Privacy Protection does not extend to links or email attachments such as PDF or other files.
<https://support.apple.com/guide/iphone/use-mail-privacy-protection-iphf084865c7/ios>
Applies: measurement

## Batch 2026-10-20 · from zero · queries: "email marketing benchmarks", "newsletter benchmarks open click unsubscribe", "best time to send email", "email frequency per week unsubscribe", "subject line length emoji personalization", "preview text length", "plain text vs HTML A/B", "dark mode email share", "Apple Mail Privacy Protection open rates", "bot clicks", "double opt-in vs single opt-in", "welcome email", "popup signup form conversion fields", "sunset policy inactive subscribers", "unsubscribe reasons", "Gmail clipping 102kb", "email accessibility font size", "Substack recommendations growth", "OpenAlex: personalization subject line field experiment", "arXiv: newsletter email" · unreachable: Kit/ConvertKit (blog and creator-economy report URLs 404), Klaviyo (benchmark and blog pages JS-only or 404), Campaign Monitor design and plain-text guides (404), Mailchimp live site (geo-redirects to German; used Wayback snapshots), Sumo (empty response), Semantic Scholar API (HTTP 429), SSRN (403), Mailgun unsubscribe-survey URL (404), Email on Acid width and image-ratio articles (403/empty), Mailchimp double opt-in research stats page (not archived)

### E078 · Mailchimp (Email Marketing Benchmarks and Statistics by Industry) · benchmark · published 2023-12 · fetched 2026-10-20 · via fetch
> Mailchimp sends billions of emails a month for millions of users. Needless to say, we track a lot of data. So we scanned billions of emails delivered by our system (where campaign tracking was activated, and where users reported their industry) and calculated the average unique open rates, click rates, soft bounce rate, and hard bounce rate by industry.
> We only tracked campaigns that went to at least 1,000 subscribers. Our customers range from 1-person startups, small businesses to Fortune 500 companies, so the whole spectrum is represented in this data. Here’s your apples-to-apples comparison of email marketing metrics with others in your industry.
> The data provided on this page was last updated in December 2023 and may vary from email marketing benchmarks data provided within the Mailchimp application.
> …
> Industry
> Avg. Open Rate
> Avg. Click Rate
> Unsubscription Rate
> …
> Non-Profits
> 40.04%
> 3.27%
> 0.18%
> …
> Ecommerce
> 29.81%
> 1.74%
> 0.19%
> All Users
> 35.63%
> 2.62%
> 0.22%
> …
> The accuracy of email open rates may be impacted by Apple's privacy changes and their Mail Privacy Protection (MPP) feature, and this should be considered as you interpret open rate data.
<https://mailchimp.com/resources/email-marketing-benchmarks/>
Applies: engagement, measurement, unsubscribe
Observer note: Live URL geo-redirects to a German translation; quoted from the Wayback Machine snapshot of 2026-10-15 (web.archive.org/web/20261015144427/…). Table cells appear one per line on the page. The FAQ on the same page gives different all-industry figures: aim for "around 34.23 percent" opens and an "optimal CTR" of "2.66 percent". Date: data as stated.

### E079 · Mailchimp (Email Marketing Benchmarks and Statistics by Industry (subject line advice)) · guide · published unknown · fetched 2026-10-20 · via fetch
> For many recipients, especially those reading your emails on mobile devices, shorter is often better. We recommend you use no more than 9 words and 60 characters.
> Our research shows that it’s best to use no more than 3 punctuation marks per subject line. Too many punctuation marks can make your email look like spam, especially if you use a lot of special characters.
> According to our research, you should use no more than 1 emoji at a time. Use emojis to supplement words rather than replace them, to make sure your main message gets across. Different operating systems render different versions of emojis, so it’s important to test.
<https://mailchimp.com/resources/email-marketing-benchmarks/>
Applies: subject, mobile
Observer note: Wayback snapshot 2026-10-15. No sample or method given for "our research". Date: undated (page data 2023-12).

### E080 · Mailchimp (What Is the Best Time to Send a Marketing Email Blast?) · benchmark · published unknown · fetched 2026-10-20 · via fetch
> You can send to most subscribers on a weekday and receive pretty good engagement. Sunday is the best day to send a newsletter or email blast to the fewest number of subscribers, while Monday through Friday are all quite similar in terms of the percentage of email addresses that have that day as optimal.
> It's somewhat established that the best days to send email blasts are Tuesday, Wednesday, and Thursday across different industries and audiences. When we look at the typical distribution of optimal send times across Mailchimp’s whole system, the best time to send email newsletters is at 10AM in the recipients’ own time zones. Note that no single day wins hands-down. This is what we should expect when studying the data of billions of humans’ inbox activity.
> Unless you have a good reason, I’d avoid sending your content too early (say, 3 or 4 am) in the recipients’ time zones.…
<https://mailchimp.com/resources/insights-from-mailchimps-send-time-optimization-system/>
Applies: timing
Observer note: Wayback snapshot 2025-12-14 (live URL geo-redirects). Based on the "Email Genome Project" per the page; no sample size stated.

### E081 · Mailchimp (Email Unsubscribe Rate (resource article)) · guide · published unknown · fetched 2026-10-20 · via fetch
> Understanding industry benchmarks for unsubscribe rates can help you evaluate your email strategies effectively. Different industries have varying average unsubscribe rates, typically ranging from 1% to 2%. If your rate is significantly higher, it may be time to reassess your email content and frequency.
> We've studied our customers of all types to provide you with email marketing benchmarks to help you understand the average unsubscribe rate for your industry. Many industries have average unsubscribe rates as high as 1 or 2%, so don't worry if your rates are slightly higher or lower than the average.
<https://mailchimp.com/resources/email-unsubscribe-rate/>
Applies: unsubscribe, measurement
Observer note: Wayback snapshot 2026-05-15. Contradicts Mailchimp's own benchmark table (0.15%–0.22% unsubscription rate) on the benchmarks page.

### E082 · Nielsen Norman Group (Marketing Email and Newsletter Design to Increase Conversion and Loyalty, 6th ed. (Flaherty, Schade, Nielsen) – method) · study · published unknown · fetched 2026-10-20 · via fetch
> This 537-page report presents 199 design guidelines based on 6 rounds of usability research. Discussions and 535 screenshot illustrations supplement our findings.
> The information in this report is based on 6 rounds of studies, conducted over 16 years. Usability research methods included:
> Representative users tested the usability of 500+ newsletters. The studies took place in the United States, Australia, England, Hong Kong, Japan, and Sweden.
<https://www.nngroup.com/reports/email-newsletter-design/>
Applies: measurement
Observer note: Free PDF linked from the report page; guidelines below quoted from its table of contents. Date: undated (6th edition).

### E083 · Nielsen Norman Group (Marketing Email and Newsletter Design report, 6th ed. – subscription and confirmation guidelines) · study · published unknown · fetched 2026-10-20 · via fetch
> 13. Make subscription via a small form on the homepage or other page of the site a one-step process. If it leads to a longer process, indicate that there are further steps and pre-populate the email address on any subsequent forms.
> 14. Don’t rely on pop-up windows to promote newsletters.
> 25. Explicitly state the newsletter frequency in the subscription process.
> 28. Require only users’ email addresses (and names, if necessary) during the newsletter subscription process. If you ask for anything else, ask for five or fewer additional pieces of information, and make them clearly optional.
> …
> 46. Set proper expectations by stating when the first newsletter will be sent.
> 51. Carefully consider if it’s worth asking users to confirm their subscriptions (double opt-in). If you must make users confirm their subscription, warn them about this during the sign-up process and on the confirmation screen.
> 54. Send new subscribers one welcome email confirming their successful subscription to any and all newsletters.
> 55. If possible and appropriate, include newsletter content in the welcome message. If not, tell users when to expect the first newsletter in the welcome message
<https://media.nngroup.com/media/reports/free/Marketing_Email_and_Newsletter_Design_to_Increase_Conversion_and_Loyalty_6th_Edition.pdf>
Applies: signup, consent, welcome, frequency
Observer note: Guideline text joined across PDF line wraps; numbering is the report's. Date: undated (6th edition).

### E084 · Nielsen Norman Group (Marketing Email and Newsletter Design report, 6th ed. – double opt-in and frequency findings) · study · published unknown · fetched 2026-10-20 · via fetch
> Some sites require users to complete a double opt-in process, meaning that after users submit their subscription information on the site, the site sends the user a confirmation message. The user must then click a link to confirm the subscription before the first newsletter will be delivered. This is commonly done to protect users from being signed up erroneously and to prevent sending newsletters to users who don’t want them. However, this process requires more action on the user’s part. Users might miss confirmation messages, forget to look for them, or fail to open them. If they have any problems with the confirmation step, users won’t receive the email newsletter.
> …
> It’s easier for users if they’re simply subscribed when they finish the subscription process on the site. Rather than sending a confirmation message, consider sending a welcome message.
> …
> Attitudes about frequency depend on the type of content your newsletter provides. When signing up, users are less concerned about the frequency of messages from ecommerce sites that consisted of marketing messages and promotional updates. Users assumed they would get messages daily or multiple times a week. They knew they could ignore these messages if they were too frequent or not relevant to them at the time. Frequency was more of a concern for users signing up for content-based newsletters such as news digests or industry newsletters. In these situations, users intended to spend time reading them, so knowing how frequently they would be delivered was more important to users.
<https://media.nngroup.com/media/reports/free/Marketing_Email_and_Newsletter_Design_to_Increase_Conversion_and_Loyalty_6th_Edition.pdf>
Applies: consent, signup, frequency
Observer note: Body text, pp. 23 and 192–193 of the PDF; line wraps joined. Date: undated (6th edition).

### E085 · Nielsen Norman Group (Marketing Email and Newsletter Design report, 6th ed. – sender, subject, preheader, content and layout guidelines) · study · published unknown · fetched 2026-10-20 · via fetch
> 64. Use short, informative, unambiguous sender information — no more than 20 characters — that shows the name of your company, site, or newsletter.
> 68. Front-load the subject line with key words and limit it to 40 characters.
> 71. Be cautious using recipient’s names in the subject line.
> 74. Ensure the preheader text supports the subject line and includes email content.
> …
> 125. Use the first line of text in the message, the preheader, to provide additional details or supplementary information about the newsletter content. Don’t repeat details provided in the sender information or subject line.
> 135. In headlines, use no more than 10 words and state the content’s key point
> 137. Limit your newsletter’s length.
> 138. Consider mobile users when creating content. Brief is better.
> 139. If the newsletter covers more than five topics or articles, provide brief, concise headlines followed by two- to four-sentence blurbs. Also, provide a link to further information on the website.
> 141. Include no more than two full-length articles in an email newsletter. Restrict their length with concise editing.
> …
> 149. Use a clickable (or tappable) area of at least 1cm x 1cm and leave generous amounts of space around it.
> 160. Consider using a single column layout, especially for mobile readers.
> 162. Left-align content.
> 163. Use font sizes that will remain large enough to be easily read on a variety of screen sizes.
> 165. Avoid dark background colors.
> 166. Limit file size to 100 kB. Keep file size as small as possible.
<https://media.nngroup.com/media/reports/free/Marketing_Email_and_Newsletter_Design_to_Increase_Conversion_and_Loyalty_6th_Edition.pdf>
Applies: subject, preheader, length, structure, layout, mobile, links, accessibility
Observer note: Date: undated (6th edition).

### E086 · Nielsen Norman Group (Marketing Email and Newsletter Design report, 6th ed. – unsubscribe, archive and reasons for leaving) · study · published unknown · fetched 2026-10-20 · via fetch
> 168. Include a link to subscribe in every email.
> 174. Provide archives of your newsletter on your site or give users a way to save and access newsletter content on your site.
> 175. Include a link to unsubscribe in every edition.
> 176. Place links to unsubscribe at the end of the email and ensure links stand out from fine print.
> 184. Don’t make users log in to unsubscribe.
> 186. Don’t ask users to enter information to complete the unsubscribe process.
> 187. If you ask for feedback from users about why they are unsubscribing, make this input secondary, optional and as simple as possible
> 193. Offer users an option to change the frequency of the newsletter as an alternative to unsubscribing
> …
> Participants occasionally made the effort to unsubscribe from newsletters. Of those who had unsubscribed, the most frequently cited reason was that they were no longer interested in the content. Others said the newsletter was unsolicited or delivered too frequently. Users also unsubscribed if the content was not as they expected, or they thought the information was useless.
<https://media.nngroup.com/media/reports/free/Marketing_Email_and_Newsletter_Design_to_Increase_Conversion_and_Loyalty_6th_Edition.pdf>
Applies: unsubscribe, retention, growth, frequency
Observer note: Date: undated (6th edition).

### E087 · Nielsen Norman Group (E-Mail Newsletters: Increasing Usability (Jakob Nielsen)) · study · published 2010-11-28 · fetched 2026-10-20 · via fetch
> So, in total, our design guidelines for newsletter user experience are now based on systematic studies of 270 newsletters across 124 participants, as well as single-user testing of several hundred additional newsletters that study participants already received in their personal inboxes.
> …
> …the number of new or unread messages is now 300% higher than it was just 4 years ago.
> …
> In addition to subject lines, users now pay more attention to message previews. This change is partly driven by the increasing email volume (users can decide whether to dispose of or keep messages without opening them all) and partly driven by more mobile access (users can't see much on a small screen).
> …
> In our latest study, we asked users to "receive updates" from companies. Only 10% elected to do so through Facebook, while 90% opted for a newsletter.
> …
> One big problem in the latest study was that many newsletters were poorly formatted for reading on small mobile screens. Indeed, people rated the ease of reading newsletters on their mobile devices a miserable 3.3 on a 1–7 scale.
<https://www.nngroup.com/articles/e-mail-newsletters-usability/>
Applies: preheader, mobile, engagement
Observer note: Old (2010); four study rounds with 124 participants as stated on the page.

### E088 · Nielsen Norman Group (Email Subject Lines: 5 Tips to Attract Readers (Janelle Estes)) · guide · published 2014-05-04 · fetched 2026-10-20 · via fetch
> Summary: Focus on the first 40 characters. Descriptive and well-written subject lines allow recipients to make an informed decision to get more details or move on.
> 2. Front-load the subject line with keywords and limit it to 40 characters.
> …Although we recommend displaying no more than 40 characters in the subject line, many email programs display far fewer than that.
<https://www.nngroup.com/articles/email-subject-lines/>
Applies: subject, mobile

### E089 · Nielsen Norman Group (Emojis in Email Subject Lines: Advantage or Impediment? (Kim Flaherty)) · study · published 2020-06-28 · fetched 2026-10-20 · via fetch
> Summary: Our research shows that emojis in subject lines increase negative sentiment toward an email and do not increase the likelihood of an email being opened.
> …
> …Adding an emoji to an email subject line increases the negative sentiment towards that email by 26%.…
> …
> …(On average, each emoji email was selected .04% of the time, whereas, for the corresponding no-emoji email, that number was .03%; p >0.1).…
<https://www.nngroup.com/articles/emojis-email/>
Applies: subject, engagement
Observer note: Method on page: 13 real retail marketing emails with emoji subject lines vs. matched no-emoji versions; reaction-word test and an inbox selection task.

### E090 · Nielsen Norman Group (Animated GIFs in Email Are Worse Than Static Emails (Kim Flaherty)) · study · published 2020-03-15 · fetched 2026-10-20 · via fetch
> To explore this question, we conducted two online surveys. We showed 14 different marketing emails with a variety of animated GIFs to a sample of 55 respondents. We also created a corresponding static version of each of these emails using a single frame from each. These static variations were shown to a different set of 66 respondents.
> When we asked people to select three words from a list of positive and negative terms, respondents selected more positive reaction words and fewer negative reaction words for the emails with static images compared to those that had animated images. On average, an email with animation received 1.7 negative words and 1.3 positive words, whereas an email without animation received 1.2 negative words and 1.8 positive words. Both these differences were statistically significant at p < 0.005.
> In other words, adding animation to an email increased its negative word count by 40% and decreased its positive word count by 30%.
<https://www.nngroup.com/articles/gif-emails/>
Applies: images, engagement

### E091 · Litmus (Email Client Market Share (monthly leaderboard)) · benchmark · published 2026-10-01 · fetched 2026-10-20 · via fetch
> Calculated from over 1.5 billion opens in Litmus Email Analytics, in September 2026.
> Apple
> …
> 76.56%
> Gmail
> …
> 16.10%
> …
> 2. Impact of Mail Privacy Protection (MPP): Studies show that Apple’s MPP now impact roughly 55-60% of all email opens. shift from tracking “open rates” to more reliable engagement metrics like click-through rates and conversion data.
> 3. Rise of Mobile and Dark Mode: Mobile devices continue to be the primary reading environment for the majority of subscribers, while Dark Mode adoption remains a critical design consideration, now used by over 25% of the total user base.
<https://www.litmus.com/email-client-market-share>
Applies: measurement, mobile, darkmode
Observer note: The Apple row explicitly includes MPP opens, which "are not considered reliable opens". The MPP and Dark Mode shares in the takeaways cite unnamed "studies". Date: data for September 2026.

### E092 · Litmus (Ultimate Guide to Dark Mode (originally by Alice Li)) · guide · published 2025-02-27 · fetched 2026-10-20 · via fetch
> Litmus’ Email Client Market Share indicates that of the opens tracked, an average of 35% used Dark Mode in 2022, representing steady adoption year over year.
> …
> The first Dark Mode theme is what I like to call a “Partial Color Invert”. It only detects areas with light backgrounds and inverts them so the light backgrounds are dark, while the dark text becomes light.
> The Full Color Invert is the most invasive color scheme: It not only inverts the areas with light backgrounds, but impacts dark backgrounds as well.
<https://www.litmus.com/blog/the-ultimate-guide-to-dark-mode-for-email-marketers>
Applies: darkmode
Observer note: Litmus' own market share page (Sept 2026 data) gives "over 25%" for Dark Mode, lower than the 35% for 2022 quoted here. Date: first published 2019-11.

### E093 · Email on Acid (Master the Art of Dark Mode Email Design and Coding) · guide · published 2025-10-02 · fetched 2026-10-20 · via fetch
> In Apple Mail’s dark mode, it tends to invert pure black (#000000) and pure white hex (#FFFFFF) codes. Apple Mail will completely disregard contrast or other-such rules if you have those hex codes and invert them regardless.
> We advise to choose slightly different hex codes, such as #000001 or #FFFFFE, the difference in color is not noticeable and it’ll help you avoid any surprises.
> Avoid pure black backgrounds: Very dark greys are often easier on the eyes than pure black.
<https://www.emailonacid.com/blog/article/email-development/dark-mode-for-email/>
Applies: darkmode, accessibility

### E094 · Validity (What’s Really Behind Gmail’s Open Rate Drop — And What to Do About It (Guy Hanson)) · guide · published 2026-05-15 · fetched 2026-10-20 · via fetch
> Email senders are reporting major drops in Gmail open rates over the past few months—some Validity customers are seeing quarter-over-quarter drops of 30 percent or more.
> Validity’s own engagement data confirms it: Gmail image loading activity—including tracking pixels—dropped by roughly one third in late November 2025.
> …
> These false opens are triggered if the emails are delivered successfully, regardless of whether users opened them or not.
> …
> However, open rates remain directionally useful, and senders shouldn’t suppress Gmail addresses based on open inactivity alone. Instead, continue to treat open rates as a health signal, where a sudden drop may provide early warning of a deeper underlying problem.
<https://www.validity.com/blog/whats-really-behind-gmails-open-rate-drop-and-what-to-do-about-it/>
Applies: measurement, engagement, hygiene

### E095 · Validity (Email open rates are actually up to three times lower than reported (Nicky Copland)) · guide · published 2020-01-30 · fetched 2026-10-20 · via fetch
> According to Yahoo (including AOL and Yahoo Mail), open rates provided by email service providers (ESPs) are dramatically inflated.
> ESPs report data up to three times greater than Verizon's own data.…
> “Since we control the email client, we can measure when emails are truly opened,” said Marcel Becker, director of product management at Yahoo. “When we look at that data, we see the open rate numbers on our side not matching the numbers typically reported by open tracking pixels.”
<https://www.validity.com/blog/email-open-rates-are-actually-up-to-three-times-lower-than-reported/>
Applies: measurement
Observer note: Reports a statement by Yahoo at a Validity event; page shows a meme caption run into the sentence, cut here.

### E096 · Validity (Are These Clicks for Real? The Rise of Bot Clicks (Megan Farquharson)) · guide · published 2026-01-28 · fetched 2026-10-20 · via fetch
> Instant clicks, clicks on all links, and a lack of conversions are key signs that bots may be interacting with your emails.
> Unchecked, bot clicks can wreak havoc across your marketing efforts. They can ruin A/B tests, distort performance reporting, falsely inflate unsubscribe rates, and lead teams to make poor decisions based on misleading data.
> …
> Instant clicks: If the click occurs within a second of delivery, they’re likely automated.
<https://www.validity.com/blog/are-these-clicks-for-real-the-rise-of-bot-clicks-and-what-marketers-can-do-about-it/>
Applies: measurement, engagement
Observer note: Counterpoint to guides that treat clicks as the reliable post-MPP signal.

### E097 · arXiv (Kong, Zhu, Konstan) (Learning to Ignore: A Case Study of Organization-Wide Bulk Email Effectiveness) · study · published 2020-06-30 · fetched 2026-10-20 · via fetch
> Third, this disagreement was amplified by the success metric used by communicators. They viewed their bulk emails as successful if they had a high open rate. But recipients often opened and then rapidly discarded emails without reading the details.
<https://arxiv.org/abs/2006.16508>
Applies: measurement, engagement
Observer note: Qualitative study at a large university (communicators, recipients, managers); abstract via arXiv API.

### E098 · MailerLite (Email marketing benchmarks by industry and region for 2026) · benchmark · published 2025-12-03 · fetched 2026-10-20 · via fetch
> Includes over 3.6 million campaigns sent from 181,000 approved accounts
> Covers a full year of data, from December 2024 to November 2025
> Shows the median value for all benchmarks
> …
> The average email open rate in 2025 was 43.46%. This was a slight increase on 2024’s average open rate of 42.35%.
> The average email click rate in 2025 was 2.09%. This was a slight increase on 2024’s average email click rate of 2%.
> The average unsubscribe rate in 2025 was 0.22%. This was over double 2024’s unsubscribe rate of 0.08%.
> …The latter is likely due to the Gmail changes that make it easier for people to unsubscribe from email lists in a click without ever opening the email.
<https://www.mailerlite.com/blog/compare-your-email-performance-metrics-industry-benchmarks>
Applies: engagement, unsubscribe, measurement
Observer note: Method says medians, the summary lines say "average"; the 43.46% figure is also called "the median" further down the page.

### E099 · MailerLite (Email marketing benchmarks 2026 (industry spread and measurement notes)) · benchmark · published 2025-12-03 · fetched 2026-10-20 · via fetch
> The benchmark open rate for authors is 43.14% and the click rate is 2.75%
> The benchmark open rate for e-commerce is 32.67% and the click rate is 1.07%
> The benchmark open rate for non-profits is 52.38% and the click rate is 2.90%
> …
> Apple Mail Privacy Protection automatically marks emails sent to Apple Mail clients as opened. This means real open rates are lower than the metrics you see in your dashboard and in this article. Your actual click-to-open rate will also be higher due to the inflated number of opens recorded.
> …
> Considering how Apple’s Privacy changes impact open rate and CTOR, click rate is currently the most accurate indicator of email newsletter engagement since it’s not reliant on tracking opens.
<https://www.mailerlite.com/blog/compare-your-email-performance-metrics-industry-benchmarks>
Applies: engagement, measurement

### E100 · Brevo (Email Marketing Benchmarks 2026: Region & Industry Data) · benchmark · published 2026-05-27 · fetched 2026-10-20 · via fetch
> The benchmark is built on aggregated, anonymized messaging data from over 175,000 active Brevo customers across 2025. We calculated unweighted averages and eliminated extreme outliers to give you a realistic, baseline view of typical performance. By analyzing active senders across all channels, this report reflects how modern marketing pressure is being distributed today.
> …
> Metric
> Average
> Top 10% Performers
> Open rate
> 20.73%
> 44.02%
> Open rate (incl. Apple MPP)
> 33.87%
> 59.89%
> Click-through rate
> 2.27%
> 5.22%
> Unsubscribe rate
> 0.46%
> 0.05%
> Note: Automation/Transactional emails average open rate is 30.63% and click-through rate is 7.39%.
<https://www.brevo.com/blog/email-marketing-benchmarks/>
Applies: engagement, measurement, unsubscribe
Observer note: One of few benchmarks reporting open rates both with and without MPP-triggered opens.

### E101 · Brevo (Email Marketing Benchmarks 2026 (region and weekday)) · benchmark · published 2026-05-27 · fetched 2026-10-20 · via fetch
> Region
> Open rate
> Open rate (incl. Apple MPP)
> Click-through rate
> Unsubscribe rate
> Europe
> 22.83%
> 36.90%
> 2.29%
> 0.51%
> North America (NORAM)
> 17.32%
> 32.49%
> 1.78%
> 0.46%
> …
> Across most industries, Monday and Tuesday consistently show the highest open and click-through rates, while Saturday and Sunday see the lowest engagement. For a deeper analysis including optimal send hours by industry, see our dedicated guide: Best Time to Send an Email.
<https://www.brevo.com/blog/email-marketing-benchmarks/>
Applies: timing, engagement

### E102 · Campaign Monitor (Ultimate Email Marketing Benchmarks for 2022: By Industry and Day) · benchmark · published 2022 · fetched 2026-10-20 · via fetch
> …For this report, we analyzed over 100 billion emails sent across our platforms in 2021 to give you the most comprehensive benchmark data and email marketing stats to date.
> …
> Average Totals
> 21.5% (+3.5%)
> 2.3% (-0.3%)
> 10.5% (-3.6%)
> 0.1% (-)
> …
> MPP effectively disables open rate tracking by pre-fetching (or downloading) an email and its images (including each email’s tracking pixels). By doing this, MPP is marking an email as opened, artificially inflating open rates for each user that has opted into the new feature. For a deep dive on MPP, be sure to check out our guide.
<https://www.campaignmonitor.com/resources/guides/email-marketing-benchmarks/>
Applies: engagement, measurement, unsubscribe
Observer note: Columns in order: open rate, click-through rate, click-to-open rate, unsubscribe rate (2021 data vs 2020). Date: modified 2023-01-31.

### E103 · GetResponse (Email Marketing Benchmarks 2024) · benchmark · published 2024 · fetched 2026-10-20 · via fetch
> We analyzed more than 4.4 billion messages sent by GetResponse customers in 2023.
> To keep things accurate, we focused on active senders with at least 500 contacts.
> …
> The average email open rate is 39.64%
> The average email click-through rate is 3.25%
> The average email unsubscribe rate is 0.15%
> …
> For example, the average open rate for newsletters is 40.08%, while for triggered emails, it’s 45.38%.
> …This year, welcome emails saw an average open rate of 83.63%.
<https://www.getresponse.com/resources/reports/email-marketing-benchmarks>
Applies: engagement, welcome, measurement
Observer note: Date: 2023 data.

### E104 · GetResponse (Email Marketing Benchmarks 2024 – number of newsletters per week) · benchmark · published 2024 · fetched 2026-10-20 · via fetch
> Number of newsletters per week
> Open rate
> Click-through rate
> Click to open rate
> Unsubscribe rate
> Complaint Rate
> Bounce rate
> % of marketers
> 1
> 48.31%
> 5.71%
> 11.82%
> 0.25%
> 0.01%
> 2.85%
> 46.77%
> 2
> 43.2%
> 4.73%
> 10.95%
> 0.18%
> 0.01%
> 2.82%
> 19.54%
> …
> 7
> 41.12%
> 2.87%
> 6.99%
> 0.09%
> 0.01%
> 2.45%
> 2.7%
> …
> It is safe to say that the optimal frequency is 2 newsletters per week if you seek to maintain high open and click-through rates.
> However, the data shows that higher frequencies (more than 6 newsletters per week) tend to have lower unsubscribe rates, indicating higher content tolerance among engaged subscribers.
<https://www.getresponse.com/resources/reports/email-marketing-benchmarks>
Applies: frequency, unsubscribe, engagement
Observer note: Cross-sectional: senders who mail more often are different senders, not the same list at a higher cadence. The same section also says "open rates reaching 4.3%", which does not match its table. Date: 2023 data.

### E105 · GetResponse (Email Marketing Benchmarks 2024 – time, day and engagement decay) · benchmark · published 2024 · fetched 2026-10-20 · via fetch
> Also once again we saw that there are two time slots you may want to consider. One early in the morning (4-6 AM) and the second one later in the afternoon (5-7 PM).
> …
> So what did we learn? There’s little difference in terms of engagement between individual weekdays. While the average open and click-through rates are the highest on Tuesdays, they aren’t that far different from what we see on the other days.
> …
> The first hour is when most recipients open and interact with your email, capturing 21.20% of opens and 44.14% of clicks.…
> Also, most email engagement happens within the first day, with about 70% of opens and 85% of clicks occurring within the first 24 hours. However, a significant portion of engagement still happens after 26 hours, which means that recipients engage with emails much later.
<https://www.getresponse.com/resources/reports/email-marketing-benchmarks>
Applies: timing, measurement
Observer note: Date: 2023 data.

### E106 · GetResponse (Email Marketing Benchmarks 2024 – subject line length and emojis) · benchmark · published 2024 · fetched 2026-10-20 · via fetch
> Looking at our data, we can see that indeed it’s the shorter subject lines (below 70 characters) that get the highest open rates. They are also the most popular ones, meaning that the best practice has reached the masses.
> …
> Emoji
> Open rate
> Click-through rate
> Click-to-open rate
> Unsubscribe rate
> Spam rate
> Bounce rate
> % of messages
> No
> 42.23%
> 4.16%
> 9.85%
> 0.14%
> 0.01%
> 2.41%
> 74.98%
> Yes
> 37.5%
> 3.32%
> 8.85%
> 0.16%
> 0.01%
> 2.41%
> 25.02%
<https://www.getresponse.com/resources/reports/email-marketing-benchmarks>
Applies: subject
Observer note: Date: 2023 data.

### E107 · GetResponse (Email Marketing Benchmarks 2024 – personalization in subject lines) · benchmark · published 2024 · fetched 2026-10-20 · via fetch
> The trend has not changed since last year, and most marketers (90.33%) still don’t pursue personalization in their subject lines. The open rate of such emails is higher than that of those who go the extra mile and choose to personalize their subject lines (41.87% against 35.78%).
> Similarly, non-personalized emails' click-through rate (CTR) is almost double that of personalized ones (4.23% against 2.11%). Even the click-to-open rate shows a higher engagement for non-personalized emails (10.09% against 5.90%).
<https://www.getresponse.com/resources/reports/email-marketing-benchmarks>
Applies: personalization, subject
Observer note: Observational comparison across senders; contrasts with the Sahni et al. field experiment below. Date: 2023 data.

### E108 · GetResponse (Email Marketing Benchmarks 2024 – opt-in method and autoresponder length) · benchmark · published 2024 · fetched 2026-10-20 · via fetch
> Industry
> Double opt-in %
> Single opt-in %
> …
> Publishing
> 34.75%
> 65.25%
> …
> All
> 10.99%
> 89.01%
> When deciding which opt-in method to use, most businesses find single opt-in to be more effective due to its simplicity and higher subscription rates. However, the best choice ultimately depends on your business goals and the importance you place on the quality of your email list.
> …
> Shorter autoresponder cycles with 1 to 3 messages show the highest open and click-through rates. This messaging frequency seems the norm, with the majority (57.9%) of cycles.
<https://www.getresponse.com/resources/reports/email-marketing-benchmarks>
Applies: consent, signup, welcome
Observer note: Share of GetResponse accounts using each method; Sinch Mailgun's sender survey (below) reports 39.7% using double opt-in. Date: 2023 data.

### E109 · Omnisend (Email marketing statistics (citing Omnisend’s 2026 ecommerce marketing statistics report)) · benchmark · published 2026-01-30 · fetched 2026-10-20 · via fetch
> Open rates rose for the fifth consecutive year, increasing from 26.6% in 2024 to 30.7% in 2025, even as click rates declined
> Automations delivered outsized results. They accounted for just 2% of email sends, but drove 30% of revenue, earning 16× more per send than scheduled campaigns
> Globally, email campaign open rates averaged 30.7%, while automated emails reached 38% and generated $2.87 per email compared to $0.18 for campaigns
<https://www.omnisend.com/blog/email-marketing-statistics/>
Applies: engagement, welcome
Observer note: Ecommerce senders only. Date: modified 2026-08-18.

### E110 · Omnisend (Welcome email subject lines) · guide · published unknown · fetched 2026-10-20 · via fetch
> Together with abandoned carts, welcome emails drive 76% of all automation-generated orders. And you guessed it right: none of that would happen without a catchy and relevant subject line to get your subscribers to open the email.
> Deliver the goods immediately: If they signed up for a 15% off, tell them the 15% off is inside. “Your 15% off is waiting” beats “Welcome to the club” every time because it finishes the transaction they just started.
<https://www.omnisend.com/blog/welcome-email/>
Applies: welcome, subject

### E111 · Omnisend (Email popup statistics (Vytautas Palubeckas)) · benchmark · published 2026-02-09 · fetched 2026-10-20 · via fetch
> Total displays: 1.24B (+54% YoY from 803M)
> Average conversion rate: 2.1% (down slightly from 2.3%)
> …
> …Popups shown immediately (0–1 seconds) converted at 1.9%, while giving visitors a brief moment to settle improved performance. The best results came from a 6–10 second delay, which reached 2.4% on average.
> …
> 1 field: 2.1%
> 2 fields: 2.2%
> 3 fields: 2.1%
> 4 fields: 1.5%
> 5+ fields: 1.4%
> …
> Mobile optimization is essential, as popups designed for mobile devices convert at 2.2%, compared to just 1.4% for desktop-only popups, highlighting the importance of reaching modern shoppers effectively.
<https://www.omnisend.com/blog/email-popup-statistics/>
Applies: signup, growth, mobile
Observer note: Ecommerce popups. Chart captions on the same page disagree with the text (e.g. multi-step 2.1% in the caption vs 2.3% in the text; 2 fields 1.9% vs 2.2%). The page also cites other vendors at 3.49%–11.09%. NN/g guideline 14 advises not relying on pop-ups. Date: page also shows 2026-09-10.

### E112 · beehiiv (The State of Newsletters 2026 (Tyler Denk, Kanishka, Taylor Cromwell)) · benchmark · published 2026-01-05 · fetched 2026-10-20 · via fetch
> Publishers sent 28 billion emails last year and reached more than 255 million unique readers.
> …
> 2.  The open rates increased from 37.98% in 2024 to 41.24% in 2025, a sign that audiences continue to engage consistently.
> 3.  CTR is down Y/Y - largely because of inbox competition. It underscores that the bar for compelling calls-to-action or engaging links is rising. It also reflects emerging behavior: readers often consume the content in the newsletter itself (especially if it’s high-quality) rather than clicking out.
> 4.  An extremely low spam complaint rate (0.02%) shows that when people subscribe to a beehiiv newsletter, they generally want to be there.…
> …
> …Industry-wide, we’ve seen a shift toward measuring “true” engagement metrics, like verified clicks, replies, and post-email actions, to gauge audience interest.
<https://www.beehiiv.com/blog/the-state-of-newsletters-2026>
Applies: engagement, measurement
Observer note: Quoted from the page's Markdown version (same URL + .md), bold markers removed. An image caption on the same page gives the 2024 open rate as 37.67% and CTR 4.74% → 3.23%.

### E113 · beehiiv (The State of Newsletters 2026 – timing and cadence) · benchmark · published 2026-01-05 · fetched 2026-10-20 · via fetch
> 1.  Early morning (5–6am EST) remains the top-performing window of the day, reaching 44.46% at 5am and 42.12% at 6am — the highest open rates of the entire 24-hour cycle.
> Open rates fall sharply between 9pm–2am EST, hitting a low of 28.61% around 10pm EST.
> Monday leads with a 40.45% open rate, closely followed by:
> Wednesday: 40.31%
> Friday: 40.23%
> Sunday: 40.15%
> Overall, Monday through Friday are very stable, hovering in the 40% range, with only a mild dip on Saturday (39.41%).
> …
> Daily: 1.37%
> Weekly: 46.67% (dominant)
> Monthly: 27.15%
> …
> The most important part is to set expectations with readers and deliver on the schedule you promise, whether that’s daily or monthly.
<https://www.beehiiv.com/blog/the-state-of-newsletters-2026>
Applies: timing, frequency
Observer note: A chart caption on the same page places the late-morning peak at "10–11am UTC".

### E114 · beehiiv (The State of Newsletters 2026 – subject line length) · benchmark · published 2026-01-05 · fetched 2026-10-20 · via fetch
> 41.56% open rate for 0–20 word subjects
> Drops sharply to 34.03%, 26.74%, and 28.62% as subject lines lengthen into 20–80 words.
> 7.78% CTR for 80–100 word subject lines, more than 2.5× the global newsletter CTR.
<https://www.beehiiv.com/blog/the-state-of-newsletters-2026>
Applies: subject
Observer note: Bucket labels say "word"; 80–100-word subject lines are implausible, so the unit may be characters. Recorded as published.

### E115 · beehiiv (The State of Newsletters 2026 – Boosts and Recommendations) · benchmark · published 2026-01-05 · fetched 2026-10-20 · via fetch
> While boosts account for 5% of total earnings, they are central to list growth: the average newsletter using Boosts gained 84 new subscribers per month directly from the feature.
> As The Pour Over remarked, “Our highest quality growth channels were referrals and other newsletters. They’re just harder to scale.”
> …
> The Recommendation Network now includes 30,000+ active publishers and has generated millions of new subscribers.
> Newsletters that participate grow 2.75x faster, and those that recommend others are 32x more likely to be recommended back.
<https://www.beehiiv.com/blog/the-state-of-newsletters-2026>
Applies: growth
Observer note: Platform-reported, no method given. Markdown link syntax reduced to its link text.

### E116 · Substack (Upgrading Substack’s recommendation network (On Substack)) · benchmark · published 2024-02-22 · fetched 2026-10-20 · via fetch
> …Today, Recommendations, along with the Substack app, powers a network that drives 50% of all new subscriptions and 25% of new paid subscriptions on Substack.…
<https://on.substack.com/p/substacks-recommendations-network>
Applies: growth

### E117 · Substack (How to welcome your new readers (On Substack resources)) · guide · published 2023-03-06 · fetched 2026-10-20 · via fetch
> But this overlooked space can be the difference between instant churn (unsubscribe or inactive reading) and upsell from free to paying subscribers. The welcome email can bring your reader deeper into your publication from the get-go and deliver on the promise of your welcome page and your About page—both instrumental in converting browsing readers to signups.
> …
> Our team advises including an upgrade button “above the fold” of the email (before the reader needs to scroll), along with details of the paid benefits laying out the differences for free and paid subscribers.
> …
> Some writers ask their readers to reply to the welcome email with why they signed up, or invite them to a Chat or introductory post to share more about where they are in the world and why they subscribed.
> …Kevin told us: “Someone recently asked me how many people reply. And after the ParentData shoutout earlier in the month, I saw about 1,000 new followers and 25 emails. So around 2.5% of folks reply.”
<https://on.substack.com/p/writing-a-good-welcome-email-on-substack>
Applies: welcome, engagement
Observer note: The 2.5% reply rate is one writer's anecdote (about 1,000 new followers, 25 emails).

### E118 · Substack (How can I optimize my Substack publication for SEO? (Help Center)) · guide · published 2026-09-24 · fetched 2026-10-20 · via fetch
> At Substack, we do a number of things behind the scenes to ensure that all publications are indexed and ranked well in search engines. Some SEO features, such as sitemaps and Google Search Console verification, become available as your publication grows; publishing regularly, growing your subscriber list, and enabling paid subscriptions all help unlock them.
> …
> The best way to improve your SEO ranking is to get more inbound links to your Substack publication.
<https://support.substack.com/hc/en-us/articles/4407702258836-How-can-I-optimize-my-Substack-publication-for-SEO>
Applies: growth
Observer note: Fetched via the Zendesk Help Center JSON API (article body HTML). Date: created 2021-09-28.

### E119 · HubSpot (Plain Text vs. HTML Emails: Which Is Better? [New Data] (Niti Shah)) · study · published 2025-06-13 · fetched 2026-10-20 · via fetch
> In a 2014 survey, we asked over a thousand professionals whether they preferred HTML-based or text-based emails, and whether they preferred emails that consisted of mostly text or mostly images. Nearly 2/3 of the respondents said they preferred HTML and image-based emails.
> …
> In every single A/B test, the simpler-designed email won. The emails with fewer HTML elements won with statistical significance.
> …
> For the plain-text vs. HTML template with images test, the HTML email version had a 21% lower clickthrough rate, and combined with the open rate the email had 51% fewer clicks.
> For the simple HTML template vs. HTML-heavy template, the simpler email had a 5.3% higher clickthrough rate, and combined with the higher open rate, resulted in getting 30% more clicks.
<https://blog.hubspot.com/marketing/plain-text-vs-html-emails-data>
Applies: plaintext, images, engagement
Observer note: HubSpot's own sends; number of tests given only as "at least 10 more"; original test date not shown on the page. Date: updated.

### E120 · Litmus (The Results Are In: A/B Testing HTML vs. Plain Text Emails) · study · published 2020-10-23 · fetched 2026-10-20 · via fetch
> Our customers engaged a little bit better with our plain text style email, indicated by a slightly higher open and click-through rate. So we dug a little deeper to understand if the elevated email engagement was significant. And it was! Out of all the customers we emailed, 60% converted from the plain text style version.
> …
> …This seemed to confirm our hypothesis that for this segment, the email format doesn’t matter.
<https://www.litmus.com/blog/the-results-are-in-a-b-testing-html-vs-plain-text-emails>
Applies: plaintext
Observer note: Single sender (Litmus webinar invites), two tests.

### E121 · Litmus (Experts Say: Why You Shouldn’t Send Image-Only Emails (Whitney Rudeseal Peet)) · guide · published 2021-08-26 · fetched 2026-10-20 · via fetch
> Blocked images can result in emails that do not communicate anything immediately and fail to make an impact—or even appear to be broken. It can be worse if you don’t have any ALT text set up for your imagery because then, the email simply appears blank.
> …
> This may have been the case several years ago, but spam filters have since evolved and now pay closer attention to mailstream behavior, links in the message, and user interaction rather than doing naive word matching through supposed spam trigger words.
> So it’s not the images themselves that can hurt your deliverability—but the lower engagement you may see from having less accessible emails that people can’t interact with.
<https://www.litmus.com/blog/why-you-shouldnt-send-image-only-emails>
Applies: images, accessibility, deliverability
Observer note: Directly addresses the "image-to-text ratio" spam-filter folklore. Date: first published 2019-03-05.

### E122 · Litmus (A Strategic Guide to Calls-to-Action (CTAs) in Email Marketing) · guide · published 2024-03-18 · fetched 2026-10-20 · via fetch
> Mobile is the top way to read email, with 44.7% of opens taking place on a mobile device or client. As mobile continues to gain popularity, the physical size of CTAs are more important than ever. While clicking a link with your mouse on a desktop provides very precise control, touching a CTA with your thumb can become frustrating when targets are too small or cramped too close together on mobile devices.
> Generally, you’ll want to keep CTAs big enough for even large thumbs to easily tap. Apple suggests making touch targets at least 44×44 px. We recommend that as a great starting point when designing any CTA.
> …
> By styling your primary and secondary CTAs differently, it prevents them from competing with each other.…
<https://www.litmus.com/blog/click-tap-and-touch-a-guide-to-cta-best-practices>
Applies: links, mobile, layout
Observer note: NN/g guideline 149 gives "at least 1cm x 1cm". Litmus 2015 reported 53% mobile opens (adjusted) for 2014.

### E123 · Litmus (2026 Guide to Creating Accessible Emails) · guide · published 2026-03-24 · fetched 2026-10-20 · via fetch
> Text can appear smaller on mobile devices, forcing users to work harder to read your email. Use media queries to increase the minimum font size from 14 to 16 pixels on smaller devices to help users read your email:
> …
> …Set an appropriate line height on text to make it easier to read for all. We recommend choosing a line height that’s 1.5 times more than your font size.…
> …
> For most businesses, your sweet spot is somewhere between 60 and 70 to capture a general audience. Of course, if your audience is highly educated, then don’t be afraid to use more complex language.
<https://www.litmus.com/blog/ultimate-guide-accessible-emails>
Applies: accessibility, mobile, layout, tone
Observer note: The page's own code sample right after the 1.5× advice uses font-size 14px with line-height 18px (about 1.29×). The 60–70 refers to a Flesch reading-ease score.

### E124 · Litmus (How to Use Email Typography That Speaks Volumes) · guide · published 2023-11-16 · fetched 2026-10-20 · via fetch
> Use at least a 14px font size. There isn’t really an upper limit to how big you style your text, but there is a limit to how small text should be. Anything below 14px becomes hard for many people to read, and iOS Apple Mail’s automatic text adjustment feature will kick in and enlarge your text.…
> …At Litmus, we rarely stray below 18px.…
> …
> …Center-aligned text should be reserved for headlines or short paragraphs of no more than 3 lines. Use left-aligned for left-to-right languages to create a visual anchor point to return to each time a line is read.
<https://www.litmus.com/blog/email-typography-fonts>
Applies: layout, accessibility, mobile

### E125 · Litmus (How to Keep Gmail from Clipping Your Emails) · guide · published 2024-02-29 · fetched 2026-10-20 · via fetch
> If you’re not already familiar, Gmail clips any message with a file size of over 102 kilobytes (kb). Once the email reaches that limit, the rest of the content is hidden, with “[Message clipped]” text and a “View entire message” link displayed leading to a webview version of your email.
> It’s also worth noting that the 102kb limit does not include images, only the data size of the code itself.…
<https://www.litmus.com/blog/how-to-keep-gmail-from-clipping-your-emails>
Applies: length, layout
Observer note: NN/g guideline 166 says "Limit file size to 100 kB."

### E126 · Litmus (The Ultimate Guide to Email Preview Text) · guide · published 2024-11-08 · fetched 2026-10-20 · via fetch
> Character limit varies by client and device. As your subscriber can change preview text settings in their email client, they can see anywhere from 5 lines of preview text (about 278 characters) to 0 lines of preview text. Even Gmail allows users to turn off snippets (friendly reminder, that’s what they call preview text).
> With no way to track how much preview text shows up, we recommend keeping the preview text under 90 characters. Keep in mind that sometimes shorter or no preview text may be just the thing to make your email stand out in the inbox.
> …
> …Autoplicity saw almost an 8% increase in their open rates when they started using preview text. And WeddingWire saw a 30% increase in click-through rates by testing theirs.
<https://www.litmus.com/blog/the-ultimate-guide-to-preview-text-support>
Applies: preheader
Observer note: The 8% and 30% figures are single-brand anecdotes without method.

### E127 · Litmus (The Great Debate: Subject Line Length) · guide · published 2023-05-03 · fetched 2026-10-20 · via fetch
> Different devices have different display capabilities––A typical desktop inbox displays about 60 characters of an email’s subject line, while mobile devices show just 25-30 characters. Additionally, many email clients tend to truncate subject lines that are too long, making it harder for recipients to understand the purpose of the email at a glance.
> …That said, we recommend no more than 9 words and around 60 characters.
<https://www.litmus.com/blog/the-great-debate-subject-line-length>
Applies: subject, mobile
Observer note: NN/g recommends 40 characters; GetResponse finds highest opens below 70 characters.

### E128 · Litmus (Using Emojis in Subject Lines to Stand Out in the Inbox) · guide · published 2022-03-24 · fetched 2026-10-20 · via fetch
> An emoji in an email subject line can increase click-through rates (CTR) by 28%. However, this can vary based on the frequency of emoji use, if you choose the right emoji, and each subscriber’s familiarity with using emojis.
<https://www.litmus.com/blog/emojis-in-subject-lines>
Applies: subject
Observer note: No source or method given for the 28%; contradicts NN/g (no open lift, more negative sentiment) and GetResponse (lower opens and CTR with emoji).

### E129 · Litmus (What’s the Best Time to Send Email? We Analyzed Billions of Email Opens (Bettina Specht)) · benchmark · published 2021-10-07 · fetched 2026-10-20 · via fetch
> Almost 22% of all email campaigns are opened in the very first hour after they’re sent. And as each hour passes, the likelihood of your email getting more opens, decreases.
> In the United States, the most popular reading time for email is the morning. Opens start to pick up around 6am and continue to rise until the 10am peak. About 7.4% of all opens happen between 10am and 11am, and 21.2% of all opens happen between 9am and noon local time.
> …
> …In the 2021 State of Email Engagement report, we analyzed almost 8 billion email opens, anonymously collected and aggregated with Litmus Email Analytics from January 1 – August 31, 2021.…
<https://www.litmus.com/blog/whats-the-best-time-to-send-email-we-analyzed-billions-of-email-opens-to-find-out>
Applies: timing
Observer note: Pre-MPP data window (Jan–Aug 2021). Date: first published 2019-08-21.

### E130 · Litmus (Email marketing assumptions: I need to use double opt-in for all signups (Jason Rodriguez)) · guide · published 2021-04-22 · fetched 2026-10-20 · via fetch
> First, it’s good to understand that there aren’t explicit requirements for double opt-in when it comes to email marketing consent under GDPR. It focuses on getting consent that is, “freely given, specific, informed, and unambiguous.” If your signup forms make clear what subscribers are signing up for and don’t use pre-checked consent boxes, you’re good to go with single opt-in.
> …
> We’re marketers focused on growing our subscriber lists, leads, and customer base. As such, we still love using single opt-in where possible. But, we have seen an increase in bot signups and fake email addresses on some signup forms, like the one for our Litmus News newsletter. We implemented double opt-in for it, but for everything else—like signups to access our extensive collection of resources for email marketers—we still use single opt-in.
<https://www.litmus.com/blog/email-marketing-assumptions-i-need-to-use-double-opt-in-for-all-signups>
Applies: consent, signup, hygiene

### E131 · Litmus (What You Need to Know to Manage the 3 Kinds of Inactive Email Subscribers) · guide · published 2021-07-14 · fetched 2026-10-20 · via fetch
> While six months of inactivity is sometimes cited as a good trigger, it may feel pretty aggressive or too lax depending on how frequently your average subscriber gets an email from you.
> If you send emails one or more times a day, waiting six months may be too long. For lower frequency senders that send less than once a week, a longer timeframe may be better. For example, brands with a lot of customers who only buy during the holidays or supporters who only donate at the end of the year may want to look at 13 months of inactivity to accommodate this once-a-year seasonal behavior.
> Whether you use 6, 13, or any other number of months of inactivity as your threshold, that should trigger effective re-engagement emails followed by last-ditch re-permissioning efforts that ask inactives to confirm they want to continue receiving your emails. It’s only if all of those efforts fail that you purge these subscribers from your active mailing list and add them to a suppression list.
<https://www.litmus.com/blog/how-to-manage-the-3-kinds-of-inactive-email-subscribers>
Applies: hygiene, retention, frequency

### E132 · Validity (How to Explain Email Sunsetting to Your Boss (Julie Stuck)) · guide · published 2025-03-27 · fetched 2026-10-20 · via fetch
> Email sunsetting is the process of systematically removing email addresses that haven’t engaged with your email communications over a specified period—typically 9-12 months.
> …
> …Mailbox providers (MBPs) closely monitor user engagement. High levels of inactivity can prompt them to route your emails to spam folders, even for those subscribers that are actively engaging with your program. Low engagement signals—like deleting without reading or taking no action at all (just letting the message sit in the inbox)—indicate to MBPs that your emails may be unwanted, which can negatively impact your entire list.
<https://www.validity.com/blog/how-to-explain-email-sunsetting-to-your-boss/>
Applies: hygiene, deliverability, retention
Observer note: Another Validity strategist (2025-04-09) writes "6-9 months"; Mailgun's example uses three months.

### E133 · Validity (Improving Email Engagement by Understanding Negative Signals (Megan Farquharson)) · guide · published 2025-04-09 · fetched 2026-10-20 · via fetch
> Getting ignored hurts. If someone hasn’t opened/clicked within 6-9 months, it’s time to look into sunsetting. You can also try a win-back campaign, but if they don’t act, you must honor their choice.
> …
> …Use category CTAs, not a million product links. That’s what landing pages are for!
<https://www.validity.com/blog/improving-email-engagement-by-understanding-negative-signals/>
Applies: hygiene, links, retention

### E134 · Validity (Breaking Up is Hard to Do: The Top 6 Reasons for Email Unsubscribes (Danielle Gallant)) · guide · published 2023-05-30 · fetched 2026-10-20 · via fetch
> The good: Unsubscribes are not harmful to your email program’s reputation with mailbox providers. In the world of email engagement metrics, unsubscribes are considered neutral, meaning they won’t harm your email deliverability—your ability to reach subscribers’ inboxes instead of being filtered to their spam folders.
> …
> ‘Too much email’ is cited as a major reason for unsubscribing in almost every survey or study you’ll find on this topic.…
<https://www.validity.com/blog/breaking-up-is-hard-to-do-the-top-6-reasons-for-email-unsubscribes/>
Applies: unsubscribe, frequency, deliverability
Observer note: The six reasons listed: never meant to opt in; too much mail; images don't render; lost relevancy; spammy characteristics; no value. No survey numbers given.

### E135 · Sinch Mailgun (State of Email Deliverability – Chapter 5: Email list building and hygiene) · survey · published 2025-06-26 · fetched 2026-10-20 · via fetch
> • 27% Monthly or more
> • 16% Quarterly
> • 18% Once or twice per year
> • 27% Rarely
> • 12% Never
> …
> …For example, if someone hasn’t opened an email from you in three months, that contact is “sunsetted” or moved off the main list.
> Our research found sunset policies are uncommon among typical email senders. Just under 24% of all senders use a sunset policy. Nearly 59% say they do not and about 17% are unsure. These findings are almost unchanged from what we found in our 2023 survey.
> …
> Sinch Mailgun’s research shows nearly 40% of senders are using the double opt-in method while more than 47% are not and about 13% are unsure.
<https://www.mailgun.com/state-of-email-deliverability/chapter/list-building-hygiene/>
Applies: hygiene, consent
Observer note: Survey of senders; respondent count is on a separate "about the survey" chapter not fetched. Date: modified 2026-07-22.

### E136 · M3AAWG (Sender Best Common Practices, Version 3.0) · standard · published 2015-02 · fetched 2026-10-20 · via fetch
> Single opt-in should be used with extreme caution as it allows unconfirmed addresses to be added to mailing lists.
> …
> 4. The sender should notify recipients of the potential frequency and type of communications, and should give recipients a choice in each of these areas. Unexpected increases in the frequency of messages often leads to increased abuse complaints.
> …
> 8. A confirmation message should be sent (see 2.1, 2 above) immediately following an end user submitting their address to a list or as soon as possible (within 24 hours). These messages should adhere to the following guidelines when possible:
> …
> 9. Confirmed opt-in (also called “double opt-in” or “closed-loop subscription”) is the highest standard opt-in best practice. Recipients of the confirmation message are required to take an affirmative action to be added to the list. This prevents a typo or a maliciously submitted address from being added to ongoing mailings.
<https://www.m3aawg.org/sites/default/files/document/M3AAWG_Senders_BCP_Ver3-2015-02.pdf>
Applies: consent, signup, welcome, frequency
Observer note: Industry working group best practice (not law). Ranks confirmed opt-in "Best", whereas NN/g advises carefully weighing double opt-in.

### E137 · Marketing Science (INFORMS) (Personalization in Email Marketing: The Role of Noninformative Advertising Content (Sahni, Wheeler, Chintagunta)) · study · published 2018-02-27 · fetched 2026-10-20 · via fetch
> In our main experiment, we found that adding the name of the message recipient to the email’s subject line increased the probability of the recipient opening it by 20% (from 9.05% to 10.80%), which translated to an increase in sales’ leads by 31% (from 0.39% to 0.51%) and a reduction in the number of individuals unsubscribing from the email campaign by 17% (from 1.2% to 1.0%).
<https://doi.org/10.1287/mksc.2017.1066>
Applies: personalization, subject, unsubscribe
Observer note: Randomized field experiments with three companies, "millions of individuals" (abstract, via OpenAlex API).

### E138 · Marketing Letters (Springer) (Personalized subject lines in email marketing (Defau, Zauner)) · study · published 2023-09-08 · fetched 2026-10-20 · via fetch
> In our empirical research, we find no indication that using the first name of the recipient in the subject line has a positive effect on opening rates or click-through rates; as such we are not able to reproduce the findings from (Sahni et al., Marketing Science 37:236–258, 2018). However, we do find that using the surname of the recipient has a positive effect on opening rates and click-through rates; an indication that similar marketing techniques still have potential, but that they need to be more creative and surprising.
<https://doi.org/10.1007/s11002-023-09701-7>
Applies: personalization, subject
Observer note: Replication attempt of the study above; abstract via OpenAlex API.

### E139 · Review of Marketing Science (De Gruyter) (How to Improve the Performance of Email Marketing Campaigns? A Field Experiment Using the Authority Principle (Defau, Zauner, Sycik)) · study · published 2023-04-03 · fetched 2026-10-20 · via fetch
> For this project, we collaborated with a digital marketing company and run two randomized field experiments: examining if a small change in the email design – adding CEO to the sender line – has an effect on performance. Our results indicate that organizations can use authority figures to improve the performance of their email campaigns. However, there might be important differences between target groups, suggesting that specific audiences are more easily seduced by job titles than others.
<https://doi.org/10.1515/roms-2022-0095>
Applies: personalization, engagement
Observer note: Sender-name effect; NN/g guideline 65 says to use a person's name as sender only in special cases. Abstract via OpenAlex API.

### E140 · ACM CHI 2023 (“We Need a Big Revolution in Email Advertising”: Users’ Perception of Persuasion in Permission-based Advertising Emails (Sergeeva, Rohles, Distler, Koenig)) · study · published 2023-04-19 · fetched 2026-10-20 · via fetch
> Persuasive tactics intend to encourage users to open advertising emails. However, these tactics can overwhelm users, which makes them frustrated and leads to lower open rates.
> …
> Our qualitative findings reveal poor user experience with advertising emails, which was related to feeling surveilled by companies, forced subscription, high prior knowledge about persuasive tactics, and a desire for more agency. We also found that using certain persuasive tactics on the inbox level is perceived as ethically inappropriate.
<https://doi.org/10.1145/3544548.3581163>
Applies: subject, tone, consent
Observer note: Inbox prototype plus interviews, N=32 (abstract via OpenAlex API).

### E141 · ACM CHI 2020 (Why Johnny Can’t Unsubscribe: Barriers to Stopping Unwanted Email (Dev, Rader, Patil)) · study · published 2020-04-21 · fetched 2026-10-20 · via fetch
> We investigated the user experience of unsubscribing from unwanted email messages by recruiting 18 individuals for via a lab study followed by semi-structured interviews. Based on unsubscribing practices of the study participants, we synthesized eight common unsubscription mechanisms and identified the corresponding user experience challenges.
> …
> Our findings reveal frustration with the prevailing options for limiting access to the self by managing email boundaries.
<https://doi.org/10.1145/3313831.3376165>
Applies: unsubscribe
Observer note: Abstract via OpenAlex API; "for via" is as published.

### E142 · arXiv (Seidenberger, Ajisegiri, Pursell, Raja) (Why You’ve Got Mail: Evaluating Inbox Privacy Implications of Email Marketing Practices in Online Apps and Services) · study · published 2024-10-10 · fetched 2026-10-20 · via fetch
> Over the course of a year, we collected and analyzed the source, volume, frequency, and content of emails received by users after signing up for the 150 most popular online services and apps across various sectors.
> …
> Our study findings, conducted post-CCPA and GDPR, indicate that while no unknown third-party spam email was detected, internal and authorized third-party email marketing practices were pervasive, with companies frequently sending promotional and CRM emails despite opt-out preferences.
<https://arxiv.org/abs/2410.08302>
Applies: consent, frequency, unsubscribe
Observer note: Abstract via arXiv API.

### E143 · arXiv (Kong, Zhang, Sun, Chhabra et al.) (Multi-Objective Personalization in Multi-Stakeholder Organizational Bulk E-mail: A Field Experiment) · study · published 2023-02-22 · fetched 2026-10-20 · via fetch
> In an 8-week field experiment with a university newsletter, we implemented a 4x5x5 factorial design on personalizing subject lines, top news, and message order based on both the employees' and the organization's preferences.
> …
> Only when the top news solely contained the employee-preferred messages were the employees slightly more interested in the newsletter (strategic goal).
<https://arxiv.org/abs/2302.11156>
Applies: personalization, structure, subject
Observer note: Abstract via arXiv API.

### E144 · Boomerang (7 Tips for Getting More Responses to Your Emails (With Data!)) · study · published 2016-02 · fetched 2026-10-20 · via fetch
> The sweet spot for email length is between 50-125 words, all of which yielded response rates above 50%.
> Response rates declined slowly from 50% for 125-word messages to about 44% for 500-word messages.…
> …
> …Subject lines with only 3-4 words (excluding email conventions like Re: and Fwd:) received the most responses.…
> …
> …We found that emails that asked 1-3 questions are 50% more likely to get a response than emails asking no questions.
<https://blog.boomerangapp.com/2016/02/7-tips-for-getting-more-responses-to-your-emails-with-data/>
Applies: length, subject, engagement, tone
Observer note: Based on over 40 million one-to-one emails where users asked for a no-reply reminder; measures replies, not newsletter engagement. Date from URL.
