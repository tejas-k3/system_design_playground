package engine

import (
	"strings"
	"testing"
)

func TestCoordinatorDescriptionSeparatesSimulatorFromZooKeeperReads(t *testing.T) {
	spec := Spec(KCoordinator)
	if !strings.Contains(spec.Description, "simulator models a leader-based cluster") || !strings.Contains(spec.Description, "writes wait for a majority") {
		t.Errorf("coordinator description must explain the simulated leader and majority: %q", spec.Description)
	}
	if !strings.Contains(spec.Description, "Real ZooKeeper may serve reads from a follower, so they can be stale.") {
		t.Errorf("coordinator description must distinguish ZooKeeper follower reads: %q", spec.Description)
	}
	if !strings.Contains(spec.Lesson, "(n-1)/2 failures") {
		t.Errorf("coordinator lesson must retain odd-cluster failure tolerance: %q", spec.Lesson)
	}
}

// A three-node coordination cluster (Raft/ZooKeeper-like) that an app server writes small pieces of state to. While
// it has no leader it answers nothing.
func coordRun(coord Params, chaos []ChaosEvent, secs float64) *Sim {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": 0.0, "timeoutMs": 1500.0})
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 200.0})
	p := Params{"nodes": 3.0, "electionTimeoutMs": 1000.0, "rttMs": 2.0}
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

// 200 elections after a leader crash, each with a different seed, timed from the engine's own event log (the moment
// the leader is lost to the moment a new one is elected). A candidate waits a random time between one and two election
// timeouts, plus a round trip: every outage lies in [timeout, 2 x timeout + rtt] and the mean is 1.5 x timeout + rtt.
func TestTwoHundredElectionsStayInTheDocumentedRangeAroundTheirMean(t *testing.T) {
	const timeout, rtt, trials = 1000.0, 2.0, 200
	var sum, lo, hi float64
	lo = 1e9
	for seed := uint32(1); seed <= trials; seed++ {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 10.0, "readPct": 0.0, "timeoutMs": 1500.0})
		b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 200.0})
		b.node("zk", KCoordinator, Params{"nodes": 3.0, "electionTimeoutMs": timeout, "rttMs": rtt})
		b.edge("u", "a").edge("a", "zk")
		d, _ := Sanitize(b.d)
		s := New(d, Options{Seed: seed, Chaos: []ChaosEvent{{Type: "leaderDown", NodeID: "zk", AtSec: 5, DurationSec: 60}}})
		s.Advance(15000)
		var lost, elected float64
		for _, e := range s.events {
			switch e.Type {
			case "noLeader":
				lost = e.T * 1000
			case "elected":
				elected = e.T * 1000
			}
		}
		if lost == 0 || elected <= lost {
			t.Fatalf("seed %d: no election recorded (lost %.0f, elected %.0f)", seed, lost, elected)
		}
		gap := elected - lost
		sum += gap
		lo, hi = min(lo, gap), max(hi, gap)
	}
	mean := sum / trials
	t.Logf("%d elections: shortest %.0f ms, longest %.0f ms, mean %.0f ms (timeout %.0f ms; expected mean %.0f)", trials, lo, hi, mean, timeout, 1.5*timeout+rtt)
	if lo < timeout || hi > 2*timeout+rtt+1 {
		t.Errorf("every election lies in [timeout, 2 x timeout + rtt] = [%.0f, %.0f] ms: got [%.0f, %.0f]", timeout, 2*timeout+rtt, lo, hi)
	}
	if mean < 1.5*timeout+rtt-60 || mean > 1.5*timeout+rtt+60 {
		t.Errorf("the mean outage should be about 1.5 x timeout + rtt = %.0f ms: %.0f", 1.5*timeout+rtt, mean)
	}
}

// A leader cut off from the others does not know it yet. Until it notices (it steps down after a while) it goes on
// acknowledging writes that no other member has; when the others elect a new leader (a higher term) those writes are
// lost. A leader that steps down quickly refuses instead of acknowledging.
func cutLeaderRun(stepDownSec float64) *Sim {
	return coordRun(Params{"stepDownSec": stepDownSec}, []ChaosEvent{{Type: "leaderCut", NodeID: "zk", AtSec: 5, DurationSec: 15}}, 30)
}

func TestALeaderThatDoesNotNoticeAcknowledgesWritesThatAreLost(t *testing.T) {
	slow, quick, none := cutLeaderRun(6), cutLeaderRun(0.2), coordRun(nil, nil, 30)
	sr, qr := slow.RunSnapshot(), quick.RunSnapshot()
	t.Logf("steps down after 6 s: %d acknowledged writes lost, %d ok; after 0.2 s: %d lost, %d ok", sr.LostWrites, sr.OK, qr.LostWrites, qr.OK)
	// 50 writes a second for the 6 s the old leader keeps answering
	if sr.LostWrites < 250 || sr.LostWrites > 350 {
		t.Errorf("about 6 s of writes at 50 a second are acknowledged by the old leader and lost: %d", sr.LostWrites)
	}
	if qr.LostWrites > sr.LostWrites/20+1 || none.RunSnapshot().LostWrites != 0 {
		t.Errorf("the loss is the time the old leader keeps answering: 0.2 s loses a small fraction of the 6 s case: %d", qr.LostWrites)
	}
	if now := cutLeaderRun(0).RunSnapshot(); now.LostWrites != 0 {
		t.Errorf("a leader that steps down at once acknowledges nothing it cannot commit: %d", now.LostWrites)
	}
}

func TestAnElectionRaisesTheTermAndIsCounted(t *testing.T) {
	s := cutLeaderRun(0.2)
	x := s.Snapshot().Nodes["zk"].Extra
	if x["term"] < 2 || x["elections"] < 1 {
		t.Errorf("a new leader is elected in a higher term: term %v, elections %v", x["term"], x["elections"])
	}
	quiet := coordRun(nil, nil, 30).Snapshot().Nodes["zk"].Extra
	if quiet["elections"] != 0 || quiet["term"] != 1 {
		t.Errorf("no failure, no election: term %v, elections %v", quiet["term"], quiet["elections"])
	}
}

func TestTheOldLeaderStopsAnsweringWhenTheCutEnds(t *testing.T) {
	// a 30 s step-down cannot outlast a cut of 3 s: about 3 s of writes at 50 a second are lost, not 30 s
	s := coordRun(Params{"stepDownSec": 30.0}, []ChaosEvent{{Type: "leaderCut", NodeID: "zk", AtSec: 5, DurationSec: 3}}, 30)
	if lost := s.RunSnapshot().LostWrites; lost < 100 || lost > 200 {
		t.Errorf("the window is the shorter of the step-down time and the cut: %d", lost)
	}
}
