package engine

import "testing"

// Clients "ua" and "ub" both write one key through an app server into a 5-node, 3-copy store (W=2, R=2); an optional
// third client "uc" reads and writes it too. All use the same key, so their writes are concurrent unless the writer
// has read what the other wrote.
func conflictRun(kv Params, withReader bool) *Sim {
	b := &builder{}
	w := func(rps, readPct float64) Params {
		return Params{"rps": rps, "readPct": readPct, "keySpace": 1.0, "keyDist": "uniform", "timeoutMs": 3000.0}
	}
	b.node("ua", KClient, w(20, 0)).node("ub", KClient, w(20, 0))
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 200.0})
	p := Params{"nodes": 5.0, "replicas": 3.0, "readMs": 4.0, "writeMs": 10.0, "variability": "exponential", "concurrency": 32.0}
	for k, v := range kv {
		p[k] = v
	}
	b.node("kv", KKVStore, p)
	b.edge("ua", "a").edge("ub", "a").edge("a", "kv")
	if withReader {
		b.node("uc", KClient, w(20, 50))
		b.edge("uc", "a")
	}
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 11})
	s.Advance(30000)
	return s
}

func kvExtraOf(s *Sim, key string) float64 { return s.Snapshot().Nodes["kv"].Extra[key] }

// Hand calculation of the vector rules.
func TestVersionVectorRules(t *testing.T) {
	a1 := kvVV{{1, 1}}
	b1 := kvVV{{2, 1}}
	a2 := a1.bump(1)
	ab := a1.merge(b1)
	if len(a2) != 1 || a2[0] != (kvVVEnt{1, 2}) {
		t.Fatalf("bump: %v", a2)
	}
	if !a2.dominates(a1) || a1.dominates(a2) {
		t.Errorf("{a:2} dominates {a:1} and not the other way round")
	}
	if a1.dominates(b1) || b1.dominates(a1) {
		t.Errorf("{a:1} and {b:1} are concurrent")
	}
	if !ab.dominates(a1) || !ab.dominates(b1) || len(ab) != 2 {
		t.Errorf("merge {a:1}+{b:1} = {a:1,b:1} dominates both: %v", ab)
	}
	if !ab.bump(1).dominates(ab) {
		t.Errorf("a write that has seen both siblings dominates them")
	}
	// the sibling set after concurrent writes: two siblings; after a write that saw both: one
	set := kvAddSibling(nil, kvSib{vv: a1, ver: 1})
	set = kvAddSibling(set, kvSib{vv: b1, ver: 2})
	if len(set) != 2 {
		t.Fatalf("concurrent writes must both be kept: %v", set)
	}
	set = kvAddSibling(set, kvSib{vv: ab.bump(3), ver: 3})
	if len(set) != 1 || set[0].ver != 3 {
		t.Errorf("the resolving write replaces both: %v", set)
	}
	// a stale write (already dominated) is dropped, and adding the same sibling twice is idempotent
	set = kvAddSibling(set, kvSib{vv: a1, ver: 1})
	set = kvAddSibling(set, kvSib{vv: set[0].vv, ver: 3})
	if len(set) != 1 {
		t.Errorf("a dominated or duplicate write must not add a sibling: %v", set)
	}
}

// Two clients write the same key without reading. Last-write-wins keeps the newest and silently drops the other's:
// with two equally fast writers the next writer differs from the previous one half of the time, and then it has not
// seen the value it replaces, so about half of all writes destroy an update. With siblings nothing is dropped.
func TestLastWriteWinsLosesConcurrentUpdatesSiblingsKeepThem(t *testing.T) {
	lww := conflictRun(nil, false)
	sib := conflictRun(Params{"siblings": true}, false)
	writes := float64(lww.nodes["kv"].kx.writes)
	lost := kvExtraOf(lww, "lostUpdates")
	t.Logf("last write wins: %.0f writes, %.0f lost updates (expected about half = %.0f)", writes, lost, writes/2)
	if writes < 500 {
		t.Fatalf("expected around 1200 writes in 30 s at 40 rps, got %.0f", writes)
	}
	if lost < writes/2*0.85 || lost > writes/2*1.15 {
		t.Errorf("lost updates should be about half the writes within 15%%: %.0f of %.0f", lost, writes)
	}
	t.Logf("siblings: %.0f lost updates, %.0f open siblings at the end, %.0f writes that met a concurrent one",
		kvExtraOf(sib, "lostUpdates"), kvExtraOf(sib, "siblingsOpen"), kvExtraOf(sib, "concurrentWrites"))
	if kvExtraOf(sib, "lostUpdates") != 0 {
		t.Errorf("keeping siblings must lose no update")
	}
	// nobody reads, so each writer's newest write stays: exactly two siblings (one per writer)
	if got := kvExtraOf(sib, "siblingsOpen"); got != 2 {
		t.Errorf("two writers that never read leave exactly 2 siblings of the key, got %.0f", got)
	}
	if kvExtraOf(sib, "concurrentWrites") < 100 {
		t.Errorf("concurrent writes should be counted")
	}
	if kvExtraOf(sib, "siblingsResolved") != 0 {
		t.Errorf("nobody saw the siblings, so none can be resolved")
	}
}

// A client that reads the siblings and then writes has seen them: its write replaces them.
func TestReadingTheSiblingsAndWritingResolvesThem(t *testing.T) {
	s := conflictRun(Params{"siblings": true}, true)
	seen, reads, resolved := kvExtraOf(s, "siblingsSeen"), kvExtraOf(s, "siblingReads"), kvExtraOf(s, "siblingsResolved")
	t.Logf("with a reading client: %.0f reads returned siblings (%.0f siblings in all), %.0f resolutions, %.0f lost updates",
		reads, seen, resolved, kvExtraOf(s, "lostUpdates"))
	if reads == 0 || seen < 2*reads {
		t.Errorf("a read that finds siblings returns at least two: %.0f reads, %.0f siblings", reads, seen)
	}
	if resolved == 0 {
		t.Errorf("a write that has read the siblings must resolve them")
	}
	if kvExtraOf(s, "lostUpdates") != 0 {
		t.Errorf("no update may be lost")
	}
}

// The switch is off by default and changes nothing about what clients see.
func TestSiblingsOffIsTheDefault(t *testing.T) {
	a := conflictRun(nil, true).RunSnapshot()
	b := conflictRun(Params{"siblings": false}, true).RunSnapshot()
	if a.OK != b.OK || a.P50 != b.P50 || a.Stale != b.Stale || a.Fresh != b.Fresh {
		t.Errorf("explicit off must equal the default: %+v vs %+v", a, b)
	}
	// and switching it on does not change latency or availability either, only what is kept
	c := conflictRun(Params{"siblings": true}, true).RunSnapshot()
	t.Logf("OK %d (off) vs %d (on); p50 %.2f vs %.2f", a.OK, c.OK, a.P50, c.P50)
	if a.OK != c.OK {
		t.Errorf("siblings must not change availability: %d vs %d", a.OK, c.OK)
	}
}

// Siblings survive an outage: a node misses writes while down, hints and repair rounds hand them over, and in the
// end every copy of the key holds exactly the set a copy that had seen every write would hold (the two writers'
// newest writes), with nothing lost.
func TestSiblingsConvergeAfterAnOutage(t *testing.T) {
	b := &builder{}
	w := Params{"rps": 20.0, "readPct": 0.0, "keySpace": 1.0, "keyDist": "uniform", "timeoutMs": 3000.0}
	b.node("ua", KClient, w).node("ub", KClient, w)
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 200.0})
	b.node("kv", KKVStore, Params{"nodes": 5.0, "replicas": 3.0, "writeMs": 10.0, "concurrency": 32.0, "siblings": true, "hintedHandoff": true, "antiEntropySec": 1.0})
	b.edge("ua", "a").edge("ub", "a").edge("a", "kv")
	d, _ := Sanitize(b.d)
	chaos := []ChaosEvent{{Type: "kvNodeDown", NodeID: "kv", AtSec: 5, DurationSec: 10, Factor: 2}}
	s := New(d, Options{Seed: 11, Chaos: chaos, StopTrafficAtMs: 20000})
	s.Advance(40000)
	n := s.nodes["kv"]
	key, truth := 0, []kvSib(nil)
	for k, v := range n.kx.truth { // one key in use
		key, truth = k, v
	}
	t.Logf("%d writes, %d lost, truth holds %d siblings", n.kx.writes, n.kx.lost, len(truth))
	if len(truth) != 2 || n.kx.lost != 0 {
		t.Fatalf("two writers leave two siblings and lose nothing: %d, lost %d", len(truth), n.kx.lost)
	}
	first := kvModFirst(key, 5)
	for j := 0; j < 3; j++ {
		set := n.kx.sib[(first+j)%5][key]
		if len(set) != 2 {
			t.Errorf("copy %d holds %d siblings, want 2", j, len(set))
			continue
		}
		for _, x := range truth {
			found := false
			for _, y := range set {
				found = found || y.ver == x.ver
			}
			if !found {
				t.Errorf("copy %d lacks the write of version %d", j, x.ver)
			}
		}
	}
}
