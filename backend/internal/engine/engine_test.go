package engine

import (
	"math"
	"reflect"
	"testing"
)

func engineDesign(engine string, readPct, rps float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "readPct": readPct, "timeoutMs": 30000.0, "retries": 0.0}).node("a", KAppServer, Params{"threads": 128.0, "queueCap": 4000.0})
	b.node("db", KDatabase, Params{"engine": engine, "connections": 64.0, "compactionEverySec": 20.0, "compactionStallMs": 600.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func stats(d Diagram, seed uint32) RunStats {
	return run(d, Options{Seed: seed}, 120).Snapshot().Global.Run
}

func TestAnLSMWritesFasterAndReadsSlower(t *testing.T) {
	w := func(e string) float64 { return stats(engineDesign(e, 0, 20), 6).Mean }
	r := func(e string) float64 { return stats(engineDesign(e, 100, 20), 6).Mean }
	bw, lw := w("btree"), w("lsm")
	// a 30 ms write costs 15 ms on an LSM (half): about 16 ms less, before the stalls are averaged in (600 ms every 20 s)
	if bw-lw < 8 {
		t.Fatalf("an LSM's sequential writes are cheaper: btree %.1f ms lsm %.1f ms", bw, lw)
	}
	br, lr := r("btree"), r("lsm")
	// a 10 ms read costs 15 ms (x1.5): about 5 ms more
	if d := lr - br; math.Abs(d-5.25) > 1.5 {
		t.Fatalf("an LSM's reads touch several levels: want about +5 ms, got %.2f (btree %.1f lsm %.1f)", d, br, lr)
	}
}

func TestCompactionStallsTheTailOfAnLSMsWrites(t *testing.T) {
	bt := stats(engineDesign("btree", 0, 20), 6)
	ls := stats(engineDesign("lsm", 0, 20), 6)
	// about 3% of writes (20 a second x 0.6 s every 20 s) arrive during a stall and wait 0..600 ms, which is where p99 sits
	if ls.P99 < bt.P99+250 {
		t.Fatalf("a 600 ms compaction every 20 s should put hundreds of ms into the write tail: btree p99 %.0f, lsm p99 %.0f", bt.P99, ls.P99)
	}
	if ls.P50 > bt.P50 {
		t.Fatalf("but the typical LSM write is still faster: btree p50 %.0f, lsm p50 %.0f", bt.P50, ls.P50)
	}
	n := 0
	for _, e := range run(engineDesign("lsm", 0, 20), Options{Seed: 6}, 120).Snapshot().Events {
		if e.Type == "compaction" {
			n++
		}
	}
	if n < 5 || n > 6 {
		t.Fatalf("a compaction every 20 s over 120 s is 5-6 events, got %d", n)
	}
}

func TestReadsAreNotStalledByCompaction(t *testing.T) {
	ls := stats(engineDesign("lsm", 100, 20), 6)
	if ls.P99 > 400 {
		t.Fatalf("compaction stalls writes, reads only pay the read amplification: p99 %.0f", ls.P99)
	}
}

func TestBTreeIsTheDefaultAndChangesNothing(t *testing.T) {
	explicit := run(engineDesign("btree", 70, 60), Options{Seed: 3}, 30).RunSnapshot()
	plain := &builder{}
	plain.node("u", KClient, Params{"rps": 60.0, "readPct": 70.0, "timeoutMs": 30000.0, "retries": 0.0}).node("a", KAppServer, Params{"threads": 128.0, "queueCap": 4000.0}).node("db", KDatabase, Params{"connections": 64.0})
	plain.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(plain.d)
	if b := run(d, Options{Seed: 3}, 30).RunSnapshot(); !reflect.DeepEqual(explicit, b) {
		t.Fatal("engine btree must be identical to a database that never mentions an engine")
	}
}

func TestEngineSettingsAreInTheRegistry(t *testing.T) {
	var eng, every, stall bool
	for _, p := range Spec(KDatabase).Params {
		eng = eng || (p.Key == "engine" && p.Default == "btree" && len(p.Options) == 2)
		every = every || (p.Key == "compactionEverySec" && p.ShowIf != nil)
		stall = stall || p.Key == "compactionStallMs"
	}
	if !eng || !every || !stall {
		t.Fatalf("engine (default btree), compactionEverySec and compactionStallMs must exist: %v %v %v", eng, every, stall)
	}
}
