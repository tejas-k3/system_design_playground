package engine

import "math"

// Clients: traffic generation, request attempts, timeouts and retries.

func (s *Sim) rate(n *node, tMs float64) float64 {
	t := tMs / 1000
	if s.opts.Traffic != nil {
		pts := s.opts.Traffic[n.id]
		if len(pts) == 0 {
			return 0
		}
		if t <= pts[0][0] {
			return pts[0][1]
		}
		for i := 0; i < len(pts)-1; i++ {
			a, b := pts[i], pts[i+1]
			if t < b[0] {
				if b[0] > a[0] {
					return a[1] + (b[1]-a[1])*(t-a[0])/(b[0]-a[0])
				}
				return b[1]
			}
		}
		return pts[len(pts)-1][1]
	}
	base, peak := n.f("rps"), n.f("peakRps")
	period := math.Max(0.001, n.f("periodSec"))
	switch n.s("pattern") {
	case "ramp":
		if t >= period {
			return peak
		}
		return base + (peak-base)*t/period
	case "spike":
		m := math.Mod(t, period)
		if m >= period/2 && m < period*0.75 {
			return peak
		}
		return base
	case "wave":
		return base + (peak-base)*(1-math.Cos(2*math.Pi*t/period))/2
	}
	return base
}

func (s *Sim) trafficStopped() bool {
	return s.opts.StopTrafficAtMs > 0 && s.now >= s.opts.StopTrafficAtMs
}

func (s *Sim) startClient(n *node) {
	n.arrivalGen++
	s.nextArrival(n, n.arrivalGen)
}

func (s *Sim) nextArrival(n *node, gen int) {
	if gen != n.arrivalGen || n.removed || n.down || s.trafficStopped() {
		return
	}
	r := s.rate(n, s.now)
	if r <= 0 {
		s.after(100, func() { s.nextArrival(n, gen) })
		return
	}
	s.after(s.rng.Exp()/r*1000, func() {
		if gen != n.arrivalGen || n.removed || n.down || s.trafficStopped() {
			return
		}
		s.createLogical(n)
		s.nextArrival(n, gen)
	})
}

type logical struct {
	id        int
	client    *node
	write     bool
	createdAt float64
	attempts  int
	resolved  bool
	counted   bool
	key       int
	wl        int
	hash      uint32
	tr        *tracer
}

type attempt struct {
	lg    *logical
	at    float64
	n     int
	done  bool
	ghost bool
}

const (
	wlKV = iota
	wlGeo
	wlLeader
	wlBooking
)

func workloadOf(n *node) int {
	switch n.s("workload") {
	case "geo":
		return wlGeo
	case "leaderboard":
		return wlLeader
	case "booking":
		return wlBooking
	}
	return wlKV
}

func (s *Sim) createLogical(c *node) {
	s.logicalSeq++
	lg := &logical{
		id: s.logicalSeq, client: c, createdAt: s.now,
		write:   s.rng.Next() >= c.f("readPct")/100,
		counted: s.now >= s.opts.WarmupMs,
		hash:    Hash32(uint32(s.logicalSeq)),
		wl:      workloadOf(c),
	}
	lg.key = c.keys.Sample(c.s("keyDist"), int(c.f("keySpace")), c.f("zipfS"), c.f("hotKeyPct"), s.keyRng)
	s.unresolved++
	if lg.counted {
		s.runPending++
	}
	s.created.Push(s.now, true, 0)
	lg.tr = s.startTrace(lg)
	s.startAttempt(lg)
}

func (s *Sim) startAttempt(lg *logical) {
	if lg.resolved {
		return
	}
	c := lg.client
	e := s.pickCyclic(c, "out", c.out, nil)
	if e == nil {
		s.resolve(lg, Result{Reason: RNoRoute})
		return
	}
	lg.attempts++
	s.attempts.Push(s.now, true, 0)
	at := &attempt{lg: lg, at: s.now, n: lg.attempts}
	call := &call{lg: lg, at: at, write: lg.write, key: lg.key, wl: lg.wl, hash: lg.hash, rid: lg.id, counted: lg.counted, ver: -1, tr: lg.tr}
	s.hop(c, e, call, func(res Result) {
		if at.done {
			return
		}
		at.done = true
		if res.OK {
			s.resolve(lg, res)
		} else {
			s.attemptFailed(at, res.Reason)
		}
	})
	s.after(c.f("timeoutMs"), func() {
		if !at.done {
			at.done, at.ghost = true, true
			s.attemptFailed(at, RTimeout)
		}
	})
}

func (s *Sim) attemptFailed(at *attempt, r Reason) {
	lg := at.lg
	if lg.resolved {
		return
	}
	cp := lg.client.p
	if lg.attempts < 1+int(pf(cp, "retries", 0)) {
		delay := 0.0
		if v, _ := cp["backoff"].(string); v != "none" {
			delay = 100 * math.Pow(2, float64(lg.attempts-1)) * (0.5 + s.rng.Next())
		}
		s.after(delay, func() { s.startAttempt(lg) })
		return
	}
	s.resolve(lg, Result{Reason: r})
}

func (s *Sim) resolve(lg *logical, res Result) {
	if lg.resolved {
		return
	}
	lg.resolved = true
	s.unresolved--
	if lg.tr != nil {
		s.finishTrace(lg, res)
	}
	lat := s.now - lg.createdAt
	s.res.Push(s.now, res.OK, lat)
	cl := lg.client
	if res.OK {
		cl.c.ok++
	} else {
		cl.c.err++
		cl.c.reason[res.Reason]++
	}
	if lg.counted {
		s.runPending--
		s.runResolved++
		if res.OK {
			s.runOK++
			s.hist.Add(lat)
			s.countFreshness(lg, res)
		} else {
			s.runFailed++
			s.failHist.Add(lat)
			s.runErrors[res.Reason]++
		}
	}
}
