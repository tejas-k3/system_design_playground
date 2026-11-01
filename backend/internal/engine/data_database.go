package engine

import "math"

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

// diskReadMult is how much slower a page read from disk is than one found in memory.
const diskReadMult = 10

// lsmExtraLevels is how many levels beyond the first an LSM read must look in. Each level holds about ten times the one
// above, so the count grows smoothly with the log of the data size: none up to 10 GB (and with no size set, 0), one at
// 100 GB, two at 1 TB.
func lsmExtraLevels(dataGB float64) float64 {
	if dataGB <= 10 {
		return 0
	}
	return math.Log10(dataGB) - 1
}

func (s *Sim) readMult(n *node, c *call) float64 {
	m := s.readMultBase(n, c)
	if n.s("engine") == "lsm" && n.kind == KDatabase {
		// a read may have to look in several levels: 1.5x the read, and half a read more for each extra level (a scan is not charged per level)
		m = m*1.5 + 0.5*lsmExtraLevels(n.f("dataGB"))
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

// rangeRead says whether this read returns a range of rows rather than one (chosen from the request id, so no random draw).
func (s *Sim) rangeRead(c *call) bool {
	if c.lg == nil || c.write {
		return false
	}
	pct := c.lg.client.f("rangePct")
	return pct > 0 && float64(Hash32(uint32(c.rid)*2654435761+193)%100) < pct
}

// wideScanMult is how many times worse than a plain scan it is to scan every partition of a wide-column store.
const wideScanMult = 3

// Per-row time of reading a range through an index, and of reading it from a layout kept in time order.
const (
	indexRangeRowMs  = 0.01
	seriesRangeRowMs = 0.003
)

// rangeMult is the cost of a range read as a multiple of the read time: a time-series store reads a time range in
// order; a relational or document store needs an index on the field (rows x a small per-row time), else it scans
// everything; a wide-column store has no access path but its partition key, so it scans every partition.
func (s *Sim) rangeMult(n *node, c *call) float64 {
	rows := c.lg.client.f("rangeRows")
	readMs := max(n.f("readMs"), 0.001)
	switch n.s("model") {
	case "timeSeries":
		return 1 + rows*seriesRangeRowMs/readMs
	case "wideColumn":
		return n.f("scanMult") * wideScanMult
	}
	m := 1 + rows*indexRangeRowMs/readMs
	if n.s("model") == "document" {
		m *= 0.8 // single-document reads are cheaper in a document store
	}
	if n.f("indexes") < 1 { // a scan reads at least the rows an index would have found
		return max(n.f("scanMult"), m)
	}
	return m
}

func (s *Sim) readMultBase(n *node, c *call) float64 {
	if s.rangeRead(c) && c.wl == wlKV {
		return s.rangeMult(n, c)
	}
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
			m *= wideScanMult // no secondary access path at all: the scan is over every partition
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
	ms += s.graphMs(n, c)
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
			start := max(s.now, n.diskFree)
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
