package engine

import "testing"

// Two workers read a four-partition log as one group. When one crashes, its partitions must go to the other. With a
// stop-the-world rebalance the whole group stops reading for a while first, so the backlog grows for everyone.
func groupRebalanceRun(pauseSec float64, crash bool) (peakBacklog int, s *Sim) {
	d := logDesign(2, "key", nil)
	for i := range d.Nodes {
		if d.Nodes[i].ID == "l" {
			d.Nodes[i].Params["rebalanceSec"] = pauseSec
		}
	}
	opts := Options{Seed: 8}
	if crash {
		opts.Chaos = []ChaosEvent{{Type: "crash", NodeID: "w1", AtSec: 10, DownSec: 20}}
	}
	d, _ = Sanitize(d)
	s = New(d, opts)
	for range 45 {
		s.Advance(1000)
		if b := s.RunSnapshot().Jobs; b != nil && b.Backlog > peakBacklog {
			peakBacklog = b.Backlog
		}
	}
	return
}

func TestARebalancePausesTheWholeGroup(t *testing.T) {
	instant, _ := groupRebalanceRun(0, true)
	paused, s := groupRebalanceRun(8, true)
	rb := s.Snapshot().Nodes["l"].Extra["rebalances"]
	t.Logf("peak backlog %d with an instant handover, %d with an 8 s rebalance; %.0f rebalances", instant, paused, rb)
	if paused < instant+300 {
		t.Errorf("60 messages a second for 8 s with nobody reading: the backlog should be about 480 higher, got %d vs %d", paused, instant)
	}
	if rb != 2 {
		t.Errorf("one when the member leaves and one when it comes back: %.0f", rb)
	}
}

func TestNoMembershipChangeNoRebalance(t *testing.T) {
	_, s := groupRebalanceRun(8, false)
	if rb := s.Snapshot().Nodes["l"].Extra["rebalances"]; rb != 0 {
		t.Errorf("a stable group never rebalances: %.0f", rb)
	}
}

func TestRebalanceSecZeroIsTheOldInstantHandover(t *testing.T) {
	a, _ := groupRebalanceRun(0, true)
	d := logDesign(2, "key", nil)
	d, _ = Sanitize(d)
	s := New(d, Options{Seed: 8, Chaos: []ChaosEvent{{Type: "crash", NodeID: "w1", AtSec: 10, DownSec: 20}}})
	peak := 0
	for range 45 {
		s.Advance(1000)
		if b := s.RunSnapshot().Jobs; b != nil && b.Backlog > peak {
			peak = b.Backlog
		}
	}
	if a != peak {
		t.Errorf("unset must equal 0: %d vs %d", a, peak)
	}
}
