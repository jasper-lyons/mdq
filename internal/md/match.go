package md

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	stripCharsRe = regexp.MustCompile("[`*~\"]+")
	linkRe       = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	spaceRe      = regexp.MustCompile(`\s+`)
	trailParenRe = regexp.MustCompile(`\s*\([^()]*\)\s*$`)
	sepRe        = regexp.MustCompile(`(?:\s+(?:\x{2014}|\x{2013}|--|-)\s+|:\s+)`)
	statusTailRe = regexp.MustCompile(`^(done|closed|clear|cleared|fixed|resolved|wontfix|superseded|todo|wip|open|blocked|deferred|cancelled|canceled|in progress)\b`)
	trailPunctRe = regexp.MustCompile(`[\s:.,;!?]+$`)
	headingNumRe = regexp.MustCompile(`^\d+$`)
)

// Normalize reduces a heading to a comparable form: no leading hashes, no
// emoji, no markup characters, lower case, single spaces.
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "#")
	s = linkRe.ReplaceAllString(s, "$1")
	s = stripCharsRe.ReplaceAllString(s, "")
	s = StripLeadingSymbols(s)
	s = strings.ToLower(s)
	s = spaceRe.ReplaceAllString(s, " ")
	s = trailPunctRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// Core is Normalize with trailing status text removed, so that
// "Deploy gate - CLEAR (closed 2026-08-05)" answers to "deploy gate".
// Core is Normalize with trailing status text removed, so that
// "Deploy gate - CLEAR (closed 2026-08-05)" answers to "deploy gate".
func Core(s string) string {
	s = Normalize(s)
	for {
		before := s
		s = trailParenRe.ReplaceAllString(s, "")
		if seps := sepRe.FindAllStringIndex(s, -1); seps != nil {
			for i := len(seps) - 1; i >= 0; i-- {
				if statusTailRe.MatchString(s[seps[i][1]:]) {
					s = strings.TrimSpace(s[:seps[i][0]])
					break
				}
			}
		}
		for _, w := range statusWords {
			if strings.HasSuffix(s, " "+w) {
				s = strings.TrimSpace(strings.TrimSuffix(s, " "+w))
			}
		}
		s = trailPunctRe.ReplaceAllString(s, "")
		if s == before || s == "" {
			return s
		}
	}
}

var statusWords = []string{"done", "closed", "clear", "fixed", "resolved", "wontfix", "superseded"}

// Match is the outcome of resolving a heading query against a document.
type Match struct {
	Found     []Heading // one entry on success, several when ambiguous
	Ambiguous bool
}

// FindHeading resolves a query to a heading. The query may be a line number, a
// /regex/, or heading text matched case insensitively as an exact name, then a
// prefix, then a substring. The first tier that matches anything wins.
func (d *Document) FindHeading(query string) Match {
	q := strings.TrimSpace(query)
	if q == "" {
		return Match{}
	}
	if headingNumRe.MatchString(q) {
		n, err := strconv.Atoi(q)
		if err == nil {
			for _, h := range d.Headings {
				if h.Line == n {
					return Match{Found: []Heading{h}}
				}
			}
			if h, ok := d.HeadingAt(n); ok {
				return Match{Found: []Heading{h}}
			}
		}
		return Match{}
	}
	if len(q) > 2 && strings.HasPrefix(q, "/") && strings.HasSuffix(q, "/") {
		re, err := regexp.Compile("(?i)" + q[1:len(q)-1])
		if err != nil {
			return Match{}
		}
		var hits []Heading
		for _, h := range d.Headings {
			if re.MatchString(h.Text) || re.MatchString(Normalize(h.Text)) {
				hits = append(hits, h)
			}
		}
		return decide(hits)
	}

	nq, cq := Normalize(q), Core(q)
	var exact, prefix, sub []Heading
	for _, h := range d.Headings {
		nh, ch := Normalize(h.Text), Core(h.Text)
		switch {
		case nh == nq || ch == nq || ch == cq:
			exact = append(exact, h)
		case strings.HasPrefix(nh, nq) || strings.HasPrefix(ch, cq):
			prefix = append(prefix, h)
		case strings.Contains(nh, nq):
			sub = append(sub, h)
		}
	}
	for _, tier := range [][]Heading{exact, prefix, sub} {
		if len(tier) > 0 {
			return decide(tier)
		}
	}
	return Match{}
}

func decide(hits []Heading) Match {
	if len(hits) == 0 {
		return Match{}
	}
	if len(hits) == 1 {
		return Match{Found: hits}
	}
	return Match{Found: hits, Ambiguous: true}
}

var (
	wordRe   = regexp.MustCompile(`[a-z0-9]+`)
	escapeRe = regexp.MustCompile(`\\.`)
)

// stopWords carry no signal when comparing a query to a heading.
var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "in": true, "is": true,
	"it": true, "its": true, "of": true, "on": true, "or": true, "that": true,
	"the": true, "this": true, "to": true, "was": true, "with": true,
}

// QueryWords reduces a search pattern or a heading query to the distinct words
// worth comparing: lower case, two characters or more, no stop words. Regex
// escapes and punctuation are dropped, so `^## (Plan|Code)$` gives plan, code.
func QueryWords(s string) []string {
	s = escapeRe.ReplaceAllString(strings.ToLower(s), " ")
	seen := map[string]bool{}
	var out []string
	for _, w := range wordRe.FindAllString(s, -1) {
		if len(w) < 2 || stopWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

// HeadingScore counts how many of words appear as whole words in the heading
// text, ignoring case. The synthetic preamble entry always scores zero.
func HeadingScore(h Heading, words []string) int {
	if h.Pseudo || len(words) == 0 {
		return 0
	}
	have := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(h.Text), -1) {
		have[w] = true
	}
	n := 0
	for _, w := range words {
		if have[w] {
			n++
		}
	}
	return n
}

// Nearest returns up to n headings sharing the most words with query, best
// first and then by line. Headings sharing no word at all are left out, so an
// empty result means nothing in the document resembles the query.
func (d *Document) Nearest(query string, n int) []Heading {
	words := QueryWords(query)
	if len(words) == 0 || n <= 0 {
		return nil
	}
	type scored struct {
		h Heading
		s int
	}
	var all []scored
	for _, h := range d.Headings {
		if s := HeadingScore(h, words); s > 0 {
			all = append(all, scored{h, s})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].s != all[j].s {
			return all[i].s > all[j].s
		}
		return all[i].h.Line < all[j].h.Line
	})
	if len(all) > n {
		all = all[:n]
	}
	out := make([]Heading, len(all))
	for i, s := range all {
		out[i] = s.h
	}
	return out
}
