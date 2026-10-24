package engine

import "math"

// A stream job in the diagram: events reach it from an app server, it keeps windows of event time (see WindowAgg),
// and a crash makes the windows that fired since its last checkpoint fire again unless it delivers exactly once.

func (s *Sim) arriveStream(n *node, c *call, cb func(Result)) {
	if !c.write {
		s.failAt(n, RNoRoute, cb)
		return
	}
	if n.win == nil {
		slide := n.f("slideSec")
		if slide <= 0 {
			slide = n.f("windowSec")
		}
		n.win = NewWindowAgg(n.f("windowSec"), slide, n.f("watermarkDelaySec"), n.f("allowedLatenessSec"))
	}
	at := s.now / 1000
	if pct := n.f("lateEventPct"); pct > 0 && float64(Hash32(uint32(c.rid)*2246822519+11)%100) < pct { // no random draw: the same events are late in every run
		at -= n.f("lateBySec")
	}
	n.win.Add(at)
	s.countVisitor(n, c.key)
	s.trackTop(n, c.key)
	if s.now-n.winCPAt >= n.f("checkpointSec")*1000 {
		n.winCPAt, n.winCP = s.now, len(n.win.Emitted)
	}
	ms := n.f("opMs")
	if n.s("delivery") == "exactlyOnce" {
		ms += 2 // the output is committed together with the checkpoint
	}
	s.after(s.service(n, ms)*n.slow, func() { s.okAt(n, Result{Ver: -1, Done: true}, cb) })
}

// streamCrashed: the job restarts from its last checkpoint. With at-least-once delivery every window it had emitted
// since then is emitted again; with exactly-once the sink never saw them.
func (s *Sim) streamCrashed(n *node) {
	if n.win == nil || n.s("delivery") == "exactlyOnce" {
		return
	}
	for _, e := range n.win.Emitted[n.winCP:] {
		if !e.Update {
			n.winDup++
		}
	}
}

// streamLate is the events every stream job has dropped for arriving after their window closed for good.
func (s *Sim) streamLate() int {
	n := 0
	for _, nd := range s.order {
		if nd.kind == KStream && nd.win != nil && !nd.removed {
			n += nd.win.LateDropped
		}
	}
	return n
}

// streamDelay is the longest wait for a first result among the stream jobs that took in events: a window is emitted
// when the watermark passes its end, which is the watermark delay after the newest event it could hold.
func (s *Sim) streamDelay() float64 {
	d := 0.0
	for _, nd := range s.order {
		if nd.kind == KStream && nd.win != nil && !nd.removed {
			d = max(d, nd.f("watermarkDelaySec"))
		}
	}
	return d
}

// countVisitor adds a visitor id to the stream job's distinct count, exactly or in a HyperLogLog sketch.
func (s *Sim) countVisitor(n *node, id int) {
	mode := n.s("counting")
	if mode != "exact" && mode != "hyperLogLog" {
		return
	}
	if n.uniqExact == nil {
		n.uniqExact = map[int]struct{}{}
	}
	n.uniqExact[id] = struct{}{} // the truth; in hyperLogLog mode it is only kept to measure the error
	if mode == "hyperLogLog" {
		if n.uniqHLL == nil {
			n.uniqHLL = NewHLL(uint(n.f("hllPrecision")))
		}
		n.uniqHLL.Add(uint32(id))
	}
}

// visitorStats is the distinct count, the memory a real job would need for it and the error of the estimate.
func (s *Sim) visitorStats(n *node) (uniques, stateKB, errPct float64, ok bool) {
	switch n.s("counting") {
	case "exact":
		if n.uniqExact == nil {
			return 0, 0, 0, false
		}
		return float64(len(n.uniqExact)), float64(len(n.uniqExact)) * 16 / 1024, 0, true // an id and its slot: about 16 bytes
	case "hyperLogLog":
		if n.uniqHLL == nil {
			return 0, 0, 0, false
		}
		est, truth := n.uniqHLL.Count(), float64(len(n.uniqExact))
		return est, float64(len(n.uniqHLL.reg)) / 1024, math.Abs(est-truth) / truth * 100, true
	}
	return 0, 0, 0, false
}

// streamCounting is the memory and the error of the worst stream job that counts visitors.
func (s *Sim) streamCounting() (stateKB, errPct float64) {
	for _, nd := range s.order {
		if nd.kind == KStream && !nd.removed {
			kb := 0.0
			if _, k, e, ok := s.visitorStats(nd); ok {
				kb, errPct = k, max(errPct, e)
			}
			if _, k, ok := s.topStats(nd); ok {
				kb += k
			}
			stateKB = max(stateKB, kb)
		}
	}
	return
}

func (s *Sim) stateKB() float64  { kb, _ := s.streamCounting(); return kb }
func (s *Sim) countErr() float64 { _, e := s.streamCounting(); return e }

// trackTop feeds an item to the job's top-ten tracker, and to an exact table that serves as the truth.
func (s *Sim) trackTop(n *node, id int) {
	switch n.s("topK") {
	case "exact":
		if n.topExact == nil {
			n.topExact = NewExactTopK()
		}
		n.topExact.Add(uint32(id))
	case "countMin":
		if n.topSketch == nil {
			n.topSketch = NewSketchTopK(10, int(n.f("sketchWidth")), int(n.f("sketchDepth")), 7)
			n.topTruth = NewExactTopK()
		}
		n.topSketch.Add(uint32(id))
		n.topTruth.Add(uint32(id))
	}
}

// topStats is how many of the true ten most popular items the tracker found, and the memory it needs.
func (s *Sim) topStats(n *node) (recall, kb float64, ok bool) {
	switch n.s("topK") {
	case "exact":
		if n.topExact == nil {
			return 0, 0, false
		}
		return 1, float64(n.topExact.Counters()) * 16 / 1024, true
	case "countMin":
		if n.topSketch == nil {
			return 0, 0, false
		}
		in := map[uint32]bool{}
		for _, x := range n.topTruth.Top(10) {
			in[x] = true
		}
		hit := 0
		for _, x := range n.topSketch.Top(10) {
			if in[x] {
				hit++
			}
		}
		return float64(hit) / 10, (float64(n.topSketch.Counters())*4 + float64(n.topSketch.Candidates())*16) / 1024, true
	}
	return 0, 0, false
}

// topRecall is the lowest recall among the stream jobs that track their top ten (1 when none does).
func (s *Sim) topRecall() float64 {
	r, any := 1.0, false
	for _, nd := range s.order {
		if nd.kind == KStream && !nd.removed {
			if v, _, ok := s.topStats(nd); ok {
				r, any = min(r, v), true
			}
		}
	}
	if !any {
		return 0
	}
	return r
}

// hostEdge is the external service a request goes to: the first one, or with hostChoice "byKey" the one its key
// belongs to (as a crawler picks the site a page lives on).
func (s *Sim) hostEdge(n *node, key int) *edge {
	if n.s("hostChoice") != "byKey" {
		return s.first(n, KExternalAPI)
	}
	hosts := s.outOf(n, KExternalAPI)
	if len(hosts) == 0 {
		return nil
	}
	if key < 0 {
		key = -key
	}
	return hosts[key%len(hosts)]
}

// politely runs a call to an external service now, or when the server's per-service limit lets it.
func (s *Sim) politely(n *node, e *edge, call func()) {
	rate := n.f("perHostRps")
	if rate <= 0 {
		call()
		return
	}
	at := max(s.now, e.politeAt)
	e.politeAt = at + 1000/rate
	if at <= s.now {
		call()
		return
	}
	s.after(at-s.now, call)
}

// vectorRecall is the lowest recall among the vector stores of the design (0 when there is none).
func (s *Sim) vectorRecall() float64 {
	r, any := 1.0, false
	for _, nd := range s.order {
		if nd.kind == KVectorStore && !nd.removed {
			r, any = min(r, 1-math.Pow(0.5, nd.f("efSearch")/32)), true
		}
	}
	if !any {
		return 0
	}
	return r
}

// bloomBlocks is whether the app server's Bloom filter of existing ids says this request's id is not there. The filter
// is built on first use from the users' id range; ids the users ask for that are not in that range are the absent ones.
func (s *Sim) bloomBlocks(n *node, c *call) bool {
	ks := int(c.lg.client.f("keySpace"))
	if n.guard == nil {
		bits := max(64, int(n.f("bloomBitsPerKey"))*(ks+1))
		n.guard = NewBloom(bits, int(n.f("bloomHashes")))
		for id := 0; id <= ks; id++ {
			n.guard.Add(uint32(id))
		}
	}
	if c.key >= 0 {
		return false // a real id always passes: the filter never forgets what it holds
	}
	n.guardChecked++
	if n.guard.Has(uint32(-c.key)) {
		n.guardFalse++
		return false
	}
	n.guardBlocked++
	return true
}
