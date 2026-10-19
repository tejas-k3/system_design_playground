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

func gatherDesignK(shards int, gather string, k float64, partial bool) Diagram {
	d := gatherDesign(shards, gather, "exponential")
	for i := range d.Nodes {
		if d.Nodes[i].Kind == KAppServer {
			d.Nodes[i].Params["k"] = k
			d.Nodes[i].Params["partialResults"] = partial
		}
	}
	return d
}

// Answering once 12 of 16 shards have replied skips the slowest four: E[12th of 16] = mean x (H16 - H4) = 30 x 1.30.
func TestFirstKCutsTheTailButStragglersStillCostCapacity(t *testing.T) {
	all := stat(gatherDesign(16, "all", "exponential"))
	k12 := stat(gatherDesignK(16, "firstK", 12, false))
	// all waits for the max: 30 x H16 = 101 ms; the 12th of 16: 30 x (H16 - H4) = 39 ms; difference about 62 ms
	if d := all.Mean - k12.Mean; d < 45 || d > 80 {
		t.Fatalf("answering on the first 12 of 16 should save about 62 ms: all %.0f, first-12 %.0f (difference %.0f)", all.Mean, k12.Mean, d)
	}
	s := run(gatherDesignK(16, "firstK", 12, false), Options{Seed: 11}, 30)
	e := s.Snapshot().Edges
	if e["a>s0"].Rps < 30 || e["a>s15"].Rps < 30 {
		t.Fatalf("every shard is still asked every time: %.0f and %.0f rps of 40", e["a>s0"].Rps, e["a>s15"].Rps)
	}
}

func TestKLargerThanTheShardCountMeansAll(t *testing.T) {
	all := stat(gatherDesign(4, "all", "exponential"))
	big := stat(gatherDesignK(4, "firstK", 99, false))
	if all.Mean != big.Mean {
		t.Fatalf("k beyond the number of shards must behave like all: %.1f vs %.1f", all.Mean, big.Mean)
	}
}

func TestFirstKStillFailsWhenTooFewShardsCanAnswer(t *testing.T) {
	o := Options{Seed: 11, Chaos: []ChaosEvent{{Type: "crash", NodeID: "s0", AtSec: 3}, {Type: "crash", NodeID: "s1", AtSec: 3}}}
	r := run(gatherDesignK(4, "firstK", 3, false), o, 30).RunSnapshot()
	if float64(r.OK) > 0.3*float64(r.OK+r.Failed) {
		t.Fatalf("2 of 4 shards down cannot give 3 answers: %d ok of %d", r.OK, r.OK+r.Failed)
	}
	r2 := run(gatherDesignK(4, "firstK", 2, false), o, 30).RunSnapshot()
	if float64(r2.OK) < 0.9*float64(r2.OK+r2.Failed) {
		t.Fatalf("2 of 4 shards down still gives 2 answers: %d ok of %d", r2.OK, r2.OK+r2.Failed)
	}
}

func TestPartialResultsTradeCompletenessForAvailability(t *testing.T) {
	o := Options{Seed: 11, Chaos: []ChaosEvent{{Type: "crash", NodeID: "s2", AtSec: 3}}}
	strict := run(gatherDesignK(4, "all", 4, false), o, 30).RunSnapshot()
	loose := run(gatherDesignK(4, "all", 4, true), o, 30).RunSnapshot()
	if float64(strict.OK) > 0.3*float64(strict.OK+strict.Failed) {
		t.Fatalf("strict wait-for-all fails when a shard is down: %d ok", strict.OK)
	}
	if float64(loose.OK) < 0.9*float64(loose.OK+loose.Failed) {
		t.Fatalf("partial results keep answering: %d ok of %d", loose.OK, loose.OK+loose.Failed)
	}
	if loose.Partial < loose.OK/2 || strict.Partial != 0 {
		t.Fatalf("answers missing a shard are counted: partial %d (strict %d) of %d ok", loose.Partial, strict.Partial, loose.OK)
	}
}

func hedgeDesign(hedgeMs float64) Diagram {
	d := gatherDesign(8, "all", "heavy")
	for i := range d.Nodes {
		if d.Nodes[i].Kind == KAppServer {
			d.Nodes[i].Params["hedgeAfterMs"] = hedgeMs
		}
	}
	return d
}

// Heavy-tailed shards sometimes take far longer than their mean. Asking again after a short wait gives a second,
// independent draw, so the slow outliers stop dominating the request: the tail shrinks for a little extra load.
func TestHedgingShrinksTheTailForAnExtraLoad(t *testing.T) {
	off := stat(hedgeDesign(0))
	on := stat(hedgeDesign(70))
	t.Logf("p99 off %.0f on %.0f; hedged %d of %d requests", off.P99, on.P99, on.Hedged, on.OK)
	if on.P99 > off.P99*0.8 {
		t.Fatalf("hedging should cut the p99 clearly: %.0f without, %.0f with", off.P99, on.P99)
	}
	if off.Hedged != 0 || on.Hedged == 0 {
		t.Fatalf("hedged requests are counted only when it is on: off %d on %d", off.Hedged, on.Hedged)
	}
	// each hedge is one more query on a shard: the load rises by about the number of hedges
	if on.Hedged > on.OK*8/2 {
		t.Fatalf("a short hedge delay on well-behaved shards should hedge a minority of the queries: %d of %d", on.Hedged, on.OK*8)
	}
}

func TestHedgeOffIsIdenticalToNeverMentioningIt(t *testing.T) {
	if !reflect.DeepEqual(stat(hedgeDesign(0)), stat(gatherDesign(8, "all", "heavy"))) {
		t.Fatal("hedgeAfterMs 0 must change nothing")
	}
}

// Every shard of a fan-out shows up in the trace as its own span, all of them children of the app server's span and
// overlapping in time (parallel legs), and the app server's span lasts as long as the slowest leg.
func TestTraceShowsEveryFanOutLegInParallel(t *testing.T) {
	d := gatherDesign(4, "all", "exponential")
	s := New(d, Options{Seed: 11})
	s.EnableTraces(true)
	s.Advance(20000)
	ts := s.Traces()
	if len(ts.OK) == 0 {
		t.Fatal("no traces kept")
	}
	tr := ts.OK[0]
	var app SpanOut
	var legs []SpanOut
	for _, sp := range tr.Spans {
		if sp.Kind == KAppServer {
			app = sp
		}
		if sp.Kind == KIndex {
			legs = append(legs, sp)
		}
	}
	if len(legs) != 4 {
		t.Fatalf("one span per shard expected, got %d in %+v", len(legs), tr.Spans)
	}
	latestEnd := 0.0
	for _, l := range legs {
		if l.Depth != app.Depth+1 {
			t.Errorf("a leg should sit directly under the app server: %+v", l)
		}
		if l.Start < app.Start || l.Start > app.Start+60 {
			t.Errorf("all legs start together, right after the app's own work: leg %.0f, app %.0f", l.Start, app.Start)
		}
		latestEnd = max(latestEnd, l.End)
	}
	if app.End < latestEnd-1e-6 {
		t.Errorf("the app server waits for the slowest leg: app ends %.1f, last leg %.1f", app.End, latestEnd)
	}
}

func TestTracingDoesNotChangeAFanOutRun(t *testing.T) {
	plain := New(gatherDesign(6, "firstK", "heavy"), Options{Seed: 11})
	plain.Advance(30000)
	withTraces := New(gatherDesign(6, "firstK", "heavy"), Options{Seed: 11})
	withTraces.EnableTraces(true)
	withTraces.Advance(30000)
	if !reflect.DeepEqual(plain.RunSnapshot(), withTraces.RunSnapshot()) {
		t.Fatal("tracing is observation only")
	}
}

// The expected slowest of N exponential service times is mean x H(N). Relative to a 2-shard run (same fixed legs) the
// extra latency is mean x (H(N) - H(2)); the engine's default service model runs about 5% above its setting.
func TestWaitForAllFollowsTheHarmonicNumberAt10And100Shards(t *testing.T) {
	h := func(n int) float64 {
		s := 0.0
		for i := 1; i <= n; i++ {
			s += 1 / float64(i)
		}
		return s
	}
	base := run(gatherDesign(2, "all", "exponential"), Options{Seed: 11}, 120).RunSnapshot().Mean
	for _, n := range []int{10, 100} {
		got := run(gatherDesign(n, "all", "exponential"), Options{Seed: 11}, 120).RunSnapshot().Mean - base
		want := 30 * 1.05 * (h(n) - h(2))
		t.Logf("N=%d: measured +%.1f ms, theory +%.1f ms", n, got, want)
		if got < want*0.9 || got > want*1.1 {
			t.Fatalf("N=%d: expected +%.1f ms within 10%%, measured +%.1f", n, want, got)
		}
	}
}
