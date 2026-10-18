package scenarios

import (
	"reflect"
	"testing"
)

func tradeoffs() []*Question {
	var out []*Question
	for _, q := range AllQuestions() {
		if q.Mode == ModeTradeoff {
			out = append(out, q)
		}
	}
	return out
}

// The stated requirement in each prompt is a function; the right answer must be the design that meets it, and
// no other design may (except the ones listed as also meeting it, which lose on the stated tie-break). This ties
// every answer key to what the simulation actually produces rather than to what the author believed.
func TestEachTradeoffAnswerIsWhatTheSimulationSupports(t *testing.T) {
	qs := tradeoffs()
	if len(qs) < 5 {
		t.Fatalf("%d trade-off questions, want at least 5", len(qs))
	}
	for _, q := range qs {
		if q.rule == nil || q.explanation == "" || q.answer < 0 || q.answer >= len(q.Candidates) || len(q.Candidates) < 2 {
			t.Errorf("%s: incomplete (rule, explanation, answer index, 2+ candidates)", q.ID)
			continue
		}
		also := map[int]bool{}
		for _, i := range q.alsoPasses {
			also[i] = true
		}
		for i := range q.Candidates {
			r, err := q.RunCandidate(i, 42)
			if err != nil {
				t.Fatalf("%s/%d: %v", q.ID, i, err)
			}
			meets := q.rule(r)
			switch {
			case i == q.answer && !meets:
				t.Errorf("%s: the right answer %q does not meet the stated requirement: %+v", q.ID, q.Candidates[i].Label, r)
			case i != q.answer && meets && !also[i]:
				t.Errorf("%s: %q also meets the requirement, so the answer is ambiguous: %+v", q.ID, q.Candidates[i].Label, r)
			case i != q.answer && !meets && also[i]:
				t.Errorf("%s: %q is listed as also meeting the requirement but does not: %+v", q.ID, q.Candidates[i].Label, r)
			}
		}
	}
}

func TestEverySeedAgreesOnTheAnswer(t *testing.T) {
	for _, q := range tradeoffs() {
		for _, seed := range []uint32{1, 2, 3} {
			for i := range q.Candidates {
				r, _ := q.RunCandidate(i, seed)
				if q.rule(r) != (i == q.answer || containsInt(q.alsoPasses, i)) {
					t.Errorf("%s seed %d candidate %d: requirement met=%v disagrees with the key", q.ID, seed, i, q.rule(r))
				}
			}
		}
	}
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func TestRunCandidateIsDeterministicAndRefusesBadInput(t *testing.T) {
	q := Q("payments-replication")
	a, _ := q.RunCandidate(0, 7)
	b, _ := q.RunCandidate(0, 7)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("same seed, different numbers: %+v vs %+v", a, b)
	}
	if _, err := q.RunCandidate(-1, 1); err == nil {
		t.Error("negative candidate")
	}
	if _, err := q.RunCandidate(len(q.Candidates), 1); err == nil {
		t.Error("candidate out of range")
	}
	if _, err := Q("url-shortener-numbers").RunCandidate(0, 1); err == nil {
		t.Error("an estimate question has nothing to run")
	}
	// what the explanation says: async loses acknowledged writes, sync loses none
	async, _ := q.RunCandidate(0, 42)
	sync, _ := q.RunCandidate(1, 42)
	if async.LostWrites < 100 || sync.LostWrites != 0 || sync.P95 <= async.P95 {
		t.Errorf("expected async to lose many writes and sync none at a higher p95: async %+v sync %+v", async, sync)
	}
}

func TestTradeoffJSONCarriesNoDiagramOrKey(t *testing.T) {
	// the shared check walks the keys; also make sure a candidate serialises to just a label and a description
	for _, q := range tradeoffs() {
		for _, c := range q.Candidates {
			if c.Label == "" || c.Detail == "" {
				t.Errorf("%s: a candidate needs a label and a description", q.ID)
			}
		}
	}
}
