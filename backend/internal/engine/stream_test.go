package engine

import "testing"

// Clicks go from the app server into a stream job that counts them in 10 s windows of event time. One in ten events
// is delayed by 8 s (a phone that was offline). With a watermark 2 s behind and no allowed lateness, the late tenth
// is dropped; allowing 10 s of lateness updates the windows instead.
func streamRun(sp Params, crash bool) (*Sim, map[string]float64) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 100.0, "readPct": 0.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 2000.0})
	b.node("a", KAppServer, Params{"threads": 32.0})
	p := Params{"windowSec": 10.0, "watermarkDelaySec": 2.0, "lateEventPct": 10.0, "lateBySec": 8.0}
	for k, v := range sp {
		p[k] = v
	}
	b.node("st", KStream, p)
	b.edge("u", "a").edge("a", "st")
	d, _ := Sanitize(b.d)
	var ch []ChaosEvent
	if crash {
		ch = []ChaosEvent{{Type: "crash", NodeID: "st", AtSec: 33, DownSec: 2}}
	}
	s := New(d, Options{Seed: 12, Chaos: ch})
	s.Advance(61000)
	return s, s.Snapshot().Nodes["st"].Extra
}

func TestAStreamDropsEventsThatAreLaterThanItAllows(t *testing.T) {
	_, strict := streamRun(nil, false)
	_, patient := streamRun(Params{"allowedLatenessSec": 10.0}, false)
	t.Logf("strict: %.0f windows, %.0f late events dropped; 10 s lateness: %.0f dropped, %.0f window updates", strict["windows"], strict["lateDropped"], patient["lateDropped"], patient["windowUpdates"])
	if strict["windows"] < 4 {
		t.Fatalf("six 10 s windows in a minute: %v", strict["windows"])
	}
	// 6,000 events, 10% of them 8 s late against a 2 s watermark delay in 10 s windows: an event is dropped when its
	// window ended at least 6 s after it happened, which is 6 of every 10 positions: 6,000 x 0.1 x 0.6 = 360
	if strict["lateDropped"] < 300 || strict["lateDropped"] > 450 {
		t.Errorf("about 360 late events should miss their window: %v", strict["lateDropped"])
	}
	if patient["lateDropped"] != 0 || patient["windowUpdates"] == 0 {
		t.Errorf("lateness of 10 s catches them as updates: dropped %v, updates %v", patient["lateDropped"], patient["windowUpdates"])
	}
}

func TestACrashRepeatsWindowsOnlyWithAtLeastOnce(t *testing.T) {
	_, alo := streamRun(Params{"windowSec": 2.0, "checkpointSec": 10.0}, true)
	_, eo := streamRun(Params{"windowSec": 2.0, "checkpointSec": 10.0, "delivery": "exactlyOnce"}, true)
	t.Logf("duplicate outputs after a crash at 33 s: at-least-once %.0f, exactly-once %.0f", alo["duplicateWindows"], eo["duplicateWindows"])
	if alo["duplicateWindows"] == 0 {
		t.Errorf("windows that fired after the checkpoint fire again")
	}
	if eo["duplicateWindows"] != 0 {
		t.Errorf("exactly-once never repeats an output: %v", eo["duplicateWindows"])
	}
}

// Counting the different visitors in a stream: exactly (a table with every id seen) or with HyperLogLog (a few
// bytes, a small error). 200 events a second over 5,000 possible visitors for a minute.
func uniqueRun(counting string, p float64) map[string]float64 {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 200.0, "readPct": 0.0, "keySpace": 5000.0, "keyDist": "uniform", "timeoutMs": 2000.0})
	b.node("a", KAppServer, Params{"threads": 32.0})
	b.node("st", KStream, Params{"counting": counting, "hllPrecision": p})
	b.edge("u", "a").edge("a", "st")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 12})
	s.Advance(60000)
	return s.Snapshot().Nodes["st"].Extra
}

func TestHyperLogLogCountsVisitorsInAFractionOfTheMemory(t *testing.T) {
	exact := uniqueRun("exact", 10)
	hll := uniqueRun("hyperLogLog", 10)
	t.Logf("exact: %.0f visitors, %.1f KB; HyperLogLog p=10: %.0f, %.1f KB, error %.2f%%", exact["uniques"], exact["stateKB"], hll["uniques"], hll["stateKB"], hll["countErrorPct"])
	if exact["uniques"] < 4500 || exact["countErrorPct"] != 0 {
		t.Errorf("exact counting is exact: %v, error %v", exact["uniques"], exact["countErrorPct"])
	}
	if hll["stateKB"]*20 > exact["stateKB"] {
		t.Errorf("1,024 registers take a small fraction of the table: %.1f vs %.1f KB", hll["stateKB"], exact["stateKB"])
	}
	// the standard error at p = 10 is 1.04 / sqrt(1024) = 3.25%; three of them is the limit
	if hll["countErrorPct"] > 9.75 {
		t.Errorf("error beyond three standard errors: %.2f%%", hll["countErrorPct"])
	}
	tiny := uniqueRun("hyperLogLog", 4)
	if tiny["countErrorPct"] <= hll["countErrorPct"] {
		t.Errorf("sixteen registers are less accurate than 1,024: %.2f%% vs %.2f%%", tiny["countErrorPct"], hll["countErrorPct"])
	}
}

// The ten most popular items of a Zipf crowd, found by a stream job: counting every item exactly, or with a
// count-min sketch and a short candidate list in a fixed table. Recall is how many of the true top ten it finds.
func topKRun(mode string, width, depth float64) map[string]float64 {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 300.0, "readPct": 0.0, "keySpace": 200000.0, "keyDist": "zipf", "zipfS": 1.1, "timeoutMs": 2000.0})
	b.node("a", KAppServer, Params{"threads": 32.0})
	b.node("st", KStream, Params{"topK": mode, "sketchWidth": width, "sketchDepth": depth})
	b.edge("u", "a").edge("a", "st")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 12})
	s.Advance(60000)
	return s.Snapshot().Nodes["st"].Extra
}

func TestASketchFindsTheTopTenInFarLessMemory(t *testing.T) {
	exact := topKRun("exact", 0, 0)
	sk := topKRun("countMin", 1000, 3)
	tiny := topKRun("countMin", 40, 2)
	t.Logf("exact: recall %.0f/10, %.1f KB; count-min 1000x3: recall %.0f/10, %.1f KB; 40x2: recall %.0f/10, %.1f KB", exact["topKRecall"]*10, exact["stateKB"], sk["topKRecall"]*10, sk["stateKB"], tiny["topKRecall"]*10, tiny["stateKB"])
	if exact["topKRecall"] != 1 {
		t.Errorf("counting every item finds the true top ten: %v", exact["topKRecall"])
	}
	if sk["topKRecall"] < 0.9 || sk["stateKB"]*3 > exact["stateKB"] {
		t.Errorf("the sketch should find at least 9 of 10 in a third of the memory: %v, %.1f vs %.1f KB", sk["topKRecall"], sk["stateKB"], exact["stateKB"])
	}
	if tiny["topKRecall"] >= sk["topKRecall"] {
		t.Errorf("less memory should cost recall: %v vs %v", tiny["topKRecall"], sk["topKRecall"])
	}
}
