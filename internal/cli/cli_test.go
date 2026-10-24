package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

const fixture = "../../testdata/sample.md"

const (
	realTodo   = "/Users/jasperlyons/workspace/projects/mavexa/mavexa/docs/todo.md"
	realKanban = "/Users/jasperlyons/workspace/Personal/Pages/Projects.md"
)

func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errw bytes.Buffer
	code := Run(args, &out, &errw)
	return out.String(), errw.String(), code
}

func requireFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("%s is not on this machine", path)
	}
}

func TestTree(t *testing.T) {
	out, _, code := run(t, "tree", fixture)
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.HasPrefix(lines[0], fixture+"  50 lines") {
		t.Fatalf("header is %q", lines[0])
	}
	if !strings.Contains(out, "8-50") || !strings.Contains(out, "# Sample Doc") {
		t.Fatalf("missing the top heading:\n%s", out)
	}
	if !strings.Contains(out, "    ## Plan") {
		t.Fatalf("subheadings should be indented:\n%s", out)
	}
	if !strings.Contains(out, "      ### Deep") {
		t.Fatalf("level three should be indented twice:\n%s", out)
	}
}

func TestTreeDepth(t *testing.T) {
	out, _, _ := run(t, "tree", fixture, "--depth", "2")
	if strings.Contains(out, "### Deep") {
		t.Fatalf("--depth 2 should hide level three:\n%s", out)
	}
}

func TestTreeIsDeterministic(t *testing.T) {
	a, _, _ := run(t, "tree", fixture)
	b, _, _ := run(t, "tree", fixture)
	if a != b {
		t.Fatal("two runs disagree")
	}
}

func TestToc(t *testing.T) {
	out, _, code := run(t, "toc", fixture)
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, fixture+":17: ## Plan\n") {
		t.Fatalf("toc line missing:\n%s", out)
	}
	if strings.Contains(out, "~") {
		t.Fatalf("toc should carry no sizes:\n%s", out)
	}
}

func TestSection(t *testing.T) {
	out, _, code := run(t, "section", fixture, "Plan")
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(out, fixture+":17: ## Plan\n") {
		t.Fatalf("heading line is wrong:\n%s", out)
	}
	if !strings.Contains(out, "- [ ] first task #alpha") {
		t.Fatalf("body missing:\n%s", out)
	}
}

func TestSectionIncludesSubsectionsByDefault(t *testing.T) {
	out, _, _ := run(t, "section", fixture, "Ambiguous Two")
	if !strings.Contains(out, "### Deep") {
		t.Fatalf("subsection missing:\n%s", out)
	}
	out, _, _ = run(t, "section", fixture, "Ambiguous Two", "--no-children")
	if strings.Contains(out, "### Deep") {
		t.Fatalf("--no-children should stop at the subheading:\n%s", out)
	}
}

func TestSectionNoBody(t *testing.T) {
	out, _, _ := run(t, "section", fixture, "Ambiguous Two", "--no-body")
	if strings.Contains(out, "deep body") {
		t.Fatalf("--no-body printed prose:\n%s", out)
	}
	if !strings.Contains(out, "### Deep") || !strings.Contains(out, "body not printed") {
		t.Fatalf("--no-body should print the outline:\n%s", out)
	}
}

func TestSectionMaxLines(t *testing.T) {
	out, _, _ := run(t, "section", fixture, "Plan", "--max-lines", "2")
	if !strings.Contains(out, "more lines, use --max-lines or a narrower heading") {
		t.Fatalf("no truncation marker:\n%s", out)
	}
}

func TestSectionByLineNumber(t *testing.T) {
	out, _, code := run(t, "section", fixture, "17")
	if code != OK || !strings.HasPrefix(out, fixture+":17: ## Plan") {
		t.Fatalf("exit %d, out:\n%s", code, out)
	}
}

func TestSectionAmbiguousExitsTwo(t *testing.T) {
	out, _, code := run(t, "section", fixture, "ambiguous")
	if code != Ambiguous {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(out, ":40: ## Ambiguous One") || !strings.Contains(out, ":44: ## Ambiguous Two") {
		t.Fatalf("candidates not listed:\n%s", out)
	}
}

func TestSectionNoMatchExitsOne(t *testing.T) {
	_, _, code := run(t, "section", fixture, "no such heading")
	if code != NoMatch {
		t.Fatalf("exit %d, want 1", code)
	}
}

func TestSectionSeveralHeadingsInOneCall(t *testing.T) {
	out, _, code := run(t, "section", fixture, "Code", "Plan")
	if code != OK {
		t.Fatalf("exit %d, out:\n%s", code, out)
	}
	code0 := strings.Index(out, fixture+":25: ## Code")
	plan := strings.Index(out, fixture+":17: ## Plan")
	if code0 != 0 || plan < 0 {
		t.Fatalf("both sections should print, in the order asked:\n%s", out)
	}
	if plan < code0 {
		t.Fatalf("order was not preserved:\n%s", out)
	}
	if !strings.Contains(out, "Text before the fence.") || !strings.Contains(out, "first task #alpha") {
		t.Fatalf("a body is missing:\n%s", out)
	}
}

func TestSectionMaxLinesAppliesPerSection(t *testing.T) {
	out, _, _ := run(t, "section", fixture, "Plan", "Code", "--max-lines", "2")
	if strings.Count(out, "more lines, use --max-lines") != 2 {
		t.Fatalf("each section should truncate on its own:\n%s", out)
	}
}

func TestSectionKeepsPrintingPastAMiss(t *testing.T) {
	out, _, code := run(t, "section", fixture, "no such heading", "Plan")
	if code != Ambiguous {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(out, "no heading matching 'no such heading'") {
		t.Fatalf("the miss was not reported:\n%s", out)
	}
	if !strings.Contains(out, fixture+":17: ## Plan") || !strings.Contains(out, "first task #alpha") {
		t.Fatalf("a miss should not stop the other headings:\n%s", out)
	}
}

func TestSectionAmbiguousAmongSeveralHeadingsExitsTwo(t *testing.T) {
	out, _, code := run(t, "section", fixture, "Plan", "ambiguous")
	if code != Ambiguous {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(out, fixture+":17: ## Plan") {
		t.Fatalf("the resolved heading should still print:\n%s", out)
	}
	if !strings.Contains(out, "headings match") {
		t.Fatalf("candidates not listed:\n%s", out)
	}
}

func TestSectionBareNumbersStillResolveAsLines(t *testing.T) {
	out, _, code := run(t, "section", fixture, "17", "25")
	if code != OK {
		t.Fatalf("exit %d, out:\n%s", code, out)
	}
	if !strings.Contains(out, fixture+":17: ## Plan") || !strings.Contains(out, fixture+":25: ## Code") {
		t.Fatalf("line numbers should each resolve:\n%s", out)
	}
}

func TestSectionFallsBackToAnUnquotedMultiWordHeading(t *testing.T) {
	out, _, code := run(t, "section", fixture, "Ambiguous", "Two")
	if code != OK {
		t.Fatalf("exit %d, out:\n%s", code, out)
	}
	if !strings.Contains(out, fixture+":44: ## Ambiguous Two") || !strings.Contains(out, "body b") {
		t.Fatalf("unquoted words should join when they only resolve together:\n%s", out)
	}
	if strings.Contains(out, "no heading matching") {
		t.Fatalf("the joined form resolved, so nothing should be reported missing:\n%s", out)
	}
}

func TestSectionMissingHeadingSaysWhatItSearched(t *testing.T) {
	out, _, code := run(t, "section", fixture, "Ambiguous Three")
	if code != NoMatch {
		t.Fatalf("exit %d, want 1", code)
	}
	want := "no heading matching 'Ambiguous Three' in " + fixture + " (9 headings)\n"
	if !strings.HasPrefix(out, want) {
		t.Fatalf("first line is wrong:\n%s", out)
	}
	if !strings.Contains(out, `closest headings: "Ambiguous One" :40`) {
		t.Fatalf("closest headings missing:\n%s", out)
	}
	if !strings.Contains(out, "\n                  \"Ambiguous Two\" :44\n") {
		t.Fatalf("continuation lines should align under the first:\n%s", out)
	}
	if !strings.Contains(out, "try a shorter heading or `mdq toc "+fixture+"`") {
		t.Fatalf("no next step suggested:\n%s", out)
	}
}

func TestSectionMissingHeadingWithNothingCloseListsNoHeadings(t *testing.T) {
	out, _, code := run(t, "section", fixture, "zzzqqq")
	if code != NoMatch {
		t.Fatalf("exit %d, want 1", code)
	}
	if strings.Contains(out, "closest headings") {
		t.Fatalf("a query sharing no word should list nothing:\n%s", out)
	}
}

func TestGrepTagsEachHitWithItsSection(t *testing.T) {
	out, _, code := run(t, "grep", fixture, "task")
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(out, fixture+"  6 hits\n") {
		t.Fatalf("expected one header line per file:\n%s", out)
	}
	if !strings.Contains(out, "\n19 [Sample Doc > Plan]: - [ ] first task #alpha\n") {
		t.Fatalf("hit line is wrong:\n%s", out)
	}
	if !strings.Contains(out, "\n31 [Sample Doc > Code]:") {
		t.Fatalf("a hit inside a fence should still report its section:\n%s", out)
	}
}

func TestGrepRegexAndCase(t *testing.T) {
	out, _, _ := run(t, "grep", fixture, "^## (Plan|Code)$")
	if strings.Count(out, "\n") != 3 {
		t.Fatalf("expected a header and two hits:\n%s", out)
	}
	if _, _, code := run(t, "grep", fixture, "PLAN"); code != NoMatch {
		t.Fatal("search should be case sensitive by default")
	}
	if _, _, code := run(t, "grep", fixture, "PLAN", "-i"); code != OK {
		t.Fatal("-i should ignore case")
	}
}

func TestGrepNoMatchSaysWhatItSearched(t *testing.T) {
	out, _, code := run(t, "grep", fixture, "Ambiguous Three sausage")
	if code != NoMatch {
		t.Fatalf("exit %d, want 1", code)
	}
	want := "no match for /Ambiguous Three sausage/ in " + fixture + " (50 lines, 9 headings)\n"
	if !strings.HasPrefix(out, want) {
		t.Fatalf("first line is wrong:\n%s", out)
	}
	if !strings.Contains(out, `closest headings: "Ambiguous One" :40`) {
		t.Fatalf("closest headings missing:\n%s", out)
	}
	if !strings.Contains(out, "\n                  \"Ambiguous Two\" :44\n") {
		t.Fatalf("continuation lines should align under the first:\n%s", out)
	}
	if !strings.HasSuffix(out, "try a shorter pattern or `mdq toc "+fixture+"`\n") {
		t.Fatalf("no next step suggested:\n%s", out)
	}
}

func TestGrepNoMatchListsAtMostThreeHeadings(t *testing.T) {
	out, _, _ := run(t, "grep", fixture, "ambiguous plan code deep sausage")
	if n := strings.Count(out, ":"); n < 1 {
		t.Fatalf("expected some candidates:\n%s", out)
	}
	if got := strings.Count(out, "\" :"); got > 3 {
		t.Fatalf("listed %d headings, want at most 3:\n%s", got, out)
	}
}

func TestGrepNoMatchWithNothingCloseListsNoHeadings(t *testing.T) {
	out, _, code := run(t, "grep", fixture, "zzzqqq")
	if code != NoMatch {
		t.Fatalf("exit %d, want 1", code)
	}
	if strings.Contains(out, "closest headings") {
		t.Fatalf("a pattern sharing no word should list nothing:\n%s", out)
	}
	if !strings.Contains(out, "no match for /zzzqqq/") {
		t.Fatalf("miss line missing:\n%s", out)
	}
}

func TestGrepNoMatchOnADirectoryCountsFiles(t *testing.T) {
	out, _, code := run(t, "grep", "../../testdata", "Ambiguous Three sausage")
	if code != NoMatch {
		t.Fatalf("exit %d, want 1", code)
	}
	if !regexp.MustCompile(`^no match for /Ambiguous Three sausage/ in \.\./\.\./testdata \(\d+ files, \d+ headings\)\n`).MatchString(out) {
		t.Fatalf("a directory should report files scanned:\n%s", out)
	}
	if !strings.Contains(out, `"Ambiguous One" ../../testdata/sample.md:40`) {
		t.Fatalf("a directory candidate should name its file:\n%s", out)
	}
	if !strings.HasSuffix(out, "try a shorter pattern or one file\n") {
		t.Fatalf("no next step suggested:\n%s", out)
	}
}

func TestGrepRewritesGrepStyleAlternation(t *testing.T) {
	out, _, code := run(t, "grep", fixture, `Plan\|Code`)
	if code != OK {
		t.Fatalf("exit %d, output:\n%s", code, out)
	}
	if !strings.Contains(out, "## Plan") || !strings.Contains(out, "## Code") {
		t.Fatalf("both branches should match:\n%s", out)
	}
	if strings.Count(out, "\n") != 3 {
		t.Fatalf("expected a header and two hits:\n%s", out)
	}
	// The rewrite ran, so the miss is a real miss and the note would be wrong.
	out, _, code = run(t, "grep", fixture, `nothing\|missing`)
	if code != NoMatch {
		t.Fatalf("exit %d, output:\n%s", code, out)
	}
	if strings.Contains(out, "note:") {
		t.Fatalf("a rewritten pattern should not be told its pipe was literal:\n%s", out)
	}
	// Another metacharacter is present, so the pattern was left as written and
	// the \| really did match a literal pipe.
	out, _, code = run(t, "grep", fixture, `.*\|missing`)
	if code != NoMatch {
		t.Fatalf("exit %d, output:\n%s", code, out)
	}
	if !strings.Contains(out, `note: \| is a literal pipe here, use A|B for alternation`) {
		t.Fatalf("a miss on an untouched \\| pattern should explain the escape:\n%s", out)
	}
}

func TestTasksDefaultsToOpen(t *testing.T) {
	out, _, code := run(t, "tasks", fixture)
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(out, "[x]") {
		t.Fatalf("done items should be hidden by default:\n%s", out)
	}
	if !strings.HasPrefix(out, fixture+"  4 open, 0 done\n") {
		t.Fatalf("expected one header line per file:\n%s", out)
	}
	if !strings.Contains(out, "\n19 [Plan] first task #alpha\n") {
		t.Fatalf("task line is wrong:\n%s", out)
	}
	if !strings.Contains(out, "child task #beta  < parent task") {
		t.Fatalf("nested item lost its parent context:\n%s", out)
	}
}

func TestTasksFilters(t *testing.T) {
	out, _, _ := run(t, "tasks", fixture, "--done")
	if !strings.Contains(out, "[x] done task") || strings.Contains(out, "first task") {
		t.Fatalf("--done is wrong:\n%s", out)
	}
	out, _, _ = run(t, "tasks", fixture, "--all")
	if !strings.Contains(out, "first task") || !strings.Contains(out, "[x] done task") {
		t.Fatalf("--all is wrong:\n%s", out)
	}
	out, _, _ = run(t, "tasks", fixture, "--tag", "beta")
	if !strings.Contains(out, "child task") || strings.Contains(out, "first task") {
		t.Fatalf("--tag is wrong:\n%s", out)
	}
	out, _, _ = run(t, "tasks", fixture, "--section", "Plan")
	if !strings.Contains(out, "first task") || strings.Contains(out, "Shipped thing") {
		t.Fatalf("--section is wrong:\n%s", out)
	}
	if _, _, code := run(t, "tasks", fixture, "--tag", "nosuchtag"); code != NoMatch {
		t.Fatal("an empty task list should exit 1")
	}
}

func TestTasksIncludesBoldLeadBulletsByDefault(t *testing.T) {
	const f = "../../testdata/followups.md"
	out, _, _ := run(t, "tasks", f, "--all")
	if !strings.Contains(out, "Database tracing waits on the Postgres port") {
		t.Fatalf("bold lead bullets should count by default:\n%s", out)
	}
	if !strings.Contains(out, "[x] **The Datadog API key is resolved.**") {
		t.Fatalf("a bullet with an upper case status word is done:\n%s", out)
	}
	out, _, _ = run(t, "tasks", f, "--all", "--no-bullets")
	if strings.Contains(out, "Database tracing") {
		t.Fatalf("--no-bullets should leave only checkbox and heading items:\n%s", out)
	}
	if !strings.Contains(out, "a plain checkbox still counts") {
		t.Fatalf("--no-bullets dropped a checkbox item:\n%s", out)
	}
	if _, _, code := run(t, "tasks", f, "--all", "--bullets"); code != OK {
		t.Fatal("--bullets should still be accepted")
	}
	out, _, _ = run(t, "tasks", f, "--done")
	if !strings.Contains(out, "The agent has no span of its own") {
		t.Fatalf("a bold **FIXED** later in the item should close it:\n%s", out)
	}
	out, _, _ = run(t, "tasks", f, "--open")
	if !strings.Contains(out, "Database tracing") || strings.Contains(out, "no span of its own") {
		t.Fatalf("open list is wrong:\n%s", out)
	}
}

func TestTasksTruncationNamesTheNarrowingFlags(t *testing.T) {
	out, _, _ := run(t, "tasks", fixture, "--all", "--max-lines", "2")
	if !strings.Contains(out, "narrow with --section or --tag") {
		t.Fatalf("truncation marker should name the narrowing flags:\n%s", out)
	}
}

func TestTasksJSONKeepsTheFullPath(t *testing.T) {
	out, _, _ := run(t, "tasks", fixture, "--all", "--json")
	if !strings.Contains(out, `"file": "`+fixture+`"`) {
		t.Fatalf("json lost the path:\n%s", out)
	}
}

func TestTasksJSON(t *testing.T) {
	out, _, _ := run(t, "tasks", fixture, "--all", "--json")
	if !strings.Contains(out, `"kind": "checkbox"`) || !strings.Contains(out, `"line": 19`) {
		t.Fatalf("json is wrong:\n%s", out)
	}
}

func TestUnknownFlagIsAUsageError(t *testing.T) {
	_, errw, code := run(t, "tree", fixture, "--nope")
	if code != Ambiguous || !strings.Contains(errw, "unknown flag") {
		t.Fatalf("exit %d, stderr %q", code, errw)
	}
}

func TestRealTodoTree(t *testing.T) {
	requireFile(t, realTodo)
	out, _, code := run(t, "tree", realTodo, "--depth", "2")
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "## Housekeeping") {
		t.Fatalf("known heading missing from the outline")
	}
	if strings.Contains(out, "### ") {
		t.Fatal("--depth 2 leaked level three headings")
	}
}

func TestRealTodoSection(t *testing.T) {
	requireFile(t, realTodo)
	out, _, code := run(t, "section", realTodo, "Housekeeping", "--max-lines", "5")
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	first := strings.SplitN(out, "\n", 2)[0]
	if !regexp.MustCompile(`todo\.md:\d+: ## Housekeeping$`).MatchString(first) {
		t.Fatalf("heading line is %q", first)
	}
	if !strings.Contains(out, "more lines, use --max-lines") {
		t.Fatal("expected the body to be truncated")
	}
}

func TestRealTodoSeveralHeadingsInOneCall(t *testing.T) {
	requireFile(t, realTodo)
	out, _, code := run(t, "section", realTodo, "Housekeeping", "LLM", "--no-body")
	if code != OK {
		t.Fatalf("exit %d, out:\n%s", code, out)
	}
	if strings.Count(out, "body not printed") != 2 {
		t.Fatalf("expected two section outlines:\n%s", out)
	}
	if !strings.Contains(out, "## Housekeeping") || !strings.Contains(out, "## LLM") {
		t.Fatalf("a heading is missing:\n%s", out)
	}
}

func TestRealTodoGrepMissSuggestsTheNearestHeading(t *testing.T) {
	requireFile(t, realTodo)
	out, _, code := run(t, "grep", realTodo, "Housekeeping sausage widget")
	if code != NoMatch {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(out, "no match for /Housekeeping sausage widget/ in "+realTodo+" (") {
		t.Fatalf("miss line is wrong:\n%s", out)
	}
	if !strings.Contains(out, `closest headings: "Housekeeping"`) {
		t.Fatalf("the obvious near heading was not offered:\n%s", out)
	}
}

func TestRealTodoResolvesAnEmojiHeading(t *testing.T) {
	requireFile(t, realTodo)
	out, _, code := run(t, "section", realTodo, "Deploy gate", "--no-body")
	if code != OK {
		t.Fatalf("exit %d, out:\n%s", code, out)
	}
	if !strings.Contains(out, "## ✅ Deploy gate") {
		t.Fatalf("emoji and trailing status text should be ignored when matching:\n%s", out)
	}
}

func TestRealTodoTasks(t *testing.T) {
	requireFile(t, realTodo)
	out, _, code := run(t, "tasks", realTodo, "--done")
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "[x] ✅ Follow-up emails generate but cannot be sent") {
		t.Fatalf("known closed item missing:\n%s", out)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, realTodo) || strings.HasPrefix(line, "...") {
			continue
		}
		if !strings.Contains(line, "[x]") {
			t.Fatalf("--done returned an open item: %s", line)
		}
	}
}

func TestRealKanbanTasks(t *testing.T) {
	requireFile(t, realKanban)
	out, _, code := run(t, "tasks", realKanban, "--section", "Doing", "--max-lines", "0")
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected a header and several items in Doing, got %d lines", len(lines))
	}
	if !strings.HasPrefix(lines[0], realKanban+"  ") || !strings.Contains(lines[0], " open, ") {
		t.Fatalf("header line is %q", lines[0])
	}
	for _, line := range lines[1:] {
		if !strings.Contains(line, "[Doing]") {
			t.Fatalf("item from another column: %s", line)
		}
		if strings.Contains(line, realKanban) {
			t.Fatalf("the path should appear once, in the header: %s", line)
		}
	}
}

func TestRealKanbanColumnsAreHeadings(t *testing.T) {
	requireFile(t, realKanban)
	out, _, code := run(t, "toc", realKanban)
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	for _, col := range []string{"## Tirage", "## Todo", "## Doing", "## Done"} {
		if !strings.Contains(out, col) {
			t.Fatalf("column %q missing from the outline", col)
		}
	}
}

func TestReadOnly(t *testing.T) {
	requireFile(t, realKanban)
	before, err := os.Stat(realKanban)
	if err != nil {
		t.Fatal(err)
	}
	run(t, "tasks", realKanban, "--all")
	run(t, "tree", realKanban)
	after, err := os.Stat(realKanban)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) || before.Size() != after.Size() {
		t.Fatal("mdq must never write to the files it reads")
	}
}

const linksFixture = "../../testdata/links.md"

func TestLinksTextOutput(t *testing.T) {
	out, _, code := run(t, "links", linksFixture)
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(out, linksFixture+"  12 links\n") {
		t.Fatalf("header is wrong:\n%s", out)
	}
	if !strings.Contains(out, "\n 7 [Links]: sample.md  -> ../../testdata/sample.md\n") {
		t.Fatalf("internal row is wrong:\n%s", out)
	}
	if !strings.Contains(out, "\n12 [Links > External]: https://example.com/a\n") {
		t.Fatalf("external row is wrong:\n%s", out)
	}
	if !strings.Contains(out, "split.md  -> ../../testdata/split.md (missing)\n") {
		t.Fatalf("missing marker absent:\n%s", out)
	}
}

func TestLinksInternalExternal(t *testing.T) {
	out, _, _ := run(t, "links", linksFixture, "--internal")
	if strings.Contains(out, "https://") || !strings.Contains(out, "sample.md") {
		t.Fatalf("--internal is wrong:\n%s", out)
	}
	out, _, _ = run(t, "links", linksFixture, "--external")
	if strings.Contains(out, "sample.md") || !strings.Contains(out, "mailto:a@b.co") {
		t.Fatalf("--external is wrong:\n%s", out)
	}
	if _, _, code := run(t, "links", linksFixture, "--internal", "--external"); code != Ambiguous {
		t.Fatalf("both flags should exit 2, got %d", code)
	}
}

func TestLinksSection(t *testing.T) {
	out, _, code := run(t, "links", linksFixture, "--section", "External")
	if code != OK || !strings.Contains(out, "example.com") || strings.Contains(out, "sample.md") {
		t.Fatalf("--section is wrong (exit %d):\n%s", code, out)
	}
}

func TestLinksBare(t *testing.T) {
	out, _, _ := run(t, "links", linksFixture)
	if strings.Contains(out, "bare.example.com") {
		t.Fatalf("bare URL shown by default:\n%s", out)
	}
	out, _, _ = run(t, "links", linksFixture, "--bare")
	if !strings.Contains(out, "https://bare.example.com/path\n") {
		t.Fatalf("--bare did not add the URL:\n%s", out)
	}
}

func TestLinksJSON(t *testing.T) {
	out, _, code := run(t, "links", linksFixture, "--json", "--section", "Split")
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{`"file": "` + linksFixture + `"`, `"line": 27`, `"kind": "inline"`, `"target": "split.md"`, `"missing": true`, `"section": "Split"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("json lacks %s:\n%s", want, out)
		}
	}
	out, _, code = run(t, "links", "../../testdata/sample.md", "--json")
	if code != NoMatch || out != "[]\n" {
		t.Fatalf("empty json: exit %d, out %q", code, out)
	}
}

func TestLinksNoneExitsOne(t *testing.T) {
	out, _, code := run(t, "links", fixture)
	if code != NoMatch || out != "" {
		t.Fatalf("exit %d, out %q", code, out)
	}
}

func TestLinksDirectory(t *testing.T) {
	out, _, code := run(t, "links", "../../testdata/linkrepo")
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(out, "../../testdata/linkrepo/docs/sub/a.md  3 links\n") {
		t.Fatalf("header is wrong:\n%s", out)
	}
	if !strings.Contains(out, "docs/b.md  -> ../../testdata/linkrepo/docs/b.md\n") || !strings.Contains(out, "nope.md  -> ../../testdata/linkrepo/docs/sub/nope.md (missing)\n") {
		t.Fatalf("rows are wrong:\n%s", out)
	}
}

func TestRealLinksGlossaryAnchor(t *testing.T) {
	const f = "/Users/jasperlyons/workspace/projects/mavexa/mavexa/docs/pages-and-pipeline.md"
	requireFile(t, f)
	out, _, code := run(t, "links", f, "--json")
	if code != OK {
		t.Fatalf("exit %d", code)
	}
	var links []struct {
		Line           int
		Target, Anchor string
	}
	if err := json.Unmarshal([]byte(out), &links); err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		if l.Line == 234 && l.Target == "glossary.md" && l.Anchor == "effective-kind" {
			return
		}
	}
	t.Fatalf("glossary.md#effective-kind at line 234 not found")
}
