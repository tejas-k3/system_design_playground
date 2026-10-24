package engine

import (
	"math"
	"testing"
)

// Some of the requests ask for things that do not exist (a crawler's already-seen check, a lookup of a deleted id).
// Every one of them used to reach the database to be told "nothing here". A Bloom filter in front of it holds every
// id that does exist: an id it has never seen is answered at once, and only the filter's false positives go on to the
// database. The false-positive rate after n ids in m bits with k hashes is (1 - e^(-kn/m))^k.
func guardRun(app Params) (*Sim, RunStats) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 500.0, "readPct": 100.0, "keySpace": 10000.0, "keyDist": "uniform", "missingPct": 50.0, "timeoutMs": 2000.0, "retries": 0.0})
	ap := Params{"threads": 64.0, "queueCap": 1000.0}
	for k, v := range app {
		ap[k] = v
	}
	b.node("a", KAppServer, ap)
	b.node("db", KDatabase, Params{"connections": 64.0, "readMs": 2.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 8})
	guardDB = 0
	for i := 0; i < 120; i++ { // add up the database's answers second by second
		s.Advance(1000)
		guardDB += s.Snapshot().Nodes["db"].Ok
	}
	return s, s.RunSnapshot()
}

var guardDB float64

func TestABloomGuardKeepsAbsentIdsOffTheDatabase(t *testing.T) {
	_, _ = guardRun(nil)
	plainReads := guardDB
	guarded, _ := guardRun(Params{"bloomGuard": true, "bloomBitsPerKey": 5.0, "bloomHashes": 3.0})
	guardedReads := guardDB
	ex := guarded.Snapshot().Nodes["a"].Extra
	want := math.Pow(1-math.Exp(-3.0/5.0), 3) // k = 3 hashes, 5 bits per stored id
	got := ex["bloomFalsePositivePct"] / 100
	t.Logf("database reads: %.0f without the filter, %.0f with it; false-positive rate %.4f measured, %.4f by the formula", plainReads, guardedReads, got, want)
	if math.Abs(got-want)/want > 0.10 {
		t.Errorf("measured false-positive rate is more than 10%% from the formula: %.4f vs %.4f", got, want)
	}
	// half the requests are for absent ids; all but the false positives stop at the filter
	expected := plainReads * (0.5 + 0.5*want)
	if math.Abs(guardedReads-expected)/expected > 0.05 {
		t.Errorf("the database should see the real ids plus the false positives: %.0f vs %.0f", guardedReads, expected)
	}
}

func TestBloomGuardOffChangesNothing(t *testing.T) {
	_, a := guardRun(nil)
	_, b := guardRun(Params{"bloomGuard": false})
	if a.OK != b.OK || a.P95 != b.P95 {
		t.Errorf("off must equal the default: %d vs %d", a.OK, b.OK)
	}
}
