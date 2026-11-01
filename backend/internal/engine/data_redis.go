package engine

func (s *Sim) procRedis(n *node, c *call, cb func(Result), release func()) {
	var ms float64
	switch c.cmd {
	case cmdGeo:
		ms = n.f("geoSearchMs")
	case cmdScore:
		ms = n.f("scoreMs")
	case cmdRank, cmdSession:
		ms = 0.1
	default:
		ms = 0.05
	}
	s.hold(n, ms*n.slow, release, cb, func() {
		release()
		switch c.cmd {
		case cmdLock:
			if l, ok := n.locks[c.key]; ok && l.exp > s.now && l.token != c.token {
				n.c.lockBusy++
				n.c.ok++
				r := fail(RLockBusy)
				cb(r)
				return
			}
			n.locks[c.key] = redisLock{token: c.token, exp: s.now + c.ttl}
		case cmdUnlock:
			if l, ok := n.locks[c.key]; ok && l.token == c.token {
				delete(n.locks, c.key)
			}
		case cmdScore:
			s.noteRedisWrite(n, c.counted)
		}
		s.okAt(n, Result{Ver: -1, Done: true, Src: n}, cb)
	})
}

// noteRedisWrite remembers a counted write so a later crash can tell how much was lost.
func (s *Sim) noteRedisWrite(n *node, counted bool) {
	if !counted {
		return
	}
	n.rwTotal++
	if n.s("persistence") == "aof" {
		n.rwRecent = append(n.rwRecent, s.now)
		s.pruneRecentWrites(n)
	}
}

func (s *Sim) pruneRecentWrites(n *node) {
	i := 0
	for i < len(n.rwRecent) && n.rwRecent[i] < s.now-1000 {
		i++
	}
	n.rwRecent = n.rwRecent[i:]
}

// loseRedisData is called when a Redis node crashes: without persistence every write since it
// started is gone; with an append-only file only the last second (not yet fsynced) is.
func (s *Sim) loseRedisData(n *node) {
	lost := n.rwTotal
	if n.s("persistence") == "aof" {
		s.pruneRecentWrites(n)
		lost = len(n.rwRecent)
	}
	s.runLost += lost
	n.rwTotal, n.rwRecent = 0, nil
	n.locks = map[int]redisLock{}
}
