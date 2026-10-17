package engine

import (
	"math"
	"testing"
)

// squared coefficient of variation of each mode (steady is the original model: 0.5 + 0.5*Exp)
var wantSCV = map[string]float64{"steady": 0.25, "exponential": 1, "heavy": 4}

func TestServiceVariabilityModes(t *testing.T) {
	for mode, want := range wantSCV {
		r := NewRNG(11)
		const n = 2000000
		var sum, sumSq float64
		for i := 0; i < n; i++ {
			v := r.ServiceDist(10, mode)
			sum += v
			sumSq += v * v
		}
		mean := sum / n
		scv := (sumSq/n - mean*mean) / (mean * mean)
		if math.Abs(mean-10) > 0.1 {
			t.Errorf("%s: mean %.3f, want 10 (the setting changes the spread, never the average)", mode, mean)
		}
		if math.Abs(scv-want)/want > 0.12 {
			t.Errorf("%s: SCV %.3f, want %.2f", mode, scv, want)
		}
	}
}

func TestDefaultVariabilityIsTheOriginalModel(t *testing.T) {
	r1, r2 := NewRNG(5), NewRNG(5)
	for i := 0; i < 1000; i++ {
		if a, b := r1.Service(7), r2.ServiceDist(7, ""); a != b {
			t.Fatalf("draw %d: Service %v vs ServiceDist(\"\") %v", i, a, b)
		}
	}
}

func waitWithVariability(mode string, rps float64, seed uint32) float64 {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "readPct": 100.0, "timeoutMs": 60000.0, "keySpace": wantKeySpan})
	b.node("a", KAppServer, Params{"threads": 1.0, "cpuMs": cpuMs, "queueCap": 2000.0, "variability": mode}).
		node("db", KDatabase, Params{"readMs": 1.0, "connections": 256.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: seed, WarmupMs: warmupMs})
	s.Advance(600 * 1000)
	return s.Snapshot().Global.Run.Mean - clientRTT - meanHold
}

// More variable service times queue longer at the same load: the Pollaczek-Khinchine mean wait grows with
// the service-time variance, and the engine follows it for each mode (single server, 70% utilisation).
func TestVariabilityFollowsPollaczekKhinchine(t *testing.T) {
	const rps = 22.58 // 70% of 1 / 31 ms
	lam := rps / 1000
	rho := lam * meanHold
	var prev float64
	for _, mode := range []string{"steady", "exponential", "heavy"} {
		varHold := wantSCV[mode]*cpuMs*cpuMs + 0.25 // the 1 ms database read stays steady
		want := lam * (varHold + meanHold*meanHold) / (2 * (1 - rho))
		var got float64
		const seeds = 6
		for s := uint32(1); s <= seeds; s++ {
			got += waitWithVariability(mode, rps, s) / seeds
		}
		t.Logf("%-11s simulated wait %.1f ms, theory %.1f ms", mode, got, want)
		within(t, mode, got, want, 0.2)
		if got <= prev {
			t.Errorf("%s waits %.1f ms, not longer than the previous mode (%.1f ms)", mode, got, prev)
		}
		prev = got
	}
}
