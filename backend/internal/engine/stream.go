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
