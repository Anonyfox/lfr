package web

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// stock holds the fonts and colours page generators default to, copied from
// lfm (testdata/stock.json names the source). docs/web-design.md explains
// why the list sites must not use them.
type stock struct {
	Accents        []struct{ Name, Value string }
	Creams         []string
	Terracottas    []string
	WindowControls [][]string
	GenericSans    []string
	Display        []string
	Mono           []string
	KitTokens      []string
}

func loadStock(t *testing.T) stock {
	t.Helper()
	b, err := os.ReadFile("testdata/stock.json")
	if err != nil {
		t.Fatal(err)
	}
	var s stock
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPagesAvoidStockDesign(t *testing.T) {
	s := loadStock(t)
	for name, page := range samplePages(t) {
		for _, p := range s.check(cssOf(string(page))) {
			t.Errorf("%s: %s", name, p)
		}
	}
}

// The check itself must catch what generators do.
func TestStockCheckCatchesDefaults(t *testing.T) {
	s := loadStock(t)
	for _, css := range []string{
		`body{font-family:Inter,sans-serif}`,
		`:root{--f:"Space Grotesk",serif}h1{font:700 40px/1.1 var(--f)}`,
		`.label{font:12px/1 "JetBrains Mono",monospace}`,
		`a{color:#6366f1}`,
		`a{color:oklch(58.5% 0.233 277.117)}`,
		`.b{background:#a855f7}`,
		`body{background:#faf9f5}a{color:#d97757}`,
		`:root{--primary:222.2 47.4% 11.2%}`,
	} {
		if len(s.check(css)) == 0 {
			t.Errorf("not caught: %s", css)
		}
	}
	if p := s.check(`pre,code{font:14px ui-monospace,monospace}body{font:18px Palatino,serif;color:#142033}`); len(p) != 0 {
		t.Errorf("caught: %v", p)
	}
}

var (
	styleBlock = regexp.MustCompile(`(?s)<style[^>]*>(.*?)</style>`)
	styleAttr  = regexp.MustCompile(`style="([^"]*)"`)
	cssRule    = regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
	customProp = regexp.MustCompile(`(--[\w-]+)\s*:\s*([^;{}]+)`)
	fontSize   = regexp.MustCompile(`^(?:.*?\s)?[\d.]+(?:px|em|rem|%)(?:/[\w.%]+)?\s+(.+)$`)
	hexColor   = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	oklchColor = regexp.MustCompile(`oklch\(\s*([\d.]+)(%?)\s+([\d.]+)\s+([\d.]+)\s*\)`)
	varRef     = regexp.MustCompile(`var\((--[\w-]+)\)`)
	codeSel    = regexp.MustCompile(`\b(code|pre|kbd|samp)\b`)
)

// cssOf collects a page's style sheets and style attributes.
func cssOf(page string) string {
	var b strings.Builder
	for _, m := range styleBlock.FindAllStringSubmatch(page, -1) {
		b.WriteString(m[1] + "\n")
	}
	for _, m := range styleAttr.FindAllStringSubmatch(page, -1) {
		b.WriteString("[style]{" + m[1] + "}\n")
	}
	return b.String()
}

func (s stock) check(css string) []string {
	var problems []string
	in := func(list []string, family string) bool {
		family = strings.ToLower(strings.Trim(strings.TrimSpace(family), `"'`))
		for _, f := range list {
			if f == family {
				return true
			}
		}
		return false
	}
	vars := map[string]string{}
	for _, m := range customProp.FindAllStringSubmatch(css, -1) {
		vars[m[1]] = strings.TrimSpace(m[2])
		for _, k := range s.KitTokens {
			if strings.Join(strings.Fields(strings.ToLower(m[2])), " ") == k {
				problems = append(problems, fmt.Sprintf("%s holds the component kit token %q", m[1], k))
			}
		}
	}
	resolve := func(v string) string {
		return varRef.ReplaceAllStringFunc(v, func(r string) string { return vars[varRef.FindStringSubmatch(r)[1]] })
	}
	for _, rule := range cssRule.FindAllStringSubmatch(css, -1) {
		selector := strings.TrimSpace(rule[1][strings.LastIndex(rule[1], ";")+1:])
		for _, decl := range strings.Split(rule[2], ";") {
			prop, value, ok := strings.Cut(decl, ":")
			if !ok {
				continue
			}
			prop, value = strings.TrimSpace(prop), resolve(strings.TrimSpace(value))
			var families string
			switch {
			case prop == "font-family" || strings.HasPrefix(prop, "--") && fontSize.FindStringSubmatch(value) == nil && strings.Contains(value, ","):
				families = value
			case prop == "font":
				if m := fontSize.FindStringSubmatch(value); m != nil {
					families = m[1]
				}
			}
			for _, f := range strings.Split(families, ",") {
				switch {
				case in(s.GenericSans, f):
					problems = append(problems, fmt.Sprintf("%s sets the stock sans %s", selector, strings.TrimSpace(f)))
				case in(s.Display, f):
					problems = append(problems, fmt.Sprintf("%s sets the stock display face %s", selector, strings.TrimSpace(f)))
				case in(s.Mono, f) && !codeSel.MatchString(selector) && !strings.HasPrefix(prop, "--"):
					problems = append(problems, fmt.Sprintf("%s sets monospace %s outside code", selector, strings.TrimSpace(f)))
				}
			}
		}
	}
	colors := map[string]oklab{}
	for _, h := range hexColor.FindAllString(css, -1) {
		if c, ok := parseHex(h); ok {
			colors[h] = c
		}
	}
	for _, m := range oklchColor.FindAllStringSubmatch(css, -1) {
		colors[m[0]] = parseOKLCH(m[1:])
	}
	near := func(c oklab, values []string, limit float64) bool {
		for _, v := range values {
			if c.distance(parseColor(v)) < limit {
				return true
			}
		}
		return false
	}
	var cream, terracotta bool
	controls := make([]bool, len(s.WindowControls))
	for text, c := range colors {
		for _, a := range s.Accents {
			if c.distance(parseColor(a.Value)) < 0.02 {
				problems = append(problems, fmt.Sprintf("%s is the stock accent %s", text, a.Name))
			}
		}
		if c.purpleish() {
			problems = append(problems, fmt.Sprintf("%s is in the violet band of generated accents", text))
		}
		cream = cream || near(c, s.Creams, 0.012)
		terracotta = terracotta || near(c, s.Terracottas, 0.02)
		for i, set := range s.WindowControls {
			controls[i] = controls[i] || near(c, set, 0.04)
		}
	}
	if cream && terracotta {
		problems = append(problems, "cream with terracotta: the assistant house palette")
	}
	if len(controls) > 0 && !strings.Contains(fmt.Sprint(controls), "false") {
		problems = append(problems, "red, yellow and green window dots")
	}
	return problems
}

// oklab is a colour in the OKLab space, where distances follow perception.
type oklab struct{ L, A, B float64 }

func (c oklab) distance(o oklab) float64 {
	return math.Sqrt((c.L-o.L)*(c.L-o.L) + (c.A-o.A)*(c.A-o.A) + (c.B-o.B)*(c.B-o.B))
}

func (c oklab) purpleish() bool {
	chroma, hue := math.Hypot(c.A, c.B), math.Atan2(c.B, c.A)*180/math.Pi
	if hue < 0 {
		hue += 360
	}
	return chroma >= 0.12 && hue >= 270 && hue <= 330 && c.L > 0.3 && c.L < 0.85
}

func parseColor(v string) oklab {
	if m := oklchColor.FindStringSubmatch(v); m != nil {
		return parseOKLCH(m[1:])
	}
	c, _ := parseHex(v)
	return c
}

func parseOKLCH(m []string) oklab {
	l, _ := strconv.ParseFloat(m[0], 64)
	if m[1] == "%" {
		l /= 100
	}
	chroma, _ := strconv.ParseFloat(m[2], 64)
	hue, _ := strconv.ParseFloat(m[3], 64)
	return oklab{l, chroma * math.Cos(hue*math.Pi/180), chroma * math.Sin(hue*math.Pi/180)}
}

func parseHex(h string) (oklab, bool) {
	h = strings.TrimPrefix(h, "#")
	switch len(h) {
	case 3, 4:
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	case 6, 8:
		h = h[:6]
	default:
		return oklab{}, false
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return oklab{}, false
	}
	lin := func(x uint64) float64 {
		c := float64(x) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	r, g, b := lin(v>>16&255), lin(v>>8&255), lin(v&255)
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return oklab{0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s}, true
}
