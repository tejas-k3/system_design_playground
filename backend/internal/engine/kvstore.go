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
	type reply struct {
		node int
		at   float64
	}
	var replies []reply
	for j := 0; j < copies; j++ {
		i := (first + j) % nodes
		if n.kvDownUntil[i] > s.now {
			continue
		}
		slot := 0
		for k, f := range n.kvFree[i] {
			if f < n.kvFree[i][slot] {
				slot = k
			}
		}
		start := max(s.now, n.kvFree[i][slot])
		done := start + s.service(n, ms)*n.slow
		n.kvFree[i][slot] = done
		replies = append(replies, reply{i, done})
	}
	if len(replies) < need {
		s.failAt(n, RQuorum, cb)
		return
	}
	sort.SliceStable(replies, func(a, b int) bool { return replies[a].at < replies[b].at })
	// A write becomes visible (gets its version) when it is acknowledged, that is when W copies have it; the copies
	// that finished earlier hold it from that moment, the rest as each of them finishes.
	ver, seen, newest := -1, 0, 0
	for idx, rp := range replies {
		idx, rp := idx, rp
		s.at(rp.at, func() {
			seen++
			switch {
			case write && seen < need:
				return
			case write && seen == need:
				ver = s.commit(n, c)
				for _, e := range replies[:idx+1] {
					if ver > n.kvVer[e.node][c.key] {
						n.kvVer[e.node][c.key] = ver
					}
				}
				s.okAt(n, Result{Ver: ver, Done: true, Src: n}, cb)
			case write:
				if ver > n.kvVer[rp.node][c.key] {
					n.kvVer[rp.node][c.key] = ver
				}
			default:
				if v := n.kvVer[rp.node][c.key]; v > newest {
					newest = v
				}
				if seen == need {
					s.okAt(n, Result{Ver: newest, Done: true, Src: n}, cb)
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
	}
}
