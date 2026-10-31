package engine

import "testing"

func TestAGoalCanLimitMessagesFinishedOutOfOrder(t *testing.T) {
	max := 5
	g := Goals{MinSuccessRate: 0.5, MaxOrderViolations: &max}
	rr := run(logDesign(4, "roundRobin", nil), Options{Seed: 2}, 40).RunSnapshot()
	keyed := run(logDesign(4, "key", nil), Options{Seed: 2}, 40).RunSnapshot()
	if find(Evaluate(g, Diagram{}, rr, 1).Checks, "Messages handled out of order").Pass {
		t.Errorf("round robin overtakes: %d", rr.Jobs.OrderViolations)
	}
	if !find(Evaluate(g, Diagram{}, keyed, 1).Checks, "Messages handled out of order").Pass {
		t.Errorf("a key stays in order: %d", keyed.Jobs.OrderViolations)
	}
}

func find(cs []Check, label string) Check {
	for _, c := range cs {
		if c.Label == label {
			return c
		}
	}
	return Check{Label: "missing"}
}
