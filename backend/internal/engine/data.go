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
			qe := s.first(n, KQueue)
			if qe == nil {
				qe = s.first(n, KTopic)
			}
			if qe == nil {
				qe = s.first(n, KLog)
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

func (s *Sim) invalidate(n *node, c *call, ver int, then func()) {
	if n.local != nil {
		n.local.Delete(c.key)
	}
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
	took := s.service(n, ms) * n.slow
	if write && n.stallUntil > s.now { // an LSM database is compacting: the write waits for it to finish
		took += n.stallUntil - s.now
	}
	if write && n.kind == KDatabase && n.f("diskIops") > 0 {
		per := 1000 / n.f("diskIops")
		start := math.Max(s.now, n.diskFree)
		n.diskFree = start + per*(1+n.f("indexes"))
		took += n.diskFree - s.now
	}
	s.hold(n, took, release, cb, func() {
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
	if s.queued(n)+n.delayed >= int(n.f("capacity")) {
		s.failAt(n, RQueueFull, cb)
		return
	}
	j := &job{rid: c.rid, key: c.key, wl: c.wl, idem: c.idem, counted: c.counted, enq: s.now, high: c.high}
	if j.counted {
		s.jobEnqueued++
	}
	if d := n.f("delayMs"); d > 0 {
		n.delayed++
		s.after(d, func() {
			n.delayed--
			s.requeue(n, j)
			s.dispatch(n)
		})
	} else {
		s.requeue(n, j)
	}
	s.after(0.2, func() {
		s.okAt(n, Result{Ver: -1, Done: true}, cb)
		s.dispatch(n)
	})
}

// queued is the number of jobs waiting for a worker.
func (s *Sim) queued(q *node) int {
	if q.kind == KLog {
		n := 0
		for _, l := range q.lanes {
			n += len(l.jobs) - l.head
		}
		return n
	}
	if q.kind == KTopic {
		n := 0
		for _, e := range q.out {
			n += len(e.subJobs) - e.subHead
		}
		return n
	}
	return len(q.jobs) - q.jh + len(q.hjobs) - q.hh
}

// requeue puts a job at the back of its line: the urgent line of a priority queue, or the ordinary one.
func (s *Sim) requeue(q *node, j *job) {
	if j.sub != nil {
		j.sub.subJobs = append(j.sub.subJobs, j)
		return
	}
	if j.high && q.b("priorityQueue") {
		q.hjobs = append(q.hjobs, j)
		return
	}
	q.jobs = append(q.jobs, j)
}

func (s *Sim) dispatch(q *node) {
	if q.kind == KTopic {
		s.dispatchTopic(q)
		return
	}
	if q.kind == KLog {
		s.dispatchLog(q)
		return
	}
	for s.queued(q) > 0 && !q.down {
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
		var j *job
		if q.hh < len(q.hjobs) {
			j = q.hjobs[q.hh]
			q.hjobs[q.hh] = nil
			q.hh++
		} else {
			j = q.jobs[q.jh]
			q.jobs[q.jh] = nil
			q.jh++
		}
		s.runJob(q, w, j)
	}
	if q.hh > 256 && q.hh*2 > len(q.hjobs) {
		q.hjobs = append([]*job(nil), q.hjobs[q.hh:]...)
		q.hh = 0
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
	j.finished = false
	ep := w.epoch
	if vis := q.f("visibilityTimeoutMs"); vis > 0 && !j.ghost {
		s.after(vis, func() {
			if j.finished || q.removed {
				return
			}
			// not acknowledged in time: the queue offers the job again; this copy still runs, and so does the new one
			if j.counted {
				s.jobRedelivered++
			}
			s.requeue(q, &job{rid: j.rid, key: j.key, wl: j.wl, idem: j.idem, counted: j.counted, enq: j.enq, attempts: j.attempts, ghost: true, high: j.high})
			s.dispatch(q)
		})
	}
	fin := func(res Result) {
		s.markBusy(w)
		w.busy--
		if j.ghost {
			if res.OK {
				w.c.ok++
				w.c.done++
			} else {
				w.c.err++
				w.c.reason[res.Reason]++
			}
			s.dispatch(q)
			return
		}
		j.finished = true
		if res.OK && j.lane != nil {
			j.lane.running = false
			s.orderCheck(q, j)
		}
		if res.OK {
			w.c.ok++
			w.c.done++
			if j.counted {
				s.jobCompleted++
				s.jobHist.Add(s.now - j.enq)
				if j.high {
					s.jobHistHigh.Add(s.now - j.enq)
				}
				s.jobsW.Push(s.now, true, s.now-j.enq)
			}
		} else {
			w.c.err++
			w.c.reason[res.Reason]++
			if j.attempts < int(q.f("maxAttempts")) {
				if j.counted {
					s.jobRetried++
				}
				s.after(100*math.Pow(2, float64(j.attempts-1)), func() {
					if j.lane != nil { // retried in place: the partition waited, so later messages did not overtake it
						j.lane.jobs = append([]*job{j}, j.lane.jobs[j.lane.head:]...)
						j.lane.head = 0
						j.lane.running = false
					} else {
						s.requeue(q, j)
					}
					s.dispatch(q)
				})
			} else if j.lane != nil {
				j.lane.running = false
				if j.counted {
					if q.b("deadLetter") {
						s.jobDead++
					} else {
						s.jobLost++
					}
				}
			} else if j.counted {
				if q.b("deadLetter") {
					s.jobDead++
				} else {
					s.jobLost++
				}
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
		if pct := w.f("poisonPct"); pct > 0 && float64(Hash32(uint32(j.rid)*2654435761+211)%100) < pct {
			fin(fail(RPoisonJob)) // a malformed job fails the same way every time
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

// ---- topic (publish/subscribe) -------------------------------------------------------

// arriveTopic gives every subscriber (each outgoing edge to a worker) its own copy of the message. The publisher
// is acknowledged at once: it never waits for, or hears about, a slow subscriber. A subscriber whose backlog is
// full loses its copy (counted as a lost delivery).
func (s *Sim) arriveTopic(n *node, c *call, cb func(Result)) {
	if !c.write {
		s.failAt(n, RNoRoute, cb)
		return
	}
	for _, e := range n.out {
		if e.removed || e.to.kind != KWorker || e.to.removed {
			continue
		}
		j := &job{rid: c.rid, key: c.key, wl: c.wl, idem: c.idem, counted: c.counted, enq: s.now, sub: e}
		if j.counted {
			s.jobEnqueued++
		}
		if len(e.subJobs)-e.subHead >= int(n.f("capacity")) {
			if j.counted {
				s.jobLost++
			}
			continue
		}
		e.subJobs = append(e.subJobs, j)
	}
	s.after(0.2, func() {
		s.okAt(n, Result{Ver: -1, Done: true}, cb)
		s.dispatch(n)
	})
}

// dispatchTopic hands each subscriber's deliveries to its own worker, in order, as the worker has room.
func (s *Sim) dispatchTopic(q *node) {
	for _, e := range q.out {
		w := e.to
		if e.removed || w.kind != KWorker || w.down || w.removed {
			continue
		}
		for e.subHead < len(e.subJobs) && w.busy < s.capacity(w) && !q.down {
			j := e.subJobs[e.subHead]
			e.subJobs[e.subHead] = nil
			e.subHead++
			s.runJob(q, w, j)
		}
		if e.subHead > 256 && e.subHead*2 > len(e.subJobs) {
			e.subJobs = append([]*job(nil), e.subJobs[e.subHead:]...)
			e.subHead = 0
		}
	}
}

// ---- partitioned log -----------------------------------------------------------------

// lane is one partition as seen by one consumer group: an ordered line of messages that one member works through
// one at a time.
type lane struct {
	part, group int
	jobs        []*job
	head        int
	running     bool
}

// logGroups lists the consumer groups reading a log (the distinct group numbers of its workers), smallest first.
func (s *Sim) logGroups(q *node) []int {
	var gs []int
	seen := map[int]bool{}
	for _, e := range q.out {
		if e.removed || e.to.kind != KWorker || e.to.removed {
			continue
		}
		g := int(e.to.f("group"))
		if !seen[g] {
			seen[g] = true
			gs = append(gs, g)
		}
	}
	for i := 1; i < len(gs); i++ { // tiny insertion sort: determinism without importing sort here
		for k := i; k > 0 && gs[k] < gs[k-1]; k-- {
			gs[k], gs[k-1] = gs[k-1], gs[k]
		}
	}
	return gs
}

// laneOwner is the member of the lane's group that reads its partition: the group's running members take the
// partitions in turn.
func (s *Sim) laneOwner(q *node, l *lane) *node {
	var members []*node
	for _, e := range q.out {
		if e.removed || e.to.kind != KWorker || e.to.removed || e.to.down || int(e.to.f("group")) != l.group {
			continue
		}
		members = append(members, e.to)
	}
	if len(members) == 0 {
		return nil
	}
	return members[l.part%len(members)]
}

// workerLag is the number of messages waiting in the partitions a worker reads.
func (s *Sim) workerLag(q, w *node) int {
	n := 0
	for _, l := range q.lanes {
		if s.laneOwner(q, l) == w {
			n += len(l.jobs) - l.head
		}
	}
	return n
}

func (s *Sim) arriveLog(n *node, c *call, cb func(Result)) {
	if !c.write {
		s.failAt(n, RNoRoute, cb)
		return
	}
	parts := int(n.f("partitions"))
	p := int(Hash32(uint32(c.key)*2654435761+7) % uint32(parts))
	if n.s("partitionBy") == "roundRobin" {
		p = n.rrNext % parts
		n.rrNext++
	}
	s.logSeq++
	seq := s.logSeq
	if n.laneIdx == nil {
		n.laneIdx = map[[2]int]*lane{}
	}
	for _, g := range s.logGroups(n) {
		l := n.laneIdx[[2]int{p, g}]
		if l == nil {
			l = &lane{part: p, group: g}
			n.laneIdx[[2]int{p, g}] = l
			n.lanes = append(n.lanes, l)
		}
		j := &job{rid: c.rid, key: c.key, wl: c.wl, idem: c.idem, counted: c.counted, enq: s.now, lane: l, seq: seq}
		if j.counted {
			s.jobEnqueued++
		}
		if len(l.jobs)-l.head >= int(n.f("capacity")) {
			if j.counted {
				s.jobLost++
			}
			continue
		}
		l.jobs = append(l.jobs, j)
	}
	s.after(0.2, func() {
		s.okAt(n, Result{Ver: -1, Done: true}, cb)
		s.dispatch(n)
	})
}

func (s *Sim) dispatchLog(q *node) {
	for _, l := range q.lanes {
		if l.running || l.head >= len(l.jobs) || q.down {
			continue
		}
		w := s.laneOwner(q, l)
		if w == nil || w.busy >= s.capacity(w) {
			continue
		}
		j := l.jobs[l.head]
		l.jobs[l.head] = nil
		l.head++
		l.running = true
		s.runJob(q, w, j)
		if l.head > 256 && l.head*2 > len(l.jobs) {
			l.jobs = append([]*job(nil), l.jobs[l.head:]...)
			l.head = 0
		}
	}
}

// orderCheck counts a message that a group's consumer finished after a newer message of the same key.
func (s *Sim) orderCheck(q *node, j *job) {
	if q.lastSeq == nil {
		q.lastSeq = map[[2]int]int{}
	}
	k := [2]int{j.lane.group, j.key}
	if j.seq < q.lastSeq[k] {
		if j.counted {
			s.jobOrderViolations++
		}
		return
	}
	q.lastSeq[k] = j.seq
}

// heavyMult is how many times a heavy request costs what a light one does (1 for a light request).
func (s *Sim) heavyMult(c *call) float64 {
	if !c.heavy || c.lg == nil {
		return 1
	}
	return c.lg.client.f("heavyMult")
}

func (s *Sim) classMult(c *call, n *node) float64 { return s.heavyMult(c) }
