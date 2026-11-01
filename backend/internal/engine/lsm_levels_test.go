package engine

import (
	"math"
	"testing"
)

// An LSM tree keeps its data in levels, each about ten times the one above, and a read may have to look in every level
// that could hold the key. A tenfold larger data set is one more level: the read multiplier is 1 + 0.5 x levels, 1.5
// for up to 10 GB (as it was before the data size was modelled), 2.0 at 100 GB and 2.5 at 1 TB.
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
