package engine

import "testing"

// When a log's leader dies the producers get errors until a follower takes over. Without followers modelled the log
// is simply down until the node returns; with a take-over time of 3 s the outage is 3 s whatever the crash lasts.
func leaderRun(electionSec float64) (failed int, s *Sim) {
	d := logDesign(2, "key", nil)
	for i := range d.Nodes {
		switch d.Nodes[i].ID {
		case "l":
			d.Nodes[i].Params["ackMode"] = "all"
			d.Nodes[i].Params["replicationLagMs"] = 50.0
			d.Nodes[i].Params["leaderElectionSec"] = electionSec
		case "u":
			d.Nodes[i].Params["rps"] = 100.0
		}
	}
	d, _ = Sanitize(d)
	s = New(d, Options{Seed: 8, Chaos: []ChaosEvent{{Type: "crash", NodeID: "l", AtSec: 10, DownSec: 20}}})
	s.Advance(40000)
	r := s.RunSnapshot()
	return r.Failed, s
}

func TestAFollowerTakesOverAfterTheElectionTime(t *testing.T) {
	none, _ := leaderRun(0)
	three, s := leaderRun(3)
	t.Logf("failed publishes in a 20 s crash: %d without a take-over, %d with a 3 s election", none, three)
	// 100 a second: 20 s is about 2,000 failures, 3 s about 300
	if none < 1700 || none > 2300 {
		t.Errorf("the log is down for the whole 20 s: %d", none)
	}
	if three < 200 || three > 450 {
		t.Errorf("only the 3 s of election should fail: %d", three)
	}
	if s.nodes["l"].down {
		t.Errorf("a follower is leading, so the log is up again")
	}
}
