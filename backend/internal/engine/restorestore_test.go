package engine

import (
	"math"
	"reflect"
	"testing"
)

// A 20 GB database whose backups sit in an object store, reached over a link. At 25 s a bad migration corrupts it; at
// 28 s it is restored. The backup streams back from the store: the rate is the slowest of
//
//	the link:           bandwidth / 8
//	the store's streams: throughputMBps x (parts in parallel when multipart is on, else 1)
//	the database's own restoreMBps (a ceiling on how fast it can write)
//
// and the database answers nothing for first byte + size / rate.
func restoreFromStore(linkMbps, chunkKB, parallel float64) *Sim {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 20.0, "readPct": 50.0, "keySpace": 1000.0, "timeoutMs": 2000.0})
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0})
	b.node("db", KDatabase, Params{"connections": 64.0, "dataGB": 20.0, "backupEverySec": 10.0, "restoreMBps": 2000.0})
	b.node("s", KObjectStore, Params{"firstByteMs": 30.0, "throughputMBps": 50.0, "multipartKB": chunkKB, "multipartParallel": parallel})
	b.edge("u", "a").edge("a", "db").edge("db", "s")
	d, _ := Sanitize(b.d)
	for i := range d.Edges {
		if d.Edges[i].ID == "db>s" {
			d.Edges[i].BandwidthMbps = linkMbps
		}
	}
	s := New(d, Options{Seed: 8, Chaos: corruptAndRestore()})
	s.Advance(600000)
	return s
}

func TestARestoreStreamsBackThroughTheSlowestOfLinkAndStreams(t *testing.T) {
	const mb = 20.0 * 1024
	cases := []struct {
		name                  string
		link, chunk, parallel float64
		rate                  float64 // MB/s, by hand
	}{
		{"one stream, a link that is wider (800 Mbit/s = 100 MB/s)", 800, 0, 4, 50},
		{"one stream, a narrow link (200 Mbit/s = 25 MB/s)", 200, 0, 4, 25},
		{"4 parts at once, link 800 Mbit/s: link-bound", 800, 1000, 4, 100},
		{"4 parts at once, link 4,000 Mbit/s: stream-bound (200 MB/s)", 4000, 1000, 4, 200},
		{"8 parts at once, link 4,000 Mbit/s", 4000, 1000, 8, 400},
	}
	for _, c := range cases {
		got := restoreFromStore(c.link, c.chunk, c.parallel).RunSnapshot().RtoSec
		want := 0.03 + mb/c.rate
		t.Logf("%s: RTO %.2f s (by hand %.2f s)", c.name, got, want)
		if math.Abs(got-want)/want > 0.05 {
			t.Errorf("%s: RTO %.2f s, by hand %.2f s", c.name, got, want)
		}
	}
}

// the database really is away for the whole restore: requests fail for about that long
func TestTheDatabaseAnswersNothingWhileTheBackupStreamsBack(t *testing.T) {
	s := restoreFromStore(800, 0, 4)
	r := s.RunSnapshot()
	// writes hit the database; reads (half the traffic) also need it: all 20 a second fail for ~410 s
	lost := float64(r.Failed) / 20
	t.Logf("failed %d requests = %.0f s of traffic at 20 a second, RTO %.1f s", r.Failed, lost, r.RtoSec)
	if math.Abs(lost-r.RtoSec)/r.RtoSec > 0.1 {
		t.Fatalf("the outage should last about the RTO: %.0f s of failures against %.1f s", lost, r.RtoSec)
	}
}

// with no store behind the database nothing changes: restoreMBps alone sets the time
func TestRestoreWithoutAStoreIsUnchanged(t *testing.T) {
	s := backupRun(Params{"backupEverySec": 10.0, "restoreMBps": 200.0}, corruptAndRestore())
	if got, want := s.RunSnapshot().RtoSec, 10.0*1024/200; math.Abs(got-want) > 0.01 {
		t.Fatalf("RTO %.2f, want %.2f", got, want)
	}
	// and the explicit zeros of the new settings make no difference to a store-less design
	again := backupRun(Params{"backupEverySec": 10.0, "restoreMBps": 200.0, "nicMbps": 0.0}, corruptAndRestore())
	if !reflect.DeepEqual(s.RunSnapshot(), again.RunSnapshot()) {
		t.Fatal("explicit zeros must equal a design that never mentions them")
	}
}
