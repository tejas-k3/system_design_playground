package engine

import "testing"

// Two-phase commit: every service first votes (prepare) and holds its row, then all of them commit. A failed vote
// aborts everyone, so nothing is left half done, but if the coordinator (the app server) dies after the votes and
// before the decision, the services sit locked and cannot decide on their own until it returns.
func twoPhaseRun(mode string, crash bool) RunStats {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": 0.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 4000.0, "retries": 0.0})
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 100.0, "workflow": mode, "termTimeoutMs": 200.0})
	b.node("stock", KDatabase, Params{"connections": 32.0, "writeMs": 10.0})
	b.node("pay", KExternalAPI, Params{"latencyMs": 40.0, "errorPct": 20.0})
	b.node("ship", KExternalAPI, Params{"latencyMs": 30.0, "errorPct": 0.0})
	b.edge("u", "a").edge("a", "stock").edge("a", "pay").edge("a", "ship")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 22})
	s.Advance(10000)
	if crash {
		s.CrashNode("a", 8)
	}
	s.Advance(30000)
	return s.RunSnapshot()
}

func TestTwoPhaseCommitLeavesNothingHalfDone(t *testing.T) {
	two, saga := twoPhaseRun("twoPhase", false), twoPhaseRun("saga", false)
	t.Logf("2pc: inconsistent %d, aborted %d, ok p50 %.0f ms; saga ok p50 %.0f ms", two.Inconsistent, two.Compensated, two.P50, saga.P50)
	if two.Inconsistent != 0 || two.InDoubt != 0 {
		t.Errorf("without a crash every vote ends in a decision: %d half done, %d in doubt", two.Inconsistent, two.InDoubt)
	}
	if two.P50 <= saga.P50+20 {
		t.Errorf("two rounds with every service cost more than one: %.0f vs %.0f ms", two.P50, saga.P50)
	}
}

func TestACoordinatorCrashLeavesServicesLocked(t *testing.T) {
	two := twoPhaseRun("twoPhase", true)
	saga := twoPhaseRun("saga", true)
	t.Logf("2pc: %d in doubt, %.1f lock-seconds blocked; saga: %d in doubt", two.InDoubt, two.BlockedSec, saga.InDoubt)
	if two.InDoubt == 0 || two.BlockedSec < 4 {
		t.Errorf("a crash with transactions in flight leaves them waiting for the coordinator: %d, %.1f s", two.InDoubt, two.BlockedSec)
	}
	if saga.InDoubt != 0 {
		t.Errorf("a saga holds no locks across steps: %d", saga.InDoubt)
	}
}

// Three-phase commit adds a pre-commit round so that no service is ever uncertain while locked: if the coordinator dies,
// the services finish or abort on their own after a timeout instead of waiting for it. It costs one more round.
func TestThreePhaseCommitDoesNotBlockOnACoordinatorCrash(t *testing.T) {
	two, three := twoPhaseRun("twoPhase", true), twoPhaseRun("threePhase", true)
	t.Logf("2pc: %d in doubt, %.1f s blocked; 3pc: %d in doubt, %.1f s blocked", two.InDoubt, two.BlockedSec, three.InDoubt, three.BlockedSec)
	if three.InDoubt != 0 {
		t.Errorf("the services decide on their own, so none is left in doubt: %d", three.InDoubt)
	}
	if three.BlockedSec <= 0 || three.BlockedSec >= two.BlockedSec/2 {
		t.Errorf("they still wait out one timeout each, far less than the coordinator's downtime: %.1f against %.1f s", three.BlockedSec, two.BlockedSec)
	}
}

func TestThreePhaseCommitCostsAnExtraRound(t *testing.T) {
	two, three := twoPhaseRun("twoPhase", false), twoPhaseRun("threePhase", false)
	if three.Inconsistent != 0 || three.P50 <= two.P50+20 {
		t.Errorf("still atomic, but the extra round is slower: %.0f against %.0f ms, %d half done", three.P50, two.P50, three.Inconsistent)
	}
}
