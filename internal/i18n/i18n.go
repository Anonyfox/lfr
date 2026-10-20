// Package i18n holds the few sentences lfr itself writes to readers, in
// English and German. Lists choose their language; anything else falls back
// to English.
package i18n

import "strings"

var text = map[string]map[string]string{
	"en": {
		"confirm.subject":      "Please confirm your subscription to %s",
		"confirm.intro":        "Somebody, hopefully you, asked to receive %s at this address.",
		"confirm.action":       "Yes, subscribe me",
		"confirm.ignore":       "If that was not you, ignore this message and nothing happens.",
		"confirm.cadence":      "What to expect: %s.\n\n",
		"page.check.title":     "Check your inbox",
		"page.check.body":      "We sent a confirmation link to your address. Click it to finish subscribing.",
		"page.confirm.title":   "Confirm your subscription",
		"page.confirm.body":    "One click and you receive %s.",
		"page.confirmed.title": "You are subscribed",
		"page.confirmed.body":  "Thanks for confirming. The next issue of %s comes straight to your inbox.",
		"page.unsub.title":     "Unsubscribe",
		"page.unsub.body":      "Stop receiving %s at this address?",
		"page.unsub.action":    "Unsubscribe",
		"page.unsubbed.title":  "You are unsubscribed",
		"page.unsubbed.body":   "You will not receive %s anymore. Changed your mind? You can subscribe again any time.",
		"page.invalid.title":   "This link does not work",
		"page.invalid.body":    "It may be incomplete or out of date.",
		"page.error.title":     "Something went wrong",
		"page.error.body":      "Please try again in a moment.",
		"page.limit.title":     "Too many attempts",
		"page.limit.body":      "Please try again later.",
		"form.email":           "Email address",
		"form.submit":          "Subscribe",
		"form.consent":         "I want to receive %s by email and can unsubscribe at any time.",
		"form.consent.tracked": "I want to receive %s by email, with statistics on which issues are opened and which links clicked, and can unsubscribe at any time.",
		"home.archive":         "Past issues",
		"home.all":             "All issues",
		"home.empty":           "The first issue is on its way.",
		"issue.back":           "All issues",
		"footer.why":           "You receive this because you subscribed to %s.",
		"footer.since":         "You receive this because you subscribed to %s on %s.",
		"footer.since.at":      "You receive this because you subscribed to %s on %s at %s.",
		"footer.forwarded":     "Forwarded to you? Subscribe at %s",
		"footer.unsub":         "Unsubscribe",
		"footer.web":           "View in browser",
		"date":                 "January 2, 2006",
		"unsub.reply.subject":  "Your unsubscribe link for %s",
		"unsub.reply.body":     "You asked to leave %s. One click on this link and no further issue comes:\n\n%s\n",
		"footer.imprint":       "Imprint",
		"footer.privacy":       "Privacy",
		"feed":                 "Feed",
	},
	"de": {
		"confirm.subject":      "Bitte bestätige dein Abonnement von %s",
		"confirm.intro":        "Jemand, hoffentlich du, möchte %s an diese Adresse bekommen.",
		"confirm.action":       "Ja, ich will das abonnieren",
		"confirm.ignore":       "Warst du das nicht, ignoriere diese Nachricht einfach. Dann passiert nichts.",
		"confirm.cadence":      "Was dich erwartet: %s.\n\n",
		"page.check.title":     "Schau in dein Postfach",
		"page.check.body":      "Wir haben dir einen Bestätigungslink geschickt. Klick ihn an, um das Abonnement abzuschließen.",
		"page.confirm.title":   "Abonnement bestätigen",
		"page.confirm.body":    "Ein Klick, und du bekommst %s.",
		"page.confirmed.title": "Du bist dabei",
		"page.confirmed.body":  "Danke für die Bestätigung. Die nächste Ausgabe von %s kommt direkt in dein Postfach.",
		"page.unsub.title":     "Abmelden",
		"page.unsub.body":      "%s nicht mehr an diese Adresse bekommen?",
		"page.unsub.action":    "Abmelden",
		"page.unsubbed.title":  "Du bist abgemeldet",
		"page.unsubbed.body":   "Du bekommst %s nicht mehr. Doch anders entschieden? Du kannst dich jederzeit wieder anmelden.",
		"page.invalid.title":   "Dieser Link funktioniert nicht",
		"page.invalid.body":    "Vielleicht ist er unvollständig oder veraltet.",
		"page.error.title":     "Etwas ist schiefgelaufen",
		"page.error.body":      "Bitte versuch es gleich noch einmal.",
		"page.limit.title":     "Zu viele Versuche",
		"page.limit.body":      "Bitte versuch es später noch einmal.",
		"form.email":           "E-Mail-Adresse",
		"form.submit":          "Abonnieren",
		"form.consent":         "Ich möchte %s per E-Mail bekommen und kann mich jederzeit abmelden.",
		"form.consent.tracked": "Ich möchte %s per E-Mail bekommen, mit einer Statistik, welche Ausgaben geöffnet und welche Links geklickt werden, und kann mich jederzeit abmelden.",
		"home.archive":         "Bisherige Ausgaben",
		"home.all":             "Alle Ausgaben",
		"home.empty":           "Die erste Ausgabe ist unterwegs.",
		"issue.back":           "Alle Ausgaben",
		"footer.why":           "Du bekommst diese E-Mail, weil du %s abonniert hast.",
		"footer.since":         "Du bekommst diese E-Mail, weil du %s am %s abonniert hast.",
		"footer.since.at":      "Du bekommst diese E-Mail, weil du %s am %s auf %s abonniert hast.",
		"footer.forwarded":     "Weitergeleitet bekommen? Abonnieren auf %s",
		"footer.unsub":         "Abmelden",
		"footer.web":           "Im Browser ansehen",
		"date":                 "2.1.2006",
		"unsub.reply.subject":  "Dein Abmeldelink für %s",
		"unsub.reply.body":     "Du möchtest %s abbestellen. Ein Klick auf diesen Link, und es kommt keine Ausgabe mehr:\n\n%s\n",
		"footer.imprint":       "Impressum",
		"footer.privacy":       "Datenschutz",
		"feed":                 "Feed",
	},
}

// Supported reports whether lang has its own texts.
func Supported(lang string) bool { _, ok := text[base(lang)]; return ok }

func base(lang string) string {
	lang = strings.ToLower(lang)
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		lang = lang[:i]
	}
	return lang
}

// T returns the text for key in lang, falling back to English.
func T(lang, key string) string {
	if m, ok := text[base(lang)]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	if s, ok := text["en"][key]; ok {
		return s
	}
	return key
}
