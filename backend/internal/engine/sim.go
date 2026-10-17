package engine

import (
	"math"
)

// Options configures a run.
type Options struct {
	Seed            uint32
	Traffic         map[string][]TrafficPoint // optional per-client piecewise-linear rps curves
	Chaos           []ChaosEvent
	StopTrafficAtMs float64 // 0 = never stop
	WarmupMs        float64 // requests created before this are not counted
}

const (
	tickMs      = 100.0
	ringSize    = 10
	seriesCap   = 600
	statsWindow = 5000.0 // ms of history kept for sliding windows
	breakerFail = 5
	breakerOpen = 5000.0
	commitKeep  = 60000.0
	verKeep     = 8
	ejectMs     = 10000.0
)

type counters struct {
	arr, ok, err, qfull, adm             int
	hit, miss, evict, expired            int
	lockBusy, conflict, stale, dup, done int
	reason                               [NumReasons]int
}

func (c *counters) add(o *counters) {
	c.arr += o.arr
	c.ok += o.ok
	c.err += o.err
	c.qfull += o.qfull
	c.adm += o.adm
	c.hit += o.hit
	c.miss += o.miss
	c.evict += o.evict
	c.expired += o.expired
	c.lockBusy += o.lockBusy
	c.conflict += o.conflict
	c.stale += o.stale
	c.dup += o.dup
	c.done += o.done
	for i := range c.reason {
		c.reason[i] += o.reason[i]
	}
}

type task struct {
	c   *call
	cb  func(Result)
	run func(release func())
	enq float64
}

type keyLock struct {
	held    bool
	waiters []func(unlock func())
}

type redisLock struct {
	token int
	exp   float64
}

type job struct {
	rid      int
	key      int
	wl       int
	idem     bool
	counted  bool
	enq      float64
	attempts int
}

type node struct {
	id, kind, label string
	x, y            float64
	p               Params
	removed         bool
	down            bool
	downAt          float64
	epoch           int
	slow            float64
	slowUntil       float64
	out, in         []*edge

	busy     int
	q        []*task
	qh       int
	busyArea float64
	lastBusy float64
	areaMark float64

	// client
	arrivalGen int
	keys       KeySampler

	// cache / cdn
	store     *Store
	storePol  string
	preloaded bool

	// rate limiter
	tokens   float64
	tokensAt float64

	// database
	openConns int
	booked    map[int]int
	klocks    map[int]*keyLock

	// app / worker
	pools map[*node]*pool

	// redis
	locks map[int]redisLock

	// queue
	jobs []*job
	jh   int

	rr map[string]int

	c    counters
	ring [ringSize]counters
	util [ringSize]float64
}

func (n *node) f(k string) float64 { return pf(n.p, k, 0) }
func (n *node) s(k string) string  { v, _ := n.p[k].(string); return v }
func (n *node) b(k string) bool    { v, _ := n.p[k].(bool); return v }

type edge struct {
	id        string
	from, to  *node
	latency   float64
	removed   bool
	fwd       int
	fwdRing   [ringSize]int
	inflight  int
	brState   int // 0 closed, 1 open, 2 half-open
	brFails   int
	brUntil   float64
	trial     bool
	oCnt      [20]float64
	oSum      [20]float64
	oErr      [20]float64
	ejectTill float64
}

// Particle is a visual hint that a request crossed an edge.
type Particle struct {
	Edge string `json:"e"`
	Back bool   `json:"b,omitempty"`
	Fail bool   `json:"f,omitempty"`
}

// Sim is a deterministic discrete-event simulation of a system diagram.
type Sim struct {
	now                   float64
	opts                  Options
	rng, keyRng, chaosRng *RNG
	heap                  eventHeap
	seq                   uint64

	nodes map[string]*node
	order []*node
	edges map[string]*edge
	eord  []*edge

	logicalSeq int
	tokenSeq   int
	unresolved int

	res, created, attempts, jobsW, conflictsW Window
	hist                                      Hist

	runResolved, runOK, runFailed, runPending int
	runErrors                                 [NumReasons]int
	runConflicts, runFresh, runStale, runDup  int

	jobEnqueued, jobCompleted, jobLost, jobRetried int
	jobHist                                        Hist
	hasQueue, hasBooking, hasFresh                 bool

	vers      map[int]*verLog
	committed map[int]float64
	commitLog []commitRec
	commitHd  int

	tickCount int
	bi        int
	series    []SeriesPoint
	chaosLog  []ChaosRecord

	born      []Particle
	bornOn    bool
	stride    int
	hopCount  int
	finalized bool
}

type verLog struct {
	v []int
	t []float64
}

type commitRec struct {
	rid int
	t   float64
}

// SeriesPoint is one point of the global time series.
type SeriesPoint struct {
	T       float64 `json:"t"`
	OkRps   float64 `json:"okRps"`
	ErrRps  float64 `json:"errRps"`
	P50     float64 `json:"p50"`
	P95     float64 `json:"p95"`
	Success float64 `json:"successRate"`
	InFly   int     `json:"inFlight"`
}

// ChaosRecord is a chaos event that fired.
type ChaosRecord struct {
	T      float64 `json:"t"`
	Type   string  `json:"type"`
	NodeID string  `json:"nodeId"`
	Label  string  `json:"label"`
}

// New builds a simulation of d. The diagram should already be sanitized.
func New(d Diagram, opts Options) *Sim {
	seed := opts.Seed
	s := &Sim{
		opts:      opts,
		rng:       NewRNG(seed),
		keyRng:    NewRNG(Hash32(seed ^ 1540483477)),
		chaosRng:  NewRNG(Hash32(seed ^ 739982445)),
		nodes:     map[string]*node{},
		edges:     map[string]*edge{},
		vers:      map[int]*verLog{},
		committed: map[int]float64{},
		stride:    1,
	}
	for _, n := range d.Nodes {
		s.addNode(n)
	}
	keySpace := 0
	for _, n := range s.order {
		if n.kind == KClient {
			keySpace = int(math.Max(float64(keySpace), n.f("keySpace")))
		}
	}
	for _, n := range s.order {
		if n.store != nil && n.b("startWarm") {
			n.store.Preload(keySpace, 0)
		}
	}
	for _, e := range d.Edges {
		s.addEdge(e)
	}
	s.afterTopologyChange()
	for _, ev := range opts.Chaos {
		ev := ev
		s.at(ev.AtSec*1000, func() { s.fireChaos(ev) })
	}
	for _, n := range s.order {
		if n.kind == KClient {
			s.startClient(n)
		}
	}
	s.at(tickMs, s.tick)
	return s
}

// Now is the simulated time in milliseconds.
func (s *Sim) Now() float64 { return s.now }

func (s *Sim) at(t float64, fn func()) {
	s.seq++
	s.heap.Push(event{t: t, seq: s.seq, fn: fn})
}

func (s *Sim) after(d float64, fn func()) { s.at(s.now+d, fn) }

// Advance runs all events up to now+dt.
func (s *Sim) Advance(dt float64) {
	target := s.now + math.Max(0, dt)
	for s.heap.PeekTime() <= target {
		e := s.heap.Pop()
		s.now = e.t
		e.fn()
	}
	s.now = target
}

// EnableParticles turns on collection of edge-crossing particles.
func (s *Sim) EnableParticles(on bool) { s.bornOn = on }

// TakeParticles returns and clears particles born since the last call.
func (s *Sim) TakeParticles() []Particle {
	p := s.born
	s.born = nil
	return p
}

func (s *Sim) emit(e *edge, back, fail bool) {
	if !s.bornOn {
		return
	}
	s.hopCount++
	if len(s.born) >= 256 || s.hopCount%(1+len(s.born)/96) != 0 {
		return
	}
	s.born = append(s.born, Particle{Edge: e.id, Back: back, Fail: fail})
}

// ---- topology -----------------------------------------------------------------

func (s *Sim) addNode(d Node) *node {
	n := &node{
		id: d.ID, kind: d.Kind, label: d.Label, x: d.X, y: d.Y,
		p: Resolve(d.Kind, d.Params), slow: 1,
		pools: map[*node]*pool{}, locks: map[int]redisLock{},
		booked: map[int]int{}, klocks: map[int]*keyLock{}, rr: map[string]int{},
		lastBusy: s.now,
	}
	n.tokens = n.f("burst")
	n.tokensAt = s.now
	s.configStore(n)
	s.nodes[n.id] = n
	s.order = append(s.order, n)
	switch d.Kind {
	case KQueue:
		s.hasQueue = true
	}
	if d.Kind == KClient {
		if n.s("workload") == "booking" {
			s.hasBooking = true
		}
		if n.s("consistency") != "eventual" {
			s.hasFresh = true
		}
	}
	return n
}

func (s *Sim) configStore(n *node) {
	if n.kind != KCache && n.kind != KCDN {
		return
	}
	pol := "lru"
	if n.kind == KCache {
		pol = n.s("eviction")
	}
	capItems := int(n.f("capacityItems"))
	if n.store == nil || n.storePol != pol {
		n.store = NewStore(pol, capItems, n.f("ttlSec")*1000, s.rng)
		n.storePol = pol
		return
	}
	n.c.evict += n.store.SetCapacity(capItems)
	n.store.SetTTL(n.f("ttlSec") * 1000)
}

func (s *Sim) addEdge(d Edge) {
	from, to := s.nodes[d.From], s.nodes[d.To]
	if from == nil || to == nil {
		return
	}
	e := &edge{id: d.ID, from: from, to: to, latency: d.LatencyMs}
	s.edges[e.id] = e
	s.eord = append(s.eord, e)
	from.out = append(from.out, e)
	to.in = append(to.in, e)
}

func (s *Sim) removeEdge(e *edge) {
	e.removed = true
	delete(s.edges, e.id)
	drop := func(list []*edge) []*edge {
		out := list[:0]
		for _, x := range list {
			if x != e {
				out = append(out, x)
			}
		}
		return out
	}
	e.from.out = drop(e.from.out)
	e.to.in = drop(e.to.in)
}

func (s *Sim) removeNode(n *node) {
	n.removed = true
	delete(s.nodes, n.id)
	for _, e := range append([]*edge(nil), n.out...) {
		s.removeEdge(e)
	}
	for _, e := range append([]*edge(nil), n.in...) {
		s.removeEdge(e)
	}
	s.failQueued(n, RNodeDown)
	s.closePools(n)
	s.dropConns(n)
	n.arrivalGen++
}

func (s *Sim) compact() {
	o := s.order[:0]
	for _, n := range s.order {
		if !n.removed {
			o = append(o, n)
		}
	}
	s.order = o
	e := s.eord[:0]
	for _, x := range s.eord {
		if !x.removed {
			e = append(e, x)
		}
	}
	s.eord = e
}

func (s *Sim) afterTopologyChange() {
	for _, n := range s.order {
		if n.removed {
			continue
		}
		s.prunePools(n)
		for _, e := range n.out {
			e.brState, e.brFails, e.trial = 0, 0, false
		}
	}
}

// Sync applies an edited diagram to a running simulation without restarting it.
func (s *Sim) Sync(d Diagram) {
	want := map[string]bool{}
	for _, dn := range d.Nodes {
		want[dn.ID] = true
		if n := s.nodes[dn.ID]; n != nil && n.kind == dn.Kind {
			n.label, n.x, n.y = dn.Label, dn.X, dn.Y
			n.p = Resolve(dn.Kind, dn.Params)
			s.configStore(n)
			if n.kind == KRateLimiter {
				n.tokens = math.Min(n.tokens, n.f("burst"))
			}
			if n.kind == KClient && n.s("workload") == "booking" {
				s.hasBooking = true
			}
			if n.kind == KClient && n.s("consistency") != "eventual" {
				s.hasFresh = true
			}
		} else {
			if n != nil {
				s.removeNode(n)
			}
			nn := s.addNode(dn)
			if nn.kind == KClient {
				s.startClient(nn)
			}
		}
	}
	for _, n := range append([]*node(nil), s.order...) {
		if !n.removed && !want[n.id] {
			s.removeNode(n)
		}
	}
	wantE := map[string]Edge{}
	for _, de := range d.Edges {
		wantE[de.ID] = de
	}
	for _, e := range append([]*edge(nil), s.eord...) {
		if de, ok := wantE[e.id]; !e.removed && (!ok || de.From != e.from.id || de.To != e.to.id) {
			s.removeEdge(e)
		}
	}
	for _, de := range d.Edges {
		if e := s.edges[de.ID]; e != nil {
			e.latency = de.LatencyMs
		} else {
			s.addEdge(de)
		}
	}
	s.compact()
	s.afterTopologyChange()
	for _, n := range s.order {
		s.startNext(n)
		s.pumpAll(n)
	}
}

// ---- chaos --------------------------------------------------------------------

func (s *Sim) fireChaos(ev ChaosEvent) {
	var t *node
	if len(ev.NodeID) > 7 && ev.NodeID[:7] == "random:" {
		kind := ev.NodeID[7:]
		var c []*node
		for _, n := range s.order {
			if n.kind == kind && !n.down && !n.removed {
				c = append(c, n)
			}
		}
		if len(c) == 0 {
			return
		}
		t = c[int(s.chaosRng.Next()*float64(len(c)))]
	} else {
		t = s.nodes[ev.NodeID]
	}
	if t == nil || t.removed {
		return
	}
	s.chaosLog = append(s.chaosLog, ChaosRecord{T: s.now, Type: ev.Type, NodeID: t.id, Label: t.label})
	if ev.Type == "crash" {
		s.CrashNode(t.id, ev.DownSec)
		return
	}
	t.slow = math.Max(1, ev.Factor)
	t.slowUntil = s.now + ev.DurationSec*1000
	until := t.slowUntil
	s.at(until, func() {
		if s.now >= t.slowUntil && t.slowUntil == until {
			t.slow = 1
		}
	})
}

// CrashNode takes a node down; if downSec > 0 it restarts automatically.
func (s *Sim) CrashNode(id string, downSec float64) {
	n := s.nodes[id]
	if n == nil || n.down {
		return
	}
	n.down, n.downAt = true, s.now
	n.epoch++
	s.failQueued(n, RNodeDown)
	s.closePools(n)
	s.dropConns(n)
	if n.kind == KRedis {
		n.locks = map[int]redisLock{}
	}
	if downSec > 0 {
		ep := n.epoch
		s.after(downSec*1000, func() {
			if n.down && n.epoch == ep {
				s.RestartNode(n.id)
			}
		})
	}
}

// RestartNode brings a crashed node back.
func (s *Sim) RestartNode(id string) {
	n := s.nodes[id]
	if n == nil || !n.down {
		return
	}
	n.down = false
	n.slow = 1
	if n.kind == KClient {
		s.startClient(n)
	}
	s.startNext(n)
	if n.kind == KQueue {
		s.dispatch(n)
	}
}

// ChaosLog returns chaos events that have fired so far.
func (s *Sim) ChaosLog() []ChaosRecord { return s.chaosLog }

// ---- clients ------------------------------------------------------------------

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
	period := math.Max(0.001, n.f("periodSec"))
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
	s.nextArrival(n, n.arrivalGen)
}

func (s *Sim) nextArrival(n *node, gen int) {
	if gen != n.arrivalGen || n.removed || n.down || s.trafficStopped() {
		return
	}
	r := s.rate(n, s.now)
	if r <= 0 {
		s.after(100, func() { s.nextArrival(n, gen) })
		return
	}
	s.after(s.rng.Exp()/r*1000, func() {
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
)

func workloadOf(n *node) int {
	switch n.s("workload") {
	case "geo":
		return wlGeo
	case "leaderboard":
		return wlLeader
	case "booking":
		return wlBooking
	}
	return wlKV
}

func (s *Sim) createLogical(c *node) {
	s.logicalSeq++
	lg := &logical{
		id: s.logicalSeq, client: c, createdAt: s.now,
		write:   s.rng.Next() >= c.f("readPct")/100,
		counted: s.now >= s.opts.WarmupMs,
		hash:    Hash32(uint32(s.logicalSeq)),
		wl:      workloadOf(c),
	}
	lg.key = c.keys.Sample(c.s("keyDist"), int(c.f("keySpace")), c.f("zipfS"), c.f("hotKeyPct"), s.keyRng)
	s.unresolved++
	if lg.counted {
		s.runPending++
	}
	s.created.Push(s.now, true, 0)
	s.startAttempt(lg)
}

func (s *Sim) startAttempt(lg *logical) {
	if lg.resolved {
		return
	}
	c := lg.client
	e := s.pickCyclic(c, "out", c.out, nil)
	if e == nil {
		s.resolve(lg, Result{Reason: RNoRoute})
		return
	}
	lg.attempts++
	s.attempts.Push(s.now, true, 0)
	at := &attempt{lg: lg, at: s.now, n: lg.attempts}
	call := &call{lg: lg, at: at, write: lg.write, key: lg.key, wl: lg.wl, hash: lg.hash, rid: lg.id, counted: lg.counted, ver: -1}
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

func (s *Sim) attemptFailed(at *attempt, r Reason) {
	lg := at.lg
	if lg.resolved {
		return
	}
	cp := lg.client.p
	if lg.attempts < 1+int(pf(cp, "retries", 0)) {
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
	s.unresolved--
	lat := s.now - lg.createdAt
	s.res.Push(s.now, res.OK, lat)
	cl := lg.client
	if res.OK {
		cl.c.ok++
	} else {
		cl.c.err++
		cl.c.reason[res.Reason]++
	}
	if lg.counted {
		s.runPending--
		s.runResolved++
		if res.OK {
			s.runOK++
			s.hist.Add(lat)
			s.countFreshness(lg, res)
		} else {
			s.runFailed++
			s.runErrors[res.Reason]++
		}
	}
}

// ---- versions / freshness -------------------------------------------------------

func (s *Sim) itemVer(key int) int {
	if v := s.vers[key]; v != nil {
		return v.v[len(v.v)-1]
	}
	return 0
}

func (s *Sim) verAt(key int, t float64) int {
	v := s.vers[key]
	if v == nil {
		return 0
	}
	for i := len(v.t) - 1; i >= 0; i-- {
		if v.t[i] <= t {
			return v.v[i]
		}
	}
	return int(math.Max(0, float64(v.v[0]-1)))
}

func (s *Sim) bumpVer(key int) int {
	v := s.vers[key]
	if v == nil {
		v = &verLog{}
		s.vers[key] = v
	}
	n := 1
	if len(v.v) > 0 {
		n = v.v[len(v.v)-1] + 1
	}
	v.v = append(v.v, n)
	v.t = append(v.t, s.now)
	if len(v.v) > verKeep {
		v.v, v.t = v.v[1:], v.t[1:]
	}
	return n
}

func maxStale(cl *node) float64 {
	switch cl.s("consistency") {
	case "strong":
		return 0
	case "bounded":
		return math.Max(0, cl.f("maxStaleMs"))
	}
	return -1
}

func (s *Sim) countFreshness(lg *logical, res Result) {
	if lg.write || res.Ver < 0 {
		return
	}
	ms := maxStale(lg.client)
	if ms < 0 {
		return
	}
	s.runFresh++
	if s.verAt(lg.key, lg.createdAt-ms) > res.Ver {
		s.runStale++
		if res.Src != nil {
			res.Src.c.stale++
		}
	}
}

func (s *Sim) commit(n *node, c *call) int {
	if c.rid != 0 {
		if _, seen := s.committed[c.rid]; seen {
			if c.idem {
				return s.itemVer(c.key)
			}
			n.c.dup++
			if c.counted {
				s.runDup++
			}
		} else {
			s.committed[c.rid] = s.now
			s.commitLog = append(s.commitLog, commitRec{c.rid, s.now})
		}
	}
	return s.bumpVer(c.key)
}

func (s *Sim) pruneCommitted() {
	cut := s.now - commitKeep
	for s.commitHd < len(s.commitLog) && s.commitLog[s.commitHd].t < cut {
		delete(s.committed, s.commitLog[s.commitHd].rid)
		s.commitHd++
	}
	if s.commitHd > 4096 && s.commitHd > len(s.commitLog)/2 {
		s.commitLog = append([]commitRec(nil), s.commitLog[s.commitHd:]...)
		s.commitHd = 0
	}
}

// ---- tick ---------------------------------------------------------------------

func (s *Sim) tick() {
	i := s.bi
	for _, n := range s.order {
		if n.removed {
			continue
		}
		s.markBusy(n)
		capN := float64(s.capacity(n))
		switch {
		case n.kind == KQueue:
			n.util[i] = float64(len(n.jobs)-n.jh) / math.Max(1, n.f("capacity"))
		case n.kind == KRateLimiter:
			n.util[i] = math.Min(1, float64(n.c.adm)/math.Max(1e-9, n.f("limitRps")*tickMs/1000))
		case n.kind == KClient || n.kind == KLoadBalancer || capN > 1e6:
			n.util[i] = 0
		default:
			n.util[i] = math.Min(1, (n.busyArea-n.areaMark)/(capN*tickMs))
		}
		n.areaMark = n.busyArea
		n.ring[i] = n.c
		n.c = counters{}
	}
	for _, e := range s.eord {
		e.fwdRing[i] = e.fwd
		e.fwd = 0
	}
	s.bi = (i + 1) % ringSize
	s.tickCount++
	slot := s.tickCount % 20
	for _, e := range s.eord {
		if e.from.kind == KLoadBalancer {
			e.oCnt[slot], e.oSum[slot], e.oErr[slot] = 0, 0, 0
		}
	}
	for _, n := range s.order {
		if n.kind == KLoadBalancer && n.b("outlierEjection") && !n.down {
			s.ejectOutliers(n)
		}
		if n.kind == KQueue && !n.down {
			s.dispatch(n)
		}
	}
	if s.tickCount%10 == 0 {
		s.pruneCommitted()
	}
	cut := s.now - statsWindow
	s.res.Prune(cut)
	s.created.Prune(cut)
	s.attempts.Prune(cut)
	s.jobsW.Prune(cut)
	s.conflictsW.Prune(cut)
	if s.tickCount%5 == 0 {
		w := s.res.Stats(s.now - 1000)
		succ := 1.0
		if w.OK+w.Failed > 0 {
			succ = float64(w.OK) / float64(w.OK+w.Failed)
		}
		s.series = append(s.series, SeriesPoint{T: s.now / 1000, OkRps: float64(w.OK), ErrRps: float64(w.Failed), P50: w.P50, P95: w.P95, Success: succ, InFly: s.unresolved})
		if len(s.series) > seriesCap {
			s.series = s.series[len(s.series)-seriesCap:]
		}
	}
	s.at(s.now+tickMs, s.tick)
}

// Finished reports whether a scenario run has stopped traffic and drained all work.
func (s *Sim) Finished() bool {
	if s.opts.StopTrafficAtMs <= 0 {
		return false
	}
	if s.now < s.opts.StopTrafficAtMs {
		return false
	}
	if s.unresolved == 0 && s.backlog() == 0 {
		return true
	}
	return s.now > s.opts.StopTrafficAtMs+30000
}

func (s *Sim) backlog() int { return s.jobEnqueued - s.jobCompleted - s.jobLost }
