package engine

import "testing"

// twoClients builds an author and a reader that share one key-value store, both asking for the given model.
func twoClients(authorsModel, readersModel string) (*Sim, *node, *node, *node) {
	d := Diagram{Nodes: []Node{
		{ID: "authors", Kind: KClient, Params: Params{"consistency": authorsModel, "rps": 50.0, "readPct": 0.0, "keySpace": 1.0, "keyDist": "uniform"}},
		{ID: "readers", Kind: KClient, Params: Params{"consistency": readersModel, "rps": 50.0, "readPct": 100.0, "keySpace": 1.0, "keyDist": "uniform"}},
		{ID: "kv", Kind: KKVStore},
	}}
	d, _ = Sanitize(d)
	s := New(d, Options{Seed: 1})
	return s, s.nodes["authors"], s.nodes["readers"], s.nodes["kv"]
}

func readOf(client *node, key int) *logical { return &logical{client: client, counted: true, key: key} }

// What makes causal order more than monotonic reads: a reader who never saw the cause is held to it once it has seen an
// effect built on it, because the author's write remembers what the author had read.
func TestAReaderIsHeldToTheCauseOfAnEffectItSaw(t *testing.T) {
	run := func(readersModel string) string {
		s, authors, readers, kv := twoClients("causalOrder", readersModel)
		s.countSession(readOf(authors, 1), Result{OK: true, Ver: 5}) // the author reads item 1 at version 5
		post := &logical{id: 7, client: authors, write: true, counted: true, key: 2}
		authors.stampSeen(post)
		effect := s.commit(kv, &call{lg: post, key: 2}) // and writes item 2 because of it
		s.countSession(readOf(readers, 2), Result{OK: true, Ver: effect})
		s.countSession(readOf(readers, 1), Result{OK: true, Ver: 4}) // the reader then finds item 1 older than that
		return sessionViolation(s)
	}
	if got := run("causalOrder"); got != "1" {
		t.Errorf("a reader that saw the effect and then an older cause should count once, got %s", got)
	}
	if got := run("monotonic"); got != "0" {
		t.Errorf("monotonic reads alone cannot see this, got %s", got)
	}
}

// A write is built on what its author had read when the write was issued, not on whatever it read while the write was in flight.
func TestAWriteRemembersTheReadItWasIssuedAfter(t *testing.T) {
	s, authors, readers, kv := twoClients("causalOrder", "causalOrder")
	s.countSession(readOf(authors, 1), Result{OK: true, Ver: 5})
	post := &logical{id: 7, client: authors, write: true, counted: true, key: 2}
	authors.stampSeen(post)                                      // issued after reading item 1 at 5 ...
	s.countSession(readOf(authors, 3), Result{OK: true, Ver: 9}) // ... then the author reads item 3 while the write is in flight
	effect := s.commit(kv, &call{lg: post, key: 2})
	if dep := s.causalDeps[[2]int{2, effect}]; dep != [2]int{1, 5} {
		t.Errorf("the write should depend on item 1 at version 5, got %v", dep)
	}
	_ = readers
}

// A read of an item that was never written (version 0) is no cause worth recording.
func TestReadingAnUnwrittenItemLeavesNoDependency(t *testing.T) {
	s, authors, _, kv := twoClients("causalOrder", "causalOrder")
	s.countSession(readOf(authors, 1), Result{OK: true, Ver: 0})
	post := &logical{id: 7, client: authors, write: true, counted: true, key: 2}
	authors.stampSeen(post)
	s.commit(kv, &call{lg: post, key: 2})
	if len(s.causalDeps) != 0 {
		t.Errorf("version 0 cannot be older than anything: %v", s.causalDeps)
	}
}

// The dependency records of a long run must not grow for ever: a dependency only matters while its version can still be read.
func TestCausalDependenciesAreKeptOnlyForRecentVersions(t *testing.T) {
	d := Diagram{Nodes: []Node{
		{ID: "users", Kind: KClient, Params: Params{"consistency": "causalOrder", "rps": 400.0, "readPct": 50.0, "keySpace": 20.0, "keyDist": "uniform", "timeoutMs": 3000.0}},
		{ID: "app", Kind: KAppServer, Params: Params{"threads": 64.0, "dbPool": 200.0}},
		{ID: "kv", Kind: KKVStore, Params: Params{"nodes": 5.0, "replicas": 3.0, "readQuorum": 1.0, "writeQuorum": 1.0, "readMs": 4.0, "writeMs": 20.0, "concurrency": 64.0}},
	}, Edges: []Edge{{ID: "a", From: "users", To: "app", LatencyMs: 20}, {ID: "b", From: "app", To: "kv", LatencyMs: 2}}}
	d, _ = Sanitize(d)
	s := New(d, Options{Seed: 1, StopTrafficAtMs: 60000})
	for !s.Finished() && s.Now() < 120000 {
		s.Advance(100)
	}
	if n := len(s.causalDeps); n == 0 || n > causalKeepMax {
		t.Errorf("after about 12,000 writes the record holds %d dependencies, want some but at most %d", n, causalKeepMax)
	}
}
