package engine

import (
	"reflect"
	"testing"
)

// 250 requests a second at 16 threads: light requests take 20 ms of CPU, a heavy one 40 times that. The average
// request costs 59 ms, so the threads are about 92% busy and every heavy request holds one for most of a second.
func mixDesign(heavyPct float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 250.0, "readPct": 100.0, "timeoutMs": 10000.0, "heavyPct": heavyPct, "heavyMult": 40.0}).
		node("a", KAppServer, Params{"threads": 16.0, "queueCap": 2000.0, "cpuMs": 20.0}).
		node("db", KDatabase, Params{"connections": 32.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

// 5% heavy requests are 5% of the arrivals but, at 40x the work, a third of the work: the
// light requests wait behind them (head-of-line blocking).
func TestAFewHeavyRequestsBlockTheLightOnes(t *testing.T) {
	plain := run(mixDesign(0), Options{Seed: 3}, 60).RunSnapshot()
	mixed := run(mixDesign(5), Options{Seed: 3}, 60).RunSnapshot()
	t.Logf("p95 without heavy requests %.0f ms; with 5%% heavy: light p95 %.0f ms, heavy p95 %.0f ms", plain.P95, mixed.LightP95, mixed.HeavyP95)
	if mixed.LightP95 < plain.P95*3 {
		t.Fatalf("light requests should queue behind the heavy ones: %.0f vs %.0f ms", mixed.LightP95, plain.P95)
	}
	if mixed.HeavyP95 <= mixed.LightP95 {
		t.Fatalf("heavy requests are slower themselves: %.0f vs %.0f", mixed.HeavyP95, mixed.LightP95)
	}
}

func TestMoreThreadsAbsorbHeavyRequests(t *testing.T) {
	few, many := mixDesign(5), mixDesign(5)
	for i := range many.Nodes {
		if many.Nodes[i].Kind == KAppServer {
			many.Nodes[i].Params["threads"] = 64.0
		}
	}
	a, b := run(few, Options{Seed: 3}, 60).RunSnapshot(), run(many, Options{Seed: 3}, 60).RunSnapshot()
	if b.LightP95 > a.LightP95/3 {
		t.Fatalf("four times the threads should cut the light requests' wait a lot: %.0f vs %.0f", b.LightP95, a.LightP95)
	}
}

func TestNoHeavyMixChangesNothing(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 250.0, "readPct": 100.0, "timeoutMs": 10000.0}).
		node("a", KAppServer, Params{"threads": 16.0, "queueCap": 2000.0, "cpuMs": 20.0}).node("db", KDatabase, Params{"connections": 32.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	if !reflect.DeepEqual(run(d, Options{Seed: 3}, 30).RunSnapshot(), run(mixDesign(0), Options{Seed: 3}, 30).RunSnapshot()) {
		t.Fatal("heavyPct 0 must equal a design that never mentions classes")
	}
}

// Serving the newest request first (LIFO) under this load: the light requests that are served are the ones that have
// waited least, so their p95 falls; the ones left at the back of the queue time out instead of being served late.
func TestNewestFirstQueueingCutsTheLightRequestsWait(t *testing.T) {
	fifo, lifo := mixDesign(5), mixDesign(5)
	for i := range lifo.Nodes {
		if lifo.Nodes[i].Kind == KAppServer {
			lifo.Nodes[i].Params["queueOrder"] = "lifo"
			lifo.Nodes[i].Params["queueTimeoutMs"] = 2000.0
		}
	}
	a, b := run(fifo, Options{Seed: 3}, 60).RunSnapshot(), run(lifo, Options{Seed: 3}, 60).RunSnapshot()
	t.Logf("light p95: %.0f ms first-in-first-out, %.0f ms newest first (success %.3f vs %.3f)", a.LightP95, b.LightP95, float64(a.OK)/float64(a.OK+a.Failed), float64(b.OK)/float64(b.OK+b.Failed))
	if b.LightP95 > a.LightP95/2 {
		t.Errorf("newest first should at least halve the light requests' p95: %.0f vs %.0f", b.LightP95, a.LightP95)
	}
}
