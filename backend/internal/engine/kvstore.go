package engine

import "sort"

// The quorum key-value store: one component standing for a cluster of nodes that keep N copies of every key. A
// write goes to all N copies and is acknowledged once W of them have it; a read asks all N and answers with the
// newest version among the first R that reply. So latency is the Wth (or Rth) fastest of N, and a read can return an
// old version exactly when R + W <= N: the copies it asks need not include one the last write reached.

func (n *node) kvEnsure(nodes, slots int) {
	if len(n.kvFree) != nodes {
		free := make([][]float64, nodes)
		ver := make([]map[int]int, nodes)
		down := make([]float64, nodes)
		for i := range free {
			if i < len(n.kvFree) {
				free[i], ver[i], down[i] = n.kvFree[i], n.kvVer[i], n.kvDownUntil[i]
			} else {
				ver[i] = map[int]int{}
			}
		}
		n.kvFree, n.kvVer, n.kvDownUntil = free, ver, down
	}
	for i := range n.kvFree {
		if len(n.kvFree[i]) != slots {
			f := make([]float64, slots)
			copy(f, n.kvFree[i])
			n.kvFree[i] = f
		}
	}
}

func clampInt(v, lo, hi int) int { return max(lo, min(hi, v)) }

// kvShape reads the cluster settings: nodes, copies per key, and the two quorums, each kept within what is possible.
func kvShape(n *node) (nodes, copies, r, w int) {
	nodes = clampInt(int(n.f("nodes")), 1, 64)
	copies = clampInt(int(n.f("replicas")), 1, nodes)
	r = clampInt(int(n.f("readQuorum")), 1, copies)
	w = clampInt(int(n.f("writeQuorum")), 1, copies)
	return
}

func (s *Sim) procKV(n *node, c *call, cb func(Result), release func()) {
	release()
	if c.cmd != cmdRead && c.cmd != cmdWrite {
		s.failAt(n, RNoRoute, cb)
		return
	}
	nodes, copies, rq, wq := kvShape(n)
	n.kvEnsure(nodes, clampInt(int(n.f("concurrency")), 1, 1024))
	write := c.cmd == cmdWrite
	need, ms := rq, n.f("readMs")
	if write {
		need, ms = wq, n.f("writeMs")
	}
	first := int(Hash32(uint32(c.key)*2654435761+11) % uint32(nodes))
	s.kvAntiEntropy(n)
	hinted, sloppy, repair := n.b("hintedHandoff"), n.b("sloppyQuorum"), n.b("readRepair")
	type reply struct {
		node   int
		target int // a stand-in's reply: the node it holds the write for (-1 = a real copy)
		at     float64
		counts bool // counts towards the quorum
	}
	var replies []reply
	slot := func(i int, ms float64) float64 {
		k0 := 0
		for k, f := range n.kvFree[i] {
			if f < n.kvFree[i][k0] {
				k0 = k
			}
		}
		start := max(s.now, n.kvFree[i][k0])
		done := start + s.service(n, ms)*n.slow
		n.kvFree[i][k0] = done
		return done
	}
	next := copies // where stand-ins are looked for: the nodes after the preference list
	counting := 0
	for j := 0; j < copies; j++ {
		i := (first + j) % nodes
		if n.kvDownUntil[i] <= s.now {
			replies = append(replies, reply{i, -1, slot(i, ms), true})
			counting++
			continue
		}
		if !write || (!hinted && !sloppy) {
			continue
		}
		for ; next < nodes; next++ { // a stand-in keeps the write as a hint for the node that is down
			if st := (first + next) % nodes; n.kvDownUntil[st] <= s.now {
				replies = append(replies, reply{st, i, slot(st, ms), sloppy})
				if sloppy {
					counting++
				}
				next++
				break
			}
		}
	}
	if counting < need {
		s.failAt(n, RQuorum, cb)
		return
	}
	sort.SliceStable(replies, func(a, b int) bool { return replies[a].at < replies[b].at })
	if n.kvHints == nil {
		n.kvHints = map[int][][2]int{}
	}
	apply := func(r reply, ver int) {
		if r.target >= 0 {
			n.kvHints[r.target] = append(n.kvHints[r.target], [2]int{c.key, ver})
		} else if ver > n.kvVer[r.node][c.key] {
			n.kvVer[r.node][c.key] = ver
		}
	}
	// A write becomes visible (gets its version) when it is acknowledged, that is when W copies have it; the copies
	// that finished earlier hold it from that moment, the rest as each of them finishes.
	var pending []reply
	acked, counted, seen, newest, wver := false, 0, 0, 0, 0
	for _, rp := range replies {
		rp := rp
		s.at(rp.at, func() {
			seen++
			if write {
				if acked {
					apply(rp, wver)
					return
				}
				pending = append(pending, rp)
				if rp.counts {
					counted++
				}
				if counted == need {
					acked = true
					wver = s.commit(n, c)
					for _, e := range pending {
						apply(e, wver)
					}
					s.okAt(n, Result{Ver: wver, Done: true, Src: n}, cb)
				}
				return
			}
			if v := n.kvVer[rp.node][c.key]; v > newest {
				newest = v
			}
			if seen == need {
				s.okAt(n, Result{Ver: newest, Done: true, Src: n}, cb)
			}
			if repair && seen == len(replies) { // every copy has answered: bring the old ones up to date
				for _, e := range replies {
					if n.kvVer[e.node][c.key] < newest {
						n.kvVer[e.node][c.key] = newest
						n.kvRepairs++
					}
				}
			}
		})
	}
}

// kvNodesDown takes the first `count` nodes of the cluster out for `dur` ms (0 = for good).
func (s *Sim) kvNodesDown(n *node, count int, dur float64) {
	nodes, _, _, _ := kvShape(n)
	n.kvEnsure(nodes, clampInt(int(n.f("concurrency")), 1, 1024))
	until := 1e18
	if dur > 0 {
		until = s.now + dur
	}
	for i := 0; i < count && i < nodes; i++ {
		n.kvDownUntil[i] = until
		if dur > 0 {
			i := i
			s.at(until, func() { s.kvReplayHints(n, i) })
		}
	}
}

// kvReplayHints hands a node that is back the writes the other nodes kept for it while it was down.
func (s *Sim) kvReplayHints(n *node, i int) {
	for _, h := range n.kvHints[i] {
		if h[1] > n.kvVer[i][h[0]] {
			n.kvVer[i][h[0]] = h[1]
		}
		n.kvHintsReplayed++
	}
	delete(n.kvHints, i)
}

// kvAntiEntropy starts the periodic repair rounds (once): each round brings every copy of every key that a live node
// holds up to the newest version among that key's copies.
func (s *Sim) kvAntiEntropy(n *node) {
	every := n.f("antiEntropySec") * 1000
	if every <= 0 || n.kvAEOn {
		return
	}
	n.kvAEOn = true
	var round func()
	round = func() {
		nodes, copies, _, _ := kvShape(n)
		keys := map[int]struct{}{}
		for i := range n.kvVer {
			for k := range n.kvVer[i] {
				keys[k] = struct{}{}
			}
		}
		for k := range keys {
			first := int(Hash32(uint32(k)*2654435761+11) % uint32(nodes))
			newest := 0
			for j := 0; j < copies; j++ {
				if v := n.kvVer[(first+j)%nodes][k]; v > newest {
					newest = v
				}
			}
			for j := 0; j < copies; j++ {
				i := (first + j) % nodes
				if n.kvDownUntil[i] <= s.now && n.kvVer[i][k] < newest { // a node that is down cannot be repaired yet
					n.kvVer[i][k] = newest
					n.kvAERepairs++
				}
			}
		}
		s.after(every, round)
	}
	s.after(every, round)
}
