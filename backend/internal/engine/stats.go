package engine

import "math"

// NodeStats is the per-node view the UI draws.
type NodeStats struct {
	ID        string             `json:"id"`
	Arr       float64            `json:"arr"`
	Ok        float64            `json:"ok"`
	Err       float64            `json:"err"`
	Util      float64            `json:"util"`
	Busy      int                `json:"busy"`
	Cap       int                `json:"cap"`
	QLen      int                `json:"qLen"`
	HitRate   float64            `json:"hitRate"` // -1 when not applicable
	Size      int                `json:"size,omitempty"`
	Reasons   map[string]float64 `json:"reasons,omitempty"`
	Down      bool               `json:"down,omitempty"`
	Cut       bool               `json:"cut,omitempty"`          // partitioned from the network
	Primary   bool               `json:"primary,omitempty"`      // a replica that took over as the primary
	Phase     string             `json:"phase,omitempty"`        // failover in progress on a crashed database: detecting | promoting
	PhaseLeft float64            `json:"phaseLeftSec,omitempty"` // seconds until that phase ends
	Booting   int                `json:"booting,omitempty"`      // autoscaled app server: servers still booting
	Instances int                `json:"instances,omitempty"`    // autoscaled app server: instances running now
	Slow      float64            `json:"slow,omitempty"`
	Extra     map[string]float64 `json:"extra,omitempty"`
}

type EdgeStats struct {
	ID      string  `json:"id"`
	Rps     float64 `json:"rps"`
	Breaker string  `json:"breaker,omitempty"`
	Ejected bool    `json:"ejected,omitempty"`
	Mbps    float64 `json:"mbps,omitempty"` // data carried over the window, both directions (only when sizes are set)
	Util    float64 `json:"util,omitempty"` // Mbps as a share of the link's bandwidth, if it has a limit
}

type JobStats struct {
	Enqueued  int     `json:"enqueued"`
	Completed int     `json:"completed"`
	Lost      int     `json:"lost"`
	Retried   int     `json:"retried"`
	Backlog   int     `json:"backlog"`
	P50       float64 `json:"p50"`
	P95       float64 `json:"p95"`
}

// RunStats are totals over the counted part of the run.
// LatencyBucket is one bar of the latency distribution: successes with latency up to LeMs (the last bucket has LeMs 1e9 = "and above").
type LatencyBucket struct {
	LeMs  float64 `json:"leMs"`
	Count int     `json:"count"`
}

type RunStats struct {
	Resolved       int             `json:"resolved"`
	Pending        int             `json:"pending"`
	OK             int             `json:"ok"`
	Failed         int             `json:"failed"`
	P50            float64         `json:"p50"`
	P95            float64         `json:"p95"`
	P99            float64         `json:"p99"`
	LatencyBuckets []LatencyBucket `json:"latencyBuckets"` // distribution of successful requests
	FailP50        float64         `json:"failP50"`        // median time to failure; the latencies above count successes only
	Mean           float64         `json:"mean"`
	Errors         map[string]int  `json:"errors"`
	Conflicts      int             `json:"conflicts"`
	Fresh          int             `json:"freshReads"`
	Stale          int             `json:"staleReads"`
	Dups           int             `json:"duplicates"`
	FanoutWrites   int             `json:"fanoutWrites,omitempty"` // timeline inserts caused by posts (feed workload)
	FanoutReads    int             `json:"fanoutReads,omitempty"`  // followed authors read to build timelines
	Hedged         int             `json:"hedged,omitempty"`       // second queries sent because a shard was slow
	Partial        int             `json:"partial,omitempty"`      // answers sent with fewer shards than required (partial results)
	LostWrites     int             `json:"lostWrites"`
	LostUpdates    int             `json:"lostUpdates,omitempty"` // transfers that silently overwrote a concurrent commit (read committed)
	TxAborts       int             `json:"txAborts,omitempty"`    // transactions aborted for a write conflict or as a deadlock victim
	Deadlocks      int             `json:"deadlocks,omitempty"`
	WriteSkew      int             `json:"writeSkew,omitempty"` // on-call pairs left with the rule broken (two doctors off at once)
	Jobs           *JobStats       `json:"jobs,omitempty"`
}

// Event is one thing that happened to the system, for placing on the charts: crashes and restarts, slow-downs and
// partitions starting and ending, failover steps, autoscaling steps. T is in seconds, like the series.
type Event struct {
	T      float64 `json:"t"`
	Type   string  `json:"type"` // crash restart slow slowEnd partition heal failoverDetected promotion splitbrain scaleUp serverUp scaleDown
	NodeID string  `json:"nodeId"`
	Label  string  `json:"label"`
	Lost   int     `json:"lost,omitempty"` // promotion and split brain: acknowledged writes lost
}

const maxEvents = 200

type GlobalStats struct {
	Now      float64  `json:"now"` // seconds
	InFlight int      `json:"inFlight"`
	Cost     float64  `json:"cost"`    // dollars per hour of the current diagram, with the servers running now
	AvgCost  float64  `json:"avgCost"` // the same averaged over the run so far: what a challenge scores
	Run      RunStats `json:"run"`
}

// Stats is a complete snapshot for the UI.
type Stats struct {
	Global GlobalStats          `json:"global"`
	Nodes  map[string]NodeStats `json:"nodes"`
	Edges  map[string]EdgeStats `json:"edges"`
	Events []Event              `json:"events,omitempty"` // omitted by the session when unchanged since the last frame
	Series []SeriesPoint        `json:"series"`
}

func (s *Sim) ringSpan() float64 {
	t := s.tickCount
	if t > ringSize {
		t = ringSize
	}
	if t == 0 {
		return 1
	}
	return float64(t) * tickMs / 1000
}

// Snapshot builds the current stats. Series is a copy.
func (s *Sim) Snapshot() Stats {
	span := s.ringSpan()
	st := Stats{Nodes: map[string]NodeStats{}, Edges: map[string]EdgeStats{}}
	for _, n := range s.order {
		if n.removed {
			continue
		}
		sum, u := s.nodeWindow(n)
		ns := NodeStats{
			ID: n.id, Arr: float64(sum.arr) / span, Ok: float64(sum.ok) / span, Err: float64(sum.err) / span,
			Util: math.Min(1, u), Busy: n.busy, QLen: len(n.q) - n.qh, HitRate: -1, Down: n.down, Cut: n.cut, Primary: n.promoted, Instances: s.scaledInstances(n),
		}
		ns.Phase, ns.PhaseLeft = s.failoverPhase(n)
		if s.autoscaled(n) {
			ns.Booting = n.as.pending
		}
		if c := s.capacity(n); c < 1<<20 {
			ns.Cap = c
		}
		if n.kind == KQueue {
			ns.QLen = len(n.jobs) - n.jh
			ns.Cap = int(n.f("capacity"))
		}
		if n.slow > 1 {
			ns.Slow = n.slow
		}
		if sum.hit+sum.miss > 0 {
			ns.HitRate = float64(sum.hit) / float64(sum.hit+sum.miss)
		}
		if n.store != nil {
			ns.Size = n.store.Size()
		}
		for r := Reason(0); r < NumReasons; r++ {
			if v := sum.reason[r]; v > 0 {
				if ns.Reasons == nil {
					ns.Reasons = map[string]float64{}
				}
				ns.Reasons[r.String()] = float64(v) / span
			}
		}
		ex := map[string]float64{}
		if n.kind == KAppServer || n.kind == KWorker {
			ex["poolWaiting"] = float64(s.poolWaiting(n))
		}
		if n.kind == KDatabase || n.kind == KReplica {
			ex["openConns"] = float64(n.openConns)
			ex["maxConns"] = n.f("maxConnections")
		}
		if n.kind == KObjectStore && (n.f("dataGB") > 0 || n.f("growthGBPerDay") > 0) {
			ex["storedGB"] = n.f("dataGB")
			// the average size over the next 365 days, charged by the month
			ex["yearStorageCost"] = (n.f("dataGB") + n.f("growthGBPerDay")*365/2) * n.f("storagePerGBMonth") * 12
		}
		if sum.evict > 0 {
			ex["evictRps"] = float64(sum.evict) / span
		}
		if sum.stale > 0 {
			ex["staleRps"] = float64(sum.stale) / span
		}
		if sum.dup > 0 {
			ex["dupRps"] = float64(sum.dup) / span
		}
		if sum.conflict > 0 {
			ex["conflictRps"] = float64(sum.conflict) / span
		}
		if sum.lockBusy > 0 {
			ex["lockBusyRps"] = float64(sum.lockBusy) / span
		}
		if len(ex) > 0 {
			ns.Extra = ex
		}
		st.Nodes[n.id] = ns
	}
	for _, e := range s.eord {
		var f int
		for _, v := range e.fwdRing {
			f += v
		}
		es := EdgeStats{ID: e.id, Rps: float64(f) / span, Ejected: e.ejectTill > s.now}
		var bits float64
		for _, v := range e.bitsRing {
			bits += v
		}
		if bits > 0 {
			es.Mbps = bits / span / 1e6
			if e.bw > 0 {
				es.Util = math.Min(1, es.Mbps/e.bw)
			}
		}
		if e.from.kind == KAppServer && e.from.b("circuitBreaker") {
			es.Breaker = [...]string{"closed", "open", "halfOpen"}[s.breakerState(e)]
		}
		st.Edges[e.id] = es
	}
	buckets := s.hist.Buckets(LatencyBounds)
	lb := make([]LatencyBucket, len(buckets))
	for i, c := range buckets {
		le := 1e9
		if i < len(LatencyBounds) {
			le = LatencyBounds[i]
		}
		lb[i] = LatencyBucket{LeMs: le, Count: c}
	}
	run := RunStats{LatencyBuckets: lb,
		Resolved: s.runResolved, Pending: s.runPending, OK: s.runOK, Failed: s.runFailed,
		P50: s.hist.Quantile(0.5), P95: s.hist.Quantile(0.95), P99: s.hist.Quantile(0.99), Mean: s.hist.Mean(), FailP50: s.failHist.Quantile(0.5),
		Errors: map[string]int{}, Conflicts: s.runConflicts, Fresh: s.runFresh, Stale: s.runStale, Dups: s.runDup, Partial: s.runPartial, Hedged: s.runHedged, WriteSkew: s.runWriteSkew, FanoutWrites: s.runFanWrites, FanoutReads: s.runFanReads, LostWrites: s.runLost, LostUpdates: s.runLostUpdates, TxAborts: s.runTxAborts, Deadlocks: s.runDeadlocks,
	}
	for r := Reason(0); r < NumReasons; r++ {
		if s.runErrors[r] > 0 {
			run.Errors[r.String()] = s.runErrors[r]
		}
	}
	if s.hasQueue {
		run.Jobs = &JobStats{
			Enqueued: s.jobEnqueued, Completed: s.jobCompleted, Lost: s.jobLost, Retried: s.jobRetried,
			Backlog: s.backlog(), P50: s.jobHist.Quantile(0.5), P95: s.jobHist.Quantile(0.95),
		}
	}
	var cost float64
	for _, n := range s.order {
		if !n.removed {
			cost += Spec(n.kind).cost(n.p) * float64(s.instancesOf(n))
		}
	}
	st.Global = GlobalStats{Now: s.now / 1000, InFlight: s.unresolved, Cost: cost, AvgCost: s.CostPerHour(), Run: run}
	st.Series = append(make([]SeriesPoint, 0, len(s.series)), s.series...) // never nil: JSON must be [] not null
	st.Events = append(make([]Event, 0, len(s.events)), s.events...)
	return st
}

// RunSnapshot returns just the run totals (cheap, used by scoring).
func (s *Sim) RunSnapshot() RunStats { return s.Snapshot().Global.Run }

// nodeWindow sums a node's counters over the stats window (one second) and averages its utilisation.
func (s *Sim) nodeWindow(n *node) (counters, float64) {
	var sum counters
	for i := range n.ring {
		sum.add(&n.ring[i])
	}
	var u float64
	for _, v := range n.util {
		u += v
	}
	return sum, u / ringSize
}

// nodeHistory is how many one-second points of per-node history are kept.
const nodeHistory = 120

// NodePoint is one second of a node's life: for the timelines shown when a component is selected.
type NodePoint struct {
	T         float64 `json:"t"` // seconds
	Util      float64 `json:"util"`
	QLen      int     `json:"qLen"`
	Arr       float64 `json:"arr"`
	Err       float64 `json:"err"`
	Instances int     `json:"instances,omitempty"`
}

// recordNodePoints appends one point per node; called once per simulated second.
func (s *Sim) recordNodePoints() {
	span := s.ringSpan()
	for _, n := range s.order {
		if n.removed {
			continue
		}
		sum, u := s.nodeWindow(n)
		q := len(n.q) - n.qh
		if n.kind == KQueue {
			q = len(n.jobs) - n.jh
		}
		n.hist = append(n.hist, NodePoint{T: s.now / 1000, Util: math.Min(1, u), QLen: q, Arr: float64(sum.arr) / span, Err: float64(sum.err) / span, Instances: s.scaledInstances(n)})
		if len(n.hist) > nodeHistory {
			n.hist = append([]NodePoint(nil), n.hist[len(n.hist)-nodeHistory:]...)
		}
	}
}

// NodeSeries returns the recorded history of one node (empty, never nil, for an unknown node).
func (s *Sim) NodeSeries(id string) []NodePoint {
	n := s.nodes[id]
	if n == nil {
		return []NodePoint{}
	}
	return append([]NodePoint{}, n.hist...)
}

// failoverPhase says where a crashed database is in its failover and how long that step has left.
func (s *Sim) failoverPhase(n *node) (string, float64) {
	if n.kind != KDatabase || !n.down || n.successor != nil || n.s("onFailure") != "promote" {
		return "", 0
	}
	elapsed := (s.now - n.downAt) / 1000
	detect, promote := n.f("detectSec"), n.f("promoteSec")
	switch {
	case elapsed < detect:
		return "detecting", detect - elapsed
	case elapsed < detect+promote:
		return "promoting", detect + promote - elapsed
	}
	return "", 0
}
