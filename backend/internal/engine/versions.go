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

// countSession checks a read against what this client already knows, for the per-client models: read-your-writes
// (a read must not be older than the client's own last acknowledged write of the key), monotonic reads (a read must
// not be older than one the client already had), and causal, which asks for both.
func (s *Sim) countSession(lg *logical, res Result) {
	if res.Ver < 0 {
		return
	}
	cl := lg.client
	model := cl.s("consistency")
	if model != "readYourWrites" && model != "monotonic" && model != "causal" {
		return
	}
	if cl.sessWrite == nil {
		cl.sessWrite, cl.sessRead = map[int]int{}, map[int]int{}
	}
	if lg.write {
		if res.Ver > cl.sessWrite[lg.key] {
			cl.sessWrite[lg.key] = res.Ver
		}
		return
	}
	if lg.counted {
		s.runSessReads++
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
	return s.bumpVer(c.key)
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
