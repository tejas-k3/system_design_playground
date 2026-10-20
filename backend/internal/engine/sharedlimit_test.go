package engine

import (
	"fmt"
	"reflect"
	"testing"
)

// users -> N limiters (100 req/s each) -> app. Each limiter alone allows 100/s.
func limiterFleet(n int, shared bool) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 600.0, "readPct": 100.0, "keySpace": 200.0, "keyDist": "zipf", "timeoutMs": 3000.0}).
		node("a", KAppServer, Params{"threads": 128.0, "queueCap": 1000.0}).node("db", KDatabase, Params{"connections": 64.0})
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("rl%d", i+1)
		b.node(id, KRateLimiter, Params{"limitRps": 100.0, "burst": 10.0, "shared": shared})
		b.edge("u", id).edge(id, "a")
	}
	b.edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func admitted(d Diagram) float64 {
	s := New(d, Options{Seed: 5})
	s.Advance(10000)
	var sum float64
	for i := 0; i < 20; i++ {
		s.Advance(1000)
		sum += s.Snapshot().Nodes["a"].Arr
	}
	return sum / 20
}

// A limiter that keeps its counts in its own memory admits its limit once per instance, so a fleet of four admits
// four times what was configured; counters shared through a common store keep to the limit however many instances.
func TestLocalLimitersOverAdmitByTheNumberOfInstances(t *testing.T) {
	one, local, shared := admitted(limiterFleet(1, false)), admitted(limiterFleet(4, false)), admitted(limiterFleet(4, true))
	t.Logf("admitted per second: one limiter %.0f, four local limiters %.0f, four sharing state %.0f", one, local, shared)
	if one < 90 || one > 125 {
		t.Fatalf("one limiter admits about its 100/s: %.0f", one)
	}
	if local < 360 || local > 440 {
		t.Fatalf("four independent limiters admit about four times as much: %.0f", local)
	}
	if shared < 90 || shared > 125 {
		t.Fatalf("four limiters sharing one count admit about 100/s together: %.0f", shared)
	}
}

func TestSharedStateOffChangesNothing(t *testing.T) {
	if !reflect.DeepEqual(run(limiterFleet(4, false), Options{Seed: 5}, 20).RunSnapshot(), run(limiterFleetWithoutKey(), Options{Seed: 5}, 20).RunSnapshot()) {
		t.Fatal("shared false must change nothing")
	}
}

func limiterFleetWithoutKey() Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 600.0, "readPct": 100.0, "keySpace": 200.0, "keyDist": "zipf", "timeoutMs": 3000.0}).
		node("a", KAppServer, Params{"threads": 128.0, "queueCap": 1000.0}).node("db", KDatabase, Params{"connections": 64.0})
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("rl%d", i+1)
		b.node(id, KRateLimiter, Params{"limitRps": 100.0, "burst": 10.0})
		b.edge("u", id).edge(id, "a")
	}
	b.edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}
