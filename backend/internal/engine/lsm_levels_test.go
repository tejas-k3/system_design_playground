package engine

import (
	"math"
	"testing"
)

// An LSM tree keeps its data in levels, each about ten times the one above, and a read may have to look in every level
// that could hold the key. A tenfold larger data set is one more level, worth half a read: 1.5x the read time up to
// 10 GB (as it was before the data size was modelled), 2.0x at 100 GB and 2.5x at 1 TB, growing smoothly in between.
func lsmReadMean(engine string, dataGB float64) float64 {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 20.0, "readPct": 100.0, "timeoutMs": 30000.0, "retries": 0.0}).node("a", KAppServer, Params{"threads": 128.0, "queueCap": 4000.0})
	b.node("db", KDatabase, Params{"engine": engine, "connections": 64.0, "dataGB": dataGB, "bufferPoolGB": 4096.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return run(d, Options{Seed: 6}, 120).Snapshot().Global.Run.Mean
}

func TestAnLSMReadsGetSlowerAsTheDataGrowsAndABTreesDoNot(t *testing.T) {
	bt := lsmReadMean("btree", 1000)
	small, mid, big := lsmReadMean("lsm", 10)-bt, lsmReadMean("lsm", 100)-bt, lsmReadMean("lsm", 1000)-bt
	t.Logf("extra read time over a B-tree: %.1f ms at 10 GB, %.1f ms at 100 GB, %.1f ms at 1 TB", small, mid, big)
	for _, c := range []struct{ got, want float64 }{{small, 5}, {mid, 10}, {big, 15}} {
		if math.Abs(c.got-c.want) > 1.5 {
			t.Errorf("a 10 ms read should cost %.0f ms more than on a B-tree, got %.1f", c.want, c.got)
		}
	}
	if d := lsmReadMean("btree", 10) - bt; math.Abs(d) > 0.01 {
		t.Errorf("a B-tree does not care about the data size: %.2f ms apart", d)
	}
}

func TestAnLSMWithNoDataSizeKeepsItsOldReadCost(t *testing.T) {
	if a, b := lsmReadMean("lsm", 0), lsmReadMean("lsm", 10); math.Abs(a-b) > 0.01 {
		t.Errorf("not modelled (0) equals up to 10 GB: %.2f vs %.2f ms", a, b)
	}
}

func TestTheExtraLevelsGrowWithTheLogOfTheDataSize(t *testing.T) {
	for _, c := range []struct{ gb, want float64 }{{0, 0}, {1, 0}, {10, 0}, {100, 1}, {1000, 2}, {31.6228, 0.5}} {
		if got := lsmExtraLevels(c.gb); math.Abs(got-c.want) > 1e-3 {
			t.Errorf("%.1f GB: %.3f extra levels, want %.1f", c.gb, got, c.want)
		}
	}
}

// A full scan reads each level once; the extra levels add half a read, they do not multiply the scan.
func TestAScanOnALargeLSMIsNotChargedPerLevel(t *testing.T) {
	scan := func(gb float64) float64 {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 10.0, "readPct": 100.0, "secondaryPct": 100.0, "timeoutMs": 30000.0, "retries": 0.0}).node("a", KAppServer, Params{"threads": 128.0})
		b.node("db", KDatabase, Params{"engine": "lsm", "connections": 64.0, "dataGB": gb, "bufferPoolGB": 4096.0, "scanMult": 20.0})
		b.edge("u", "a").edge("a", "db")
		d, _ := Sanitize(b.d)
		return run(d, Options{Seed: 6}, 60).Snapshot().Global.Run.Mean
	}
	small, big := scan(10), scan(1000)
	t.Logf("scan: %.1f ms at 10 GB, %.1f ms at 1 TB", small, big)
	if d := big - small; d < 5 || d > 15 {
		t.Errorf("two extra levels add about 10 ms (2 x half of a 10 ms read), not a multiple of the 200 ms scan: %.1f", d)
	}
}
