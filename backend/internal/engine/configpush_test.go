package engine

import "testing"

// A configuration push: at 10 s every app server gets a smaller thread pool (a bad value rolled out to everyone at
// once), and 10 s later the old value is pushed back. Success drops for those 10 s and for no longer.
func pushDesign() Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 1000.0, "retries": 0.0})
	b.node("lb", KLoadBalancer, nil)
	b.node("a1", KAppServer, Params{"threads": 16.0, "cpuMs": 20.0, "queueCap": 20.0})
	b.node("a2", KAppServer, Params{"threads": 16.0, "cpuMs": 20.0, "queueCap": 20.0})
	b.node("db", KDatabase, Params{"connections": 32.0})
	b.edge("u", "lb").edge("lb", "a1").edge("lb", "a2").edge("a1", "db").edge("a2", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestAConfigPushChangesASettingEverywhereAndRevertsIt(t *testing.T) {
	ev := ChaosEvent{Type: "configPush", Kind: KAppServer, Key: "threads", Value: 1.0, AtSec: 10, DurationSec: 10}
	s := New(pushDesign(), Options{Seed: 5, Chaos: []ChaosEvent{ev}})
	s.Advance(15000)
	if s.nodes["a1"].f("threads") != 1 || s.nodes["a2"].f("threads") != 1 {
		t.Errorf("during the push both servers have 1 thread: %v %v", s.nodes["a1"].f("threads"), s.nodes["a2"].f("threads"))
	}
	s.Advance(10000)
	if s.nodes["a1"].f("threads") != 16 || s.nodes["a2"].f("threads") != 16 {
		t.Errorf("the old value is pushed back after 10 s: %v %v", s.nodes["a1"].f("threads"), s.nodes["a2"].f("threads"))
	}
}

func TestAConfigPushToAKindLeavesOtherKindsAlone(t *testing.T) {
	ev := ChaosEvent{Type: "configPush", Kind: KAppServer, Key: "threads", Value: 4.0, AtSec: 5}
	s := New(pushDesign(), Options{Seed: 5, Chaos: []ChaosEvent{ev}})
	s.Advance(10000)
	if s.nodes["a1"].f("threads") != 4 || s.nodes["a2"].f("threads") != 4 {
		t.Errorf("every app server gets the new value: %v %v", s.nodes["a1"].f("threads"), s.nodes["a2"].f("threads"))
	}
	if s.nodes["db"].f("connections") != 32 {
		t.Errorf("the database is untouched: %v", s.nodes["db"].f("connections"))
	}
}

func TestAPushedBadValueHurtsOnlyWhileItLasts(t *testing.T) {
	// a pool of 1 thread per server with a 40 ms request is 25 a second each: 50 of the 100 asked for
	bad := ChaosEvent{Type: "configPush", Kind: KAppServer, Key: "cpuMs", Value: 40.0, AtSec: 10, DurationSec: 10}
	rate := func(chaos []ChaosEvent) float64 {
		d := pushDesign()
		for i := range d.Nodes {
			if d.Nodes[i].Kind == KAppServer {
				d.Nodes[i].Params["threads"] = 2.0
			}
		}
		s := New(d, Options{Seed: 5, Chaos: chaos})
		s.Advance(40000)
		r := s.RunSnapshot()
		return float64(r.OK) / float64(r.OK+r.Failed)
	}
	clean, pushed := rate(nil), rate([]ChaosEvent{bad})
	t.Logf("success: %.3f without the push, %.3f with a bad cpuMs from 10 s to 20 s of 40", clean, pushed)
	if clean < 0.98 {
		t.Errorf("without the push the two servers cope: %.3f", clean)
	}
	if pushed > clean-0.05 || pushed < 0.5 {
		t.Errorf("the push should cost success only in its window: %.3f vs %.3f", pushed, clean)
	}
}
