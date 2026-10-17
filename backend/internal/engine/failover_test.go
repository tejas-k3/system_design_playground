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

func TestWritesFailAsNoPrimaryWhileFailingOver(t *testing.T) {
	s := failoverSim(failover, Params{"replicationLagMs": 500.0}, ChaosEvent{Type: "crash", NodeID: "db", AtSec: 10, DownSec: 300})
	s.Advance(27000) // promoted at 25 s
	r := s.RunSnapshot()
	if r.Errors["noPrimary"] < 300 {
		t.Errorf("writes during the failover window should say 'no primary': %+v", r.Errors)
	}
	if r.Errors["nodeDown"] > 60 {
		t.Errorf("only requests already in flight at the crash are 'node down': %+v", r.Errors)
	}
	before := r.Errors["noPrimary"]
	s.Advance(30000) // 30 s more
	if after := s.RunSnapshot().Errors["noPrimary"]; after > before+30 {
		t.Errorf("no more 'no primary' once a replica is promoted: %d -> %d", before, after)
	}
	plain := failoverSim(Params{}, Params{"replicationLagMs": 500.0}, ChaosEvent{Type: "crash", NodeID: "db", AtSec: 10, DownSec: 300})
	plain.Advance(24000)
	if plain.RunSnapshot().Errors["noPrimary"] != 0 {
		t.Error("without failover configured a dead database is simply down")
	}
}

func TestSynchronousReplicationLosesNothingButWritesWaitForTheReplica(t *testing.T) {
	crash := ChaosEvent{Type: "crash", NodeID: "db", AtSec: 20, DownSec: 300}
	async := failoverSim(failover, Params{"replicationLagMs": 2000.0}, crash)
	async.Advance(40000)
	sync := failoverSim(failover, Params{"replicationLagMs": 2000.0, "replication": "sync"}, crash)
	sync.Advance(40000)
	if lost := sync.RunSnapshot().LostWrites; lost != 0 {
		t.Errorf("synchronous replication loses no acknowledged write, lost %d", lost)
	}
	if lost := async.RunSnapshot().LostWrites; lost < 100 {
		t.Fatalf("the asynchronous baseline should lose about 160 writes, lost %d", lost)
	}
	// the price: every write also waits for the replica (its write time plus the network round trip)
	quiet := ChaosEvent{Type: "crash", NodeID: "nope", AtSec: 1000}
	a, b := failoverSim(failover, Params{"replicationLagMs": 2000.0}, quiet), failoverSim(failover, Params{"replicationLagMs": 2000.0, "replication": "sync"}, quiet)
	a.Advance(20000)
	b.Advance(20000)
	if b.RunSnapshot().P50 < a.RunSnapshot().P50+25 {
		t.Errorf("synchronous writes should be slower by roughly the replica's write time: p50 %.0f vs %.0f ms", b.RunSnapshot().P50, a.RunSnapshot().P50)
	}
}

func TestSynchronousReplicaThatIsDownStopsWrites(t *testing.T) {
	s := failoverSim(Params{}, Params{"replication": "sync"}, ChaosEvent{Type: "crash", NodeID: "r", AtSec: 10, DownSec: 30})
	if during := successBetween(s, 15, 35); during > 0.05 {
		t.Errorf("a primary that must wait for a dead synchronous replica cannot acknowledge writes, success %.2f", during)
	}
	if s.RunSnapshot().Errors["syncReplica"] == 0 {
		t.Errorf("failures should name the replica: %+v", s.RunSnapshot().Errors)
	}
	if after := successBetween(s, 48, 60); after < 0.99 {
		t.Errorf("writes resume when the replica is back, success %.2f", after)
	}
}

func TestUnfencedOldPrimaryLosesTheWritesMadeAfterPromotion(t *testing.T) {
	restartsAfterPromotion := ChaosEvent{Type: "crash", NodeID: "db", AtSec: 10, DownSec: 40} // promoted at 25 s, old primary back at 50 s
	lost := func(db Params) int {
		s := failoverSim(db, Params{"replicationLagMs": 500.0}, restartsAfterPromotion)
		s.Advance(70000)
		return s.RunSnapshot().LostWrites
	}
	fenced := lost(failover)
	unfenced := lost(Params{"onFailure": "promote", "detectSec": 5.0, "promoteSec": 10.0, "fencing": false})
	if fenced > 100 {
		t.Errorf("a fenced old primary stays out: only the lag window is lost, got %d", fenced)
	}
	if unfenced < 1500 {
		t.Errorf("an unfenced old primary rejoins as primary and everything written to the replica since promotion (about 80/s for 25 s) is lost, got %d", unfenced)
	}
}
