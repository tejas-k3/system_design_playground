package engine

import "testing"

// An app server connected only to a cache, whose miss path leads to the database (app -> cache -> database). Reads
// already went through the cache; a write has to as well: the cache passes it on to the database and keeps its own
// copy current (write-through), instead of refusing it for want of a database the app server is wired to directly.
func chainRun(readPct float64, appToDB bool) (*Sim, RunStats) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": readPct, "keySpace": 500.0, "keyDist": "uniform", "timeoutMs": 2000.0, "retries": 0.0})
	b.node("a", KAppServer, Params{"threads": 32.0})
	b.node("c", KCache, Params{})
	b.node("db", KDatabase, Params{"connections": 32.0})
	b.edge("u", "a").edge("a", "c").edge("c", "db")
	if appToDB {
		b.edge("a", "db")
	}
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 2})
	s.Advance(30000)
	return s, s.RunSnapshot()
}

func TestWritesGoThroughACacheToTheDatabase(t *testing.T) {
	for _, rp := range []float64{0, 50} {
		_, chain := chainRun(rp, false)
		_, direct := chainRun(rp, true)
		t.Logf("reads %.0f%%: through the cache only: %d ok, %d failed %v; with a direct database edge too: %d ok", rp, chain.OK, chain.Failed, chain.Errors, direct.OK)
		if chain.Failed != 0 {
			t.Errorf("writes should reach the database through the cache: %d failed (%v)", chain.Failed, chain.Errors)
		}
	}
}

func TestAWriteThroughTheCacheLeavesTheCacheCurrent(t *testing.T) {
	s, r := chainRun(50, false)
	if r.Fresh > 0 && float64(r.Stale)/float64(r.Fresh) > 0.01 {
		t.Errorf("a write-through cache should not serve stale data after its own writes: %d of %d", r.Stale, r.Fresh)
	}
	if s.nodes["db"].c.ok == 0 && len(s.committed) == 0 {
		t.Errorf("the database should have taken the writes")
	}
}
