package scenarios

import (
	"math"
	"testing"

	"sysdesign/internal/engine"
)

// Every stored alternative is run again from the challenge's starting design, and what is shown to the learner must
// match what the engine says now: the pass or fail, and the numbers to the digits that are displayed.
func TestAlternativesMatchTheEngine(t *testing.T) {
	total := 0
	for id, alts := range alternatives {
		sc := Get(id)
		if sc == nil {
			t.Errorf("alternatives for unknown challenge %q", id)
			continue
		}
		for _, a := range alts {
			total++
			if a.Label == "" || len(a.Changes) == 0 {
				t.Errorf("%s: an alternative needs a label and at least one change", id)
				continue
			}
			res := engine.RunToEnd(applyChanges(sc.Diagram, a.Changes), sc.Options(42), sc.Goals)
			if res.Pass != a.Pass {
				t.Errorf("%s / %s: stored pass=%v, engine says %v (%+v)", id, a.Label, a.Pass, res.Pass, res.Checks)
			}
			success := math.Floor(100*float64(res.Run.OK)/math.Max(1, float64(res.Run.Resolved+res.Run.Pending))*10) / 10
			if math.Abs(success-a.SuccessPct) > 0.05 || math.Abs(math.Round(res.Run.P95)-a.P95Ms) > 0.5 || math.Abs(math.Round(res.CostPerHour*100)/100-a.CostPerHour) > 0.005 {
				t.Errorf("%s / %s: stored %.1f%% p95 %.0f ms $%.2f, engine %.1f%% p95 %.0f ms $%.2f", id, a.Label, a.SuccessPct, a.P95Ms, a.CostPerHour, success, math.Round(res.Run.P95), res.CostPerHour)
			}
		}
	}
	if total < 12 {
		t.Errorf("only %d alternatives stored", total)
	}
}

func TestAlternativesNameRealNodes(t *testing.T) {
	// a change that names a node or setting the design does not have would silently do nothing
	for id, alts := range alternatives {
		sc := Get(id)
		for _, a := range alts {
			for _, c := range a.Changes {
				found := false
				for _, n := range sc.Diagram.Nodes {
					if n.ID == c.Node {
						found = true
					}
				}
				if !found {
					t.Errorf("%s / %s: no node %q", id, a.Label, c.Node)
				}
				if c.Param == "" {
					t.Errorf("%s / %s: empty setting name", id, a.Label)
				}
			}
		}
	}
}
