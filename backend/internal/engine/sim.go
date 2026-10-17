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
	store    *Store
	storePol string

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
	locks    map[int]redisLock
	rwTotal  int       // counted writes since the process last started
	rwRecent []float64 // times of counted writes in the last second (what an append-only file could still lose)

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
	runLost                                   int // writes lost when an in-memory store crashed

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

	born     []Particle
	bornOn   bool
	stride   int
	hopCount int
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
