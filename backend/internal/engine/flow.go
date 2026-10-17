package engine

import (
	"math"
	"slices"
	"sort"
)

// Result is the outcome of one call travelling back up the chain.
type Result struct {
	OK     bool
	Reason Reason
	Ver    int  // item version served, -1 when not applicable
	Hit    bool // served from a cache
	Done   bool // data was resolved (hit, or read-through fetched it)
	Src    *node
}

type cmd int

const (
	cmdNone cmd = iota
	cmdRead
	cmdWrite
	cmdCacheGet
	cmdCacheSet
	cmdCacheInv
	cmdLock
	cmdUnlock
	cmdGeo
	cmdRank
	cmdScore
	cmdEnqueue
	cmdExternal
)

type call struct {
	lg      *logical
	at      *attempt
	write   bool
	key     int
	wl      int
	hash    uint32
	rid     int
	idem    bool
	counted bool
	ticket  *ticket
	cmd     cmd
	ver     int
	ttl     float64
	token   int
	job     *job
}

func fail(r Reason) Result { return Result{Reason: r, Ver: -1} }

// ---- hops -----------------------------------------------------------------------

// hop sends c over edge e to its target and delivers the response to cb after the round trip.
func (s *Sim) hop(from *node, e *edge, c *call, cb func(Result)) {
	s.hopVia(e, nil, e.to, e.latency, c, cb)
}

func (s *Sim) hopVia(e, via *edge, target *node, oneWay float64, c *call, cb func(Result)) {
	if e.removed {
		cb(fail(RNodeDown))
		return
	}
	e.fwd++
	s.emit(e, false, false)
	if via != nil {
		via.fwd++
	}
	s.after(oneWay, func() {
		s.arrive(target, c, func(res Result) {
			s.after(oneWay, func() {
				s.emit(e, true, !res.OK)
				cb(res)
			})
		})
	})
}

// resolveTarget looks through a shard router to the node that really serves key/cmd.
func (s *Sim) resolveTarget(e *edge, c *call, cm cmd) (*node, float64, *edge) {
	t := e.to
	if t.kind != KShardRouter {
		return t, e.latency, nil
	}
	wantRedis := cm == cmdLock || cmdIsRedis(cm)
	var cand []*edge
	for _, x := range t.out {
		if x.removed {
			continue
		}
		if (x.to.kind == KRedis) == wantRedis {
			cand = append(cand, x)
		}
	}
	if len(cand) == 0 {
		return nil, 0, nil
	}
	x := cand[int(Hash32(uint32(c.key)+0x9e3779b9)%uint32(len(cand)))]
	t.c.arr++
	t.c.ok++
	return x.to, e.latency + x.latency + 0.2, x
}

func cmdIsRedis(c cmd) bool {
	return c == cmdLock || c == cmdUnlock || c == cmdGeo || c == cmdRank || c == cmdScore
}

// callCmd runs a command against edge e from node `from`, applying breaker, timeout and pooling.
func (s *Sim) callCmd(from *node, e *edge, c *call, cm cmd, ver int, cb func(Result)) {
	cc := *c
	cc.cmd, cc.ver, cc.ticket = cm, ver, nil
	switch cm {
	case cmdWrite, cmdCacheInv, cmdCacheSet, cmdScore, cmdEnqueue, cmdUnlock, cmdExternal:
		cc.write = true
	case cmdRead, cmdCacheGet, cmdGeo, cmdRank:
		cc.write = false
	}
	s.callDown(from, e, &cc, cm, cb)
}

func (s *Sim) callDown(from *node, e *edge, c *call, cm cmd, cb func(Result)) {
	t, lat, via := s.resolveTarget(e, c, cm)
	if t == nil {
		cb(fail(RNoRoute))
		return
	}
	app := from.kind == KAppServer
	useBreaker := app && from.b("circuitBreaker")
	if useBreaker {
		switch s.breakerState(e) {
		case 1:
			cb(fail(RCircuitOpen))
			return
		case 2:
			if e.trial {
				cb(fail(RCircuitOpen))
				return
			}
			e.trial = true
		}
	}
	settled := false
	finish := func(res Result) {
		if settled {
			return
		}
		settled = true
		if useBreaker {
			s.breakerResult(e, res.OK || res.Reason == RLockBusy)
		}
		cb(res)
	}
	if app {
		if ct := from.f("callTimeoutMs"); ct > 0 {
			s.after(ct, func() { finish(fail(RCallTimeout)) })
		}
	}
	send := func(tk *ticket) {
		cc := *c
		cc.ticket = tk
		s.hopVia(e, via, t, lat, &cc, func(res Result) {
			if tk != nil {
				s.releaseTicket(tk)
			}
			finish(res)
		})
	}
	if t.kind == KDatabase || t.kind == KReplica {
		p := s.poolOf(from, t)
		if tk := s.acquire(p); tk != nil {
			send(tk)
		} else {
			p.wait = append(p.wait, &waiter{send: send, fail: func() { finish(fail(RNodeDown)) }, dead: func() bool { return settled }})
		}
		return
	}
	send(nil)
}

// ---- circuit breaker ---------------------------------------------------------------

func (s *Sim) breakerState(e *edge) int {
	if e.brState == 1 && s.now >= e.brUntil {
		e.brState, e.trial = 2, false
	}
	return e.brState
}

func (s *Sim) breakerResult(e *edge, ok bool) {
	switch s.breakerState(e) {
	case 2:
		if !e.trial {
			return
		}
		e.trial = false
		if ok {
			e.brState, e.brFails = 0, 0
		} else {
			s.openBreaker(e)
		}
	case 0:
		if ok {
			e.brFails = 0
		} else if e.brFails++; e.brFails >= breakerFail {
			s.openBreaker(e)
		}
	}
}

func (s *Sim) openBreaker(e *edge) {
	e.brState, e.brUntil, e.brFails, e.trial = 1, s.now+breakerOpen, 0, false
}

// ---- connection pools --------------------------------------------------------------

type ticket struct {
	pool     *pool
	opening  bool
	released bool
}

type waiter struct {
	send func(*ticket)
	fail func()
	dead func() bool
}

type pool struct {
	owner, db *node
	inUse     int
	opened    int
	conf      int
	wait      []*waiter
	wh        int
	dead      bool
}

func (s *Sim) poolSize(owner *node) int {
	switch owner.kind {
	case KWorker:
		return int(math.Max(1, owner.f("concurrency")))
	case KCache:
		return 64 // a read-through cache keeps its own generous pool
	}
	return int(math.Max(1, owner.f("dbPool")))
}

func (s *Sim) poolOf(owner, db *node) *pool {
	p := owner.pools[db]
	if p == nil {
		p = &pool{owner: owner, db: db}
		owner.pools[db] = p
	}
	return p
}

func (s *Sim) acquire(p *pool) *ticket {
	n := s.poolSize(p.owner)
	if p.inUse < min(p.opened, n) {
		p.inUse++
		return &ticket{pool: p}
	}
	if p.opened < n {
		p.opened++
		p.inUse++
		return &ticket{pool: p, opening: true}
	}
	return nil
}

func (s *Sim) releaseTicket(t *ticket) {
	if t.released {
		return
	}
	t.released = true
	p := t.pool
	p.inUse--
	if t.opening {
		t.opening = false
		p.opened--
	}
	if !p.dead {
		s.trimPool(p)
		s.pumpPool(p)
	}
}

func (s *Sim) pumpPool(p *pool) {
	for p.wh < len(p.wait) {
		w := p.wait[p.wh]
		if w.dead() {
			p.wh++
			continue
		}
		tk := s.acquire(p)
		if tk == nil {
			break
		}
		p.wh++
		w.send(tk)
	}
	if p.wh > 256 && p.wh*2 > len(p.wait) {
		p.wait = append([]*waiter(nil), p.wait[p.wh:]...)
		p.wh = 0
	}
}

func (s *Sim) pumpAll(n *node) {
	for _, p := range n.pools {
		if !p.dead {
			s.trimPool(p)
			s.pumpPool(p)
		}
	}
}

func (s *Sim) poolWaiting(n *node) int {
	c := 0
	for _, p := range n.pools {
		for i := p.wh; i < len(p.wait); i++ {
			if !p.wait[i].dead() {
				c++
			}
		}
	}
	return c
}

// connect is called when a call that just opened a pooled connection reaches the database.
func (s *Sim) connect(db *node, tk *ticket) bool {
	if tk == nil || !tk.opening || tk.released {
		return true
	}
	tk.opening = false
	p := tk.pool
	if db.down || p.dead || p.db != db {
		p.opened--
		return true
	}
	if float64(db.openConns) >= db.f("maxConnections") {
		p.opened--
		return false
	}
	db.openConns++
	p.conf++
	return true
}

func (s *Sim) trimPool(p *pool) {
	t := min(p.opened-s.poolSize(p.owner), p.opened-p.inUse, p.conf)
	if t <= 0 {
		return
	}
	p.opened -= t
	p.conf -= t
	p.db.openConns = max(0, p.db.openConns-t)
}

func (s *Sim) closePools(owner *node) {
	for _, p := range owner.pools {
		p.dead = true
		p.db.openConns = max(0, p.db.openConns-p.conf)
		s.failWaiters(p)
	}
	owner.pools = map[*node]*pool{}
}

func (s *Sim) prunePools(owner *node) {
	if len(owner.pools) == 0 {
		return
	}
	keep := map[*node]bool{}
	for _, e := range owner.out {
		switch e.to.kind {
		case KDatabase, KReplica:
			keep[e.to] = true
		case KShardRouter:
			for _, x := range e.to.out {
				if x.to.kind == KDatabase {
					keep[x.to] = true
				}
			}
		}
	}
	for db, p := range owner.pools {
		if !keep[db] {
			p.dead = true
			db.openConns = max(0, db.openConns-p.conf)
			delete(owner.pools, db)
			s.failWaiters(p)
		}
	}
}

func (s *Sim) failWaiters(p *pool) {
	w := p.wait[p.wh:]
	p.wait, p.wh = nil, 0
	for _, x := range w {
		if !x.dead() {
			x.fail()
		}
	}
}

// dropConns forgets all connections to a database that went away or restarted.
func (s *Sim) dropConns(db *node) {
	if db.kind != KDatabase && db.kind != KReplica {
		return
	}
	for _, o := range s.order {
		if p := o.pools[db]; p != nil {
			p.opened -= p.conf
			p.conf = 0
		}
	}
	db.openConns = 0
}

// ---- stations (capacity + queue) -----------------------------------------------------

func (s *Sim) capacity(n *node) int {
	switch n.kind {
	case KAppServer:
		return int(n.f("threads"))
	case KDatabase, KReplica:
		return int(n.f("connections"))
	case KWorker:
		return int(n.f("concurrency"))
	case KRedis:
		return 1
	case KCache:
		return 10000
	}
	return 1 << 30
}

func (s *Sim) queueCap(n *node) int {
	switch n.kind {
	case KAppServer, KDatabase, KReplica, KRedis:
		return int(n.f("queueCap"))
	case KCache:
		return 2000
	}
	return 0
}

func (s *Sim) markBusy(n *node) {
	n.busyArea += float64(n.busy) * (s.now - n.lastBusy)
	n.lastBusy = s.now
}

func (s *Sim) releaser(n *node) func() {
	done := false
	return func() {
		if done {
			return
		}
		done = true
		s.markBusy(n)
		n.busy--
		s.startNext(n)
	}
}

func (s *Sim) submit(n *node, c *call, cb func(Result), run func(release func())) {
	if n.busy < s.capacity(n) {
		s.markBusy(n)
		n.busy++
		run(s.releaser(n))
		return
	}
	if len(n.q)-n.qh < s.queueCap(n) {
		n.q = append(n.q, &task{c: c, cb: cb, run: run, enq: s.now})
		return
	}
	s.failAt(n, RQueueFull, cb)
}

func (s *Sim) startNext(n *node) {
	for !n.down && n.busy < s.capacity(n) && n.qh < len(n.q) {
		var t *task
		if n.kind == KAppServer && n.s("queueOrder") == "lifo" {
			t = n.q[len(n.q)-1]
			n.q = n.q[:len(n.q)-1]
		} else {
			t = n.q[n.qh]
			n.q[n.qh] = nil
			n.qh++
		}
		if n.qh > 256 && n.qh*2 > len(n.q) {
			n.q = append([]*task(nil), n.q[n.qh:]...)
			n.qh = 0
		}
		if qt := n.f("queueTimeoutMs"); n.kind == KAppServer && qt > 0 && s.now-t.enq > qt {
			s.failAt(n, RQueueTimeout, t.cb)
			continue
		}
		s.markBusy(n)
		n.busy++
		t.run(s.releaser(n))
	}
}

func (s *Sim) failQueued(n *node, r Reason) {
	q := n.q[n.qh:]
	n.q, n.qh = nil, 0
	for _, t := range q {
		if t != nil {
			s.failAt(n, r, t.cb)
		}
	}
}

func (s *Sim) failAt(n *node, r Reason, cb func(Result)) {
	n.c.err++
	n.c.reason[r]++
	if r == RQueueFull {
		n.c.qfull++
	}
	cb(fail(r))
}

func (s *Sim) okAt(n *node, res Result, cb func(Result)) {
	n.c.ok++
	res.OK = true
	cb(res)
}

// passBack records a pass-through node's outcome and forwards the result.
func (s *Sim) passBack(n *node, res Result, cb func(Result)) {
	if res.OK {
		n.c.ok++
	} else {
		n.c.err++
		n.c.reason[res.Reason]++
	}
	cb(res)
}

// ---- arrival -----------------------------------------------------------------------

func (s *Sim) arrive(n *node, c *call, cb func(Result)) {
	if n.removed {
		cb(fail(RNodeDown))
		return
	}
	n.c.arr++
	if (n.kind == KDatabase || n.kind == KReplica) && c.ticket != nil && !s.connect(n, c.ticket) {
		s.failAt(n, RConnRefused, cb)
		return
	}
	if n.down {
		s.failAt(n, RNodeDown, cb)
		return
	}
	switch n.kind {
	case KQueue:
		s.arriveQueue(n, c, cb)
		return
	case KClient, KWorker:
		s.failAt(n, RNoRoute, cb)
		return
	case KReplica:
		if c.cmd == cmdWrite {
			s.failAt(n, RNoRoute, cb)
			return
		}
	case KRedis, KCache:
		if c.cmd == cmdNone {
			s.failAt(n, RNoRoute, cb)
			return
		}
	case KRateLimiter:
		if !s.takeToken(n) {
			s.failAt(n, RRateLimited, cb)
			return
		}
		n.c.adm++
	}
	s.submit(n, c, cb, func(release func()) { s.process(n, c, cb, release) })
}

func (s *Sim) takeToken(n *node) bool {
	n.tokens = math.Min(n.f("burst"), n.tokens+(s.now-n.tokensAt)*n.f("limitRps")/1000)
	n.tokensAt = s.now
	if n.tokens < 1 {
		return false
	}
	n.tokens--
	return true
}

// hold schedules fn after ms of service, failing the call if the node crashed meanwhile.
func (s *Sim) hold(n *node, ms float64, release func(), cb func(Result), fn func()) {
	ep := n.epoch
	s.after(ms, func() {
		if n.epoch != ep || n.removed {
			release()
			s.failAt(n, RNodeDown, cb)
			return
		}
		fn()
	})
}

func (s *Sim) process(n *node, c *call, cb func(Result), release func()) {
	switch n.kind {
	case KLoadBalancer, KRateLimiter:
		s.procRoute(n, c, cb, release)
	case KCDN:
		s.procCDN(n, c, cb, release)
	case KAppServer:
		s.procApp(n, c, cb, release)
	case KCache:
		s.procCache(n, c, cb, release)
	case KDatabase, KReplica:
		s.procDB(n, c, cb, release)
	case KRedis:
		s.procRedis(n, c, cb, release)
	case KExternalAPI:
		s.hold(n, s.rng.Service(n.f("latencyMs"))*n.slow, release, cb, func() {
			release()
			if s.rng.Next() < n.f("errorPct")/100 {
				s.failAt(n, RDependency, cb)
				return
			}
			s.okAt(n, Result{Ver: -1, Done: true}, cb)
		})
	default:
		release()
		s.failAt(n, RNoRoute, cb)
	}
}

// ---- load balancer / rate limiter -----------------------------------------------------

func (s *Sim) believedDown(lb, t *node) bool {
	if !t.down {
		return false
	}
	wait := 0.0
	if lb.kind == KLoadBalancer {
		wait = lb.f("healthCheckSec") * 1000
	}
	return s.now-t.downAt >= wait
}

func (s *Sim) pickBackend(n *node) *edge {
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
	s.hold(n, 0.2*n.slow, release, cb, func() {
		release()
		e := s.pickBackend(n)
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

// ---- CDN -----------------------------------------------------------------------------

func (s *Sim) procCDN(n *node, c *call, cb func(Result), release func()) {
	ms := 0.2
	if !c.write {
		ms = s.rng.Service(1)
	}
	s.hold(n, ms*n.slow, release, cb, func() {
		release()
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
			n.store.Delete(c.key)
		}
		e := s.pickBackend(n)
		if e == nil {
			s.failAt(n, RNoBackend, cb)
			return
		}
		s.hop(n, e, c, func(res Result) {
			if res.OK && !c.write && res.Ver >= 0 {
				n.c.evict += n.store.Set(s.now, c.key, res.Ver)
			}
			s.passBack(n, res, cb)
		})
	})
}
