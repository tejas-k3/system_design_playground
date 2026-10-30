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
