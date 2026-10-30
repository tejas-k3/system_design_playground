package engine

import (
	"strings"
	"testing"
)

func TestCausalOptionIsNamedForItsSessionGuarantees(t *testing.T) {
	var help string
	var storedID bool
	for _, p := range Spec(KClient).Params {
		if p.Key != "consistency" {
			continue
		}
		help = p.Help
		for _, option := range p.Options {
			if option.Value == "causal" {
				storedID = true
				if option.Label != "Read your writes + monotonic reads" {
					t.Errorf("causal option label = %q", option.Label)
				}
			}
		}
	}
	if !storedID {
		t.Fatal("consistency option id causal must remain for saved designs")
	}
	if strings.Contains(strings.ToLower(help), "causal") {
		t.Errorf("consistency help still calls the session guarantees causal: %q", help)
	}
}

func TestSessionOptionsIncludeTheOtherClientGuarantees(t *testing.T) {
	options := map[string]bool{}
	for _, p := range Spec(KClient).Params {
		if p.Key == "consistency" {
			for _, option := range p.Options {
				options[option.Value] = true
			}
		}
	}
	for _, id := range []string{"monotonicWrites", "writesFollowReads"} {
		if !options[id] {
			t.Errorf("consistency option %q is missing", id)
		}
	}
}

func TestMonotonicWritesFollowTheClientIssueOrder(t *testing.T) {
	ordered := monotonicWriteViolations(t, []int{1, 2})
	reversed := monotonicWriteViolations(t, []int{2, 1})
	if ordered != "0" || reversed != "1" {
		t.Fatalf("ordered commits should have 0 violations and reversed commits 1, got %s and %s", ordered, reversed)
	}
}

func monotonicWriteViolations(t *testing.T, ids []int) string {
	t.Helper()
	s, client, kv := sessionSimulation("monotonicWrites", nil)
	for _, id := range ids {
		lg := &logical{id: id, client: client, write: true, counted: true, key: 1}
		ver := s.commit(kv, &call{lg: lg, key: lg.key})
		s.countSession(lg, Result{OK: true, Ver: ver})
	}
	return sessionViolation(s)
}

func TestWritesFollowReadsOnlyApplyToCopiesAtLeastAsNew(t *testing.T) {
	stale := writesFollowReadViolations(t, 4)
	current := writesFollowReadViolations(t, 5)
	if stale != "1" || current != "0" {
		t.Fatalf("writing to copies older than the read should count one violation, got %s; current copies got %s", stale, current)
	}
}

func writesFollowReadViolations(t *testing.T, copyVersion int) string {
	t.Helper()
	s, client, kv := sessionSimulation("writesFollowReads", Params{
		"nodes": 3.0, "replicas": 3.0, "readQuorum": 1.0, "writeQuorum": 1.0,
		"readMs": 0.1, "writeMs": 0.1, "variability": "steady",
	})
	client.down = true
	for i := 0; i < 5; i++ {
		s.bumpVer(1)
	}
	s.countSession(&logical{client: client, counted: true, key: 1}, Result{OK: true, Ver: 5})
	kv.kvEnsure(3, 1)
	for _, versions := range kv.kvVer {
		versions[1] = copyVersion
	}
	lg := &logical{id: 6, client: client, write: true, counted: true, key: 1, sawRead: client.sessRead[1]}
	var got Result
	s.procKV(kv, &call{lg: lg, write: true, key: 1, cmd: cmdWrite}, func(r Result) { got = r }, func() {})
	s.Advance(1)
	if !got.OK {
		t.Fatalf("write did not complete: %+v", got)
	}
	s.countSession(lg, got)
	return sessionViolation(s)
}

func sessionSimulation(model string, kv Params) (*Sim, *node, *node) {
	d := Diagram{Nodes: []Node{
		{ID: "users", Kind: KClient, Params: Params{"consistency": model, "rps": 50.0, "readPct": 0.0, "keySpace": 1.0, "keyDist": "uniform"}},
		{ID: "kv", Kind: KKVStore, Params: kv},
	}}
	d, _ = Sanitize(d)
	s := New(d, Options{Seed: 1})
	return s, s.nodes["users"], s.nodes["kv"]
}

func sessionViolation(s *Sim) string {
	zero := 0
	score := Evaluate(Goals{MaxSessionViolations: &zero}, Diagram{}, s.RunSnapshot(), 1)
	for _, check := range score.Checks {
		if check.Label == "Per-client guarantees broken" {
			return check.Actual
		}
	}
	return "missing"
}

// Per-client guarantees, counted against what the same client has already written or read. A client that keeps
// asking for its own latest data sees a violation when a read returns something older than what it knows.
func sessionCounts(model string, kv Params) RunStats {
	s := quorumRunWith(Params{"consistency": model}, kv, nil)
	return s.RunSnapshot()
}

func TestSessionViolationsFollowTheQuorums(t *testing.T) {
	loose := sessionCounts("causal", Params{"readQuorum": 1.0, "writeQuorum": 1.0})
	tight := sessionCounts("causal", Params{"readQuorum": 2.0, "writeQuorum": 2.0})
	t.Logf("R=W=1: %d own-write misses, %d going-backwards reads of %d reads; R=W=2: %d, %d of %d",
		loose.ReadYourWritesBroken, loose.MonotonicBroken, loose.SessionReads, tight.ReadYourWritesBroken, tight.MonotonicBroken, tight.SessionReads)
	if loose.SessionReads == 0 || tight.SessionReads == 0 {
		t.Fatalf("no session reads were counted")
	}
	if loose.ReadYourWritesBroken == 0 {
		t.Errorf("with R=W=1 a client's own write can be missed by its next read")
	}
	if loose.MonotonicBroken == 0 {
		t.Errorf("with R=W=1 consecutive reads can go back to an older version")
	}
	if tight.ReadYourWritesBroken != 0 || tight.MonotonicBroken != 0 {
		t.Errorf("overlapping quorums keep both guarantees: %d, %d", tight.ReadYourWritesBroken, tight.MonotonicBroken)
	}
}

func TestEachModelCountsOnlyItsOwnViolation(t *testing.T) {
	kv := Params{"readQuorum": 1.0, "writeQuorum": 1.0}
	ryw := sessionCounts("readYourWrites", kv)
	mono := sessionCounts("monotonic", kv)
	if ryw.ReadYourWritesBroken == 0 || ryw.MonotonicBroken != 0 {
		t.Errorf("readYourWrites counts only its own kind: %d, %d", ryw.ReadYourWritesBroken, ryw.MonotonicBroken)
	}
	if mono.MonotonicBroken == 0 || mono.ReadYourWritesBroken != 0 {
		t.Errorf("monotonic counts only its own kind: %d, %d", mono.ReadYourWritesBroken, mono.MonotonicBroken)
	}
	if ev := sessionCounts("eventual", kv); ev.SessionReads != 0 || ev.ReadYourWritesBroken != 0 {
		t.Errorf("eventual clients are not held to a session guarantee")
	}
}

// A replica that lags the primary misses the client's own write; routing reads by freshness to the primary keeps it.
func TestReplicaLagBreaksReadYourWritesUnlessReadsGoToThePrimary(t *testing.T) {
	run := func(routing string) RunStats {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 100.0, "readPct": 50.0, "keySpace": 20.0, "keyDist": "uniform", "consistency": "readYourWrites", "timeoutMs": 3000.0})
		b.node("a", KAppServer, Params{"threads": 64.0, "readRouting": routing})
		b.node("db", KDatabase, Params{"connections": 32.0})
		b.node("r", KReplica, Params{"connections": 32.0, "replicationLagMs": 800.0})
		b.edge("u", "a").edge("a", "db").edge("a", "r")
		d, _ := Sanitize(b.d)
		s := New(d, Options{Seed: 5})
		s.Advance(30000)
		return s.RunSnapshot()
	}
	near, byC := run("nearest"), run("byConsistency")
	t.Logf("own-write misses: %d with the nearest copy, %d with reads by freshness (of %d and %d reads)", near.ReadYourWritesBroken, byC.ReadYourWritesBroken, near.SessionReads, byC.SessionReads)
	if near.ReadYourWritesBroken == 0 {
		t.Errorf("a replica 800 ms behind should miss recent own writes")
	}
	if byC.ReadYourWritesBroken != 0 {
		t.Errorf("reading from the primary cannot miss an own write: %d", byC.ReadYourWritesBroken)
	}
}

func TestSessionGoalFailsOnLooseQuorumsAndPassesOnOverlapping(t *testing.T) {
	zero := 0
	g := Goals{MinSuccessRate: 0.5, MaxSessionViolations: &zero}
	check := func(r, w float64) Check {
		s := quorumRunWith(Params{"consistency": "causal"}, Params{"readQuorum": r, "writeQuorum": w}, nil)
		sc := Evaluate(g, Diagram{}, s.RunSnapshot(), 1)
		for _, c := range sc.Checks {
			if c.Label == "Per-client guarantees broken" {
				return c
			}
		}
		t.Fatal("no session check in the score")
		return Check{}
	}
	if c := check(1, 1); c.Pass {
		t.Fatalf("R=W=1 breaks the guarantees: %+v", c)
	}
	if c := check(2, 2); !c.Pass || c.Actual != "0" {
		t.Fatalf("R=W=2 keeps them: %+v", c)
	}
}
