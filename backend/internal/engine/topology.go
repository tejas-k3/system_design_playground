package engine

import "math"

// Topology: building the node and edge graph and applying live edits (Sync).

func (s *Sim) addNode(d Node) *node {
	n := &node{
		id: d.ID, kind: d.Kind, label: d.Label, x: d.X, y: d.Y,
		p: Resolve(d.Kind, d.Params), slow: 1,
		pools: map[*node]*pool{}, locks: map[int]redisLock{},
		booked: map[int]int{}, klocks: map[int]*keyLock{}, tver: map[int]int{}, txl: map[int]*txLock{}, txWaits: map[int]int{}, rr: map[string]int{},
		lastBusy: s.now,
	}
	n.zone, n.region = n.s("zone"), n.s("region")
	n.tokens = n.f("burst")
	n.tokensAt = s.now
	s.configStore(n)
	s.nodes[n.id] = n
	s.order = append(s.order, n)
	n.idBase = len(s.order) * 1_000_000
	switch d.Kind {
	case KQueue, KTopic, KLog:
		s.hasQueue = true
	}
	if d.Kind == KClient {
		if n.b("abuse") {
			s.hasAbuse = true
		}
		if n.s("workload") == "booking" {
			s.hasBooking = true
		}
		if n.s("consistency") != "eventual" {
			s.hasFresh = true
		}
	}
	return n
}

func (s *Sim) configStore(n *node) {
	if n.kind != KCache && n.kind != KCDN {
		return
	}
	pol := "lru"
	if n.kind == KCache {
		pol = n.s("eviction")
	}
	capItems := int(n.f("capacityItems"))
	ttlMs := n.f("ttlSec") * 1000
	if ttlMs > 0 {
		ttlMs = math.Max(1, ttlMs-n.f("clockSkewMs")) // a clock ahead sees the stamped lifetime as shorter
	}
	if mb := n.f("capacityMB"); mb > 0 {
		// bodies are as large as the biggest response any client asks for (1 KB when none says)
		kb := 1.0
		for _, c := range s.order {
			if c.kind == KClient && c.f("respKB") > kb {
				kb = c.f("respKB")
			}
		}
		capItems = min(capItems, int(mb*1000/kb))
	}
	if n.store == nil || n.storePol != pol {
		n.store = NewStore(pol, capItems, ttlMs, s.rng)
		n.store.SetJitter(n.f("ttlJitterPct") / 100)
		n.storePol = pol
		return
	}
	n.c.evict += n.store.SetCapacity(capItems)
	n.store.SetTTL(ttlMs)
	n.store.SetJitter(n.f("ttlJitterPct") / 100)
}

func (s *Sim) addEdge(d Edge) {
	from, to := s.nodes[d.From], s.nodes[d.To]
	if from == nil || to == nil {
		return
	}
	e := &edge{id: d.ID, from: from, to: to, latency: d.LatencyMs, bw: d.BandwidthMbps, loss: d.LossPct, jitter: d.JitterMs}
	s.edges[e.id] = e
	s.eord = append(s.eord, e)
	from.out = append(from.out, e)
	to.in = append(to.in, e)
}

func (s *Sim) removeEdge(e *edge) {
	e.removed = true
	delete(s.edges, e.id)
	drop := func(list []*edge) []*edge {
		out := list[:0]
		for _, x := range list {
			if x != e {
				out = append(out, x)
			}
		}
		return out
	}
	e.from.out = drop(e.from.out)
	e.to.in = drop(e.to.in)
}

func (s *Sim) removeNode(n *node) {
	n.removed = true
	delete(s.nodes, n.id)
	for _, e := range append([]*edge(nil), n.out...) {
		s.removeEdge(e)
	}
	for _, e := range append([]*edge(nil), n.in...) {
		s.removeEdge(e)
	}
	s.failQueued(n, RNodeDown)
	s.closePools(n)
	s.dropConns(n)
	n.arrivalGen++
}

func (s *Sim) compact() {
	o := s.order[:0]
	for _, n := range s.order {
		if !n.removed {
			o = append(o, n)
		}
	}
	s.order = o
	e := s.eord[:0]
	for _, x := range s.eord {
		if !x.removed {
			e = append(e, x)
		}
	}
	s.eord = e
}

func (s *Sim) afterTopologyChange() {
	for _, n := range s.order {
		if n.removed {
			continue
		}
		s.prunePools(n)
		for _, e := range n.out {
			e.brState, e.brFails, e.trial = 0, 0, false
		}
	}
}

// Sync applies an edited diagram to a running simulation without restarting it.
func (s *Sim) Sync(d Diagram) {
	want := map[string]bool{}
	for _, dn := range d.Nodes {
		want[dn.ID] = true
		if n := s.nodes[dn.ID]; n != nil && n.kind == dn.Kind {
			n.label, n.x, n.y = dn.Label, dn.X, dn.Y
			n.p = Resolve(dn.Kind, dn.Params)
			n.zone, n.region = n.s("zone"), n.s("region")
			s.configStore(n)
			if n.kind == KRateLimiter {
				n.tokens = math.Min(n.tokens, n.f("burst"))
			}
			if n.kind == KClient && n.s("workload") == "booking" {
				s.hasBooking = true
			}
			if n.kind == KClient && n.s("consistency") != "eventual" {
				s.hasFresh = true
			}
		} else {
			if n != nil {
				s.removeNode(n)
			}
			nn := s.addNode(dn)
			if nn.kind == KClient {
				s.startClient(nn)
			}
		}
	}
	for _, n := range append([]*node(nil), s.order...) {
		if !n.removed && !want[n.id] {
			s.removeNode(n)
		}
	}
	wantE := map[string]Edge{}
	for _, de := range d.Edges {
		wantE[de.ID] = de
	}
	for _, e := range append([]*edge(nil), s.eord...) {
		if de, ok := wantE[e.id]; !e.removed && (!ok || de.From != e.from.id || de.To != e.to.id) {
			s.removeEdge(e)
		}
	}
	for _, de := range d.Edges {
		if e := s.edges[de.ID]; e != nil {
			e.latency = de.LatencyMs
			e.bw = de.BandwidthMbps
			e.loss, e.jitter = de.LossPct, de.JitterMs
		} else {
			s.addEdge(de)
		}
	}
	s.compact()
	s.afterTopologyChange()
	s.noteReshard()
	for _, n := range s.order {
		s.startNext(n)
		s.pumpAll(n)
	}
}
