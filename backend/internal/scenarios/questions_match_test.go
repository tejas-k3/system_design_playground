package scenarios

import (
	"encoding/json"
	"strings"
	"testing"
)

func matchQuestion(t *testing.T) *Question {
	t.Helper()
	q := Q("what-does-each-box-do")
	if q == nil || q.Mode != ModeMatch {
		t.Fatal("the warm-up question is not registered as a match question")
	}
	return q
}

func TestMatchWarmUpHasTwelveItemsOfFourDistinctOptions(t *testing.T) {
	q := matchQuestion(t)
	if len(q.Items) != 12 {
		t.Fatalf("%d items, want 12", len(q.Items))
	}
	for i, it := range q.Items {
		seen := map[string]bool{}
		for _, o := range it.Options {
			if seen[o] {
				t.Errorf("item %d repeats the option %q", i, o)
			}
			seen[o] = true
		}
		if len(it.Options) != 4 {
			t.Errorf("item %d has %d options, want 4", i, len(it.Options))
		}
		if it.answer < 0 || it.answer >= len(it.Options) {
			t.Errorf("item %d has no valid answer", i)
		}
		if strings.TrimSpace(it.why) == "" {
			t.Errorf("item %d has no explanation", i)
		}
	}
}

// Every component the brief names must be the right answer at least once, so the set covers the lot.
func TestMatchWarmUpCoversTheEightComponentsInTheBrief(t *testing.T) {
	q := matchQuestion(t)
	right := map[string]bool{}
	for _, it := range q.Items {
		right[it.Options[it.answer]] = true
	}
	for _, c := range []string{"Cache", "Database", "Message Queue", "Load Balancer", "CDN", "Object Store", "Read Replica", "Rate Limiter"} {
		if !right[c] {
			t.Errorf("no item has %q as its answer", c)
		}
	}
}

// The right option must not be guessable from where it sits or how long it is.
func TestMatchWarmUpAnswersAreNotGuessableByPositionOrLength(t *testing.T) {
	q := matchQuestion(t)
	pos := map[int]int{}
	longest := 0
	for _, it := range q.Items {
		pos[it.answer]++
		top := true
		for i, o := range it.Options {
			if i != it.answer && len(o) >= len(it.Options[it.answer]) {
				top = false
			}
		}
		if top {
			longest++
		}
	}
	for p, n := range pos {
		if n > 4 {
			t.Errorf("%d of 12 answers sit in position %d", n, p+1)
		}
	}
	if len(pos) < 4 {
		t.Errorf("answers use only %d of 4 positions", len(pos))
	}
	if longest > 4 {
		t.Errorf("the right option is the longest in %d of 12 items", longest)
	}
}

func TestMatchQuestionIsServedWithoutItsAnswers(t *testing.T) {
	b, err := json.Marshal(matchQuestion(t))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	items, _ := m["items"].([]any)
	if len(items) != 12 {
		t.Fatalf("served %d items", len(items))
	}
	for _, it := range items {
		for k := range it.(map[string]any) {
			if k != "need" && k != "options" {
				t.Errorf("an item is served with the field %q", k)
			}
		}
	}
	for _, it := range matchQuestion(t).Items {
		if strings.Contains(string(b), it.why) {
			t.Errorf("the reason %q is served before answering", it.why)
		}
	}
}

func TestGradingMatchStarsAndReveal(t *testing.T) {
	q := matchQuestion(t)
	choices := func(wrong int) []int {
		c := make([]int, len(q.Items))
		for i, it := range q.Items {
			c[i] = it.answer
			if i < wrong {
				c[i] = (it.answer + 1) % len(it.Options)
			}
		}
		return c
	}
	for _, c := range []struct{ wrong, correct, stars int }{{0, 12, 3}, {1, 11, 3}, {2, 10, 2}, {4, 8, 2}, {5, 7, 1}, {6, 6, 1}, {7, 5, 0}, {12, 0, 0}} {
		r, err := q.GradeMatch(choices(c.wrong))
		if err != nil {
			t.Fatal(err)
		}
		if r.Correct != c.correct || r.Stars != c.stars {
			t.Errorf("%d wrong: %d correct, %d stars; want %d and %d", c.wrong, r.Correct, r.Stars, c.correct, c.stars)
		}
		if len(r.Items) != 12 || r.Items[0].Explain == "" {
			t.Error("the result must carry the answer and the reason for every item")
		}
	}
}

func TestGradingMatchRefusesAMalformedSubmission(t *testing.T) {
	q := matchQuestion(t)
	if _, err := q.GradeMatch([]int{0, 1}); err == nil {
		t.Error("too few choices must be refused, not graded as wrong")
	}
	bad := make([]int, len(q.Items))
	bad[3] = 4
	if _, err := q.GradeMatch(bad); err == nil {
		t.Error("an option that does not exist must be refused")
	}
	bad[3] = -1
	if _, err := q.GradeMatch(bad); err == nil {
		t.Error("a negative choice must be refused")
	}
	if _, err := Q("daily-storage-estimate").GradeMatch(nil); err == nil {
		t.Error("only a match question can be graded this way")
	}
}
