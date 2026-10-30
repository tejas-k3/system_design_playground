package engine

import (
	"strings"
	"testing"
)

// sessionRun drives a real run: 200 requests a second, half of them writes, over 20 keys, through an app server
// to a key-value store with the given quorums (or to a plain database when kvQuorums is nil).
func sessionRun(model string, kvQuorums []float64) RunStats {
	store := Node{ID: "kv", Kind: KKVStore}
	if kvQuorums != nil {
		store.Params = Params{"nodes": 5.0, "replicas": 3.0, "readQuorum": kvQuorums[0], "writeQuorum": kvQuorums[1], "readMs": 4.0, "writeMs": 60.0, "variability": "exponential", "concurrency": 32.0}
	} else {
		store = Node{ID: "kv", Kind: KDatabase}
	}
	d := Diagram{Nodes: []Node{
		{ID: "users", Kind: KClient, Params: Params{"rps": 200.0, "readPct": 50.0, "keySpace": 20.0, "keyDist": "uniform", "consistency": model, "timeoutMs": 3000.0}},
		{ID: "app", Kind: KAppServer, Params: Params{"threads": 32.0, "dbPool": 200.0}},
		store,
	}, Edges: []Edge{{ID: "a", From: "users", To: "app", LatencyMs: 40}, {ID: "b", From: "app", To: "kv", LatencyMs: 2}}}
	d, _ = Sanitize(d)
	s := New(d, Options{Seed: 1, StopTrafficAtMs: 40000})
	for !s.Finished() && s.Now() < 100000 {
		s.Advance(100)
	}
	return s.RunSnapshot()
}

// Each new option can be broken in a real run, and breaks only its own counter.
func TestEachSessionModelBreaksOnlyItsOwnCounterInARealRun(t *testing.T) {
	for _, c := range []struct {
		model string
		own   func(RunStats) int
	}{
		{"readYourWrites", func(r RunStats) int { return r.ReadYourWritesBroken }},
		{"monotonic", func(r RunStats) int { return r.MonotonicBroken }},
		{"monotonicWrites", func(r RunStats) int { return r.MonoWritesBroken }},
		{"writesFollowReads", func(r RunStats) int { return r.FollowReadBroken }},
	} {
		r := sessionRun(c.model, []float64{1, 1})
		counts := map[string]int{"readYourWrites": r.ReadYourWritesBroken, "monotonic": r.MonotonicBroken, "monotonicWrites": r.MonoWritesBroken, "writesFollowReads": r.FollowReadBroken}
		for name, n := range counts {
			switch {
			case name == c.model && n == 0:
				t.Errorf("%s: a real run at R=W=1 never broke it", c.model)
			case name != c.model && n != 0:
				t.Errorf("%s: also counted %d against %s", c.model, n, name)
			}
		}
	}
}

// Monotonic writes measure the order writes commit against the order they were issued. Raising the quorums does
// not repair that, so no quorum setting is a lever for it; the help text says so.
func TestQuorumSettingsDoNotRepairWriteOrder(t *testing.T) {
	loose, strict := sessionRun("monotonicWrites", []float64{1, 1}).MonoWritesBroken, sessionRun("monotonicWrites", []float64{2, 2}).MonoWritesBroken
	if loose == 0 || strict == 0 {
		t.Errorf("write order broke %d times at R=W=1 and %d at R=W=2; the help text says quorums do not fix it, so both should be above zero", loose, strict)
	}
}

// Writes follow reads is judged against the copies of a key-value store; a plain database has none, so it counts nothing.
func TestWritesFollowReadsCountsNothingWithoutAKeyValueStore(t *testing.T) {
	if r := sessionRun("writesFollowReads", nil); r.FollowReadBroken != 0 {
		t.Errorf("counted %d violations on a plain database", r.FollowReadBroken)
	}
}

func TestTheConsistencyHelpStatesWhatTheNewOptionsCanAndCannotDo(t *testing.T) {
	var help string
	for _, p := range Spec(KClient).Params {
		if p.Key == "consistency" {
			help = p.Help
		}
	}
	help = strings.ToLower(help)
	for _, want := range []string{"key-value store", "quorum"} {
		if !strings.Contains(help, want) {
			t.Errorf("the consistency help does not mention %q: %s", want, help)
		}
	}
}
