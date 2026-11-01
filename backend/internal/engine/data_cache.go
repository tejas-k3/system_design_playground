package engine

// invalidate applies each connected cache's write policy, one after another.
// localStore is the app server's own small cache for hot items, or nil when the lever is off.
func (s *Sim) localStore(n *node) *Store {
	if n.s("hotKeyMode") != "localCache" {
		n.local = nil
		return nil
	}
	if n.local == nil {
		n.local = newStore("lru", 0, 0, s.rng)
	}
	n.local.setCapacity(int(n.f("localCacheItems")))
	n.local.setTTL(n.f("localCacheTtlSec") * 1000)
	return n.local
}

// countCacheMsg counts a message the origin sent to a cache about a write, once the run is past its warm-up.
func (s *Sim) countCacheMsg(c *call) {
	if c.counted {
		s.runCacheMsgs++
	}
}

func (s *Sim) invalidate(n *node, c *call, ver int, then func()) {
	if n.local != nil {
		n.local.Delete(c.key)
	}
	caches := s.outOf(n, KCache)
	var step func(i int)
	step = func(i int) {
		for ; i < len(caches); i++ {
			switch caches[i].to.s("writePolicy") {
			case "lease": // tell only a cache that holds the item: its entry is the lease, and a cache without one will fetch afresh
				if to := caches[i].to; !to.store.Live(s.now, c.key) && to.f("rangeKB") <= 0 { // pieces are held under other keys: always tell
					continue
				}
				fallthrough
			case "invalidate":
				e, next := caches[i], i+1
				s.countCacheMsg(c)
				s.callCmd(n, e, c, cmdCacheInv, 0, func(Result) { step(next) })
				return
			case "writeThrough":
				e, next := caches[i], i+1
				s.countCacheMsg(c)
				s.callCmd(n, e, c, cmdCacheSet, ver, func(Result) { step(next) })
				return
			}
		}
		then()
	}
	step(0)
}

func (s *Sim) procCache(n *node, c *call, cb func(Result), release func()) {
	ms := s.rng.Service(n.f("latencyMs"))
	if c.cmd == cmdCacheInv || c.cmd == cmdCacheSet {
		ms = 0.1
	}
	checks := false
	if c.cmd == cmdCacheGet && n.b("revalidate") {
		if _, stale := s.revalidatable(n, c.key); stale {
			checks = true
			ms += n.f("revalidateMs") // the version check goes to the origin before the entry is answered or dropped
		}
	}
	s.hold(n, ms*n.slow, release, cb, func() {
		release()
		switch c.cmd {
		case cmdCacheInv:
			if pieces, _, _ := s.rangeOf(n, c); pieces > 0 {
				s.rangeDeleteAll(n, c, pieces)
			}
			n.store.Delete(c.key)
			s.okAt(n, Result{Ver: -1, Done: true}, cb)
		case cmdCacheSet:
			if pieces, _, _ := s.rangeOf(n, c); pieces > 0 {
				s.rangeSetAll(n, c, pieces, c.ver)
				s.okAt(n, Result{Ver: -1, Done: true}, cb)
				return
			}
			n.c.evict += n.store.Set(s.now, c.key, c.ver)
			s.okAt(n, Result{Ver: -1, Done: true}, cb)
		case cmdWrite: // a write that came through the cache: on to the database, keeping this copy current (write-through)
			e := s.primaryEdge(n)
			if e == nil {
				s.failAt(n, RNoRoute, cb)
				return
			}
			s.callCmd(n, e, c, cmdWrite, 0, func(res Result) {
				if res.OK && res.Ver >= 0 {
					if pieces, _, _ := s.rangeOf(n, c); pieces > 0 {
						s.rangeSetAll(n, c, pieces, res.Ver)
					} else {
						n.c.evict += n.store.Set(s.now, c.key, res.Ver)
					}
				}
				s.passBack(n, res, cb)
			})
		case cmdCacheGet:
			if pieces, size, piece := s.rangeOf(n, c); pieces > 0 {
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
				if e := s.readThroughEdge(n); e != nil {
					s.rangeMiss(n, c, missing, pieces, size, piece, func(cc *call, done func(Result)) { s.callCmd(n, e, cc, cmdRead, 0, done) }, cb)
					return
				}
				s.okAt(n, Result{Ver: -1}, cb)
				return
			}
			if checks {
				if sv, stale := s.revalidatable(n, c.key); stale && s.itemVer(c.key) == sv {
					n.store.Set(s.now, c.key, sv) // still current: renewed without fetching it again
					n.c.hit++
					if c.counted {
						s.runRevalidated++
					}
					s.okAt(n, Result{Ver: sv, Hit: true, Done: true, Src: n}, cb)
					return
				}
			}
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
			if e := s.readThroughEdge(n); e != nil {
				coalesce := n.b("coalesce")
				if coalesce {
					if waiters, busy := n.fetching[c.key]; busy { // someone is already fetching this key: wait for their answer
						n.fetching[c.key] = append(waiters, func(res Result) { s.passBack(n, res, cb) })
						return
					}
					if n.fetching == nil {
						n.fetching = map[int][]func(Result){}
					}
					n.fetching[c.key] = nil
				}
				s.callCmd(n, e, c, cmdRead, 0, func(res Result) {
					if res.OK && res.Ver >= 0 {
						n.c.evict += n.store.Set(s.now, c.key, res.Ver)
					}
					s.passBack(n, res, cb)
					if coalesce {
						waiters := n.fetching[c.key]
						delete(n.fetching, c.key)
						for _, w := range waiters {
							w(res)
						}
					}
				})
				return
			}
			s.okAt(n, Result{Ver: -1}, cb)
		default:
			s.failAt(n, RNoRoute, cb)
		}
	})
}

// revalidatable returns the version of a held but expired entry that a revalidating cache may check. The check is a cost
// model (the origin is not sent a request), so it needs a read-through origin that is up, and it is not used with Range size.
func (s *Sim) revalidatable(n *node, key int) (int, bool) {
	if e := s.readThroughEdge(n); e == nil || e.to.down || s.rangePiecesOf(n) > 0 {
		return 0, false
	}
	return n.store.Expired(s.now, key)
}

// readThroughEdge is where a cache fetches misses from, preferring a replica.
func (s *Sim) readThroughEdge(n *node) *edge {
	if e := s.first(n, KReplica); e != nil {
		return e
	}
	return s.primaryEdge(n)
}
