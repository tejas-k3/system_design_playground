package engine

import (
	"math"
	"reflect"
	"testing"
)

// users ask for the same 1 MB object through a cache that cuts objects into rangeKB pieces.
func rangeDesign(rangeKB, capMB float64, users float64, coalesce bool) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"arrival": "closed", "users": users, "thinkMs": 0.0, "readPct": 100.0, "keySpace": 1.0, "respKB": 1000.0, "timeoutMs": 20000.0}).
		node("a", KAppServer, Params{"threads": 64.0}).
		node("c", KCache, Params{"capacityItems": 100000.0, "capacityMB": capMB, "rangeKB": rangeKB, "ttlSec": 0.0, "startWarm": false, "coalesce": coalesce}).
		node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "a").edge("a", "c").edge("c", "db")
	d, _ := Sanitize(b.d)
	return d
}

func rangeStats(d Diagram, sec float64) (requests, originMB, hit float64) {
	s := run(d, Options{Seed: 4}, sec)
	ns := s.Snapshot().Nodes["c"]
	r := s.RunSnapshot()
	return float64(r.OK), ns.Extra["originMB"], ns.HitRate
}

// A 1 MB object is 4 ranges of 250 KB. The cache has room for 2 ranges. Every request looks at ranges 0..3 in order
// and then fetches what is missing; with least-recently-used eviction the two ranges that stayed are exactly the two
// the next request does not need first, so it finds 2 of 4 in the cache and fetches the other 2: half the bytes.
func TestARangedCacheFetchesOnlyTheMissingRanges(t *testing.T) {
	req, mb, _ := rangeStats(rangeDesign(250, 0.5, 1, false), 60)
	if req < 200 {
		t.Fatalf("too few requests to judge: %.0f", req)
	}
	got := mb / req
	t.Logf("origin MB per request: %.3f over %.0f requests (hand calculation 0.5)", got, req)
	if math.Abs(got-0.5) > 0.5*0.05 {
		t.Fatalf("half of each 1 MB object should come from the origin, got %.3f MB per request", got)
	}
}

// Ten users ask at the same moment for a cold hot object. With coalescing each of the 4 ranges goes to the origin once,
// exactly 1 MB however many users wait; without it every early request fetches its own copy.
func TestRangesFillOnceEachWhenMissesAreCoalesced(t *testing.T) {
	_, with, hitWith := rangeStats(rangeDesign(250, 10, 10, true), 20)
	_, without, _ := rangeStats(rangeDesign(250, 10, 10, false), 20)
	t.Logf("origin MB: %.2f with coalescing, %.2f without; hit rate %.3f", with, without, hitWith)
	if math.Abs(with-1.0) > 0.05 {
		t.Fatalf("each of the 4 ranges is fetched once: want 1.00 MB, got %.2f", with)
	}
	if without < 2*with {
		t.Fatalf("without coalescing the first requests each fetch the object: %.2f MB", without)
	}
	if hitWith < 0.99 {
		t.Fatalf("once filled, the hot object is served from the cache, hit rate %.3f", hitWith)
	}
}

// A cache smaller than one whole object (0.5 MB for 1 MB) holds nothing without ranges.
func TestAWholeObjectCacheTooSmallForTheObjectNeverHits(t *testing.T) {
	s := run(rangeDesign(0, 0.5, 1, false), Options{Seed: 4}, 30)
	if hr := s.Snapshot().Nodes["c"].HitRate; hr > 0.01 {
		t.Fatalf("a 1 MB object does not fit in 0.5 MB: hit rate %.2f", hr)
	}
}

func TestRangeOffChangesNothing(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"arrival": "closed", "users": 1.0, "thinkMs": 0.0, "readPct": 100.0, "keySpace": 1.0, "respKB": 1000.0, "timeoutMs": 20000.0}).
		node("a", KAppServer, Params{"threads": 64.0}).
		node("c", KCache, Params{"capacityItems": 100000.0, "capacityMB": 0.5, "ttlSec": 0.0, "startWarm": false}).
		node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "a").edge("a", "c").edge("c", "db")
	d, _ := Sanitize(b.d)
	if !reflect.DeepEqual(run(rangeDesign(0, 0.5, 1, false), Options{Seed: 4}, 30).RunSnapshot(), run(d, Options{Seed: 4}, 30).RunSnapshot()) {
		t.Fatal("rangeKB 0 must equal a design that never mentions it")
	}
}
