package engine

import (
	"math"
	"reflect"
	"testing"
)

// one user at a time uploads 10 MB through an app server to an object store (a database finishes the write).
func multipartDesign(chunkKB, parallel, linkMbps float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"arrival": "closed", "users": 1.0, "thinkMs": 0.0, "readPct": 0.0, "reqKB": 10000.0, "timeoutMs": 30000.0}).
		node("a", KAppServer, Params{"threads": 8.0}).
		node("s", KObjectStore, Params{"firstByteMs": 30.0, "throughputMBps": 50.0, "multipartKB": chunkKB, "multipartParallel": parallel}).
		node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "s").edge("a", "db")
	d, _ := Sanitize(b.d)
	for i := range d.Edges {
		if d.Edges[i].ID == "a>s" {
			d.Edges[i].BandwidthMbps = linkMbps
		}
	}
	return d
}

func uploadMean(d Diagram) float64 {
	return run(d, Options{Seed: 3}, 600).Snapshot().Global.Run.Mean
}

// The store part of an upload is first byte + rounds x chunk time, where rounds = ceil(parts / parallel):
//
//	single:                 30 + 10,000 KB / 50 MB/s          = 230 ms
//	1 MB chunks, 4 at once: 10 parts, 3 rounds x 20 ms        = 30 + 60  = 90 ms (saves 140)
//	1 MB chunks, 10 at once: 1 round x 20 ms                  = 30 + 20  = 50 ms (saves 180)
//	5 MB chunks, 1 at a time: 2 rounds x 100 ms               = 230 ms: no gain without parallelism
func TestMultipartUploadTimeFollowsTheRoundsFormula(t *testing.T) {
	single := uploadMean(multipartDesign(0, 4, 0))
	cases := []struct {
		chunk, par, saves float64
	}{{1000, 4, 140}, {1000, 10, 180}, {5000, 1, 0}}
	for _, c := range cases {
		got := single - uploadMean(multipartDesign(c.chunk, c.par, 0))
		t.Logf("chunk %.0f KB x %.0f: saves %.1f ms (formula %.0f)", c.chunk, c.par, got, c.saves)
		if math.Abs(got-c.saves) > max(2, c.saves*0.05) {
			t.Errorf("chunk %.0f KB x %.0f: expected to save %.0f ms, saved %.1f", c.chunk, c.par, c.saves, got)
		}
	}
}

// The 'Video uploads' case: 10 MB over a 200 Mbit/s link is 400 ms on the wire whatever the store does; multipart
// shrinks only the store's part (230 ms -> 50 ms), so the link stays the floor.
func TestMultipartCannotBeatTheLink(t *testing.T) {
	single := uploadMean(multipartDesign(0, 4, 200))
	multi := uploadMean(multipartDesign(1000, 10, 200))
	t.Logf("10 MB over 200 Mbit/s: %.0f ms single, %.0f ms multipart (link alone 400 ms)", single, multi)
	if math.Abs((single-multi)-180) > 9 {
		t.Fatalf("multipart should save the store's 180 ms: %.1f", single-multi)
	}
	if multi < 400 {
		t.Fatalf("the link needs 400 ms for 10 MB at 200 Mbit/s, got %.0f", multi)
	}
}

func TestMultipartOffChangesNothing(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"arrival": "closed", "users": 1.0, "thinkMs": 0.0, "readPct": 0.0, "reqKB": 10000.0, "timeoutMs": 30000.0}).
		node("a", KAppServer, Params{"threads": 8.0}).
		node("s", KObjectStore, Params{"firstByteMs": 30.0, "throughputMBps": 50.0}).
		node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "s").edge("a", "db")
	d, _ := Sanitize(b.d)
	if !reflect.DeepEqual(run(multipartDesign(0, 4, 0), Options{Seed: 3}, 60).RunSnapshot(), run(d, Options{Seed: 3}, 60).RunSnapshot()) {
		t.Fatal("multipartKB 0 must equal a design that never mentions it")
	}
}

// a chunk bigger than the body is just a single upload
func TestAChunkBiggerThanTheBodyChangesNothing(t *testing.T) {
	a := uploadMean(multipartDesign(0, 4, 0))
	b := uploadMean(multipartDesign(20000, 4, 0))
	if a != b {
		t.Fatalf("one chunk is one upload: %.2f vs %.2f", a, b)
	}
}
