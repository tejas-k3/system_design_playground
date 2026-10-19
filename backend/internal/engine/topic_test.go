package engine

import (
	"fmt"
	"testing"
)

func topicDesign(subs []float64) Diagram { // processMs per subscriber
	b := &builder{}
	b.node("u", KClient, Params{"rps": 30.0, "readPct": 0.0, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 32.0}).
		node("t", KTopic, Params{"capacity": 20000.0})
	b.edge("u", "a").edge("a", "t")
	for i, ms := range subs {
		id := fmt.Sprintf("w%d", i)
		b.node(id, KWorker, Params{"concurrency": 1.0, "processMs": ms})
		b.edge("t", id)
	}
	d, _ := Sanitize(b.d)
	return d
}

// Unlike a queue, where workers compete for each job, every subscriber of a topic gets its own copy of every message.
func TestEveryPublishIsDeliveredToEverySubscriber(t *testing.T) {
	s := run(topicDesign([]float64{10, 10, 10}), Options{Seed: 4}, 40)
	r := s.RunSnapshot()
	published := r.OK
	// publishes still in flight at the end of the run have been fanned out but are not counted as answered yet
	if r.Jobs.Enqueued < published*3-9 || r.Jobs.Enqueued > published*3+30 {
		t.Fatalf("one delivery per subscriber: %d deliveries for %d publishes", r.Jobs.Enqueued, published)
	}
	for i := 0; i < 3; i++ {
		got := s.Snapshot().Nodes[fmt.Sprintf("w%d", i)]
		if got.Ok < 25 { // 30 messages a second
			t.Fatalf("subscriber %d should handle about 30/s, handles %.1f", i, got.Ok)
		}
	}
	if r.Jobs.Completed < r.Jobs.Enqueued*95/100 {
		t.Fatalf("healthy subscribers keep up: %d of %d", r.Jobs.Completed, r.Jobs.Enqueued)
	}
}

// A subscriber that cannot keep up falls behind on its own: its backlog grows, the others and the publisher do not notice.
func TestASlowSubscriberFallsBehindAloneAndTheOthersAreUnaffected(t *testing.T) {
	s := run(topicDesign([]float64{10, 10, 200}), Options{Seed: 4}, 40) // the third can do about 5 a second
	snap := s.Snapshot()
	slow, fast := snap.Nodes["w2"].QLen, snap.Nodes["w0"].QLen
	if slow < 400 || fast > 5 {
		t.Fatalf("the slow subscriber's backlog should climb (about 25 a second) while a fast one's stays empty: slow %d fast %d", slow, fast)
	}
	r := s.RunSnapshot()
	if r.Failed != 0 {
		t.Fatalf("the publisher never waits for subscribers: %d failures", r.Failed)
	}
	if got := snap.Nodes["w0"].Ok; got < 25 {
		t.Fatalf("a fast subscriber still handles the full rate: %.1f/s", got)
	}
}
