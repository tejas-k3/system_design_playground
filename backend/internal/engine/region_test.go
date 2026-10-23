package engine

import (
	"math"
	"testing"
)

func regionRun(client, app, db string, chaos []ChaosEvent) *Sim {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 30.0, "readPct": 100.0, "timeoutMs": 3000.0, "region": client})
	b.node("a", KAppServer, Params{"threads": 32.0, "region": app})
	b.node("db", KDatabase, Params{"connections": 16.0, "region": db})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 9, Chaos: chaos})
	s.Advance(30000)
	return s
}

// Calls between regions pay the distance, each way: us to eu 45 ms, us to ap 75 ms, eu to ap 100 ms.
func TestUsersFarFromTheServerPayTheDistanceBothWays(t *testing.T) {
	near := regionRun("us", "us", "us", nil).RunSnapshot().P50
	eu := regionRun("eu", "us", "us", nil).RunSnapshot().P50
	ap := regionRun("ap", "us", "us", nil).RunSnapshot().P50
	t.Logf("median latency: users in the same region %.0f ms, in eu %.0f ms, in ap %.0f ms", near, eu, ap)
	if d := eu - near; math.Abs(d-90) > 12 {
		t.Errorf("a user in eu is 45 ms each way from us: expected about +90 ms, got %+.0f", d)
	}
	if d := ap - near; math.Abs(d-150) > 12 {
		t.Errorf("a user in ap is 75 ms each way from us: expected about +150 ms, got %+.0f", d)
	}
}

func TestADatabaseInAnotherRegionTaxesEveryCall(t *testing.T) {
	same := regionRun("us", "us", "us", nil).RunSnapshot().P50
	far := regionRun("us", "us", "eu", nil).RunSnapshot().P50
	if d := far - same; math.Abs(d-90) > 12 {
		t.Errorf("each database call crosses the ocean twice: expected about +90 ms, got %+.0f", d)
	}
}

func TestARegionOutageTakesDownEverythingInIt(t *testing.T) {
	s := regionRun("us", "us", "eu", []ChaosEvent{{Type: "regionDown", Region: "eu", AtSec: 10, DurationSec: 10}})
	var during, after float64
	for _, p := range s.Snapshot().Series {
		if p.T > 11 && p.T < 19 {
			during += p.OkRps
		}
		if p.T > 22 {
			after += p.OkRps
		}
	}
	if during > 5 || after < 100 {
		t.Errorf("the database in eu is down for 10 s and everything else is fine: %.0f during, %.0f after", during, after)
	}
}

func TestUnsetRegionsChangeNothing(t *testing.T) {
	a := regionRun("us", "us", "us", nil).RunSnapshot()
	b := regionRun("", "", "", nil).RunSnapshot()
	if a.OK != b.OK || a.P50 != b.P50 {
		t.Errorf("no region set must equal everything in us: %d/%.1f vs %d/%.1f", a.OK, a.P50, b.OK, b.P50)
	}
}
