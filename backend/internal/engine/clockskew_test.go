package engine

import (
	"reflect"
	"testing"
)

func skewDesign(skewMs float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 50.0, "keyDist": "uniform", "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 32.0}).
		node("c", KCache, Params{"capacityItems": 1000.0, "ttlSec": 10.0, "startWarm": false, "clockSkewMs": skewMs}).
		node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "c").edge("c", "db")
	d, _ := Sanitize(b.d)
	return d
}

// Expiry is decided by the cache's own clock against a time stamped by the origin, so a cache whose clock runs ahead
// by 8 s sees a 10 s lifetime as 2 s: entries expire early and the origin takes far more of the traffic.
func TestACacheClockRunningAheadExpiresEntriesEarly(t *testing.T) {
	misses := func(skew float64) int {
		s := run(skewDesign(skew), Options{Seed: 4}, 120)
		return s.Snapshot().Global.Run.Resolved - int(float64(s.Snapshot().Global.Run.Resolved)*s.Snapshot().Nodes["c"].HitRate)
	}
	ok, fast := misses(0), misses(8000)
	t.Logf("misses in 120 s: in sync %d, 8 s ahead %d", ok, fast)
	// 50 keys refilled every 10 s is about 600 misses; every 2 s about 3,000
	if fast < ok*3 {
		t.Fatalf("a clock 8 s ahead should multiply the misses several times: %d vs %d", fast, ok)
	}
}

func TestACacheClockRunningBehindKeepsEntriesLonger(t *testing.T) {
	ok := run(skewDesign(0), Options{Seed: 4}, 120).Snapshot().Nodes["c"].HitRate
	slow := run(skewDesign(-20000), Options{Seed: 4}, 120).Snapshot().Nodes["c"].HitRate
	if slow <= ok {
		t.Fatalf("a clock behind by 20 s lets entries live 30 s instead of 10: hit rate %.3f vs %.3f", slow, ok)
	}
}

func TestNoSkewChangesNothing(t *testing.T) {
	plain := &builder{}
	plain.node("u", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 50.0, "keyDist": "uniform", "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 32.0}).node("c", KCache, Params{"capacityItems": 1000.0, "ttlSec": 10.0, "startWarm": false}).node("db", KDatabase, nil)
	plain.edge("u", "a").edge("a", "c").edge("c", "db")
	d, _ := Sanitize(plain.d)
	if !reflect.DeepEqual(run(skewDesign(0), Options{Seed: 4}, 30).RunSnapshot(), run(d, Options{Seed: 4}, 30).RunSnapshot()) {
		t.Fatal("clockSkewMs 0 must change nothing")
	}
}
