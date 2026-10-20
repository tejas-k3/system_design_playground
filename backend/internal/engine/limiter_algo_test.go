package engine

import (
	"reflect"
	"testing"
)

func limiterSim(algo string, limit, window float64) (*Sim, *node) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 1.0}).node("rl", KRateLimiter, Params{"limitRps": limit, "burst": 1000.0, "algorithm": algo, "windowSec": window}).node("a", KAppServer, nil).node("db", KDatabase, nil)
	b.edge("u", "rl").edge("rl", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 1})
	return s, s.nodes["rl"]
}

// send admits n requests at time t (ms) and reports how many got through
func send(s *Sim, rl *node, t float64, n int) int {
	s.now = t
	got := 0
	for i := 0; i < n; i++ {
		if s.admit(rl, &call{}) {
			got++
		}
	}
	return got
}

// A fixed window forgets everything at its edge: a burst just before the boundary and another just after both fit.
func TestAFixedWindowAdmitsDoubleTheLimitAcrossItsBoundary(t *testing.T) {
	s, rl := limiterSim("fixedWindow", 10, 1) // 10 per one-second window
	a := send(s, rl, 950, 10)
	b := send(s, rl, 1050, 10)
	if a != 10 || b != 10 {
		t.Fatalf("20 requests in 100 ms straddling the boundary all fit a fixed window: %d + %d", a, b)
	}
	if c := send(s, rl, 1060, 5); c != 0 {
		t.Fatalf("but the new window is then full: %d", c)
	}
}

// A sliding log counts the requests of the last full window, so the same two bursts cannot both get through.
func TestASlidingLogNeverAdmitsMoreThanTheLimitInAnyWindow(t *testing.T) {
	s, rl := limiterSim("slidingWindowLog", 10, 1)
	a := send(s, rl, 950, 10)
	b := send(s, rl, 1050, 10)
	if a != 10 || b != 0 {
		t.Fatalf("only 10 in any 1 s span: %d then %d", a, b)
	}
	if c := send(s, rl, 1960, 10); c != 10 {
		t.Fatalf("once the first burst is more than a window old the allowance is back: %d", c)
	}
}

// The counter approximation weights the previous window by how much of it still overlaps.
func TestASlidingCounterSitsBetweenFixedAndLog(t *testing.T) {
	s, rl := limiterSim("slidingWindowCounter", 10, 1)
	send(s, rl, 950, 10)
	got := send(s, rl, 1500, 10) // half of the previous window of 10 still counts: 5 left
	if got < 4 || got > 6 {
		t.Fatalf("about half the previous window still counts, so about 5 of the next 10 fit: %d", got)
	}
}

func TestTokenBucketIsTheDefaultAndChangesNothing(t *testing.T) {
	mk := func(extra Params) Diagram {
		b := &builder{}
		p := Params{"limitRps": 80.0, "burst": 5.0}
		for k, v := range extra {
			p[k] = v
		}
		b.node("u", KClient, Params{"rps": 100.0, "readPct": 100.0, "timeoutMs": 3000.0}).node("rl", KRateLimiter, p).node("a", KAppServer, Params{"threads": 64.0}).node("db", KDatabase, nil)
		b.edge("u", "rl").edge("rl", "a").edge("a", "db")
		d, _ := Sanitize(b.d)
		return d
	}
	if !reflect.DeepEqual(run(mk(nil), Options{Seed: 5}, 20).RunSnapshot(), run(mk(Params{"algorithm": "tokenBucket", "windowSec": 1.0}), Options{Seed: 5}, 20).RunSnapshot()) {
		t.Fatal("tokenBucket is the old behaviour")
	}
}
