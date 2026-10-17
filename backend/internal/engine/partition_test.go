package engine

import "testing"

func partitionRun(ev ChaosEvent) *Sim {
	b := stack(1, Params{"rps": 60.0, "readPct": 100.0, "retries": 0.0, "timeoutMs": 800.0}, Params{"threads": 64.0, "queueCap": 500.0}, nil, nil)
	d, _ := Sanitize(b.d)
	return New(d, Options{Seed: 3, Chaos: []ChaosEvent{ev}})
}

// A partition is silence, not an error: callers get no answer at all and only their own timeouts save them,
// where a crashed node refuses at once. That difference is the lesson.
func TestPartitionMeansSilenceNotErrors(t *testing.T) {
	s := partitionRun(ChaosEvent{Type: "partition", NodeID: "db", AtSec: 3, DurationSec: 4})
	s.Advance(5000)
	if !s.Snapshot().Nodes["db"].Cut {
		t.Error("the snapshot should say the node is cut off while the partition lasts")
	}
	s.Advance(2000)
	r := s.RunSnapshot()
	if r.Errors["nodeDown"] != 0 {
		t.Errorf("a partitioned node must not answer 'down': %+v", r.Errors)
	}
	if r.Errors["timeout"] < 100 {
		t.Errorf("callers should have given up on the silence: %+v", r.Errors)
	}
	if r.FailP50 < 700 || r.FailP50 > 900 {
		t.Errorf("failures should take about the 800 ms client timeout, median %.0f ms", r.FailP50)
	}

	crash := partitionRun(ChaosEvent{Type: "crash", NodeID: "db", AtSec: 3, DownSec: 4})
	crash.Advance(7000)
	cr := crash.RunSnapshot()
	if cr.Errors["nodeDown"] == 0 || cr.FailP50 > 300 {
		t.Errorf("a crash should be refused quickly: errors %+v, median failure %.0f ms", cr.Errors, cr.FailP50)
	}
}

func TestPartitionHealsAndHeldCallsAreDelivered(t *testing.T) {
	s := partitionRun(ChaosEvent{Type: "partition", NodeID: "db", AtSec: 3, DurationSec: 4})
	if rate := successBetween(s, 15, 25); rate < 0.99 {
		t.Errorf("the system should be healthy again after the partition heals, success %.2f", rate)
	}
	if n := s.nodes["db"]; len(n.held) != 0 || n.cut {
		t.Errorf("partition state left behind: cut=%v held=%d", n.cut, len(n.held))
	}
}
