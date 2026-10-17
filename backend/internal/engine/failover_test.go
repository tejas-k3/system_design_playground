package engine

import (
	"math"
	"testing"
)

// users write distinct items through one app server to a primary database that has a replica.
func failoverSim(dbP, replicaP Params, crash ChaosEvent) *Sim {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 80.0, "readPct": 0.0, "timeoutMs": 2000.0, "retries": 0.0, "keySpace": 100000.0, "keyDist": "uniform"})
	b.node("a", KAppServer, Params{"threads": 32.0, "queueCap": 500.0})
	b.node("db", KDatabase, dbP).node("r", KReplica, replicaP)
	b.edge("u", "a").edge("a", "db").edge("a", "r")
	d, _ := Sanitize(b.d)
	return New(d, Options{Seed: 5, Chaos: []ChaosEvent{crash}})
}

var failover = Params{"onFailure": "promote", "detectSec": 5.0, "promoteSec": 10.0}

func TestFailoverRestoresWritesAfterDetectionAndPromotion(t *testing.T) {
	s := failoverSim(failover, Params{"replicationLagMs": 2000.0}, ChaosEvent{Type: "crash", NodeID: "db", AtSec: 10, DownSec: 300})
	if before := successBetween(s, 2, 9); before < 0.99 {
		t.Fatalf("healthy before the crash: %.2f", before)
	}
	if during := successBetween(s, 11, 24); during > 0.05 {
		t.Errorf("no writes can succeed while there is no primary (5 s detection + 10 s promotion), success %.2f", during)
	}
	if after := successBetween(s, 27, 60); after < 0.99 {
		t.Errorf("writes should succeed on the promoted replica, success %.2f", after)
	}
	if !s.nodes["r"].promoted {
		t.Error("the replica should be the primary now")
	}
}

func TestAsyncFailoverLosesTheLastLagWorthOfWrites(t *testing.T) {
	for _, c := range []struct {
		lag      float64
		min, max int
	}{
		{2000, 120, 200}, // 80 writes/s x 2 s
		{500, 30, 55},    // 80 x 0.5
		{0, 0, 0},
	} {
		s := failoverSim(failover, Params{"replicationLagMs": c.lag}, ChaosEvent{Type: "crash", NodeID: "db", AtSec: 20, DownSec: 300})
		s.Advance(40000)
		lost := s.RunSnapshot().LostWrites
		if lost < c.min || lost > c.max {
			t.Errorf("lag %.0f ms: %d acknowledged writes lost, want %d-%d", c.lag, lost, c.min, c.max)
		}
	}
}

func TestWithoutFailoverTheDatabaseStaysDownUntilItRestarts(t *testing.T) {
	s := failoverSim(Params{}, Params{"replicationLagMs": 2000.0}, ChaosEvent{Type: "crash", NodeID: "db", AtSec: 10, DownSec: 40})
	if during := successBetween(s, 15, 45); during > 0.05 {
		t.Errorf("default behaviour is unchanged: writes fail while the primary is down, success %.2f", during)
	}
	if s.nodes["r"].promoted || s.RunSnapshot().LostWrites != 0 {
		t.Error("nothing is promoted and nothing is lost without the setting")
	}
	if after := successBetween(s, 52, 70); after < 0.99 {
		t.Errorf("recovers when the primary restarts, success %.2f", after)
	}
}

func TestPrimaryBackBeforeDetectionMeansNoFailover(t *testing.T) {
	s := failoverSim(failover, Params{"replicationLagMs": 2000.0}, ChaosEvent{Type: "crash", NodeID: "db", AtSec: 10, DownSec: 3})
	s.Advance(40000)
	if s.nodes["r"].promoted || s.RunSnapshot().LostWrites != 0 {
		t.Errorf("a 3 s blip under a 5 s detection time must not promote anything (promoted=%v lost=%d)", s.nodes["r"].promoted, s.RunSnapshot().LostWrites)
	}
}

func TestFailoverIsDeterministic(t *testing.T) {
	run := func() RunStats {
		s := failoverSim(failover, Params{"replicationLagMs": 2000.0}, ChaosEvent{Type: "crash", NodeID: "db", AtSec: 10, DownSec: 300})
		s.Advance(45000)
		return s.RunSnapshot()
	}
	a, b := run(), run()
	if a.OK != b.OK || a.Failed != b.Failed || a.LostWrites != b.LostWrites || math.Abs(a.P95-b.P95) > 1e-9 {
		t.Fatalf("same seed, different result: %+v vs %+v", a, b)
	}
}
