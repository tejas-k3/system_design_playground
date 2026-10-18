package engine

import (
	"fmt"
	"reflect"
	"testing"
)

func gatherDesign(shards int, gather string, variability string) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 40.0, "readPct": 100.0, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 64.0, "gather": gather})
	b.edge("u", "a")
	for i := 0; i < shards; i++ {
		id := fmt.Sprintf("s%d", i)
		b.node(id, KIndex, Params{"queryMs": 30.0, "connections": 16.0, "variability": variability})
		b.edge("a", id)
	}
	d, _ := Sanitize(b.d)
	return d
}

func stat(d Diagram) RunStats { return run(d, Options{Seed: 11}, 60).RunSnapshot() }

// The latency of a request that needs every shard is the slowest of N draws, so it grows with N even though each
// shard is exactly as fast as before: the tail-at-scale lesson.
func TestWaitingForEveryShardMakesLatencyTheMaximumOfN(t *testing.T) {
	two := stat(gatherDesign(2, "all", "exponential"))
	many := stat(gatherDesign(16, "all", "exponential"))
	if many.OK < 1500 || two.OK < 1500 {
		t.Fatalf("both should serve their load: %d and %d ok", two.OK, many.OK)
	}
	// Everything but the shards (two 40 ms client hops, CPU, two shard hops) is the same in both runs, so compare the
	// difference: E[max of N exponentials] = mean x H(N), H(2)=1.5 and H(16)=3.38, i.e. 30 x (3.38-1.5) = 56 ms more.
	if d := many.Mean - two.Mean; d < 40 || d > 75 {
		t.Fatalf("16 shards should add about 56 ms over 2: %.0f vs %.0f ms (difference %.0f)", many.Mean, two.Mean, d)
	}
}

func TestEveryShardIsQueriedOncePerRequest(t *testing.T) {
	s := run(gatherDesign(4, "all", "steady"), Options{Seed: 11}, 30)
	r := s.RunSnapshot()
	total := r.OK + r.Failed
	for i := 0; i < 4; i++ {
		got := s.Snapshot().Edges[fmt.Sprintf("a>s%d", i)]
		if got.Rps <= 0 {
			t.Fatalf("shard %d received nothing", i)
		}
	}
	if total < 1000 {
		t.Fatalf("expected about 1,200 requests, got %d", total)
	}
}

func TestOneDownShardFailsAWaitForAllRequest(t *testing.T) {
	d := gatherDesign(4, "all", "steady")
	o := Options{Seed: 11, Chaos: []ChaosEvent{{Type: "crash", NodeID: "s2", AtSec: 5}}}
	r := run(d, o, 30).RunSnapshot()
	if float64(r.OK) > 0.3*float64(r.OK+r.Failed) {
		t.Fatalf("with one of four shards down almost every request needs it: %d ok of %d", r.OK, r.OK+r.Failed)
	}
}

func TestGatherOffLeavesTheAppAloneAndIsDeterministic(t *testing.T) {
	// with gather none an app server with several shards behind it does not fan out: a read goes to no data store
	a := stat(gatherDesign(3, "none", "steady"))
	b := stat(gatherDesign(3, "none", "steady"))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("deterministic")
	}
	if a.OK != 0 {
		t.Fatalf("without gather the shards are not a read path: %d ok", a.OK)
	}
}
