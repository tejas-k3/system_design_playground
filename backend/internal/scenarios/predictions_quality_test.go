package scenarios

import (
	"testing"
	"unicode/utf8"
)

// The wording of the options must not give the answer away. A test-wise learner picks the longest option, the
// hedged one or the only specific one; these checks keep the right option from being a tell.

func optionLengths(q *Prediction) (longest, shortest, answer int, answerIsLongest bool) {
	shortest = 1 << 30
	for _, o := range q.Options {
		n := utf8.RuneCountInString(o)
		longest = max(longest, n)
		shortest = min(shortest, n)
	}
	answer = utf8.RuneCountInString(q.Options[q.Answer])
	answerIsLongest = true
	for i, o := range q.Options {
		if i != q.Answer && utf8.RuneCountInString(o) >= answer {
			answerIsLongest = false
		}
	}
	return
}

func TestTheRightOptionIsRarelyTheLongest(t *testing.T) {
	total, longestCount := 0, 0
	for _, sc := range All() {
		for i := range sc.Predictions {
			total++
			if _, _, _, isLongest := optionLengths(&sc.Predictions[i]); isLongest {
				longestCount++
			}
		}
	}
	if total < 70 {
		t.Fatalf("expected the full set of predictions, found %d", total)
	}
	share := float64(longestCount) / float64(total)
	t.Logf("the right option is strictly the longest in %d of %d predictions (%.0f%%)", longestCount, total, share*100)
	if share > 0.40 {
		t.Errorf("the right option is strictly the longest in %.0f%% of predictions, want at most 40%%", share*100)
	}
}

func TestOptionsOfAPredictionHaveComparableLength(t *testing.T) {
	for _, sc := range All() {
		for i := range sc.Predictions {
			q := &sc.Predictions[i]
			longest, shortest, _, _ := optionLengths(q)
			if float64(longest) > 1.6*float64(shortest) {
				t.Errorf("%s/%s: longest option is %d characters, shortest %d (more than 1.6x)", sc.ID, q.ID, longest, shortest)
			}
		}
	}
}
