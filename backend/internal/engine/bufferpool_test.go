package engine

import (
	"reflect"
	"testing"
)

func poolDesign(dataGB, poolGB float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": 100.0, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 32.0}).
		node("db", KDatabase, Params{"connections": 32.0, "dataGB": dataGB, "bufferPoolGB": poolGB})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestReadsSlowDownOnceDataOutgrowsTheBufferPool(t *testing.T) {
	fits := run(poolDesign(8, 16), Options{Seed: 3}, 40).Snapshot().Global.Run
	half := run(poolDesign(32, 16), Options{Seed: 3}, 40).Snapshot().Global.Run
	huge := run(poolDesign(320, 16), Options{Seed: 3}, 40).Snapshot().Global.Run
	off := run(poolDesign(0, 16), Options{Seed: 3}, 40).Snapshot().Global.Run
	if d := fits.Mean - off.Mean; d < -1 || d > 1 {
		t.Fatalf("data that fits in memory costs nothing extra: %.1f against %.1f", fits.Mean, off.Mean)
	}
	// reads cost 10 ms; half the pages missing at 10x is about +45 ms, 95% missing about +85 ms
	if got := half.Mean - off.Mean; got < 35 || got > 60 {
		t.Fatalf("half the data on disk should add about 45 ms, added %.1f", got)
	}
	if got := huge.Mean - off.Mean; got < 70 || got > 110 {
		t.Fatalf("almost all of the data on disk should add about 85 ms, added %.1f", got)
	}
}

func TestDataVolumeOffChangesNothing(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": 100.0, "timeoutMs": 5000.0}).node("a", KAppServer, Params{"threads": 32.0}).node("db", KDatabase, Params{"connections": 32.0}).edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	if !reflect.DeepEqual(run(poolDesign(0, 16), Options{Seed: 3}, 20).RunSnapshot(), run(d, Options{Seed: 3}, 20).RunSnapshot()) {
		t.Fatal("dataGB 0 means no volume model")
	}
}
