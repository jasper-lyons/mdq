package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jasperlyons/mdq/internal/md"
)

// Exit codes. 0 is success, 1 is no match, 2 is an ambiguous match or a
// usage error.
const (
	OK        = 0
	NoMatch   = 1
	Ambiguous = 2
)

const usage = `mdq - query markdown without reading the whole file

  mdq tree FILE [--depth N]                 outline: headings, line ranges, sizes
  mdq toc FILE [--depth N]                  headings and line numbers only
  mdq section FILE HEADING|LINE... [flags]  print one or more sections
  mdq grep FILE|DIR PATTERN [-i]            search, each hit tagged with its section
  mdq tasks FILE|DIR [flags]                list task items
  mdq links FILE|DIR [--internal|--external] list links, each tagged with its section
  mdq help                                  this text

section flags:
  --no-children      stop at the first subheading
  --no-body          print the subsection outline instead of the prose
  --max-lines N      truncate the body, default 200

tasks flags:
  --open             open items only, the default
  --done             completed items only
  --all              both
  --tag T            only items carrying #T
  --section H        only items under the heading H
  --no-bullets       drop bold lead bullets, keeping checkbox and heading items
  --json             machine readable output
  --width N          truncate item text, default 160, 0 for no limit

global:
  --max-lines N      cap any printed body, default 200

grep PATTERN is a Go regular expression, so alternation is A|B. A \| is a
literal pipe, not alternation as it would be in GNU grep, and mdq rewrites it
to A|B when nothing else in the pattern is regex syntax.

HEADING matching is case insensitive and ignores leading hashes, emoji and
trailing status text. A unique prefix works, so does /a regex/, so does a line
number. Several matches print the candidates and exit 2.

section takes several HEADING arguments and prints each one in the order given,
each under its own FILE:LINE: header. --max-lines applies per section. A heading
that does not resolve reports itself and the rest still print.
`

// Run executes one mdq invocation and returns the process exit code.
func Run(argv []string, out, errw io.Writer) int {
	if len(argv) == 0 {
		fmt.Fprint(errw, usage)
		return Ambiguous
	}
	cmd, rest := argv[0], argv[1:]
	var err error
	var code int
	switch cmd {
	case "tree":
		code, err = cmdTree(rest, out, false)
	case "toc":
		code, err = cmdTree(rest, out, true)
	case "section":
		code, err = cmdSection(rest, out)
	case "grep":
		code, err = cmdGrep(rest, out)
	case "tasks":
		code, err = cmdTasks(rest, out)
	case "links":
		code, err = cmdLinks(rest, out)
	case "help", "-h", "--help":
		fmt.Fprint(out, usage)
		return OK
	default:
		fmt.Fprintf(errw, "mdq: unknown command %q\n\n%s", cmd, usage)
		return Ambiguous
	}
	if err != nil {
		fmt.Fprintf(errw, "mdq: %v\n", err)
		if code == OK {
			code = Ambiguous
		}
	}
	return code
}

func cmdTree(argv []string, out io.Writer, tocOnly bool) (int, error) {
	a, err := parseArgs(argv, flagSet("depth", "max-lines"), flagSet("depth", "max-lines"))
	if err != nil {
		return Ambiguous, err
	}
	if len(a.pos) != 1 {
		return Ambiguous, fmt.Errorf("usage: mdq tree FILE [--depth N]")
	}
	depth, err := a.num("depth", 0)
	if err != nil {
		return Ambiguous, err
	}
	doc, err := md.ParseFile(a.pos[0])
	if err != nil {
		return NoMatch, err
	}
	if tocOnly {
		for _, h := range doc.Headings {
			if depth > 0 && h.Level > depth {
				continue
			}
			fmt.Fprintf(out, "%s:%d: %s\n", doc.File, h.Line, marked(h))
		}
		return OK, nil
	}
	fmt.Fprintf(out, "%s  %d lines  %s tokens  %d headings\n",
		doc.File, len(doc.Lines), tokens(total(doc)), len(doc.Headings))
	rows := make([]md.Heading, 0, len(doc.Headings))
	for _, h := range doc.Headings {
		if depth > 0 && h.Level > depth {
			continue
		}
		rows = append(rows, h)
	}
	rw, tw := 0, 0
	for _, h := range rows {
		rw = max(rw, len(rangeOf(h)))
		tw = max(tw, len(tokens(h.Tokens)))
	}
	for _, h := range rows {
		indent := ""
		if h.Level > 1 {
			indent = strings.Repeat("  ", h.Level-1)
		}
		fmt.Fprintf(out, "%*s  %*s  %s%s\n", rw, rangeOf(h), tw, tokens(h.Tokens), indent, marked(h))
	}
	return OK, nil
}

func cmdSection(argv []string, out io.Writer) (int, error) {
	known := flagSet("children", "no-children", "no-body", "body", "max-lines")
	a, err := parseArgs(argv, known, flagSet("max-lines"))
	if err != nil {
		return Ambiguous, err
	}
	if len(a.pos) < 2 {
		return Ambiguous, fmt.Errorf("usage: mdq section FILE HEADING [HEADING...] [--no-children] [--no-body]")
	}
	maxLines, err := a.num("max-lines", 200)
	if err != nil {
		return Ambiguous, err
	}
	file := a.pos[0]
	doc, err := md.ParseFile(file)
	if err != nil {
		return NoMatch, err
	}

	resolved, ambiguous, missing := 0, 0, 0
	for i, query := range sectionQueries(doc, a.pos[1:]) {
		if i > 0 {
			fmt.Fprintln(out)
		}
		m := doc.FindHeading(query)
		switch {
		case len(m.Found) == 0:
			missing++
			reportNoHeading(out, doc, query)
		case m.Ambiguous:
			ambiguous++
			fmt.Fprintf(out, "%s: %d headings match %q, pick one\n", doc.File, len(m.Found), query)
			for _, h := range m.Found {
				fmt.Fprintf(out, "%s:%d: %s\n", doc.File, h.Line, marked(h))
			}
		default:
			resolved++
			printSection(out, doc, m.Found[0], a, maxLines)
		}
	}

	switch {
	case missing == 0 && ambiguous == 0:
		return OK, nil
	case resolved == 0 && ambiguous == 0:
		return NoMatch, nil
	default:
		return Ambiguous, nil
	}
}

// sectionQueries reads every positional argument as a heading of its own. An
// unquoted multi word heading arrives as several arguments, so when at least
// one argument fails to resolve on its own and the joined form resolves to a
// single heading, the joined form is used instead.
func sectionQueries(doc *md.Document, args []string) []string {
	if len(args) < 2 {
		return args
	}
	for _, q := range args {
		if len(doc.FindHeading(q).Found) != 1 {
			joined := strings.Join(args, " ")
			if len(doc.FindHeading(joined).Found) == 1 {
				return []string{joined}
			}
			return args
		}
	}
	return args
}

func printSection(out io.Writer, doc *md.Document, h md.Heading, a argSet, maxLines int) {
	if !h.Pseudo {
		fmt.Fprintf(out, "%s:%d: %s\n", doc.File, h.Line, h.Raw)
	} else {
		fmt.Fprintf(out, "%s:%d: (preamble)\n", doc.File, h.Line)
	}

	if a.has("no-body") {
		kids := doc.Children(h)
		rw, tw := 0, 0
		for _, c := range kids {
			rw = max(rw, len(rangeOf(c)))
			tw = max(tw, len(tokens(c.Tokens)))
		}
		for _, c := range kids {
			indent := strings.Repeat("  ", c.Level-h.Level)
			fmt.Fprintf(out, "%*s  %*s  %s%s\n", rw, rangeOf(c), tw, tokens(c.Tokens), indent, marked(c))
		}
		fmt.Fprintf(out, "(section %s, %d lines, %s tokens, body not printed)\n",
			rangeOf(h), h.End-h.Line+1, tokens(h.Tokens))
		return
	}

	body := doc.Body(h)
	if a.has("no-children") || a.str("children", "true") == "false" {
		body = doc.OwnBody(h)
	}
	printBody(out, body, maxLines)
}

// reportNoHeading says what was looked for, how big the haystack was and which
// headings come closest, so a miss is an answer rather than a bare exit code.
func reportNoHeading(out io.Writer, doc *md.Document, query string) {
	fmt.Fprintf(out, "no heading matching '%s' in %s (%s)\n",
		query, doc.File, plural(len(doc.Headings), "heading"))
	near := doc.Nearest(query, 3)
	labels := make([]string, len(near))
	for i, h := range near {
		labels[i] = fmt.Sprintf("%q :%d", clip(h.Text, 60), h.Line)
	}
	printClosest(out, labels)
	fmt.Fprintf(out, "try a shorter heading or `mdq toc %s`\n", doc.File)
}

// printClosest writes the closest headings block, continuation lines aligned
// under the first one.
func printClosest(out io.Writer, labels []string) {
	const lead = "closest headings: "
	for i, l := range labels {
		if i == 0 {
			fmt.Fprint(out, lead)
		} else {
			fmt.Fprint(out, strings.Repeat(" ", len(lead)))
		}
		fmt.Fprintln(out, l)
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func printBody(out io.Writer, body []string, maxLines int) {
	shown := body
	if maxLines > 0 && len(body) > maxLines {
		shown = body[:maxLines]
	}
	for _, l := range shown {
		fmt.Fprintln(out, l)
	}
	if len(shown) < len(body) {
		fmt.Fprintf(out, "... (%d more lines, use --max-lines or a narrower heading)\n", len(body)-len(shown))
	}
}

func cmdGrep(argv []string, out io.Writer) (int, error) {
	known := flagSet("i", "ignore-case", "width", "max-lines")
	a, err := parseArgs(argv, known, flagSet("width", "max-lines"))
	if err != nil {
		return Ambiguous, err
	}
	if len(a.pos) != 2 {
		return Ambiguous, fmt.Errorf("usage: mdq grep FILE|DIR PATTERN")
	}
	width, err := a.num("width", 200)
	if err != nil {
		return Ambiguous, err
	}
	re, err := compile(a.pos[1], a.has("i") || a.has("ignore-case"))
	if err != nil {
		return Ambiguous, err
	}
	target := a.pos[0]
	pattern := a.pos[1]
	info, err := os.Stat(target)
	if err != nil {
		return NoMatch, err
	}
	files, err := collect(target)
	if err != nil {
		return NoMatch, err
	}
	words := md.QueryWords(pattern)
	scope := grepScope{dir: info.IsDir()}
	var near []nearHit
	hits := 0
	for _, f := range files {
		doc, err := md.ParseFile(f)
		if err != nil {
			return NoMatch, err
		}
		scope.files++
		scope.lines += len(doc.Lines)
		scope.headings += len(doc.Headings)
		var found []int
		for i, line := range doc.Lines {
			if re.MatchString(line) {
				found = append(found, i+1)
			}
		}
		if len(found) == 0 {
			// Only worth ranking while the search is still empty overall.
			if hits == 0 && len(words) > 0 {
				for _, h := range doc.Headings {
					if s := md.HeadingScore(h, words); s > 0 {
						near = append(near, nearHit{doc.File, h, s})
					}
				}
			}
			continue
		}
		hits += len(found)
		// One header per file, then bare line numbers, so a long path is
		// paid for once rather than on every hit.
		fmt.Fprintf(out, "%s  %d hits\n", doc.File, len(found))
		lw := 0
		for _, n := range found {
			lw = max(lw, len(strconv.Itoa(n)))
		}
		for _, n := range found {
			fmt.Fprintf(out, "%*d [%s]: %s\n", lw, n, where(doc, n), clip(strings.TrimSpace(doc.Lines[n-1]), width))
		}
	}
	if hits == 0 {
		reportNoGrepMatch(out, target, pattern, scope, near)
		return NoMatch, nil
	}
	return OK, nil
}

// grepScope is how much was searched, so a miss can say whether the pattern was
// too narrow or the haystack too small.
type grepScope struct {
	dir      bool
	files    int
	lines    int
	headings int
}

type nearHit struct {
	file  string
	h     md.Heading
	score int
}

func reportNoGrepMatch(out io.Writer, target, pattern string, scope grepScope, near []nearHit) {
	size := fmt.Sprintf("%s, %s", plural(scope.lines, "line"), plural(scope.headings, "heading"))
	if scope.dir {
		size = fmt.Sprintf("%s, %s", plural(scope.files, "file"), plural(scope.headings, "heading"))
	}
	fmt.Fprintf(out, "no match for /%s/ in %s (%s)\n", pattern, target, size)
	// Only worth saying when the pattern was left as written. Once the rewrite
	// has run, mdq did read the \| as alternation and the note would be wrong.
	if strings.Contains(pattern, `\|`) && md.FixAlternation(pattern) == pattern {
		fmt.Fprintf(out, "note: \\| is a literal pipe here, use A|B for alternation\n")
	}

	sort.SliceStable(near, func(i, j int) bool {
		if near[i].score != near[j].score {
			return near[i].score > near[j].score
		}
		if near[i].file != near[j].file {
			return near[i].file < near[j].file
		}
		return near[i].h.Line < near[j].h.Line
	})
	if len(near) > 3 {
		near = near[:3]
	}
	labels := make([]string, len(near))
	for i, n := range near {
		if scope.dir {
			labels[i] = fmt.Sprintf("%q %s:%d", clip(n.h.Text, 60), n.file, n.h.Line)
		} else {
			labels[i] = fmt.Sprintf("%q :%d", clip(n.h.Text, 60), n.h.Line)
		}
	}
	printClosest(out, labels)

	if scope.dir {
		fmt.Fprintf(out, "try a shorter pattern or one file\n")
		return
	}
	fmt.Fprintf(out, "try a shorter pattern or `mdq toc %s`\n", target)
}

func where(doc *md.Document, line int) string {
	h, ok := doc.HeadingAt(line)
	if !ok {
		return "(preamble)"
	}
	parts := doc.Path(h)
	// Two levels is enough context to place a hit, and keeps the line short.
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}
	for i := range parts {
		parts[i] = clip(parts[i], 44)
	}
	return strings.Join(parts, " > ")
}

func collect(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}
	var files []string
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != path && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor") {
				return fs.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(name)) {
		case ".md", ".markdown", ".mdx":
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func total(d *md.Document) int {
	chars := 0
	for _, l := range d.Lines {
		chars += len([]rune(l)) + 1
	}
	return chars / 4
}

func rangeOf(h md.Heading) string {
	return fmt.Sprintf("%d-%d", h.Line, h.End)
}

func marked(h md.Heading) string {
	if h.Pseudo {
		return "(preamble)"
	}
	return strings.Repeat("#", h.Level) + " " + h.Text
}

func tokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("~%d", n)
	}
	return fmt.Sprintf("~%.1fk", float64(n)/1000)
}

func clip(s string, width int) string {
	if width <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width-3]) + "..."
}
