package engine

// Login sessions. A request from a user with an identity (the Users component's "Distinct users") needs that user's
// session. It lives on the app server that signed the user in (local), in a connected Redis (shared) or nowhere (none).
// A user the system has never seen is signed in, which takes a moment. A user who has signed in before and whose session
// cannot be found is refused and signed in again: they were logged out. Sessions can also expire when idle (soft state).

// sessionGate looks up the request's session, then lets it continue or refuses it.
func (s *Sim) sessionGate(n *node, c *call, then func(Result)) {
	mode := n.s("sessionState")
	if !n.keepsLoginSessions() || c.user == 0 {
		then(Result{OK: true, Ver: -1, Done: true, Src: n})
		return
	}
	if mode == "local" {
		s.sessionLookup(n, &n.loginStore, c, then)
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
		s.sessionLookup(n, &re.to.loginStore, c, then)
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
		s.runLoginsLost++
	}
	if ttl > 0 {
		m[c.user] = s.now + ttl
	} else {
		m[c.user] = 0
	}
	if s.loginSeen == nil {
		s.loginSeen = map[int]struct{}{}
	}
	if _, known := s.loginSeen[c.user]; known {
		if c.counted {
			s.runLoginRefused++
		}
		then(fail(RSignedOut))
		return
	}
	s.loginSeen[c.user] = struct{}{}
	ep := n.epoch
	s.after(s.service(n, n.f("loginMs"))*n.slow, func() {
		if n.epoch != ep || n.removed {
			then(fail(RNodeDown)) // crashed while signing the user in: do not start downstream work
			return
		}
		then(Result{OK: true, Ver: -1, Done: true, Src: n})
	})
}

// dropLoginSessions forgets the sessions a crashed app server or Redis held in memory.
func (s *Sim) dropLoginSessions(n *node) {
	for _, exp := range n.loginStore {
		if exp == 0 || exp > s.now { // one that had already lapsed was counted when it did
			s.runLoginsLost++
		}
	}
	n.loginStore = nil
}

// keepsLoginSessions is whether this app server looks up a login session for requests from users with an identity.
func (n *node) keepsLoginSessions() bool {
	m := n.s("sessionState")
	return m == "local" || m == "shared"
}
