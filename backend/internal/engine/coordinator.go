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
		n.coordElections++
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

// coordLeaderCut cuts the leader off from the other members for dur. The others elect a new leader after the usual
// timeout. The old leader goes on answering for stepDownSec before it notices it has lost its majority.
func (s *Sim) coordLeaderCut(n *node, dur float64) {
	s.coordInit(n)
	if n.coordLeader < 0 {
		return
	}
	if stale := n.f("stepDownSec") * 1000; stale > 0 {
		if dur > 0 {
			stale = min(stale, dur)
		}
		n.coordStaleUntil = s.now + stale
	}
	s.coordLeaderDown(n, dur)
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
	// the old leader has not noticed it was replaced; the others can only elect a new one with a majority and no vote
	if s.now < n.coordStaleUntil && c.cmd == cmdWrite && s.coordMajority(n) && n.coordFaults == nil && n.s("faultModel") != "arbitrary" {
		s.after(s.service(n, n.f("opMs")+n.f("rttMs"))*n.slow, func() {
			if c.counted {
				n.coordStaleAcks++
				s.runLost++ // acknowledged, but only the old leader has it
			}
			s.okAt(n, Result{Ver: -1, Done: true, Src: n}, cb)
		})
		return
	}
	if n.coordLeader < 0 {
		s.failAt(n, RNoQuorum, cb)
		return
	}
	if c.cmd == cmdLock || c.cmd == cmdUnlock {
		s.coordLock(n, c, cb)
		return
	}
	ms := n.f("opMs") + n.f("rttMs") // the leader answers once a majority has acknowledged
	voting := n.coordFaults != nil || n.s("faultModel") == "arbitrary"
	if n.s("faultModel") == "arbitrary" {
		ms += n.f("rttMs") // the extra round in which members compare what they heard
	}
	ep := n.coordTerm
	s.after(s.service(n, ms)*n.slow, func() {
		if n.coordTerm != ep && n.coordLeader < 0 { // the leader was lost while the write was in flight
			s.failAt(n, RNoQuorum, cb)
			return
		}
		if voting {
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
// share of requests each affects. A member whose time is in the past is healthy again.
type coordFaults struct {
	muteUntil, lieUntil []float64
	mutePct, liePct     []float64
	corrupt             []bool // the member's wrong answers are damaged data, which a checksum catches; otherwise lies
}

func (f *coordFaults) fit(members int) {
	for len(f.muteUntil) < members {
		f.muteUntil, f.lieUntil = append(f.muteUntil, 0), append(f.lieUntil, 0)
		f.mutePct, f.liePct, f.corrupt = append(f.mutePct, 0), append(f.liePct, 0), append(f.corrupt, false)
	}
}

func (s *Sim) coordMembersFault(n *node, ev ChaosEvent) {
	s.coordInit(n)
	m := len(n.coordDown)
	if n.coordFaults == nil {
		n.coordFaults = &coordFaults{}
	}
	f := n.coordFaults
	f.fit(m)
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
			f.muteUntil[i], f.mutePct[i] = until, pct
		} else {
			f.lieUntil[i], f.liePct[i], f.corrupt[i] = until, pct, ev.Mode == "corrupt"
		}
		count--
	}
}

// memberHit is whether a fault that lasts until `until` and touches pct of requests hits this attempt at member j. The
// verdict is a hash of the request, its attempt and the member, so a retry can get a different one and no random number
// is drawn.
func (s *Sim) memberHit(c *call, j int, until, pct float64, salt uint32) bool {
	if until <= s.now {
		return false
	}
	attempt := uint32(0)
	if c.at != nil {
		attempt = uint32(c.at.n)
	}
	return float64(Hash32(uint32(c.rid)*2654435761+salt+uint32(j)*97+attempt*7919)%100) < pct
}

// coordVote decides one request that goes to the group of members. It reports whether the group answered, and whether
// the answer it accepted was wrong. Under the crash model with no fault injected it is not called.
func (s *Sim) coordVote(n *node, c *call) (answered, wrong bool) {
	f := n.coordFaults
	if f == nil {
		f = &coordFaults{}
	}
	members := len(n.coordDown)
	f.fit(members)
	honest, liars := 0, 0
	for j := range members {
		if !s.coordUp(n, j) || s.memberHit(c, j, f.muteUntil[j], f.mutePct[j], 11) {
			continue // down, or it drops this request
		}
		if s.memberHit(c, j, f.lieUntil[j], f.liePct[j], 29) {
			if !(f.corrupt[j] && n.b("answerChecksums")) { // damage is seen and the answer dropped
				liars++
			}
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
	if alive < members-k {
		return false, false // more members are silent than the group was built for
	}
	got := max(1, members-k) // the group cannot wait for the k that may never answer
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
