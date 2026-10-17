package engine

import (
	"math"
	"testing"
)

func TestFailoverPhasesCountDown(t *testing.T) {
	s := failoverSim(failover, Params{"replicationLagMs": 500.0}, ChaosEvent{Type: "crash", NodeID: "db", AtSec: 10, DownSec: 300})
	phase := func(atSec float64) NodeStats {
		s.Advance(atSec*1000 - s.now)
		return s.Snapshot().Nodes["db"]
	}
	if n := phase(8); n.Phase != "" {
		t.Errorf("healthy before the crash: %+v", n)
	}
	if n := phase(12); n.Phase != "detecting" || math.Abs(n.PhaseLeft-3) > 0.2 {
		t.Errorf("2 s after the crash it is still being noticed, 3 s to go: %q %.2f", n.Phase, n.PhaseLeft)
	}
	if n := phase(17); n.Phase != "promoting" || math.Abs(n.PhaseLeft-8) > 0.2 {
		t.Errorf("at 17 s a replica is being promoted, 8 s to go: %q %.2f", n.Phase, n.PhaseLeft)
	}
	if n := phase(27); n.Phase != "" {
		t.Errorf("after promotion there is no phase left on the old primary: %q", n.Phase)
	}
	if !s.Snapshot().Nodes["r"].Primary {
		t.Error("the replica is the primary now")
	}
}

func TestNoFailoverPhaseWithoutTheSetting(t *testing.T) {
	s := failoverSim(Params{}, Params{"replicationLagMs": 500.0}, ChaosEvent{Type: "crash", NodeID: "db", AtSec: 5, DownSec: 300})
	s.Advance(10000)
	if n := s.Snapshot().Nodes["db"]; n.Phase != "" {
		t.Errorf("a database that stays down has no failover phase: %q", n.Phase)
	}
}

func TestBootingServersAreCounted(t *testing.T) {
	s := scaleSim(Params{"scaleUpDelaySec": 20.0}, []TrafficPoint{{0, 40}, {20, 40}, {22, 160}, {60, 160}, {62, 40}, {400, 40}}, 3)
	sawBooting := false
	for sec := 1; sec <= 200; sec++ {
		s.Advance(1000)
		n := s.Snapshot().Nodes["a"]
		if n.Booting > 0 {
			sawBooting = true
		}
		if n.Booting != s.nodes["a"].as.pending {
			t.Fatalf("at %d s the snapshot says %d booting, the scaler has %d", sec, n.Booting, s.nodes["a"].as.pending)
		}
	}
	if !sawBooting {
		t.Error("a step in load should have ordered servers that were booting for a while")
	}
	if n := s.Snapshot().Nodes["a"]; n.Booting != 0 {
		t.Errorf("nothing is booting once the fleet has caught up: %d", n.Booting)
	}
}
