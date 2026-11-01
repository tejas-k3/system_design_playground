package engine

import "math"

// Adaptive concurrency limit and database contention.
//
// Contention (database setting `contention`, 0 = off): the time of a query grows with the number of queries running at
// once beyond a knee. A query that starts while `running` queries hold connections (itself included) takes
//
//	service = base x (1 + contention x max(0, running - knee) / knee)
//
// where knee is `contentionKnee`. With k queries running each takes base x (1 + c(k-n)/n), so throughput k / service
// peaks at k = n and, for c > 1, falls towards n / (c x base) as k grows: the classic collapse. The factor is sampled
// when the query starts (it is not revised while it runs).
//
// Adaptive limit (app server setting `concurrencyLimit: adaptive`): AIMD on observed latency. The latency of a request
// is the time it held a thread (its CPU plus every dependency call, not the wait in the queue). Requests are admitted
// only while fewer than `limit` are running; the rest wait in the queue (queueCap, queueTimeoutMs, queueOrder apply as
// usual). Once per window of adaptWindowMs (and at least adaptMinSamples requests):
//
//	base = the lowest window mean seen so far (the no-load latency), creeping up adaptDrift per window so that a
//	       dependency that is permanently slower is learned instead of throttled for ever
//	mean > adaptHigh x base                  -> limit = max(1, limit x adaptDecrease)   (multiplicative decrease)
//	mean <= adaptLow x base and the limit was reached in the window
//	                                         -> limit x 2 while still in slow start, else limit + 1 (additive increase)
//	otherwise                                -> hold
//
// The limit never exceeds the server's threads (x instances). Slow start ends at the first decrease. The default
// (fixed) keeps no state and consumes no random draw; adaptive does not draw either.
const (
	adaptStart      = 4.0   // starting limit
	adaptWindowMs   = 250.0 // length of a measurement window
	adaptMinSamples = 5     // requests needed before a window counts
	adaptLow        = 1.1   // increase while mean latency is within 10% of the no-load latency
	adaptHigh       = 1.3   // decrease when it is above 130% of it
	adaptDecrease   = 0.9
	adaptDrift      = 0.0005 // per window, about 0.2% a second
)

// adaptiveLimit is the state of one adaptive app server; nil on every other node.
type adaptiveLimit struct {
	limit     float64
	slow      bool
	winStart  float64
	sum       float64
	cnt       int
	peak      int
	base      float64
	decreases int
}

// adaptiveSync creates or drops the controller to match the node's setting (called when a node is added and every tick,
// so live edits take effect).
func (s *Sim) adaptiveSync(n *node) {
	if n.kind != KAppServer {
		return
	}
	want := n.s("concurrencyLimit") == "adaptive"
	switch {
	case want && n.ad == nil:
		n.ad = &adaptiveLimit{limit: adaptStart, slow: true, winStart: s.now}
	case !want && n.ad != nil:
		n.ad = nil
		s.startNext(n)
	}
}

// hasRoom is whether n may start one more request now.
func (s *Sim) hasRoom(n *node) bool {
	if n.busy >= s.capacity(n) {
		return false
	}
	return n.ad == nil || float64(n.busy) < n.ad.limit
}

// adaptiveReleaser is the releaser of an adaptive server: it also feeds the request's thread-holding time to the controller.
func (s *Sim) adaptiveReleaser(n *node) func() {
	start := s.now
	done := false
	return func() {
		if done {
			return
		}
		done = true
		s.markBusy(n)
		busy := n.busy
		n.busy--
		if a := n.ad; a != nil {
			s.adaptiveSample(n, a, s.now-start, busy)
		}
		s.startNext(n)
	}
}

func (s *Sim) adaptiveSample(n *node, a *adaptiveLimit, rtt float64, busy int) {
	a.sum += rtt
	a.cnt++
	if busy > a.peak {
		a.peak = busy
	}
	if s.now-a.winStart < adaptWindowMs || a.cnt < adaptMinSamples {
		return
	}
	mean := a.sum / float64(a.cnt)
	if a.base == 0 || mean < a.base {
		a.base = mean
	} else {
		a.base *= 1 + adaptDrift
	}
	maxLimit := float64(s.capacity(n))
	switch {
	case mean > adaptHigh*a.base:
		a.limit = max(1, a.limit*adaptDecrease)
		a.slow = false
		a.decreases++
	case mean <= adaptLow*a.base && float64(a.peak) >= math.Floor(a.limit):
		if a.slow {
			a.limit *= 2
		} else {
			a.limit++
		}
	}
	a.limit = min(a.limit, maxLimit)
	a.winStart, a.sum, a.cnt, a.peak = s.now, 0, 0, 0
}

// adaptiveExtra adds the node's current limit to its stats.
func (s *Sim) adaptiveExtra(n *node, ex map[string]float64) {
	if n.ad != nil {
		ex["concurrencyLimit"] = math.Round(n.ad.limit*10) / 10
	}
}

// contentionMult is the factor a query that starts now on database n takes longer by (1 when contention is off).
func contentionMult(n *node) float64 {
	c := n.f("contention")
	if c <= 0 {
		return 1
	}
	knee := n.f("contentionKnee")
	if over := float64(n.busy) - knee; over > 0 && knee > 0 {
		return 1 + c*over/knee
	}
	return 1
}
