// Package md parses a markdown file into headings, sections and task items.
// It handles the GFM shapes that show up in plans, specs and todo files:
// ATX and setext headings, fenced code blocks, YAML front matter,
// checkbox task items and hashtags.
package md

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Heading is one heading and the extent of the section it opens.
type Heading struct {
	Level  int    // 1 to 6, or 0 for the synthetic preamble entry
	Text   string // heading text with markers and trailing hashes removed
	Raw    string // the source line as written
	Line   int    // 1 based line of the heading itself
	End    int    // 1 based last non blank line of the section, subsections included
	Tokens int    // approximate token count of Line..End, runes over 4
	Parent int    // index into Document.Headings, or -1
	Index  int    // own index into Document.Headings
	Pseudo bool   // true for the preamble entry, which has no heading line
}

// Document is a parsed markdown file.
type Document struct {
	File     string
	Lines    []string
	Headings []Heading
	Fenced   []bool // per line, true when the line sits inside a fenced code block
	FMEnd    int    // 1 based last line of YAML front matter, 0 when there is none
}

var (
	atxRe        = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?[ \t]*$`)
	fenceRe      = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
	setextRe     = regexp.MustCompile(`^ {0,3}(=+|-+)[ \t]*$`)
	thematicRe   = regexp.MustCompile(`^ {0,3}((\*[ \t]*){3,}|(-[ \t]*){3,}|(_[ \t]*){3,})$`)
	listStartRe  = regexp.MustCompile(`^[ \t]*([-*+]|\d+[.)])([ \t]|$)`)
	trailHashRe  = regexp.MustCompile(`[ \t]+#+$`)
	blockquoteRe = regexp.MustCompile(`^ {0,3}>`)
)

// ParseFile reads and parses a markdown file.
func ParseFile(path string) (*Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(path, f)
}

// Parse parses markdown read from r. file is used only for output prefixes.
func Parse(file string, r io.Reader) (*Document, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var lines []string
	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return New(file, lines), nil
}

// New builds a Document from lines already in memory.
func New(file string, lines []string) *Document {
	d := &Document{File: file, Lines: lines, Fenced: make([]bool, len(lines))}
	d.markFrontMatter()
	d.markFences()
	d.collectHeadings()
	d.computeExtents()
	return d
}

func (d *Document) markFrontMatter() {
	if len(d.Lines) == 0 || strings.TrimSpace(d.Lines[0]) != "---" {
		return
	}
	limit := len(d.Lines)
	if limit > 200 {
		limit = 200
	}
	for i := 1; i < limit; i++ {
		t := strings.TrimSpace(d.Lines[i])
		if t == "---" || t == "..." {
			d.FMEnd = i + 1
			return
		}
	}
}

func (d *Document) markFences() {
	open := ""
	for i, line := range d.Lines {
		if i+1 <= d.FMEnd {
			continue
		}
		m := fenceRe.FindStringSubmatch(line)
		if open == "" {
			if m == nil {
				continue
			}
			// A backtick fence cannot carry a backtick in its info string.
			if m[1][0] == '`' && strings.Contains(m[2], "`") {
				continue
			}
			open = m[1]
			d.Fenced[i] = true
			continue
		}
		d.Fenced[i] = true
		if m != nil && m[1][0] == open[0] && len(m[1]) >= len(open) && strings.TrimSpace(m[2]) == "" {
			open = ""
		}
	}
}

func (d *Document) collectHeadings() {
	consumed := -1
	for i, line := range d.Lines {
		if i+1 <= d.FMEnd || d.Fenced[i] {
			continue
		}
		if m := atxRe.FindStringSubmatch(line); m != nil {
			text := strings.TrimSpace(trailHashRe.ReplaceAllString(m[2], ""))
			d.add(len(m[1]), text, line, i+1)
			consumed = i
			continue
		}
		if m := setextRe.FindStringSubmatch(line); m != nil && i > 0 && i-1 != consumed {
			prev := d.Lines[i-1]
			if d.setextCandidate(i-1, prev) {
				level := 1
				if m[1][0] == '-' {
					level = 2
				}
				d.add(level, strings.TrimSpace(prev), prev, i)
				consumed = i
			}
		}
	}
}

func (d *Document) setextCandidate(idx int, line string) bool {
	if idx+1 <= d.FMEnd || d.Fenced[idx] {
		return false
	}
	if strings.TrimSpace(line) == "" {
		return false
	}
	if atxRe.MatchString(line) || thematicRe.MatchString(line) || setextRe.MatchString(line) {
		return false
	}
	if listStartRe.MatchString(line) || blockquoteRe.MatchString(line) {
		return false
	}
	if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
		return false
	}
	return true
}

func (d *Document) add(level int, text, raw string, line int) {
	d.Headings = append(d.Headings, Heading{
		Level: level,
		Text:  text,
		Raw:   raw,
		Line:  line,
		Index: len(d.Headings),
	})
}

func (d *Document) computeExtents() {
	n := len(d.Lines)
	for i := range d.Headings {
		h := &d.Headings[i]
		end := n
		for j := i + 1; j < len(d.Headings); j++ {
			if d.Headings[j].Level <= h.Level {
				end = d.Headings[j].Line - 1
				break
			}
		}
		h.End = d.trimBlank(h.Line, end)
		h.Tokens = d.tokens(h.Line, h.End)
		h.Parent = -1
		for j := i - 1; j >= 0; j-- {
			if d.Headings[j].Level < h.Level {
				h.Parent = j
				break
			}
		}
	}
	d.addPreamble()
}

// addPreamble records any content that sits after the front matter and before
// the first heading, so a tree call still shows it.
func (d *Document) addPreamble() {
	start := d.FMEnd + 1
	end := len(d.Lines)
	if len(d.Headings) > 0 {
		end = d.Headings[0].Line - 1
	}
	if start > end {
		return
	}
	if d.trimBlank(start, end) < start {
		return
	}
	end = d.trimBlank(start, end)
	p := Heading{
		Level:  0,
		Text:   "(preamble)",
		Raw:    "(preamble)",
		Line:   start,
		End:    end,
		Tokens: d.tokens(start, end),
		Parent: -1,
		Pseudo: true,
	}
	d.Headings = append([]Heading{p}, d.Headings...)
	for i := range d.Headings {
		d.Headings[i].Index = i
		if i > 0 && d.Headings[i].Parent >= 0 {
			d.Headings[i].Parent++
		}
	}
}

// trimBlank returns the last non blank line in start..end, or start-1 when the
// whole range is blank. Both bounds are 1 based and inclusive.
func (d *Document) trimBlank(start, end int) int {
	if end > len(d.Lines) {
		end = len(d.Lines)
	}
	for e := end; e >= start; e-- {
		if strings.TrimSpace(d.Lines[e-1]) != "" {
			return e
		}
	}
	return start - 1
}

func (d *Document) tokens(start, end int) int {
	if end < start {
		return 0
	}
	if end > len(d.Lines) {
		end = len(d.Lines)
	}
	chars := 0
	for i := start; i <= end; i++ {
		chars += utf8.RuneCountInString(d.Lines[i-1]) + 1
	}
	return chars / 4
}

// Body returns the section lines under a heading, the heading line excluded.
func (d *Document) Body(h Heading) []string {
	start := h.Line + 1
	if h.Pseudo {
		start = h.Line
	}
	if start > h.End {
		return nil
	}
	return d.Lines[start-1 : h.End]
}

// OwnBody returns the lines under a heading up to its first subheading.
func (d *Document) OwnBody(h Heading) []string {
	start := h.Line + 1
	if h.Pseudo {
		start = h.Line
	}
	end := h.End
	for j := h.Index + 1; j < len(d.Headings); j++ {
		if d.Headings[j].Line > h.End {
			break
		}
		end = d.trimBlank(start, d.Headings[j].Line-1)
		break
	}
	if start > end {
		return nil
	}
	return d.Lines[start-1 : end]
}

// Children returns every heading nested under h, at any depth.
func (d *Document) Children(h Heading) []Heading {
	var out []Heading
	for j := h.Index + 1; j < len(d.Headings); j++ {
		c := d.Headings[j]
		if c.Level <= h.Level && !c.Pseudo {
			break
		}
		if c.Line > h.End {
			break
		}
		out = append(out, c)
	}
	return out
}

// Path returns the heading texts from the outermost ancestor down to h.
func (d *Document) Path(h Heading) []string {
	var out []string
	for i := h.Index; i >= 0; {
		out = append([]string{d.Headings[i].Text}, out...)
		i = d.Headings[i].Parent
	}
	return out
}

// HeadingAt returns the innermost heading whose section contains a 1 based
// line, and false when the line sits above every heading.
func (d *Document) HeadingAt(line int) (Heading, bool) {
	best := -1
	for i, h := range d.Headings {
		if h.Line <= line && line <= h.End {
			if best < 0 || h.Level > d.Headings[best].Level {
				best = i
			}
		}
	}
	if best < 0 {
		return Heading{}, false
	}
	return d.Headings[best], true
}

// StripLeadingSymbols removes leading runes that are neither letters nor
// digits, which is how status emoji and bullet glyphs are dropped.
func StripLeadingSymbols(s string) string {
	for i, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return s[i:]
		}
	}
	return ""
}
