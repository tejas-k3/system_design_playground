package scenarios

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNetworkAssumptions(t *testing.T) {
	q := Q("network-assumptions")
	if q == nil || q.Mode != ModeMatch || q.Difficulty != "Medium" {
		t.Fatal("network assumptions question is not registered")
	}
	if len(q.Items) < 8 || len(q.Items) > 12 {
		t.Fatalf("got %d items", len(q.Items))
	}
	positions := map[int]int{}
	answers := map[string]bool{}
	longest := 0
	for i, it := range q.Items {
		if len(it.Options) != 4 || it.answer < 0 || it.answer >= len(it.Options) {
			t.Errorf("item %d has invalid options or answer", i)
			continue
		}
		positions[it.answer]++
		answers[it.Options[it.answer]] = true
		if strings.TrimSpace(it.why) == "" {
			t.Errorf("item %d has no reason", i)
		}
		top := true
		for n, o := range it.Options {
			if n != it.answer && len(o) >= len(it.Options[it.answer]) {
				top = false
			}
		}
		if top {
			longest++
		}
		for _, bad := range []string{"also works", "also work", "would also", "can also"} {
			if strings.Contains(strings.ToLower(it.why), bad) {
				t.Errorf("item %d concedes another option", i)
			}
		}
	}
	for p, n := range positions {
		if n*5 > len(q.Items)*2 {
			t.Errorf("position %d holds %d of %d answers", p, n, len(q.Items))
		}
	}
	if len(positions) < 3 || longest*5 > len(q.Items)*2 {
		t.Errorf("answers are guessable: positions=%v longest=%d", positions, longest)
	}
	for _, assumption := range []string{"The network is reliable", "The network is secure", "The network is homogeneous", "The topology does not change", "Latency is zero", "Bandwidth is infinite", "Transport cost is zero", "There is one administrator"} {
		if !answers[assumption] {
			t.Errorf("no item tests %q", assumption)
		}
	}
	b, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(b, &got)
	for _, raw := range got["items"].([]any) {
		for k := range raw.(map[string]any) {
			if k != "need" && k != "options" {
				t.Errorf("served answer field %q", k)
			}
		}
	}
	choices := make([]int, len(q.Items))
	for i, it := range q.Items {
		choices[i] = it.answer
	}
	graded, err := q.GradeMatch(choices)
	if err != nil || graded.Correct != len(q.Items) || graded.Stars != 3 {
		t.Fatalf("grading = %+v, %v", graded, err)
	}
}
