package md

import (
	"strings"
	"testing"
)

const fixture = "../../testdata/sample.md"

func load(t *testing.T) *Document {
	t.Helper()
	d, err := ParseFile(fixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return d
}

func TestFrontMatterIsNotContent(t *testing.T) {
	d := load(t)
	if d.FMEnd != 4 {
		t.Fatalf("front matter ends at %d, want 4", d.FMEnd)
	}
	for _, h := range d.Headings {
		if h.Line <= 4 {
			t.Fatalf("heading found inside front matter at line %d", h.Line)
		}
	}
}

func TestPreambleIsRecorded(t *testing.T) {
	d := load(t)
	if len(d.Headings) == 0 || !d.Headings[0].Pseudo {
		t.Fatal("expected a preamble entry first")
	}
	p := d.Headings[0]
	if p.Line != 5 || p.End != 6 {
		t.Fatalf("preamble is %d-%d, want 5-6", p.Line, p.End)
	}
}

func TestHeadingLevelsAndExtents(t *testing.T) {
	d := load(t)
	want := []struct {
		text  string
		level int
		line  int
		end   int
	}{
		{"Sample Doc", 1, 8, 50},
		{"Setext Heading", 2, 12, 15},
		{"Plan", 2, 17, 23},
		{"Code", 2, 25, 34},
		{"Ambiguous Two", 2, 44, 50},
		{"Deep", 3, 48, 50},
	}
	for _, w := range want {
		h, ok := find(d, w.text)
		if !ok {
			t.Fatalf("heading %q not found", w.text)
		}
		if h.Level != w.level || h.Line != w.line || h.End != w.end {
			t.Fatalf("heading %q is level %d at %d-%d, want level %d at %d-%d",
				w.text, h.Level, h.Line, h.End, w.level, w.line, w.end)
		}
	}
}

func TestHashInsideAFenceIsNotAHeading(t *testing.T) {
	d := load(t)
	for _, h := range d.Headings {
		if strings.Contains(h.Text, "not a heading") {
			t.Fatal("a fenced line was read as a heading")
		}
	}
	if !d.Fenced[28] {
		t.Fatal("line 29 should be inside a fence")
	}
}

func TestBodyAndOwnBody(t *testing.T) {
	d := load(t)
	h, _ := find(d, "Ambiguous Two")
	if got := len(d.Body(h)); got != 6 {
		t.Fatalf("body is %d lines, want 6", got)
	}
	if got := len(d.OwnBody(h)); got != 2 {
		t.Fatalf("own body is %d lines, want 2", got)
	}
}

func TestPathAndHeadingAt(t *testing.T) {
	d := load(t)
	h, ok := d.HeadingAt(49)
	if !ok || h.Text != "Deep" {
		t.Fatalf("line 49 sits in %q, want Deep", h.Text)
	}
	got := strings.Join(d.Path(h), " > ")
	if got != "Sample Doc > Ambiguous Two > Deep" {
		t.Fatalf("path is %q", got)
	}
}

func TestTokensAreApproximate(t *testing.T) {
	d := load(t)
	h, _ := find(d, "Plan")
	if h.Tokens < 10 || h.Tokens > 60 {
		t.Fatalf("Plan is ~%d tokens, expected a small number", h.Tokens)
	}
}

func find(d *Document, text string) (Heading, bool) {
	for _, h := range d.Headings {
		if h.Text == text {
			return h, true
		}
	}
	return Heading{}, false
}
