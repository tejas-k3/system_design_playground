package engine

import (
	"math"
	"testing"
)

// A range read returns many rows in order ("the last hour of readings"). Whether that is cheap depends on the store:
// a relational or document store needs an index on the field, a wide-column store has no access path but its
// partition key (so it scans every partition), and a time-series store is built to read a time range in order.
func rangeMean(client, db Params) float64 {
	cl := Params{"rps": 20.0, "readPct": 100.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 30000.0, "retries": 0.0}
	for k, v := range client {
		cl[k] = v
	}
	dp := Params{"connections": 64.0, "readMs": 10.0}
	for k, v := range db {
		dp[k] = v
	}
	b := &builder{}
	b.node("u", KClient, cl).node("a", KAppServer, Params{"threads": 128.0}).node("db", KDatabase, dp)
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return run(d, Options{Seed: 4}, 60).Snapshot().Global.Run.Mean
}

func TestRangeReadsCostWhatTheStoreMakesThem(t *testing.T) {
	r := Params{"rangePct": 100.0, "rangeRows": 100.0}
	plain := rangeMean(nil, nil)
	extra := func(db Params) float64 { return rangeMean(r, db) - plain }
	indexed := extra(Params{"indexes": 1.0})       // 1 + 100 rows x 0.01 ms / 10 ms read = 1.1x
	unindexed := extra(nil)                        // a full scan: the unindexed-lookup cost, 20x
	wide := extra(Params{"model": "wideColumn"})   // every partition: 3 x 20x
	series := extra(Params{"model": "timeSeries"}) // in time order: 1 + 100 x 0.003 / 10 = 1.03x, 0.3 ms over a 10 ms read
	t.Logf("extra over a point read: indexed %.1f ms, unindexed %.1f, wide-column %.1f, time-series %.1f", indexed, unindexed, wide, series)
	for _, c := range []struct {
		name      string
		got, want float64
	}{{"indexed", indexed, 1}, {"unindexed", unindexed, 190}, {"wide-column", wide, 590}, {"time-series", series, 0.3}} {
		if math.Abs(c.got-c.want) > math.Max(2, 0.02*c.want) { // service times are sampled, so a long scan wanders a little
			t.Errorf("%s: want about %.0f ms more than a point read, got %.1f", c.name, c.want, c.got)
		}
	}
}

func TestNoRangeShareChangesNothing(t *testing.T) {
	a, b := rangeMean(nil, nil), rangeMean(Params{"rangePct": 0.0, "rangeRows": 5000.0}, Params{"model": "relational"})
	if a != b {
		t.Errorf("rows without a share of range reads are inert: %.3f vs %.3f", a, b)
	}
}

func TestOnlyReadsAreRangeReads(t *testing.T) {
	w := Params{"readPct": 0.0}
	if a, b := rangeMean(w, nil), rangeMean(Params{"readPct": 0.0, "rangePct": 100.0, "rangeRows": 100.0}, nil); a != b {
		t.Errorf("a write is not a range read: %.3f vs %.3f", a, b)
	}
}
