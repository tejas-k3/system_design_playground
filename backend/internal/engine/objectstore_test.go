package engine

import (
	"math"
	"testing"
)

// users upload `reqKB` per write to an app server; with a store the app uploads the body there first and queues a pointer.
func uploadFlow(reqKB, queueLimitKB float64, withStore bool) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 20.0, "readPct": 0.0, "reqKB": reqKB, "timeoutMs": 30000.0, "retries": 0.0}).node("a", KAppServer, Params{"threads": 32.0})
	b.node("q", KQueue, Params{"maxMessageKB": queueLimitKB}).node("w", KWorker, Params{"concurrency": 8.0, "processMs": 20.0}).node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "q").edge("q", "w").edge("w", "db")
	if withStore {
		b.node("s", KObjectStore, Params{"firstByteMs": 30.0, "throughputMBps": 50.0}).edge("a", "s")
	}
	d, _ := Sanitize(b.d)
	return d
}

func TestClaimCheckLetsABigUploadThroughAQueueThatRefusesTheBody(t *testing.T) {
	without := run(uploadFlow(500, 256, false), Options{Seed: 2}, 20).Snapshot().Global.Run
	if without.OK != 0 || without.Errors["messageTooLarge"] == 0 {
		t.Fatalf("500 KB bodies through a 256 KB queue fail: %+v", without)
	}
	with := run(uploadFlow(500, 256, true), Options{Seed: 2}, 20).Snapshot().Global.Run
	if with.Failed != 0 || with.OK == 0 || with.Errors["messageTooLarge"] != 0 {
		t.Fatalf("with the body in an object store and a pointer queued, every upload succeeds: %+v", with)
	}
}

func TestAnUploadPaysTheStoresFirstByteAndTransferTime(t *testing.T) {
	mean := func(withStore bool) float64 {
		return run(uploadFlow(500, 1024, withStore), Options{Seed: 2}, 30).Snapshot().Global.Run.Mean
	}
	// the store adds a round trip to it (20 ms each way by default) plus firstByteMs 30 ms plus 500 KB at 50 MB/s = 10 ms,
	// all with the default service-time model's ~5% overhead on the 40 ms of service
	extra := mean(true) - mean(false)
	want := 40 + (30+10)*1.05
	if math.Abs(extra-want) > 6 {
		t.Fatalf("uploading 500 KB should add about %.0f ms, added %.1f ms", want, extra)
	}
}

func TestOnlyUploadsTouchTheStore(t *testing.T) {
	d := uploadFlow(500, 256, true)
	for i := range d.Nodes {
		if d.Nodes[i].ID == "u" {
			d.Nodes[i].Params["readPct"] = 100.0
		}
	}
	s := run(d, Options{Seed: 2}, 10)
	if got := s.Snapshot().Nodes["s"].Arr; got != 0 {
		t.Fatalf("reads must not touch the object store, saw %.1f arrivals", got)
	}
}

func TestAStoppedStoreFailsTheUpload(t *testing.T) {
	s := New(uploadFlow(500, 256, true), Options{Seed: 2, Chaos: []ChaosEvent{{Type: "crash", NodeID: "s", AtSec: 1, DownSec: 100}}})
	for s.Now() < 15000 {
		s.Advance(100)
	}
	r := s.RunSnapshot()
	if r.Errors["nodeDown"] == 0 {
		t.Fatalf("with the store down, uploads fail: %+v", r.Errors)
	}
}

func TestObjectStoreIsARegisteredComponentWithSettings(t *testing.T) {
	sp := Spec(KObjectStore)
	if sp == nil || sp.Name == "" {
		t.Fatal("objectStore must be a registered kind")
	}
	keys := map[string]bool{}
	for _, p := range sp.Params {
		keys[p.Key] = true
	}
	for _, k := range []string{"firstByteMs", "throughputMBps", "maxConcurrent"} {
		if !keys[k] {
			t.Errorf("object store needs %s", k)
		}
	}
	if !CanConnect(KAppServer, KObjectStore) {
		t.Error("an app server must be able to send to an object store")
	}
	if CanConnect(KObjectStore, KDatabase) {
		t.Error("a store does not call anything")
	}
}

func TestTheLintAcceptsTheClaimCheckAndStillFlagsTheBigBody(t *testing.T) {
	has := func(d Diagram) bool {
		for _, f := range Lint(d) {
			if f.Rule == "large-payload-into-queue" {
				return true
			}
		}
		return false
	}
	if !has(uploadFlow(500, 256, false)) {
		t.Error("a body too big for the queue, with no store, is flagged")
	}
	if has(uploadFlow(500, 256, true)) {
		t.Error("an app server that uploads the body to an object store and queues a pointer is the fix, not the problem")
	}
}
