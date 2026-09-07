package md

import (
	"regexp"
	"strings"
)

// Task is one task item found in a document.
type Task struct {
	File    string   `json:"file"`
	Line    int      `json:"line"`
	Done    bool     `json:"done"`
	Text    string   `json:"text"`
	Tags    []string `json:"tags,omitempty"`
	Section string   `json:"section,omitempty"`
	Path    []string `json:"path,omitempty"`
	Parent  string   `json:"parent,omitempty"`
	Kind    string   `json:"kind"` // checkbox, heading or bullet
}

var (
	checkboxRe  = regexp.MustCompile(`^([ \t]*)(?:[-*+]|\d+[.)])[ \t]+\[([ xX/~<>!?-])\][ \t]*(.*)$`)
	boldLeadRe  = regexp.MustCompile(`^([ \t]*)[-*+][ \t]+\*\*(.+)$`)
	tagRe       = regexp.MustCompile(`(^|\s)#([A-Za-z][A-Za-z0-9_/-]*)`)
	upperStatRe = regexp.MustCompile(`\b(DONE|CLOSED|CLEAR|FIXED|RESOLVED|WONTFIX|SUPERSEDED|CANCELLED|CANCELED|SHIPPED)\b`)
	upperOpenRe = regexp.MustCompile(`\b(TODO|WIP|BLOCKED|IN PROGRESS|PENDING)\b`)
	boldDoneRe  = regexp.MustCompile(`\*\*(FIXED|DONE|CLOSED|RESOLVED|SHIPPED|CLEAR)\b`)
)

var (
	doneGlyphs = []string{"✅", "✔", "☑", "✓", "✗", "❌", "\U0001F7E2", "✖"}
	openGlyphs = []string{"⬜", "\U0001F6A7", "⏳", "\U0001F534", "\U0001F7E1", "\U0001F7E0", "▶", "\U0001F51C", "☐"}
)

// TaskOptions controls which items count as tasks.
type TaskOptions struct {
	Bullets bool // also treat bold lead bullets as tasks
}

// Status reports whether a heading or bullet carries a status marker, and
// whether that marker means done. Text with no marker is not a task.
func Status(text string) (isTask, done bool) {
	t := strings.TrimSpace(text)
	if t == "" {
		return false, false
	}
	for _, g := range doneGlyphs {
		if strings.HasPrefix(t, g) {
			return true, true
		}
	}
	for _, g := range openGlyphs {
		if strings.HasPrefix(t, g) {
			return true, false
		}
	}
	if strings.HasPrefix(t, "~~") {
		return true, true
	}
	if upperStatRe.MatchString(t) {
		return true, true
	}
	if upperOpenRe.MatchString(t) {
		return true, false
	}
	// A trailing segment such as "- resolved: ..." after a separator counts.
	n := Normalize(t)
	if seps := sepRe.FindAllStringIndex(n, -1); seps != nil {
		for i := len(seps) - 1; i >= 0; i-- {
			tail := n[seps[i][1]:]
			if statusTailRe.MatchString(tail) {
				return true, !isOpenWord(tail)
			}
		}
	}
	return false, false
}

func isOpenWord(tail string) bool {
	for _, w := range []string{"todo", "wip", "open", "blocked", "deferred", "in progress"} {
		if strings.HasPrefix(tail, w) {
			return true
		}
	}
	return false
}

// Tasks returns every task item in the document, in line order.
func (d *Document) Tasks(opt TaskOptions) []Task {
	var out []Task
	type frame struct {
		indent int
		text   string
	}
	var stack []frame

	headingTask := map[int]bool{}
	for _, h := range d.Headings {
		// Level 1 is the document title in every file shape seen so far, so
		// only level 2 and below can carry a task marker.
		if h.Pseudo || h.Level < 2 {
			continue
		}
		isTask, done := Status(h.Text)
		if !isTask {
			continue
		}
		headingTask[h.Line] = true
		path := d.Path(h)
		section := ""
		if len(path) > 1 {
			section = path[len(path)-2]
		}
		out = append(out, Task{
			File:    d.File,
			Line:    h.Line,
			Done:    done,
			Text:    h.Text,
			Tags:    tags(h.Text),
			Section: section,
			Path:    path[:len(path)-1],
			Kind:    "heading",
		})
	}

	for i, line := range d.Lines {
		if i+1 <= d.FMEnd || d.Fenced[i] {
			continue
		}
		m := checkboxRe.FindStringSubmatch(line)
		kind := "checkbox"
		var indent int
		var text string
		var done bool
		switch {
		case m != nil:
			indent = width(m[1])
			done = m[2] == "x" || m[2] == "X" || m[2] == "-" || m[2] == "~"
			text = strings.TrimSpace(m[3])
		case opt.Bullets:
			b := boldLeadRe.FindStringSubmatch(line)
			if b == nil {
				continue
			}
			kind = "bullet"
			indent = width(b[1])
			text = strings.TrimSpace("**" + b[2])
			_, done = Status(text)
			// A prose item carries its outcome in its body, as a bold
			// **FIXED** or **CLOSED** on a later line of the same item.
			if !done {
				done = d.bulletIsClosed(i, indent)
			}
		default:
			continue
		}

		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		parent := ""
		if len(stack) > 0 {
			parent = stack[len(stack)-1].text
		}
		stack = append(stack, frame{indent: indent, text: text})

		var path []string
		section := ""
		if h, ok := d.HeadingAt(i + 1); ok {
			path = d.Path(h)
			section = h.Text
		}
		out = append(out, Task{
			File:    d.File,
			Line:    i + 1,
			Done:    done,
			Text:    text,
			Tags:    tags(text),
			Section: section,
			Path:    path,
			Parent:  parent,
			Kind:    kind,
		})
	}

	sortTasks(out)
	return out
}

// bulletIsClosed looks through the rest of a bullet item, the lines after its
// first, for a bold status marker such as **FIXED**.
func (d *Document) bulletIsClosed(start, indent int) bool {
	blanks := 0
	for i := start + 1; i < len(d.Lines) && i < start+60; i++ {
		line := d.Lines[i]
		if strings.TrimSpace(line) == "" {
			blanks++
			if blanks >= 2 {
				return false
			}
			continue
		}
		blanks = 0
		if atxRe.MatchString(line) || d.Fenced[i] {
			return false
		}
		if m := listStartRe.FindStringSubmatch(line); m != nil {
			if width(line[:len(line)-len(strings.TrimLeft(line, " \t"))]) <= indent {
				return false
			}
		}
		if boldDoneRe.MatchString(line) {
			return true
		}
	}
	return false
}

func sortTasks(t []Task) {
	for i := 1; i < len(t); i++ {
		for j := i; j > 0 && t[j].Line < t[j-1].Line; j-- {
			t[j], t[j-1] = t[j-1], t[j]
		}
	}
}

func width(indent string) int {
	n := 0
	for _, r := range indent {
		if r == '\t' {
			n += 4
		} else {
			n++
		}
	}
	return n
}

func tags(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range tagRe.FindAllStringSubmatch(text, -1) {
		if seen[m[2]] {
			continue
		}
		seen[m[2]] = true
		out = append(out, m[2])
	}
	return out
}
