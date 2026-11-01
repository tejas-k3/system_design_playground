package engine

import "testing"

func ledgerRun(retries float64) (RunStats, *Sim) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": 0.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 4000.0, "retries": 0.0})
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 100.0, "workflow": "saga", "compensationRetries": retries})
	b.node("stock", KDatabase, Params{"connections": 32.0, "writeMs": 10.0})
	b.node("pay", KExternalAPI, Params{"latencyMs": 40.0, "errorPct": 30.0})
	b.node("ship", KExternalAPI, Params{"latencyMs": 30.0, "errorPct": 20.0})
	b.edge("u", "a").edge("a", "stock").edge("a", "pay").edge("a", "ship")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 22})
	s.Advance(30000)
	return s.RunSnapshot(), s
}

// The books balance when every step that moved money has its counterpart: an entry left half done, or a saved write
// whose event never went out, is one that does not. (A write applied twice is the duplicates goal's business.)
func TestUnbalancedEntriesAddUpTheWaysABookGoesWrong(t *testing.T) {
	r, _ := ledgerRun(0)
	if r.Unbalanced != r.Inconsistent+r.EventsLost || r.Unbalanced == 0 {
		t.Errorf("unbalanced %d should be %d half done + %d unannounced", r.Unbalanced, r.Inconsistent, r.EventsLost)
	}
}

func TestTheLedgerGoalPassesOnlyWhenTheBooksBalance(t *testing.T) {
	limit := 5
	goals := Goals{MinSuccessRate: 0, MaxUnbalanced: &limit}
	verdict := func(retries float64) Check {
		_, s := ledgerRun(retries)
		for _, c := range Evaluate(goals, Diagram{}, s.RunSnapshot(), 0).Checks {
			if c.Label == "Entries that do not balance" {
				return c
			}
		}
		t.Fatal("no ledger check in the verdict")
		return Check{}
	}
	if c := verdict(0); c.Pass {
		t.Errorf("stranded orders unbalance the books: %+v", c)
	}
	if c := verdict(5); !c.Pass {
		t.Errorf("retried undo calls leave the books balanced: %+v", c)
	}
}
