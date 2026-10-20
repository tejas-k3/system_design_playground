package engine

import (
	"reflect"
	"testing"
)

// 400 requests a second at a server that can do about 250, behind a queue of 2,000 and clients that give up after 1 s.
func deadlineDesign(propagate bool) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 400.0, "readPct": 100.0, "timeoutMs": 1000.0}).
		node("a", KAppServer, Params{"threads": 8.0, "queueCap": 2000.0, "propagateDeadline": propagate}).
		node("db", KDatabase, Params{"connections": 32.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

// With a long queue each request waits several seconds, longer than its client will: the server then spends its
// time on requests nobody is waiting for. Skipping the expired ones leaves the capacity for the live ones.
func TestWorkNobodyIsWaitingForStillCostsCapacity(t *testing.T) {
	without := run(deadlineDesign(false), Options{Seed: 3}, 40).RunSnapshot()
	with := run(deadlineDesign(true), Options{Seed: 3}, 40).RunSnapshot()
	t.Logf("ok without %d, with deadlines %d; skipped %d", without.OK, with.OK, with.DeadlineSkipped)
	if without.OK > 4000 {
		t.Fatalf("an overloaded FIFO queue serves mostly abandoned requests: %d ok", without.OK)
	}
	if with.OK < without.OK*3 {
		t.Fatalf("skipping expired work should multiply the goodput: %d vs %d ok", with.OK, without.OK)
	}
	if with.DeadlineSkipped == 0 || without.DeadlineSkipped != 0 {
		t.Fatalf("skips are counted only when it is on: %d vs %d", with.DeadlineSkipped, without.DeadlineSkipped)
	}
}

func TestDeadlinesOffChangeNothing(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 400.0, "readPct": 100.0, "timeoutMs": 1000.0}).
		node("a", KAppServer, Params{"threads": 8.0, "queueCap": 2000.0}).node("db", KDatabase, Params{"connections": 32.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	if !reflect.DeepEqual(run(d, Options{Seed: 3}, 20).RunSnapshot(), run(deadlineDesign(false), Options{Seed: 3}, 20).RunSnapshot()) {
		t.Fatal("propagateDeadline false must change nothing")
	}
}
