package engine

import (
	"reflect"
	"testing"
)

func connectDesign(ms float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 80.0, "readPct": 100.0, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 32.0, "dbPool": 10.0, "connectMs": ms}).
		node("db", KDatabase, Params{"connections": 32.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func meanBetween(s *Sim, lo, hi float64) float64 {
	var sum float64
	var n int
	for _, p := range s.Snapshot().Series {
		if p.T >= lo && p.T < hi && p.OkRps > 0 {
			sum += p.P95
			n++
		}
	}
	if n == 0 {
		panic("no completions in the window") // a NaN would make every comparison below pass
	}
	return sum / float64(n)
}

func TestNewConnectionsPayASetupCostOnlyOnce(t *testing.T) {
	s := run(connectDesign(300), Options{Seed: 2}, 40)
	cold, warm := meanBetween(s, 0, 3), meanBetween(s, 20, 40)
	if cold-warm < 100 {
		t.Fatalf("the first requests open connections and pay the 300 ms handshake: cold %.0f warm %.0f", cold, warm)
	}
	base := run(connectDesign(0), Options{Seed: 2}, 40)
	if w := meanBetween(base, 20, 40); warm > w*1.1 {
		t.Fatalf("a reused connection pays nothing: warm %.0f against %.0f without setup cost", warm, w)
	}
}

func TestARestartedServerHasColdPoolsAgain(t *testing.T) {
	o := Options{Seed: 2, Chaos: []ChaosEvent{{Type: "crash", NodeID: "a", AtSec: 20, DownSec: 2}}}
	s := run(connectDesign(300), o, 40)
	if after, before := meanBetween(s, 23, 25), meanBetween(s, 10, 19); after-before < 100 {
		t.Fatalf("after a restart the pool is empty and handshakes are paid again: %.0f against %.0f", after, before)
	}
}

func TestConnectMsZeroChangesNothing(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 80.0, "readPct": 100.0, "timeoutMs": 5000.0}).node("a", KAppServer, Params{"threads": 32.0, "dbPool": 10.0}).node("db", KDatabase, Params{"connections": 32.0}).edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	if !reflect.DeepEqual(run(connectDesign(0), Options{Seed: 2}, 20).RunSnapshot(), run(d, Options{Seed: 2}, 20).RunSnapshot()) {
		t.Fatal("connectMs 0 must be identical to a design that never mentions it")
	}
}
