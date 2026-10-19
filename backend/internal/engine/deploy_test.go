package engine

import (
	"fmt"
	"reflect"
	"testing"
)

func fleetDesign(servers int, ejection bool) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 80.0, "readPct": 100.0, "timeoutMs": 3000.0}).
		node("lb", KLoadBalancer, Params{"healthCheckSec": 1.0, "outlierEjection": ejection})
	b.edge("u", "lb")
	b.node("db", KDatabase, Params{"connections": 64.0})
	for i := 0; i < servers; i++ {
		id := fmt.Sprintf("a%d", i+1)
		b.node(id, KAppServer, Params{"threads": 16.0})
		b.edge("lb", id).edge(id, "db")
	}
	d, _ := Sanitize(b.d)
	return d
}

func okShare(s *Sim, lo, hi float64) float64 {
	var ok, err float64
	for _, p := range s.Snapshot().Series {
		if p.T >= lo && p.T < hi {
			ok += p.OkRps
			err += p.ErrRps
		}
	}
	return ok / (ok + err)
}

// One bad release out of four servers: a quarter of the traffic fails until it is rolled back, then everything recovers.
func TestABadReleaseOnOneOfFourServersFailsAQuarterUntilRollback(t *testing.T) {
	o := Options{Seed: 7, Chaos: []ChaosEvent{{Type: "badDeploy", Kind: KAppServer, FractionPct: 25, Mode: "errors", AtSec: 10, DurationSec: 20}}}
	s := run(fleetDesign(4, false), o, 50)
	before, during, after := okShare(s, 2, 10), okShare(s, 12, 30), okShare(s, 32, 50)
	t.Logf("success before %.3f during %.3f after rollback %.3f", before, during, after)
	if before < 0.99 || after < 0.99 {
		t.Fatalf("healthy before and after: %.3f and %.3f", before, after)
	}
	if during < 0.70 || during > 0.80 {
		t.Fatalf("a quarter of the traffic hits the bad server: expected about 0.75 during the bad release, got %.3f", during)
	}
}

func TestASlowBadReleaseDragsLatencyNotSuccess(t *testing.T) {
	o := Options{Seed: 7, Chaos: []ChaosEvent{{Type: "badDeploy", Kind: KAppServer, FractionPct: 25, Mode: "slow", AtSec: 10, DurationSec: 20}}}
	s := run(fleetDesign(4, false), o, 50)
	if got := okShare(s, 12, 30); got < 0.97 {
		t.Fatalf("slow is not broken: success %.3f", got)
	}
	var base, bad float64
	for _, p := range s.Snapshot().Series {
		if p.T >= 2 && p.T < 10 {
			base = p.P95
		}
		if p.T >= 15 && p.T < 28 {
			bad = p.P95
		}
	}
	if bad < base*1.5 { // 5x slower app time on a quarter of the servers, on top of fixed hops and the database
		t.Fatalf("the slow server should drag the p95 up clearly: %.0f vs %.0f", bad, base)
	}
}

// A rolling restart takes servers out one at a time; with health checks a fleet of four loses only a sliver, and a
// restarted cache starts empty.
func TestARollingRestartOfAFleetLosesLittleWithHealthChecks(t *testing.T) {
	o := Options{Seed: 7, Chaos: []ChaosEvent{{Type: "rollingRestart", Kind: KAppServer, IntervalSec: 5, DownSec: 2, AtSec: 5}}}
	r := run(fleetDesign(4, false), o, 40).RunSnapshot()
	if float64(r.OK) < 0.93*float64(r.OK+r.Failed) || r.Failed == 0 {
		t.Fatalf("a short outage per server: some failures but under 7%%: %d ok of %d", r.OK, r.OK+r.Failed)
	}
}

func TestARestartedCacheComesBackEmpty(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 100.0, "keyDist": "uniform", "timeoutMs": 3000.0}).
		node("a", KAppServer, Params{"threads": 32.0}).node("c", KCache, Params{"capacityItems": 1000.0, "ttlSec": 0.0, "startWarm": true}).node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "c").edge("c", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 7, Chaos: []ChaosEvent{{Type: "rollingRestart", Kind: KCache, IntervalSec: 5, DownSec: 1, AtSec: 10}}})
	s.Advance(9000)
	warm := s.Snapshot().Nodes["c"].Size
	s.Advance(2100) // restarted at 10 s, back at 11 s, sampled just after
	cold := s.Snapshot().Nodes["c"].Size
	if warm < 90 || cold > warm/2 {
		t.Fatalf("a restarted cache has lost what it held: %d items before, %d just after", warm, cold)
	}
}

func TestNoChangeEventChangesNothing(t *testing.T) {
	a := run(fleetDesign(4, false), Options{Seed: 7}, 20).RunSnapshot()
	b := run(fleetDesign(4, false), Options{Seed: 7, Chaos: []ChaosEvent{{Type: "badDeploy", Kind: KAppServer, FractionPct: 25, Mode: "errors", AtSec: 100, DurationSec: 20}}}, 20).RunSnapshot()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("an event scheduled after the run ends must change nothing")
	}
}
