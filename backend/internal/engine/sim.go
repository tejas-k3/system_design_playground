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
	hit, miss, evict, expired, localHit  int
	connects                             int
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
	c.localHit += o.localHit
	c.connects += o.connects
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
	fence int // the fencing token issued with the grant (coordinator locks only)
}

// bucket is one account's token bucket in a per-user rate limiter.
type bucket struct {
	tokens, at float64
	winStart   float64   // window algorithms: when the current window began
	count      float64   // requests admitted in the current window
	prev       float64   // requests admitted in the previous window
	log        []float64 // sliding log: the times of admitted requests still inside the window
}

type job struct {
	rid      int
	key      int
	wl       int
	idem     bool
	counted  bool
	enq      float64
	attempts int
	lane     *lane // a partitioned-log delivery: the (partition, group) line it is in
	seq      int   // publish order, for checking that a consumer saw one key's messages in order
	sub      *edge // topic delivery: the subscription (edge to a worker) it belongs to
	high     bool  // an urgent request: first in line in a priority queue
	finished bool  // a worker has acknowledged it (or given up): a later visibility timeout does nothing
	ghost    bool  // a redelivered copy of a job still being worked on: it does the work again but is not counted again
}

type mlWrite struct {
	t      float64
	leader int
}

type node struct {
	backups         []float64       // database: when each backup finished
	mlLast          map[int]mlWrite // multi-leader database: the latest write of each key and where it landed
	mlConflicts     int
	writeLog        []float64 // database that a restore is planned for: when each write was committed
	logWrites       bool
	corruptAt       float64          // database: when silent corruption began (0 = none)
	sessWrite       map[int]int      // client: the newest version of each key this client has had acknowledged for its own writes
	sessRead        map[int]int      // client: the newest version of each key this client has read
	kvFree          [][]float64      // quorum store: when each slot of each cluster node is next free
	kvVer           []map[int]int    // quorum store: the version of each key each cluster node holds
	kvDownUntil     []float64        // quorum store: when each cluster node comes back (0 = up)
	kvHints         map[int][][2]int // quorum store: writes held for a node that is down (key, version), by that node
	kvHintsReplayed int
	kvRepairs       int
	coordDown       []float64 // coordinator: when each cluster node comes back (0 = up)
	coordLeader     int       // coordinator: the leader, -1 while there is none
	coordTerm       int
	coordElecting   bool
	outboxBacklog   int               // events saved with the data and not yet taken by the queue
	coordLocks      map[int]redisLock // coordinator: the lock on each key
	coordFence      int               // coordinator: the last fencing token handed out
	fenceSeen       map[int]int       // database: the newest fencing token each key has accepted
	hashRing        *ring             // consistent-hashing ring of a shard router
	assign          []string          // where a sample of keys goes now (shard router), to measure how many move on a change
	shardCount      int
	keysMoved       float64 // share of the sample that moved at the last change of the children
	movedKeys       []bool  // shard router: which sampled keys changed shard at the last change
	moveUntil       float64 // shard router: the data of those keys is still moving until then
	slowedMoves     int
	reshards        int
	sessConns       int // long-lived connections held open (sessions arrival)
	sessions        map[*session]bool
	id, kind, label string
	x, y            float64
	p               Params
	removed         bool
	canaryAcc       float64 // load balancer: share of a request owed to the canary so far
	retryFirsts     float64 // decayed count of first attempts, for the retry budget
	retryRetries    float64 // decayed count of retries taken
	retryAt         float64
	idBase          int             // users of this client are numbered from here, so two clients never share an account
	userKeys        KeySampler      // samples which account sends a request
	global          *bucket         // the single allowance of a window-based limiter that is not per user
	buckets         map[int]*bucket // a per-user rate limiter's allowance for each account seen so far
	badUntil        float64         // a bad release answers every request with an error until then (0 = none)
	zone            string          // availability zone, kept in step with the 'zone' setting
	down            bool
	downAt          float64
	epoch           int
	slow            float64
	slowUntil       float64
	pausedUntil     float64     // a stop-the-world pause (GC) lasts until then; 0 = none
	cpuWork         float64     // CPU-ms of work started since the last burst-credit step
	credits         float64     // banked full-speed CPU-ms of a burstable server
	throttled       bool        // a burstable server with no credits left runs at its baseline
	diskFree        float64     // when the disk finishes the IO already queued on it
	stallUntil      float64     // an LSM database's compaction holds writes until then; 0 = none
	cut             bool        // partitioned: calls to it get no answer
	promoted        bool        // replica that took over as the primary after a failover
	as              scaler      // app server autoscaling state
	hist            []NodePoint // one point per second for the node timelines
	successor       *node       // database: the replica that replaced it
	sincePromotion  int         // writes a promoted replica has accepted since it took over
	held            []func()    // calls waiting out a partition, delivered when it heals
	out, in         []*edge

	busy     int
	q        []*task
	qh       int
	busyArea float64
	lastBusy float64
	areaMark float64

	// client
	arrivalGen int
	burstOn    bool    // bursty client: inside a rush now
	burstUntil float64 // bursty client: when the current phase ends
	keys       KeySampler

	// cache / cdn
	store    *Store
	storePol string
	local    *Store                 // hot-key local cache on an app server
	fetching map[int][]func(Result) // cache: waiters sharing an in-flight miss fetch, by key (coalescing)

	// rate limiter
	tokens   float64
	tokensAt float64

	// database
	openConns int
	booked    map[int]int
	klocks    map[int]*keyLock
	tver      map[int]int // transfers: version of each account, bumped by every commit
	txl       map[int]*txLock
	txWaits   map[int]int // transaction -> the account it is waiting for (for deadlock detection)

	// app / worker
	pools map[*node]*pool

	// redis
	locks       map[int]redisLock
	egressBytes float64   // object store: bytes served to downloads (billed)
	osRequests  int       // object store: requests served (billed per 1,000)
	rwTotal     int       // counted writes since the process last started
	rwRecent    []float64 // times of counted writes in the last second (what an append-only file could still lose)

	// queue
	jobs    []*job
	jh      int
	lanes   []*lane // a log: one line per (partition, consumer group), in creation order
	laneIdx map[[2]int]*lane
	rrNext  int            // round-robin partition choice
	lastSeq map[[2]int]int // per (group, key): the newest message a consumer has finished
	hjobs   []*job         // urgent jobs of a priority queue, served before jobs
	hh      int
	delayed int // jobs held back by the queue's delivery delay

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
	bulk      int    // calls in flight over this connection, for the caller's bulkhead
	subJobs   []*job // topic subscription: deliveries waiting for this edge's worker
	subHead   int
	downReq   bool // linkDown: requests on this connection are held until it heals
	downResp  bool // linkDown: answers are held until it heals
	held      []func()
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
	// link bandwidth: 0 = unlimited. Each direction is a first-in-first-out transmitter that is busy until txFree.
	loss, jitter float64 // see Edge.LossPct, Edge.JitterMs
	bw           float64
	txFree       [2]float64
	bits         float64
	bitsRing     [ringSize]float64
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
	failHist                                  Hist

	runResolved, runOK, runFailed, runPending int
	runErrors                                 [NumReasons]int
	runConflicts, runFresh, runStale, runDup  int
	runSessReads, runRYW, runMono             int
	shardAdds                                 int
	runEventsLost                             int
	runRPO, runRTO                            float64 // seconds of data lost and seconds without the database at the last restore
	runPartial, runHedged                     int
	runFanWrites, runFanReads                 int
	runWriteSkew                              int
	runLostUpdates, runTxAborts, runDeadlocks int
	txnSeq                                    int
	runLost                                   int // writes lost when an in-memory store crashed

	jobEnqueued, jobCompleted, jobLost, jobRetried int
	jobDead, jobRedelivered                        int
	jobOrderViolations, logSeq                     int
	runDeadlineSkipped                             int
	runHighOK, runHighTotal                        int
	sharedLimit                                    *node // the first limiter that shares its count: its bucket is everyone's
	runCustOK, runCustTotal                        int   // requests of clients that are not bots (only counted when a bot exists)
	hasAbuse                                       bool
	lightHist, heavyHist                           Hist // latencies of light and heavy requests when the users mix both
	jobHist, jobHistHigh                           Hist
	hasQueue, hasBooking, hasFresh                 bool

	vers      map[int]*verLog
	committed map[int]float64
	commitLog []commitRec
	commitHd  int

	tickCount int
	bi        int
	series    []SeriesPoint
	chaosLog  []ChaosRecord
	events    []Event // everything worth a marker on the charts (capped)

	born       []Particle
	bornOn     bool
	traceOn    bool
	traceEvery int
	failedTr   []*tracer
	okTr       []*tracer
	stride     int
	hopCount   int
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
	s.noteReshard()
	for _, n := range s.order {
		if n.kind == KAppServer && n.f("gcPauseMs") > 0 {
			s.scheduleGC(n)
		}
		if n.kind == KAppServer && n.f("burstBaselinePct") < 100 {
			n.credits = n.f("burstCreditSec") * 1000 * float64(s.capacity(n))
			s.scheduleBurst(n)
		}
		if n.kind == KDatabase && n.s("engine") == "lsm" {
			s.scheduleCompaction(n)
		}
		if n.kind == KDatabase && n.f("backupEverySec") > 0 {
			s.scheduleBackup(n)
		}
	}
	for _, ev := range opts.Chaos {
		ev := ev
		if ev.Type == "restore" && s.nodes[ev.NodeID] != nil { // count the writes a restore will lose exactly
			s.nodes[ev.NodeID].logWrites = true
		}
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
		s.autoscale(n)
		if n.kind == KAppServer && !n.down && n.f("memLimitMB") > 0 {
			held := float64(n.busy+len(n.q)-n.qh) / float64(s.instancesOf(n))
			conns := float64(n.sessConns) / float64(s.instancesOf(n)) * n.f("memPerConnKB") / 1024
			if n.f("memBaseMB")+n.f("memPerReqMB")*held+conns > n.f("memLimitMB") {
				s.logEvent("oom", n, 0)
				s.CrashNode(n.id, n.f("memRestartSec"))
			}
		}
		capN := float64(s.capacity(n))
		switch {
		case n.kind == KQueue || n.kind == KTopic || n.kind == KLog:
			n.util[i] = float64(s.queued(n)) / math.Max(1, n.f("capacity"))
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
		e.bitsRing[i] = e.bits
		e.bits = 0
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
		if (n.kind == KQueue || n.kind == KTopic || n.kind == KLog) && !n.down {
			s.dispatch(n)
		}
	}
	if s.tickCount%10 == 0 {
		s.pruneCommitted()
		s.recordNodePoints()
	}
	s.retuneTraceRate()
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

func (s *Sim) backlog() int { return s.jobEnqueued - s.jobCompleted - s.jobLost - s.jobDead }

// service draws a service time for one unit of work at node n, honouring its "variability" setting.
func (s *Sim) service(n *node, mean float64) float64 {
	mode, _ := n.p["variability"].(string)
	return s.rng.ServiceDist(mean, mode)
}

// logEvent records something that happened to node n at the current time (see Event).
func (s *Sim) logEvent(typ string, n *node, lost int) {
	s.events = append(s.events, Event{T: s.now / 1000, Type: typ, NodeID: n.id, Label: n.label, Lost: lost})
	if len(s.events) > maxEvents {
		s.events = append([]Event(nil), s.events[len(s.events)-maxEvents:]...)
	}
}
