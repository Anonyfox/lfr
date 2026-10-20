package spam

import (
	"context"
	"database/sql"
	"encoding/json"
	"hash/fnv"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Anonyfox/lfr/internal/mailauth"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/store"
)

// Prior are the starting weights; learning moves away from them only as far
// as the operator's corrections justify.
var Prior = map[string]float64{
	"bias":              -3.0,
	"spf_pass":          -0.3,
	"spf_fail":          2.0,
	"spf_softfail":      0.8,
	"spf_none":          0.4,
	"dkim_aligned":      -1.5,
	"dkim_fail":         0.7,
	"dkim_none":         0.3,
	"dmarc_pass":        -1.0,
	"dmarc_fail":        2.5,
	"dmarc_none":        0.2,
	"dnsbl":             3.5,
	"no_ptr":            1.0,
	"ptr_unconfirmed":   0.6,
	"helo_bad":          1.0,
	"known_contact":     -3.0,
	"own_domain_forged": 5.0,
	"no_message_id":     1.0,
	"no_date":           0.8,
	"date_skew":         0.5,
	"subject_empty":     0.4,
	"subject_caps":      0.8,
	"subject_bait":      1.0,
	"scam_phrases":      3.0,
	"html_only":         0.4,
	"many_links":        0.6,
	"link_mismatch":     1.5,
	"shortener":         0.6,
	"from_spoof":        1.5,
	"risky_attachment":  1.5,
	"bayes":             4.0,
	"rep_sender":        2.5,
	"rep_domain":        1.5,
	"rep_net":           1.0,
}

// Bayes becomes part of the score once it has seen this much of each class.
const minTrained = 20

// Sample is what is stored with each message for later learning.
type Sample struct {
	X    map[string]float64 `json:"x"`
	Keys []string           `json:"keys"` // reputation keys
}

// Verdict is the classification of one message.
type Verdict struct {
	P       float64            `json:"p"`
	Bayes   float64            `json:"bayes,omitempty"` // -1 when not trained enough
	Hits    []string           `json:"hits,omitempty"`  // features that pushed the score, strongest first
	Sample  Sample             `json:"-"`
	Contrib map[string]float64 `json:"-"`
}

// Classifier scores and learns.
type Classifier struct {
	DB  *store.DB
	Now func() time.Time

	mu      sync.Mutex
	weights map[string]float64
}

func (c *Classifier) loadWeights(ctx context.Context) map[string]float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.weights != nil {
		return c.weights
	}
	w := map[string]float64{}
	for k, v := range Prior {
		w[k] = v
	}
	if s, _ := c.DB.Setting(ctx, "spam.weights", ""); s != "" {
		var stored map[string]float64
		if json.Unmarshal([]byte(s), &stored) == nil {
			for k, v := range stored {
				if _, known := Prior[k]; known && !math.IsNaN(v) && !math.IsInf(v, 0) {
					w[k] = v
				}
			}
		}
	}
	c.weights = w
	return w
}

// Weights returns the current weights.
func (c *Classifier) Weights(ctx context.Context) map[string]float64 {
	w := c.loadWeights(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]float64, len(w))
	for k, v := range w {
		out[k] = v
	}
	return out
}

// ReputationKeys are the identities whose history counts for a message.
func ReputationKeys(in Input) []string {
	var keys []string
	if in.Parsed != nil && in.Parsed.From.Email != "" {
		keys = append(keys, "addr:"+in.Parsed.From.Email)
	}
	if d := in.Auth.FromDomain; d != "" {
		// Domain reputation only counts for authenticated mail, or anyone could
		// borrow a good domain's standing.
		if in.Auth.DMARC == "pass" {
			keys = append(keys, "dom:"+mailauth.OrgDomain(d))
		} else {
			keys = append(keys, "unauth:"+mailauth.OrgDomain(d))
		}
	}
	if in.IP != nil {
		keys = append(keys, "net:"+network(in.IP))
	}
	return keys
}

func network(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return (&net.IPNet{IP: v4.Mask(net.CIDRMask(24, 32)), Mask: net.CIDRMask(24, 32)}).String()
	}
	return (&net.IPNet{IP: ip.Mask(net.CIDRMask(64, 128)), Mask: net.CIDRMask(64, 128)}).String()
}

// Classify scores a message.
func (c *Classifier) Classify(ctx context.Context, in Input) Verdict {
	x := Features(in)
	keys := ReputationKeys(in)
	for _, k := range keys {
		var spam, ham int
		if c.DB.R.QueryRowContext(ctx, "SELECT spam, ham FROM reputation WHERE key = ?", k).Scan(&spam, &ham) != nil {
			continue
		}
		v := float64(spam-ham) / float64(spam+ham+2)
		switch {
		case strings.HasPrefix(k, "addr:"):
			x["rep_sender"] = v
		case strings.HasPrefix(k, "dom:"), strings.HasPrefix(k, "unauth:"):
			x["rep_domain"] = v
		case strings.HasPrefix(k, "net:"):
			x["rep_net"] = v
		}
	}
	v := Verdict{Bayes: -1}
	if in.Parsed != nil {
		if b, ok := c.bayes(ctx, Tokens(in.Parsed)); ok {
			v.Bayes = b
			x["bayes"] = (b - 0.5) * 2
		}
	}
	w := c.loadWeights(ctx)
	c.mu.Lock()
	logit := w["bias"]
	contrib := map[string]float64{}
	for k, val := range x {
		if val != 0 {
			contrib[k] = w[k] * val
			logit += contrib[k]
		}
	}
	c.mu.Unlock()
	v.P = sigmoid(logit)
	v.Sample = Sample{X: x, Keys: keys}
	v.Contrib = contrib
	type kv struct {
		k string
		v float64
	}
	var all []kv
	for k, val := range contrib {
		all = append(all, kv{k, val})
	}
	sort.Slice(all, func(i, j int) bool { return math.Abs(all[i].v) > math.Abs(all[j].v) })
	for _, e := range all {
		if math.Abs(e.v) >= 0.3 {
			v.Hits = append(v.Hits, e.k+"="+strconv.FormatFloat(e.v, 'f', 1, 64))
		}
	}
	return v
}

func sigmoid(x float64) float64 { return 1 / (1 + math.Exp(-x)) }

// Tokens extracts the Bayes tokens of a message, each once.
func Tokens(p *msg.Parsed) []string {
	seen := map[string]bool{}
	var out []string
	add := func(t string) {
		if !seen[t] && len(out) < 3000 {
			seen[t] = true
			out = append(out, t)
		}
	}
	words := func(prefix, s string, limit int) {
		n := 0
		for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '$' && r != '€' && r != '%' && r != '!'
		}) {
			if l := len(w); l < 3 || l > 30 {
				continue
			}
			add(prefix + w)
			if n++; n > limit {
				return
			}
		}
	}
	words("s:", p.Subject, 50)
	words("", p.Text, 5000)
	if d := strings.ToLower(p.From.Email); d != "" {
		add("from:" + d)
		if i := strings.LastIndexByte(d, '@'); i >= 0 {
			add("fromdom:" + d[i+1:])
		}
	}
	for _, m := range hrefRE.FindAllStringSubmatch(p.HTML, 200) {
		if h := hostOf(m[1]); h != "" {
			add("url:" + mailauth.OrgDomain(h))
		}
	}
	for _, a := range p.Attachments {
		add("att:" + a.Type)
	}
	if p.HTML != "" {
		add("has:html")
	}
	return out
}

func hash(t string) int64 {
	h := fnv.New64a()
	h.Write([]byte(t))
	return int64(h.Sum64())
}

// bayes returns the spam probability by Robinson's chi-square combining, and
// false while the classifier has seen too few examples.
func (c *Classifier) bayes(ctx context.Context, tokens []string) (float64, bool) {
	nspam, nham := c.counts(ctx)
	if nspam < minTrained || nham < minTrained || len(tokens) == 0 {
		return 0, false
	}
	var probs []float64
	// Query in chunks: one round trip per 500 tokens.
	for i := 0; i < len(tokens); i += 500 {
		chunk := tokens[i:min(i+500, len(tokens))]
		args := make([]any, len(chunk))
		for j, t := range chunk {
			args[j] = hash(t)
		}
		rows, err := c.DB.R.QueryContext(ctx, "SELECT spam, ham FROM bayes WHERE token IN (?"+strings.Repeat(",?", len(chunk)-1)+")", args...)
		if err != nil {
			return 0, false
		}
		for rows.Next() {
			var s, h float64
			rows.Scan(&s, &h)
			ps := min(s/float64(nspam), 1)
			ph := min(h/float64(nham), 1)
			if ps+ph == 0 {
				continue
			}
			p := ps / (ps + ph)
			n := s + h
			f := (0.45 + n*p) / (1 + n) // Robinson: strength 1, assumed 0.45 for rare tokens
			if math.Abs(f-0.5) >= 0.1 {
				probs = append(probs, f)
			}
		}
		rows.Close()
	}
	if len(probs) == 0 {
		return 0.5, true
	}
	sort.Slice(probs, func(i, j int) bool { return math.Abs(probs[i]-0.5) > math.Abs(probs[j]-0.5) })
	if len(probs) > 150 {
		probs = probs[:150]
	}
	var lnS, lnH float64
	for _, p := range probs {
		p = min(max(p, 0.0001), 0.9999)
		lnH += math.Log(p)
		lnS += math.Log(1 - p)
	}
	n := len(probs)
	S := 1 - chi2Q(-2*lnS, 2*n)
	H := 1 - chi2Q(-2*lnH, 2*n)
	return (S - H + 1) / 2, true
}

// chi2Q is the survival function of chi-square with even degrees of freedom v.
func chi2Q(x2 float64, v int) float64 {
	m := x2 / 2
	sum := math.Exp(-m)
	term := sum
	for i := 1; i < v/2; i++ {
		term *= m / float64(i)
		sum += term
	}
	return min(sum, 1)
}

func (c *Classifier) counts(ctx context.Context) (int, int) {
	s, _ := c.DB.Setting(ctx, "spam.nspam", "0")
	h, _ := c.DB.Setting(ctx, "spam.nham", "0")
	ns, _ := strconv.Atoi(s)
	nh, _ := strconv.Atoi(h)
	return ns, nh
}

// Train learns that a message is spam (or ham). prev is how it was trained
// before ("" for never); a correction first undoes the earlier lesson.
func (c *Classifier) Train(ctx context.Context, p *msg.Parsed, sampleJSON string, label, prev string) error {
	if label == prev {
		return nil
	}
	if label != "spam" && label != "ham" {
		return nil
	}
	var sample Sample
	json.Unmarshal([]byte(sampleJSON), &sample)
	tokens := Tokens(p)
	err := c.DB.Tx(ctx, func(tx *sql.Tx) error {
		if prev != "" {
			if err := c.apply(ctx, tx, tokens, sample.Keys, prev, -1); err != nil {
				return err
			}
		}
		return c.apply(ctx, tx, tokens, sample.Keys, label, +1)
	})
	if err != nil {
		return err
	}
	if len(sample.X) > 0 {
		return c.learnWeights(ctx, sample.X, label == "spam")
	}
	return nil
}

func (c *Classifier) apply(ctx context.Context, tx *sql.Tx, tokens, keys []string, label string, delta int) error {
	col := "ham"
	if label == "spam" {
		col = "spam"
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO bayes(token, spam, ham) VALUES(?, ?, ?) ON CONFLICT(token) DO UPDATE SET "+col+" = max(0, "+col+" + ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, t := range tokens {
		s, h := 0, 0
		if delta > 0 {
			if col == "spam" {
				s = 1
			} else {
				h = 1
			}
		}
		if _, err := stmt.ExecContext(ctx, hash(t), s, h, delta); err != nil {
			return err
		}
	}
	now := c.Now().Unix()
	for _, k := range keys {
		s, h := 0, 0
		if delta > 0 {
			if col == "spam" {
				s = 1
			} else {
				h = 1
			}
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO reputation(key, spam, ham, updated) VALUES(?, ?, ?, ?) ON CONFLICT(key) DO UPDATE SET "+col+" = max(0, "+col+" + ?), updated = ?", k, s, h, now, delta, now); err != nil {
			return err
		}
	}
	key := "spam.nham"
	if col == "spam" {
		key = "spam.nspam"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES(?, CAST(max(0, ?) AS TEXT))
		ON CONFLICT(key) DO UPDATE SET value = CAST(max(0, CAST(value AS INTEGER) + ?) AS TEXT)`, key, delta, delta)
	return err
}

// learnWeights takes one regularized gradient step of logistic regression
// toward the operator's label, pulling each weight back toward its prior.
func (c *Classifier) learnWeights(ctx context.Context, x map[string]float64, spam bool) error {
	w := c.loadWeights(ctx)
	c.mu.Lock()
	logit := w["bias"]
	for k, v := range x {
		logit += w[k] * v
	}
	y := 0.0
	if spam {
		y = 1
	}
	const rate, pull = 0.15, 0.01
	grad := y - sigmoid(logit)
	update := func(k string, v float64) {
		nw := w[k] + rate*grad*v - pull*(w[k]-Prior[k])
		w[k] = min(max(nw, Prior[k]-6), Prior[k]+6)
	}
	update("bias", 1)
	for k, v := range x {
		if _, known := Prior[k]; known && v != 0 {
			update(k, v)
		}
	}
	b, _ := json.Marshal(w)
	c.mu.Unlock()
	return c.DB.SetSetting(ctx, "spam.weights", string(b))
}

// Stats describes what the filter has learned.
type Stats struct {
	Spam       int  `json:"trained_spam"`
	Ham        int  `json:"trained_ham"`
	BayesReady bool `json:"bayes_active"`
	Tokens     int  `json:"tokens"`
	Reputation int  `json:"reputation_keys"`
}

// Stats reports training state.
func (c *Classifier) Stats(ctx context.Context) Stats {
	var s Stats
	s.Spam, s.Ham = c.counts(ctx)
	s.BayesReady = s.Spam >= minTrained && s.Ham >= minTrained
	c.DB.R.QueryRowContext(ctx, "SELECT count(*) FROM bayes").Scan(&s.Tokens)
	c.DB.R.QueryRowContext(ctx, "SELECT count(*) FROM reputation").Scan(&s.Reputation)
	return s
}

// Reset forgets everything learned.
func (c *Classifier) Reset(ctx context.Context) error {
	err := c.DB.Tx(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{"DELETE FROM bayes", "DELETE FROM reputation", "DELETE FROM settings WHERE key IN ('spam.weights', 'spam.nspam', 'spam.nham')", "UPDATE messages SET trained = ''"} {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return err
			}
		}
		return nil
	})
	c.mu.Lock()
	c.weights = nil
	c.mu.Unlock()
	return err
}
