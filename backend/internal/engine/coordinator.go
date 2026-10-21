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
	ep := n.coordTerm
	s.after(s.service(n, ms)*n.slow, func() {
		if n.coordTerm != ep && n.coordLeader < 0 { // the leader was lost while the write was in flight
			s.failAt(n, RNoQuorum, cb)
			return
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
