package engine

import (
	"reflect"
	"testing"
)

func visDesign(processMs, visibilityMs float64, idem bool) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 20.0, "readPct": 0.0, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 32.0, "idempotencyKeys": idem}).
		node("q", KQueue, Params{"capacity": 5000.0, "visibilityTimeoutMs": visibilityMs}).
		node("w", KWorker, Params{"concurrency": 16.0, "processMs": processMs}).
		node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "q").edge("q", "w").edge("w", "db")
	d, _ := Sanitize(b.d)
	return d
}

// A job that is still being processed when its visibility timeout runs out becomes visible to the queue again, so a
// second worker takes it: at-least-once delivery. Without idempotency the work is applied twice.
func TestASlowJobIsDeliveredAgainAndAppliedTwice(t *testing.T) {
	slow := run(visDesign(300, 200, false), Options{Seed: 9}, 60).RunSnapshot()
	if slow.Jobs.Redelivered < slow.Jobs.Enqueued*9/10 {
		t.Fatalf("every job outlives a 200 ms timeout at 300 ms: %d redelivered of %d", slow.Jobs.Redelivered, slow.Jobs.Enqueued)
	}
	if slow.Dups < slow.Jobs.Redelivered*9/10 {
		t.Fatalf("each redelivery applies the write a second time: %d duplicates for %d redeliveries", slow.Dups, slow.Jobs.Redelivered)
	}
	if slow.Jobs.Completed < slow.Jobs.Enqueued*9/10 {
		t.Fatalf("the jobs themselves still complete once each: %d of %d", slow.Jobs.Completed, slow.Jobs.Enqueued)
	}
}

func TestATimeoutLongerThanTheWorkCausesNoRedelivery(t *testing.T) {
	ok := run(visDesign(300, 2000, false), Options{Seed: 9}, 60).RunSnapshot()
	if ok.Jobs.Redelivered != 0 || ok.Dups != 0 {
		t.Fatalf("redelivered %d duplicates %d", ok.Jobs.Redelivered, ok.Dups)
	}
}

func TestIdempotencyAbsorbsTheRedeliveries(t *testing.T) {
	r := run(visDesign(300, 200, true), Options{Seed: 9}, 60).RunSnapshot()
	if r.Jobs.Redelivered == 0 || r.Dups != 0 {
		t.Fatalf("redeliveries still happen but with idempotency keys they apply once: redelivered %d duplicates %d", r.Jobs.Redelivered, r.Dups)
	}
}

func TestNoVisibilityTimeoutChangesNothing(t *testing.T) {
	if !reflect.DeepEqual(run(visDesign(300, 0, false), Options{Seed: 9}, 30).RunSnapshot(), run(plainQueueDesign300(), Options{Seed: 9}, 30).RunSnapshot()) {
		t.Fatal("visibilityTimeoutMs 0 must equal a design that never mentions it")
	}
}

func plainQueueDesign300() Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 20.0, "readPct": 0.0, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 32.0}).node("q", KQueue, Params{"capacity": 5000.0}).
		node("w", KWorker, Params{"concurrency": 16.0, "processMs": 300.0}).node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "q").edge("q", "w").edge("w", "db")
	d, _ := Sanitize(b.d)
	return d
}
