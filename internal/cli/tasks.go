package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/jasperlyons/mdq/internal/md"
)

func compile(pattern string, ignoreCase bool) (*regexp.Regexp, error) {
	pattern = md.FixAlternation(pattern)
	if ignoreCase {
		pattern = "(?i)" + pattern
	}
	return regexp.Compile(pattern)
}

func cmdTasks(argv []string, out io.Writer) (int, error) {
	known := flagSet("open", "done", "all", "tag", "section", "json", "bullets", "no-bullets", "width", "max-lines")
	a, err := parseArgs(argv, known, flagSet("tag", "section", "width", "max-lines"))
	if err != nil {
		return Ambiguous, err
	}
	if len(a.pos) != 1 {
		return Ambiguous, fmt.Errorf("usage: mdq tasks FILE|DIR [--open|--done|--all]")
	}
	width, err := a.num("width", 160)
	if err != nil {
		return Ambiguous, err
	}
	maxLines, err := a.num("max-lines", 200)
	if err != nil {
		return Ambiguous, err
	}
	wantOpen, wantDone := true, false
	switch {
	case a.has("all"):
		wantOpen, wantDone = true, true
	case a.has("done"):
		wantOpen, wantDone = false, true
	}
	tag := strings.TrimPrefix(a.str("tag", ""), "#")
	files, err := collect(a.pos[0])
	if err != nil {
		return NoMatch, err
	}

	var picked []md.Task
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
		// Bold lead bullets count by default. A query that silently drops
		// hundreds of items is wrong more often than one that is long.
		bullets := !a.has("no-bullets")
		for _, t := range doc.Tasks(md.TaskOptions{Bullets: bullets}) {
			if t.Done && !wantDone {
				continue
			}
			if !t.Done && !wantOpen {
				continue
			}
			if tag != "" && !hasTag(t.Tags, tag) {
				continue
			}
			if within != nil && !inAny(t, within) {
				continue
			}
			picked = append(picked, t)
		}
	}

	if a.has("json") {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if picked == nil {
			picked = []md.Task{}
		}
		if err := enc.Encode(picked); err != nil {
			return Ambiguous, err
		}
		if len(picked) == 0 {
			return NoMatch, nil
		}
		return OK, nil
	}

	printTasks(out, picked, width, maxLines)
	if len(picked) == 0 {
		return NoMatch, nil
	}
	return OK, nil
}

// printTasks writes one header per file and then one line per item, so a long
// absolute path is paid for once rather than on every line.
func printTasks(out io.Writer, tasks []md.Task, width, maxLines int) {
	budget := maxLines
	if maxLines <= 0 {
		budget = len(tasks)
	}
	left := 0
	for _, group := range groupByFile(tasks) {
		if budget <= 0 {
			left += len(group.items)
			continue
		}
		items := group.items
		if len(items) > budget {
			left += len(items) - budget
			items = items[:budget]
		}
		budget -= len(items)
		open, done := 0, 0
		for _, t := range group.items {
			if t.Done {
				done++
			} else {
				open++
			}
		}
		fmt.Fprintf(out, "%s  %d open, %d done\n", group.file, open, done)
		lw := 0
		for _, t := range items {
			lw = max(lw, len(strconv.Itoa(t.Line)))
		}
		for _, t := range items {
			mark := ""
			if t.Done {
				mark = "[x] "
			}
			section := t.Section
			if section == "" {
				section = "-"
			}
			line := fmt.Sprintf("%*d [%s] %s%s", lw, t.Line, clip(section, 48), mark, clip(t.Text, width))
			if t.Parent != "" {
				line += "  < " + clip(t.Parent, 60)
			}
			fmt.Fprintln(out, line)
		}
	}
	if left > 0 {
		fmt.Fprintf(out, "... (%d more items, narrow with --section or --tag, or raise --max-lines)\n", left)
	}
}

type taskGroup struct {
	file  string
	items []md.Task
}

func groupByFile(tasks []md.Task) []taskGroup {
	var groups []taskGroup
	for _, t := range tasks {
		if len(groups) == 0 || groups[len(groups)-1].file != t.File {
			groups = append(groups, taskGroup{file: t.File})
		}
		groups[len(groups)-1].items = append(groups[len(groups)-1].items, t)
	}
	return groups
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}

func inAny(t md.Task, within []md.Heading) bool {
	for _, h := range within {
		if t.Line >= h.Line && t.Line <= h.End {
			return true
		}
	}
	return false
}
