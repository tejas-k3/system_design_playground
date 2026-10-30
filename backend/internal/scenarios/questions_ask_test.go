package scenarios

import (
	"encoding/json"
	"strings"
	"testing"

	"sysdesign/internal/engine"
)

func asks() []*Question {
	var out []*Question
	for _, q := range AllQuestions() {
		if q.Mode == ModeAsk {
			out = append(out, q)
		}
	}
	return out
}

// The one design every prompt is sized against: a balancer, four servers, a small cache and a database with a
// small connection pool. It is tight on purpose, so each decisive question moves it across the pass line.
func askDesign(users P) engine.Diagram {
	return (&bld{}).n("users", engine.KClient, cols[0], rowMid, users).
		n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).e("users", "lb").
		n("db", engine.KDatabase, cols[4], rowMid, P{"connections": 6.0}).
		n("cache", engine.KCache, cols[3], rowTop, P{"ttlSec": 60.0, "capacity": 2000.0}).
		servers(4, cols[2], rowTop, 70, P{"threads": 16.0}, "cache").done()
}

func askPasses(v askVariant, seed uint32) bool {
	users := P{"rps": 400.0, "readPct": 90.0, "keySpace": 100000.0, "keyDist": "zipf", "zipfS": 1.0}
	for k, val := range v.users {
		users[k] = val
	}
	goals := engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(400)}
	if v.goals != nil {
		v.goals(&goals)
	}
	sc := &Scenario{DurationSec: 40, WarmupSec: 5, Speed: 1}
	return engine.RunToEnd(askDesign(users), sc.Options(seed), goals).Pass
}

func TestThereAreSixRequirementsPromptsOfEightQuestionsEach(t *testing.T) {
	qs := asks()
	if len(qs) != 6 {
		t.Fatalf("%d prompts, want 6", len(qs))
	}
	for _, q := range qs {
		if len(q.Asks) != 8 {
			t.Errorf("%s: %d candidate questions, want 8", q.ID, len(q.Asks))
		}
		d := 0
		for _, a := range q.Asks {
			if a.decisive {
				d++
			}
			if strings.TrimSpace(a.Text) == "" || strings.TrimSpace(a.why) == "" {
				t.Errorf("%s: a question or its reason is empty", q.ID)
			}
		}
		if d != 2 || q.Picks != d+1 {
			t.Errorf("%s: %d decisive and %d picks; want 2 decisive and one spare pick", q.ID, d, q.Picks)
		}
	}
}

// Every decisive question is tied to a real setting: with the same design, one answer passes and the other does not.
func TestEveryDecisiveQuestionFlipsTheOutcomeAndNoOtherDoes(t *testing.T) {
	for _, q := range asks() {
		for _, a := range q.Asks {
			if !a.decisive {
				if a.flip != nil {
					t.Errorf("%s: %q is not decisive but claims a flip", q.ID, a.Text)
				}
				continue
			}
			if a.flip == nil || strings.TrimSpace(a.flip.says) == "" {
				t.Errorf("%s: %q is decisive but shows no flip", q.ID, a.Text)
				continue
			}
			for _, seed := range []uint32{1, 2, 3} {
				if !askPasses(a.flip.a, seed) || askPasses(a.flip.b, seed) {
					t.Errorf("%s: %q seed %d: one answer should pass and the other fail", q.ID, a.Text, seed)
				}
			}
		}
	}
}

// Where the decisive questions sit must not give them away.
func TestDecisiveQuestionsAreSpreadOverThePositions(t *testing.T) {
	at := map[int]int{}
	for _, q := range asks() {
		for i, a := range q.Asks {
			if a.decisive {
				at[i]++
			}
		}
	}
	for pos, n := range at {
		if n > 2 {
			t.Errorf("%d of the 12 decisive questions sit at position %d", n, pos+1)
		}
	}
	if len(at) < 6 {
		t.Errorf("decisive questions use only %d of 8 positions", len(at))
	}
}

func TestPromptsAreServedWithoutWhichQuestionsMatter(t *testing.T) {
	for _, q := range asks() {
		b, _ := json.Marshal(q)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		for _, a := range m["asks"].([]any) {
			for k := range a.(map[string]any) {
				if k != "text" {
					t.Errorf("%s: a question is served with the field %q", q.ID, k)
				}
			}
		}
		for _, a := range q.Asks {
			if strings.Contains(string(b), a.why) {
				t.Errorf("%s: a reason is served before answering", q.ID)
			}
		}
	}
}

func decisiveAt(q *Question) (dec, other []int) {
	for i, a := range q.Asks {
		if a.decisive {
			dec = append(dec, i)
		} else {
			other = append(other, i)
		}
	}
	return
}

func TestGradingRewardsTheDecisiveQuestionsAndPenalisesWaste(t *testing.T) {
	q := asks()[0]
	dec, other := decisiveAt(q)
	pick := func(ids ...int) AskResult {
		r, err := q.GradeAsk(ids)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	for _, c := range []struct {
		name         string
		ids          []int
		found, waste int
		stars        int
	}{
		{"both, nothing wasted", []int{dec[0], dec[1]}, 2, 0, 3},
		{"both and one wasted", []int{dec[0], other[0], dec[1]}, 2, 1, 3},
		{"one only", []int{dec[0]}, 1, 0, 2},
		{"one and one wasted", []int{dec[1], other[0]}, 1, 1, 1},
		{"one and two wasted", []int{dec[1], other[0], other[1]}, 1, 2, 0},
		{"nothing useful", []int{other[0], other[1], other[2]}, 0, 3, 0},
	} {
		r := pick(c.ids...)
		if r.Found != c.found || r.Wasted != c.waste || r.Stars != c.stars {
			t.Errorf("%s: found %d wasted %d stars %d; want %d, %d, %d", c.name, r.Found, r.Wasted, r.Stars, c.found, c.waste, c.stars)
		}
	}
	r := pick(dec[0], other[0])
	if len(r.Items) != 8 || r.Missed != 1 {
		t.Fatalf("every question must come back with its reason, and the missed one counted: %d items, %d missed", len(r.Items), r.Missed)
	}
	if !r.Items[dec[1]].Decisive || r.Items[dec[1]].Chosen || r.Items[dec[1]].Flip == "" || r.Items[other[0]].Explain == "" {
		t.Errorf("a missed decisive question should be revealed with its flip: %+v", r.Items[dec[1]])
	}
}

func TestGradingRefusesAMalformedSelection(t *testing.T) {
	q := asks()[0]
	for name, ids := range map[string][]int{
		"none":         {},
		"too many":     {0, 1, 2, 3},
		"a repeat":     {1, 1},
		"out of range": {0, 8},
		"negative":     {-1},
	} {
		if _, err := q.GradeAsk(ids); err == nil {
			t.Errorf("%s: should be refused", name)
		}
	}
	if _, err := Q("daily-users-rps").GradeAsk([]int{0}); err == nil {
		t.Error("only a requirements prompt can be graded this way")
	}
}

// A decisive question without a flip is a mistake in the data; grading must not crash the server over it.
func TestGradingSurvivesADecisiveQuestionWithoutAFlip(t *testing.T) {
	q := &Question{ID: "t", Mode: ModeAsk, Picks: 2, Asks: []AskOption{{Text: "a", decisive: true, why: "x"}, {Text: "b", why: "y"}}}
	r, err := q.GradeAsk([]int{0})
	if err != nil || r.Found != 1 || r.Items[0].Flip != "" {
		t.Errorf("%+v %v", r, err)
	}
}
