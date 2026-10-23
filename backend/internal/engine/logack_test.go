package engine

import "testing"

// A log with a leader and followers. In leader mode a publish is acknowledged as soon as the leader has it, and
// followers copy it a little later: if the leader dies in that gap, messages the producer was told were safe are gone.
// In all mode the acknowledgement waits for the copies, so nothing acknowledged is lost, at the price of latency.
func ackRun(mode string, lagMs float64) (lost int, p50 float64, s *Sim) {
	d := logDesign(2, "key", nil)
	for i := range d.Nodes {
		switch d.Nodes[i].ID {
		case "l":
			if mode != "" {
				d.Nodes[i].Params["ackMode"] = mode
				d.Nodes[i].Params["replicationLagMs"] = lagMs
			}
		case "u":
			d.Nodes[i].Params["rps"] = 200.0
		case "w0", "w1":
			d.Nodes[i].Params["processMs"] = 60.0 // slow consumers: acknowledged messages are still waiting in the log
		}
	}
	d, _ = Sanitize(d)
	s = New(d, Options{Seed: 8, Chaos: []ChaosEvent{{Type: "crash", NodeID: "l", AtSec: 10, DownSec: 5}}})
	s.Advance(30000)
	r := s.RunSnapshot()
	return r.LostWrites, r.P50, s
}

func TestALeaderCrashLosesWhatWasAcknowledgedButNotYetCopied(t *testing.T) {
	lost, _, _ := ackRun("leader", 300)
	t.Logf("leader acknowledgement, 300 ms copy lag, 200 publishes a second: %d acknowledged messages lost", lost)
	// 200 a second for 300 ms is about 60 messages in flight to the followers
	if lost < 30 || lost > 70 {
		t.Errorf("about 60 messages were acknowledged but not yet copied: %d", lost)
	}
}

func TestWaitingForAllCopiesLosesNothingAndCostsTheLag(t *testing.T) {
	lostAll, p50All, _ := ackRun("all", 300)
	lostInst, p50Inst, _ := ackRun("", 0)
	t.Logf("ack from all: %d lost, p50 %.0f ms; no replication modelled: %d lost, p50 %.0f ms", lostAll, p50All, lostInst, p50Inst)
	if lostAll != 0 || lostInst != 0 {
		t.Errorf("neither loses acknowledged messages: %d, %d", lostAll, lostInst)
	}
	if p50All < p50Inst+250 {
		t.Errorf("waiting for the copies adds about the 300 ms lag to every publish: %.0f vs %.0f", p50All, p50Inst)
	}
}
