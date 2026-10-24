package engine

import (
	"reflect"
	"testing"
)

// 100 jobs/s offered to workers that can do 80/s: a backlog grows. 20% of the jobs are marked urgent.
func prioDesign(prio bool, delayMs float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 100.0, "readPct": 0.0, "priorityPct": 20.0, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 32.0}).
		node("q", KQueue, Params{"capacity": 20000.0, "priorityQueue": prio, "delayMs": delayMs}).
		node("w", KWorker, Params{"concurrency": 8.0, "processMs": 100.0}).
		node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "q").edge("q", "w").edge("w", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestUrgentJobsOvertakeABacklogOnlyInAPriorityQueue(t *testing.T) {
	fifo := run(prioDesign(false, 0), Options{Seed: 5}, 60).RunSnapshot().Jobs
	prio := run(prioDesign(true, 0), Options{Seed: 5}, 60).RunSnapshot().Jobs
	t.Logf("urgent p95: fifo %.0f ms, priority %.0f ms; all-job p95: fifo %.0f ms, priority %.0f ms", fifo.HighP95, prio.HighP95, fifo.P95, prio.P95)
	if fifo.HighP95 < 3000 {
		t.Fatalf("in a FIFO queue behind a growing backlog the urgent jobs wait as long as the rest: %.0f ms", fifo.HighP95)
	}
	if prio.HighP95 > fifo.HighP95/10 {
		t.Fatalf("urgent jobs should skip the line: %.0f ms against %.0f ms", prio.HighP95, fifo.HighP95)
	}
	if prio.Completed+prio.Backlog < prio.Enqueued*95/100 {
		t.Fatalf("priority only reorders the work, nothing is dropped: %+v", prio)
	}
}

func TestDelayedDeliveryHoldsEveryJobBack(t *testing.T) {
	// 30 jobs/s against about 50/s capacity: no backlog, so the extra latency is the delay
	d0, d1 := prioDesign(false, 0), prioDesign(false, 2000)
	for _, d := range []Diagram{d0, d1} {
		for i := range d.Nodes {
			if d.Nodes[i].Kind == KClient {
				d.Nodes[i].Params["rps"] = 30.0
			}
		}
	}
	a, b := run(d0, Options{Seed: 5}, 60).RunSnapshot().Jobs, run(d1, Options{Seed: 5}, 60).RunSnapshot().Jobs
	if diff := b.P50 - a.P50; diff < 1900 || diff > 2300 {
		t.Fatalf("a 2 s delivery delay adds about 2 s to a job's latency, added %.0f ms (%.0f vs %.0f)", diff, b.P50, a.P50)
	}
}

func TestPriorityAndDelayOffChangeNothing(t *testing.T) {
	plain := &builder{}
	plain.node("u", KClient, Params{"rps": 100.0, "readPct": 0.0, "priorityPct": 20.0, "timeoutMs": 5000.0}).node("a", KAppServer, Params{"threads": 32.0}).
		node("q", KQueue, Params{"capacity": 20000.0}).node("w", KWorker, Params{"concurrency": 8.0, "processMs": 100.0}).node("db", KDatabase, nil)
	plain.edge("u", "a").edge("a", "q").edge("q", "w").edge("w", "db")
	d, _ := Sanitize(plain.d)
	x, y := run(d, Options{Seed: 5}, 30).RunSnapshot(), run(prioDesign(false, 0), Options{Seed: 5}, 30).RunSnapshot()
	if !reflect.DeepEqual(x, y) {
		t.Fatal("priorityQueue off and delayMs 0 must equal a design that never mentions them")
	}
}

// With nine requests in ten urgent against workers that do 80 jobs a second of 100 offered, the urgent work alone
// fills the workers and the ordinary jobs wait behind it; once the traffic stops, everything drains: nothing waits
// for ever and nothing is lost.
func TestStrictPriorityDelaysOrdinaryJobsButDrainsOnceTheLoadDrops(t *testing.T) {
	d := prioDesign(true, 0)
	for i := range d.Nodes {
		if d.Nodes[i].Kind == KClient {
			d.Nodes[i].Params["priorityPct"] = 90.0
		}
	}
	over := run(d, Options{Seed: 5}, 40).RunSnapshot().Jobs
	s := run(d, Options{Seed: 5, StopTrafficAtMs: 40000}, 200)
	end := s.RunSnapshot().Jobs
	t.Logf("after 40 s of overload %d jobs wait (%d enqueued, %d done); 160 s after the traffic stops: %d wait, %d of %d done", over.Backlog, over.Enqueued, over.Completed, end.Backlog, end.Completed, end.Enqueued)
	if over.Backlog < 500 {
		t.Errorf("100 offered against 80 a second leaves a backlog: %d", over.Backlog)
	}
	if end.Backlog != 0 || end.Completed != end.Enqueued {
		t.Errorf("once the load drops the backlog drains completely: %d waiting, %d of %d done", end.Backlog, end.Completed, end.Enqueued)
	}
}
