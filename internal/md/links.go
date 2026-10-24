package md

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Link is one link found in a document.
type Link struct {
	File        string   `json:"file"`
	Line        int      `json:"line"`
	Text        string   `json:"text"`
	Target      string   `json:"target"`
	Path        string   `json:"path,omitempty"`
	Anchor      string   `json:"anchor,omitempty"`
	Kind        string   `json:"kind"` // inline, autolink, wiki, code or bare
	Section     string   `json:"section,omitempty"`
	SectionPath []string `json:"section_path,omitempty"`
	Missing     bool     `json:"missing,omitempty"`
}

// LinkOptions controls which links are reported.
type LinkOptions struct {
	Bare bool // also report bare http(s) URLs in prose
}

var (
	inlineLinkRe = regexp.MustCompile(`\[([^\[\]]*)\]\(\s*(<[^>]*>|[^\s()]*(?:\([^\s()]*\)[^\s()]*)*)(?:\s+(?:"[^"]*"|'[^']*'))?\s*\)`)
	autolinkRe   = regexp.MustCompile(`<((?:https?://|mailto:)[^>\s]+)>`)
	wikiLinkRe   = regexp.MustCompile(`\[\[([^\[\]\n|]+)(?:\|([^\[\]\n]*))?\]\]`)
	bareLinkRe   = regexp.MustCompile(`https?://[^\s<>"'\]]+`)
	codePathRe   = regexp.MustCompile(`(?i)^[^\s#*?<>|:]+\.(?:md|markdown|mdx)(?:#\S*)?$`)
	blankGapRe   = regexp.MustCompile(`\n[ \t]*\n`)
)

type rawLink struct {
	off  int
	link Link
}

// Links returns every inline, autolink, wiki and code span link in the
// document, in line order. Fenced code and front matter are skipped.
func (d *Document) Links() []Link {
	return d.LinksWith(LinkOptions{})
}

// LinksWith is Links with options.
func (d *Document) LinksWith(opt LinkOptions) []Link {
	// One string for the whole body, with excluded lines blanked so a match
	// can never run across a fence or out of front matter.
	lines := make([]string, len(d.Lines))
	for i, l := range d.Lines {
		if i+1 <= d.FMEnd || d.Fenced[i] {
			l = ""
		}
		lines[i] = l
	}
	text := strings.Join(lines, "\n")
	starts := make([]int, len(lines))
	off := 0
	for i, l := range lines {
		starts[i] = off
		off += len(l) + 1
	}
	lineOf := func(o int) int {
		return sort.Search(len(starts), func(i int) bool { return starts[i] > o })
	}

	var raws []rawLink
	add := func(o int, kind, label, target string) {
		raws = append(raws, rawLink{o, d.newLink(lineOf(o), kind, label, target)})
	}

	masked, spans := maskCode(text)
	for _, s := range spans {
		c := strings.TrimSpace(s.content)
		if codePathRe.MatchString(c) {
			add(s.off, "code", c, c)
		}
	}

	masked = replaceMatches(masked, wikiLinkRe, func(o int, m []string) {
		label := m[1]
		if m[2] != "" {
			label = m[2]
		}
		add(o, "wiki", strings.TrimSpace(label), strings.TrimSpace(m[1]))
	})
	masked = replaceMatches(masked, inlineLinkRe, func(o int, m []string) {
		if o > 0 && masked[o-1] == '!' {
			return
		}
		if blankGapRe.MatchString(m[0]) {
			return
		}
		target := strings.TrimSuffix(strings.TrimPrefix(m[2], "<"), ">")
		if target == "" {
			return
		}
		add(o, "inline", strings.Join(strings.Fields(m[1]), " "), target)
	})
	masked = replaceMatches(masked, autolinkRe, func(o int, m []string) {
		add(o, "autolink", m[1], m[1])
	})
	if opt.Bare {
		replaceMatches(masked, bareLinkRe, func(o int, m []string) {
			u := trimBare(m[0])
			if u != "" {
				add(o, "bare", u, u)
			}
		})
	}

	sort.SliceStable(raws, func(i, j int) bool { return raws[i].off < raws[j].off })
	out := make([]Link, len(raws))
	for i, r := range raws {
		out[i] = r.link
	}
	return out
}

func (d *Document) newLink(line int, kind, label, target string) Link {
	l := Link{File: d.File, Line: line, Text: label, Target: target, Kind: kind}
	if h, ok := d.HeadingAt(line); ok {
		l.Section = h.Text
		l.SectionPath = d.Path(h)
	}
	if IsExternal(target) {
		return l
	}
	if i := strings.Index(target, "#"); i >= 0 {
		l.Target, l.Anchor = target[:i], target[i+1:]
	}
	l.Path, l.Missing = d.resolve(l.Target, kind == "code")
	return l
}

// IsExternal reports whether a link target leaves the filesystem.
func IsExternal(target string) bool {
	return strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:")
}

// resolve finds the file an internal link points at, printed relative to the
// working directory. A code span path that is not next to the file is tried
// against each ancestor directory up to the repository root.
func (d *Document) resolve(target string, code bool) (string, bool) {
	abs, err := filepath.Abs(d.File)
	if err != nil {
		abs = d.File
	}
	if target == "" {
		return relToCwd(abs), false
	}
	if u, err := url.PathUnescape(target); err == nil {
		target = u
	}
	dir := filepath.Dir(abs)
	var cands []string
	if filepath.IsAbs(target) {
		cands = []string{filepath.Clean(target)}
	} else {
		cands = []string{filepath.Join(dir, target)}
		if code {
			for cur := dir; ; {
				if exists(filepath.Join(cur, ".git")) {
					break
				}
				parent := filepath.Dir(cur)
				if parent == cur {
					break
				}
				cur = parent
				cands = append(cands, filepath.Join(cur, target))
				// The repo root is included, then the walk stops.
			}
		}
	}
	for _, c := range cands {
		if exists(c) {
			return relToCwd(c), false
		}
	}
	return relToCwd(cands[0]), true
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func relToCwd(abs string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return abs
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil {
		return abs
	}
	return rel
}

// replaceMatches calls fn for each match of re in s, then returns s with the
// matched text replaced by spaces (newlines kept), so later passes cannot see
// it and offsets stay valid.
func replaceMatches(s string, re *regexp.Regexp, fn func(off int, m []string)) string {
	idx := re.FindAllStringSubmatchIndex(s, -1)
	if idx == nil {
		return s
	}
	b := []byte(s)
	for _, loc := range idx {
		m := make([]string, len(loc)/2)
		for i := range m {
			if loc[2*i] >= 0 {
				m[i] = s[loc[2*i]:loc[2*i+1]]
			}
		}
		fn(loc[0], m)
		blank(b, loc[0], loc[1])
	}
	return string(b)
}

func blank(b []byte, from, to int) {
	for i := from; i < to; i++ {
		if b[i] != '\n' {
			b[i] = ' '
		}
	}
}

type codeSpan struct {
	off     int
	content string
}

// maskCode blanks every inline code span and returns the spans. A span opens
// with a run of backticks and closes at the next run of the same length; it
// cannot cross a blank line.
func maskCode(s string) (string, []codeSpan) {
	b := []byte(s)
	var spans []codeSpan
	for i := 0; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] == '`' {
			j++
		}
		n := j - i
		end := -1
		for k := j; k < len(s); {
			if s[k] != '`' {
				k++
				continue
			}
			m := k
			for m < len(s) && s[m] == '`' {
				m++
			}
			if m-k == n {
				end = k
				break
			}
			k = m
		}
		if end < 0 || blankGapRe.MatchString(s[j:end]) {
			i = j
			continue
		}
		spans = append(spans, codeSpan{off: i, content: s[j:end]})
		blank(b, i, end+n)
		i = end + n
	}
	return string(b), spans
}

// trimBare drops trailing sentence punctuation from a bare URL, and a closing
// parenthesis that has no opener inside the URL.
func trimBare(u string) string {
	for u != "" {
		last := u[len(u)-1]
		switch {
		case strings.IndexByte(".,;:!?", last) >= 0:
		case last == ')' && strings.Count(u, ")") > strings.Count(u, "("):
		default:
			return u
		}
		u = u[:len(u)-1]
	}
	return u
}
