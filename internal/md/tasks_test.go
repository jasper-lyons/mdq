package md

import "testing"

func TestStatusMarkers(t *testing.T) {
	cases := []struct {
		text   string
		isTask bool
		done   bool
	}{
		{"✅ Deploy gate — CLEAR (closed)", true, true},
		{"Shipped thing — CLOSED (2026-01-02)", true, true},
		{"~~Pull from Salesforce~~ (superseded)", true, true},
		{"P4 shipped before P3 — resolved: password replaced sudo", true, true},
		{"🚧 Half built", true, false},
		{"TODO write the migration", true, false},
		{"Architectural review — first walk", false, false},
		{"Item library — the deliberate gaps", false, false},
	}
	for _, c := range cases {
		isTask, done := Status(c.text)
		if isTask != c.isTask || done != c.done {
			t.Errorf("Status(%q) = (%v, %v), want (%v, %v)", c.text, isTask, done, c.isTask, c.done)
		}
	}
}

func TestTasksFromFixture(t *testing.T) {
	d := load(t)
	tasks := d.Tasks(TaskOptions{})
	if len(tasks) != 6 {
		t.Fatalf("found %d tasks, want 6", len(tasks))
	}
	byLine := map[int]Task{}
	for _, task := range tasks {
		byLine[task.Line] = task
	}
	if got := byLine[19]; got.Done || got.Section != "Plan" || len(got.Tags) != 1 || got.Tags[0] != "alpha" {
		t.Fatalf("line 19 parsed as %+v", got)
	}
	if !byLine[20].Done {
		t.Fatal("line 20 should be done")
	}
	if got := byLine[22]; got.Parent != "parent task" {
		t.Fatalf("nested item lost its parent: %+v", got)
	}
	if got := byLine[23]; got.Done {
		t.Fatal("an in progress box is open, not done")
	}
	if got := byLine[36]; got.Kind != "heading" || !got.Done {
		t.Fatalf("heading task parsed as %+v", got)
	}
	for _, task := range tasks {
		if task.Line > 27 && task.Line < 34 {
			t.Fatalf("a fenced line became a task: %+v", task)
		}
	}
}

func TestBulletsAreOptional(t *testing.T) {
	d := load(t)
	plain := len(d.Tasks(TaskOptions{}))
	if got := len(d.Tasks(TaskOptions{Bullets: true})); got < plain {
		t.Fatalf("bullets mode returned fewer tasks: %d < %d", got, plain)
	}
}
