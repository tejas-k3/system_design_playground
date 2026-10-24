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
