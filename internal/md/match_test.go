package md

import (
	"strings"
	"testing"
)

func TestNormalizeAndCore(t *testing.T) {
	cases := []struct{ in, norm, core string }{
		{"## ✅ Deploy gate — CLEAR (opened 2026-08-04, closed 2026-08-05)", "deploy gate — clear (opened 2026-08-04, closed 2026-08-05)", "deploy gate"},
		{"### `sfListView` criteria leaf — DONE (ENG-320)", "sflistview criteria leaf — done (eng-320)", "sflistview criteria leaf"},
		{"Package: api", "package: api", "package: api"},
		{"~~Pull from Salesforce~~", "pull from salesforce", "pull from salesforce"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.norm {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.norm)
		}
		if got := Core(c.in); got != c.core {
			t.Errorf("Core(%q) = %q, want %q", c.in, got, c.core)
		}
	}
}

func TestFindHeadingTiers(t *testing.T) {
	d := load(t)
	if m := d.FindHeading("plan"); len(m.Found) != 1 || m.Found[0].Text != "Plan" {
		t.Fatalf("exact match failed: %+v", m.Found)
	}
	if m := d.FindHeading("Ambiguous O"); len(m.Found) != 1 || m.Found[0].Text != "Ambiguous One" {
		t.Fatalf("prefix match failed: %+v", m.Found)
	}
	if m := d.FindHeading("shipped thing"); len(m.Found) != 1 {
		t.Fatalf("trailing status text should be ignored: %+v", m.Found)
	}
	m := d.FindHeading("ambiguous")
	if !m.Ambiguous || len(m.Found) != 2 {
		t.Fatalf("expected two ambiguous matches, got %d", len(m.Found))
	}
	if m := d.FindHeading("/^deep$/"); len(m.Found) != 1 || m.Found[0].Text != "Deep" {
		t.Fatalf("regex match failed: %+v", m.Found)
	}
	if m := d.FindHeading("17"); len(m.Found) != 1 || m.Found[0].Text != "Plan" {
		t.Fatalf("line number match failed: %+v", m.Found)
	}
	if m := d.FindHeading("19"); len(m.Found) != 1 || m.Found[0].Text != "Plan" {
		t.Fatalf("line inside a section should resolve to it: %+v", m.Found)
	}
	if m := d.FindHeading("nothing like this"); len(m.Found) != 0 {
		t.Fatalf("expected no match, got %d", len(m.Found))
	}
}

func TestQueryWords(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Asset chat image swap", "asset chat image swap"},
		{"^## (Plan|Code)$", "plan code"},
		{`\bqueue\b writes`, "queue writes"},
		{"the and of a", ""},
		{"Plan plan PLAN", "plan"},
	}
	for _, c := range cases {
		if got := strings.Join(QueryWords(c.in), " "); got != c.want {
			t.Errorf("QueryWords(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNearestRanksBySharedWords(t *testing.T) {
	d := load(t)
	near := d.Nearest("Ambiguous Three", 3)
	if len(near) != 2 {
		t.Fatalf("expected the two ambiguous headings, got %+v", near)
	}
	if near[0].Text != "Ambiguous One" || near[1].Text != "Ambiguous Two" {
		t.Fatalf("ties should fall back to line order: %+v", near)
	}
	near = d.Nearest("deep plan", 3)
	if len(near) != 2 || near[0].Text != "Plan" {
		t.Fatalf("expected Plan and Deep in line order: %+v", near)
	}
	if got := d.Nearest("zzzqqq", 3); len(got) != 0 {
		t.Fatalf("a query sharing no word should rank nothing: %+v", got)
	}
	if got := d.Nearest("Ambiguous Three", 1); len(got) != 1 {
		t.Fatalf("n should cap the result: %+v", got)
	}
}

func TestNearestIsDeterministic(t *testing.T) {
	d := load(t)
	a := d.Nearest("ambiguous plan code deep", 3)
	b := d.Nearest("ambiguous plan code deep", 3)
	for i := range a {
		if a[i].Line != b[i].Line {
			t.Fatal("two runs disagree")
		}
	}
}

func TestNumberedDecisionHeadings(t *testing.T) {
	d, err := ParseFile("../../testdata/decisions.md")
	if err != nil {
		t.Fatal(err)
	}
	m := d.FindHeading("### 07")
	if len(m.Found) != 1 || m.Found[0].Text != "07. Queue writes behind one worker" {
		t.Fatalf("numbered decision not addressable: %+v", m.Found)
	}
	m = d.FindHeading("Queue writes")
	if len(m.Found) != 1 {
		t.Fatalf("decision text should match too: %+v", m.Found)
	}
	m = d.FindHeading("Decisions")
	if len(m.Found) != 1 || m.Found[0].Level != 2 {
		t.Fatalf("container heading not found: %+v", m.Found)
	}
}

func TestFixAlternation(t *testing.T) {
	cases := []struct{ in, want string }{
		{`section\|tasks`, "section|tasks"},
		{`a\|b\|c`, "a|b|c"},
		{`.*\|tasks`, `.*\|tasks`},
		{`[st]\|tasks`, `[st]\|tasks`},
		{`\bqueue\|writes`, `\bqueue\|writes`},
		{"section|tasks", "section|tasks"},
		{"plain words", "plain words"},
	}
	for _, c := range cases {
		if got := FixAlternation(c.in); got != c.want {
			t.Errorf("FixAlternation(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
