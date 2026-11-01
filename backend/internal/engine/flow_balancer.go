package engine

import (
	"slices"
	"sort"
)

func (s *Sim) believedDown(lb, t *node) bool {
	if lb.kind == KLoadBalancer && lb.s("membership") == "registry" {
		// worst case: a dead server stays listed (still receives traffic) until the entry outlives the crash by its lifetime and the read after that is as late as it can be;
		// a restarted server registers at once but is used only from the next read
		if t.down {
			return s.now-t.downAt >= (lb.f("registryTtlSec")+lb.f("registryRefreshSec"))*1000
		}
		// back up: kept out until the next read, but only if the entry had expired (it was dropped) before the restart
		listed := (lb.f("registryTtlSec") + lb.f("registryRefreshSec")) * 1000
		return t.upAt > 0 && t.upAt-t.downAt >= listed && s.now-t.upAt < lb.f("registryRefreshSec")*1000
	}
	if !t.down {
		return false
	}
	wait := 0.0
	if lb.kind == KLoadBalancer {
		wait = lb.f("healthCheckSec") * 1000
	}
	return s.now-t.downAt >= wait
}

func (s *Sim) pickBackend(n *node) *edge { return s.pickBackendFor(n, 0) }

// pickBackendFor chooses a backend for a request from this user (0 = unknown). A sticky balancer prefers the backend the
// user was sent to before, when it is still among the candidates left by health, zone and canary rules.
func (s *Sim) pickBackendFor(n *node, user int) *edge {
	var cand []*edge
	for _, e := range n.out {
		if e.removed || e.to.removed || s.believedDown(n, e.to) || e.ejectTill > s.now {
			continue
		}
		cand = append(cand, e)
	}
	if len(cand) == 0 {
		return nil
	}
	if n.kind == KLoadBalancer && n.s("zoneRouting") == "local" {
		var near []*edge
		for _, e := range cand {
			if e.to.zone == n.zone {
				near = append(near, e)
			}
		}
		if len(near) > 0 {
			cand = near
		} else if !n.b("crossZoneFailover") {
			return nil
		}
	}
	if n.kind == KLoadBalancer {
		if pct := n.f("canaryPct"); pct > 0 && n.s("layer") != "layer4" {
			cand = s.canarySplit(n, cand, pct)
		}
	}
	sticky := user != 0 && n.kind == KLoadBalancer && n.b("sticky") && n.s("layer") != "layer4"
	if sticky {
		if home := n.stickyHome[user]; home != nil {
			for _, e := range cand {
				if e == home {
					return e
				}
			}
		}
	}
	e := s.chooseFrom(n, cand)
	if sticky {
		if n.stickyHome == nil {
			n.stickyHome = map[int]*edge{}
		}
		n.stickyHome[user] = e
	}
	return e
}

func (s *Sim) chooseFrom(n *node, cand []*edge) *edge {
	alg := "roundRobin"
	if n.kind == KLoadBalancer {
		alg = n.s("algorithm")
	}
	switch alg {
	case "random":
		return cand[int(s.rng.Next()*float64(len(cand)))]
	case "leastConn":
		best := cand[0]
		for _, e := range cand[1:] {
			if e.inflight < best.inflight {
				best = e
			}
		}
		return best
	}
	i := n.rr["out"] % len(cand)
	n.rr["out"]++
	return cand[i]
}

func (s *Sim) procRoute(n *node, c *call, cb func(Result), release func()) {
	base, wait := 0.2, 0.0
	if n.kind == KLoadBalancer {
		if n.s("layer") == "layer4" {
			base = 0.05 // it forwards connections without reading the request
		}
		if per := lbServiceMs(n); per > 0 {
			start := max(s.now, n.lbFree)
			if start-s.now > n.f("lbQueueMs") {
				release()
				s.failAt(n, RQueueFull, cb)
				return
			}
			n.lbFree, wait = start+per, start-s.now
		}
	}
	s.hold(n, wait+base*n.slow, release, cb, func() {
		release()
		e := s.pickBackendFor(n, c.user)
		if e == nil {
			s.failAt(n, RNoBackend, cb)
			return
		}
		start := s.now
		e.inflight++
		s.hop(n, e, c, func(res Result) {
			e.inflight--
			s.recordBackend(e, res.OK, s.now-start)
			s.passBack(n, res, cb)
		})
	})
}

func (s *Sim) recordBackend(e *edge, ok bool, lat float64) {
	if e.from.kind != KLoadBalancer {
		return
	}
	i := s.tickCount % 20
	e.oCnt[i]++
	e.oSum[i] += lat
	if !ok {
		e.oErr[i]++
	}
}

func sum20(a [20]float64) float64 {
	var t float64
	for _, v := range a {
		t += v
	}
	return t
}

func (s *Sim) ejectOutliers(lb *node) {
	var apps []*edge
	for _, e := range lb.out {
		if !e.removed && e.to.kind == KAppServer {
			apps = append(apps, e)
		}
	}
	maxEject := len(apps) / 2
	ejected := 0
	for _, e := range apps {
		if e.ejectTill > s.now {
			ejected++
		}
	}
	if ejected >= maxEject {
		return
	}
	cnt := make([]float64, len(apps))
	mean := make([]float64, len(apps))
	for i, e := range apps {
		cnt[i] = sum20(e.oCnt)
		if cnt[i] > 0 {
			mean[i] = sum20(e.oSum) / cnt[i]
		}
	}
	type cand struct {
		e     *edge
		score float64
	}
	var picks []cand
	for i, e := range apps {
		if e.ejectTill > s.now || cnt[i] < 10 {
			continue
		}
		if er := sum20(e.oErr) / cnt[i]; er > 0.5 {
			picks = append(picks, cand{e, 1e9 + er})
			continue
		}
		var peers []float64
		for j := range apps {
			if j != i && cnt[j] > 0 {
				peers = append(peers, mean[j])
			}
		}
		if len(peers) == 0 {
			continue
		}
		slices.Sort(peers)
		med := peers[len(peers)/2]
		if len(peers)%2 == 0 {
			med = (peers[len(peers)/2-1] + peers[len(peers)/2]) / 2
		}
		if med > 0 && mean[i] > 3*med {
			picks = append(picks, cand{e, mean[i] / med})
		}
	}
	sort.Slice(picks, func(i, j int) bool { return picks[i].score > picks[j].score })
	for _, p := range picks {
		if ejected >= maxEject {
			break
		}
		p.e.ejectTill = s.now + ejectMs
		ejected++
	}
}

func (s *Sim) pickCyclic(n *node, key string, list []*edge, ok func(*edge) bool) *edge {
	if len(list) == 0 {
		return nil
	}
	start := n.rr[key] % len(list)
	for o := 0; o < len(list); o++ {
		i := (start + o) % len(list)
		if e := list[i]; !e.removed && (ok == nil || ok(e)) {
			n.rr[key] = i + 1
			return e
		}
	}
	return nil
}

// canarySplit narrows the candidates to the new version (v2) for the canary share of the requests and to the old one
// for the rest. The split is an accumulator, not a random draw, so it is exact and costs no random numbers. If one of
// the versions has no server available, everything goes to the other.
func (s *Sim) canarySplit(lb *node, cand []*edge, pct float64) []*edge {
	var v1, v2 []*edge
	for _, e := range cand {
		if e.to.s("version") == "v2" {
			v2 = append(v2, e)
		} else {
			v1 = append(v1, e)
		}
	}
	if len(v1) == 0 || len(v2) == 0 {
		return cand
	}
	lb.canaryAcc += pct / 100
	if lb.canaryAcc >= 1 {
		lb.canaryAcc--
		return v2
	}
	return v1
}

// lbServiceMs is the time a balancer spends on each request at its throughput limit (0 = no limit): a layer-4 switch,
// which reads no content, passes four times as many requests as a layer-7 one.
func lbServiceMs(n *node) float64 {
	rps := n.f("maxRps")
	if rps <= 0 {
		return 0
	}
	per := 1000 / rps
	if n.s("layer") == "layer4" {
		per /= 4
	}
	return per
}
