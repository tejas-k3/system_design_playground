package engine

import (
	"math"
	"reflect"
	"testing"
)

// users -> app -> db with a limited link between users and app.
func linkDesign(mbps, reqKB, respKB, rps float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "readPct": 100.0, "reqKB": reqKB, "respKB": respKB, "timeoutMs": 30000.0, "retries": 0.0}).node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0}).node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "a").edge("a", "db")
	for i := range b.d.Edges {
		if b.d.Edges[i].ID == "u>a" {
			b.d.Edges[i].BandwidthMbps = mbps
		}
	}
	d, _ := Sanitize(b.d)
	return d
}

func TestASaturatedLinkCarriesExactlyItsBandwidth(t *testing.T) {
	// 100 KB requests on an 80 Mbit/s link: 80e6 / (100,000 x 8) = 100 requests a second at most, offered 300
	s := run(linkDesign(80, 100, 0, 300), Options{Seed: 3}, 20)
	var st EdgeStats
	for range 220 { // let the window fill
		s.Advance(100)
	}
	st = s.Snapshot().Edges["u>a"]
	if math.Abs(st.Mbps-80)/80 > 0.05 {
		t.Fatalf("a saturated 80 Mbit/s link should carry 80 Mbit/s (+/-5%%), carried %.1f", st.Mbps)
	}
	if st.Util < 0.95 {
		t.Fatalf("utilisation should read about 100%%, got %.2f", st.Util)
	}
	// the edge's rps is what is offered to the link (300); what it carries follows from the data rate: 80e6 / 800,000 bits = 100 a second
	if carried := st.Mbps * 1e6 / (100 * 1000 * 8); carried > 105 || carried < 95 {
		t.Fatalf("a link carrying %.1f Mbit/s moves ~100 requests a second of 100 KB, computed %.0f", st.Mbps, carried)
	}
	if st.Rps < 250 {
		t.Fatalf("the edge's rps reports what is offered to it (~300), got %.0f", st.Rps)
	}
}

func TestOneLargeObjectTakesItsTransmitTime(t *testing.T) {
	base := run(linkDesign(0, 125, 0, 1), Options{Seed: 9}, 30).Snapshot().Global.Run.P50
	slow := run(linkDesign(10, 125, 0, 1), Options{Seed: 9}, 30).Snapshot().Global.Run.P50
	want := 125000.0 * 8 / 10e6 * 1000 // 100 ms on a 10 Mbit/s link
	if got := slow - base; math.Abs(got-want)/want > 0.05 {
		t.Fatalf("a 125 KB request over 10 Mbit/s adds %.0f ms, want %.0f ms (+/-5%%); base p50 %.1f, slow p50 %.1f", got, want, base, slow)
	}
}

func TestTheResponseTravelsBackOverTheSameLink(t *testing.T) {
	base := run(linkDesign(0, 0, 250, 1), Options{Seed: 9}, 30).Snapshot().Global.Run.P50
	slow := run(linkDesign(10, 0, 250, 1), Options{Seed: 9}, 30).Snapshot().Global.Run.P50
	want := 250000.0 * 8 / 10e6 * 1000 // 200 ms
	if got := slow - base; math.Abs(got-want)/want > 0.05 {
		t.Fatalf("a 250 KB response over 10 Mbit/s adds %.0f ms, want %.0f ms", got, want)
	}
}

func TestNoBandwidthOrNoSizeChangesNothing(t *testing.T) {
	plain := run(linkDesign(0, 0, 0, 120), Options{Seed: 4}, 20).RunSnapshot()
	// a link limit with no sizes set is not modelled (there are no bytes to carry)
	limited := run(linkDesign(10, 0, 0, 120), Options{Seed: 4}, 20).RunSnapshot()
	if !reflect.DeepEqual(plain, limited) {
		t.Fatalf("a bandwidth limit with sizes at 0 must change nothing:\n%+v\n%+v", plain, limited)
	}
	// sizes set but the link unlimited: no transmit time either
	a := run(linkDesign(0, 100, 100, 120), Options{Seed: 4}, 20).RunSnapshot()
	b := run(linkDesign(0, 0, 0, 120), Options{Seed: 4}, 20).RunSnapshot()
	if a.P50 != b.P50 || a.OK != b.OK {
		t.Fatalf("sizes on an unlimited link must change nothing: %+v vs %+v", a, b)
	}
}

func TestBandwidthIsSanitized(t *testing.T) {
	d := linkDesign(80, 10, 0, 10)
	d.Edges[0].BandwidthMbps = -5
	out, adjusted := Sanitize(d)
	if out.Edges[0].BandwidthMbps != 0 || !adjusted {
		t.Fatalf("a negative bandwidth is dropped to unlimited and reported: %+v adjusted=%v", out.Edges[0], adjusted)
	}
	d.Edges[0].BandwidthMbps = 1e9
	out, _ = Sanitize(d)
	if out.Edges[0].BandwidthMbps != maxBandwidthMbps {
		t.Fatalf("clamped to %v, got %v", maxBandwidthMbps, out.Edges[0].BandwidthMbps)
	}
	d.Edges[0].BandwidthMbps = 80
	if out, adj := Sanitize(d); out.Edges[0].BandwidthMbps != 80 || adj {
		t.Fatalf("a valid value survives untouched")
	}
}
