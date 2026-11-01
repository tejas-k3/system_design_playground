package engine

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
	cpu := s.service(n, n.f("cpuMs")*s.classMult(c, n)+s.securityMs(n, c)+s.rankingMs(n, c)) * n.slow
	cold := s.coldStartWait(n) // waiting for a function to start is not CPU work
	n.cpuWork += cpu
	if n.throttled {
		cpu *= 100 / n.f("burstBaselinePct")
	}
	s.after(cpu+cold, func() {
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
		ae := s.first(n, KAuth)
		asks := ae != nil && n.s("authMode") == "service"
		signedIn := cc.user != 0 && n.keepsLoginSessions()
		if !asks && !signedIn { // the common case: nothing to check, and no closure to allocate
			s.proceed(n, &cc, run, done)
			return
		}
		s.checkCaller(n, ae, asks, &cc, done, func() { s.proceed(n, &cc, run, done) })
	})
}

// checkCaller runs the token check, then the session lookup, and continues with next if both let the request through.
func (s *Sim) checkCaller(n *node, ae *edge, asks bool, c *call, done func(Result), next func()) {
	session := func() {
		s.sessionGate(n, c, func(res Result) {
			if !res.OK {
				done(res)
				return
			}
			next()
		})
	}
	if !asks {
		session()
		return
	}
	s.authenticate(n, ae, c, func(res Result) {
		if !res.OK {
			done(res)
			return
		}
		session()
	})
}

// proceed is what a request does once the caller is known: retrieval and the model, a claim check, a call to an external
// service, or the read or write itself.
func (s *Sim) proceed(n *node, cc *call, run func(), done func(Result)) {
	if ve := s.first(n, KVectorStore); ve != nil { // retrieval first; the answer, if there is a model, is written from what it found
		s.callCmd(n, ve, cc, cmdRead, 0, func(res Result) {
			if !res.OK {
				done(res)
				return
			}
			if le := s.first(n, KLLMServer); le != nil {
				s.callCmd(n, le, cc, cmdEnqueue, 0, done)
				return
			}
			done(res)
		})
		return
	}
	if le := s.first(n, KLLMServer); le != nil { // the answer is written by the LLM server
		s.callCmd(n, le, cc, cmdEnqueue, 0, done)
		return
	}
	// claim check: a write with a body goes to the object store first; from then on only a pointer travels
	if oe := s.first(n, KObjectStore); oe != nil && cc.write && cc.lg != nil && cc.lg.client.f("reqKB") > 0 {
		s.callCmd(n, oe, cc, cmdPut, 0, func(res Result) {
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
			s.callCmd(n, ee, cc, cmdExternal, 0, func(res Result) {
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
			if m := n.s("workflow"); isWorkflowMode(m) && s.workflowWrite(n, c, m, done) {
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

// ---- cache --------------------------------------------------------------------------

// ---- database -----------------------------------------------------------------------

// ---- redis ----------------------------------------------------------------------------
