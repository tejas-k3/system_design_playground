package engine

import "testing"

func TestLostUpdatesGoalFailsOnReadCommittedAndPassesOnSerializable(t *testing.T) {
	zero := 0
	g := Goals{MinSuccessRate: 0.9, MaxLostUpdates: &zero}
	check := func(iso string) Check {
		d := transferDesign(iso, 60, 60)
		sc := Evaluate(g, d, run(d, Options{Seed: 5}, 30).RunSnapshot(), 1)
		for _, c := range sc.Checks {
			if c.Label == "Lost updates" {
				return c
			}
		}
		t.Fatal("no Lost updates check in the score")
		return Check{}
	}
	if c := check("readCommitted"); c.Pass {
		t.Fatalf("read committed loses updates on 60 accounts: %+v", c)
	}
	if c := check("serializable"); !c.Pass || c.Actual != "0" {
		t.Fatalf("serializable loses none: %+v", c)
	}
}
