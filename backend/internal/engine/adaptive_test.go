package engine

import (
	"math"
	"reflect"
	"testing"
)

// Contention design: one app server (1 ms of CPU, then a database read) in front of a database whose queries slow
// down once more than `knee` of them run at once: service = readMs x (1 + contention x max(0, inflight-knee)/knee).
const (
	cBase  = 20.0 // readMs
	cKnee  = 8.0
	cCont  = 3.0
	cCPU   = 1.0
	cQueue = 200.0 // queue wait limit, same everywhere
)

var (
	cNet = 2 * DefaultLatency(KAppServer, KDatabase) // the thread also waits out the network round trip to the database
)

func contendedDesign(threads float64, limit string, contention float64, rps float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "readPct": 100.0, "timeoutMs": 2000.0}).
		node("a", KAppServer, Params{"threads": threads, "cpuMs": cCPU, "queueCap": 50.0, "queueTimeoutMs": cQueue, "dbPool": 200.0, "concurrencyLimit": limit}).
		node("db", KDatabase, Params{"connections": 128.0, "readMs": cBase, "contention": contention, "contentionKnee": cKnee})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

// goodput runs the design and returns successes per second after a warm-up.
func goodput(d Diagram, seed uint32, warm, measure float64) (float64, *Sim) {
	s := New(d, Options{Seed: seed})
	s.Advance(warm * 1000)
	ok0 := s.RunSnapshot().OK
	s.Advance(measure * 1000)
	return float64(s.RunSnapshot().OK-ok0) / measure, s
}

// theory: threads T all busy; m of them are inside the database (the rest in their 1 ms of CPU or the network round trip). A query starting
// with m running takes S(m) = base x (1 + c x max(0, m-knee)/knee); m solves m = T x S/(cpu+net+S). Throughput = T/(cpu+net+S).
func contendedTheory(T float64) (tput, m float64) {
	m = T
	for range 200 {
		S := cBase * (1 + cCont*max(0, m-cKnee)/cKnee)
		m = T * S / (cCPU + cNet + S)
	}
	S := cBase * (1 + cCont*max(0, m-cKnee)/cKnee)
	return 1000 * T / (cCPU + cNet + S), m
}

func TestThroughputPeaksAtTheKneeAndCollapsesBeyondIt(t *testing.T) {
	var last float64
	for _, T := range []float64{4, 8, 16, 32, 64} {
		got, _ := goodput(contendedDesign(T, "fixed", cCont, 3000), 3, 5, 20)
		want, m := contendedTheory(T)
		t.Logf("threads %2.0f: measured %.0f/s, theory %.0f/s (%.1f queries inside the database on average)", T, got, want, m)
		if math.Abs(got-want) > 0.1*want {
			t.Errorf("threads %.0f: measured %.0f/s, theory %.0f/s", T, got, want)
		}
		if T > 8 && got >= last {
			t.Errorf("more concurrency than the knee must lower throughput: %.0f threads gave %.0f after %.0f", T, got, last)
		}
		last = got
	}
	peak, at := bestFixed(3)
	t.Logf("ideal peak (sharp knee) knee/base = %.0f/s; best measured fixed %.0f/s at %.0f threads", 1000*cKnee/cBase, peak, at)
	if last > 0.65*peak {
		t.Fatalf("64 threads should collapse to well under the peak: %.0f vs peak %.0f", last, peak)
	}
}

func TestContentionOffMeansNoCollapse(t *testing.T) {
	a, _ := goodput(contendedDesign(8, "fixed", 0, 3000), 3, 5, 20)
	b, _ := goodput(contendedDesign(64, "fixed", 0, 3000), 3, 5, 20)
	t.Logf("contention 0: 8 threads %.0f/s, 64 threads %.0f/s", a, b)
	if b < a {
		t.Fatalf("without contention more concurrency never hurts: %.0f vs %.0f", b, a)
	}
}

func TestAdaptiveLimitKeepsGoodputNearThePeak(t *testing.T) {
	for _, seed := range []uint32{1, 2, 3} {
		peak, at := bestFixed(seed)
		fixed, _ := goodput(contendedDesign(64, "fixed", cCont, 3000), seed, 5, 40)
		ad, s := goodput(contendedDesign(64, "adaptive", cCont, 3000), seed, 15, 40)
		lim := s.Snapshot().Nodes["a"].Extra["concurrencyLimit"]
		t.Logf("seed %d: peak %.0f/s (best fixed, %.0f threads), fixed 64 threads %.0f/s (%.0f%%), adaptive %.0f/s (%.0f%% of peak), limit now %.1f", seed, peak, at, fixed, 100*fixed/peak, ad, 100*ad/peak, lim)
		if fixed > 0.65*peak {
			t.Errorf("seed %d: the fixed limit far above the knee should collapse: %.0f vs peak %.0f", seed, fixed, peak)
		}
		if ad < 0.85*peak {
			t.Errorf("seed %d: adaptive goodput %.0f is more than 15%% below the peak %.0f", seed, ad, peak)
		}
	}
}

// bestFixed is the highest goodput over fixed thread counts around the knee: the measured peak (the knee is blurred a
// little by the spread of service times, so it sits under the ideal knee/base).
func bestFixed(seed uint32) (best, at float64) {
	for _, T := range []float64{8, 9, 10, 11, 12} {
		if g, _ := goodput(contendedDesign(T, "fixed", cCont, 3000), seed, 5, 40); g > best {
			best, at = g, T
		}
	}
	return
}

// The limit climbs from its start, overshoots the knee, backs off and then stays in a band around it.
func TestAdaptiveLimitConvergesWithinABand(t *testing.T) {
	// hand calculation of the top of the band: the limit stops rising when a request's thread time passes 1.3 x its
	// no-load value (cpu + network + base). That is a database time S with cpu+net+S = 1.3 (cpu+net+base); the queries
	// inside the database are then m = knee x (1 + (S/base - 1)/c) and the threads needed to hold them are m x (cpu+net+S)/S.
	hold := cCPU + cNet
	S := 1.3*(hold+cBase) - hold
	m := cKnee * (1 + (S/cBase-1)/cCont)
	upper := m * (hold + S) / S
	t.Logf("theory: the limit tops out at %.1f (database queries inside %.2f, S %.1f ms)", upper, m, S)
	for _, seed := range []uint32{1, 2, 3} {
		s := New(contendedDesign(64, "adaptive", cCont, 3000), Options{Seed: seed})
		s.Advance(15000)
		lo, hi := math.Inf(1), 0.0
		for range 90 {
			s.Advance(1000)
			l := s.Snapshot().Nodes["a"].Extra["concurrencyLimit"]
			lo, hi = min(lo, l), max(hi, l)
		}
		t.Logf("seed %d: limit stayed between %.1f and %.1f over 90 s (knee %.0f)", seed, lo, hi, cKnee)
		if lo < cKnee || hi > upper+1 {
			t.Errorf("seed %d: limit left the band [%.1f, %.1f]: %.1f..%.1f", seed, cKnee, upper+1, lo, hi)
		}
	}
}

func TestAdaptiveLimitDoesNotThrottleAHealthySystem(t *testing.T) {
	for _, seed := range []uint32{1, 2, 3} {
		f := run(contendedDesign(32, "fixed", 0, 300), Options{Seed: seed}, 60).RunSnapshot()
		a := run(contendedDesign(32, "adaptive", 0, 300), Options{Seed: seed}, 60).RunSnapshot()
		sf, sa := 100*float64(f.OK)/float64(f.OK+f.Failed), 100*float64(a.OK)/float64(a.OK+a.Failed)
		t.Logf("seed %d: no contention, 300 rps: success fixed %.2f%%, adaptive %.2f%% (p95 %.0f vs %.0f ms)", seed, sf, sa, f.P95, a.P95)
		if math.Abs(sf-sa) > 1 {
			t.Errorf("seed %d: adaptive changed a healthy system's success by %.2f points", seed, sf-sa)
		}
	}
}

func TestConcurrencyLimitFixedEqualsDefault(t *testing.T) {
	mk := func(withSetting bool) Diagram {
		ap := Params{"threads": 16.0, "queueCap": 100.0}
		dp := Params{"connections": 8.0}
		if withSetting {
			ap["concurrencyLimit"] = "fixed"
			dp["contention"] = 0.0
			dp["contentionKnee"] = 4.0
		}
		b := &builder{}
		b.node("u", KClient, Params{"rps": 300.0, "timeoutMs": 1000.0, "retries": 1.0}).node("a", KAppServer, ap).node("db", KDatabase, dp)
		b.edge("u", "a").edge("a", "db")
		d, _ := Sanitize(b.d)
		return d
	}
	for _, seed := range []uint32{1, 2, 3} {
		a, b := run(mk(false), Options{Seed: seed}, 30).Snapshot(), run(mk(true), Options{Seed: seed}, 30).Snapshot()
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("seed %d: concurrencyLimit fixed and contention 0 must equal a design that never mentions them", seed)
		}
	}
}
