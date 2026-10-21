package engine

import "sort"

// Shard routing by key. The default maps a key to child number hash mod n, so adding a child remaps almost every
// key. A consistent-hashing ring puts each child on the ring at several points (virtual nodes) and sends a key to
// the first point at or after its hash, so a new child takes over only about 1/n of the keys.

type ringPoint struct {
	h  uint32
	to string
}

type ring struct {
	sig    string // which children and how many points each: the ring is rebuilt when it changes
	points []ringPoint
}

func strHash(s string) uint32 { // FNV-1a, stable across runs and platforms
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h = (h ^ uint32(s[i])) * 16777619
	}
	return h
}

func (s *Sim) ringFor(t *node, cand []*edge) *ring {
	v := int(t.f("vnodes"))
	sig := ""
	for _, e := range cand {
		sig += e.to.id + ","
	}
	sig += string(rune(v))
	if t.hashRing != nil && t.hashRing.sig == sig {
		return t.hashRing
	}
	r := &ring{sig: sig}
	for _, e := range cand {
		for i := 0; i < v; i++ {
			r.points = append(r.points, ringPoint{Hash32(strHash(e.to.id) + uint32(i)*0x9e3779b1), e.to.id})
		}
	}
	sort.Slice(r.points, func(i, j int) bool {
		if r.points[i].h != r.points[j].h {
			return r.points[i].h < r.points[j].h
		}
		return r.points[i].to < r.points[j].to
	})
	t.hashRing = r
	return r
}

// pickShard chooses the child edge that serves a key hash.
func (s *Sim) pickShard(t *node, cand []*edge, h uint32) *edge {
	if t.s("hashing") != "ring" || len(cand) == 1 {
		return cand[int(Hash32(h)%uint32(len(cand)))]
	}
	r := s.ringFor(t, cand)
	kh := Hash32(h)
	i := sort.Search(len(r.points), func(i int) bool { return r.points[i].h >= kh })
	if i == len(r.points) {
		i = 0
	}
	for _, e := range cand {
		if e.to.id == r.points[i].to {
			return e
		}
	}
	return cand[0]
}

// shardChildren are the edges a router sends keys to: databases if it has any, otherwise Redis nodes.
func shardChildren(t *node) []*edge {
	var db, rd []*edge
	for _, x := range t.out {
		if x.removed {
			continue
		}
		if x.to.kind == KRedis {
			rd = append(rd, x)
		} else {
			db = append(db, x)
		}
	}
	if len(db) > 0 {
		return db
	}
	return rd
}

const reshardSample = 4096

// noteReshard compares where a sample of keys goes now with where it went last time, and records the share that
// moved when the set of children changed. Called after a live edit.
func (s *Sim) noteReshard() {
	for _, t := range s.order {
		if t.removed || t.kind != KShardRouter {
			continue
		}
		cand := shardChildren(t)
		assign := make([]string, reshardSample)
		if len(cand) > 0 {
			for k := range assign {
				assign[k] = s.pickShard(t, cand, uint32(k)+0x9e3779b9).to.id
			}
		}
		if t.assign != nil {
			moved := 0
			was := make([]bool, reshardSample)
			for k := range assign {
				if assign[k] != t.assign[k] {
					moved++
					was[k] = true
				}
			}
			if moved > 0 || len(cand) != t.shardCount {
				t.keysMoved, t.reshards = float64(moved)/reshardSample, t.reshards+1
				if sec := t.f("rebalanceSec"); sec > 0 && moved > 0 {
					t.movedKeys, t.moveUntil = was, s.now+sec*1000
				}
			}
		}
		t.assign, t.shardCount = assign, len(cand)
	}
}

// ringImbalance is how much more than its fair share the busiest child gets, over n keys.
func (s *Sim) ringImbalance(t *node, n int) float64 {
	cand := shardChildren(t)
	count := map[string]int{}
	for k := 0; k < n; k++ {
		count[s.pickShard(t, cand, uint32(k)*2654435761+7).to.id]++
	}
	max := 0
	for _, c := range count {
		if c > max {
			max = c
		}
	}
	return float64(max) / (float64(n) / float64(len(cand)))
}
