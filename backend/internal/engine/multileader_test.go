package engine

import (
	"math"
	"testing"
)

// Several leaders each accept writes for any key and send them to the others after a delay. Two writes to one key at
// different leaders within that delay are a conflict: last-writer-wins keeps one and silently drops the other (a lost
// update); a counter that merges keeps both.
func multiLeaderRun(db Params) *Sim {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 400.0, "readPct": 0.0, "keySpace": 200.0, "keyDist": "uniform", "timeoutMs": 3000.0})
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 200.0})
	p := Params{"connections": 64.0}
	for k, v := range db {
		p[k] = v
	}
	b.node("db", KDatabase, p)
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 9})
	s.Advance(40000)
	return s
}

func TestConflictRateMatchesTheory(t *testing.T) {
	// 400 writes/s over 200 keys is 2 per key per second; a write conflicts when the previous write of its key was
	// within the sync delay at another leader: P = (1 - 1/L) * (1 - exp(-rate * delay)).
	for _, c := range []struct{ leaders, lagMs float64 }{{2, 100}, {2, 400}, {3, 200}} {
		s := multiLeaderRun(Params{"topology": "multiLeader", "leaders": c.leaders, "syncLagMs": c.lagMs, "conflictPolicy": "lastWriterWins"})
		r := s.RunSnapshot()
		writes := float64(len(s.committed))
		got := float64(r.LostUpdates) / writes
		want := (1 - 1/c.leaders) * (1 - math.Exp(-2*c.lagMs/1000))
		t.Logf("%.0f leaders, %.0f ms: %.3f of writes lost, theory %.3f", c.leaders, c.lagMs, got, want)
		if math.Abs(got-want) > 0.2*want+0.01 {
			t.Errorf("conflict rate %.3f is far from the %.3f the arrival process predicts", got, want)
		}
	}
}

func TestMergingKeepsEveryUpdateAndASingleLeaderHasNoConflicts(t *testing.T) {
	lww := multiLeaderRun(Params{"topology": "multiLeader", "leaders": 2.0, "syncLagMs": 300.0, "conflictPolicy": "lastWriterWins"}).RunSnapshot()
	merge := multiLeaderRun(Params{"topology": "multiLeader", "leaders": 2.0, "syncLagMs": 300.0, "conflictPolicy": "merge"}).RunSnapshot()
	single := multiLeaderRun(nil).RunSnapshot()
	t.Logf("lost updates: last-writer-wins %d, merge %d, single leader %d", lww.LostUpdates, merge.LostUpdates, single.LostUpdates)
	if lww.LostUpdates == 0 {
		t.Errorf("concurrent writes at two leaders must lose updates under last-writer-wins")
	}
	if merge.LostUpdates != 0 || single.LostUpdates != 0 {
		t.Errorf("merging and a single leader lose nothing: %d, %d", merge.LostUpdates, single.LostUpdates)
	}
}

func TestSingleLeaderIsTheDefault(t *testing.T) {
	a := multiLeaderRun(nil).RunSnapshot()
	b := multiLeaderRun(Params{"topology": "singleLeader"}).RunSnapshot()
	if a.OK != b.OK || a.LostUpdates != 0 {
		t.Errorf("singleLeader must equal the default")
	}
}
