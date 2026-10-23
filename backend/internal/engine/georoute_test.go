package engine

import (
	"math"
	"testing"
)

// Users in Europe, with a stack in the US and a stack in Europe. Round robin sends half of them across the ocean;
// geo routing sends each user to the nearest region.
func geoRun(geo bool, userRegion string) float64 {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 60.0, "readPct": 100.0, "timeoutMs": 3000.0, "region": userRegion, "geoRouting": geo})
	for _, r := range []string{"us", "eu"} {
		b.node("lb-"+r, KLoadBalancer, Params{"region": r})
		b.node("a-"+r, KAppServer, Params{"threads": 32.0, "region": r})
		b.node("db-"+r, KDatabase, Params{"connections": 16.0, "region": r})
		b.edge("u", "lb-"+r).edge("lb-"+r, "a-"+r).edge("a-"+r, "db-"+r)
	}
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 12})
	s.Advance(30000)
	return s.RunSnapshot().P50
}

func TestGeoRoutingSendsEachUserToTheNearestRegion(t *testing.T) {
	rr, geo := geoRun(false, "eu"), geoRun(true, "eu")
	t.Logf("users in eu with stacks in us and eu: median %.0f ms round robin, %.0f ms with geo routing", rr, geo)
	// half of the round-robin requests cross the ocean twice (about +90 ms): the median sits between the two
	if rr-geo < 40 {
		t.Errorf("geo routing should avoid the ocean: %.0f vs %.0f", geo, rr)
	}
	if usGeo := geoRun(true, "us"); math.Abs(usGeo-geo) > 8 {
		t.Errorf("users in us served in us and users in eu served in eu see the same latency: %.0f vs %.0f", usGeo, geo)
	}
}

func TestGeoRoutingOffKeepsRoundRobin(t *testing.T) {
	a, b := geoRun(false, "eu"), geoRun(false, "eu")
	if a != b {
		t.Errorf("deterministic: %.1f vs %.1f", a, b)
	}
}

func TestGeoRoutingFailsOverToTheNextNearestRegion(t *testing.T) {
	run := func(geo bool) float64 {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 60.0, "readPct": 100.0, "timeoutMs": 3000.0, "region": "eu", "geoRouting": geo})
		for _, r := range []string{"us", "eu"} {
			b.node("lb-"+r, KLoadBalancer, Params{"region": r})
			b.node("a-"+r, KAppServer, Params{"threads": 32.0, "region": r})
			b.node("db-"+r, KDatabase, Params{"connections": 16.0, "region": r})
			b.edge("u", "lb-"+r).edge("lb-"+r, "a-"+r).edge("a-"+r, "db-"+r)
		}
		d, _ := Sanitize(b.d)
		s := New(d, Options{Seed: 12, Chaos: []ChaosEvent{{Type: "regionDown", Region: "eu", AtSec: 10, DurationSec: 15}}})
		s.Advance(30000)
		r := s.RunSnapshot()
		return float64(r.OK) / float64(r.Resolved+r.Pending)
	}
	rr, geo := run(false), run(true)
	t.Logf("success with the nearest region down for 15 s: %.3f taking turns, %.3f with geo routing", rr, geo)
	if geo < 0.99 || rr > 0.85 {
		t.Errorf("geo routing fails over to the US while round robin keeps sending half into the dead region: %.3f vs %.3f", geo, rr)
	}
}
