package md

import (
	"path/filepath"
	"testing"
)

func parseLinks(t *testing.T, file string, opt LinkOptions) []Link {
	t.Helper()
	doc, err := ParseFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return doc.LinksWith(opt)
}

// findLink returns the first link of the given kind and target.
func findLink(links []Link, kind, target string) (Link, bool) {
	for _, l := range links {
		if l.Kind == kind && l.Target == target {
			return l, true
		}
	}
	return Link{}, false
}

func TestLinksKinds(t *testing.T) {
	links := parseLinks(t, "../../testdata/links.md", LinkOptions{})
	for _, w := range []struct {
		kind, target string
		line         int
	}{
		{"inline", "sample.md", 7},
		{"inline", "https://example.com/a", 12},
		{"autolink", "https://auto.example.com", 12},
		{"autolink", "mailto:a@b.co", 12},
		{"wiki", "note", 14},
		{"wiki", "page", 14},
		{"code", "docs/b.md", 18},
		{"code", "missing.md", 18},
	} {
		l, ok := findLink(links, w.kind, w.target)
		if !ok {
			t.Errorf("no %s link to %q in %+v", w.kind, w.target, links)
			continue
		}
		if l.Line != w.line {
			t.Errorf("%s %q at line %d, want %d", w.kind, w.target, l.Line, w.line)
		}
	}
	if l, _ := findLink(links, "wiki", "page"); l.Text != "label" {
		t.Errorf("wiki label is %q", l.Text)
	}
}

func TestLinksSkipped(t *testing.T) {
	links := parseLinks(t, "../../testdata/links.md", LinkOptions{Bare: true})
	for _, l := range links {
		switch l.Target {
		case "fm.md", "fenced.md", "b.md", "img.png", "not a path.md", "plain":
			t.Errorf("%q should be skipped: %+v", l.Target, l)
		}
	}
}

func TestLinksSplitAcrossLines(t *testing.T) {
	links := parseLinks(t, "../../testdata/links.md", LinkOptions{})
	l, ok := findLink(links, "inline", "split.md")
	if !ok || l.Line != 27 || l.Text != "split text" {
		t.Fatalf("split text link is %+v", l)
	}
	if l, ok := findLink(links, "inline", "followups.md"); !ok || l.Line != 28 {
		t.Fatalf("split target link is %+v", l)
	}
}

func TestLinksBareOnlyWhenAsked(t *testing.T) {
	if _, ok := findLink(parseLinks(t, "../../testdata/links.md", LinkOptions{}), "bare", "https://bare.example.com/path"); ok {
		t.Fatal("bare URL reported without the option")
	}
	if _, ok := findLink(parseLinks(t, "../../testdata/links.md", LinkOptions{Bare: true}), "bare", "https://bare.example.com/path"); !ok {
		t.Fatal("bare URL missing, or its trailing comma was kept")
	}
}

func TestTrimBare(t *testing.T) {
	for in, want := range map[string]string{
		"https://a.b/c.":       "https://a.b/c",
		"https://a.b/c).":      "https://a.b/c",
		"https://a.b/c_(d)":    "https://a.b/c_(d)",
		"https://a.b/c_(d)),;": "https://a.b/c_(d)",
	} {
		if got := trimBare(in); got != want {
			t.Errorf("trimBare(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLinksAnchorsAndPaths(t *testing.T) {
	links := parseLinks(t, "../../testdata/links.md", LinkOptions{})
	l, _ := findLink(links, "inline", "decisions.md")
	if l.Anchor != "effective-kind" || filepath.Base(l.Path) != "decisions.md" || l.Missing {
		t.Errorf("anchored link is %+v", l)
	}
	l, _ = findLink(links, "inline", "")
	if l.Anchor != "details" || filepath.Base(l.Path) != "links.md" || l.Missing {
		t.Errorf("same file anchor is %+v", l)
	}
	l, _ = findLink(links, "inline", "https://example.com/a")
	if l.Path != "" || l.Anchor != "" {
		t.Errorf("external link should carry no path or anchor: %+v", l)
	}
	l, _ = findLink(links, "inline", "split.md")
	if !l.Missing || filepath.Base(l.Path) != "split.md" {
		t.Errorf("missing link is %+v", l)
	}
	if l.Section != "Split" || len(l.SectionPath) != 2 {
		t.Errorf("section is %q %v", l.Section, l.SectionPath)
	}
}

func TestLinksAncestorResolution(t *testing.T) {
	links := parseLinks(t, "../../testdata/linkrepo/docs/sub/a.md", LinkOptions{})
	l, ok := findLink(links, "code", "docs/b.md")
	if !ok || l.Missing || l.Path != filepath.FromSlash("../../testdata/linkrepo/docs/b.md") {
		t.Errorf("ancestor path is %+v", l)
	}
	l, ok = findLink(links, "code", "nope.md")
	if !ok || !l.Missing || l.Path != filepath.FromSlash("../../testdata/linkrepo/docs/sub/nope.md") {
		t.Errorf("missing code path is %+v", l)
	}
	if l, _ := findLink(links, "inline", "../b.md"); l.Missing || l.Anchor != "top" {
		t.Errorf("relative link is %+v", l)
	}
}
