package engine

import "testing"

// Replaying the log: a bug is found in the consumers, they are fixed, and the group reads the last 10 seconds again.
// At 60 messages a second that is about 600 messages processed a second time (counted as redelivered), if the log still has them.
func replayRun(retSec, backSec float64) RunStats {
	d := logDesign(2, "key", nil)
	for i := range d.Nodes {
		if d.Nodes[i].ID == "l" && retSec > 0 {
			d.Nodes[i].Params["retentionSec"] = retSec
		}
	}
	d, _ = Sanitize(d)
	s := New(d, Options{Seed: 8, Chaos: []ChaosEvent{{Type: "replay", NodeID: "l", AtSec: 30, Factor: backSec}}})
	s.Advance(60000)
	return s.RunSnapshot()
}

func TestReplayReadsTheRetainedMessagesAgain(t *testing.T) {
	base := replayRun(60, 0)
	again := replayRun(60, 10)
	short := replayRun(5, 10)
	extra := again.Jobs.Redelivered - base.Jobs.Redelivered
	t.Logf("read again: %d without a replay, %d replaying 10 s; with a retention of only 5 s: %d", base.Jobs.Redelivered, extra, short.Jobs.Redelivered)
	if extra < 500 || extra > 700 {
		t.Errorf("10 s at 60 a second is about 600 messages: %d", extra)
	}
	if e := short.Jobs.Redelivered; e < 200 || e > 400 {
		t.Errorf("only the last 5 s are still retained, about 300 messages: %d", e)
	}
}
