package engine

import "testing"

// A log keeps messages for a while, not forever. Consumers that are away for longer than the retention find the
// oldest messages gone when they return: 60 messages a second, both consumers down from 5 s to 35 s, retention 10 s,
// so what was published from 5 s to 25 s has expired by the time they are back (20 s x 60 = 1,200 messages).
func retentionRun(retSec float64) RunStats {
	d := logDesign(2, "key", nil)
	for i := range d.Nodes {
		if d.Nodes[i].ID == "l" && retSec > 0 {
			d.Nodes[i].Params["retentionSec"] = retSec
		}
	}
	d, _ = Sanitize(d)
	s := New(d, Options{Seed: 8, Chaos: []ChaosEvent{
		{Type: "crash", NodeID: "w0", AtSec: 5, DownSec: 30},
		{Type: "crash", NodeID: "w1", AtSec: 5, DownSec: 30},
	}})
	s.Advance(60000)
	return s.RunSnapshot()
}

func TestConsumersAwayLongerThanTheRetentionLoseMessages(t *testing.T) {
	forever, short := retentionRun(0), retentionRun(10)
	t.Logf("lost messages: %d with no expiry, %d with 10 s retention", forever.Jobs.Lost, short.Jobs.Lost)
	if forever.Jobs.Lost != 0 {
		t.Errorf("nothing expires by default: %d", forever.Jobs.Lost)
	}
	if short.Jobs.Lost < 1000 || short.Jobs.Lost > 1400 {
		t.Errorf("about 1,200 messages outlive the retention while the consumers are away: %d", short.Jobs.Lost)
	}
}
