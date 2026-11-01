package engine

import (
	"math"
	"testing"
)

// one app server that can do about 20 requests a second (2 threads, ~100 ms each), a deep queue, and a client we vary
func slowServer(client Params) *Sim {
	b := &builder{}
	b.node("u", KClient, client)
	b.node("a", KAppServer, Params{"threads": 2.0, "cpuMs": 100.0, "queueCap": 1000.0})
	b.node("db", KDatabase, Params{"readMs": 1.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return New(d, Options{Seed: 4})
}

func TestAClosedCrowdSlowsDownInsteadOfDrowningTheServer(t *testing.T) {
	// 100 users who think for 1 s between requests would offer ~100 rps if they never waited: five times what the server can do
	closed := slowServer(Params{"arrival": "closed", "users": 100.0, "thinkMs": 1000.0, "readPct": 100.0, "timeoutMs": 30000.0, "retries": 0.0})
	closed.Advance(120000)
	c := closed.RunSnapshot()
	if rate := float64(c.OK) / float64(c.Resolved); rate < 0.99 {
		t.Errorf("a closed crowd waits for answers, so nothing is rejected: success %.3f (%+v)", rate, c.Errors)
	}
	thr := float64(c.Resolved) / 120
	if thr < 15 || thr > 23 {
		t.Errorf("throughput should settle at what the server can do (about 20/s), got %.1f/s", thr)
	}
	// Little's law for a closed system: users = throughput x (think time + response time)
	response := c.Mean / 1000
	if want := 100/thr - 1; math.Abs(response-want)/want > 0.15 {
		t.Errorf("mean response %.2f s, Little's law says %.2f s", response, want)
	}
	if closed.unresolved > 100 {
		t.Errorf("at most one request per user in flight, got %d", closed.unresolved)
	}

	open := slowServer(Params{"rps": 100.0, "readPct": 100.0, "timeoutMs": 2000.0, "retries": 0.0})
	open.Advance(120000)
	o := open.RunSnapshot()
	if rate := float64(o.OK) / float64(o.Resolved); rate > 0.5 {
		t.Errorf("an open crowd at the same nominal rate is drowned: success %.3f", rate)
	}
}

func TestAClosedCrowdBelowCapacityIsLimitedByThinkTime(t *testing.T) {
	s := slowServer(Params{"arrival": "closed", "users": 10.0, "thinkMs": 1000.0, "readPct": 100.0, "timeoutMs": 30000.0})
	s.Advance(200000)
	c := s.RunSnapshot()
	thr := float64(c.Resolved) / 200
	want := 10 / (1 + c.Mean/1000)
	if math.Abs(thr-want)/want > 0.1 {
		t.Errorf("throughput %.2f/s, users / (think + response) = %.2f/s", thr, want)
	}
}

// arrivals per second over a run
func arrivalCounts(s *Sim, seconds int) []float64 {
	out := make([]float64, 0, seconds)
	prev := s.logicalSeq
	for range seconds {
		s.Advance(1000)
		out = append(out, float64(s.logicalSeq-prev))
		prev = s.logicalSeq
	}
	return out
}

func meanVar(x []float64) (float64, float64) {
	var m, v float64
	for _, a := range x {
		m += a
	}
	m /= float64(len(x))
	for _, a := range x {
		v += (a - m) * (a - m)
	}
	return m, v / float64(len(x))
}

func TestBurstyTrafficKeepsTheMeanButNotThePace(t *testing.T) {
	poisson := arrivalCounts(slowServer(Params{"rps": 15.0, "readPct": 100.0}), 3000)
	bursty := arrivalCounts(slowServer(Params{"arrival": "bursty", "rps": 15.0, "burstFactor": 4.0, "burstSec": 2.0, "readPct": 100.0}), 3000)
	pm, pv := meanVar(poisson)
	bm, bv := meanVar(bursty)
	if math.Abs(bm-pm)/pm > 0.08 {
		t.Errorf("bursty traffic must keep the average rate: %.2f vs %.2f per second", bm, pm)
	}
	// index of dispersion (variance / mean): 1 for Poisson, far above for rushes
	if pv/pm > 1.5 {
		t.Errorf("the baseline should be Poisson, dispersion %.2f", pv/pm)
	}
	if bv/bm < 5 {
		t.Errorf("bursty arrivals should be much lumpier than Poisson: dispersion %.2f vs %.2f", bv/bm, pv/pm)
	}
}

func TestBurstsHurtTheTailAtTheSameAverageLoad(t *testing.T) {
	p95 := func(client Params) float64 {
		s := slowServer(client)
		s.Advance(300000)
		return s.RunSnapshot().P95
	}
	calm := p95(Params{"rps": 14.0, "readPct": 100.0, "timeoutMs": 30000.0})
	rush := p95(Params{"arrival": "bursty", "rps": 14.0, "burstFactor": 4.0, "burstSec": 2.0, "readPct": 100.0, "timeoutMs": 30000.0})
	if rush < 2*calm {
		t.Errorf("at 70%% average load, rushes should at least double the p95: %.0f ms vs %.0f ms", rush, calm)
	}
}

func TestClientBehavioursAreDeterministic(t *testing.T) {
	for _, c := range []Params{
		{"arrival": "closed", "users": 30.0, "thinkMs": 500.0, "readPct": 100.0},
		{"arrival": "bursty", "rps": 15.0, "readPct": 100.0},
	} {
		run := func() RunStats {
			s := slowServer(c)
			s.Advance(60000)
			return s.RunSnapshot()
		}
		a, b := run(), run()
		if a.OK != b.OK || a.Failed != b.Failed || math.Abs(a.P95-b.P95) > 1e-9 {
			t.Errorf("%v: same seed, different result: %+v vs %+v", c["arrival"], a, b)
		}
	}
}
