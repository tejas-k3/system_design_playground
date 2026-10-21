package engine

import "testing"

// A three-node coordination cluster (Raft/ZooKeeper-like) that an app server writes small pieces of state to. While
// it has no leader it answers nothing.
func coordRun(coord Params, chaos []ChaosEvent, secs float64) *Sim {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": 0.0, "timeoutMs": 1500.0})
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 200.0})
	p := Params{"nodes": 3.0, "electionTimeoutMs": 1000.0, "heartbeatMs": 150.0, "rttMs": 2.0}
	for k, v := range coord {
		p[k] = v
	}
	b.node("zk", KCoordinator, p)
	b.edge("u", "a").edge("a", "zk")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 12, Chaos: chaos})
	s.Advance(secs * 1000)
	return s
}

// the time with no leader, from the writes that were refused for it (50 a second)
func outageSeconds(s *Sim, from, to float64) float64 {
	return float64(s.RunSnapshot().Errors["noQuorum"]) / 50
}

func TestALeaderCrashCostsAnElectionAndNothingMore(t *testing.T) {
	s := coordRun(nil, []ChaosEvent{{Type: "leaderDown", NodeID: "zk", AtSec: 10, DurationSec: 60}}, 40)
	gap := outageSeconds(s, 10, 40)
	st := s.Snapshot().Nodes["zk"].Extra
	t.Logf("no leader for about %.1f s; term %.0f, %.0f of 3 nodes up", gap, st["term"], st["nodesUp"])
	// the election timeout is randomised between 1 and 2 timeouts, plus a round trip
	if gap < 0.8 || gap > 2.4 {
		t.Errorf("an election takes between one and two election timeouts (1-2 s here), got %.1f s", gap)
	}
	if st["term"] < 2 {
		t.Errorf("a new leader means a new term: %.0f", st["term"])
	}
	if st["nodesUp"] != 2 {
		t.Errorf("two nodes are left and they are a majority: %.0f", st["nodesUp"])
	}
}

func TestLosingAFollowerChangesNothing(t *testing.T) {
	// node 0 is the first leader; take down nodes 1 only
	s := coordRun(nil, []ChaosEvent{{Type: "clusterNodeDown", NodeID: "zk", AtSec: 10, DurationSec: 60, Factor: 1, Mode: "follower"}}, 40)
	if gap := outageSeconds(s, 10, 40); gap > 0.2 {
		t.Errorf("a follower down with a majority left should not interrupt writes: %.1f s", gap)
	}
}

func TestWithoutAMajorityThereIsNoLeaderUntilNodesReturn(t *testing.T) {
	s := coordRun(nil, []ChaosEvent{{Type: "clusterNodeDown", NodeID: "zk", AtSec: 10, DurationSec: 15, Factor: 2}}, 45)
	during, after := 0.0, 0.0
	for _, p := range s.Snapshot().Series {
		if p.T > 12 && p.T < 24 {
			during += p.OkRps
		}
		if p.T > 30 {
			after += p.OkRps
		}
	}
	r := s.RunSnapshot()
	t.Logf("ok during the lost quorum: %.0f, after it: %.0f; failures: %v", during, after, r.Errors)
	if during > 1 || after < 100 {
		t.Errorf("nothing can be written without a majority, and writes resume once one is back: %.0f during, %.0f after", during, after)
	}
	if r.Errors["noQuorum"] == 0 {
		t.Errorf("the failures should say there is no quorum: %v", r.Errors)
	}
}

func TestElectionsAreReproducibleForASeed(t *testing.T) {
	ev := []ChaosEvent{{Type: "leaderDown", NodeID: "zk", AtSec: 10, DurationSec: 60}}
	a, b := outageSeconds(coordRun(nil, ev, 40), 10, 40), outageSeconds(coordRun(nil, ev, 40), 10, 40)
	if a != b {
		t.Errorf("same seed, same election: %.3f vs %.3f", a, b)
	}
}

func TestAShorterElectionTimeoutRecoversSooner(t *testing.T) {
	gap := func(ms float64) float64 {
		return outageSeconds(coordRun(Params{"electionTimeoutMs": ms}, []ChaosEvent{{Type: "leaderDown", NodeID: "zk", AtSec: 10, DurationSec: 60}}, 40), 10, 40)
	}
	slow, fast := gap(3000), gap(500)
	t.Logf("outage with a 3 s timeout: %.1f s, with 0.5 s: %.1f s", slow, fast)
	if fast >= slow {
		t.Errorf("a shorter election timeout recovers sooner: %.1f vs %.1f", fast, slow)
	}
}
