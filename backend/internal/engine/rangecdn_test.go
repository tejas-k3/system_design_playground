package engine

import (
	"math"
	"testing"
)

// The same hand calculation as for the cache, through a CDN in front of the origin.
func TestARangedCDNFetchesOnlyTheMissingRanges(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"arrival": "closed", "users": 1.0, "thinkMs": 0.0, "readPct": 100.0, "keySpace": 1.0, "respKB": 1000.0, "timeoutMs": 20000.0}).
		node("c", KCDN, Params{"capacityItems": 100000.0, "capacityMB": 0.5, "rangeKB": 250.0, "ttlSec": 0.0, "startWarm": false}).
		node("a", KAppServer, Params{"threads": 64.0}).node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "c").edge("c", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	req, mb, _ := rangeStats(d, 60)
	if got := mb / req; math.Abs(got-0.5) > 0.025 {
		t.Fatalf("CDN: half of each object from the origin, got %.3f MB per request over %.0f", got, req)
	}
}
