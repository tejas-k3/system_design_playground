package engine

import (
	"math"
	"math/bits"
	"sort"
)

// Probabilistic structures: a Bloom filter (is it in the set?), a count-min sketch (how often?) and HyperLogLog
// (how many different?). Each trades a small, measurable error for a fixed small memory.

func hash64(x, seed uint32) uint64 {
	return uint64(Hash32(x^seed*0x9e3779b1))<<32 | uint64(Hash32(x*0x85ebca6b+seed+0x7f4a7c15))
}

// Bloom is m bits and k hash functions: no false negatives, and a false-positive rate of (1 - e^(-kn/m))^k after n adds.
type Bloom struct {
	bits []uint64
	m, k int
}

func NewBloom(m, k int) *Bloom { return &Bloom{bits: make([]uint64, (m+63)/64), m: m, k: k} }

func (b *Bloom) positions(x uint32, f func(pos int) bool) bool {
	h := hash64(x, 1)
	h1, h2 := uint32(h>>32), uint32(h)|1
	for i := 0; i < b.k; i++ {
		if !f(int((h1 + uint32(i)*h2) % uint32(b.m))) {
			return false
		}
	}
	return true
}

func (b *Bloom) Add(x uint32) {
	b.positions(x, func(p int) bool { b.bits[p/64] |= 1 << (p % 64); return true })
}

func (b *Bloom) Has(x uint32) bool {
	return b.positions(x, func(p int) bool { return b.bits[p/64]&(1<<(p%64)) != 0 })
}

// CountMin has depth rows of width counters. An estimate is the smallest counter of the item: never too low, and
// too high by more than epsilon x N with probability at most delta, where width = e / epsilon and depth = ln(1 / delta).
type CountMin struct {
	t    [][]uint32
	w    int
	seed uint32
}

func NewCountMin(width, depth int, seed uint32) *CountMin {
	t := make([][]uint32, depth)
	for i := range t {
		t[i] = make([]uint32, width)
	}
	return &CountMin{t: t, w: width, seed: seed}
}

func (c *CountMin) slot(row int, x uint32) int {
	return int(Hash32(x*0x9e3779b1+c.seed+uint32(row)*0x85ebca6b+uint32(row)) % uint32(c.w))
}

func (c *CountMin) Add(x uint32, n int) {
	for r := range c.t {
		c.t[r][c.slot(r, x)] += uint32(n)
	}
}

func (c *CountMin) Estimate(x uint32) int {
	best := uint32(math.MaxUint32)
	for r := range c.t {
		if v := c.t[r][c.slot(r, x)]; v < best {
			best = v
		}
	}
	return int(best)
}

// HLL is HyperLogLog with 2^p registers; its relative standard error is 1.04 / sqrt(2^p).
type HLL struct {
	reg []uint8
	p   uint
}

func NewHLL(p uint) *HLL { return &HLL{reg: make([]uint8, 1<<p), p: p} }

func (h *HLL) Add(x uint32) {
	v := hash64(x, 7)
	idx := v >> (64 - h.p)
	rest := v<<h.p | 1<<(h.p-1) // the lowest set bit guards the count
	rank := uint8(bits.LeadingZeros64(rest)) + 1
	if rank > h.reg[idx] {
		h.reg[idx] = rank
	}
}

func (h *HLL) Count() float64 {
	m := float64(len(h.reg))
	alpha := 0.7213 / (1 + 1.079/m)
	var sum float64
	zeros := 0
	for _, r := range h.reg {
		sum += math.Pow(2, -float64(r))
		if r == 0 {
			zeros++
		}
	}
	est := alpha * m * m / sum
	if est <= 2.5*m && zeros > 0 { // small cardinalities: count the empty registers instead
		return m * math.Log(m/float64(zeros))
	}
	return est
}

// ExactTopK counts every item exactly: perfect, and one counter for each distinct item seen.
type ExactTopK struct{ n map[uint32]int }

func NewExactTopK() *ExactTopK     { return &ExactTopK{n: map[uint32]int{}} }
func (e *ExactTopK) Add(x uint32)  { e.n[x]++ }
func (e *ExactTopK) Counters() int { return len(e.n) }
func (e *ExactTopK) Top(k int) []uint32 {
	return topOf(e.n, k)
}

// SketchTopK finds heavy hitters in fixed memory: a count-min sketch estimates every item and a short list of
// candidates keeps the ones with the highest estimates.
type SketchTopK struct {
	cm   *CountMin
	cand map[uint32]int
	cap  int
}

func NewSketchTopK(k, width, depth int, seed uint32) *SketchTopK {
	return &SketchTopK{cm: NewCountMin(width, depth, seed), cand: map[uint32]int{}, cap: 3 * k}
}

func (s *SketchTopK) Counters() int   { return len(s.cm.t) * s.cm.w }
func (s *SketchTopK) Candidates() int { return len(s.cand) }

func (s *SketchTopK) Add(x uint32) {
	s.cm.Add(x, 1)
	est := s.cm.Estimate(x)
	if _, ok := s.cand[x]; ok || len(s.cand) < s.cap {
		s.cand[x] = est
		return
	}
	lowKey, low := uint32(0), int(^uint(0)>>1)
	for k, v := range s.cand { // the weakest candidate, ties by smallest key so the result is reproducible
		if v < low || (v == low && k < lowKey) {
			lowKey, low = k, v
		}
	}
	if est > low {
		delete(s.cand, lowKey)
		s.cand[x] = est
	}
}

func (s *SketchTopK) Top(k int) []uint32 { return topOf(s.cand, k) }

func topOf(m map[uint32]int, k int) []uint32 {
	keys := make([]uint32, 0, len(m))
	for x := range m {
		keys = append(keys, x)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > k {
		keys = keys[:k]
	}
	return keys
}
