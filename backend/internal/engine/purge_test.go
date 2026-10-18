package engine

import "testing"

func purgeDesign() Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 200.0, "readPct": 100.0, "keySpace": 500.0, "keyDist": "uniform", "timeoutMs": 5000.0}).
		node("c", KCDN, Params{"capacityItems": 5000.0, "ttlSec": 600.0}).
		node("a", KAppServer, Params{"threads": 64.0}).
		node("db", KDatabase, Params{"connections": 16.0})
	b.edge("u", "c").edge("c", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func cacheSize(o Options, sec float64) int {
	return run(purgeDesign(), o, sec).Snapshot().Nodes["c"].Size
}

func TestPurgeEmptiesTheCacheAndItRefills(t *testing.T) {
	o := Options{Seed: 4, Chaos: []ChaosEvent{{Type: "purge", NodeID: "c", AtSec: 40}}}
	if before := cacheSize(o, 39.9); before < 400 {
		t.Fatalf("a warm CDN holds nearly all 500 items, has %d", before)
	}
	if just := cacheSize(o, 40.3); just > 150 {
		t.Fatalf("right after the purge only the items asked for since are back, found %d", just)
	}
	if later := cacheSize(o, 80); later < 400 {
		t.Fatalf("it refills as traffic comes, has %d", later)
	}
	var purged bool
	for _, e := range run(purgeDesign(), o, 45).Snapshot().Events {
		if e.Type == "purge" {
			purged = true
		}
	}
	if !purged {
		t.Fatal("a purge event should be recorded")
	}
}
