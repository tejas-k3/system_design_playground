package engine

import (
	"fmt"
	"testing"
)

// users -> app -> log (4 partitions) -> workers (consumer group 1, 2 members unless stated)
func logDesign(workers int, partitionBy string, groups []float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 60.0, "readPct": 0.0, "keySpace": 20.0, "keyDist": "uniform", "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 32.0}).
		node("l", KLog, Params{"partitions": 4.0, "partitionBy": partitionBy, "capacity": 20000.0})
	b.edge("u", "a").edge("a", "l")
	for i := 0; i < workers; i++ {
		g := 1.0
		if i < len(groups) {
			g = groups[i]
		}
		id := fmt.Sprintf("w%d", i)
		b.node(id, KWorker, Params{"concurrency": 4.0, "processMs": 40.0, "variability": "exponential", "group": g})
		b.edge("l", id)
	}
	d, _ := Sanitize(b.d)
	return d
}

func TestPartitionsAreSharedByTheMembersOfAGroup(t *testing.T) {
	s := run(logDesign(2, "key", nil), Options{Seed: 2}, 40)
	r := s.RunSnapshot()
	if r.Jobs.Enqueued < r.OK-5 || r.Jobs.Enqueued > r.OK+60 {
		t.Fatalf("one group means one delivery per message: %d deliveries for %d publishes", r.Jobs.Enqueued, r.OK)
	}
	a, b := s.Snapshot().Nodes["w0"].Ok, s.Snapshot().Nodes["w1"].Ok
	if a < 20 || b < 20 || a+b < 50 {
		t.Fatalf("each of the two members consumes two of the four partitions (about half of 60/s): %.1f and %.1f", a, b)
	}
}

func TestEachGroupGetsEveryMessage(t *testing.T) {
	s := run(logDesign(2, "key", []float64{1, 2}), Options{Seed: 2}, 40)
	r := s.RunSnapshot()
	if r.Jobs.Enqueued < 2*r.OK-10 {
		t.Fatalf("two groups mean two deliveries per message: %d for %d publishes", r.Jobs.Enqueued, r.OK)
	}
	for _, id := range []string{"w0", "w1"} {
		if got := s.Snapshot().Nodes[id].Ok; got < 50 {
			t.Fatalf("a lone member of its group consumes everything (about 60/s): %s %.1f", id, got)
		}
	}
}

// Messages of one key land on one partition and are consumed in order. Spread them round-robin and two workers handle
// the same key's messages at once: a later message can overtake an earlier one.
func TestKeyPartitioningPreservesPerKeyOrderAndRoundRobinDoesNot(t *testing.T) {
	keyed := run(logDesign(4, "key", nil), Options{Seed: 2}, 40).RunSnapshot()
	rr := run(logDesign(4, "roundRobin", nil), Options{Seed: 2}, 40).RunSnapshot()
	t.Logf("order violations: key %d, round robin %d", keyed.Jobs.OrderViolations, rr.Jobs.OrderViolations)
	if keyed.Jobs.OrderViolations != 0 {
		t.Fatalf("a key stays on one partition and one worker: %d violations", keyed.Jobs.OrderViolations)
	}
	if rr.Jobs.OrderViolations < 20 {
		t.Fatalf("round robin spreads one key over several workers: only %d violations", rr.Jobs.OrderViolations)
	}
}

// One partition is consumed by one member, so more members than partitions leave the extra ones idle.
func TestMoreMembersThanPartitionsAddNothing(t *testing.T) {
	a := run(logDesign(4, "key", nil), Options{Seed: 2}, 40).Snapshot()
	b := run(logDesign(8, "key", nil), Options{Seed: 2}, 40).Snapshot()
	idle := 0
	for i := 4; i < 8; i++ {
		if b.Nodes[fmt.Sprintf("w%d", i)].Ok == 0 {
			idle++
		}
	}
	if idle != 4 {
		t.Fatalf("the 5th to 8th members get no partition: %d idle", idle)
	}
	if a.Nodes["w0"].Ok < 10 {
		t.Fatalf("with four members each takes one partition: %v", a.Nodes["w0"].Ok)
	}
}
