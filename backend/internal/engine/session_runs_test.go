package engine

import (
	"strings"
	"testing"
)

func dnsRun(ttl float64) (RunStats, float64) {
	d, _ := Sanitize(Diagram{Nodes: []Node{
		{ID: "users", Kind: KClient, Params: Params{"rps": 60.0, "readPct": 100.0, "keySpace": 200.0, "timeoutMs": 1000.0, "dnsTtlSec": ttl}},
		{ID: "a", Kind: KAppServer, Params: Params{"threads": 64.0}},
		{ID: "b", Kind: KAppServer, Params: Params{"threads": 64.0}},
		{ID: "db", Kind: KDatabase, Params: Params{"connections": 64.0}},
	}, Edges: []Edge{{ID: "users>a", From: "users", To: "a"}, {ID: "users>b", From: "users", To: "b"}, {ID: "a>db", From: "a", To: "db"}, {ID: "b>db", From: "b", To: "db"}}})
	s := New(d, Options{Seed: 42, StopTrafficAtMs: 40000, Chaos: []ChaosEvent{{Type: "crash", NodeID: "a", AtSec: 5, DownSec: 60}}})
	for !s.Finished() && s.Now() < 45000 {
		s.Advance(100)
	}
	return s.RunSnapshot(), s.CostPerHour()
}

func TestDNSStaleAddressesExpireAndShorterTTLResolvesMoreOften(t *testing.T) {
	long, _ := dnsRun(30)
	short, _ := dnsRun(2)
	plain, _ := dnsRun(0)
	if long.Errors["staleDNS"] <= short.Errors["staleDNS"] || short.Errors["staleDNS"] == 0 {
		t.Fatalf("stale address failures long=%d short=%d", long.Errors["staleDNS"], short.Errors["staleDNS"])
	}
	if short.DNSLookups <= long.DNSLookups {
		t.Fatalf("lookups short=%d long=%d", short.DNSLookups, long.DNSLookups)
	}
	if plain.DNSLookups != 0 || plain.Errors["staleDNS"] != 0 || plain.Errors["nodeDown"] == 0 {
		t.Fatalf("TTL 0 changed today's routing: %+v", plain)
	}
}

func TestDNSLookupCostIsIncludedInHourlyCost(t *testing.T) {
	short, shortCost := dnsRun(2)
	long, longCost := dnsRun(30)
	if short.DNSLookups <= long.DNSLookups {
		t.Fatal("short TTL should resolve more often")
	}
	if short.DNSLookups == 0 || long.DNSLookups == 0 || shortCost <= longCost {
		t.Fatalf("lookups/cost: short=%d/$%.5f, long=%d/$%.5f", short.DNSLookups, shortCost, long.DNSLookups, longCost)
	}
}

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

// numericRun is sessionRun with any extra settings on the users.
func numericRun(extra Params, quorums []float64) RunStats {
	d := Diagram{Nodes: []Node{
		{ID: "users", Kind: KClient, Params: Params{"rps": 200.0, "readPct": 50.0, "keySpace": 20.0, "keyDist": "uniform", "timeoutMs": 3000.0}},
		{ID: "app", Kind: KAppServer, Params: Params{"threads": 32.0, "dbPool": 200.0}},
		{ID: "kv", Kind: KKVStore, Params: Params{"nodes": 5.0, "replicas": 3.0, "readQuorum": quorums[0], "writeQuorum": quorums[1], "readMs": 4.0, "writeMs": 60.0, "variability": "exponential", "concurrency": 32.0}},
	}, Edges: []Edge{{ID: "a", From: "users", To: "app", LatencyMs: 40}, {ID: "b", From: "app", To: "kv", LatencyMs: 2}}}
	for k, v := range extra {
		d.Nodes[0].Params[k] = v
	}
	d, _ = Sanitize(d)
	s := New(d, Options{Seed: 1, StopTrafficAtMs: 40000})
	for !s.Finished() && s.Now() < 100000 {
		s.Advance(100)
	}
	return s.RunSnapshot()
}

// A numerical bound counts a read as too stale when it misses more committed writes of the item than allowed.
func TestBoundedByMissedWritesCountsReadsThatMissTooMany(t *testing.T) {
	bound := func(max float64, q []float64) RunStats {
		return numericRun(Params{"consistency": "boundedWrites", "maxMissedWrites": max}, q)
	}
	strict, lax, quorum := bound(0, []float64{1, 1}), bound(3, []float64{1, 1}), bound(0, []float64{2, 2})
	if strict.Fresh == 0 || strict.Stale == 0 {
		t.Fatalf("at R=W=1 and a bound of 0 some reads must miss a write: %d of %d", strict.Stale, strict.Fresh)
	}
	if lax.Stale >= strict.Stale {
		t.Errorf("allowing 3 missed writes should count fewer reads than allowing none: %d against %d", lax.Stale, strict.Stale)
	}
	if quorum.Stale != 0 {
		t.Errorf("R=W=2 over three copies reads every acknowledged write, so none can be too stale: %d", quorum.Stale)
	}
}

// With no bound chosen the old behaviour is untouched: an eventual client counts nothing.
func TestEventualClientsAreStillNotJudgedForFreshness(t *testing.T) {
	if r := numericRun(nil, []float64{1, 1}); r.Fresh != 0 || r.Stale != 0 {
		t.Errorf("an eventual client was judged: %d of %d", r.Stale, r.Fresh)
	}
}

// In a real run, effects are seen before their causes when a read can land on a copy that has not caught up.
func TestCausalOrderBreaksOnLaggingCopiesAndQuorumsReduceIt(t *testing.T) {
	causal := func(q []float64) int {
		return numericRun(Params{"consistency": "causalOrder"}, q).CausalBroken
	}
	loose, strict := causal([]float64{1, 1}), causal([]float64{2, 2})
	if loose == 0 {
		t.Fatal("at R=W=1 some read must show an effect and then an older cause")
	}
	if strict >= loose {
		t.Errorf("overlapping quorums should reduce it: %d at R=W=2 against %d at R=W=1", strict, loose)
	}
}
