package engine

func (s *Sim) procCDN(n *node, c *call, cb func(Result), release func()) {
	ms := 0.2
	if !c.write {
		ms = s.rng.Service(1)
	}
	s.hold(n, ms*n.slow, release, cb, func() {
		release()
		pieces, size, piece := s.rangeOf(n, c)
		if !c.write && pieces > 0 {
			missing, ver, expired := s.rangeLookup(n, c, pieces)
			if expired {
				n.c.expired++
			}
			if len(missing) == 0 {
				n.c.hit++
				s.okAt(n, Result{Ver: ver, Hit: true, Done: true, Src: n}, cb)
				return
			}
			n.c.miss++
			e := s.pickBackend(n)
			if e == nil {
				s.failAt(n, RNoBackend, cb)
				return
			}
			s.rangeMiss(n, c, missing, pieces, size, piece, func(cc *call, done func(Result)) { s.hop(n, e, cc, done) }, cb)
			return
		}
		if !c.write {
			ver, hit, expired := n.store.Get(s.now, c.key)
			if expired {
				n.c.expired++
			}
			if hit {
				n.c.hit++
				s.okAt(n, Result{Ver: ver, Hit: true, Done: true, Src: n}, cb)
				return
			}
			n.c.miss++
		} else {
			if pieces > 0 {
				s.rangeDeleteAll(n, c, pieces)
			}
			n.store.Delete(c.key)
		}
		e := s.pickBackend(n)
		if e == nil {
			s.failAt(n, RNoBackend, cb)
			return
		}
		collapse := n.b("coalesce") && !c.write
		if collapse {
			if waiters, busy := n.fetching[c.key]; busy { // an identical request is already on its way to the origin
				n.fetching[c.key] = append(waiters, func(res Result) { s.passBack(n, res, cb) })
				return
			}
			if n.fetching == nil {
				n.fetching = map[int][]func(Result){}
			}
			n.fetching[c.key] = nil
		}
		s.hop(n, e, c, func(res Result) {
			if res.OK && !c.write && res.Ver >= 0 {
				n.c.evict += n.store.Set(s.now, c.key, res.Ver)
			}
			s.passBack(n, res, cb)
			if collapse {
				waiters := n.fetching[c.key]
				delete(n.fetching, c.key)
				for _, w := range waiters {
					w(res)
				}
			}
		})
	})
}
