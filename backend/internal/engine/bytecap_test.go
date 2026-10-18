package engine

import (
	"reflect"
	"testing"
)

func byteCapDesign(respKB, capMB float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 2000.0, "keyDist": "uniform", "respKB": respKB, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 64.0}).
		node("c", KCache, Params{"capacityItems": 100000.0, "capacityMB": capMB, "ttlSec": 0.0, "startWarm": false}).
		node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "a").edge("a", "c").edge("c", "db")
	d, _ := Sanitize(b.d)
	return d
}

func hitRate(d Diagram) float64 {
	return run(d, Options{Seed: 4}, 120).Snapshot().Nodes["c"].HitRate
}

func TestBigObjectsFillAByteLimitedCacheSooner(t *testing.T) {
	small := hitRate(byteCapDesign(1, 100))   // 100 MB / 1 KB = 100,000 items: everything fits
	big := hitRate(byteCapDesign(100, 100))   // 100 MB / 100 KB = 1,000 items of 2,000
	huge := hitRate(byteCapDesign(1000, 100)) // 100 items of 2,000
	if small < 0.9 {
		t.Fatalf("2,000 small objects fit in 100 MB, hit rate %.2f", small)
	}
	if !(big < small-0.2 && huge < big-0.2) {
		t.Fatalf("larger objects mean fewer fit, hit rates must fall: %.2f %.2f %.2f", small, big, huge)
	}
}

func TestByteCapacityOffChangesNothing(t *testing.T) {
	if !reflect.DeepEqual(run(byteCapDesign(100, 0), Options{Seed: 4}, 30).RunSnapshot(), run(byteCapDesign(100, 0), Options{Seed: 4}, 30).RunSnapshot()) {
		t.Fatal("deterministic")
	}
	b := &builder{}
	b.node("u", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 2000.0, "keyDist": "uniform", "respKB": 100.0, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 64.0}).node("c", KCache, Params{"capacityItems": 100000.0, "ttlSec": 0.0, "startWarm": false}).node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "a").edge("a", "c").edge("c", "db")
	d, _ := Sanitize(b.d)
	if !reflect.DeepEqual(run(byteCapDesign(100, 0), Options{Seed: 4}, 30).RunSnapshot(), run(d, Options{Seed: 4}, 30).RunSnapshot()) {
		t.Fatal("capacityMB 0 must equal a design that never mentions it")
	}
}
