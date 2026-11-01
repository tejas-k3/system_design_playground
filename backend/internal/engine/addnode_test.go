package engine

import "testing"

// A shard router over four databases gets a fifth at 10 s, as a scheduled event rather than a live edit.
func addNodeRun(hashing string) *Sim {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 4000.0, "keyDist": "uniform", "timeoutMs": 5000.0})
	b.node("a", KAppServer, Params{"threads": 256.0, "queueCap": 2000.0, "dbPool": 200.0})
	b.node("sr", KShardRouter, Params{"hashing": hashing, "vnodes": 128.0, "rebalanceSec": 15.0, "rebalanceMs": 400.0})
	b.edge("u", "a").edge("a", "sr")
	for i := range 4 {
		id := "db" + string(rune('a'+i))
		b.node(id, KDatabase, Params{"connections": 32.0, "readMs": 5.0})
		b.edge("sr", id)
	}
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 3, Chaos: []ChaosEvent{{Type: "addNode", NodeID: "sr", AtSec: 10}}})
	s.Advance(40000)
	return s
}

func TestAnAddNodeEventGivesTheRouterAFifthShardAndMovesKeys(t *testing.T) {
	s := addNodeRun("mod")
	ex := s.Snapshot().Nodes["sr"].Extra
	t.Logf("keys moved %.2f, %.0f requests slowed", ex["keysMoved"], ex["slowedByMove"])
	if len(shardChildren(s.nodes["sr"])) != 5 {
		t.Fatalf("the router should have five shards, has %d", len(shardChildren(s.nodes["sr"])))
	}
	if ex["keysMoved"] < 0.7 {
		t.Errorf("mod remaps about 4 keys in 5: %.2f", ex["keysMoved"])
	}
	if s.Snapshot().Nodes["dba-new1"].Arr == 0 {
		t.Errorf("the new shard should be taking requests")
	}
}

func TestARingMovesAndSlowsFarFewerThanMod(t *testing.T) {
	mod, ring := addNodeRun("mod"), addNodeRun("ring")
	m, r := mod.Snapshot().Nodes["sr"].Extra["slowedByMove"], ring.Snapshot().Nodes["sr"].Extra["slowedByMove"]
	t.Logf("requests slowed by the move: %.0f with mod, %.0f with a ring", m, r)
	if r == 0 || m < 3*r {
		t.Errorf("a ring slows about a fifth as many requests: %.0f vs %.0f", m, r)
	}
}

func TestWithoutTheEventTheRouterKeepsFourShards(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 10.0})
	b.node("a", KAppServer, Params{})
	b.node("sr", KShardRouter, nil)
	b.edge("u", "a").edge("a", "sr")
	for i := range 4 {
		id := "db" + string(rune('a'+i))
		b.node(id, KDatabase, nil)
		b.edge("sr", id)
	}
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 3})
	s.Advance(20000)
	if n := len(shardChildren(s.nodes["sr"])); n != 4 {
		t.Errorf("%d shards", n)
	}
}
