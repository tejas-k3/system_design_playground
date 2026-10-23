package engine

// A write that takes several steps in several services (reserve stock, charge the card, book shipping). If a step
// fails after earlier ones succeeded, the earlier work either stays (naive: the order is left half done) or is undone
// by a compensating call to each of those services, newest first (a saga). A compensation that itself fails leaves
// the order stuck for someone to fix.

// inDoubtRec is a two-phase commit whose coordinator died between the votes and the decision.
type inDoubtRec struct {
	n     *node
	since float64
}

func (s *Sim) workflowSteps(n *node) []*edge {
	var steps []*edge
	for _, e := range n.out {
		if !e.removed && (e.to.kind == KDatabase || e.to.kind == KExternalAPI) {
			steps = append(steps, e)
		}
	}
	return steps
}

func stepCmd(e *edge) cmd {
	if e.to.kind == KExternalAPI {
		return cmdExternal
	}
	return cmdWrite
}

// workflowWrite runs the steps in order. It reports false when the server has fewer than two steps to run.
func (s *Sim) workflowWrite(n *node, c *call, mode string, done func(Result)) bool {
	steps := s.workflowSteps(n)
	if len(steps) < 2 {
		return false
	}
	if mode == "twoPhase" {
		s.twoPhase(n, c, steps, done)
		return true
	}
	var run func(i int)
	run = func(i int) {
		if i == len(steps) {
			done(Result{OK: true, Ver: -1, Done: true, Src: n})
			return
		}
		s.callCmd(n, steps[i], c, stepCmd(steps[i]), 0, func(res Result) {
			if res.OK {
				run(i + 1)
				return
			}
			if i == 0 { // nothing has been done yet: nothing to undo
				done(res)
				return
			}
			if mode != "saga" {
				if c.counted {
					s.runInconsistent++ // the earlier steps stay done
				}
				done(res)
				return
			}
			s.compensate(n, c, steps, i-1, false, func() { done(res) })
		})
	}
	run(0)
	return true
}

// twoPhase asks every service to prepare (vote and hold its row), then tells every one to commit. A failed vote
// aborts the ones that already voted yes. If the coordinator dies after a yes vote and before the decision is
// delivered, the services that voted cannot decide on their own: they stay locked until the coordinator restarts.
func (s *Sim) twoPhase(n *node, c *call, steps []*edge, done func(Result)) {
	ep := n.epoch
	lost := func() bool {
		if n.epoch == ep {
			return false
		}
		if c.counted {
			s.runInDoubt++
		}
		s.inDoubt = append(s.inDoubt, inDoubtRec{n, s.now})
		done(fail(RNodeDown))
		return true
	}
	var phase func(i int, commit bool)
	phase = func(i int, commit bool) {
		if i == len(steps) {
			if !commit {
				phase(0, true)
				return
			}
			done(Result{OK: true, Ver: -1, Done: true, Src: n})
			return
		}
		s.callCmd(n, steps[i], c, stepCmd(steps[i]), 0, func(res Result) {
			if lost() {
				return
			}
			if res.OK {
				phase(i+1, commit)
				return
			}
			if i == 0 || commit {
				done(res)
				return
			}
			s.compensate(n, c, steps, i-1, false, func() { done(res) })
		})
	}
	phase(0, false)
}

// releaseInDoubt ends the wait of every transaction the restarted coordinator left undecided.
func (s *Sim) releaseInDoubt(n *node) {
	keep := s.inDoubt[:0]
	for _, r := range s.inDoubt {
		if r.n == n {
			s.runBlockedSec += (s.now - r.since) / 1000
		} else {
			keep = append(keep, r)
		}
	}
	s.inDoubt = keep
}

// compensate undoes step i, then the ones before it. stuck is whether any undo has failed so far.
func (s *Sim) compensate(n *node, c *call, steps []*edge, i int, stuck bool, then func()) {
	if i < 0 {
		if c.counted {
			if stuck {
				s.runStuck++
				s.runInconsistent++
			} else {
				s.runCompensated++
			}
		}
		then()
		return
	}
	s.callCmd(n, steps[i], c, stepCmd(steps[i]), 0, func(res Result) {
		s.compensate(n, c, steps, i-1, stuck || !res.OK, then)
	})
}

// runsWorkflow is whether this server's writes are multi-step workflows, in which case the external services are
// steps of the workflow and not an extra call made before it.
func (s *Sim) runsWorkflow(n *node) bool {
	m := n.s("workflow")
	return (m == "naive" || m == "saga" || m == "twoPhase") && len(s.workflowSteps(n)) >= 2
}
