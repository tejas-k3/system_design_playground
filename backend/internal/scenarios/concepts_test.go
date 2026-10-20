package scenarios

import "testing"

// Every challenge is tagged with at least one concept from the fixed list, and every concept on the list is
// practised by at least one challenge, so the skill map has no empty rows and no stray tags.
func TestEveryScenarioHasValidConcepts(t *testing.T) {
	valid := map[string]bool{}
	for _, c := range Concepts {
		valid[c] = true
	}
	used := map[string]bool{}
	for _, sc := range All() {
		if len(sc.Concepts) == 0 {
			t.Errorf("%s has no concept tags", sc.ID)
		}
		seen := map[string]bool{}
		for _, c := range sc.Concepts {
			if !valid[c] {
				t.Errorf("%s: %q is not a known concept", sc.ID, c)
			}
			if seen[c] {
				t.Errorf("%s: %q listed twice", sc.ID, c)
			}
			seen[c], used[c] = true, true
		}
	}
	for _, c := range Concepts {
		if !used[c] {
			t.Errorf("no challenge practises %q", c)
		}
	}
	for id := range conceptTags {
		if Get(id) == nil {
			t.Errorf("tags for unknown challenge %q", id)
		}
	}
}
