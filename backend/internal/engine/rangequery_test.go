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

func rangeLintDesign(db Params, rangePct float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 20.0, "readPct": 100.0, "rangePct": rangePct}).node("a", KAppServer, nil).node("db", KDatabase, db)
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestLintFlagsRangeReadsWithoutAnIndexExceptOnATimeSeriesStore(t *testing.T) {
	has := func(d Diagram) bool {
		for _, f := range Lint(d) {
			if f.Rule == "secondary-reads-without-index" {
				return true
			}
		}
		return false
	}
	if !has(rangeLintDesign(nil, 100)) {
		t.Error("range reads with no index scan and should be flagged")
	}
	if has(rangeLintDesign(Params{"indexes": 1.0}, 100)) || has(rangeLintDesign(Params{"model": "timeSeries"}, 100)) || has(rangeLintDesign(nil, 0)) {
		t.Error("an index, a time-series store or no range reads need no warning")
	}
}

// With few rows an index is cheap, but with very many it can cost more than a scan would: a scan never costs less than
// reading through the index.
func TestAScanNeverBeatsTheIndexedRangeRead(t *testing.T) {
	huge := Params{"rangePct": 100.0, "rangeRows": 100000.0}
	if indexed, scan := rangeMean(huge, Params{"indexes": 1.0}), rangeMean(huge, nil); scan < indexed-1 {
		t.Errorf("100,000 rows: scan %.0f ms is cheaper than the index %.0f ms", scan, indexed)
	}
}
