package engine

import (
	"math"
	"reflect"
	"testing"
)

// users read pages with a body of respKB; with an object store the app server fetches the body from it after the lookup.
func downloadFlow(respKB float64, withStore bool) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": 100.0, "respKB": respKB, "timeoutMs": 10000.0}).node("a", KAppServer, Params{"threads": 32.0}).node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "db")
	if withStore {
		b.node("s", KObjectStore, Params{"firstByteMs": 30.0, "throughputMBps": 50.0}).edge("a", "s")
	}
	d, _ := Sanitize(b.d)
	return d
}

func TestADownloadPaysTheStoresFirstByteAndTransferTime(t *testing.T) {
	without := run(downloadFlow(500, false), Options{Seed: 2}, 30).Snapshot().Global.Run.Mean
	with := run(downloadFlow(500, true), Options{Seed: 2}, 30).Snapshot().Global.Run.Mean
	// a 20 ms hop each way to the store, 30 ms to the first byte, 500 KB at 50 MB/s = 10 ms (service model about 5% over)
	want := 40 + (30+10)*1.05
	if got := with - without; math.Abs(got-want) > 8 {
		t.Fatalf("a download should add about %.0f ms, added %.1f (%.1f vs %.1f)", want, got, with, without)
	}
}

func TestEgressIsBilledPerGigabyteServed(t *testing.T) {
	s := run(downloadFlow(500, true), Options{Seed: 2}, 60)
	base := run(downloadFlow(500, false), Options{Seed: 2}, 60).CostPerHour()
	// 50 reads/s x 500 KB = 25 MB/s = 90 GB per hour at $0.09 per GB = $8.10 per hour, plus the store's own $0.03
	got := s.CostPerHour() - base
	if want := 8.10 + 0.03; got < want*0.9 || got > want*1.1 {
		t.Fatalf("egress at the current rate should cost about $%.2f/hr more, got $%.2f", want, got)
	}
}

func TestNoBodyOrNoStoreMeansNoDownload(t *testing.T) {
	a := run(downloadFlow(0, true), Options{Seed: 2}, 20).RunSnapshot()
	b := run(downloadFlow(0, false), Options{Seed: 2}, 20).RunSnapshot()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("with no response size there is no body to fetch, the store changes nothing")
	}
	if c := run(downloadFlow(0, true), Options{Seed: 2}, 20).CostPerHour(); math.Abs(c-run(downloadFlow(0, false), Options{Seed: 2}, 20).CostPerHour()-0.03) > 1e-9 {
		t.Fatalf("an idle store costs only its base price: %.4f", c)
	}
}

func TestStorageGrowthIsProjectedOverAYear(t *testing.T) {
	d := downloadFlow(500, true)
	for i := range d.Nodes {
		if d.Nodes[i].Kind == KObjectStore {
			d.Nodes[i].Params["dataGB"] = 1000.0
			d.Nodes[i].Params["growthGBPerDay"] = 10.0
		}
	}
	s := run(d, Options{Seed: 2}, 10)
	ex := s.Snapshot().Nodes["s"].Extra
	// 1,000 GB growing 10 GB a day averages 1,000 + 10 x 182.5 = 2,825 GB over the year, at $0.023 per GB-month
	if want := 2825 * 0.023 * 12; math.Abs(ex["yearStorageCost"]-want) > 1 {
		t.Fatalf("projected year of storage should cost about $%.0f, got %.1f", want, ex["yearStorageCost"])
	}
	if ex["storedGB"] != 1000 {
		t.Fatalf("stored GB: %v", ex)
	}
}
