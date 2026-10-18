package engine

import (
	"reflect"
	"testing"
)

func burstDesign(baseline, credits float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 150.0, "readPct": 100.0, "timeoutMs": 3000.0}).
		node("a", KAppServer, Params{"threads": 8.0, "cpuMs": 20.0, "burstBaselinePct": baseline, "burstCreditSec": credits}).
		node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestBurstableServerIsFastUntilCreditsRunOut(t *testing.T) {
	s := run(burstDesign(15, 3), Options{Seed: 5}, 60)
	var early, late float64
	var en, ln int
	for _, p := range s.Snapshot().Series {
		if p.T > 1 && p.T < 6 {
			early += p.OkRps
			en++
		}
		if p.T > 40 {
			late += p.OkRps
			ln++
		}
	}
	early, late = early/float64(en), late/float64(ln)
	if early < 130 {
		t.Fatalf("with credits the server should carry the full load, got %.0f rps", early)
	}
	if late > 100 {
		t.Fatalf("with credits gone the server is held to its baseline and should serve far less, got %.0f rps", late)
	}
}

func TestBurstOffChangesNothing(t *testing.T) {
	a := run(burstDesign(100, 10), Options{Seed: 9}, 20).RunSnapshot()
	plain := &builder{}
	plain.node("u", KClient, Params{"rps": 150.0, "readPct": 100.0, "timeoutMs": 3000.0}).node("a", KAppServer, Params{"threads": 8.0, "cpuMs": 20.0}).node("db", KDatabase, nil).edge("u", "a").edge("a", "db")
	d0, _ := Sanitize(plain.d)
	b := run(d0, Options{Seed: 9}, 20).RunSnapshot()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("a baseline of 100% means no throttling and must equal a design that never mentions it")
	}
}

func TestBurstRecoversWhenLoadDrops(t *testing.T) {
	// a light load below the baseline earns credits back, so it is never throttled
	d := burstDesign(60, 5)
	for i := range d.Nodes {
		if d.Nodes[i].Kind == KClient {
			d.Nodes[i].Params["rps"] = 60.0
		}
	}
	r := run(d, Options{Seed: 5}, 60).Snapshot().Global.Run
	if float64(r.OK) < 0.99*float64(r.Resolved) {
		t.Fatalf("load under the baseline never exhausts credits, ok %d of %d", r.OK, r.Resolved)
	}
}
