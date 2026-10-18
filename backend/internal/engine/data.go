package engine

import "math"

// ---- edge lookups -----------------------------------------------------------------

func (s *Sim) outOf(n *node, kinds ...string) []*edge {
	var out []*edge
	for _, e := range n.out {
		if e.removed {
			continue
		}
		for _, k := range kinds {
			if e.to.kind == k {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

func (s *Sim) first(n *node, kinds ...string) *edge {
	if l := s.outOf(n, kinds...); len(l) > 0 {
		return l[0]
	}
	return nil
}

func hasChild(sr *node, kind string) bool {
	for _, e := range sr.out {
		if !e.removed && e.to.kind == kind {
			return true
		}
	}
	return false
}

// primaryEdge is the edge to the primary data store (a database, or a shard router over databases).
func (s *Sim) primaryEdge(n *node) *edge {
	for _, e := range n.out {
		if e.removed {
			continue
		}
		if e.to.kind == KDatabase {
			if t := e.to.successor; t != nil && t.promoted && !t.removed { // a replica took over: writes go there now
				if pe := s.edgeTo(n, t); pe != nil {
					return pe
				}
				continue // this server cannot reach the new primary
			}
			return e
		}
		if e.to.kind == KShardRouter && hasChild(e.to, KDatabase) {
			return e
		}
	}
	return nil
}

func (s *Sim) edgeTo(n, to *node) *edge {
	for _, e := range n.out {
		if !e.removed && e.to == to {
			return e
		}
	}
	return nil
}

func (s *Sim) redisEdge(n *node) *edge {
	for _, e := range n.out {
		if e.removed {
			continue
		}
		if e.to.kind == KRedis || (e.to.kind == KShardRouter && hasChild(e.to, KRedis)) {
			return e
		}
	}
	return nil
}

// ---- app server ---------------------------------------------------------------------

func (s *Sim) procApp(n *node, c *call, cb func(Result), release func()) {
	ep := n.epoch
	var done func(res Result)
	done = func(res Result) {
		if n.pausedUntil > s.now { // a stop-the-world pause: nothing is sent back until it ends
			s.at(n.pausedUntil, func() { done(res) })
			return
		}
		release()
		if n.epoch != ep || n.removed {
			// the process died or was replaced while this request was waiting: its work is lost
			res = fail(RNodeDown)
		}
		if res.OK {
			n.c.ok++
		} else {
			n.c.err++
			n.c.reason[res.Reason]++
		}
		cb(res)
	}
	cpu := s.service(n, n.f("cpuMs")) * n.slow
	s.after(cpu, func() {
		if n.epoch != ep || n.removed {
			done(fail(RNodeDown)) // crashed while computing: do not start downstream work
			return
		}
		cc := *c
		cc.idem = n.b("idempotencyKeys")
		run := func() {
			if cc.write {
				s.appWrite(n, &cc, done)
			} else {
				s.appRead(n, &cc, done)
			}
		}
		// claim check: a write with a body goes to the object store first; from then on only a pointer travels
		if oe := s.first(n, KObjectStore); oe != nil && cc.write && cc.lg != nil && cc.lg.client.f("reqKB") > 0 {
			s.callCmd(n, oe, &cc, cmdPut, 0, func(res Result) {
				if !res.OK {
					done(res)
					return
				}
				cc.claim = true
				run()
			})
			return
		}
		if ee := s.first(n, KExternalAPI); ee != nil && (cc.write || ee.to.s("calledFor") == "all") {
			s.callCmd(n, ee, &cc, cmdExternal, 0, func(res Result) {
				if !res.OK {
					done(res)
					return
				}
				run()
			})
			return
		}
		run()
	})
}

func (s *Sim) appRead(n *node, c *call, done func(Result)) {
	if c.wl == wlGeo || c.wl == wlLeader {
		if re := s.redisEdge(n); re != nil {
			cm := cmdGeo
			if c.wl == wlLeader {
				cm = cmdRank
			}
			s.callCmd(n, re, c, cm, 0, done)
			return
		}
	}
	routing := n.s("readRouting")
	needFresh := c.lg != nil && c.lg.client.s("consistency") != "eventual"
	primaryOnly := routing == "primary" || (routing == "byConsistency" && needFresh)
	caches := s.outOf(n, KCache)
	useCache := !primaryOnly && len(caches) > 0
	fetch := func(cb func(Result)) {
		var e *edge
		if !primaryOnly {
			e = s.first(n, KReplica)
		}
		if e == nil {
			e = s.primaryEdge(n)
		}
		if e == nil {
			cb(fail(RNoRoute))
			return
		}
		s.callCmd(n, e, c, cmdRead, 0, cb)
	}
	if !useCache {
		fetch(done)
		return
	}
	ce := s.pickCyclic(n, "cache", caches, nil)
	s.callCmd(n, ce, c, cmdCacheGet, 0, func(res Result) {
		if res.OK && res.Done {
			done(res)
			return
		}
		populate := res.OK
		fetch(func(r Result) {
			if r.OK && populate && r.Ver >= 0 {
				s.callCmd(n, ce, c, cmdCacheSet, r.Ver, func(Result) {})
			}
			done(r)
		})
	})
}

func (s *Sim) appWrite(n *node, c *call, done func(Result)) {
	{
		switch c.wl {
		case wlBooking:
			s.bookingWrite(n, c, done)
			return
		case wlLeader, wlGeo:
			if re := s.redisEdge(n); re != nil {
				s.callCmd(n, re, c, cmdScore, 0, done)
				return
			}
		default:
			if qe := s.first(n, KQueue); qe != nil {
				s.callCmd(n, qe, c, cmdEnqueue, 0, done)
				return
			}
		}
		s.dbWrite(n, c, done)
	}
}

func (s *Sim) dbWrite(n *node, c *call, done func(Result)) {
	e := s.primaryEdge(n)
	if e == nil {
		done(fail(RNoRoute))
		return
	}
	s.callCmd(n, e, c, cmdWrite, 0, func(res Result) {
		if !res.OK {
			done(res)
			return
		}
		s.invalidate(n, c, res.Ver, func() { done(res) })
	})
}

// invalidate applies each connected cache's write policy, one after another.
func (s *Sim) invalidate(n *node, c *call, ver int, then func()) {
	caches := s.outOf(n, KCache)
	var step func(i int)
	step = func(i int) {
		for ; i < len(caches); i++ {
			switch caches[i].to.s("writePolicy") {
			case "invalidate":
				e, next := caches[i], i+1
				s.callCmd(n, e, c, cmdCacheInv, 0, func(Result) { step(next) })
				return
			case "writeThrough":
				e, next := caches[i], i+1
				s.callCmd(n, e, c, cmdCacheSet, ver, func(Result) { step(next) })
				return
			}
		}
		then()
	}
	step(0)
}

func (s *Sim) bookingWrite(n *node, c *call, done func(Result)) {
	rd := s.redisEdge(n)
	if rd == nil {
		s.dbWrite(n, c, done)
		return
	}
	bc := *c
	s.tokenSeq++
	bc.token = s.tokenSeq
	bc.ttl = n.f("lockTtlMs")
	start := s.now
	var try func()
	try = func() {
		s.callCmd(n, rd, &bc, cmdLock, 0, func(res Result) {
			if res.OK {
				s.dbWrite(n, &bc, func(r Result) {
					s.callCmd(n, rd, &bc, cmdUnlock, 0, func(Result) {})
					done(r)
				})
				return
			}
			if res.Reason == RLockBusy && n.f("lockWaitMs") > 0 && s.now-start < n.f("lockWaitMs") {
				s.after(25, try)
				return
			}
			done(res)
		})
	}
	try()
}

// ---- cache --------------------------------------------------------------------------

func (s *Sim) procCache(n *node, c *call, cb func(Result), release func()) {
	ms := s.rng.Service(n.f("latencyMs"))
	if c.cmd == cmdCacheInv || c.cmd == cmdCacheSet {
		ms = 0.1
	}
	s.hold(n, ms*n.slow, release, cb, func() {
		release()
		switch c.cmd {
		case cmdCacheInv:
			n.store.Delete(c.key)
			s.okAt(n, Result{Ver: -1, Done: true}, cb)
		case cmdCacheSet:
			n.c.evict += n.store.Set(s.now, c.key, c.ver)
			s.okAt(n, Result{Ver: -1, Done: true}, cb)
		case cmdCacheGet:
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

// readThroughEdge is where a cache fetches misses from, preferring a replica.
func (s *Sim) readThroughEdge(n *node) *edge {
	if e := s.first(n, KReplica); e != nil {
		return e
	}
	return s.primaryEdge(n)
}

// ---- database -----------------------------------------------------------------------

func (s *Sim) readMult(n *node, c *call) float64 {
	switch c.wl {
	case wlGeo:
		if !n.b("spatialIndex") {
			return 25
		}
	case wlLeader:
		return 10
	}
	// a lookup by another field: free with an index, a scan without (chosen from the request id, so no random draw)
	if c.lg != nil && n.f("indexes") < 1 {
		if pct := c.lg.client.f("secondaryPct"); pct > 0 && float64(Hash32(uint32(c.rid)*2654435761+97)%100) < pct {
			return n.f("scanMult")
		}
	}
	return 1
}

// writeMult is the cost of keeping secondary indexes up to date: 30% more per index.
func (s *Sim) writeMult(n *node) float64 { return 1 + 0.3*n.f("indexes") }

func (s *Sim) procDB(n *node, c *call, cb func(Result), release func()) {
	write := c.cmd == cmdWrite
	if write && c.wl == wlBooking {
		s.dbBooking(n, c, cb, release)
		return
	}
	ms := n.f("readMs") * s.readMult(n, c)
	if write {
		ms = n.f("writeMs") * s.writeMult(n)
		if c.idem {
			ms++
		}
		if n.kind == KDatabase {
			wait, down := s.syncReplicaCost(n)
			if down {
				release()
				s.failAt(n, RSyncReplica, cb) // the primary cannot acknowledge what its synchronous replica has not confirmed
				return
			}
			ms += wait
		}
	}
	s.hold(n, s.service(n, ms)*n.slow, release, cb, func() {
		release()
		res := Result{Ver: -1, Done: true, Src: n}
		switch {
		case write:
			res.Ver = s.commit(n, c)
			if n.promoted {
				n.sincePromotion++
			}
		case c.wl == wlGeo || c.wl == wlLeader:
		case n.kind == KReplica && !n.promoted:
			res.Ver = s.verAt(c.key, s.now-s.replicaLag(n))
		default:
			res.Ver = s.itemVer(c.key)
		}
		s.okAt(n, res, cb)
	})
}

func (s *Sim) lockKey(n *node, key int, fn func(unlock func())) {
	kl := n.klocks[key]
	if kl == nil {
		kl = &keyLock{}
		n.klocks[key] = kl
	}
	var unlock func()
	unlock = func() {
		if len(kl.waiters) > 0 {
			next := kl.waiters[0]
			kl.waiters = kl.waiters[1:]
			next(unlock)
			return
		}
		kl.held = false
	}
	if !kl.held {
		kl.held = true
		fn(unlock)
		return
	}
	kl.waiters = append(kl.waiters, fn)
}

// dbBooking models reserving a scarce item: check, then write. Without a lock two overlapping
// bookings of the same item both succeed, which is a double booking.
func (s *Sim) dbBooking(n *node, c *call, cb func(Result), release func()) {
	ep := n.epoch
	crashed := func(unlock func()) bool {
		if n.epoch != ep || n.removed {
			unlock()
			release()
			s.failAt(n, RNodeDown, cb)
			return true
		}
		return false
	}
	proceed := func(unlock func()) {
		// step 1: the availability check is a read
		s.after(s.service(n, n.f("readMs"))*n.slow, func() {
			if crashed(unlock) {
				return
			}
			if b := n.booked[c.key]; b != 0 && b != c.rid {
				unlock()
				release()
				s.okAt(n, Result{Ver: s.itemVer(c.key), Done: true, Src: n}, cb)
				return
			}
			// step 2: the write; anyone who checked in the meantime may also be writing
			ms := s.service(n, n.f("writeMs")*s.writeMult(n)) * n.slow
			if c.idem {
				ms++
			}
			s.after(ms, func() {
				if crashed(unlock) {
					return
				}
				if b := n.booked[c.key]; b != 0 && b != c.rid {
					n.c.conflict++
					if c.counted {
						s.runConflicts++
					}
				} else {
					n.booked[c.key] = c.rid
				}
				ver := s.commit(n, c)
				unlock()
				release()
				s.okAt(n, Result{Ver: ver, Done: true, Src: n}, cb)
			})
		})
	}
	if n.b("rowLocks") {
		s.lockKey(n, c.key, proceed)
	} else {
		proceed(func() {})
	}
}

// ---- redis ----------------------------------------------------------------------------

func (s *Sim) procRedis(n *node, c *call, cb func(Result), release func()) {
	var ms float64
	switch c.cmd {
	case cmdGeo:
		ms = n.f("geoSearchMs")
	case cmdRank, cmdScore:
		ms = 0.1
	default:
		ms = 0.05
	}
	s.hold(n, ms*n.slow, release, cb, func() {
		release()
		switch c.cmd {
		case cmdLock:
			if l, ok := n.locks[c.key]; ok && l.exp > s.now && l.token != c.token {
				n.c.lockBusy++
				n.c.ok++
				r := fail(RLockBusy)
				cb(r)
				return
			}
			n.locks[c.key] = redisLock{token: c.token, exp: s.now + c.ttl}
		case cmdUnlock:
			if l, ok := n.locks[c.key]; ok && l.token == c.token {
				delete(n.locks, c.key)
			}
		case cmdScore:
			s.noteRedisWrite(n, c.counted)
		}
		s.okAt(n, Result{Ver: -1, Done: true, Src: n}, cb)
	})
}

// noteRedisWrite remembers a counted write so a later crash can tell how much was lost.
func (s *Sim) noteRedisWrite(n *node, counted bool) {
	if !counted {
		return
	}
	n.rwTotal++
	if n.s("persistence") == "aof" {
		n.rwRecent = append(n.rwRecent, s.now)
		s.pruneRecentWrites(n)
	}
}

func (s *Sim) pruneRecentWrites(n *node) {
	i := 0
	for i < len(n.rwRecent) && n.rwRecent[i] < s.now-1000 {
		i++
	}
	n.rwRecent = n.rwRecent[i:]
}

// loseRedisData is called when a Redis node crashes: without persistence every write since it
// started is gone; with an append-only file only the last second (not yet fsynced) is.
func (s *Sim) loseRedisData(n *node) {
	lost := n.rwTotal
	if n.s("persistence") == "aof" {
		s.pruneRecentWrites(n)
		lost = len(n.rwRecent)
	}
	s.runLost += lost
	n.rwTotal, n.rwRecent = 0, nil
	n.locks = map[int]redisLock{}
}

// ---- queue + workers ----------------------------------------------------------------

func (s *Sim) arriveQueue(n *node, c *call, cb func(Result)) {
	if !c.write {
		s.failAt(n, RNoRoute, cb)
		return
	}
	if c.lg != nil && !c.claim && c.lg.client.f("reqKB") > n.f("maxMessageKB") { // sizes are 0 (not modelled) unless the users set them; a claim check carries a pointer
		s.failAt(n, RMessageTooLarge, cb)
		return
	}
	if len(n.jobs)-n.jh >= int(n.f("capacity")) {
		s.failAt(n, RQueueFull, cb)
		return
	}
	j := &job{rid: c.rid, key: c.key, wl: c.wl, idem: c.idem, counted: c.counted, enq: s.now}
	n.jobs = append(n.jobs, j)
	if j.counted {
		s.jobEnqueued++
	}
	s.after(0.2, func() {
		s.okAt(n, Result{Ver: -1, Done: true}, cb)
		s.dispatch(n)
	})
}

func (s *Sim) dispatch(q *node) {
	for q.jh < len(q.jobs) && !q.down {
		var w *node
		for _, e := range q.out {
			t := e.to
			if e.removed || t.kind != KWorker || t.down || t.removed || t.busy >= s.capacity(t) {
				continue
			}
			if w == nil || t.busy < w.busy {
				w = t
			}
		}
		if w == nil {
			break
		}
		j := q.jobs[q.jh]
		q.jobs[q.jh] = nil
		q.jh++
		s.runJob(q, w, j)
	}
	if q.jh > 256 && q.jh*2 > len(q.jobs) {
		q.jobs = append([]*job(nil), q.jobs[q.jh:]...)
		q.jh = 0
	}
}

func (s *Sim) runJob(q, w *node, j *job) {
	s.markBusy(w)
	w.busy++
	j.attempts++
	ep := w.epoch
	fin := func(res Result) {
		s.markBusy(w)
		w.busy--
		if res.OK {
			w.c.ok++
			w.c.done++
			if j.counted {
				s.jobCompleted++
				s.jobHist.Add(s.now - j.enq)
				s.jobsW.Push(s.now, true, s.now-j.enq)
			}
		} else {
			w.c.err++
			w.c.reason[res.Reason]++
			if j.attempts < 3 {
				if j.counted {
					s.jobRetried++
				}
				s.after(100*math.Pow(2, float64(j.attempts-1)), func() {
					q.jobs = append(q.jobs, j)
					s.dispatch(q)
				})
			} else if j.counted {
				s.jobLost++
			}
		}
		s.dispatch(q)
	}
	w.c.arr++
	s.after(s.service(w, w.f("processMs"))*w.slow, func() {
		if w.epoch != ep || w.removed {
			fin(fail(RNodeDown))
			return
		}
		c := &call{key: j.key, wl: j.wl, rid: j.rid, idem: j.idem, counted: j.counted, write: true, ver: -1, job: j}
		step := func() {
			e := s.primaryEdge(w)
			if e == nil {
				fin(Result{OK: true})
				return
			}
			s.callCmd(w, e, c, cmdWrite, 0, func(res Result) {
				if !res.OK {
					fin(res)
					return
				}
				s.invalidate(w, c, res.Ver, func() { fin(res) })
			})
		}
		if ee := s.first(w, KExternalAPI); ee != nil {
			s.callCmd(w, ee, c, cmdExternal, 0, func(res Result) {
				if !res.OK {
					fin(res)
					return
				}
				step()
			})
			return
		}
		step()
	})
}
