package engine

import (
	"math"
	"reflect"
	"testing"
)

func zoneDesign(appZone, dbZone string) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 40.0, "readPct": 100.0, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 32.0, "zone": appZone}).
		node("db", KDatabase, Params{"connections": 32.0, "zone": dbZone})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestACallAcrossAvailabilityZonesPaysTheHopBothWays(t *testing.T) {
	same := run(zoneDesign("az-a", "az-a"), Options{Seed: 3}, 40).RunSnapshot().Mean
	cross := run(zoneDesign("az-a", "az-b"), Options{Seed: 3}, 40).RunSnapshot().Mean
	// 1.5 ms each way on the app-to-database hop
	if d := cross - same; math.Abs(d-3) > 0.6 {
		t.Fatalf("a cross-AZ hop should add 3 ms to the round trip, added %.2f (%.2f vs %.2f)", d, cross, same)
	}
}

func TestZonesDefaultToOneAndChangeNothing(t *testing.T) {
	plain := &builder{}
	plain.node("u", KClient, Params{"rps": 40.0, "readPct": 100.0, "timeoutMs": 5000.0}).node("a", KAppServer, Params{"threads": 32.0}).node("db", KDatabase, Params{"connections": 32.0}).edge("u", "a").edge("a", "db")
	d, _ := Sanitize(plain.d)
	if !reflect.DeepEqual(run(d, Options{Seed: 3}, 20).RunSnapshot(), run(zoneDesign("az-a", "az-a"), Options{Seed: 3}, 20).RunSnapshot()) {
		t.Fatal("everything in az-a (the default) must equal a design that never mentions zones")
	}
}

// two app servers behind a load balancer, one per zone, one database in az-c
func twoZoneDesign(zoneA, zoneB string) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 60.0, "readPct": 100.0, "timeoutMs": 2000.0}).
		node("lb", KLoadBalancer, Params{"healthCheckSec": 1.0, "zone": "az-c"}).
		node("a1", KAppServer, Params{"threads": 32.0, "zone": zoneA}).
		node("a2", KAppServer, Params{"threads": 32.0, "zone": zoneB}).
		node("db", KDatabase, Params{"connections": 32.0, "zone": "az-c"})
	b.edge("u", "lb").edge("lb", "a1").edge("lb", "a2").edge("a1", "db").edge("a2", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestAZoneOutageTakesDownEverythingInIt(t *testing.T) {
	o := Options{Seed: 3, Chaos: []ChaosEvent{{Type: "zoneDown", Zone: "az-a", AtSec: 10, DurationSec: 20}}}
	spread := run(twoZoneDesign("az-a", "az-b"), o, 40).RunSnapshot()
	packed := run(twoZoneDesign("az-a", "az-a"), o, 40).RunSnapshot()
	t.Logf("success: spread across two zones %.3f, both servers in the failed zone %.3f", float64(spread.OK)/float64(spread.OK+spread.Failed), float64(packed.OK)/float64(packed.OK+packed.Failed))
	if float64(spread.OK) < 0.9*float64(spread.OK+spread.Failed) {
		t.Fatalf("with one server in each zone the balancer routes around the outage: %d ok of %d", spread.OK, spread.OK+spread.Failed)
	}
	if float64(packed.OK) > 0.8*float64(packed.OK+packed.Failed) {
		t.Fatalf("both servers in the failed zone: 20 of 40 seconds of outage must hurt: %d ok of %d", packed.OK, packed.OK+packed.Failed)
	}
}
