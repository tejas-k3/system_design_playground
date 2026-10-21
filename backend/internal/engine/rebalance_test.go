package engine

import "testing"

// Adding a shard under load. The keys that change shard have to be fetched from their old home while the data moves,
// so for rebalanceSec every request for a moved key is slower by rebalanceMs.
func rebalanceRun(hashing string, window float64) *Sim {
	build := func(count int) Diagram {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 4000.0, "keyDist": "uniform", "timeoutMs": 5000.0})
		b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 200.0})
		rp := Params{"hashing": hashing, "vnodes": 128.0, "rebalanceSec": window, "rebalanceMs": 400.0}
		b.node("sr", KShardRouter, rp)
		b.edge("u", "a").edge("a", "sr")
		for i := 0; i < count; i++ {
			id := "db" + string(rune('a'+i))
			b.node(id, KDatabase, Params{"connections": 32.0, "readMs": 5.0})
			b.edge("sr", id)
		}
		d, _ := Sanitize(b.d)
		return d
	}
	s := New(build(4), Options{Seed: 3})
	s.Advance(10000)
	s.Sync(build(5))
	s.Advance(30000)
	return s
}

func TestMovedKeysAreSlowOnlyWhileTheDataMoves(t *testing.T) {
	s := rebalanceRun("mod", 15)
	var during, after, n1, n2 float64
	for _, p := range s.Snapshot().Series {
		switch {
		case p.T > 12 && p.T < 24:
			during += p.P50
			n1++
		case p.T > 30:
			after += p.P50
			n2++
		}
	}
	during, after = during/n1, after/n2
	t.Logf("median latency: %.0f ms while moving, %.0f ms after", during, after)
	if during < after+200 {
		t.Errorf("most requests wait for a moved key while the data moves: %.0f vs %.0f", during, after)
	}
}

func TestARingSlowsFarFewerRequestsThanMod(t *testing.T) {
	slowed := func(h string) float64 {
		s := rebalanceRun(h, 15)
		return s.Snapshot().Nodes["sr"].Extra["slowedByMove"]
	}
	mod, ring := slowed("mod"), slowed("ring")
	t.Logf("requests slowed by the move: %.0f with mod, %.0f with a ring", mod, ring)
	if mod < 3*ring || ring == 0 {
		t.Errorf("about 4 in 5 requests move with mod and 1 in 5 with a ring: %.0f vs %.0f", mod, ring)
	}
}

func TestNoRebalanceWindowMeansNoCost(t *testing.T) {
	s := rebalanceRun("mod", 0)
	if v := s.Snapshot().Nodes["sr"].Extra["slowedByMove"]; v != 0 {
		t.Errorf("rebalanceSec 0 is the old instant move: %.0f", v)
	}
}
