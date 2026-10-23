package engine

import "testing"

// The balancer sits in az-a with one server in az-a and one in az-b, the database in az-a. Sending every request to
// the server in the balancer's own zone avoids the hop between zones; when that server is gone the balancer either
// falls back to the other zone or, with cross-zone failover off, has nowhere to send them.
func zoneRouteDesign(routing string, failover bool) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 60.0, "readPct": 100.0, "timeoutMs": 2000.0}).
		node("lb", KLoadBalancer, Params{"healthCheckSec": 1.0, "zone": "az-a", "zoneRouting": routing, "crossZoneFailover": failover}).
		node("a1", KAppServer, Params{"threads": 32.0, "zone": "az-a"}).
		node("a2", KAppServer, Params{"threads": 32.0, "zone": "az-b"}).
		node("db", KDatabase, Params{"connections": 32.0, "zone": "az-a"})
	b.edge("u", "lb").edge("lb", "a1").edge("lb", "a2").edge("a1", "db").edge("a2", "db")
	d, _ := Sanitize(b.d)
	return d
}

func succRate(s *Sim) float64 {
	r := s.RunSnapshot()
	return float64(r.OK) / float64(r.OK+r.Failed)
}

func TestStayingInTheBalancersZoneSavesTheCrossZoneHops(t *testing.T) {
	any := run(zoneRouteDesign("any", true), Options{Seed: 3}, 40).RunSnapshot().Mean
	local := run(zoneRouteDesign("local", true), Options{Seed: 3}, 40).RunSnapshot().Mean
	t.Logf("mean latency: any zone %.2f ms, own zone first %.2f ms", any, local)
	// half the requests go to the az-b server: 3 ms for the balancer hop and 3 ms for its database hop
	if d := any - local; d < 2.4 || d > 3.6 {
		t.Errorf("expected about 3 ms saved on average, got %.2f", d)
	}
}

func TestCrossZoneFailoverDecidesWhetherTheOtherZoneTakesOver(t *testing.T) {
	o := Options{Seed: 3, Chaos: []ChaosEvent{{Type: "crash", NodeID: "a1", AtSec: 10, DownSec: 20}}}
	with := succRate(run(zoneRouteDesign("local", true), o, 40))
	without := succRate(run(zoneRouteDesign("local", false), o, 40))
	any := succRate(run(zoneRouteDesign("any", false), o, 40))
	t.Logf("a1 down for 20 of 40 s: failover on %.3f, off %.3f, balancing across zones %.3f", with, without, any)
	// the balancer needs its 1 s health check to notice: all 60 requests of that second go to the dead server, 60 of 2,400
	if with < 0.96 || any < 0.96 {
		t.Errorf("the other zone should take over: %.3f, %.3f", with, any)
	}
	if without > 0.7 {
		t.Errorf("without cross-zone failover half the run has nowhere to go: %.3f", without)
	}
}

func TestZoneRoutingDefaultsToAnyZone(t *testing.T) {
	a := run(zoneRouteDesign("", true), Options{Seed: 3}, 20).RunSnapshot()
	b := run(zoneRouteDesign("any", true), Options{Seed: 3}, 20).RunSnapshot()
	if a.OK != b.OK || a.Mean != b.Mean {
		t.Errorf("the default must be any zone: %v vs %v", a.Mean, b.Mean)
	}
}
