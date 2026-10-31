package scenarios

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

var sentenceEnd = regexp.MustCompile(`[.?!]( |$)`)

func TestQuorumOverlapExplainsBothQuorumRules(t *testing.T) {
	sc := Get("quorum-overlap")
	for name, text := range map[string]string{"debrief": sc.Debrief, "deeper": sc.Deeper} {
		if !strings.Contains(text, "latest completed write") {
			t.Errorf("%s must limit the read guarantee to completed writes", name)
		}
		if !strings.Contains(text, "W > N/2") {
			t.Errorf("%s must explain that write quorums exceed half of N", name)
		}
		if strings.Contains(strings.ToLower(text), "never goes backwards") {
			t.Errorf("%s must not claim quorums alone prevent reads going backwards", name)
		}
	}
}

// Every Hard challenge carries a longer debrief for an experienced engineer, shown on request next to the
// ordinary one: trade-offs, what changes at scale, what an interviewer asks next.
func TestEveryHardChallengeHasADeeperDebrief(t *testing.T) {
	placeholders := []string{"todo", "tbd", "fixme", "lorem", "placeholder", "xxx", "coming soon"}
	hard := 0
	for _, sc := range All() {
		if sc.Difficulty != "Hard" {
			if sc.Deeper != "" && utf8.RuneCountInString(sc.Deeper) < 200 {
				t.Errorf("%s: a deeper debrief, when present, should be substantial", sc.ID)
			}
			continue
		}
		hard++
		d := sc.Deeper
		if n := utf8.RuneCountInString(d); n < 200 {
			t.Errorf("%s: deeper debrief has %d characters, want at least 200", sc.ID, n)
			continue
		}
		low := strings.ToLower(d)
		for _, p := range placeholders {
			if strings.Contains(low, p) {
				t.Errorf("%s: deeper debrief contains placeholder text %q", sc.ID, p)
			}
		}
		if n := len(sentenceEnd.FindAllString(d, -1)); n < 3 || n > 6 {
			t.Errorf("%s: deeper debrief has %d sentences, want 3 to 6", sc.ID, n)
		}
		if d == sc.Debrief || strings.Contains(d, sc.Debrief) {
			t.Errorf("%s: deeper debrief repeats the debrief", sc.ID)
		}
	}
	if hard != 25 {
		t.Errorf("expected 25 Hard challenges, found %d", hard)
	}
}
