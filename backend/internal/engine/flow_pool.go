package engine

func (s *Sim) breakerState(e *edge) int {
	if e.brState == 1 && s.now >= e.brUntil {
		e.brState, e.trial = 2, false
	}
	return e.brState
}

func (s *Sim) breakerResult(e *edge, ok bool) {
	switch s.breakerState(e) {
	case 2:
		if !e.trial {
			return
		}
		e.trial = false
		if ok {
			e.brState, e.brFails = 0, 0
		} else {
			s.openBreaker(e)
		}
	case 0:
		if ok {
			e.brFails = 0
		} else if e.brFails++; e.brFails >= breakerFail {
			s.openBreaker(e)
		}
	}
}

func (s *Sim) openBreaker(e *edge) {
	e.brState, e.brUntil, e.brFails, e.trial = 1, s.now+breakerOpen, 0, false
}

type ticket struct {
	pool     *pool
	opening  bool
	released bool
}

type waiter struct {
	send func(*ticket)
	fail func()
	dead func() bool
}

type pool struct {
	owner, db *node
	inUse     int
	opened    int
	conf      int
	wait      []*waiter
	wh        int
	dead      bool
}

func (s *Sim) poolSize(owner *node) int {
	switch owner.kind {
	case KWorker:
		return int(max(1, owner.f("concurrency")))
	case KCache:
		return 64 // a read-through cache keeps its own generous pool
	}
	return int(max(1, owner.f("dbPool")))
}

func (s *Sim) poolOf(owner, db *node) *pool {
	p := owner.pools[db]
	if p == nil {
		p = &pool{owner: owner, db: db}
		owner.pools[db] = p
	}
	return p
}

func (s *Sim) acquire(p *pool) *ticket {
	n := s.poolSize(p.owner)
	if p.inUse < min(p.opened, n) {
		p.inUse++
		return &ticket{pool: p}
	}
	if p.opened < n {
		p.opened++
		p.inUse++
		return &ticket{pool: p, opening: true}
	}
	return nil
}

func (s *Sim) releaseTicket(t *ticket) {
	if t.released {
		return
	}
	t.released = true
	p := t.pool
	p.inUse--
	if t.opening {
		t.opening = false
		p.opened--
	}
	if !p.dead {
		s.trimPool(p)
		s.pumpPool(p)
	}
}

func (s *Sim) pumpPool(p *pool) {
	for p.wh < len(p.wait) {
		w := p.wait[p.wh]
		if w.dead() {
			p.wh++
			continue
		}
		tk := s.acquire(p)
		if tk == nil {
			break
		}
		p.wh++
		w.send(tk)
	}
	if p.wh > 256 && p.wh*2 > len(p.wait) {
		p.wait = append([]*waiter(nil), p.wait[p.wh:]...)
		p.wh = 0
	}
}

func (s *Sim) pumpAll(n *node) {
	for _, p := range n.pools {
		if !p.dead {
			s.trimPool(p)
			s.pumpPool(p)
		}
	}
}

func (s *Sim) poolWaiting(n *node) int {
	c := 0
	for _, p := range n.pools {
		for i := p.wh; i < len(p.wait); i++ {
			if !p.wait[i].dead() {
				c++
			}
		}
	}
	return c
}

// connect is called when a call that just opened a pooled connection reaches the database.
func (s *Sim) connect(db *node, tk *ticket) bool {
	if tk == nil || !tk.opening || tk.released {
		return true
	}
	tk.opening = false
	p := tk.pool
	if db.down || p.dead || p.db != db {
		p.opened--
		return true
	}
	if float64(db.openConns) >= db.f("maxConnections") {
		p.opened--
		return false
	}
	db.openConns++
	p.conf++
	return true
}

func (s *Sim) trimPool(p *pool) {
	t := min(p.opened-s.poolSize(p.owner), p.opened-p.inUse, p.conf)
	if t <= 0 {
		return
	}
	p.opened -= t
	p.conf -= t
	p.db.openConns = max(0, p.db.openConns-t)
}

func (s *Sim) closePools(owner *node) {
	for _, p := range owner.pools {
		p.dead = true
		p.db.openConns = max(0, p.db.openConns-p.conf)
		s.failWaiters(p)
	}
	owner.pools = map[*node]*pool{}
}

func (s *Sim) prunePools(owner *node) {
	if len(owner.pools) == 0 {
		return
	}
	keep := map[*node]bool{}
	for _, e := range owner.out {
		switch e.to.kind {
		case KDatabase, KReplica:
			keep[e.to] = true
		case KShardRouter:
			for _, x := range e.to.out {
				if x.to.kind == KDatabase {
					keep[x.to] = true
				}
			}
		}
	}
	for db, p := range owner.pools {
		if !keep[db] {
			p.dead = true
			db.openConns = max(0, db.openConns-p.conf)
			delete(owner.pools, db)
			s.failWaiters(p)
		}
	}
}

func (s *Sim) failWaiters(p *pool) {
	w := p.wait[p.wh:]
	p.wait, p.wh = nil, 0
	for _, x := range w {
		if !x.dead() {
			x.fail()
		}
	}
}

// dropConns forgets all connections to a database that went away or restarted.
func (s *Sim) dropConns(db *node) {
	if db.kind != KDatabase && db.kind != KReplica {
		return
	}
	for _, o := range s.order {
		if p := o.pools[db]; p != nil {
			p.opened -= p.conf
			p.conf = 0
		}
	}
	db.openConns = 0
}
