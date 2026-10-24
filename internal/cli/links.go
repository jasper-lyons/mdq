package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/jasperlyons/mdq/internal/md"
)

func cmdLinks(argv []string, out io.Writer) (int, error) {
	known := flagSet("section", "json", "internal", "external", "bare", "width", "max-lines")
	a, err := parseArgs(argv, known, flagSet("section", "width", "max-lines"))
	if err != nil {
		return Ambiguous, err
	}
	if len(a.pos) != 1 {
		return Ambiguous, fmt.Errorf("usage: mdq links FILE|DIR [--internal|--external] [--bare]")
	}
	if a.has("internal") && a.has("external") {
		return Ambiguous, fmt.Errorf("--internal and --external cannot be combined")
	}
	width, err := a.num("width", 160)
	if err != nil {
		return Ambiguous, err
	}
	maxLines, err := a.num("max-lines", 200)
	if err != nil {
		return Ambiguous, err
	}
	files, err := collect(a.pos[0])
	if err != nil {
		return NoMatch, err
	}

	asJSON := a.has("json")
	picked := []md.Link{}
	budget := maxLines
	if maxLines <= 0 {
		budget = -1
	}
	left := 0
	count := 0
	for _, f := range files {
		doc, err := md.ParseFile(f)
		if err != nil {
			return NoMatch, err
		}
		var within []md.Heading
		if s := a.str("section", ""); s != "" {
			m := doc.FindHeading(s)
			if len(m.Found) == 0 {
				continue
			}
			if m.Ambiguous && len(files) == 1 {
				fmt.Fprintf(out, "%s: %d headings match %q, pick one\n", doc.File, len(m.Found), s)
				for _, h := range m.Found {
					fmt.Fprintf(out, "%s:%d: %s\n", doc.File, h.Line, marked(h))
				}
				return Ambiguous, nil
			}
			within = m.Found
		}
		var links []md.Link
		for _, l := range doc.LinksWith(md.LinkOptions{Bare: a.has("bare")}) {
			ext := md.IsExternal(l.Target)
			if (a.has("internal") && ext) || (a.has("external") && !ext) {
				continue
			}
			if within != nil && !linkInAny(l, within) {
				continue
			}
			links = append(links, l)
		}
		count += len(links)
		if asJSON {
			picked = append(picked, links...)
			continue
		}
		if len(links) == 0 {
			continue
		}
		if budget == 0 {
			left += len(links)
			continue
		}
		shown := links
		if budget > 0 && len(shown) > budget {
			left += len(shown) - budget
			shown = shown[:budget]
		}
		if budget > 0 {
			budget -= len(shown)
		}
		fmt.Fprintf(out, "%s  %d links\n", doc.File, len(links))
		lw := 0
		for _, l := range shown {
			lw = max(lw, len(strconv.Itoa(l.Line)))
		}
		for _, l := range shown {
			written := l.Target
			if l.Anchor != "" {
				written += "#" + l.Anchor
			}
			row := fmt.Sprintf("%*d [%s]: %s", lw, l.Line, where(doc, l.Line), clip(written, width))
			if !md.IsExternal(l.Target) {
				row += "  -> " + l.Path
				if l.Missing {
					row += " (missing)"
				}
			}
			fmt.Fprintln(out, row)
		}
	}

	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(picked); err != nil {
			return Ambiguous, err
		}
	} else if left > 0 {
		fmt.Fprintf(out, "... (%d more links, narrow with --section or --internal, or raise --max-lines)\n", left)
	}
	if count == 0 {
		return NoMatch, nil
	}
	return OK, nil
}

func linkInAny(l md.Link, within []md.Heading) bool {
	for _, h := range within {
		if l.Line >= h.Line && l.Line <= h.End {
			return true
		}
	}
	return false
}
