package engine

import ()

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
	cmdPut     // upload a body to an object store
	cmdGet     // download a body from an object store
	cmdAuth    // ask an auth service whether a token is valid
	cmdSession // look up or refresh a login session in a shared store
)

type call struct {
	lg       *logical
	at       *attempt
	write    bool
	bodyB    int32 // bytes of the response this call carries when it is only the missing pieces of a ranged object (0 = the whole response); fills padding
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
	fwd := oneWay + s.transmit(e, 0, req) + s.nic(e.from, req)
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
			if !res.OK && res.Reason == RNodeDown && e.from.kind == KClient && target.down && s.now-target.downAt < e.from.f("dnsTtlSec")*1000 {
				res.Reason = RStaleDNS // the answer the client had cached still pointed at a server that is gone
			}
			back := oneWay + s.transmit(e, 1, resp) + s.nic(e.to, resp)
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
	req, resp = s.lgSizes(c.lg)
	if c.bodyB > 0 {
		resp = float64(c.bodyB)
	}
	return req, resp
}

// nic puts bytes on the network card of the node they leave, one first-in-first-out transmitter for everything that
// node sends (requests and answers alike), and returns the wait plus the transmission time. No card limit (the
// default) or no bytes costs nothing.
func (s *Sim) nic(n *node, bytes float64) float64 {
	if bytes <= 0 {
		return 0
	}
	mbps := n.f("nicMbps")
	if mbps <= 0 {
		return 0
	}
	dur := bytes * 8 / (mbps * 1000)
	start := max(s.now, n.nicFree)
	n.nicFree = start + dur
	return start + dur - s.now
}

// transmit puts bytes on a link in one direction and returns how long until they have all arrived: the wait for
// the bytes ahead of them plus their own transmission time. A link with no limit, or no bytes, costs nothing.
func (s *Sim) transmit(e *edge, dir int, bytes float64) float64 {
	if e.bw <= 0 || bytes <= 0 {
		return 0
	}
	dur := bytes * 8 / (e.bw * 1000) // bits / (Mbit/s x 1e3 bits per ms)
	start := max(s.now, e.txFree[dir])
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
	return c == cmdLock || c == cmdUnlock || c == cmdGeo || c == cmdRank || c == cmdScore || c == cmdSession
}

// callCmd runs a command against edge e from node `from`, applying breaker, timeout and pooling.
func (s *Sim) callCmd(from *node, e *edge, c *call, cm cmd, ver int, cb func(Result)) {
	cc := *c
	cc.cmd, cc.ver, cc.ticket = cm, ver, nil
	switch cm {
	case cmdWrite, cmdCacheInv, cmdCacheSet, cmdScore, cmdEnqueue, cmdUnlock, cmdExternal, cmdPut:
		cc.write = true
	case cmdRead, cmdCacheGet, cmdGeo, cmdRank, cmdGet, cmdSession, cmdAuth:
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

// ---- connection pools --------------------------------------------------------------

// ---- stations (capacity + queue) -----------------------------------------------------

// ---- arrival -----------------------------------------------------------------------

// ---- load balancer / rate limiter -----------------------------------------------------

// ---- CDN -----------------------------------------------------------------------------
