package engine

import "testing"

// The primary crashes at 10 s; detection takes 5 s and promotion 10 s. With a timer the replica takes over at 25 s
// whatever else is going on. When the decision belongs to a coordination service, no one can be promoted while that
// service has no leader or no majority: promoting then is how two primaries appear.
func coordFailover(electionBy string, coordChaos []ChaosEvent) (promotedAt float64, s *Sim) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 80.0, "readPct": 0.0, "timeoutMs": 2000.0, "retries": 0.0, "keySpace": 100000.0, "keyDist": "uniform"})
	b.node("a", KAppServer, Params{"threads": 32.0, "queueCap": 500.0})
	db := Params{"onFailure": "promote", "detectSec": 5.0, "promoteSec": 10.0, "electionBy": electionBy}
	b.node("db", KDatabase, db).node("r", KReplica, Params{"replicationLagMs": 500.0})
	b.node("zk", KCoordinator, Params{"nodes": 3.0, "electionTimeoutMs": 1000.0})
	b.edge("u", "a").edge("a", "db").edge("a", "r").edge("db", "zk")
	d, _ := Sanitize(b.d)
	chaos := append([]ChaosEvent{{Type: "crash", NodeID: "db", AtSec: 10, DownSec: 300}}, coordChaos...)
	s = New(d, Options{Seed: 5, Chaos: chaos})
	for i := 0; i < 600 && !s.nodes["r"].promoted; i++ {
		s.Advance(100)
	}
	return s.Now() / 1000, s
}

func TestWithAHealthyCoordinatorPromotionTakesTheSameTime(t *testing.T) {
	timer, _ := coordFailover("timer", nil)
	coord, _ := coordFailover("coordinator", nil)
	t.Logf("promoted at %.1f s with a timer, %.1f s through a healthy coordinator", timer, coord)
	if coord < timer-0.2 || coord > timer+0.5 {
		t.Errorf("a healthy coordinator adds no meaningful delay: %.1f vs %.1f", coord, timer)
	}
}

func TestWithoutAMajorityNobodyIsPromotedThroughTheCoordinator(t *testing.T) {
	lost := []ChaosEvent{{Type: "clusterNodeDown", NodeID: "zk", AtSec: 8, DurationSec: 30, Factor: 2}}
	timer, _ := coordFailover("timer", lost)
	coord, s := coordFailover("coordinator", lost)
	t.Logf("with the coordinator's majority gone from 8 s to 38 s: promoted at %.1f s by timer, %.1f s by coordinator", timer, coord)
	if timer > 26 {
		t.Errorf("the timer ignores the coordinator and promotes on schedule: %.1f", timer)
	}
	if coord < 38 || !s.nodes["r"].promoted {
		t.Errorf("through the coordinator nothing happens until a majority is back and a leader is elected: %.1f", coord)
	}
}

func TestTimerIsTheDefault(t *testing.T) {
	d, _ := coordFailover("", nil)
	timer, _ := coordFailover("timer", nil)
	if d != timer {
		t.Errorf("default must equal timer: %.1f vs %.1f", d, timer)
	}
}
