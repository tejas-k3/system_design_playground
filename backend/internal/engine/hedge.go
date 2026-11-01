package engine

// Reads over several identical replicas. By default every read goes to the first replica. With readSpread
// roundRobin they take turns; with hedgeReadsMs set, a read that has not been answered after that long is asked
// of the next replica too, and the first good answer wins. The slow tail of a replica's service time is then
// avoided by the other replica, at the cost of a little extra load.

func (s *Sim) replicaRead(n *node, c *call, reps []*edge, cb func(Result)) {
	first := s.pickCyclic(n, "replica", reps, nil)
	hedgeMs := n.f("hedgeReadsMs")
	pending, fin := 1, false
	resolve := func(r Result) {
		pending--
		if fin {
			return
		}
		if r.OK || pending == 0 {
			fin = true
			cb(r)
		}
	}
	s.callCmd(n, first, c, cmdRead, 0, resolve)
	if hedgeMs <= 0 {
		return
	}
	s.after(hedgeMs, func() {
		if fin {
			return
		}
		next := s.pickCyclic(n, "replica", reps, func(e *edge) bool { return e != first })
		if next == nil {
			return
		}
		pending++
		s.runHedged++
		s.callCmd(n, next, c, cmdRead, 0, resolve)
	})
}
