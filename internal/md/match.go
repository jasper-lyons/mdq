package md

import (
	"regexp"
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
