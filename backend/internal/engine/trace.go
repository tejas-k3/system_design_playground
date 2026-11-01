package engine

// Request traces: for a sample of requests, record every node the request visits, when it
// arrived, when it started being served (after any queueing) and when it answered. They are
// pure observation (no extra randomness, no extra events) so enabling them never changes a run.

const (
	maxSpans        = 80 // per request; a safety net against runaway fan-out
	keepFailed      = 40 // recent failed requests retained
	keepOK          = 20 // recent successful requests retained
	traceTargetRate = 80 // sample about this many requests per second
	traceReturn     = 20 // most recent traces of each kind returned to a client
)

// span is one visit of a request to a node.
type span struct {
	node           *node
	parent         *span
	depth          int
	attempt        int
	at, start, end float64 // absolute simulation ms
	ok             bool
	reason         Reason
	done           bool
	late           bool // began after the request had already resolved (work kept running)
}

type tracer struct {
	id, key  int
	client   string
	write    bool
	created  float64
	attempts int
	spans    []*span
	resolved bool
	ok       bool
	reason   Reason
	latency  float64
}

// EnableTraces turns request tracing on or off.
func (s *Sim) EnableTraces(on bool) {
	s.traceOn = on
	if s.traceEvery < 1 {
		s.traceEvery = 1
	}
}

// startTrace decides whether to sample this request and returns its tracer (or nil).
func (s *Sim) startTrace(lg *logical) *tracer {
	if !s.traceOn || lg.id%s.traceEvery != 0 {
		return nil
	}
	return &tracer{id: lg.id, key: lg.key, client: lg.client.label, write: lg.write, created: s.now}
}

// retuneTraceRate keeps the sampling near traceTargetRate traced requests per second.
func (s *Sim) retuneTraceRate() {
	if !s.traceOn {
		return
	}
	s.traceEvery = max(1, s.created.Count(s.now-1000)/traceTargetRate)
}

// traceArrive opens a span for this node and returns a callback that closes it.
func (s *Sim) traceArrive(n *node, c *call, cb func(Result)) func(Result) {
	tr := c.tr
	if len(tr.spans) >= maxSpans {
		return cb
	}
	sp := &span{node: n, parent: c.sp, at: s.now, start: s.now, late: tr.resolved}
	if c.at != nil {
		sp.attempt = c.at.n
	}
	if c.sp != nil {
		sp.depth = c.sp.depth + 1
	}
	tr.spans = append(tr.spans, sp)
	c.sp = sp
	return func(res Result) {
		sp.end, sp.ok, sp.reason, sp.done = s.now, res.OK, res.Reason, true
		cb(res)
	}
}

// finishTrace records the outcome and files the trace among the recent failed or successful ones.
func (s *Sim) finishTrace(lg *logical, res Result) {
	tr := lg.tr
	tr.resolved, tr.ok, tr.reason = true, res.OK, res.Reason
	tr.latency = s.now - tr.created
	tr.attempts = lg.attempts
	if res.OK {
		s.okTr = appendCapped(s.okTr, tr, keepOK)
	} else {
		s.failedTr = appendCapped(s.failedTr, tr, keepFailed)
	}
}

func appendCapped(list []*tracer, t *tracer, limit int) []*tracer {
	list = append(list, t)
	if len(list) > limit {
		list = list[len(list)-limit:]
	}
	return list
}

// SpanOut, TraceOut and TraceSet are the JSON view of traces. Times are milliseconds since the
// request was created.
type SpanOut struct {
	Node    string  `json:"node"`
	Label   string  `json:"label"`
	Kind    string  `json:"kind"`
	Depth   int     `json:"depth"`
	Attempt int     `json:"attempt"`
	At      float64 `json:"at"`
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	OK      bool    `json:"ok"`
	Reason  string  `json:"reason,omitempty"`
	Open    bool    `json:"open,omitempty"`
	Late    bool    `json:"late,omitempty"`
}

// TraceOut is the wire form of one traced request.
type TraceOut struct {
	ID        int       `json:"id"`
	Client    string    `json:"client"`
	Write     bool      `json:"write"`
	Key       int       `json:"key"`
	OK        bool      `json:"ok"`
	Reason    string    `json:"reason,omitempty"`
	LatencyMs float64   `json:"latencyMs"`
	Attempts  int       `json:"attempts"`
	Spans     []SpanOut `json:"spans"`
}

// TraceSet is the traced requests kept for a run, split by outcome.
type TraceSet struct {
	Failed []TraceOut `json:"failed"`
	OK     []TraceOut `json:"ok"`
}

// Traces returns the most recent failed and successful traces, newest first.
func (s *Sim) Traces() TraceSet {
	out := TraceSet{Failed: []TraceOut{}, OK: []TraceOut{}}
	conv := func(list []*tracer) []TraceOut {
		res := []TraceOut{}
		for i := len(list) - 1; i >= 0 && len(res) < traceReturn; i-- {
			res = append(res, s.traceOut(list[i]))
		}
		return res
	}
	out.Failed, out.OK = conv(s.failedTr), conv(s.okTr)
	return out
}

func (s *Sim) traceOut(t *tracer) TraceOut {
	o := TraceOut{ID: t.id, Client: t.client, Write: t.write, Key: t.key, OK: t.ok, LatencyMs: t.latency, Attempts: t.attempts, Spans: make([]SpanOut, 0, len(t.spans))}
	if !t.ok {
		o.Reason = t.reason.String()
	}
	for _, sp := range t.spans {
		so := SpanOut{
			Node: sp.node.id, Label: sp.node.label, Kind: sp.node.kind, Depth: sp.depth, Attempt: sp.attempt,
			At: sp.at - t.created, Start: sp.start - t.created, End: sp.end - t.created, OK: sp.ok, Late: sp.late,
		}
		if !sp.done {
			so.Open, so.End, so.OK = true, s.now-t.created, false
		} else if !sp.ok {
			so.Reason = sp.reason.String()
		}
		o.Spans = append(o.Spans, so)
	}
	return o
}
