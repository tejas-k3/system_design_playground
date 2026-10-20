package engine

import (
	"reflect"
	"testing"
)

// 400 requests a second at a server that can do about 250, 20% of them urgent, clients give up after 2 s.
func shedDesign(shedMs float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 400.0, "readPct": 100.0, "timeoutMs": 2000.0, "priorityPct": 20.0}).
		node("a", KAppServer, Params{"threads": 8.0, "queueCap": 2000.0, "shedAboveQueueMs": shedMs}).
		node("db", KDatabase, Params{"connections": 32.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestWithoutSheddingUrgentRequestsDrownWithTheRest(t *testing.T) {
	r := run(shedDesign(0), Options{Seed: 6}, 40).RunSnapshot()
	t.Logf("no shedding: urgent ok %d of %d, all ok %d of %d", r.HighOK, r.HighTotal, r.OK, r.OK+r.Failed)
	if r.HighTotal == 0 || float64(r.HighOK) > 0.5*float64(r.HighTotal) {
		t.Fatalf("an overloaded FIFO queue treats every request alike: urgent ok %d of %d", r.HighOK, r.HighTotal)
	}
}

func TestSheddingOrdinaryRequestsSavesTheUrgentOnes(t *testing.T) {
	r := run(shedDesign(300), Options{Seed: 6}, 40).RunSnapshot()
	t.Logf("shedding above 300 ms: urgent ok %d of %d, shed %d, all ok %d of %d", r.HighOK, r.HighTotal, r.Errors["shed"], r.OK, r.OK+r.Failed)
	if float64(r.HighOK) < 0.9*float64(r.HighTotal) {
		t.Fatalf("urgent requests are always admitted and the queue stays short: %d of %d ok", r.HighOK, r.HighTotal)
	}
	if r.Errors["shed"] == 0 {
		t.Fatal("ordinary requests are turned away at the door when the queue is long")
	}
	if r.OK < 6000 { // about 200 a second against 19 a second without shedding
		t.Fatalf("shedding keeps the server busy with live work, total goodput should be near its capacity: %d ok", r.OK)
	}
}

func TestSheddingOffChangesNothing(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 400.0, "readPct": 100.0, "timeoutMs": 2000.0, "priorityPct": 20.0}).
		node("a", KAppServer, Params{"threads": 8.0, "queueCap": 2000.0}).node("db", KDatabase, Params{"connections": 32.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	if !reflect.DeepEqual(run(d, Options{Seed: 6}, 20).RunSnapshot(), run(shedDesign(0), Options{Seed: 6}, 20).RunSnapshot()) {
		t.Fatal("shedAboveQueueMs 0 must change nothing")
	}
}
