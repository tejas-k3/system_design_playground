package engine

import "testing"

// A stream job takes a checkpoint (its window state and its place in the input) every few seconds. If it crashes it
// restarts from the last checkpoint and reads the input again from there. Outputs already sent to the sink cannot be
// taken back, so with at-least-once delivery the windows that fired after the checkpoint fire a second time. With
// exactly-once the sink only accepts an output together with its checkpoint, so the repeat replaces it.
func deliveryEvents() []float64 {
	var ev []float64
	for i := 0; i < 6000; i++ { // 100 events a second for 60 s, in order
		ev = append(ev, float64(i)*0.01)
	}
	return ev
}

func TestAtLeastOnceRepeatsOutputsAfterACrash(t *testing.T) {
	ev := deliveryEvents()
	clean := RunStream(ev, 2, 0, 10, "atLeastOnce", -1)
	alo := RunStream(ev, 2, 0, 10, "atLeastOnce", 3800)
	eo := RunStream(ev, 2, 0, 10, "exactlyOnce", 3800)
	t.Logf("windows delivered: clean %d, at-least-once after a crash %d (%d duplicates), exactly-once %d (%d duplicates)", len(clean.Delivered), len(alo.Delivered), alo.Duplicates, len(eo.Delivered), eo.Duplicates)
	if clean.Duplicates != 0 {
		t.Errorf("no crash, no repeats: %d", clean.Duplicates)
	}
	if alo.Duplicates == 0 {
		t.Errorf("at-least-once repeats the windows that fired after the last checkpoint")
	}
	if eo.Duplicates != 0 {
		t.Errorf("exactly-once never repeats an output: %d", eo.Duplicates)
	}
	if len(eo.Delivered) != len(clean.Delivered) {
		t.Errorf("exactly-once delivers the same windows as a run without a crash: %d vs %d", len(eo.Delivered), len(clean.Delivered))
	}
	// 2 s windows, a checkpoint every 10 s, a crash at 38 s: the checkpoint is at 30 s, so the windows ending at 32, 34 and 36 s had fired and fire again
	if alo.Duplicates != 3 {
		t.Errorf("three windows fire a second time: %d", alo.Duplicates)
	}
	if eo.CheckpointCost <= alo.CheckpointCost {
		t.Errorf("exactly-once pays for its guarantee: every checkpoint also commits the outputs, %v vs %v", eo.CheckpointCost, alo.CheckpointCost)
	}
}
