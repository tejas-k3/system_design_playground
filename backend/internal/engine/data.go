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
		if e.to.kind == KKVStore || e.to.kind == KCoordinator {
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
	if c.lg != nil && c.lg.connect {
		n.c.connects++
		if cp := s.connCapacity(n); n.sessConns >= cp {
			done(fail(RConnRefused))
			return
		}
		n.sessConns++
		if n.sessions == nil {
			n.sessions = map[*session]bool{}
		}
		n.sessions[c.lg.sess] = true
		c.lg.sess.node, c.lg.sess.ep = n, n.epoch
		s.after(s.service(n, n.f("handshakeMs"))*n.slow, func() { done(Result{OK: true, Ver: -1, Done: true, Src: n}) })
		return
	}
	cpu := s.service(n, n.f("cpuMs")*s.classMult(c, n)) * n.slow
	n.cpuWork += cpu
	if n.throttled {
		cpu *= 100 / n.f("burstBaselinePct")
	}
	s.after(cpu, func() {
		if n.epoch != ep || n.removed {
			done(fail(RNodeDown)) // crashed while computing: do not start downstream work
			return
		}
		cc := *c
		cc.idem = n.b("idempotencyKeys")
		cc.honor = cc.honor || n.b("propagateDeadline")
		run := func() {
			if cc.write {
				s.appWrite(n, &cc, done)
			} else {
				s.appRead(n, &cc, done)
			}
		}
		if ve := s.first(n, KVectorStore); ve != nil { // retrieval first; the answer, if there is a model, is written from what it found
			s.callCmd(n, ve, &cc, cmdRead, 0, func(res Result) {
				if !res.OK {
					done(res)
					return
				}
				if le := s.first(n, KLLMServer); le != nil {
					s.callCmd(n, le, &cc, cmdEnqueue, 0, done)
					return
				}
				done(res)
			})
			return
		}
		if le := s.first(n, KLLMServer); le != nil { // the answer is written by the LLM server
			s.callCmd(n, le, &cc, cmdEnqueue, 0, done)
			return
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
		if ee := s.hostEdge(n, cc.key); ee != nil && !s.runsWorkflow(n) && (cc.write || ee.to.s("calledFor") == "all") {
			s.politely(n, ee, func() {
				s.callCmd(n, ee, &cc, cmdExternal, 0, func(res Result) {
					if !res.OK {
						done(res)
						return
					}
					run()
				})
			})
			return
		}
		run()
	})
}

// splitHotKey spreads the one hot item over several rows: a write goes to one of them, a read has to add all up.
func splitHotKey(n *node, c *call, read bool) *call {
	if n.s("hotKeyMode") != "split" || c.key != 0 {
		return c
	}
	shards := clampInt(int(n.f("splitShards")), 2, 64)
	cc := *c
	cc.key = 1_000_000 + int(c.hash%uint32(shards))
	if read {
		cc.readX = float64(shards)
	}
	return &cc
}

func (s *Sim) appRead(n *node, c *call, done func(Result)) {
	if n.b("bloomGuard") && c.lg != nil && s.bloomBlocks(n, c) {
		s.after(0.05, func() { done(Result{OK: true, Ver: -1, Done: true, Src: n}) }) // not found, without asking anyone
		return
	}
	c = splitHotKey(n, c, true)
	// a page with a body: after the lookup, the body is fetched from the object store the app server is wired to
	if oe := s.first(n, KObjectStore); oe != nil && c.lg != nil && (c.wl == wlKV || c.wl == wlFeed) && c.lg.client.f("respKB") > 0 {
		inner := done
		done = func(r Result) {
			if !r.OK {
				inner(r)
				return
			}
			s.callCmd(n, oe, c, cmdGet, 0, func(g Result) {
				if !g.OK {
					inner(g)
					return
				}
				inner(r)
			})
		}
	}
	if c.wl == wlFeed {
		cc := *c
		cc.fan = s.feedFan(n, c)
		c = &cc
	}
	if g := n.s("gather"); g == "all" || g == "firstK" {
		if es := s.outOf(n, KIndex); len(es) >= 2 {
			need := len(es)
			if g == "firstK" {
				need = min(need, int(n.f("k")))
			}
			s.gather(n, c, es, need, n.b("partialResults"), done)
			return
		}
	}
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
		if reps := s.outOf(n, KReplica); !primaryOnly && len(reps) >= 2 && (n.f("hedgeReadsMs") > 0 || n.s("readSpread") == "roundRobin") {
			s.replicaRead(n, c, reps, cb)
			return
		}
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
	local := s.localStore(n)
	if local != nil {
		if ver, hit, _ := local.Get(s.now, c.key); hit {
			n.c.localHit++
			done(Result{OK: true, Ver: ver, Hit: true, Done: true, Src: n})
			return
		}
	}
	ce := s.pickCyclic(n, "cache", caches, nil)
	s.callCmd(n, ce, c, cmdCacheGet, 0, func(res Result) {
		if res.OK && res.Done {
			if local != nil && res.Ver >= 0 {
				local.Set(s.now, c.key, res.Ver)
			}
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
	c = splitHotKey(n, c, false)
	if c.wl == wlFeed {
		cc := *c
		cc.fan = s.feedFan(n, c)
		c = &cc
	}
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
			if m := n.s("workflow"); (m == "naive" || m == "saga" || m == "twoPhase") && s.workflowWrite(n, c, m, done) {
				return
			}
			if m := n.s("eventPublish"); (m == "dualWrite" || m == "outbox") && s.eventWrite(n, c, m, done) {
				return
			}
			qe := s.first(n, KQueue)
			if qe == nil {
				qe = s.first(n, KTopic)
			}
			if qe == nil {
				qe = s.first(n, KLog)
			}
			if qe == nil {
				qe = s.first(n, KStream)
			}
			if qe != nil {
				s.callCmd(n, qe, c, cmdEnqueue, 0, done)
				return
			}
		}
		s.dbWrite(n, c, done)
	}
}

func (s *Sim) dbWrite(n *node, c *call, done func(Result)) {
	e := s.primaryEdge(n)
	if e == nil { // no database of its own: a cache wired to one takes the write and passes it on
		for _, ce := range s.outOf(n, KCache) {
			if s.primaryEdge(ce.to) != nil {
				s.callCmd(n, ce, c, cmdWrite, 0, done)
				return
			}
		}
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
// localStore is the app server's own small cache for hot items, or nil when the lever is off.
func (s *Sim) localStore(n *node) *Store {
	if n.s("hotKeyMode") != "localCache" {
		n.local = nil
		return nil
	}
	if n.local == nil {
		n.local = NewStore("lru", 0, 0, s.rng)
	}
	n.local.SetCapacity(int(n.f("localCacheItems")))
	n.local.SetTTL(n.f("localCacheTtlSec") * 1000)
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

func (s *Sim) bookingWrite(n *node, c *call, done func(Result)) {
	rd := s.redisEdge(n)
	if n.s("lockService") == "coordinator" {
		rd = s.first(n, KCoordinator)
	}
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
				bc.fence = res.Fence
				write := func() {
					s.dbWrite(n, &bc, func(r Result) {
						s.callCmd(n, rd, &bc, cmdUnlock, 0, func(Result) {})
						done(r)
					})
				}
				if n.pausedUntil > s.now { // frozen with the lock in hand: the write waits for the pause, the lease runs on
					// what it believes when it freezes: it saw the item free (if it was) and will write on that
					if pe := s.primaryEdge(n); pe != nil {
						if db, _, _ := s.resolveTarget(pe, &bc, cmdWrite); db != nil && db.kind == KDatabase && db.booked[bc.key] == 0 {
							bc.blind = true
						}
					}
					s.at(n.pausedUntil, write)
					return
				}
				write()
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

// readThroughEdge is where a cache fetches misses from, preferring a replica.
// revalidatable returns the version of a held but expired entry that a revalidating cache may check. The check is a cost
// model (the origin is not sent a request), so it needs a read-through origin that is up, and it is not used with Range size.
func (s *Sim) revalidatable(n *node, key int) (int, bool) {
	if e := s.readThroughEdge(n); e == nil || e.to.down || s.rangePiecesOf(n) > 0 {
		return 0, false
	}
	return n.store.Expired(s.now, key)
}

func (s *Sim) readThroughEdge(n *node) *edge {
	if e := s.first(n, KReplica); e != nil {
		return e
	}
	return s.primaryEdge(n)
}

// ---- database -----------------------------------------------------------------------

// diskReadMult is how much slower a page read from disk is than one found in memory.
const diskReadMult = 10

func (s *Sim) readMult(n *node, c *call) float64 {
	m := s.readMultBase(n, c)
	if n.s("engine") == "lsm" {
		m *= 1.5 // a read may have to look in several levels
	}
	if data := n.f("dataGB"); data > n.f("bufferPoolGB") && n.kind == KDatabase {
		miss := 1 - n.f("bufferPoolGB")/data // pages not in memory (uniform access)
		m *= 1 + miss*(diskReadMult-1)
	}
	return m
}

// secondaryLookup says whether this read looks an item up by something other than its key.
func (s *Sim) secondaryLookup(c *call) bool {
	if c.lg == nil {
		return false
	}
	pct := c.lg.client.f("secondaryPct")
	return pct > 0 && float64(Hash32(uint32(c.rid)*2654435761+97)%100) < pct
}

func (s *Sim) readMultBase(n *node, c *call) float64 {
	switch c.wl {
	case wlGeo:
		if !n.b("spatialIndex") {
			return 25
		}
	case wlLeader:
		return 10
	}
	// a lookup by another field: free with an index, a scan without (chosen from the request id, so no random draw)
	if s.secondaryLookup(c) && (n.f("indexes") < 1 || n.s("model") == "wideColumn") {
		m := n.f("scanMult")
		if n.s("model") == "wideColumn" {
			m *= 3 // no secondary access path at all: the scan is over every partition
		}
		return m
	}
	switch n.s("model") { // single-key reads
	case "document":
		return 0.8
	case "timeSeries":
		return 1.5 // random point reads are what an append-optimised layout is worst at
	}
	return 1
}

// writeMult is the cost of keeping secondary indexes up to date: 30% more per index.
func (s *Sim) writeMult(n *node) float64 {
	m := 1 + 0.3*n.f("indexes")
	switch n.s("model") {
	case "document":
		m *= 0.8
	case "wideColumn":
		m *= 0.4
	case "timeSeries":
		m *= 0.3
	}
	if n.s("engine") == "lsm" {
		m *= 0.5 // sequential appends instead of in-place page updates
	}
	return m
}

func (s *Sim) procDB(n *node, c *call, cb func(Result), release func()) {
	write := c.cmd == cmdWrite
	if write && c.wl == wlBooking {
		s.dbBooking(n, c, cb, release)
		return
	}
	if write && (c.wl == wlTransfer || c.wl == wlOnCall) && n.kind == KDatabase {
		s.dbTransfer(n, c, cb, release)
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
	if c.fan > 0 {
		ms += c.fan * feedUnitMs
	}
	if c.heavy {
		ms *= s.heavyMult(c)
	}
	if c.readX > 1 && !write { // a counter split over several rows is read by adding them all up
		ms *= c.readX
	}
	exec := func(unlock func()) {
		took := s.service(n, ms*contentionMult(n)) * n.slow
		if write && n.stallUntil > s.now { // an LSM database is compacting: the write waits for it to finish
			took += n.stallUntil - s.now
		}
		if write && n.kind == KDatabase && n.f("diskIops") > 0 {
			per := 1000 / n.f("diskIops")
			start := math.Max(s.now, n.diskFree)
			n.diskFree = start + per*(1+n.f("indexes"))
			took += n.diskFree - s.now
		}
		s.hold(n, took, func() { unlock(); release() }, cb, func() {
			unlock()
			release()
			res := Result{Ver: -1, Done: true, Src: n}
			switch {
			case write:
				if n.kind == KDatabase && n.s("topology") == "multiLeader" {
					s.multiLeaderWrite(n, c)
				}
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
	if write && n.kind == KDatabase && n.b("hotRowLock") {
		// writes to one row run one after another; a waiting write keeps its connection
		ep := n.epoch
		s.lockKey(n, c.key, func(unlock func()) {
			if n.epoch != ep || n.removed {
				unlock()
				release()
				s.failAt(n, RNodeDown, cb)
				return
			}
			exec(unlock)
		})
		return
	}
	exec(func() {})
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
			if b := n.booked[c.key]; b != 0 && b != c.rid && !c.blind { // a blind writer does not look again
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
				if n.b("fencingTokens") && c.fence > 0 { // refuse a write from a holder whose lease has been superseded
					if c.fence < n.fenceSeen[c.key] {
						unlock()
						release()
						s.failAt(n, RStaleToken, cb)
						return
					}
					if n.fenceSeen == nil {
						n.fenceSeen = map[int]int{}
					}
					n.fenceSeen[c.key] = c.fence
				}
				if b := n.booked[c.key]; b != 0 && b != c.rid && n.b("conditionalWrite") { // compare-and-set: it is taken, say so
					unlock()
					release()
					s.okAt(n, Result{Ver: s.itemVer(c.key), Done: true, Src: n}, cb)
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
	case cmdScore:
		ms = n.f("scoreMs")
	case cmdRank:
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

// gather sends the read to every shard at once and answers as soon as `need` of them have answered successfully
// (all of them for wait-for-all). The shards still working when it answers keep running: their capacity is spent
// although nobody is waiting. If success is impossible (too many failed) the request fails with the first failure in
// edge order, unless partial results are allowed and at least one shard answered, when it succeeds and is counted.
func (s *Sim) gather(n *node, c *call, es []*edge, need int, partial bool, done func(Result)) {
	results := make([]Result, len(es))
	got := make([]bool, len(es))
	okN, failN, finished := 0, 0, false
	finish := func(r Result) {
		if !finished {
			finished = true
			done(r)
		}
	}
	firstFailure := func() Result {
		for i, r := range results {
			if got[i] && !r.OK {
				return r
			}
		}
		return fail(RDependency)
	}
	hedge := n.f("hedgeAfterMs")
	for i, e := range es {
		i, e := i, e
		reply := func(r Result) {
			if got[i] { // the other copy of this query already answered
				return
			}
			results[i], got[i] = r, true
			if r.OK {
				okN++
			} else {
				failN++
			}
			switch {
			case okN >= need:
				finish(Result{OK: true, Ver: -1, Done: true, Src: n})
			case partial && okN+failN == len(es) && okN > 0:
				if c.counted {
					s.runPartial++
				}
				finish(Result{OK: true, Ver: -1, Done: true, Src: n})
			case !partial && failN > len(es)-need:
				finish(firstFailure())
			case okN+failN == len(es):
				finish(firstFailure())
			}
		}
		s.callCmd(n, e, c, cmdRead, 0, reply)
		if hedge > 0 {
			// a slow shard: ask again, the first answer counts and the slower copy still occupies its slot
			s.after(hedge, func() {
				if got[i] || finished || n.removed {
					return
				}
				if c.counted {
					s.runHedged++
				}
				s.callCmd(n, e, c, cmdRead, 0, reply)
			})
		}
	}
}

// heavyMult is how many times a heavy request costs what a light one does (1 for a light request).
func (s *Sim) heavyMult(c *call) float64 {
	if !c.heavy || c.lg == nil {
		return 1
	}
	return c.lg.client.f("heavyMult")
}

func (s *Sim) classMult(c *call, n *node) float64 { return s.heavyMult(c) }
