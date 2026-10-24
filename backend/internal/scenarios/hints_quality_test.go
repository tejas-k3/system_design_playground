package scenarios

import (
	"regexp"
	"strings"
	"testing"
)

// Hint 3 is a pointed question or a narrowing of where to look. It must not hand over the setting, the number or the
// component set: that text lives in Scenario.Answer, behind its own click.

var hintUnit = regexp.MustCompile(`(?i)\d\s*-?\s*(%|(ms|rps|s|x|mb|gb|kb|threads?|servers?|shards?|replicas?)\b)`)

var hintOrders = []string{"try ", "set ", "turn on", "add ", "use ", "run ", "put "}

// hint3Allowed lists challenges whose hint 3 may break the rules above because a rewrite would be dishonest. Each
// entry needs a reason. It is empty on purpose: every hint 3 has been rewritten.
var hint3Allowed = map[string]string{}

func TestHintThreeDoesNotGiveTheAnswer(t *testing.T) {
	for _, sc := range All() {
		if _, ok := hint3Allowed[sc.ID]; ok {
			continue
		}
		h := sc.Hints[2]
		if h == "" {
			t.Errorf("%s: no hint 3", sc.ID)
			continue
		}
		if m := hintUnit.FindString(h); m != "" {
			t.Errorf("%s: hint 3 names a number with a unit (%q): %s", sc.ID, m, h)
		}
		low := strings.ToLower(h)
		for _, p := range hintOrders {
			if strings.HasPrefix(low, p) {
				t.Errorf("%s: hint 3 starts with an instruction (%q): %s", sc.ID, p, h)
			}
		}
	}
}

func TestEveryChallengeHasAnAnswerHint(t *testing.T) {
	for _, sc := range All() {
		a := sc.AnswerHint()
		if strings.TrimSpace(a) == "" {
			t.Errorf("%s: no answer hint", sc.ID)
			continue
		}
		if !strings.HasPrefix(a, "The answer: ") {
			t.Errorf("%s: the answer hint should start with %q: %s", sc.ID, "The answer: ", a)
		}
		if a == sc.Hints[2] {
			t.Errorf("%s: the answer hint repeats hint 3", sc.ID)
		}
	}
}

func TestHintThreeAllowListIsHonest(t *testing.T) {
	for id, why := range hint3Allowed {
		if Get(id) == nil {
			t.Errorf("allow-list names an unknown challenge %q", id)
		}
		if strings.TrimSpace(why) == "" {
			t.Errorf("allow-list entry %q has no reason", id)
		}
	}
}
