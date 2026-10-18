package scenarios

import (
	"encoding/json"
	"testing"

	"sysdesign/internal/engine"
)

func reviews() []*Question {
	var out []*Question
	for _, q := range AllQuestions() {
		if q.Mode == ModeReview {
			out = append(out, q)
		}
	}
	return out
}

func idsOf(d *engine.Diagram) map[string]bool {
	m := map[string]bool{}
	for _, n := range d.Nodes {
		m[n.ID] = true
	}
	for _, e := range d.Edges {
		m[e.ID] = true
	}
	return m
}

func TestReviewScoring(t *testing.T) {
	q := &Question{ID: "t", Mode: ModeReview, Diagram: &engine.Diagram{Nodes: []engine.Node{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}},
		defects: []Defect{{ID: "d1", Nodes: []string{"a"}, Explain: "one"}, {ID: "d2", Nodes: []string{"b", "c"}, Explain: "two"}}}
	grade := func(marked ...string) ReviewResult {
		r, err := q.GradeReview(marked)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := grade("a", "b"); len(r.Found) != 2 || len(r.Missed) != 0 || len(r.FalseMarks) != 0 || r.Score != 1 || r.Stars != 3 || r.Precision != 1 || r.Recall != 1 {
		t.Errorf("exactly the defects: %+v", r)
	}
	if r := grade("a", "b", "c"); r.Score != 1 || len(r.FalseMarks) != 0 {
		t.Errorf("marking both nodes of one defect is not a false positive: %+v", r)
	}
	if r := grade("a", "d"); len(r.Found) != 1 || len(r.Missed) != 1 || len(r.FalseMarks) != 1 || r.Score != 0 || r.Precision != 0.5 || r.Recall != 0.5 {
		t.Errorf("one found, one missed, one false (found minus false = 0): %+v", r)
	}
	if r := grade("a", "b", "c", "d"); r.Score != 0.5 || r.Stars != 1 || r.Recall != 1 || r.Precision != 0.75 {
		t.Errorf("marking everything: all found but one false mark: %+v", r)
	}
	if r := grade(); r.Score != 0 || len(r.Missed) != 2 || r.Precision != 0 || r.Recall != 0 {
		t.Errorf("marking nothing: %+v", r)
	}
	if r := grade("a", "a", "b"); len(r.FalseMarks) != 0 || r.Score != 1 {
		t.Errorf("a repeated mark counts once: %+v", r)
	}
	if _, err := q.GradeReview([]string{"zzz"}); err == nil {
		t.Error("an id that is not in the diagram must be refused")
	}
	if _, err := (&Question{Mode: ModeEstimate}).GradeReview(nil); err == nil {
		t.Error("not a review question")
	}
}

func TestMarkingEverythingScoresLowAndMarkingTheDefectsScoresFully(t *testing.T) {
	qs := reviews()
	if len(qs) < 3 {
		t.Fatalf("%d review questions, want at least 3", len(qs))
	}
	for _, q := range qs {
		var exact, all []string
		for _, d := range q.defects {
			exact = append(exact, d.Nodes[0])
		}
		for id := range idsOf(q.Diagram) {
			all = append(all, id)
		}
		if r, _ := q.GradeReview(exact); r.Score != 1 || r.Stars != 3 {
			t.Errorf("%s: marking exactly the defects should score 100%%: %+v", q.ID, r)
		}
		r, _ := q.GradeReview(all)
		if r.Precision > 0.5 || r.Stars == 3 {
			t.Errorf("%s: marking everything must not look good: %+v", q.ID, r)
		}
	}
}

// Every defect must be a flaw the design lint also reports, on the node(s) the defect names, and the lint must
// report nothing else of warning or error severity: otherwise a correct reviewer could be marked wrong, or a
// defect could be something only the author believed in.
func TestReviewDefectsAreBackedByLintAndNothingElseIsFlagged(t *testing.T) {
	for _, q := range reviews() {
		findings := engine.Lint(*q.Diagram)
		seen := map[string]bool{}
		for _, d := range q.defects {
			if len(d.Nodes) == 0 || d.Explain == "" || len(d.Rules) == 0 {
				t.Errorf("%s/%s: needs nodes, an explanation and the lint rules that back it", q.ID, d.ID)
			}
			for _, n := range d.Nodes {
				if seen[n] {
					t.Errorf("%s: node %s belongs to two defects, so one mark would find both", q.ID, n)
				}
				seen[n] = true
				if !idsOf(q.Diagram)[n] {
					t.Errorf("%s/%s: %s is not in the diagram", q.ID, d.ID, n)
				}
			}
			backed := false
			for _, f := range findings {
				for _, rule := range d.Rules {
					if f.Rule == rule && sharesNode(f.Nodes, d.Nodes) {
						backed = true
					}
				}
			}
			if !backed {
				t.Errorf("%s/%s: no lint finding %v on %v backs this defect (lint says %+v)", q.ID, d.ID, d.Rules, d.Nodes, findings)
			}
		}
		for _, f := range findings {
			if f.Severity == "info" {
				continue
			}
			listed := false
			for _, d := range q.defects {
				for _, rule := range d.Rules {
					if f.Rule == rule && sharesNode(f.Nodes, d.Nodes) {
						listed = true
					}
				}
			}
			if !listed {
				t.Errorf("%s: lint reports %s on %v but no defect lists it, so a sharp reviewer would be marked wrong", q.ID, f.Rule, f.Nodes)
			}
		}
	}
}

func sharesNode(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func TestReviewJSONServesTheDiagramButNoDefects(t *testing.T) {
	for _, q := range reviews() {
		b, _ := json.Marshal(q)
		var tree map[string]any
		_ = json.Unmarshal(b, &tree)
		if tree["diagram"] == nil {
			t.Errorf("%s: a review question must serve its diagram", q.ID)
		}
		for _, k := range jsonKeys(tree) {
			if k == "defects" || k == "rules" || k == "explain" {
				t.Errorf("%s: served JSON has the key %q", q.ID, k)
			}
		}
	}
}
