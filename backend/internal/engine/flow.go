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
	Fence  int // a lock service: the fencing token that came with the lock (0 = none)
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
	cmdPut // upload a body to an object store
	cmdGet // download a body from an object store
)

type call struct {
	lg       *logical
	at       *attempt
	write    bool
	key      int
	wl       int
	hash     uint32
	tok      int32 // tokens the request carries (0 = counts as one unit); packed beside hash to keep the call small
	rid      int
	idem     bool
	counted  bool
	ticket   *ticket
	cmd      cmd
	ver      int
	ttl      float64
	token    int
	fence    int     // the fencing token the caller holds from its lock (0 = none)
	readX    float64 // a read of a counter split over several rows costs this many times as much
	blind    bool    // a holder that froze with its lock writes on what it saw before the freeze, without checking again
	job      *job
	fan      float64 // extra database work this request carries: timeline inserts or followed authors read (feed workload)
	high     bool    // an urgent request
	heavy    bool    // an expensive request: it costs heavyMult times the CPU and database time
	deadline float64 // when the client gives up (absolute ms, 0 = unknown)
	honor    bool    // an app server on the path propagates the deadline: stations skip this call once it has passed
	user     int     // the account behind the request (0 = not modelled)
	claim    bool    // the body already sits in an object store: what travels on is a pointer
	tr       *tracer // non-nil for sampled requests
	sp       *span   // the span of the node currently handling this call
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
	if e.from.zone != e.to.zone && e.from.kind != KClient && e.to.kind != KClient {
		oneWay += crossZoneMs // a hop between availability zones, each way
	}
	oneWay += regionMs(e.from.region, e.to.region) // and the distance between regions
	req, resp := s.sizes(c)
	fwd := oneWay + s.transmit(e, 0, req)
	if via != nil {
		fwd += s.transmit(via, 0, req)
	}
	extra, lost := s.linkFault(e)
	fwd += extra
	if lost { // six packets in a row: the connection is aborted and the caller hears nothing from the far end
		s.after(fwd, func() {
			s.emit(e, true, true)
			cb(fail(RConnLost))
		})
		return
	}
	s.after(fwd, func() {
		if e.downReq { // the request is stuck on the link until it heals
			e.held = append(e.held, func() { s.hopArrive(e, via, target, oneWay, resp, c, cb) })
			return
		}
		s.hopArrive(e, via, target, oneWay, resp, c, cb)
	})
}

func (s *Sim) hopArrive(e, via *edge, target *node, oneWay float64, resp float64, c *call, cb func(Result)) {
	{
		s.arrive(target, c, func(res Result) {
			back := oneWay + s.transmit(e, 1, resp)
			if via != nil {
				back += s.transmit(via, 1, resp)
			}
			extra, lost := s.linkFault(e)
			back += extra
			if lost { // the response never makes it back
				res = fail(RConnLost)
			}
			s.after(back, func() {
				deliver := func() {
					s.emit(e, true, !res.OK)
					cb(res)
				}
				if e.downResp { // the answer is stuck on the link until it heals; the caller may long since have given up
					e.held = append(e.held, deliver)
					return
				}
				deliver()
			})
		})
	}
}

// regionMs is the one-way latency between two regions (0 inside one); an unset region is the default, us.
func regionMs(a, b string) float64 {
	if a == b || a == "" && b == "us" || a == "us" && b == "" {
		return 0
	}
	if a == "" {
		a = "us"
	}
	if b == "" {
		b = "us"
	}
	if a > b {
		a, b = b, a
	}
	switch a + "-" + b {
	case "eu-us":
		return 45
	case "ap-us":
		return 75
	case "ap-eu":
		return 100
	}
	return 0
}

// crossZoneMs is the extra one-way latency of a call between two availability zones of a region.
const crossZoneMs = 1.5

// retransmitMs is how long a lost packet waits before it is sent again (a typical minimum retransmission timeout).
const retransmitMs = 200

// linkFault draws the extra delay of one trip over e: a random jitter, and a retransmission for every packet lost
// before one gets through; six losses in a row abort the connection. A perfect link (the default) draws nothing.
func (s *Sim) linkFault(e *edge) (extra float64, lost bool) {
	if e.loss <= 0 && e.jitter <= 0 {
		return 0, false
	}
	if e.jitter > 0 {
		extra += s.chaosRng.Next() * e.jitter
	}
	if e.loss > 0 {
		for k := 0; s.chaosRng.Next()*100 < e.loss; k++ {
			if k == 5 {
				return extra, true
			}
			extra += retransmitMs
		}
	}
	return extra, false
}

// sizes are the bytes of one request and one response, from the users who sent it. 0 means not modelled.
func (s *Sim) sizes(c *call) (req, resp float64) {
	if c.lg == nil {
		return 0, 0
	}
	return c.lg.client.f("reqKB") * 1000, c.lg.client.f("respKB") * 1000
}

// transmit puts bytes on a link in one direction and returns how long until they have all arrived: the wait for
// the bytes ahead of them plus their own transmission time. A link with no limit, or no bytes, costs nothing.
func (s *Sim) transmit(e *edge, dir int, bytes float64) float64 {
	if e.bw <= 0 || bytes <= 0 {
		return 0
	}
	dur := bytes * 8 / (e.bw * 1000) // bits / (Mbit/s x 1e3 bits per ms)
	start := math.Max(s.now, e.txFree[dir])
	e.txFree[dir] = start + dur
	// the bits count as carried when they start crossing, so a backlog waiting behind a saturated link is not counted early
	if wait := start - s.now; wait > 0 {
		bits := bytes * 8
		s.after(wait, func() { e.bits += bits })
	} else {
		e.bits += bytes * 8
	}
	return start - s.now + dur
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
	h := uint32(c.key) + 0x9e3779b9
	if t.s("shardBy") == "request" {
		h = c.hash*2654435761 + 0x9e3779b9 // by whoever sent it: one hot key no longer lands on one shard
	}
	x := s.pickShard(t, cand, h)
	t.c.arr++
	t.c.ok++
	lat := e.latency + x.latency + 0.2
	if s.now < t.moveUntil && t.movedKeys != nil && t.movedKeys[c.key%reshardSample] {
		lat += t.f("rebalanceMs") // this key is still being moved: fetch it from where it used to live
		t.slowedMoves++
	}
	return x.to, lat, x
}

func cmdIsRedis(c cmd) bool {
	return c == cmdLock || c == cmdUnlock || c == cmdGeo || c == cmdRank || c == cmdScore
}

// callCmd runs a command against edge e from node `from`, applying breaker, timeout and pooling.
func (s *Sim) callCmd(from *node, e *edge, c *call, cm cmd, ver int, cb func(Result)) {
	cc := *c
	cc.cmd, cc.ver, cc.ticket = cm, ver, nil
	switch cm {
	case cmdWrite, cmdCacheInv, cmdCacheSet, cmdScore, cmdEnqueue, cmdUnlock, cmdExternal, cmdPut:
		cc.write = true
	case cmdRead, cmdCacheGet, cmdGeo, cmdRank, cmdGet:
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
	bulkLimit := 0
	if app {
		bulkLimit = int(from.f("bulkheadThreads"))
	}
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
	if bulkLimit > 0 {
		if e.bulk >= bulkLimit { // this dependency already holds as many threads as it may
			cb(fail(RBulkheadFull))
			return
		}
		e.bulk++
	}
	settled := false
	finish := func(res Result) {
		if settled {
			return
		}
		settled = true
		if bulkLimit > 0 {
			e.bulk--
		}
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
		lat := lat
		if tk != nil && tk.opening { // a new connection: the handshake comes before the request can travel
			lat += from.f("connectMs")
		}
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
		return max(1, int(n.f("threads"))*s.instancesOf(n)-int(math.Round(s.pushThreads(n))))
	case KDatabase, KReplica:
		return int(n.f("connections"))
	case KWorker:
		return int(n.f("concurrency"))
	case KRedis:
		return 1
	case KCache:
		return 10000
	case KObjectStore:
		return int(n.f("maxConcurrent"))
	case KIndex:
		return int(n.f("connections"))
	case KVectorStore:
		return int(n.f("connections"))
	}
	return 1 << 30
}

func (s *Sim) queueCap(n *node) int {
	switch n.kind {
	case KAppServer, KDatabase, KReplica, KRedis, KIndex, KVectorStore:
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
	if n.ad != nil {
		return s.adaptiveReleaser(n)
	}
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
	if s.hasRoom(n) && n.pausedUntil <= s.now {
		s.markBusy(n)
		n.busy++
		if c.sp != nil {
			c.sp.start = s.now
		}
		run(s.releaser(n))
		return
	}
	if shed := n.f("shedAboveQueueMs"); shed > 0 && n.kind == KAppServer && !c.high && n.qh < len(n.q) && s.now-n.q[n.qh].enq > shed {
		s.failAt(n, RShed, cb) // the oldest request has waited too long: ordinary newcomers are turned away
		return
	}
	if len(n.q)-n.qh < s.queueCap(n) {
		n.q = append(n.q, &task{c: c, cb: cb, run: run, enq: s.now})
		return
	}
	s.failAt(n, RQueueFull, cb)
}

func (s *Sim) startNext(n *node) {
	for !n.down && n.pausedUntil <= s.now && s.hasRoom(n) && n.qh < len(n.q) {
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
		if t.c.deadline > 0 && s.now+deadlineSlack(n, t.c) > t.c.deadline && (t.c.honor || (n.kind == KAppServer && n.b("propagateDeadline"))) {
			if t.c.counted { // nobody is waiting for this any more: do not spend a slot on it
				s.runDeadlineSkipped++
			}
			s.failAt(n, RDeadline, t.cb)
			continue
		}
		s.markBusy(n)
		n.busy++
		if t.c.sp != nil {
			t.c.sp.start = s.now // waiting in the queue ends here
		}
		t.run(s.releaser(n))
	}
}

// deadlineSlack is how long the work still to do here is likely to take: a request that cannot finish before its
// deadline is not worth starting. Twice the station's own nominal time (CPU, a read or write, a job) is a cheap
// estimate and leaves the dependencies' time out of it.
func deadlineSlack(n *node, c *call) float64 {
	switch n.kind {
	case KAppServer:
		return 2 * n.f("cpuMs")
	case KDatabase, KReplica:
		if c.cmd == cmdWrite {
			return 2 * n.f("writeMs")
		}
		return 2 * n.f("readMs")
	}
	return 0
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
	if n.cut {
		n.held = append(n.held, func() { s.arrive(n, c, cb) })
		return
	}
	if c.tr != nil {
		cb = s.traceArrive(n, c, cb)
	}
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
		reason := RNodeDown
		if n.kind == KDatabase && n.s("onFailure") == "promote" && n.successor == nil {
			reason = RNoPrimary // a replica is about to take over: say so instead of 'down'
		}
		s.failAt(n, reason, cb)
		return
	}
	if n.badUntil > s.now { // a bad release: every request is answered with an error
		s.failAt(n, RBadRelease, cb)
		return
	}
	switch n.kind {
	case KQueue:
		s.arriveQueue(n, c, cb)
		return
	case KTopic:
		s.arriveTopic(n, c, cb)
		return
	case KLog:
		s.arriveLog(n, c, cb)
		return
	case KStream:
		s.arriveStream(n, c, cb)
		return
	case KLLMServer:
		s.arriveLLM(n, c, cb)
		return
	case KClient, KWorker:
		s.failAt(n, RNoRoute, cb)
		return
	case KReplica:
		if c.cmd == cmdWrite && !n.promoted {
			s.failAt(n, RNoRoute, cb)
			return
		}
	case KRedis, KCache:
		if c.cmd == cmdNone {
			s.failAt(n, RNoRoute, cb)
			return
		}
	case KIndex:
		if c.cmd != cmdRead {
			s.failAt(n, RNoRoute, cb)
			return
		}
	case KObjectStore:
		if c.cmd != cmdPut && c.cmd != cmdGet {
			s.failAt(n, RNoRoute, cb)
			return
		}
	case KRateLimiter:
		if n.s("algorithm") == "leakyBucket" {
			ok, wait := s.leak(n, c)
			if !ok {
				s.failAt(n, RRateLimited, cb)
				return
			}
			n.c.adm++
			if wait > 0 { // the request waits for its slot, then goes on as if it had just arrived
				s.after(wait, func() {
					s.submit(n, c, cb, func(release func()) { s.process(n, c, cb, release) })
				})
				return
			}
		} else if !s.admit(n, c) {
			s.failAt(n, RRateLimited, cb)
			return
		} else {
			n.c.adm++
		}
	}
	s.submit(n, c, cb, func(release func()) { s.process(n, c, cb, release) })
}

// admit decides whether a rate limiter lets a request through: one allowance for everything, or (keyBy user) one
// per account, so that a heavy account runs out without anyone else noticing.
func (s *Sim) admit(n *node, c *call) bool {
	if n.b("shared") { // every sharing limiter counts into the first one's state
		if s.sharedLimit == nil || s.sharedLimit.removed {
			s.sharedLimit = n
		}
		n = s.sharedLimit
	}
	if n.s("unit") == "tokens" {
		return s.admitTokens(n, c)
	}
	if n.s("algorithm") == "leakyBucket" {
		ok, _ := s.leak(n, c)
		return ok
	}
	if algo := n.s("algorithm"); algo != "tokenBucket" && algo != "" {
		return s.admitWindow(n, c, algo)
	}
	key := limiterKey(n, c)
	if key == 0 {
		return s.takeToken(n)
	}
	if n.buckets == nil {
		n.buckets = map[int]*bucket{}
	}
	b := n.buckets[key]
	if b == nil {
		b = &bucket{tokens: n.f("burst"), at: s.now}
		n.buckets[key] = b
	}
	b.tokens = math.Min(n.f("burst"), b.tokens+(s.now-b.at)*n.f("limitRps")/1000)
	b.at = s.now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// admitWindow is the window-counting family of rate limiters. The limit per window is the sustained rate times the
// window length. Fixed window resets its count at each edge; the sliding log keeps the time of every admitted request
// and forgets those older than a window; the sliding counter estimates the last window as the previous window's count
// weighted by how much of it still overlaps.
func (s *Sim) admitWindow(n *node, c *call, algo string) bool {
	win := n.f("windowSec") * 1000
	aligned := math.Floor(s.now/win) * win // windows are aligned to the clock, as in a real limiter
	var b *bucket
	if key := limiterKey(n, c); key != 0 {
		if n.buckets == nil {
			n.buckets = map[int]*bucket{}
		}
		if b = n.buckets[key]; b == nil {
			b = &bucket{winStart: aligned}
			n.buckets[key] = b
		}
	} else {
		if n.global == nil {
			n.global = &bucket{winStart: aligned}
		}
		b = n.global
	}
	limit := n.f("limitRps") * n.f("windowSec")
	switch algo {
	case "slidingWindowLog":
		cut := s.now - win
		i := 0
		for i < len(b.log) && b.log[i] <= cut {
			i++
		}
		b.log = b.log[i:]
		if float64(len(b.log)) >= limit {
			return false
		}
		b.log = append(b.log, s.now)
		return true
	default: // fixedWindow and slidingWindowCounter share the window bookkeeping
		for s.now-b.winStart >= win {
			if s.now-b.winStart >= 2*win {
				b.prev, b.count = 0, 0 // idle for a whole window: nothing left to weigh
				b.winStart = s.now - math.Mod(s.now-b.winStart, win)
				break
			}
			b.prev, b.count = b.count, 0
			b.winStart += win
		}
		used := b.count
		if algo == "slidingWindowCounter" {
			used += b.prev * (1 - (s.now-b.winStart)/win)
		}
		if used >= limit {
			return false
		}
		b.count++
		return true
	}
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
	var fire func()
	fire = func() {
		if n.epoch != ep || n.removed {
			release()
			s.failAt(n, RNodeDown, cb)
			return
		}
		if n.pausedUntil > s.now { // a stop-the-world pause: the work finishes when it ends
			s.at(n.pausedUntil, fire)
			return
		}
		fn()
	}
	s.after(ms, fire)
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
	case KKVStore:
		s.procKV(n, c, cb, release)
	case KCoordinator:
		s.procCoord(n, c, cb, release)
	case KVectorStore:
		ms := n.f("searchBaseMs") + n.f("efSearch")*n.f("perCandidateMs")
		s.hold(n, s.service(n, ms)*n.slow, release, cb, func() {
			release()
			s.okAt(n, Result{Ver: -1, Done: true, Src: n}, cb)
		})
	case KIndex:
		s.hold(n, s.service(n, n.f("queryMs"))*n.slow, release, cb, func() {
			release()
			s.okAt(n, Result{Ver: -1, Done: true, Src: n}, cb)
		})
	case KObjectStore:
		// time to first byte, then the body at the store's per-request throughput
		ms := n.f("firstByteMs")
		req, resp := s.sizes(c)
		body := req
		if c.cmd == cmdGet {
			body = resp
			n.egressBytes += resp // billed per GB served
		}
		n.osRequests++
		if body > 0 {
			ms += body / (n.f("throughputMBps") * 1000) // bytes / (MB/s x 1000 bytes per ms)
		}
		s.hold(n, s.service(n, ms)*n.slow, release, cb, func() {
			release()
			s.okAt(n, Result{Ver: -1, Done: true}, cb)
		})
	case KExternalAPI:
		if rate := n.f("politeRps"); rate > 0 { // a site that refuses more than it likes
			if n.hostAt == 0 { // the first call: a full bucket
				n.hostTokens = rate
			}
			n.hostTokens = min(rate, n.hostTokens+(s.now-n.hostAt)*rate/1000)
			n.hostAt = s.now
			if n.hostTokens < 1 {
				release()
				s.failAt(n, RRateLimited, cb)
				return
			}
			n.hostTokens--
		}
		s.hold(n, s.service(n, n.f("latencyMs"))*n.slow, release, cb, func() {
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
		if pct := n.f("canaryPct"); pct > 0 {
			cand = s.canarySplit(n, cand, pct)
		}
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

// admitTokens is a token bucket counted in tokens: the allowance (tokens a minute) refills steadily up to a minute's
// worth, and a request is let through only if its whole weight is there.
func (s *Sim) admitTokens(n *node, c *call) bool {
	rate := n.f("limitTokensPerMin")
	if n.tokAt == 0 {
		n.tokTokens = rate
	}
	n.tokTokens = math.Min(rate, n.tokTokens+(s.now-n.tokAt)*rate/60000)
	n.tokAt = s.now
	w := float64(max(1, int(c.tok)))
	if n.tokTokens < w {
		return false
	}
	n.tokTokens -= w
	return true
}
