package engine

// Login sessions. A request from a user with an identity (the Users component's "Distinct users") needs that user's
// session. It lives on the app server that signed the user in (local), in a connected Redis (shared) or nowhere (none).
// A user the system has never seen is signed in, which takes a moment. A user who has signed in before and whose session
// cannot be found is refused and signed in again: they were logged out. Sessions can also expire when idle (soft state).

// sessionGate looks up the request's session, then lets it continue or refuses it.
func (s *Sim) sessionGate(n *node, c *call, then func(Result)) {
	mode := n.s("sessionState")
	if (mode != "local" && mode != "shared") || c.user == 0 {
		then(Result{OK: true, Ver: -1, Done: true, Src: n})
		return
	}
	if mode == "local" {
		s.sessionLookup(n, &n.sessStore, c, then)
		return
	}
	re := s.first(n, KRedis)
	if re == nil {
		then(Result{OK: true, Ver: -1, Done: true, Src: n})
		return
	}
	s.callCmd(n, re, c, cmdSession, 0, func(res Result) {
		if !res.OK {
			then(res)
			return
		}
		s.sessionLookup(n, &re.to.sessStore, c, then)
	})
}

func (s *Sim) sessionLookup(n *node, store *map[int]float64, c *call, then func(Result)) {
	ttl := n.f("sessionTtlSec") * 1000
	if *store == nil {
		*store = map[int]float64{}
	}
	m := *store
	exp, held := m[c.user]
	if held && (exp == 0 || exp > s.now) {
		if ttl > 0 {
			m[c.user] = s.now + ttl
		}
		then(Result{OK: true, Ver: -1, Done: true, Src: n})
		return
	}
	if held { // idle for too long: it expired
		s.runSessLost++
	}
	if ttl > 0 {
		m[c.user] = s.now + ttl
	} else {
		m[c.user] = 0
	}
	if s.sessSeen == nil {
		s.sessSeen = map[int]struct{}{}
	}
	if _, known := s.sessSeen[c.user]; known {
		if c.counted {
			s.runReauth++
		}
		then(fail(RSignedOut))
		return
	}
	s.sessSeen[c.user] = struct{}{}
	s.after(s.service(n, n.f("loginMs"))*n.slow, func() { then(Result{OK: true, Ver: -1, Done: true, Src: n}) })
}

// dropLoginSessions forgets the sessions a crashed app server or Redis held in memory.
func (s *Sim) dropLoginSessions(n *node) {
	s.runSessLost += len(n.sessStore)
	n.sessStore = nil
}

// stickyBackend sends a user to the backend they were sent to before, as long as it is still in rotation.
func (s *Sim) stickyBackend(n *node, user int) *edge {
	if e := n.stickyHome[user]; e != nil && !e.removed && !e.to.removed && !s.believedDown(n, e.to) && e.ejectTill <= s.now {
		return e
	}
	e := s.pickBackend(n)
	if e != nil {
		if n.stickyHome == nil {
			n.stickyHome = map[int]*edge{}
		}
		n.stickyHome[user] = e
	}
	return e
}
