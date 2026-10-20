package engine

import "testing"

// A shard router over n databases, grown by one database while it runs.
func reshard(hashing string, vnodes float64, n int) (before, after *Sim, moved float64) {
	build := func(count int) Diagram {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 50.0, "keySpace": 4000.0, "keyDist": "uniform"})
		b.node("a", KAppServer, Params{"threads": 16.0})
		rp := Params{}
		if hashing != "" {
			rp["hashing"] = hashing
			rp["vnodes"] = vnodes
		}
		b.node("sr", KShardRouter, rp)
		b.edge("u", "a").edge("a", "sr")
		for i := 0; i < count; i++ {
			id := "db" + string(rune('a'+i))
			b.node(id, KDatabase, Params{"connections": 16.0})
			b.edge("sr", id)
		}
		d, _ := Sanitize(b.d)
		return d
	}
	s := New(build(n), Options{Seed: 3})
	s.Advance(2000)
	s.Sync(build(n + 1))
	s.Advance(1000)
	return s, s, s.Snapshot().Nodes["sr"].Extra["keysMoved"]
}

func TestModHashingMovesMostKeysWhenAShardIsAdded(t *testing.T) {
	_, _, moved := reshard("", 0, 4)
	t.Logf("mod, 4 -> 5 shards: %.0f%% of keys moved", moved*100)
	if moved < 0.7 {
		t.Errorf("hash mod n should remap about 4 keys in 5, moved %.2f", moved)
	}
}

func TestRingMovesAboutOneNthOfTheKeys(t *testing.T) {
	_, _, moved := reshard("ring", 128, 4)
	t.Logf("ring, 4 -> 5 shards: %.0f%% of keys moved", moved*100)
	if moved > 0.3 || moved < 0.1 {
		t.Errorf("a ring should move about 1/5 of the keys, moved %.2f", moved)
	}
}

func TestMoreVirtualNodesEvenOutTheRing(t *testing.T) {
	spread := func(v float64) float64 {
		b := &builder{}
		b.node("sr", KShardRouter, Params{"hashing": "ring", "vnodes": v})
		for i := 0; i < 6; i++ {
			id := "db" + string(rune('a'+i))
			b.node(id, KDatabase, Params{})
			b.edge("sr", id)
		}
		d, _ := Sanitize(b.d)
		s := New(d, Options{Seed: 1})
		return s.ringImbalance(s.nodes["sr"], 20000)
	}
	one, many := spread(1), spread(200)
	t.Logf("busiest shard over the mean: %.2fx with 1 point each, %.2fx with 200", one, many)
	if many > 1.2 || one < 1.5*many {
		t.Errorf("virtual nodes should even the load out: %.2f with 1, %.2f with 200", one, many)
	}
}

func TestNoHashingSettingKeepsTheOldMapping(t *testing.T) {
	a, _, _ := reshard("", 0, 3)
	b, _, _ := reshard("mod", 0, 3)
	if a.Snapshot().Nodes["sr"].Extra["keysMoved"] != b.Snapshot().Nodes["sr"].Extra["keysMoved"] {
		t.Errorf("mod must be the default mapping")
	}
}
