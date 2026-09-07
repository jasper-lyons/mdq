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
  mdq section FILE HEADING|LINE [flags]     print one section
  mdq grep FILE|DIR PATTERN [-i]            search, each hit tagged with its section
  mdq tasks FILE|DIR [flags]                list task items
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

HEADING matching is case insensitive and ignores leading hashes, emoji and
trailing status text. A unique prefix works, so does /a regex/, so does a line
number. Several matches print the candidates and exit 2.
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
		return Ambiguous, fmt.Errorf("usage: mdq section FILE HEADING [--no-children] [--no-body]")
	}
	maxLines, err := a.num("max-lines", 200)
	if err != nil {
		return Ambiguous, err
	}
	file := a.pos[0]
	query := strings.Join(a.pos[1:], " ")
	doc, err := md.ParseFile(file)
	if err != nil {
		return NoMatch, err
	}
	m := doc.FindHeading(query)
	if len(m.Found) == 0 {
		fmt.Fprintf(out, "%s: no heading matches %q\n", doc.File, query)
		return NoMatch, nil
	}
	if m.Ambiguous {
		fmt.Fprintf(out, "%s: %d headings match %q, pick one\n", doc.File, len(m.Found), query)
		for _, h := range m.Found {
			fmt.Fprintf(out, "%s:%d: %s\n", doc.File, h.Line, marked(h))
		}
		return Ambiguous, nil
	}

	h := m.Found[0]
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
		return OK, nil
	}

	body := doc.Body(h)
	if a.has("no-children") || a.str("children", "true") == "false" {
		body = doc.OwnBody(h)
	}
	printBody(out, body, maxLines)
	return OK, nil
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
	files, err := collect(a.pos[0])
	if err != nil {
		return NoMatch, err
	}
	hits := 0
	for _, f := range files {
		doc, err := md.ParseFile(f)
		if err != nil {
			return NoMatch, err
		}
		var found []int
		for i, line := range doc.Lines {
			if re.MatchString(line) {
				found = append(found, i+1)
			}
		}
		if len(found) == 0 {
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
		return NoMatch, nil
	}
	return OK, nil
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
