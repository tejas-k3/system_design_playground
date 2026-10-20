package scenarios

import (
	"os"
	"regexp"
	"testing"
)

// The challenge texts may mention glossary terms as {{id}}. An id that the frontend glossary does not define would
// silently show as plain words, so check every one against the glossary source.
func TestGlossaryMarkupNamesRealTerms(t *testing.T) {
	src, err := os.ReadFile("../../../frontend/src/model/glossary.ts")
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, m := range regexp.MustCompile(`\{ id: '([a-z0-9-]+)'`).FindAllStringSubmatch(string(src), -1) {
		known[m[1]] = true
	}
	if len(known) < 25 {
		t.Fatalf("found only %d glossary terms in the frontend: the pattern no longer matches", len(known))
	}
	re := regexp.MustCompile(`\{\{([^}|]*)(?:\|[^}]*)?\}\}`)
	used := 0
	check := func(where, text string) {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			used++
			if !known[m[1]] {
				t.Errorf("%s: {{%s}} is not a glossary term", where, m[1])
			}
		}
	}
	for _, sc := range All() {
		check(sc.ID+" story", sc.Story)
		check(sc.ID+" debrief", sc.Debrief)
		for _, h := range sc.Hints {
			check(sc.ID+" hint", h)
		}
	}
	if used == 0 {
		t.Errorf("no challenge text uses the glossary markup")
	}
}
