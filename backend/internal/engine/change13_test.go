package engine

import (
	"fmt"
	"math"
	"testing"
)

// Three servers, one in each zone, behind a balancer that notices a dead server after a one second health check. A
// thread is held for 20 ms of work plus the database round trip (5 + 1 + 5 ms), so a server of 8 threads serves
// 8 / 31 ms = 258 requests a second. 700 a second are offered: three servers (774) cope, two (516) can do 516 of the
// 700, so success should fall to about 516 / 700 = 0.74 while a zone is gone, and come back after.
func threeZones(rps float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "readPct": 100.0, "timeoutMs": 1000.0, "retries": 0.0})
	b.node("lb", KLoadBalancer, Params{"healthCheckSec": 1.0, "zone": "az-a"})
	b.edge("u", "lb")
	b.node("db", KDatabase, Params{"connections": 128.0, "readMs": 1.0})
	for i, z := range []string{"az-a", "az-b", "az-c"} {
		id := fmt.Sprintf("a%d", i+1)
		b.node(id, KAppServer, Params{"threads": 8.0, "cpuMs": 20.0, "queueCap": 50.0, "zone": z})
		b.edge("lb", id).edge(id, "db")
	}
	d, _ := Sanitize(b.d)
	return d
}

func TestLosingOneOfThreeZonesCostsAThirdOfTheCapacityForExactlyTheOutage(t *testing.T) {
	o := Options{Seed: 3, Chaos: []ChaosEvent{{Type: "zoneDown", Zone: "az-b", AtSec: 20, DurationSec: 20}}}
	s := run(threeZones(700), o, 60)
	before, during, after := okShare(s, 8, 20), okShare(s, 23, 40), okShare(s, 43, 60)
	want := 516.0 / 700
	t.Logf("success before %.3f, during the zone outage %.3f (capacity 516 of 700 offered = %.3f), after %.3f", before, during, want, after)
	if before < 0.95 || after < 0.95 {
		t.Errorf("healthy before and after: %.3f, %.3f", before, after)
	}
	if math.Abs(during-want) > 0.07 {
		t.Errorf("a third of the capacity is gone: expected about %.3f, got %.3f", want, during)
	}
}

// A bad release on 10% of a ten-server fleet fails about 10% of the requests until it is rolled back.
func TestABadReleaseOnTenPercentOfTenServersFailsAboutTenPercent(t *testing.T) {
	o := Options{Seed: 7, Chaos: []ChaosEvent{{Type: "badDeploy", Kind: KAppServer, FractionPct: 10, Mode: "errors", AtSec: 10, DurationSec: 20}}}
	s := run(fleetDesign(10, false), o, 50)
	before, during, after := okShare(s, 2, 10), okShare(s, 12, 30), okShare(s, 32, 50)
	t.Logf("success before %.3f, during %.3f (expected about 0.90), after the rollback %.3f", before, during, after)
	if before < 0.99 || after < 0.99 || math.Abs(during-0.90) > 0.03 {
		t.Errorf("expected 0.99+, about 0.90, 0.99+: got %.3f, %.3f, %.3f", before, during, after)
	}
}

// Each app server has its own cache; a rolling restart of the caches empties them one after another, so the hit rate
// of the fleet dips once per cache (three dips for three caches) and recovers between them.
func TestARollingRestartOfCachesDipsOncePerCacheAndRecovers(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 150.0, "readPct": 100.0, "keySpace": 60.0, "keyDist": "uniform", "timeoutMs": 3000.0})
	b.node("lb", KLoadBalancer, Params{"healthCheckSec": 1.0})
	b.edge("u", "lb")
	b.node("db", KDatabase, Params{"connections": 64.0, "readMs": 10.0})
	for i := 1; i <= 3; i++ {
		a, c := fmt.Sprintf("a%d", i), fmt.Sprintf("c%d", i)
		b.node(a, KAppServer, Params{"threads": 16.0})
		b.node(c, KCache, Params{"capacityItems": 1000.0, "ttlSec": 0.0, "startWarm": true})
		b.edge("lb", a).edge(a, c).edge(c, "db")
	}
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 7, Chaos: []ChaosEvent{{Type: "rollingRestart", Kind: KCache, IntervalSec: 12, DownSec: 1, AtSec: 10}}})
	var sizes [][3]int
	for sec := 0; sec < 50; sec++ {
		s.Advance(1000)
		n := s.Snapshot().Nodes
		sizes = append(sizes, [3]int{n["c1"].Size, n["c2"].Size, n["c3"].Size})
	}
	dips := 0
	for i := 1; i < len(sizes); i++ {
		for k := 0; k < 3; k++ {
			if sizes[i-1][k] >= 50 && sizes[i][k] < sizes[i-1][k]/2 {
				dips++ // this cache lost more than half of what it held
			}
		}
	}
	last := sizes[len(sizes)-1]
	t.Logf("caches that went cold during the rolling restart: %d of 3; sizes at the end %v", dips, last)
	if dips < 3 {
		t.Errorf("each of the three caches should have gone cold once: %d", dips)
	}
	if last[0] < 50 || last[1] < 50 || last[2] < 50 {
		t.Errorf("they refill after the restart: %v", last)
	}
}
