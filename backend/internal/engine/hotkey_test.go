package engine

import "testing"

// One item takes most of the traffic. Every request for it lands on the same cache node, which is the
// hot partition. A tiny local cache on each app server answers the hottest keys without leaving the box.
func hotKeyCacheLoad(params Params) (cacheArr, dbArr float64) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 600.0, "readPct": 100.0, "timeoutMs": 5000.0, "keyDist": "uniform", "keySpace": 1000.0, "hotKeyPct": 90.0})
	p := Params{"threads": 64.0, "queueCap": 1000.0}
	for k, v := range params {
		p[k] = v
	}
	b.node("a", KAppServer, p)
	b.node("c", KCache, Params{"capacityItems": 2000.0, "ttlSec": 60.0, "startWarm": true})
	b.node("db", KDatabase, Params{"connections": 16.0, "readMs": 5.0})
	b.edge("u", "a").edge("a", "c").edge("c", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 2})
	s.Advance(5000)
	n := 0.0
	for sec := 0; sec < 10; sec++ {
		s.Advance(1000)
		snap := s.Snapshot()
		cacheArr += snap.Nodes["c"].Arr
		dbArr += snap.Nodes["db"].Arr
		n++
	}
	return cacheArr / n, dbArr / n
}

func TestLocalCacheTakesTheHotKeyOffTheCacheNode(t *testing.T) {
	base, _ := hotKeyCacheLoad(nil)
	local, _ := hotKeyCacheLoad(Params{"hotKeyMode": "localCache"})
	t.Logf("cache node arrivals/s: %.0f without, %.0f with a local cache", base, local)
	if base < 500 {
		t.Fatalf("the baseline should send every read to the cache node, got %.0f/s", base)
	}
	if local > 0.2*base {
		t.Errorf("local cache should cut cache-node load by >= 80%%: %.0f vs %.0f", local, base)
	}
}

func TestNoHotKeyModeChangesNothing(t *testing.T) {
	a, _ := hotKeyCacheLoad(nil)
	b, _ := hotKeyCacheLoad(Params{"hotKeyMode": "none"})
	if a != b {
		t.Errorf("mode none must equal the default: %v vs %v", a, b)
	}
}
