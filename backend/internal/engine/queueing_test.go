package engine

import (
	"math"
	"testing"
)

// These tests check the engine's queueing mechanics against closed-form queueing theory, so a
// change to scheduling, queues or time accounting that distorts waiting times is caught.
//
// Setup: users -> one app server (c threads, cpuMs mean m) -> a database that answers in ~1 ms.
// A request therefore holds a thread for S = cpu + 10 ms of network to the database + db time, with
//   E[S]   = m + 10 + 1
//   Var[S] = 0.25*(m^2 + 1)        (each Service() draw has variance mean^2 / 4)
// and the client sees 80 ms of network round trip on top of queue wait and S.

const (
	cpuMs       = 20.0
	meanHold    = cpuMs + 10 + 1
	varHold     = 0.25 * (cpuMs*cpuMs + 1)
	clientRTT   = 80.0
	warmupMs    = 100000.0
	wantKeySpan = 100.0
)

func erlangC(c int, a float64) float64 { // probability an arrival has to wait in M/M/c
	sum, term := 0.0, 1.0
	for k := range c {
		sum += term
		term *= a / float64(k+1) // after the loop term = a^c / c!
	}
	top := term * float64(c) / (float64(c) - a)
	return top / (sum + top)
}

// theoryWait is the mean time spent queueing before a thread frees up: exact Pollaczek-Khinchine for
// one server, the Allen-Cunneen approximation (Erlang C scaled by service variability) for several.
func theoryWait(c int, rps float64) float64 {
	lam := rps / 1000
	if c == 1 {
		rho := lam * meanHold
		return lam * (varHold + meanHold*meanHold) / (2 * (1 - rho))
	}
	cs2 := varHold / (meanHold * meanHold)
	return erlangC(c, lam*meanHold) / (float64(c)/meanHold - lam) * (1 + cs2) / 2
}

func simulatedWait(c int, rps float64, seed uint32, seconds float64) float64 {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "readPct": 100.0, "timeoutMs": 30000.0, "keySpace": wantKeySpan})
	b.node("a", KAppServer, Params{"threads": float64(c), "cpuMs": cpuMs, "queueCap": 2000.0}).
		node("db", KDatabase, Params{"readMs": 1.0, "connections": 256.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: seed, WarmupMs: warmupMs})
	s.Advance(seconds * 1000)
	r := s.Snapshot().Global.Run
	return r.Mean - clientRTT - meanHold
}

func within(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if rel := math.Abs(got-want) / want; rel > tol {
		t.Errorf("%s: simulated wait %.1f ms vs theory %.1f ms (%.1f%% off, allowed %.0f%%)", name, got, want, rel*100, tol*100)
	}
}

func TestSingleServerQueueMatchesPollaczekKhinchine(t *testing.T) {
	for _, tc := range []struct {
		name string
		rps  float64 // utilisation = rps/1000 * 31 ms
	}{{"rho 0.50", 16}, {"rho 0.81", 26}} {
		within(t, tc.name, simulatedWait(1, tc.rps, 11, 2000), theoryWait(1, tc.rps), 0.06)
	}
}

func TestSingleServerAtHighLoadMatchesOnAverage(t *testing.T) {
	// near saturation waits are strongly autocorrelated, so one seed can be 10% off; the mean over seeds is not
	const seeds = 6
	var sum float64
	for seed := uint32(1); seed <= seeds; seed++ {
		sum += simulatedWait(1, 29, seed, 3000)
	}
	within(t, "rho 0.90 mean of 6 seeds", sum/seeds, theoryWait(1, 29), 0.08)
}

func TestMultiServerQueueMatchesAllenCunneen(t *testing.T) {
	for _, tc := range []struct {
		name string
		rps  float64
	}{{"4 threads rho 0.80", 103}, {"4 threads rho 0.90", 116}} {
		within(t, tc.name, simulatedWait(4, tc.rps, 11, 1500), theoryWait(4, tc.rps), 0.08)
	}
}

// Documents the modelling assumption that matters most for realism: every service time is
// mean*(0.5 + 0.5*Exp(1)), which is always at least half the mean and has a squared coefficient of
// variation of 0.25. Real services are usually far more variable (SCV of 1 or well above).
func TestServiceTimesAreLowVariance(t *testing.T) {
	r := NewRNG(3)
	const n = 400000
	var sum, sumSq, lowest float64
	lowest = math.MaxFloat64
	for range n {
		v := r.Service(10)
		sum += v
		sumSq += v * v
		lowest = min(lowest, v)
	}
	mean := sum / n
	scv := (sumSq/n - mean*mean) / (mean * mean)
	if math.Abs(mean-10) > 0.05 || math.Abs(scv-0.25) > 0.01 || lowest < 5 {
		t.Fatalf("service times changed: mean %.3f, SCV %.3f, min %.3f (was 10, 0.25, >=5)", mean, scv, lowest)
	}
}
