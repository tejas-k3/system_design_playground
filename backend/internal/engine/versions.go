package engine

import "math"

// Versions: per-item version history, staleness accounting and idempotent commits.

// verLog is the recent version history of one item.
type verLog struct {
	v []int
	t []float64
}

type commitRec struct {
	rid int
	t   float64
}

// SeriesPoint is one point of the global time series.

func (s *Sim) itemVer(key int) int {
	if v := s.vers[key]; v != nil {
		return v.v[len(v.v)-1]
	}
	return 0
}

func (s *Sim) verAt(key int, t float64) int {
	v := s.vers[key]
	if v == nil {
		return 0
	}
	for i := len(v.t) - 1; i >= 0; i-- {
		if v.t[i] <= t {
			return v.v[i]
		}
	}
	return int(math.Max(0, float64(v.v[0]-1)))
}

func (s *Sim) bumpVer(key int) int {
	v := s.vers[key]
	if v == nil {
		v = &verLog{}
		s.vers[key] = v
	}
	n := 1
	if len(v.v) > 0 {
		n = v.v[len(v.v)-1] + 1
	}
	v.v = append(v.v, n)
	v.t = append(v.t, s.now)
	if len(v.v) > verKeep {
		v.v, v.t = v.v[1:], v.t[1:]
	}
	return n
}

func maxStale(cl *node) float64 {
	switch cl.s("consistency") {
	case "strong":
		return 0
	case "bounded":
		return math.Max(0, cl.f("maxStaleMs"))
	}
	return -1
}

func (s *Sim) countFreshness(lg *logical, res Result) {
	if lg.write || res.Ver < 0 {
		return
	}
	if lg.client.s("consistency") == "boundedWrites" { // a numerical bound: how many committed writes the read may have missed
		s.runFresh++
		if s.verAt(lg.key, lg.createdAt)-res.Ver > int(lg.client.f("maxMissedWrites")) {
			s.runStale++
			if res.Src != nil {
				res.Src.c.stale++
			}
		}
		return
	}
	ms := maxStale(lg.client)
	if ms < 0 {
		return
	}
	s.runFresh++
	if s.verAt(lg.key, lg.createdAt-ms) > res.Ver {
		s.runStale++
		if res.Src != nil {
			res.Src.c.stale++
		}
	}
}

// causalState is what a client that asks for causal order has seen: the last version it read, and for each item the
// oldest version it may still be shown, because an effect it saw was built on that.
type causalState struct {
	lastKey, lastVer int
	min              map[int]int
}

// countCausalRead counts a read of a cause that is older than an effect this client has already seen, then notes what
// the read implies: the item itself may not go backwards, and what a write it shows was built on may not either.
func (s *Sim) countCausalRead(lg *logical, res Result) {
	if lg.write {
		return
	}
	cl := lg.client
	if cl.causal == nil {
		cl.causal = &causalState{min: map[int]int{}}
	}
	cz := cl.causal
	if lg.counted && res.Ver < cz.min[lg.key] {
		s.runCausalBroken++
	}
	if res.Ver > cz.min[lg.key] {
		cz.min[lg.key] = res.Ver
	}
	if dep, ok := s.causalDeps[[2]int{lg.key, res.Ver}]; ok && dep[1] > cz.min[dep[0]] {
		cz.min[dep[0]] = dep[1]
	}
	cz.lastKey, cz.lastVer = lg.key, res.Ver
}

// stampSeen notes on a request what its client knew when it was issued: the newest version it had of this item, and, for
// a write by a client asking for causal order, the last thing it read, which the write is built on (so a read made
// while the write is in flight does not become its cause).
func (c *node) stampSeen(lg *logical) {
	lg.sawWrite, lg.sawRead = c.sessWrite[lg.key], c.sessRead[lg.key] // nil maps read as 0
	if lg.write && c.causal != nil {
		lg.sawWrite, lg.sawRead = c.causal.lastKey, c.causal.lastVer // here: the item and version it read last
	}
}

// recordCause remembers, for a write by a client that asks for causal order, the read it was built on. Version 0 is an
// item never written, which no read can be older than, so it is not a cause worth keeping.
func (s *Sim) recordCause(c *call, ver int) {
	if c.lg == nil || !c.lg.write || c.lg.client.s("consistency") != "causalOrder" || c.lg.sawRead <= 0 {
		return
	}
	if s.causalDeps == nil {
		s.causalDeps = map[[2]int][2]int{}
	}
	s.causalDeps[[2]int{c.key, ver}] = [2]int{c.lg.sawWrite, c.lg.sawRead}
	if len(s.causalDeps) >= max(causalKeepMax, s.causalPruneAt) {
		s.pruneCauses()
	}
}

// causalKeepMax is how many dependency records are kept before the old ones are looked for, and causalWindow how many
// versions back from an item's newest a read can still return (copies lag by a few versions, not by hundreds).
const (
	causalKeepMax = 4096
	causalWindow  = 64
)

// pruneCauses drops the dependencies of versions too old for any read to return. The next look comes when the records
// have doubled, so a design with many live items is not scanned on every write.
func (s *Sim) pruneCauses() {
	for k := range s.causalDeps {
		if k[1] < s.itemVer(k[0])-causalWindow {
			delete(s.causalDeps, k)
		}
	}
	s.causalPruneAt = 2 * len(s.causalDeps)
}

// ensureSessionMaps lazily creates the maps used only by clients with per-client consistency guarantees.
func (cl *node) ensureSessionMaps() {
	if cl.sessWrite == nil {
		cl.sessWrite, cl.sessRead = map[int]int{}, map[int]int{}
	}
}

// countSession checks reads against what this client already knows and records reads for later session checks.
func (s *Sim) countSession(lg *logical, res Result) {
	if res.Ver < 0 {
		return
	}
	cl := lg.client
	model := cl.s("consistency")
	if model == "causalOrder" {
		s.countCausalRead(lg, res)
		return
	}
	if model != "readYourWrites" && model != "monotonic" && model != "causal" && model != "monotonicWrites" && model != "writesFollowReads" {
		return
	}
	cl.ensureSessionMaps()
	if lg.write {
		if model == "monotonicWrites" || model == "writesFollowReads" {
			return
		}
		if res.Ver > cl.sessWrite[lg.key] {
			cl.sessWrite[lg.key] = res.Ver
		}
		return
	}
	if lg.counted {
		if model != "monotonicWrites" {
			s.runSessReads++
		}
		if (model == "readYourWrites" || model == "causal") && res.Ver < lg.sawWrite {
			s.runRYW++
		}
		if (model == "monotonic" || model == "causal") && res.Ver < lg.sawRead {
			s.runMono++
		}
	}
	if res.Ver > cl.sessRead[lg.key] {
		cl.sessRead[lg.key] = res.Ver
	}
}

// countMonotonicWrite counts a client's write that commits after a later one to the same key. It compares the order
// writes were issued (the logical request ids) with the order they commit, which timing alone can swap when
// several of a client's writes are in flight, so quorum settings do not remove it. A logical write is counted once:
// a negative sawWrite marks it, since that field is otherwise unused for this model and a new field on every
// request would cost memory on the default path.
func (s *Sim) countMonotonicWrite(c *call) {
	lg := c.lg
	if lg == nil || lg.client.s("consistency") != "monotonicWrites" {
		return
	}
	cl := lg.client
	cl.ensureSessionMaps()
	if lg.id < cl.sessWrite[c.key] {
		if lg.sawWrite >= 0 {
			lg.sawWrite = -1 // a negative value marks this logical write as already counted
			if lg.counted {
				s.runMonoWriteBroken++
			}
		}
		return
	}
	cl.sessWrite[c.key] = lg.id
}

// countWritesFollowReads counts a write that reaches a key-value copy older than a value its client has already read.
// Only the key-value store keeps a version for each copy, so no other store reports it. Counted once, like the above.
func (s *Sim) countWritesFollowReads(lg *logical, version int) {
	if lg == nil || lg.client.s("consistency") != "writesFollowReads" || lg.sawRead <= version || lg.sawWrite < 0 {
		return
	}
	lg.sawWrite = -1 // a negative value marks this logical write as already counted
	if lg.counted {
		s.runFollowReadBroken++
	}
}

func (s *Sim) commit(n *node, c *call) int {
	if c.rid != 0 {
		if _, seen := s.committed[c.rid]; seen {
			if c.idem {
				return s.itemVer(c.key)
			}
			n.c.dup++
			if c.counted {
				s.runDup++
			}
		} else {
			s.committed[c.rid] = s.now
			s.commitLog = append(s.commitLog, commitRec{c.rid, s.now})
		}
	}
	if n.logWrites {
		n.writeLog = append(n.writeLog, s.now)
	}
	s.countMonotonicWrite(c)
	v := s.bumpVer(c.key)
	s.recordCause(c, v)
	return v
}

func (s *Sim) pruneCommitted() {
	cut := s.now - commitKeep
	for s.commitHd < len(s.commitLog) && s.commitLog[s.commitHd].t < cut {
		delete(s.committed, s.commitLog[s.commitHd].rid)
		s.commitHd++
	}
	if s.commitHd > 4096 && s.commitHd > len(s.commitLog)/2 {
		s.commitLog = append([]commitRec(nil), s.commitLog[s.commitHd:]...)
		s.commitHd = 0
	}
}

// truncateVersions forgets every item version written after cutoff (they never reached the replica that took
// over) and returns how many writes that removed.
func (s *Sim) truncateVersions(cutoff float64) int {
	lost := 0
	for key, v := range s.vers {
		keep := len(v.t)
		for keep > 0 && v.t[keep-1] > cutoff {
			keep--
		}
		lost += len(v.t) - keep
		if keep == 0 {
			delete(s.vers, key) // an item that only ever existed in the lost window is simply gone
			continue
		}
		v.v, v.t = v.v[:keep], v.t[:keep]
	}
	return lost
}
