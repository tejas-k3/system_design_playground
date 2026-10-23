package engine

// A write that takes several steps in several services (reserve stock, charge the card, book shipping). If a step
// fails after earlier ones succeeded, the earlier work either stays (naive: the order is left half done) or is undone
// by a compensating call to each of those services, newest first (a saga). A compensation that itself fails leaves
// the order stuck for someone to fix.

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
	return (m == "naive" || m == "saga") && len(s.workflowSteps(n)) >= 2
}
