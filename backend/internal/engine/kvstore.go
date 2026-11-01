package engine

import (
	"math"
	"sort"
)

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
		at := make([]float64, nodes)
		copy(at, n.kvDownAt)
		n.kvDownAt = at
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
	first := kvModFirst(c.key, nodes)
	s.kvAntiEntropy(n)
	hinted, sloppy, repair := n.b("hintedHandoff"), n.b("sloppyQuorum"), n.b("readRepair")
	kx, siblings := n.kvX(), n.b("siblings")
	who := kx.writer(c)
	var base, nvv kvVV
	if write && siblings {
		base, nvv = kx.nextVV(who, c.key)
	}
	pref := kx.place(n, nodes, copies, c.key, write && (hinted || sloppy))
	at := func(j int) int {
		if pref != nil {
			return pref[j]
		}
		return (first + j) % nodes
	}
	var sets []map[int][]kvSib
	if siblings {
		sets = kx.copies(nodes)
	}
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
	coord, stale := kvCoordinator(c, nodes), 0
	if m := n.s("failureDetection"); m == "gossip" || m == "broadcast" {
		for tries := 0; tries < nodes && n.kvDownUntil[coord] > s.now; tries++ {
			coord = (coord + 1) % nodes // a dead node receives nothing: the request lands on the next one
		}
	}
	for j := range copies {
		i := at(j)
		if n.kvDownUntil[i] <= s.now {
			replies = append(replies, reply{i, -1, slot(i, ms), true})
			counting++
			continue
		}
		if !s.kvKnowsDown(n, coord, i) { // the request goes to a node that is dead; nobody has told this coordinator
			stale++
			n.kvStaleView++
			continue
		}
		if !write || (!hinted && !sloppy) {
			continue
		}
		for ; next < nodes; next++ { // a stand-in keeps the write as a hint for the node that is down
			if st := at(next); n.kvDownUntil[st] <= s.now {
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
		if stale > 0 { // it waited for a reply that never came
			s.after(n.f("probeTimeoutMs"), func() { s.failAt(n, RQuorum, cb) })
			return
		}
		s.failAt(n, RQuorum, cb)
		return
	}
	sort.SliceStable(replies, func(a, b int) bool { return replies[a].at < replies[b].at })
	if n.kvHints == nil {
		n.kvHints = map[int][][2]int{}
	}
	apply := func(r reply, ver int) {
		if r.target >= 0 {
			s.countWritesFollowReads(c.lg, n.kvVer[r.target][c.key]) // the copy it is held for is the one it will land on
			n.kvHints[r.target] = append(n.kvHints[r.target], [2]int{c.key, ver})
			if siblings {
				if kx.hintSibs == nil {
					kx.hintSibs = map[int][]kvHintSib{}
				}
				kx.hintSibs[r.target] = append(kx.hintSibs[r.target], kvHintSib{c.key, kvSib{nvv, ver, who}})
			}
		} else {
			s.countWritesFollowReads(c.lg, n.kvVer[r.node][c.key])
			if ver > n.kvVer[r.node][c.key] {
				n.kvVer[r.node][c.key] = ver
			}
			if siblings {
				sets[r.node][c.key] = kvAddSibling(sets[r.node][c.key], kvSib{nvv, ver, who})
			}
		}
	}
	// A write becomes visible (gets its version) when it is acknowledged, that is when W copies have it; the copies
	// that finished earlier hold it from that moment, the rest as each of them finishes.
	var pending []reply
	acked, counted, seen, newest, wver := false, 0, 0, 0, 0
	var got []kvSib
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
					kx.committed(siblings, who, c.key, wver, base, nvv)
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
			if siblings && seen <= need {
				got = kvUnion(got, sets[rp.node][c.key])
			}
			if seen == need {
				if siblings {
					if len(got) > 1 {
						kx.sibReads++
						kx.seenSibs += len(got)
					}
					for _, g := range got {
						kx.learn(who, c.key, g.vv)
					}
				} else {
					kx.sawVersion(who, c.key, newest)
				}
				s.okAt(n, Result{Ver: newest, Done: true, Src: n}, cb)
			}
			if repair && seen == len(replies) { // every copy has answered: bring the old ones up to date
				if siblings {
					var all []kvSib
					for _, e := range replies {
						all = kvUnion(all, sets[e.node][c.key])
					}
					for _, e := range replies {
						if e.target < 0 {
							sets[e.node][c.key] = all
						}
					}
				}
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
	fresh := false
	for i := 0; i < count && i < nodes; i++ {
		if n.kvDownUntil[i] > s.now { // already down: nothing new to tell
			continue
		}
		n.kvDownUntil[i], n.kvDownAt[i] = until, s.now
		fresh = true
		if dur > 0 {
			i := i
			s.at(until, func() { s.kvReplayHints(n, i) })
		}
	}
	if fresh {
		s.kvSpreadNews(n, nodes)
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
	if kx := n.kx; kx != nil && len(kx.hintSibs[i]) > 0 {
		sets := kx.copies(len(n.kvVer))
		for _, h := range kx.hintSibs[i] {
			sets[i][h.key] = kvAddSibling(sets[i][h.key], h.s)
		}
		delete(kx.hintSibs, i)
	}
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
		kx, siblings := n.kvX(), n.b("siblings")
		keys := map[int]struct{}{}
		for i := range n.kvVer {
			for k := range n.kvVer[i] {
				keys[k] = struct{}{}
			}
		}
		for k := range keys {
			first := kvModFirst(k, nodes)
			pref := kx.place(n, nodes, copies, k, false)
			at := func(j int) int {
				if pref != nil {
					return pref[j]
				}
				return (first + j) % nodes
			}
			newest := 0
			var all []kvSib
			for j := range copies {
				if v := n.kvVer[at(j)][k]; v > newest {
					newest = v
				}
				if siblings {
					all = kvUnion(all, kx.copies(nodes)[at(j)][k])
				}
			}
			for j := range copies {
				i := at(j)
				if n.kvDownUntil[i] <= s.now && n.kvVer[i][k] < newest { // a node that is down cannot be repaired yet
					n.kvVer[i][k] = newest
					n.kvAERepairs++
				}
				if siblings && n.kvDownUntil[i] <= s.now {
					kx.copies(nodes)[i][k] = all
				}
			}
		}
		s.after(every, round)
	}
	s.after(every, round)
}

// How the cluster finds out that a node is dead. Instantly (the default); by gossip, where every round each node that
// knows tells `fanout` others, so the number that know grows by a factor of about 1+fanout per round until everyone
// does; or by telling every node at once. The request's coordinator is a hash of the request, so no random number is
// drawn: it learns when the news has reached its place in the order in which nodes hear.

func kvCoordinator(c *call, nodes int) int {
	return int(Hash32(uint32(c.rid)*2246822519+101) % uint32(nodes))
}

// gossipInformed is how many of n nodes know after r rounds when one node starts and each tells fanout others.
func gossipInformed(n int, fanout float64, r int) float64 {
	know := 1.0
	for ; r > 0 && know < float64(n); r-- {
		know = min(float64(n), math.Ceil(know+fanout*know*(1-know/float64(n)))) // whole nodes
	}
	return know
}

// gossipRounds is the first round after which at least `need` of n nodes know.
func gossipRounds(n int, fanout float64, need int) int {
	r := 0
	for gossipInformed(n, fanout, r) < float64(need) && r < 500 {
		r++
	}
	return r
}

// kvSpreadNews records the cost of telling the cluster about a failure: the time until everyone knows and the messages.
func (s *Sim) kvSpreadNews(n *node, nodes int) {
	mode := n.s("failureDetection")
	if mode != "gossip" && mode != "broadcast" {
		return
	}
	detect := n.f("detectMs")
	n.kvHear = nil // recomputed from the settings the first time a request needs it
	if mode == "broadcast" {
		n.kvTimeToAll = detect + n.f("broadcastMs")
		n.kvMemberMsgs += float64(nodes - 1)
		return
	}
	f := n.f("gossipFanout")
	rounds := gossipRounds(nodes, f, nodes)
	n.kvTimeToAll = detect + float64(rounds)*n.f("gossipIntervalMs")
	for r := range rounds {
		n.kvMemberMsgs += gossipInformed(nodes, f, r) * f
	}
}

// kvKnowsDown is whether the coordinator has heard by now that node `dead` is down.
func (s *Sim) kvKnowsDown(n *node, coord, dead int) bool {
	mode := n.s("failureDetection")
	if (mode != "gossip" && mode != "broadcast") || coord == dead {
		return true
	}
	nodes := len(n.kvDownUntil)
	delay := n.f("detectMs")
	if mode == "broadcast" {
		delay += n.f("broadcastMs")
	} else {
		if len(n.kvHear) != nodes-1 { // rounds after which the 1st, 2nd, ... node to hear has heard
			n.kvHear = make([]int, nodes-1)
			for r := range n.kvHear {
				n.kvHear[r] = gossipRounds(nodes, n.f("gossipFanout"), r+2)
			}
		}
		rank := (coord-dead+nodes)%nodes - 1 // 0..nodes-2: the order in which the others hear
		delay += float64(n.kvHear[rank]) * n.f("gossipIntervalMs")
	}
	return s.now >= n.kvDownAt[dead]+delay
}
