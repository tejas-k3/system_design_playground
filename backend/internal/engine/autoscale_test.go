package engine

import (
	"math"
	"testing"
)

// users -> one auto-scaled app server (4 threads, ~62 ms per request, so ~64 rps per instance) -> database.
func scaleSim(app Params, curve []TrafficPoint, seed uint32) *Sim {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 40.0, "readPct": 100.0, "timeoutMs": 1500.0, "retries": 0.0})
	p := Params{"threads": 4.0, "cpuMs": 40.0, "queueCap": 100.0, "autoscale": true, "maxInstances": 8.0, "scaleUpDelaySec": 10.0}
	for k, v := range app {
		p[k] = v
	}
	b.node("a", KAppServer, p).node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return New(d, Options{Seed: seed, Traffic: map[string][]TrafficPoint{"u": curve}})
}

var step = []TrafficPoint{{0, 40}, {20, 40}, {22, 160}, {400, 160}}

// recoverSec is when the system last had a bad 5 s window (success under 90%) after the load step at 20 s, i.e. when it
// had recovered for good; 0 if it never had one, and -1 if it was still bad at the end.
func recoverSec(s *Sim, until float64) float64 {
	last := 0.0
	for t := 25.0; t <= until; t += 5 {
		if successBetween(s, t-5, t) < 0.9 {
			last = t
		}
	}
	if last >= until {
		return -1
	}
	return last
}

func TestAutoscaleOffKeepsOneInstance(t *testing.T) {
	s := scaleSim(Params{"autoscale": false}, step, 3)
	s.Advance(60000)
	if s.capacity(s.nodes["a"]) != 4 {
		t.Fatalf("capacity %d, want the 4 threads of a single server", s.capacity(s.nodes["a"]))
	}
	if got := s.Snapshot().Nodes["a"].Instances; got != 0 {
		t.Errorf("instances should not be reported when autoscaling is off, got %d", got)
	}
}

func TestAutoscaleAddsCapacityAfterTheBootDelay(t *testing.T) {
	fast := recoverSec(scaleSim(Params{"scaleUpDelaySec": 3.0}, step, 3), 200)
	slow := recoverSec(scaleSim(Params{"scaleUpDelaySec": 30.0}, step, 3), 300)
	if fast < 0 || slow < 0 {
		t.Fatalf("both fleets should recover: fast %v slow %v", fast, slow)
	}
	if slow-fast < 15 {
		t.Errorf("a 30 s boot delay should recover much later than a 3 s one: %.0f s vs %.0f s", slow, fast)
	}
	if slow < 20+30 {
		t.Errorf("no recovery is possible before the first instance has booted: %.0f s", slow)
	}
}

func TestAutoscaleCannotExceedTheMaximum(t *testing.T) {
	s := scaleSim(Params{"maxInstances": 1.0}, step, 3)
	if r := recoverSec(s, 120); r != -1 {
		t.Errorf("one instance cannot carry 160 rps, but recovered at %.0f s", r)
	}
	if got := s.nodes["a"].as.instances; got != 1 {
		t.Errorf("instances %d, want the maximum of 1", got)
	}
}

func TestAutoscaleScalesBackDownAfterTheDownDelay(t *testing.T) {
	curve := []TrafficPoint{{0, 40}, {20, 40}, {22, 160}, {120, 160}, {122, 20}, {600, 20}}
	quick := scaleSim(Params{"scaleDownDelaySec": 20.0}, curve, 3)
	quick.Advance(300000)
	lazy := scaleSim(Params{"scaleDownDelaySec": 1000.0}, curve, 3)
	lazy.Advance(300000)
	if got := quick.nodes["a"].as.instances; got != 1 {
		t.Errorf("a 20 s scale-down delay should be back to the minimum long after the load fell, got %d", got)
	}
	if got := lazy.nodes["a"].as.instances; got <= 1 {
		t.Errorf("a very long scale-down delay should still hold capacity, got %d", got)
	}
}

func TestShortScaleDownDelayCausesThrash(t *testing.T) {
	wave := []TrafficPoint{{0, 40}}
	for k := 0; k < 30; k++ { // 20 s high, 20 s low, repeatedly
		at := float64(20 + 40*k)
		wave = append(wave, TrafficPoint{at, 40}, TrafficPoint{at + 2, 160}, TrafficPoint{at + 20, 160}, TrafficPoint{at + 22, 30})
	}
	ops := func(down float64) int {
		s := scaleSim(Params{"scaleDownDelaySec": down, "scaleUpDelaySec": 5.0}, wave, 3)
		s.Advance(600000)
		return s.nodes["a"].as.ops
	}
	jumpy, calm := ops(0), ops(120)
	if jumpy <= calm+6 {
		t.Errorf("no scale-down delay should flap far more than a 120 s one: %d vs %d scaling actions", jumpy, calm)
	}
}

func TestAutoscaleCostFollowsInstanceHours(t *testing.T) {
	s := scaleSim(Params{}, step, 3)
	var sum float64
	n := 0
	for sec := 1; sec <= 200; sec++ {
		s.Advance(1000)
		sum += float64(s.nodes["a"].as.instances)
		n++
	}
	unit := NodeCost(Node{Kind: KAppServer, Params: Params{"threads": 4.0, "cpuMs": 40.0}})
	want := unit*sum/float64(n) + NodeCost(Node{Kind: KDatabase})
	if got := s.CostPerHour(); math.Abs(got-want)/want > 0.05 {
		t.Errorf("cost %.3f $/hr, want %.3f (average %.2f instances x $%.3f + database)", got, want, sum/float64(n), unit)
	}
	if now := s.Snapshot().Global.Cost; now <= s.CostPerHour()*0.5 {
		t.Errorf("the live cost reflects the instances running now: %.3f", now)
	}
}

func TestAutoscaleIsDeterministic(t *testing.T) {
	run := func() (RunStats, int) {
		s := scaleSim(Params{}, step, 9)
		s.Advance(150000)
		return s.RunSnapshot(), s.nodes["a"].as.instances
	}
	a, ia := run()
	b, ib := run()
	if a.OK != b.OK || a.Failed != b.Failed || ia != ib {
		t.Fatalf("same seed, different result: %+v/%d vs %+v/%d", a, ia, b, ib)
	}
}

func TestAFleetCostsAtLeastItsMinimum(t *testing.T) {
	one := NodeCost(Node{Kind: KAppServer, Params: Params{"threads": 8.0}})
	three := NodeCost(Node{Kind: KAppServer, Params: Params{"threads": 8.0, "autoscale": true, "minInstances": 3.0}})
	if math.Abs(three-3*one) > 1e-9 {
		t.Errorf("3 servers should cost 3x one: %v vs %v", three, one)
	}
	off := NodeCost(Node{Kind: KAppServer, Params: Params{"threads": 8.0, "minInstances": 3.0}})
	if off != one {
		t.Errorf("minInstances is ignored while autoscaling is off: %v vs %v", off, one)
	}
}

func TestSnapshotCarriesTheRunAverageCostForScoring(t *testing.T) {
	s := scaleSim(Params{}, step, 3)
	s.Advance(150000)
	g := s.Snapshot().Global
	if math.Abs(g.AvgCost-s.CostPerHour()) > 1e-9 {
		t.Errorf("avgCost %.4f should equal the scored cost %.4f", g.AvgCost, s.CostPerHour())
	}
	if g.AvgCost >= g.Cost && g.Cost > 0 && s.nodes["a"].as.instances > 1 {
		t.Errorf("after a scale-up the live cost (%.3f) is above the run average (%.3f)", g.Cost, g.AvgCost)
	}
}
