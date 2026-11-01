package engine

import (
	"math"
)

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
	case KIndex, KAuth:
		return int(n.f("connections"))
	case KVectorStore:
		return int(n.f("connections"))
	}
	return 1 << 30
}

func (s *Sim) queueCap(n *node) int {
	switch n.kind {
	case KAppServer, KDatabase, KReplica, KRedis, KIndex, KVectorStore, KAuth:
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
	case KAuth:
		if c.cmd != cmdAuth {
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
	b.tokens = min(n.f("burst"), b.tokens+(s.now-b.at)*n.f("limitRps")/1000)
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
	n.tokens = min(n.f("burst"), n.tokens+(s.now-n.tokensAt)*n.f("limitRps")/1000)
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
	case KAuth:
		s.hold(n, s.service(n, n.f("checkMs"))*n.slow, release, cb, func() {
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
			per := n.f("throughputMBps") * 1000 // bytes per ms for one stream
			if chunk := n.f("multipartKB") * 1000; chunk > 0 && c.cmd == cmdPut && body > chunk {
				// multipart upload: parts of `chunk` bytes, `parallel` of them at once, each at the per-stream rate
				rounds := math.Ceil(math.Ceil(body/chunk) / max(1, n.f("multipartParallel")))
				ms += rounds * chunk / per
			} else {
				ms += body / per // bytes / (MB/s x 1000 bytes per ms)
			}
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

// admitTokens is a token bucket counted in tokens: the allowance (tokens a minute) refills steadily up to a minute's
// worth, and a request is let through only if its whole weight is there.
func (s *Sim) admitTokens(n *node, c *call) bool {
	rate := n.f("limitTokensPerMin")
	if n.tokAt == 0 {
		n.tokTokens = rate
	}
	n.tokTokens = min(rate, n.tokTokens+(s.now-n.tokAt)*rate/60000)
	n.tokAt = s.now
	w := float64(max(1, int(c.tok)))
	if n.tokTokens < w {
		return false
	}
	n.tokTokens -= w
	return true
}
