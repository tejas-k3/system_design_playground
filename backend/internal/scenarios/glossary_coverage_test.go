package scenarios

import (
	"regexp"
	"strings"
	"testing"
)

var markupRe = regexp.MustCompile(`\{\{([a-z0-9-]+)(?:\|([^}]+))?\}\}`)

// challengeTexts lists the fields the interface renders through the glossary markup renderer (Rich): the story,
// the three hints, the answer, the debrief and the deeper explanation. Prediction texts are shown as plain
// text, so they carry no markup.
func challengeTexts(sc *Scenario) map[string]string {
	return map[string]string{
		"story": sc.Story, "answer": sc.Answer, "debrief": sc.Debrief, "deeper": sc.Deeper,
		"hint1": sc.Hints[0], "hint2": sc.Hints[1], "hint3": sc.Hints[2],
	}
}

// notYetMarked lists challenges whose text lives outside the defs_*.go files (scenarios_open.go) and so was not
// marked up together with the others. Delete an entry once its text carries a term.
var notYetMarked = map[string]bool{"ten-thousand-students": true, "checkout-during-a-sale": true}

func TestGlossaryCoverage(t *testing.T) {
	distinct := map[string]bool{}
	for _, sc := range All() {
		marked := false
		for field, text := range challengeTexts(sc) {
			where := sc.ID + " " + field
			seen := map[string]bool{}
			for _, m := range markupRe.FindAllStringSubmatch(text, -1) {
				distinct[m[1]] = true
				if seen[m[1]] {
					t.Errorf("%s: term %s is marked twice", where, m[1])
				}
				seen[m[1]] = true
				if field == "story" || field == "debrief" {
					marked = true
				}
			}
			if s := markupRe.ReplaceAllString(text, ""); strings.Contains(s, "{{") || strings.Contains(s, "}}") {
				t.Errorf("%s: stray braces after stripping the markup", where)
			}
		}
		if !marked && !notYetMarked[sc.ID] {
			t.Errorf("%s: no glossary term is marked in the story or the debrief", sc.ID)
		}
	}
	if len(distinct) < 30 {
		t.Errorf("only %d distinct glossary terms are marked across the challenge texts, want at least 30", len(distinct))
	}
}
