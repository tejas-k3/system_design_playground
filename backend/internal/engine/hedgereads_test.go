package engine

import "testing"

// Three identical read replicas whose service times have a heavy tail: most reads take 20 ms, a few take far longer.
// More capacity does not shorten the slow ones. Asking a second replica after a short wait, and taking whichever
// answers first, does, at the cost of a little extra load.
func hedgeRun(app Params, seed uint32) *Sim {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 150.0, "readPct": 100.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 3000.0})
	ap := Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 100.0}
	for k, v := range app {
		ap[k] = v
	}
	b.node("a", KAppServer, ap)
	b.node("db", KDatabase, Params{"connections": 16.0})
	b.edge("u", "a").edge("a", "db")
	for i := 0; i < 3; i++ {
		id := "r" + string(rune('1'+i))
		b.node(id, KReplica, Params{"connections": 16.0, "readMs": 20.0, "variability": "heavy", "replicationLagMs": 50.0})
		b.edge("a", id)
	}
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: seed})
	s.Advance(40000)
	return s
}

func TestHedgingACallToAnIdenticalReplicaCutsTheTail(t *testing.T) {
	base := hedgeRun(Params{"readSpread": "roundRobin"}, 31)
	hedged := hedgeRun(Params{"hedgeReadsMs": 60.0}, 31)
	b, h := base.RunSnapshot(), hedged.RunSnapshot()
	extra := float64(h.Hedged) / float64(h.Resolved)
	t.Logf("p95 %.0f ms -> %.0f ms, p99 %.0f -> %.0f ms; %d of %d reads hedged (%.1f%%)", b.P95, h.P95, b.P99, h.P99, h.Hedged, h.Resolved, extra*100)
	if h.P99 > 0.7*b.P99 {
		t.Errorf("a second replica should cut the p99 by at least 30%%: %.0f vs %.0f", h.P99, b.P99)
	}
	if extra < 0.01 || extra > 0.25 {
		t.Errorf("only the slow reads are hedged, a small share: %.3f", extra)
	}
}

func TestMoreThreadsDoNotShortenAServiceTimeTail(t *testing.T) {
	few := hedgeRun(Params{"readSpread": "roundRobin"}, 31).RunSnapshot()
	many := hedgeRun(Params{"readSpread": "roundRobin", "threads": 256.0}, 31).RunSnapshot()
	if many.P99 < 0.9*few.P99 {
		t.Errorf("capacity does not shorten the service-time tail: p99 %.0f vs %.0f", many.P99, few.P99)
	}
}

func TestHedgeReadsOffKeepsTheOldChoiceOfReplica(t *testing.T) {
	a := hedgeRun(nil, 31).RunSnapshot()
	b := hedgeRun(Params{"hedgeReadsMs": 0.0, "readSpread": "first"}, 31).RunSnapshot()
	if a.OK != b.OK || a.P95 != b.P95 || a.Hedged != 0 {
		t.Errorf("off must equal the default: %d/%.0f vs %d/%.0f", a.OK, a.P95, b.OK, b.P95)
	}
}
