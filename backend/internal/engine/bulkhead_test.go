package engine

import (
	"reflect"
	"testing"
)

// Half the requests are writes that call a vendor taking two seconds; the other half only read the database.
func bulkheadDesign(bulkhead float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 40.0, "readPct": 50.0, "timeoutMs": 4000.0}).
		node("a", KAppServer, Params{"threads": 16.0, "queueCap": 500.0, "bulkheadThreads": bulkhead}).
		node("v", KExternalAPI, Params{"latencyMs": 2000.0, "errorPct": 0.0}).
		node("db", KDatabase, Params{"connections": 32.0})
	b.edge("u", "a").edge("a", "v").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

// 20 slow writes a second at 2 s each want 40 threads; there are 16. Without a limit the slow vendor takes every
// thread and the quick database reads queue behind it.
func TestOneSlowDependencyCanTakeEveryThread(t *testing.T) {
	r := run(bulkheadDesign(0), Options{Seed: 2}, 40).RunSnapshot()
	t.Logf("no bulkhead: ok %d failed %d p95 %.0f", r.OK, r.Failed, r.P95)
	if float64(r.OK) > 0.5*float64(r.OK+r.Failed) {
		t.Fatalf("the reads should be dragged down with the writes: %d ok of %d", r.OK, r.OK+r.Failed)
	}
}

func TestABulkheadKeepsTheHealthyPathAlive(t *testing.T) {
	r := run(bulkheadDesign(6), Options{Seed: 2}, 40).RunSnapshot()
	t.Logf("bulkhead of 6: ok %d failed %d p95 %.0f errors %v", r.OK, r.Failed, r.P95, r.Errors)
	// the reads (half the traffic) keep working; many writes are refused at once instead of tying up threads
	if float64(r.OK) < 0.5*float64(r.OK+r.Failed) {
		t.Fatalf("with the vendor fenced off the reads and the writes that fit should succeed: %d ok of %d", r.OK, r.OK+r.Failed)
	}
	if r.Errors["bulkheadFull"] == 0 {
		t.Fatalf("calls beyond the limit are refused immediately: %v", r.Errors)
	}
	if r.P95 > 2500 {
		t.Fatalf("what succeeds is not stuck behind the vendor: p95 %.0f", r.P95)
	}
}

func TestBulkheadOffChangesNothing(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 40.0, "readPct": 50.0, "timeoutMs": 4000.0}).
		node("a", KAppServer, Params{"threads": 16.0, "queueCap": 500.0}).
		node("v", KExternalAPI, Params{"latencyMs": 2000.0, "errorPct": 0.0}).node("db", KDatabase, Params{"connections": 32.0})
	b.edge("u", "a").edge("a", "v").edge("a", "db")
	d, _ := Sanitize(b.d)
	if !reflect.DeepEqual(run(d, Options{Seed: 2}, 20).RunSnapshot(), run(bulkheadDesign(0), Options{Seed: 2}, 20).RunSnapshot()) {
		t.Fatal("bulkheadThreads 0 must equal a design that never mentions it")
	}
}
