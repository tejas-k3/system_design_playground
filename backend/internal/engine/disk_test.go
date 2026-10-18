package engine

import (
	"reflect"
	"testing"
)

func diskDesign(rps, iops float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "readPct": 0.0, "timeoutMs": 4000.0}).
		node("a", KAppServer, Params{"threads": 64.0}).
		node("db", KDatabase, Params{"connections": 64.0, "diskIops": iops})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestWritesQueueBehindADiskThatCannotKeepUp(t *testing.T) {
	ok := run(diskDesign(60, 200), Options{Seed: 4}, 40).Snapshot().Global.Run
	bad := run(diskDesign(150, 100), Options{Seed: 4}, 40).Snapshot().Global.Run
	off := run(diskDesign(60, 0), Options{Seed: 4}, 40).Snapshot().Global.Run
	if ok.Mean-off.Mean > 15 {
		t.Fatalf("a disk with spare IOPS adds only its own few ms: mean %.0f against %.0f", ok.Mean, off.Mean)
	}
	if bad.P95 < 1000 || float64(bad.OK) > 0.8*float64(bad.Resolved) {
		t.Fatalf("150 writes/s on a 100 IOPS disk must back up: p95 %.0f ok %d of %d", bad.P95, bad.OK, bad.Resolved)
	}
}

func TestDiskIopsOffChangesNothing(t *testing.T) {
	a := run(diskDesign(60, 0), Options{Seed: 4}, 20).RunSnapshot()
	b := &builder{}
	b.node("u", KClient, Params{"rps": 60.0, "readPct": 0.0, "timeoutMs": 4000.0}).node("a", KAppServer, Params{"threads": 64.0}).node("db", KDatabase, Params{"connections": 64.0}).edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	if !reflect.DeepEqual(a, run(d, Options{Seed: 4}, 20).RunSnapshot()) {
		t.Fatal("diskIops 0 means no disk model")
	}
}

func TestEveryIndexIsAnotherDiskWrite(t *testing.T) {
	d := diskDesign(60, 200)
	for i := range d.Nodes {
		if d.Nodes[i].Kind == KDatabase {
			d.Nodes[i].Params["indexes"] = 3.0
		}
	}
	r := run(d, Options{Seed: 4}, 40).Snapshot().Global.Run
	if r.P95 < 500 {
		t.Fatalf("4 IOs per write at 60 writes/s needs 240 IOPS and must overload a 200 IOPS disk, p95 %.0f", r.P95)
	}
}
