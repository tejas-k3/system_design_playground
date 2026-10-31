package engine

// The coordination service: a small cluster (three or five nodes) that agrees on one leader, like Raft or ZooKeeper.
// Writes and reads go through the leader. When the leader is lost, nothing is answered until the others elect a new
// one: a node that stops hearing the leader waits a random time between one and two election timeouts and then
// stands, so the gap is about that long plus a round trip. Without a majority of nodes no leader can be elected, and
// a leader that loses its majority steps down.

func (s *Sim) coordInit(n *node) {
	if len(n.coordDown) == int(n.f("nodes")) {
		return
	}
	cnt := clampInt(int(n.f("nodes")), 1, 9)
	down := make([]float64, cnt)
	copy(down, n.coordDown)
	n.coordDown = down
	if n.coordTerm == 0 {
		n.coordLeader, n.coordTerm = 0, 1
	}
}

func (s *Sim) coordUp(n *node, i int) bool { return n.coordDown[i] <= s.now }

func (s *Sim) coordUpCount(n *node) int {
	c := 0
	for i := range n.coordDown {
		if s.coordUp(n, i) {
			c++
		}
	}
	return c
}

func (s *Sim) coordMajority(n *node) bool { return s.coordUpCount(n)*2 > len(n.coordDown) }

// coordChanged is called whenever a node goes down or comes back.
func (s *Sim) coordChanged(n *node) {
	if n.coordLeader >= 0 && (!s.coordUp(n, n.coordLeader) || !s.coordMajority(n)) {
		n.coordLeader = -1
		s.logEvent("noLeader", n, 0)
	}
	if n.coordLeader < 0 && !n.coordElecting {
		s.coordScheduleElection(n)
	}
}

func (s *Sim) coordScheduleElection(n *node) {
	n.coordElecting = true
	// randomised so that two candidates rarely stand together
	wait := n.f("electionTimeoutMs")*(1+s.chaosRng.Next()) + n.f("rttMs")
	s.after(wait, func() {
		n.coordElecting = false
		if n.coordLeader >= 0 {
			return
		}
		if !s.coordMajority(n) { // cannot win without a majority: stand again later
			s.coordScheduleElection(n)
			return
		}
		for i := range n.coordDown {
			if s.coordUp(n, i) {
				n.coordLeader = i
				break
			}
		}
		n.coordTerm++
		s.logEvent("elected", n, n.coordTerm)
	})
}

// coordNodesDown takes nodes of the cluster out for dur ms. mode "follower" spares the current leader.
func (s *Sim) coordNodesDown(n *node, count int, dur float64, mode string) {
	s.coordInit(n)
	until := 1e18
	if dur > 0 {
		until = s.now + dur
	}
	for i := 0; i < len(n.coordDown) && count > 0; i++ {
		if mode == "follower" && i == n.coordLeader {
			continue
		}
		n.coordDown[i] = until
		count--
	}
	s.coordChanged(n)
	if dur > 0 {
		s.at(until, func() { s.coordChanged(n) })
	}
}

func (s *Sim) coordLeaderDown(n *node, dur float64) {
	s.coordInit(n)
	if n.coordLeader < 0 {
		return
	}
	until := 1e18
	if dur > 0 {
		until = s.now + dur
	}
	n.coordDown[n.coordLeader] = until
	s.coordChanged(n)
	if dur > 0 {
		s.at(until, func() { s.coordChanged(n) })
	}
}

func (s *Sim) procCoord(n *node, c *call, cb func(Result), release func()) {
	release()
	if c.cmd != cmdRead && c.cmd != cmdWrite && c.cmd != cmdLock && c.cmd != cmdUnlock {
		s.failAt(n, RNoRoute, cb)
		return
	}
	s.coordInit(n)
	if n.coordLeader < 0 {
		s.failAt(n, RNoQuorum, cb)
		return
	}
	if c.cmd == cmdLock || c.cmd == cmdUnlock {
		s.coordLock(n, c, cb)
		return
	}
	ms := n.f("opMs") + n.f("rttMs") // the leader answers once a majority has acknowledged
	if n.coordFaults != nil && n.s("faultModel") == "arbitrary" {
		ms += n.f("rttMs") // the extra round in which members compare what they heard
	}
	ep := n.coordTerm
	s.after(s.service(n, ms)*n.slow, func() {
		if n.coordTerm != ep && n.coordLeader < 0 { // the leader was lost while the write was in flight
			s.failAt(n, RNoQuorum, cb)
			return
		}
		if n.coordFaults != nil {
			answered, wrong := s.coordVote(n, c)
			if !answered {
				s.failAt(n, RNoQuorum, cb)
				return
			}
			if wrong && c.counted {
				s.runWrongAnswers++
			}
		}
		s.okAt(n, Result{Ver: -1, Done: true, Src: n}, cb)
	})
}

// coordLock is the lock and lease service: a lock is held for its lease (ttl), and every grant comes with a fencing
// token that is larger than any given before, so a resource can tell a stale holder from the current one.
func (s *Sim) coordLock(n *node, c *call, cb func(Result)) {
	s.after(s.service(n, n.f("opMs")+n.f("rttMs"))*n.slow, func() {
		if n.coordLeader < 0 {
			s.failAt(n, RNoQuorum, cb)
			return
		}
		if n.coordLocks == nil {
			n.coordLocks = map[int]redisLock{}
		}
		l, held := n.coordLocks[c.key]
		if c.cmd == cmdUnlock {
			if held && l.token == c.token {
				delete(n.coordLocks, c.key)
			}
			s.okAt(n, Result{Ver: -1, Done: true}, cb)
			return
		}
		if held && l.exp > s.now && l.token != c.token {
			n.c.lockBusy++
			n.c.ok++
			cb(fail(RLockBusy))
			return
		}
		n.coordFence++
		n.coordLocks[c.key] = redisLock{token: c.token, exp: s.now + c.ttl, fence: n.coordFence}
		s.okAt(n, Result{Ver: -1, Done: true, Fence: n.coordFence}, cb)
	})
}

// coordFaults are the members of a coordinator group that drop messages (omission) or give wrong answers, with the
// share of requests affected. A member whose time is in the past is healthy again.
type coordFaults struct {
	muteUntil, lieUntil []float64
	mutePct, liePct     float64
	corrupt             bool // the wrong answers are damaged data, which a checksum catches; otherwise they are lies
}

func (s *Sim) coordMembersFault(n *node, ev ChaosEvent) {
	s.coordInit(n)
	m := len(n.coordDown)
	if n.coordFaults == nil {
		n.coordFaults = &coordFaults{muteUntil: make([]float64, m), lieUntil: make([]float64, m)}
	}
	f := n.coordFaults
	until := 1e18
	if ev.DurationSec > 0 {
		until = s.now + ev.DurationSec*1000
	}
	pct := ev.FractionPct
	if pct <= 0 {
		pct = 100
	}
	count := int(ev.Factor)
	for i := m - 1; i >= 0 && count > 0; i-- { // the last members, so the leader (the first) is spared
		if ev.Type == "memberOmission" {
			f.muteUntil[i], f.mutePct = until, pct
		} else {
			f.lieUntil[i], f.liePct, f.corrupt = until, pct, ev.Mode == "corrupt"
		}
		count--
	}
}

// coordVote decides one request that goes to the group of members. It reports whether the group answered, and whether
// the answer it accepted was wrong. With no fault injected it is not called, so existing designs are unchanged.
func (s *Sim) coordVote(n *node, c *call) (answered, wrong bool) {
	f := n.coordFaults
	members := len(n.coordDown)
	honest, liars := 0, 0
	for j := 0; j < members; j++ {
		if !s.coordUp(n, j) {
			continue
		}
		hit := func(until, pct float64, salt uint32) bool {
			return until > s.now && float64(Hash32(uint32(c.rid)*2654435761+salt+uint32(j)*97)%100) < pct
		}
		if hit(f.muteUntil[j], f.mutePct, 11) {
			continue // it drops this request
		}
		if hit(f.lieUntil[j], f.liePct, 29) {
			if f.corrupt && n.b("answerChecksums") {
				continue // the damage is seen and the answer dropped
			}
			liars++
			continue
		}
		honest++
	}
	alive := honest + liars
	if n.s("faultModel") != "arbitrary" {
		if alive*2 <= members {
			return false, false // no majority answered
		}
		return true, liars > 0 // the first answer is believed, and a liar answers first
	}
	k := clampInt(int(n.f("faultsTolerated")), 1, 4)
	got := min(alive, max(1, members-k)) // the group cannot wait for the k that may never answer
	liarsIn := min(liars, got)
	honestIn := got - liarsIn // the liars answer first, as the worst case
	switch {
	case honestIn >= k+1:
		return true, false
	case liarsIn >= k+1:
		return true, true
	}
	return false, false
}
