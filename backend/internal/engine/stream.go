package engine

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
