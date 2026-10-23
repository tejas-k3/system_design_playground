package engine

import (
	"math"
	"testing"
)

func hotShareRun(hotPct float64, dist string) RunStats {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 200.0, "readPct": 100.0, "keySpace": 5000.0, "keyDist": dist, "hotKeyPct": hotPct})
	b.node("a", KAppServer, Params{"threads": 64.0})
	b.node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 3})
	s.Advance(20000)
	return s.RunSnapshot()
}

// 40% of requests go to one item and the rest are spread over 5,000: the hottest item's share is 0.40 plus its
// 1/5000 of the rest.
func TestHottestKeyShareFindsTheOneBigItem(t *testing.T) {
	r := hotShareRun(40, "uniform")
	t.Logf("hottest item share %.3f (theory 0.40)", r.HottestKeyShare)
	if math.Abs(r.HottestKeyShare-0.40) > 0.03 {
		t.Errorf("expected about 0.40, got %.3f", r.HottestKeyShare)
	}
}

func TestHottestKeyShareIsSmallWhenNothingIsHot(t *testing.T) {
	r := hotShareRun(0, "uniform")
	if r.HottestKeyShare > 0.02 {
		t.Errorf("5,000 equal items: the busiest has about 0.0002 of the traffic, got %.3f", r.HottestKeyShare)
	}
}
