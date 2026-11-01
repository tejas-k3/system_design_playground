package engine

import "testing"

// Reads that follow friends of friends touch more and more rows: each hop multiplies the rows by the number of friends.
// A relational database joins them at a price per row, a graph database follows pointers at a tenth of it, and a cap on
// the neighbours followed bounds the work whatever the model.
func graphRun(client, db Params) RunStats {
	cl := Params{"rps": 40.0, "readPct": 100.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 3000.0}
	for k, v := range client {
		cl[k] = v
	}
	dp := Params{"connections": 16.0, "readMs": 2.0}
	for k, v := range db {
		dp[k] = v
	}
	b := &builder{}
	b.node("u", KClient, cl).node("a", KAppServer, Params{"threads": 64.0}).node("db", KDatabase, dp)
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 5})
	s.Advance(20000)
	return s.RunSnapshot()
}

func TestEachHopMultipliesTheRowsAReadTouches(t *testing.T) {
	none := graphRun(nil, nil)
	one := graphRun(Params{"friendHops": 1.0, "friendsEach": 100.0}, nil)
	two := graphRun(Params{"friendHops": 2.0, "friendsEach": 100.0}, nil)
	t.Logf("mean read: %.1f ms none, %.1f ms one hop, %.1f ms two hops (ok %d/%d)", none.Mean, one.Mean, two.Mean, two.OK, two.Resolved)
	if !(none.Mean < one.Mean && one.Mean < two.Mean && two.Mean-none.Mean > 20*(one.Mean-none.Mean)) {
		t.Errorf("a second hop reaches about 100 times as many rows: %.1f, %.1f, %.1f", none.Mean, one.Mean, two.Mean)
	}
}

func TestAGraphDatabaseFollowsPointersCheaperThanJoins(t *testing.T) {
	c := Params{"friendHops": 2.0, "friendsEach": 100.0}
	rel, gr := graphRun(c, nil), graphRun(c, Params{"model": "graph"})
	base := graphRun(nil, nil).Mean
	t.Logf("mean read: %.1f ms relational, %.1f ms graph (%.1f ms without hops)", rel.Mean, gr.Mean, base)
	if gr.Mean-base >= (rel.Mean-base)/5 {
		t.Errorf("following pointers should cost far less than joining: %.1f vs %.1f", gr.Mean, rel.Mean)
	}
}

func TestCappingTheNeighboursFollowedBoundsTheWork(t *testing.T) {
	c := Params{"friendHops": 2.0, "friendsEach": 400.0}
	all, capped := graphRun(c, nil), graphRun(c, Params{"graphCap": 40.0})
	t.Logf("mean read: %.1f ms following every friend, %.1f ms at most 40 per hop", all.Mean, capped.Mean)
	if base := graphRun(nil, nil).Mean; capped.Mean-base >= (all.Mean-base)/10 {
		t.Errorf("40 per hop touches 1,640 rows instead of 160,400: %.1f vs %.1f", capped.Mean, all.Mean)
	}
}

func TestNoHopsChangesNothing(t *testing.T) {
	a, b := graphRun(nil, nil), graphRun(Params{"friendHops": 0.0, "friendsEach": 500.0}, Params{"graphCap": 10.0})
	if a.OK != b.OK || a.Mean != b.Mean {
		t.Errorf("without hops the other settings are inert: %.2f vs %.2f", a.Mean, b.Mean)
	}
}

// A read replica has no graph model and no cap, so reads sent to it pay the full join price.
func TestAReplicaPaysTheJoinPriceForEveryFriend(t *testing.T) {
	run := func(withReplica bool) float64 {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 40.0, "readPct": 100.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 3000.0, "friendHops": 2.0, "friendsEach": 100.0})
		b.node("a", KAppServer, Params{"threads": 64.0, "readRouting": "nearest"}).node("db", KDatabase, Params{"connections": 16.0, "readMs": 2.0, "model": "graph", "graphCap": 10.0})
		b.edge("u", "a").edge("a", "db")
		if withReplica {
			b.node("r", KReplica, Params{"connections": 16.0, "readMs": 2.0, "replicationLagMs": 10.0}).edge("a", "r")
		}
		d, _ := Sanitize(b.d)
		s := New(d, Options{Seed: 5})
		s.Advance(20000)
		return s.RunSnapshot().Mean
	}
	plain, repl := run(false), run(true)
	t.Logf("mean read: %.1f ms on the capped graph database, %.1f ms with a replica taking the reads", plain, repl)
	if repl <= plain+10 {
		t.Errorf("the replica joins 10,100 rows a read, so it must not be cheaper: %.1f vs %.1f", repl, plain)
	}
}
