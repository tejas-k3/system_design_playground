package designs

import (
	"encoding/json"
	"os"
	"testing"

	"sysdesign/internal/engine"
)

// The starter templates live in the frontend as JSON; this keeps them honest against the real component registry:
// every kind exists, every connection is allowed, no setting is out of range, and each design can actually run.
type template struct {
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	Summary string         `json:"summary"`
	Diagram engine.Diagram `json:"diagram"`
}

func TestStarterTemplatesAreValidAndRunnable(t *testing.T) {
	raw, err := os.ReadFile("../../../frontend/src/model/templates.json")
	if err != nil {
		t.Fatal(err)
	}
	var list []template
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	if len(list) < 6 {
		t.Fatalf("expected the six starter templates, found %d", len(list))
	}
	seen := map[string]bool{}
	for _, tpl := range list {
		if tpl.ID == "" || tpl.Title == "" || tpl.Summary == "" || seen[tpl.ID] {
			t.Fatalf("template needs a unique id, a title and a summary: %+v", tpl.ID)
		}
		seen[tpl.ID] = true
		clean, adjusted := engine.Sanitize(tpl.Diagram)
		if adjusted || len(clean.Nodes) != len(tpl.Diagram.Nodes) || len(clean.Edges) != len(tpl.Diagram.Edges) {
			t.Errorf("%s: sanitising changed the design (adjusted %v, nodes %d/%d, edges %d/%d)", tpl.ID, adjusted, len(clean.Nodes), len(tpl.Diagram.Nodes), len(clean.Edges), len(tpl.Diagram.Edges))
			continue
		}
		s := engine.New(clean, engine.Options{Seed: 1})
		s.Advance(20000)
		r := s.RunSnapshot()
		if r.Resolved == 0 {
			t.Errorf("%s: no request finished in 20 s", tpl.ID)
		}
		if float64(r.OK) < 0.9*float64(r.Resolved) {
			t.Errorf("%s: a starter should work at its default load, %d ok of %d", tpl.ID, r.OK, r.Resolved)
		}
	}
}
