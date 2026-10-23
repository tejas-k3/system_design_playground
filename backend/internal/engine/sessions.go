package engine

import "sort"

// Long-lived connections. A client with arrival "sessions" has a fixed number of users who each keep a connection
// open to an app server for a while (opening one costs a handshake and takes a slot), send messages over it, and
// reconnect when it ends or when the server it was on goes away.

type session struct {
	id     int
	client *node
	gen    int
	node   *node // the app server holding the connection, nil when not connected
	ep     int
	life   int // bumped when a connection ends, so timers of an earlier connection do nothing
}

// connCapacity is how many connections one app server can hold: the lower of its connection limit and what its
// memory allows after the base footprint and the working memory of its threads. Connections take a slot whether or not a thread is free.
func (s *Sim) connCapacity(n *node) int {
	inst := s.instancesOf(n)
	cp := 1 << 30
	if m := int(n.f("maxConns")); m > 0 {
		cp = m * inst
	}
	if kb, lim := n.f("memPerConnKB"), n.f("memLimitMB"); kb > 0 && lim > 0 {
		// what is left once the base footprint and working memory for every busy thread are set aside
		room := (lim - n.f("memBaseMB") - n.f("memPerReqMB")*n.f("threads")) * 1024 / kb * float64(inst)
		if room < 0 {
			room = 0
		}
		cp = min(cp, int(room))
	}
	return cp
}

func (s *Sim) startSessions(c *node, gen int) {
	for i := 0; i < int(c.f("connections")); i++ {
		sess := &session{id: i, client: c, gen: gen}
		s.after(1+s.rng.Next()*5000, func() { s.connectSession(sess) })
	}
}

func (s *Sim) sessionAlive(sess *session) bool {
	c := sess.client
	return sess.gen == c.arrivalGen && !c.removed && !c.down && !s.trafficStopped()
}

func (s *Sim) connectSession(sess *session) {
	if !s.sessionAlive(sess) {
		return
	}
	c := sess.client
	lg := s.createLogical(c)
	lg.connect, lg.sess = true, sess
	lg.after = func() {
		if sess.node != nil && sess.node.epoch == sess.ep && sess.node.sessions[sess] {
			s.sessionUp(sess)
			return
		}
		sess.node = nil
		s.after(1000+s.rng.Next()*c.f("reconnectJitterSec")*1000, func() { s.connectSession(sess) })
	}
}

func (s *Sim) sessionUp(sess *session) {
	sess.life++
	life, c, node := sess.life, sess.client, sess.node
	var msg func()
	msg = func() {
		if life != sess.life || !s.sessionAlive(sess) {
			return
		}
		s.createLogical(c)
		s.after(s.rng.Exp()*60000/c.f("msgsPerMin"), msg)
	}
	if c.f("msgsPerMin") > 0 {
		s.after(s.rng.Exp()*60000/c.f("msgsPerMin"), msg)
	}
	s.after(s.rng.Exp()*c.f("sessionSec")*1000, func() {
		if life != sess.life {
			return
		}
		sess.life++
		if node.sessions[sess] {
			delete(node.sessions, sess)
			node.sessConns--
		}
		sess.node = nil
		s.connectSession(sess)
	})
}

// dropSessions is a server process dying: every connection it held is gone and its clients try again, spread over
// reconnectJitterSec (0 = all at once, the reconnect storm).
func (s *Sim) dropSessions(n *node) {
	if len(n.sessions) == 0 {
		n.sessConns = 0
		return
	}
	var list []*session
	for sess := range n.sessions {
		list = append(list, sess)
	}
	sort.Slice(list, func(i, j int) bool { // a fixed order, so the random draws below are reproducible
		a, b := list[i], list[j]
		if a.client.id != b.client.id {
			return a.client.id < b.client.id
		}
		return a.id < b.id
	})
	n.sessions, n.sessConns = nil, 0
	for _, sess := range list {
		sess.life++
		sess.node = nil
		s.after(1+s.rng.Next()*sess.client.f("reconnectJitterSec")*1000, func() { s.connectSession(sess) })
	}
}

// pushThreads is how many threads' worth of CPU the server spends pushing to its connected users: one push a second
// to each of 2,000 users at 2 ms apiece is 4 threads, taken from ordinary requests.
func (s *Sim) pushThreads(n *node) float64 {
	rate := n.f("pushPerSec")
	if rate <= 0 || n.sessConns <= 0 {
		return 0
	}
	return rate * float64(n.sessConns) * n.f("pushCpuMs") / 1000
}
