package engine

import "math"

// Clients: traffic generation, request attempts, timeouts and retries.

func (s *Sim) rate(n *node, tMs float64) float64 {
	t := tMs / 1000
	if s.opts.Traffic != nil {
		pts := s.opts.Traffic[n.id]
		if len(pts) == 0 {
			return 0
		}
		if t <= pts[0][0] {
			return pts[0][1]
		}
		for i := 0; i < len(pts)-1; i++ {
			a, b := pts[i], pts[i+1]
			if t < b[0] {
				if b[0] > a[0] {
					return a[1] + (b[1]-a[1])*(t-a[0])/(b[0]-a[0])
				}
				return b[1]
			}
		}
		return pts[len(pts)-1][1]
	}
	base, peak := n.f("rps"), n.f("peakRps")
	period := max(0.001, n.f("periodSec"))
	switch n.s("pattern") {
	case "ramp":
		if t >= period {
			return peak
		}
		return base + (peak-base)*t/period
	case "spike":
		m := math.Mod(t, period)
		if m >= period/2 && m < period*0.75 {
			return peak
		}
		return base
	case "wave":
		return base + (peak-base)*(1-math.Cos(2*math.Pi*t/period))/2
	}
	return base
}

func (s *Sim) trafficStopped() bool {
	return s.opts.StopTrafficAtMs > 0 && s.now >= s.opts.StopTrafficAtMs
}

func (s *Sim) startClient(n *node) {
	n.arrivalGen++
	if n.s("arrival") == "closed" {
		s.startCrowd(n, n.arrivalGen)
		return
	}
	if n.s("arrival") == "sessions" {
		s.startSessions(n, n.arrivalGen)
		return
	}
	s.nextArrival(n, n.arrivalGen)
}

// startCrowd runs a closed-loop population: each user sends one request, waits for it to finish (success, failure or
// give-up, retries included), thinks for a random time, and goes again. Offered load therefore falls as latency rises.
func (s *Sim) startCrowd(n *node, gen int) {
	think := func() float64 { return s.rng.Exp() * n.f("thinkMs") }
	var user func()
	user = func() {
		if gen != n.arrivalGen || n.removed || n.down || s.trafficStopped() {
			return
		}
		s.createLogical(n).after = func() { s.after(think(), user) }
	}
	for i := 0; i < int(n.f("users")); i++ {
		s.after(think()+1, user) // staggered starts, as if the users had been there a while
	}
}

// burstMultiplier scales a bursty client's rate: rushes take a fifth of the time at burstFactor times the average, and the
// quiet phases run slower so that the long-run average is unchanged.
func (s *Sim) burstMultiplier(n *node) float64 {
	factor := min(5, max(1.5, n.f("burstFactor")))
	mean := max(0.5, n.f("burstSec")) * 1000
	for s.now >= n.burstUntil {
		n.burstOn = !n.burstOn
		phase := mean
		if !n.burstOn {
			phase = mean * 4
		}
		// Erlang-4 phase lengths: random, but regular enough that the long-run average does not wander
		n.burstUntil = max(n.burstUntil, s.now) + (s.rng.Exp()+s.rng.Exp()+s.rng.Exp()+s.rng.Exp())/4*phase
	}
	if n.burstOn {
		return factor
	}
	return max(0.05, (1-0.2*factor)/0.8)
}

func (s *Sim) nextArrival(n *node, gen int) {
	if gen != n.arrivalGen || n.removed || n.down || s.trafficStopped() {
		return
	}
	r := s.rate(n, s.now)
	if n.s("arrival") == "bursty" {
		r *= s.burstMultiplier(n)
	}
	if r <= 0 {
		s.after(100, func() { s.nextArrival(n, gen) })
		return
	}
	gap := s.rng.Exp() / r * 1000
	if n.s("arrival") == "bursty" && s.now+gap > n.burstUntil {
		// the rate changes before this arrival would happen: redraw at the change (exponential gaps are memoryless),
		// otherwise a long quiet-phase gap would swallow the start of the next rush and the average would sag
		s.after(max(1, n.burstUntil-s.now), func() { s.nextArrival(n, gen) })
		return
	}
	s.after(gap, func() {
		if gen != n.arrivalGen || n.removed || n.down || s.trafficStopped() {
			return
		}
		s.createLogical(n)
		s.nextArrival(n, gen)
	})
}

type logical struct {
	id        int
	client    *node
	write     bool
	createdAt float64
	attempts  int
	resolved  bool
	counted   bool
	key       int
	wl        int
	hash      uint32
	user      int   // the account sending the request (0 = identities not modelled)
	heavy     bool  // an expensive request (heavyPct), chosen from the request number
	tok       int32 // tokens the request carries, for a limiter that counts them
	sawWrite  int   // known own-write version; negative marks an already-counted write-order violation
	sawRead   int
	high      bool // an urgent request (priorityPct): chosen from the request number, no random draw
	tr        *tracer
	connect   bool // opens a long-lived connection (sessions arrival)
	sess      *session
	after     func() // called once, when the request is resolved (closed-loop users think, then ask again)
}

type attempt struct {
	lg    *logical
	at    float64
	n     int
	done  bool
	ghost bool
}

const (
	wlKV = iota
	wlGeo
	wlLeader
	wlBooking
	wlTransfer // every request moves money between two accounts: a two-key read-modify-write transaction
	wlOnCall   // read a pair of rows, then write one of them: the write-skew workload (see txn.go)
	wlFeed     // posts fan out to followers' timelines; reads fetch a timeline (see feed.go)
	wlCrawl    // every request fetches the next URL of a crawl frontier (see crawl.go)
)

func workloadOf(n *node) int {
	switch n.s("workload") {
	case "geo":
		return wlGeo
	case "leaderboard":
		return wlLeader
	case "booking":
		return wlBooking
	case "transfer":
		return wlTransfer
	case "feed":
		return wlFeed
	case "oncall":
		return wlOnCall
	case "crawl":
		return wlCrawl
	}
	return wlKV
}

func (s *Sim) createLogical(c *node) *logical {
	s.logicalSeq++
	lg := &logical{
		id: s.logicalSeq, client: c, createdAt: s.now,
		write:   s.rng.Next() >= c.f("readPct")/100,
		counted: s.now >= s.opts.WarmupMs,
		hash:    Hash32(uint32(s.logicalSeq)),
		wl:      workloadOf(c),
	}
	if pct := c.f("heavyPct"); pct > 0 {
		lg.heavy = float64(Hash32(uint32(s.logicalSeq)*668265263+29)%100) < pct
	}
	if pct := c.f("priorityPct"); pct > 0 {
		lg.high = float64(Hash32(uint32(s.logicalSeq)*374761393+17)%100) < pct
	}
	if lg.wl == wlTransfer || lg.wl == wlOnCall {
		lg.write = true // a transfer is always a transaction
	}
	if tok := c.f("requestTokens"); tok > 0 {
		lg.tok = int32(tok)
		if float64(Hash32(uint32(s.logicalSeq)*3266489917+41)%100) < c.f("bigRequestPct") {
			lg.tok *= 20
		}
	}
	lg.key = c.keys.Sample(c.s("keyDist"), int(c.f("keySpace")), c.f("zipfS"), c.f("hotKeyPct"), s.keyRng)
	if lg.wl == wlCrawl {
		lg.key = s.crawlFetch(c) // the site of the page taken from the frontier
	}
	if lg.counted {
		s.countKey(lg.key)
	}
	if pct := c.f("missingPct"); pct > 0 && float64(Hash32(uint32(s.logicalSeq)*2891336453+53)%100) < pct {
		ks := int(c.f("keySpace")) // an id outside the real ones: negative, so it can never match a stored item
		lg.key = -(ks + 1 + int(Hash32(uint32(s.logicalSeq)*374761393+59)%uint32(10*ks)))
	}
	c.stampSeen(lg)
	if n := int(c.f("identities")); n > 0 {
		dist := "uniform"
		if c.f("identitySkew") > 0 {
			dist = "zipf"
		}
		lg.user = c.idBase + c.userKeys.Sample(dist, n, c.f("identitySkew"), 0, s.keyRng)
	}
	if c.b("abuse") { // a scraper: items picked from a huge space, so almost none is cached
		lg.key = 1 + int(s.keyRng.Next()*abuseKeySpace)
	}
	if c.f("retryBudgetPct") > 0 {
		c.decayRetryCounts(s.now)
		c.retryFirsts++
	}
	s.unresolved++
	if lg.counted {
		s.runPending++
	}
	s.created.Push(s.now, true, 0)
	lg.tr = s.startTrace(lg)
	s.startAttempt(lg)
	return lg
}

func (s *Sim) startAttempt(lg *logical) {
	if lg.resolved {
		return
	}
	c := lg.client
	ttl := c.f("dnsTtlSec")
	var e *edge
	if ttl > 0 {
		if c.dnsNextLookup <= s.now {
			if lg.counted {
				s.runDNSLookups++
			}
			c.dnsNextLookup = s.now + ttl*1000
		}
		e = s.pickCyclic(c, "out", c.out, func(edge *edge) bool {
			return !edge.to.down || s.now-edge.to.downAt < ttl*1000
		})
	} else {
		e = s.pickCyclic(c, "out", c.out, nil)
	}
	if c.b("geoRouting") {
		if ttl > 0 {
			e = s.nearestEdgeDNS(c, ttl)
		} else {
			e = s.nearestEdge(c)
		}
	}
	if e == nil {
		s.resolve(lg, Result{Reason: RNoRoute})
		return
	}
	lg.attempts++
	s.attempts.Push(s.now, true, 0)
	at := &attempt{lg: lg, at: s.now, n: lg.attempts}
	call := &call{tok: lg.tok, lg: lg, at: at, write: lg.write, key: lg.key, wl: lg.wl, hash: lg.hash, rid: lg.id, counted: lg.counted, ver: -1, tr: lg.tr, high: lg.high, heavy: lg.heavy, user: lg.user, deadline: s.now + c.f("timeoutMs") - e.latency}
	s.hop(c, e, call, func(res Result) {
		if at.done {
			return
		}
		at.done = true
		if res.OK {
			s.resolve(lg, res)
		} else {
			s.attemptFailed(at, res.Reason)
		}
	})
	s.after(c.f("timeoutMs"), func() {
		if !at.done {
			at.done, at.ghost = true, true
			s.attemptFailed(at, RTimeout)
		}
	})
}

// retryBudgetWindowMs is the time constant over which first attempts and retries are counted for the retry budget.
const retryBudgetWindowMs = 10000

// retryAllowed applies a client's retry budget: a retry is taken only while retries stay within retryBudgetPct of
// first attempts, both counted with an exponential decay over about ten seconds. No budget means always.
func (s *Sim) retryAllowed(c *node) bool {
	pct := c.f("retryBudgetPct")
	if pct <= 0 {
		return true
	}
	c.decayRetryCounts(s.now)
	if c.retryRetries+1 > pct/100*c.retryFirsts {
		return false
	}
	c.retryRetries++
	return true
}

func (n *node) decayRetryCounts(now float64) {
	if dt := now - n.retryAt; dt > 0 {
		f := math.Exp(-dt / retryBudgetWindowMs)
		n.retryFirsts *= f
		n.retryRetries *= f
		n.retryAt = now
	}
}

// abuseKeySpace is how many distinct items a scraper draws from.
const abuseKeySpace = 1_000_000

func (s *Sim) attemptFailed(at *attempt, r Reason) {
	lg := at.lg
	if lg.resolved {
		return
	}
	cp := lg.client.p
	if lg.attempts < 1+int(pf(cp, "retries", 0)) && !lg.client.b("abuse") && s.retryAllowed(lg.client) { // bots do not retry
		delay := 0.0
		if v, _ := cp["backoff"].(string); v != "none" {
			delay = 100 * math.Pow(2, float64(lg.attempts-1)) * (0.5 + s.rng.Next())
		}
		s.after(delay, func() { s.startAttempt(lg) })
		return
	}
	s.resolve(lg, Result{Reason: r})
}

func (s *Sim) resolve(lg *logical, res Result) {
	if lg.resolved {
		return
	}
	lg.resolved = true
	if lg.after != nil {
		defer lg.after()
	}
	s.unresolved--
	if lg.tr != nil {
		s.finishTrace(lg, res)
	}
	lat := s.now - lg.createdAt
	s.res.Push(s.now, res.OK, lat)
	cl := lg.client
	if res.OK {
		cl.c.ok++
	} else {
		cl.c.err++
		cl.c.reason[res.Reason]++
	}
	if lg.counted && s.hasAbuse && !lg.client.b("abuse") { // how the real customers fared, apart from the bots
		s.runCustTotal++
		if res.OK {
			s.runCustOK++
		}
	}
	if lg.counted && lg.user != 0 && lg.client.f("tenants") >= 1 { // how each tenant fared
		tn := tenantOfUser(lg.client, lg.user)
		for len(s.tenantTotal) < tn {
			s.tenantTotal, s.tenantOK = append(s.tenantTotal, 0), append(s.tenantOK, 0)
		}
		s.tenantTotal[tn-1]++
		if res.OK {
			s.tenantOK[tn-1]++
		}
	}
	if lg.counted && lg.high {
		s.runHighTotal++
		if res.OK {
			s.runHighOK++
		}
	}
	if lg.counted {
		s.runPending--
		s.runResolved++
		if res.OK {
			s.runOK++
			s.hist.Add(lat)
			if cl.f("heavyPct") > 0 {
				if lg.heavy {
					s.heavyHist.Add(lat)
				} else {
					s.lightHist.Add(lat)
				}
			}
			s.countFreshness(lg, res)
			s.countSession(lg, res)
		} else {
			s.runFailed++
			s.failHist.Add(lat)
			s.runErrors[res.Reason]++
		}
	}
}

// nearestEdge is geo routing: of the places a client can send to, those in the region closest to the user, taking
// turns among them. If every nearby one is down the next nearest is used (a failover across regions).
func (s *Sim) nearestEdge(c *node) *edge {
	return s.nearestEdgeDNS(c, 0)
}

func (s *Sim) nearestEdgeDNS(c *node, ttl float64) *edge {
	usable := func(e *edge) bool { return !e.removed && (!e.to.down || s.now-e.to.downAt < ttl*1000) }
	best := -1.0
	for _, e := range c.out {
		if !usable(e) {
			continue
		}
		if d := regionMs(c.region, e.to.region); best < 0 || d < best {
			best = d
		}
	}
	if best < 0 {
		return s.pickCyclic(c, "out", c.out, nil)
	}
	return s.pickCyclic(c, "geo", c.out, func(e *edge) bool { return usable(e) && regionMs(c.region, e.to.region) == best })
}
