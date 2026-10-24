package scenarios

import "testing"

// The options of a prediction are shown in a fixed shuffled order, so the stored order (which the authors wrote with the
// answer in the middle) is not what a player sees, and answering maps the shown position back.
func TestShownOrderIsAPermutationAndStable(t *testing.T) {
	for _, sc := range All() {
		for i := range sc.Predictions {
			q := &sc.Predictions[i]
			shown := sc.ShownOptions(q)
			if len(shown) != len(q.Options) {
				t.Fatalf("%s/%s: %d shown, %d stored", sc.ID, q.ID, len(shown), len(q.Options))
			}
			if shown[sc.ShownAnswer(q)] != q.Options[q.Answer] {
				t.Fatalf("%s/%s: ShownAnswer does not point at the stored answer", sc.ID, q.ID)
			}
			for pos := range shown {
				if shown[pos] != q.Options[sc.StoredIndex(q, pos)] {
					t.Fatalf("%s/%s: StoredIndex(%d) does not invert the shuffle", sc.ID, q.ID, pos)
				}
			}
			again := sc.ShownOptions(q)
			for pos := range shown {
				if again[pos] != shown[pos] {
					t.Fatalf("%s/%s: the shown order changed between calls", sc.ID, q.ID)
				}
			}
		}
	}
}

// A player must not be able to answer by position: no slot holds the answer in more than 45% of the questions.
func TestTheAnswerIsNotInOnePosition(t *testing.T) {
	at := map[int]int{}
	n := 0
	for _, sc := range All() {
		for i := range sc.Predictions {
			at[sc.ShownAnswer(&sc.Predictions[i])]++
			n++
		}
	}
	for pos, c := range at {
		if float64(c) > 0.45*float64(n) {
			t.Errorf("position %d holds the answer in %d of %d questions", pos, c, n)
		}
	}
	if len(at) < 3 {
		t.Errorf("the answer only ever appears in positions %v", at)
	}
}

func TestPublicPredictionsAreShuffled(t *testing.T) {
	sc := Get("who-is-the-leader")
	q := &sc.Predictions[0]
	pub := sc.Public().Predictions[0]
	for i, o := range sc.ShownOptions(q) {
		if pub.Options[i] != o {
			t.Fatalf("Public shows %q at %d, ShownOptions says %q", pub.Options[i], i, o)
		}
	}
}
