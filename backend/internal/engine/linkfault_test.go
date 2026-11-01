package engine

import (
	"math"
	"reflect"
	"testing"
)

func faultyLink(lossPct, jitterMs float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 30.0, "readPct": 100.0, "timeoutMs": 30000.0, "retries": 0.0}).node("a", KAppServer, Params{"threads": 64.0, "queueCap": 1000.0, "callTimeoutMs": 20000.0}).node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "db")
	for i := range b.d.Edges {
		if b.d.Edges[i].ID == "a>db" {
			b.d.Edges[i].LossPct = lossPct
			b.d.Edges[i].JitterMs = jitterMs
		}
	}
	d, _ := Sanitize(b.d)
	return d
}

func meanOf(d Diagram, seed uint32) float64 {
	return run(d, Options{Seed: seed}, 120).Snapshot().Global.Run.Mean
}

func TestLossAddsRetransmitDelayOfRoughlyTwoHundredMsPerLostPacket(t *testing.T) {
	base := meanOf(faultyLink(0, 0), 5)
	lossy := meanOf(faultyLink(20, 0), 5)
	// each direction loses a packet with p = 0.2 and pays 200 ms per loss: 2 x 200 x p/(1-p) = 100 ms per round trip
	want := 2 * 200 * 0.2 / 0.8
	if got := lossy - base; math.Abs(got-want) > 15 {
		t.Fatalf("20%% loss should add about %.0f ms on average, added %.1f", want, got)
	}
}

func TestJitterAddsItsAverageAndSpreadsTheTail(t *testing.T) {
	plain := run(faultyLink(0, 0), Options{Seed: 5}, 120).Snapshot().Global.Run
	jit := run(faultyLink(0, 60), Options{Seed: 5}, 120).Snapshot().Global.Run
	if got := jit.Mean - plain.Mean; math.Abs(got-60) > 6 { // two one-way trips, each + U(0,60): mean 30 each
		t.Fatalf("60 ms of jitter adds 60 ms on average over a round trip, added %.1f", got)
	}
	if jit.P99-jit.P50 < plain.P99-plain.P50+10 {
		t.Fatalf("jitter should widen the spread: plain %.0f..%.0f jitter %.0f..%.0f", plain.P50, plain.P99, jit.P50, jit.P99)
	}
}

func TestFiveLossesInARowAbortTheConnection(t *testing.T) {
	r := run(faultyLink(60, 0), Options{Seed: 5}, 60).Snapshot().Global.Run
	// P(6 losses in a row) = 0.6^6 = 4.7% per direction: roughly 9% of round trips fail as 'connLost'
	share := float64(r.Errors["connLost"]) / float64(r.Resolved)
	if share < 0.05 || share > 0.14 {
		t.Fatalf("about 9%% of round trips should abort at 60%% loss, got %.1f%% (%v)", share*100, r.Errors)
	}
	if low := run(faultyLink(1, 0), Options{Seed: 5}, 60).Snapshot().Global.Run; low.Errors["connLost"] != 0 {
		t.Fatalf("1%% loss never loses six in a row in this run: %v", low.Errors)
	}
}

func TestNoFaultsChangeNothing(t *testing.T) {
	a := run(faultyLink(0, 0), Options{Seed: 9}, 30).RunSnapshot()
	plain := &builder{}
	plain.node("u", KClient, Params{"rps": 30.0, "readPct": 100.0, "timeoutMs": 30000.0, "retries": 0.0}).node("a", KAppServer, Params{"threads": 64.0, "queueCap": 1000.0, "callTimeoutMs": 20000.0}).node("db", KDatabase, nil)
	plain.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(plain.d)
	if b := run(d, Options{Seed: 9}, 30).RunSnapshot(); !reflect.DeepEqual(a, b) {
		t.Fatal("loss 0 and jitter 0 must be identical to a link that never mentions them")
	}
	x := run(faultyLink(10, 20), Options{Seed: 9}, 30).RunSnapshot()
	y := run(faultyLink(10, 20), Options{Seed: 9}, 30).RunSnapshot()
	if !reflect.DeepEqual(x, y) {
		t.Fatal("same seed, same faults")
	}
}

func TestLinkFaultSettingsAreSanitized(t *testing.T) {
	d := faultyLink(10, 20)
	d.Edges[1].LossPct, d.Edges[1].JitterMs = -3, 99999
	out, adjusted := Sanitize(d)
	if out.Edges[1].LossPct != 0 || out.Edges[1].JitterMs != maxJitterMs || !adjusted {
		t.Fatalf("negative loss dropped, huge jitter clamped to %v: %+v adjusted=%v", maxJitterMs, out.Edges[1], adjusted)
	}
	d.Edges[1].LossPct = 250
	if out, _ := Sanitize(d); out.Edges[1].LossPct != 100 {
		t.Fatalf("loss clamps at 100%%, got %v", out.Edges[1].LossPct)
	}
}
