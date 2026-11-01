package engine

// Transactions: the transfer workload (every request moves money between two accounts) under three isolation levels.
//
// A transfer reads both accounts, then writes both. What differs is what happens when another transfer touched the
// same account in between:
//   - read committed: nothing stops it. If an account changed between this transfer's read and its commit, the
//     second writer silently overwrote the first (a lost update, counted) and still succeeds.
//   - repeatable read (snapshot): first committer wins. A transfer whose accounts changed under it aborts.
//   - serializable (locks): the transfer locks its first account, then its second, holds both until it commits, and
//     waits for any it cannot get. Two transfers that take the same two accounts in opposite order deadlock: the one
//     that would close the cycle is the victim and aborts. Waiting holds a database connection, so throughput falls.
//
// Write skew is the oncall workload: rows come in pairs (1,2), (3,4) ... and a request reads both rows of its pair
// but writes only its own. Two such requests that overlap read the same state and write different rows, so no row is
// written twice: read committed lets both through and snapshot isolation (first committer wins on the rows WRITTEN)
// lets both through too. Each counts as a write skew if the other row of the pair changed under it. Serializable
// locks both rows of the pair up front and the two take turns.

type txLock struct {
	holder  int
	waiters []txWaiter
}

type txWaiter struct {
	id      int
	granted func()
}

func (s *Sim) txAcquire(n *node, id, key int, granted, deadlock func()) {
	l := n.txl[key]
	if l == nil {
		l = &txLock{}
		n.txl[key] = l
	}
	if l.holder == 0 || l.holder == id {
		l.holder = id
		granted()
		return
	}
	// would waiting here close a cycle? follow who holds what each holder waits for
	cur := l.holder
	for range 64 {
		if cur == id {
			deadlock()
			return
		}
		wk, waiting := n.txWaits[cur]
		if !waiting {
			break
		}
		cur = n.txl[wk].holder
	}
	n.txWaits[id] = key
	l.waiters = append(l.waiters, txWaiter{id, granted})
}

func (s *Sim) txRelease(n *node, id, key int) {
	l := n.txl[key]
	if l == nil || l.holder != id {
		return
	}
	l.holder = 0
	if len(l.waiters) > 0 {
		w := l.waiters[0]
		l.waiters = l.waiters[1:]
		delete(n.txWaits, w.id)
		l.holder = w.id
		w.granted()
	}
}

// secondAccount picks the other account of a transfer from the request id (no random draw).
func secondAccount(c *call, first int) int {
	ks := 2
	if c.lg != nil {
		if v := int(c.lg.client.f("keySpace")); v > 1 {
			ks = v
		}
	}
	k := int(Hash32(uint32(c.rid)*2246822519+13) % uint32(ks))
	if k == first {
		k = (k + 1) % ks
	}
	return k
}

func (s *Sim) dbTransfer(n *node, c *call, cb func(Result), release func()) {
	ep := n.epoch
	k1, k2 := c.key, secondAccount(c, c.key)
	oncall := c.wl == wlOnCall
	if oncall {
		k2 = partnerRow(c, k1)
	}
	level := n.s("isolation")
	s.txnSeq++
	id := s.txnSeq
	var held []int
	unlockAll := func() {
		for _, k := range held {
			s.txRelease(n, id, k)
		}
		held = nil
	}
	dead := func() bool {
		if n.epoch != ep || n.removed {
			release()
			s.failAt(n, RNodeDown, cb)
			return true
		}
		return false
	}
	abort := func(deadlock bool) {
		unlockAll()
		release()
		if c.counted {
			s.runTxAborts++
			if deadlock {
				s.runDeadlocks++
			}
		}
		s.failAt(n, RTxAborted, cb)
	}
	body := func() {
		v1, v2 := n.tver[k1], n.tver[k2]
		s.after(s.service(n, 2*n.f("readMs"))*n.slow, func() {
			if dead() {
				return
			}
			writes := 2.0
			if oncall {
				writes = 1 // only its own row
			}
			s.after(s.service(n, writes*n.f("writeMs")*s.writeMult(n))*n.slow, func() {
				if dead() {
					return
				}
				changed := 0
				if n.tver[k1] != v1 {
					changed++
				}
				if oncall {
					// the partner row is only read: a change there is not a conflict, it is the skew
					if n.tver[k2] != v2 && changed == 0 && c.counted {
						s.runWriteSkew++
					}
				} else if n.tver[k2] != v2 {
					changed++
				}
				if changed > 0 && level == "repeatableRead" {
					abort(false)
					return
				}
				if changed > 0 && level == "readCommitted" && c.counted {
					s.runLostUpdates += changed
				}
				n.tver[k1]++
				if !oncall {
					n.tver[k2]++
				}
				ver := s.commit(n, c)
				unlockAll()
				release()
				s.okAt(n, Result{Ver: ver, Done: true, Src: n}, cb)
			})
		})
	}
	if level != "serializable" {
		body()
		return
	}
	lock := func(key int, next func()) {
		s.txAcquire(n, id, key, func() {
			if n.epoch != ep || n.removed {
				return
			}
			held = append(held, key)
			next()
		}, func() { abort(true) })
	}
	lock(k1, func() { lock(k2, body) })
}

// partnerRow is the other row of the pair a key belongs to: keys 1 and 2 are a pair, 3 and 4, and so on.
func partnerRow(c *call, key int) int {
	p := key + 1
	if key%2 == 0 {
		p = key - 1
	}
	if c.lg != nil {
		if ks := int(c.lg.client.f("keySpace")); ks > 1 && p > ks {
			p = key - 1
		}
	}
	return p
}
