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

// maxUnmarked bounds how many challenges may show no tooltip in their story or debrief. A tooltip that explains the
// wrong idea is worse than none, so a challenge where no term honestly fits has none (the open briefs, whose text lives
// in scenarios_open.go, and a few others); raise this only when a challenge truly has no fitting term.
const maxUnmarked = 24

func TestGlossaryCoverage(t *testing.T) {
	distinct := map[string]bool{}
	var unmarked []string
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
		if !marked {
			unmarked = append(unmarked, sc.ID)
		}
	}
	if len(unmarked) > maxUnmarked {
		t.Errorf("%d challenges show no glossary term in the story or the debrief (allowed %d): %v", len(unmarked), maxUnmarked, unmarked)
	}
	if len(distinct) < 30 {
		t.Errorf("only %d distinct glossary terms are marked across the challenge texts, want at least 30", len(distinct))
	}
}
