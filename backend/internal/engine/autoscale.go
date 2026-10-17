package engine

import "math"

// Autoscaling: an app server can be a group of identical instances that grows and shrinks with load.

const scaleEvalMs = 5000 // how often the scaler looks at the load

// scaler is the autoscaling state of one app server; its zero value means "not autoscaled".
type scaler struct {
	instances  int // servers running now (0 when autoscaling is off)
	pending    int // servers still booting
	ops        int // scale-up and scale-down steps taken, for tests and thrash detection
	instArea   float64
	capArea    float64
	evBusy     float64
	evCap      float64
	nextEval   float64
	hist       [][2]float64 // (busy, capacity) thread-milliseconds of the last few evaluation windows
	belowSince float64
	below      bool
}

func (s *Sim) autoscaled(n *node) bool { return n.kind == KAppServer && n.b("autoscale") }

// instancesOf is how many servers a node stands for (1 unless it is autoscaled).
func (s *Sim) instancesOf(n *node) int {
	if s.autoscaled(n) && n.as.instances > 1 {
		return n.as.instances
	}
	return 1
}

// scaledInstances is the instance count to report: only for autoscaled nodes.
func (s *Sim) scaledInstances(n *node) int {
	if s.autoscaled(n) {
		return s.instancesOf(n)
	}
	return 0
}

// CostPerHour is the cost rate to score: the diagram's cost with each autoscaled server counted at the average
// number of instances it actually ran, so a fleet that scaled up for a minute is not billed as if it always had.
func (s *Sim) CostPerHour() float64 {
	var c float64
	for _, n := range s.order {
		if n.removed {
			continue
		}
		c += Spec(n.kind).cost(n.p) * s.averageInstances(n)
	}
	return c
}

func (s *Sim) averageInstances(n *node) float64 {
	if s.autoscaled(n) && n.as.instArea > 0 && s.now > 0 {
		return math.Max(1, n.as.instArea/s.now)
	}
	return 1
}

// autoscale runs on every tick for each node. Cheap when off: one flag check.
func (s *Sim) autoscale(n *node) {
	if !s.autoscaled(n) {
		n.as = scaler{}
		return
	}
	a := &n.as
	lo := int(math.Max(1, n.f("minInstances")))
	hi := int(math.Max(float64(lo), n.f("maxInstances")))
	if a.instances < lo {
		a.instances = lo
		s.startNext(n)
	}
	if a.instances > hi {
		a.instances = hi
	}
	a.capArea += float64(s.capacity(n)) * tickMs
	a.instArea += float64(a.instances) * tickMs
	if a.nextEval == 0 {
		a.nextEval = s.now + scaleEvalMs
		a.evBusy, a.evCap = n.busyArea, a.capArea
		return
	}
	if s.now < a.nextEval {
		return
	}
	a.nextEval += scaleEvalMs
	a.hist = append(a.hist, [2]float64{n.busyArea - a.evBusy, a.capArea - a.evCap})
	if len(a.hist) > 6 {
		a.hist = a.hist[1:]
	}
	a.evBusy, a.evCap = n.busyArea, a.capArea
	var busy, capa float64
	for _, h := range a.hist {
		busy += h[0]
		capa += h[1]
	}
	util := 0.0
	if capa > 0 {
		util = busy / capa
	}
	target := math.Min(0.95, math.Max(0.2, n.f("targetUtilPct")/100))
	desired := int(math.Min(float64(hi), math.Max(float64(lo), math.Ceil(float64(a.instances)*util/target))))
	if total := a.instances + a.pending; desired > total {
		a.below = false
		add := desired - total
		a.pending += add
		for k := 0; k < add; k++ {
			s.logEvent("scaleUp", n, 0)
			s.after(n.f("scaleUpDelaySec")*1000, func() {
				a.pending--
				if s.autoscaled(n) && !n.removed && a.instances < hi {
					a.instances++
					a.ops++
					s.logEvent("serverUp", n, 0)
					a.hist = nil   // load measured on the old fleet size says little about the new one
					s.startNext(n) // queued work can start on the new threads
				}
			})
		}
		return
	}
	if desired < a.instances && a.pending == 0 {
		if !a.below {
			a.below, a.belowSince = true, s.now
		}
		if s.now-a.belowSince >= n.f("scaleDownDelaySec")*1000 && a.instances > lo {
			a.instances--
			a.ops++
			s.logEvent("scaleDown", n, 0)
			a.hist = nil
			a.belowSince = s.now
		}
		return
	}
	a.below = false
}
