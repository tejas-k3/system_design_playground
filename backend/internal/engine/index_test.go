package engine

import (
	"math"
	"reflect"
	"testing"
)

func indexDesign(readPct, secondaryPct, indexes, rps float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "readPct": readPct, "secondaryPct": secondaryPct, "timeoutMs": 30000.0}).node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0})
	b.node("db", KDatabase, Params{"indexes": indexes, "connections": 16.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestEachIndexAddsThirtyPercentToEveryWrite(t *testing.T) {
	p50 := func(indexes float64) float64 {
		return run(indexDesign(0, 0, indexes, 5), Options{Seed: 8}, 40).Snapshot().Global.Run.Mean
	}
	base := p50(0)
	for _, idx := range []float64{2, 4} {
		got := p50(idx) - base
		want := 30 * 0.3 * idx // writeMs 30 x 0.3 per index
		if math.Abs(got-want) > 2.5 {
			t.Errorf("%v indexes should add %.0f ms to a 30 ms write, added %.1f ms (base p50 %.1f)", idx, want, got, base)
		}
	}
}

func TestASecondaryLookupWithoutAnIndexIsAScan(t *testing.T) {
	p50 := func(secondary, indexes float64) float64 {
		return run(indexDesign(100, secondary, indexes, 5), Options{Seed: 8}, 40).Snapshot().Global.Run.Mean
	}
	plain := p50(0, 0)     // primary-key reads
	scan := p50(100, 0)    // secondary lookups, no index: readMs x scanMult (10 ms x 20)
	indexed := p50(100, 1) // the same lookups with an index
	// readMs x 20 = 200 ms instead of 10 ms is +190 ms; the default service-time model runs about 5% above its setting
	// (the same factor applies to the 10 ms and the 200 ms), so the measured difference is about 200 ms
	if got := scan - plain; math.Abs(got-199.5) > 8 {
		t.Errorf("an unindexed secondary lookup should add about 200 ms (readMs x 20 instead of x 1, with the ~5%% service-time overhead), added %.1f ms", got)
	}
	if math.Abs(indexed-plain) > 3 {
		t.Errorf("one index makes secondary lookups as cheap as key lookups: plain %.1f indexed %.1f", plain, indexed)
	}
}

func TestScansCollapseThroughputWhereAnIndexWouldNot(t *testing.T) {
	ok := func(indexes float64) int {
		return run(indexDesign(100, 100, indexes, 200), Options{Seed: 8}, 30).Snapshot().Global.Run.OK
	}
	noIdx, withIdx := ok(0), ok(1)
	// 16 slots / 200 ms = 80 scans a second at most; 200 offered
	if float64(noIdx) > 80.0*30*1.15 || float64(withIdx) < 0.9*200*30 {
		t.Fatalf("without an index the database saturates near 80 lookups/s, with one it keeps up: %d vs %d ok in 30 s", noIdx, withIdx)
	}
}

func TestDefaultsLeaveTheDatabaseAsItWas(t *testing.T) {
	a := run(indexDesign(70, 0, 0, 80), Options{Seed: 3}, 25).RunSnapshot()
	plain := &builder{}
	plain.node("u", KClient, Params{"rps": 80.0, "readPct": 70.0, "timeoutMs": 30000.0}).node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0}).node("db", KDatabase, Params{"connections": 16.0})
	plain.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(plain.d)
	if b := run(d, Options{Seed: 3}, 25).RunSnapshot(); !reflect.DeepEqual(a, b) {
		t.Fatalf("indexes 0 and secondaryPct 0 must be identical to a design that never mentions them:\n%+v\n%+v", a, b)
	}
}

func TestIndexSettingsAreInTheRegistry(t *testing.T) {
	want := map[string]map[string]float64{KDatabase: {"indexes": 0}, KReplica: {"indexes": 0}, KClient: {"secondaryPct": 0}}
	for kind, keys := range want {
		for key, def := range keys {
			ok := false
			for _, p := range Spec(kind).Params {
				if p.Key == key && p.Default == def {
					ok = true
				}
			}
			if !ok {
				t.Errorf("%s needs %s with default %v", kind, key, def)
			}
		}
	}
}

func TestLintFlagsSecondaryReadsWithoutAnIndex(t *testing.T) {
	has := func(d Diagram) bool {
		for _, f := range Lint(d) {
			if f.Rule == "secondary-reads-without-index" {
				return true
			}
		}
		return false
	}
	if !has(indexDesign(90, 20, 0, 50)) {
		t.Error("secondary lookups with no index should be flagged")
	}
	if has(indexDesign(90, 20, 1, 50)) || has(indexDesign(90, 0, 0, 50)) {
		t.Error("an index present, or no secondary lookups, must not be flagged")
	}
}
