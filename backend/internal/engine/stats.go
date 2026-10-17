package engine

import "math"

// NodeStats is the per-node view the UI draws.
type NodeStats struct {
	ID      string             `json:"id"`
	Arr     float64            `json:"arr"`
	Ok      float64            `json:"ok"`
	Err     float64            `json:"err"`
	Util    float64            `json:"util"`
	Busy    int                `json:"busy"`
	Cap     int                `json:"cap"`
	QLen    int                `json:"qLen"`
	HitRate float64            `json:"hitRate"` // -1 when not applicable
	Size    int                `json:"size,omitempty"`
	Reasons map[string]float64 `json:"reasons,omitempty"`
	Down    bool               `json:"down,omitempty"`
	Cut     bool               `json:"cut,omitempty"`     // partitioned from the network
	Primary bool               `json:"primary,omitempty"` // a replica that took over as the primary
	Slow    float64            `json:"slow,omitempty"`
	Extra   map[string]float64 `json:"extra,omitempty"`
}

type EdgeStats struct {
	ID      string  `json:"id"`
	Rps     float64 `json:"rps"`
	Breaker string  `json:"breaker,omitempty"`
	Ejected bool    `json:"ejected,omitempty"`
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
type RunStats struct {
	Resolved   int            `json:"resolved"`
	Pending    int            `json:"pending"`
	OK         int            `json:"ok"`
	Failed     int            `json:"failed"`
	P50        float64        `json:"p50"`
	P95        float64        `json:"p95"`
	P99        float64        `json:"p99"`
	FailP50    float64        `json:"failP50"` // median time to failure; the latencies above count successes only
	Mean       float64        `json:"mean"`
	Errors     map[string]int `json:"errors"`
	Conflicts  int            `json:"conflicts"`
	Fresh      int            `json:"freshReads"`
	Stale      int            `json:"staleReads"`
	Dups       int            `json:"duplicates"`
	LostWrites int            `json:"lostWrites"`
	Jobs       *JobStats      `json:"jobs,omitempty"`
}

type GlobalStats struct {
	Now      float64  `json:"now"` // seconds
	InFlight int      `json:"inFlight"`
	Cost     float64  `json:"cost"` // dollars per hour of the current diagram
	Run      RunStats `json:"run"`
}

// Stats is a complete snapshot for the UI.
type Stats struct {
	Global GlobalStats          `json:"global"`
	Nodes  map[string]NodeStats `json:"nodes"`
	Edges  map[string]EdgeStats `json:"edges"`
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
		var sum counters
		for i := range n.ring {
			sum.add(&n.ring[i])
		}
		var u float64
		for _, v := range n.util {
			u += v
		}
		u /= ringSize
		ns := NodeStats{
			ID: n.id, Arr: float64(sum.arr) / span, Ok: float64(sum.ok) / span, Err: float64(sum.err) / span,
			Util: math.Min(1, u), Busy: n.busy, QLen: len(n.q) - n.qh, HitRate: -1, Down: n.down, Cut: n.cut, Primary: n.promoted,
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
		if e.from.kind == KAppServer && e.from.b("circuitBreaker") {
			es.Breaker = [...]string{"closed", "open", "halfOpen"}[s.breakerState(e)]
		}
		st.Edges[e.id] = es
	}
	run := RunStats{
		Resolved: s.runResolved, Pending: s.runPending, OK: s.runOK, Failed: s.runFailed,
		P50: s.hist.Quantile(0.5), P95: s.hist.Quantile(0.95), P99: s.hist.Quantile(0.99), Mean: s.hist.Mean(), FailP50: s.failHist.Quantile(0.5),
		Errors: map[string]int{}, Conflicts: s.runConflicts, Fresh: s.runFresh, Stale: s.runStale, Dups: s.runDup, LostWrites: s.runLost,
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
			cost += Spec(n.kind).cost(n.p)
		}
	}
	st.Global = GlobalStats{Now: s.now / 1000, InFlight: s.unresolved, Cost: cost, Run: run}
	st.Series = append(make([]SeriesPoint, 0, len(s.series)), s.series...) // never nil: JSON must be [] not null
	return st
}

// RunSnapshot returns just the run totals (cheap, used by scoring).
func (s *Sim) RunSnapshot() RunStats { return s.Snapshot().Global.Run }
